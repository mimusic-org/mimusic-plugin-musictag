//go:build wasip1
// +build wasip1

package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"mimusic-plugin-musictag/scraper"
	"mimusic-plugin-musictag/storage"

	"github.com/mimusic-org/plugin/api/pbplugin"
	"github.com/mimusic-org/plugin/api/plugin"
)

// 用于匹配标题中的序号前缀，如 "01.歌曲名"、"1.歌曲名"、"01-歌曲名" 等
var titlePrefixPattern = regexp.MustCompile(`^\d+[\.\-\s]+`)

// 用于匹配并移除 YouTube ID，如 [cB7DIIG0ykk]
var youtubeIDPattern = regexp.MustCompile(`\s*\[[a-zA-Z0-9_-]+\]\s*$`)

// 用于匹配并移除常见的分隔符和多余信息
var extraInfoPattern = regexp.MustCompile(`\s*[-_]\s*`)

// ScraperHandler 刮削处理器
type ScraperHandler struct {
	manager *scraper.Manager
	storage *storage.Storage
}

// NewScraperHandler 创建刮削处理器
func NewScraperHandler(manager *scraper.Manager, store *storage.Storage) *ScraperHandler {
	return &ScraperHandler{
		manager: manager,
		storage: store,
	}
}

// HandleBatchScrape 处理批量刮削请求
// 支持两种模式：
// 1. 指定 song_ids：直接刮削指定歌曲
// 2. 指定 playlist_ids：获取歌单中的歌曲后刮削
func (h *ScraperHandler) HandleBatchScrape(req *http.Request) (*plugin.RouterResponse, error) {
	if req.Method != http.MethodPost {
		return plugin.ErrorResponse(http.StatusMethodNotAllowed, "Method not allowed"), nil
	}

	// 解析请求参数
	var request struct {
		PlaylistIDs []int64 `json:"playlist_ids"`
		SongIDs     []int64 `json:"song_ids"`
	}

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		return plugin.ErrorResponse(http.StatusBadRequest, "无效的请求参数"), nil
	}

	if len(request.PlaylistIDs) == 0 && len(request.SongIDs) == 0 {
		return plugin.ErrorResponse(http.StatusBadRequest, "请指定 song_ids 或 playlist_ids"), nil
	}

	var songs []scraper.SongInfo

	if len(request.SongIDs) > 0 {
		// 模式1：按 song_ids 获取歌曲信息
		songInfos, err := h.fetchSongsByIDs(req, request.SongIDs)
		if err != nil {
			return plugin.ErrorResponse(http.StatusInternalServerError, err.Error()), nil
		}
		songs = songInfos
	} else {
		// 模式2：按 playlist_ids 获取歌曲列表
		songInfos, err := h.fetchSongsByPlaylists(req, request.PlaylistIDs)
		if err != nil {
			return plugin.ErrorResponse(http.StatusInternalServerError, err.Error()), nil
		}
		songs = songInfos
	}

	if len(songs) == 0 {
		return plugin.ErrorResponse(http.StatusBadRequest, "没有找到需要刮削的歌曲"), nil
	}

	taskID, err := h.manager.StartBatchScrape(songs)
	if err != nil {
		return plugin.ErrorResponse(http.StatusInternalServerError, err.Error()), nil
	}

	return plugin.SuccessResponse(map[string]string{"task_id": taskID}), nil
}

// fetchSongsByIDs 根据歌曲 ID 列表获取歌曲信息
func (h *ScraperHandler) fetchSongsByIDs(req *http.Request, songIDs []int64) ([]scraper.SongInfo, error) {
	hostFunctions := pbplugin.NewHostFunctions()
	songs := make([]scraper.SongInfo, 0, len(songIDs))

	for _, songID := range songIDs {
		resp, err := hostFunctions.CallRouter(req.Context(), &pbplugin.CallRouterRequest{
			Method: "GET",
			Path:   fmt.Sprintf("/api/v1/songs/%d", songID),
		})
		if err != nil {
			slog.Error("获取歌曲信息失败", "songID", songID, "error", err)
			continue
		}
		if !resp.Success {
			slog.Error("获取歌曲信息返回错误", "songID", songID, "statusCode", resp.StatusCode)
			continue
		}

		var song struct {
			ID       int64  `json:"id"`
			Title    string `json:"title"`
			Artist   string `json:"artist"`
			FilePath string `json:"file_path"`
		}
		if err := json.Unmarshal(resp.Body, &song); err != nil {
			slog.Error("解析歌曲信息失败", "songID", songID, "error", err)
			continue
		}

		title := song.Title
		if title == "" && song.FilePath != "" {
			title = cleanFileName(song.FilePath)
		}

		songs = append(songs, scraper.SongInfo{
			ID:     song.ID,
			Title:  title,
			Artist: song.Artist,
		})
	}

	return songs, nil
}

// fetchSongsByPlaylists 根据歌单 ID 列表获取合并去重的歌曲列表
func (h *ScraperHandler) fetchSongsByPlaylists(req *http.Request, playlistIDs []int64) ([]scraper.SongInfo, error) {
	hostFunctions := pbplugin.NewHostFunctions()
	songMap := make(map[int64]scraper.SongInfo)

	for _, playlistID := range playlistIDs {
		resp, err := hostFunctions.CallRouter(req.Context(), &pbplugin.CallRouterRequest{
			Method: "GET",
			Path:   fmt.Sprintf("/api/v1/playlists/%d/songs?limit=100000", playlistID),
		})
		if err != nil {
			slog.Error("CallRouter failed for playlist", "playlistID", playlistID, "error", err)
			continue
		}
		if !resp.Success {
			slog.Error("CallRouter returned error for playlist", "playlistID", playlistID, "statusCode", resp.StatusCode, "message", resp.Message)
			continue
		}

		var songsResp struct {
			Songs []struct {
				ID       int64  `json:"id"`
				FilePath string `json:"file_path"`
				Artist   string `json:"artist"`
			} `json:"songs"`
		}
		if err := json.Unmarshal(resp.Body, &songsResp); err != nil {
			slog.Error("Failed to parse songs response for playlist", "playlistID", playlistID, "error", err)
			continue
		}

		for _, song := range songsResp.Songs {
			if _, exists := songMap[song.ID]; exists {
				continue
			}
			title := cleanFileName(song.FilePath)
			songMap[song.ID] = scraper.SongInfo{
				ID:     song.ID,
				Title:  title,
				Artist: song.Artist,
			}
		}
	}

	songs := make([]scraper.SongInfo, 0, len(songMap))
	for _, song := range songMap {
		songs = append(songs, song)
	}
	return songs, nil
}

// cleanFileName 从文件路径中提取并清理歌曲标题
func cleanFileName(filePath string) string {
	fileName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	cleanTitle := titlePrefixPattern.ReplaceAllString(fileName, "")
	cleanTitle = youtubeIDPattern.ReplaceAllString(cleanTitle, "")
	cleanTitle = extraInfoPattern.ReplaceAllString(cleanTitle, " ")
	cleanTitle = strings.TrimSpace(cleanTitle)
	return cleanTitle
}

// HandleStopScrape 处理停止刮削请求
func (h *ScraperHandler) HandleStopScrape(req *http.Request) (*plugin.RouterResponse, error) {
	if req.Method != http.MethodPost {
		return &plugin.RouterResponse{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       []byte("Method not allowed"),
		}, nil
	}

	var request struct {
		TaskID string `json:"task_id"`
	}

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		return &plugin.RouterResponse{
			StatusCode: http.StatusBadRequest,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(`{"success":false,"message":"无效的请求参数"}`),
		}, nil
	}

	if err := h.manager.StopTask(request.TaskID); err != nil {
		return &plugin.RouterResponse{
			StatusCode: http.StatusBadRequest,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(`{"success":false,"message":"` + err.Error() + `"}`),
		}, nil
	}

	return &plugin.RouterResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte(`{"success":true,"message":"已停止刮削"}`),
	}, nil
}

// HandleGetStatus 处理获取刮削进度请求
func (h *ScraperHandler) HandleGetStatus(req *http.Request) (*plugin.RouterResponse, error) {
	taskID := req.URL.Query().Get("task_id")
	if taskID == "" {
		return &plugin.RouterResponse{
			StatusCode: 400,
			Body:       []byte(`{"success":false,"message":"task_id is required"}`),
		}, nil
	}

	progress := h.manager.GetProgress(taskID)

	responseBody, _ := json.Marshal(progress)
	return &plugin.RouterResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       responseBody,
	}, nil
}

// HandleRetryFailed 处理重新刮削失败歌曲请求
func (h *ScraperHandler) HandleRetryFailed(req *http.Request) (*plugin.RouterResponse, error) {
	if req.Method != http.MethodPost {
		return &plugin.RouterResponse{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       []byte("Method not allowed"),
		}, nil
	}

	var request struct {
		TaskID string `json:"task_id"`
	}

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		return &plugin.RouterResponse{
			StatusCode: http.StatusBadRequest,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(`{"success":false,"message":"无效的请求参数"}`),
		}, nil
	}

	newTaskID, err := h.manager.RetryFailedSongs(request.TaskID)
	if err != nil {
		return &plugin.RouterResponse{
			StatusCode: http.StatusBadRequest,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(`{"success":false,"message":"` + err.Error() + `"}`),
		}, nil
	}

	return &plugin.RouterResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte(`{"success":true,"task_id":"` + newTaskID + `","message":"已开始重新刮削失败歌曲"}`),
	}, nil
}

// SongWithStatus 带刮削状态的歌曲信息
type SongWithStatus struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	Scraped      bool   `json:"scraped"`
	ScrapedAt    string `json:"scraped_at,omitempty"`
	ScrapeSource string `json:"scrape_source,omitempty"`
	ScrapeFailed bool   `json:"scrape_failed"`
}

// HandleGetSongs 获取歌单中的歌曲列表（合并去重），附带刮削状态
func (h *ScraperHandler) HandleGetSongs(req *http.Request) (*plugin.RouterResponse, error) {
	if req.Method != http.MethodPost {
		return plugin.ErrorResponse(http.StatusMethodNotAllowed, "Method not allowed"), nil
	}

	var request struct {
		PlaylistIDs []int64 `json:"playlist_ids"`
	}

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		return plugin.ErrorResponse(http.StatusBadRequest, "无效的请求参数"), nil
	}

	if len(request.PlaylistIDs) == 0 {
		return plugin.ErrorResponse(http.StatusBadRequest, "请选择至少一个歌单"), nil
	}

	hostFunctions := pbplugin.NewHostFunctions()

	// 收集所有歌曲（去重）
	songMap := make(map[int64]*SongWithStatus)
	songOrder := make([]int64, 0) // 保持顺序

	for _, playlistID := range request.PlaylistIDs {
		resp, err := hostFunctions.CallRouter(req.Context(), &pbplugin.CallRouterRequest{
			Method: "GET",
			Path:   fmt.Sprintf("/api/v1/playlists/%d/songs?limit=100000", playlistID),
		})
		if err != nil {
			slog.Error("获取歌单歌曲失败", "playlistID", playlistID, "error", err)
			continue
		}
		if !resp.Success {
			slog.Error("获取歌单歌曲返回错误", "playlistID", playlistID, "statusCode", resp.StatusCode)
			continue
		}

		var songsResp struct {
			Songs []struct {
				ID       int64  `json:"id"`
				Title    string `json:"title"`
				Artist   string `json:"artist"`
				Album    string `json:"album"`
				FilePath string `json:"file_path"`
			} `json:"songs"`
		}
		if err := json.Unmarshal(resp.Body, &songsResp); err != nil {
			slog.Error("解析歌曲列表失败", "playlistID", playlistID, "error", err)
			continue
		}

		for _, song := range songsResp.Songs {
			if _, exists := songMap[song.ID]; exists {
				continue
			}

			title := song.Title
			if title == "" && song.FilePath != "" {
				title = cleanFileName(song.FilePath)
			}

			songWithStatus := &SongWithStatus{
				ID:     song.ID,
				Title:  title,
				Artist: song.Artist,
				Album:  song.Album,
			}

			// 查询刮削状态
			if h.storage != nil {
				if record, ok := h.storage.GetRecord(song.ID); ok {
					songWithStatus.Scraped = record.Success
					songWithStatus.ScrapedAt = record.ScrapedAt
					songWithStatus.ScrapeSource = record.Source
					songWithStatus.ScrapeFailed = !record.Success
				}
			}

			songMap[song.ID] = songWithStatus
			songOrder = append(songOrder, song.ID)
		}
	}

	// 按顺序构建结果
	songs := make([]*SongWithStatus, 0, len(songOrder))
	for _, id := range songOrder {
		songs = append(songs, songMap[id])
	}

	result := map[string]interface{}{
		"songs": songs,
		"total": len(songs),
	}

	responseBody, _ := json.Marshal(result)
	return &plugin.RouterResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       responseBody,
	}, nil
}

// HandleClearRecords 清除刮削记录
func (h *ScraperHandler) HandleClearRecords(req *http.Request) (*plugin.RouterResponse, error) {
	if req.Method != http.MethodDelete {
		return plugin.ErrorResponse(http.StatusMethodNotAllowed, "Method not allowed"), nil
	}

	var request struct {
		SongIDs []int64 `json:"song_ids"`
	}

	// Body 可能为空（清空所有记录）
	if req.Body != nil {
		_ = json.NewDecoder(req.Body).Decode(&request)
	}

	if h.storage == nil {
		return plugin.ErrorResponse(http.StatusInternalServerError, "存储未初始化"), nil
	}

	var err error
	if len(request.SongIDs) > 0 {
		err = h.storage.RemoveRecords(request.SongIDs)
	} else {
		err = h.storage.ClearRecords()
	}

	if err != nil {
		return plugin.ErrorResponse(http.StatusInternalServerError, "清除记录失败: "+err.Error()), nil
	}

	count := "所有"
	if len(request.SongIDs) > 0 {
		count = strconv.Itoa(len(request.SongIDs)) + " 条"
	}

	return plugin.SuccessResponse(map[string]string{
		"message": "已清除 " + count + " 刮削记录",
	}), nil
}
