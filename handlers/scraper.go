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
	"strings"

	"mimusic-plugin-musictag/scraper"

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
}

// NewScraperHandler 创建刮削处理器
func NewScraperHandler(manager *scraper.Manager) *ScraperHandler {
	return &ScraperHandler{
		manager: manager,
	}
}

// HandleBatchScrape 处理批量刮削请求
func (h *ScraperHandler) HandleBatchScrape(req *http.Request) (*plugin.RouterResponse, error) {
	if req.Method != http.MethodPost {
		return &plugin.RouterResponse{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       []byte("Method not allowed"),
		}, nil
	}

	// 解析请求参数
	var request struct {
		PlaylistIDs []int64 `json:"playlist_ids"`
	}

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		return &plugin.RouterResponse{
			StatusCode: http.StatusBadRequest,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(`{"success":false,"message":"无效的请求参数"}`),
		}, nil
	}

	if len(request.PlaylistIDs) == 0 {
		return &plugin.RouterResponse{
			StatusCode: http.StatusBadRequest,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(`{"success":false,"message":"请选择至少一个歌单"}`),
		}, nil
	}

	hostFunctions := pbplugin.NewHostFunctions()

	// 收集所有需要刮削的歌曲（使用 map 去重）
	songMap := make(map[int64]scraper.SongInfo)

	// 遍历每个歌单，获取歌曲列表
	for _, playlistID := range request.PlaylistIDs {
		// 获取歌单歌曲列表
		resp, err := hostFunctions.CallRouter(req.Context(), &pbplugin.CallRouterRequest{
			Method: "GET",
			Path:   fmt.Sprintf("/api/v1/playlists/%d/songs?limit=100000", playlistID),
		})

		if err != nil {
			slog.Error("CallRouter failed for playlist", "playlistID", playlistID, "error", err)
			continue // 跳过失败的歌单，继续处理其他歌单
		}

		if !resp.Success {
			slog.Error("CallRouter returned error for playlist", "playlistID", playlistID, "statusCode", resp.StatusCode, "message", resp.Message)
			continue // 跳过失败的歌单，继续处理其他歌单
		}

		// 解析响应
		var songsResp struct {
			Songs []struct {
				ID       int64  `json:"id"`
				FilePath string `json:"file_path"`
				Artist   string `json:"artist"`
			} `json:"songs"`
		}

		if err := json.Unmarshal(resp.Body, &songsResp); err != nil {
			slog.Error("Failed to parse songs response for playlist", "playlistID", playlistID, "error", err)
			continue // 跳过失败的歌单，继续处理其他歌单
		}

		// 提取歌曲信息（去重）
		for _, song := range songsResp.Songs {
			filePath := song.FilePath
			fileName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
			// 清理标题中的序号前缀，如 "01.歌曲名" -> "歌曲名"
			cleanTitle := titlePrefixPattern.ReplaceAllString(fileName, "")
			// 清理 YouTube ID，如 "歌曲名 [xxx]" -> "歌曲名"
			cleanTitle = youtubeIDPattern.ReplaceAllString(cleanTitle, "")
			// 将分隔符替换为空格
			cleanTitle = extraInfoPattern.ReplaceAllString(cleanTitle, " ")
			// 清理多余空格
			cleanTitle = strings.TrimSpace(cleanTitle)
			slog.Info("清理歌曲标题", "songID", song.ID, "original", fileName, "cleaned", cleanTitle)
			songMap[song.ID] = scraper.SongInfo{
				ID:     song.ID,
				Title:  cleanTitle,
				Artist: song.Artist,
			}
		}
	}

	// 将 map 转换为 slice
	songs := make([]scraper.SongInfo, 0, len(songMap))
	for _, song := range songMap {
		songs = append(songs, song)
	}

	if len(songs) == 0 {
		return &plugin.RouterResponse{
			StatusCode: http.StatusBadRequest,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(`{"success":false,"message":"所选歌单中没有歌曲"}`),
		}, nil
	}

	taskID, err := h.manager.StartBatchScrape(songs)
	if err != nil {
		return &plugin.RouterResponse{
			StatusCode: http.StatusInternalServerError,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(`{"success":false,"message":"` + err.Error() + `"}`),
		}, nil
	}

	return &plugin.RouterResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte(`{"success":true,"task_id":"` + taskID + `"}`),
	}, nil
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
