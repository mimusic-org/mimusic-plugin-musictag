//go:build wasip1
// +build wasip1

// Package scraper 提供音乐标签刮削功能。
package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/mimusic-org/plugin/api/pbplugin"
	"github.com/mimusic-org/plugin/api/plugin"
	"github.com/mimusic-org/plugin/pkg/go-plugin-http/http"
)

// Manager 管理刮削任务
type Manager struct {
	currentTask *TaskStatus // 当前任务（WASM 单线程，只支持一个任务）
}

// NewManager 创建一个新的刮削管理器
func NewManager() *Manager {
	return &Manager{}
}

// Close 关闭管理器
func (m *Manager) Close() {
	m.currentTask = nil
}

// ScrapeWithInfo 使用已有歌曲信息刮削（批量刮削使用，避免重复请求）
func (m *Manager) ScrapeWithInfo(ctx context.Context, songInfo SongInfo) *ScrapeResult {
	slog.Info("开始刮削歌曲", "songID", songInfo.ID, "title", songInfo.Title)
	// 使用独立的 context，避免被外部取消
	return m.scrapeWithInfo(context.Background(), songInfo, "")
}

// scrapeWithInfo 内部刮削实现
func (m *Manager) scrapeWithInfo(ctx context.Context, songInfo SongInfo, searchQuery string) *ScrapeResult {

	// 2. 确定搜索关键词
	query := searchQuery
	if query == "" {
		query = fmt.Sprintf("%s %s", songInfo.Title, songInfo.Artist)
	}

	// 3. 从三个平台搜索
	var allPlatformSongs []PlatformSong

	// 网易云音乐
	neteaseSongs := m.searchNetease(query)
	allPlatformSongs = append(allPlatformSongs, neteaseSongs...)

	// QQ 音乐
	qqSongs := m.searchQQ(query)
	allPlatformSongs = append(allPlatformSongs, qqSongs...)

	if len(allPlatformSongs) == 0 {
		return &ScrapeResult{
			Success: false,
			Message: "未找到匹配的歌曲",
		}
	}

	// 4. 计算匹配度并排序
	for i := range allPlatformSongs {
		allPlatformSongs[i].Score = CalculateScore(
			songInfo.Title,
			songInfo.Artist,
			allPlatformSongs[i].Title,
			allPlatformSongs[i].Artist,
			"", // songInfo 没有 Album 字段
		)
	}

	// 按评分排序
	bestMatch := allPlatformSongs[0]
	for _, song := range allPlatformSongs[1:] {
		if song.Score > bestMatch.Score {
			bestMatch = song
		}
	}

	// 5. 获取详细信息（歌词等）
	metadata := m.getSongDetail(bestMatch.ID, bestMatch.Title)

	if metadata.Title == "" {
		metadata.Title = bestMatch.Title
	}
	if metadata.Artist == "" {
		metadata.Artist = bestMatch.Artist
	}
	if metadata.Album == "" {
		metadata.Album = bestMatch.Album
	}
	if metadata.CoverURL == "" {
		metadata.CoverURL = bestMatch.CoverURL
	}

	metadata.Title = mergeTitle(songInfo.Title, metadata.Title)

	// 6. 更新歌曲元数据
	if err := m.updateSongMetadata(ctx, songInfo.ID, metadata); err != nil {
		return &ScrapeResult{
			Success:  false,
			Message:  fmt.Sprintf("更新歌曲元数据失败：%v", err),
			Metadata: metadata,
		}
	}

	slog.Info("刮削成功", "songID", songInfo.ID, "metadata", metadata)

	return &ScrapeResult{
		Success:  true,
		Message:  "刮削成功",
		Metadata: metadata,
	}
}

// StartBatchScrape 开始批量刮削任务
func (m *Manager) StartBatchScrape(songs []SongInfo) (string, error) {
	// 检查是否已有任务在运行
	if m.currentTask != nil && m.currentTask.Status == "running" {
		return "", fmt.Errorf("已有任务正在运行")
	}

	// 初始化当前任务
	m.currentTask = &TaskStatus{
		Status:      "running",
		Total:       len(songs),
		Current:     0,
		Success:     0,
		Failed:      0,
		Songs:       songs,
		FailedSongs: make([]SongInfo, 0),
	}

	// 注册定时器，100ms 后开始执行
	tm := plugin.GetTimerManager()
	tm.RegisterDelayTimer(context.Background(), 100, m.processNextSong)

	return "current", nil
}

// processNextSong 处理下一首歌曲（定时器回调）
func (m *Manager) processNextSong() {
	if m.currentTask == nil {
		return
	}

	// 检查是否已停止
	if m.currentTask.Status == "stopped" {
		slog.Info("任务已停止")
		return
	}

	// 检查是否已完成
	if m.currentTask.Current >= m.currentTask.Total {
		if m.currentTask.Failed > 0 {
			m.currentTask.Status = "completed_with_errors"
		} else {
			m.currentTask.Status = "completed"
		}
		slog.Info("批量刮削完成", "success", m.currentTask.Success, "failed", m.currentTask.Failed)
		return
	}

	// 处理下一首歌曲
	songInfo := m.currentTask.Songs[m.currentTask.Current]
	m.currentTask.Current++

	slog.Info("开始刮削歌曲", "songID", songInfo.ID, "progress", fmt.Sprintf("%d/%d", m.currentTask.Current, m.currentTask.Total))

	// 执行刮削
	result := m.ScrapeWithInfo(context.Background(), songInfo)

	if result.Success {
		m.currentTask.Success++
		if result.Metadata != nil && result.Metadata.Title != "" {
			m.currentTask.CurrentSong = result.Metadata.Title
		}
	} else {
		m.currentTask.Failed++
		// 记录失败的歌曲
		m.currentTask.FailedSongs = append(m.currentTask.FailedSongs, songInfo)
		slog.Warn("刮削失败", "songID", songInfo.ID, "error", result.Message)
	}

	// 注册下一个定时器，继续处理下一首
	tm := plugin.GetTimerManager()
	tm.RegisterDelayTimer(context.Background(), 100, m.processNextSong)
}

// GetProgress 获取刮削进度
func (m *Manager) GetProgress(taskID string) *ScrapeProgress {
	if m.currentTask == nil {
		return &ScrapeProgress{
			TaskID: taskID,
			Status: "not_found",
		}
	}

	return &ScrapeProgress{
		TaskID:      "current",
		Status:      m.currentTask.Status,
		Total:       m.currentTask.Total,
		Current:     m.currentTask.Current,
		Success:     m.currentTask.Success,
		Failed:      m.currentTask.Failed,
		CurrentSong: m.currentTask.CurrentSong,
		Error:       m.currentTask.Error,
		FailedSongs: m.currentTask.FailedSongs,
	}
}

// StopTask 停止任务
func (m *Manager) StopTask(taskID string) error {
	if m.currentTask == nil {
		return fmt.Errorf("任务不存在")
	}

	m.currentTask.Status = "stopped"
	slog.Info("任务已停止")
	return nil
}

// RetryFailedSongs 重新刮削失败的歌曲
func (m *Manager) RetryFailedSongs(taskID string) (string, error) {
	if m.currentTask == nil || len(m.currentTask.FailedSongs) == 0 {
		return "", fmt.Errorf("没有失败的歌曲需要重新刮削")
	}

	// 保存失败歌曲列表
	failedSongs := m.currentTask.FailedSongs

	// 创建新任务
	m.currentTask = &TaskStatus{
		Status:      "running",
		Total:       len(failedSongs),
		Current:     0,
		Success:     0,
		Failed:      0,
		Songs:       failedSongs,
		FailedSongs: make([]SongInfo, 0),
	}

	// 注册定时器，100ms 后开始执行
	tm := plugin.GetTimerManager()
	tm.RegisterDelayTimer(context.Background(), 100, m.processNextSong)

	return "current", nil
}

// updateSongMetadata 更新歌曲元数据
func (m *Manager) updateSongMetadata(ctx context.Context, songID int64, metadata *SongMetadata) error {
	hostFunctions := pbplugin.NewHostFunctions()

	// 构建更新请求体
	updateData := map[string]interface{}{
		"title":  metadata.Title,
		"artist": metadata.Artist,
		"album":  metadata.Album,
	}

	if metadata.CoverURL != "" {
		updateData["cover_url"] = metadata.CoverURL
	}

	body, err := json.Marshal(updateData)
	if err != nil {
		return fmt.Errorf("marshal update data failed: %w", err)
	}

	slog.Info("更新歌曲元数据", "songID", songID, "body", string(body))

	resp, err := hostFunctions.CallRouter(ctx, &pbplugin.CallRouterRequest{
		Method: "PUT",
		Path:   fmt.Sprintf("/api/v1/songs/%d", songID),
		Body:   body,
	})

	if err != nil {
		return fmt.Errorf("call router failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("update failed: %s %s", resp.Message, resp.Body)
	}

	// 如果有歌词，也更新歌词
	if metadata.Lyric != "" {
		lyricsData := map[string]string{
			"lyrics": metadata.Lyric,
		}
		lyricsBody, _ := json.Marshal(lyricsData)

		_, err := hostFunctions.CallRouter(ctx, &pbplugin.CallRouterRequest{
			Method: "PUT",
			Path:   fmt.Sprintf("/api/v1/songs/%d/lyrics", songID),
			Body:   lyricsBody,
		})

		if err != nil {
			slog.Warn("更新歌词失败", "songID", songID, "error", err)
		}
	}

	slog.Info("歌曲元数据已更新", "songID", songID, "title", metadata.Title)
	return nil
}

// searchNetease 网易云音乐搜索
func (m *Manager) searchNetease(query string) []PlatformSong {
	// 网易云音乐搜索 API: https://music.163.com/api/search/get
	url := fmt.Sprintf("https://music.163.com/api/search/get?s=%s&type=1&limit=10", query)

	resp, err := http.Get(url)
	if err != nil {
		slog.Warn("网易云音乐搜索失败", "query", query, "error", err)
		return []PlatformSong{}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("网易云音乐读取响应失败", "error", err)
		return []PlatformSong{}
	}

	// 解析响应
	var result struct {
		Result struct {
			Songs []struct {
				ID      int64  `json:"id"`
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name   string `json:"name"`
					PicURL string `json:"picUrl"`
				} `json:"album"`
			} `json:"songs"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		slog.Warn("网易云音乐解析响应失败", "error", err)
		return []PlatformSong{}
	}

	songs := make([]PlatformSong, 0, len(result.Result.Songs))
	for _, song := range result.Result.Songs {
		artistNames := make([]string, len(song.Artists))
		for i, artist := range song.Artists {
			artistNames[i] = artist.Name
		}

		songs = append(songs, PlatformSong{
			ID:       fmt.Sprintf("netease_%d", song.ID),
			Title:    song.Name,
			Artist:   strings.Join(artistNames, "/"),
			Album:    song.Album.Name,
			CoverURL: song.Album.PicURL,
		})
	}

	return songs
}

// searchQQ QQ 音乐搜索
func (m *Manager) searchQQ(query string) []PlatformSong {
	// QQ 音乐搜索 API
	url := fmt.Sprintf("https://c.y.qq.com/soso/fcgi-bin/client_search_cp?w=%s&format=json&p=1&n=10", query)

	resp, err := http.Get(url)
	if err != nil {
		slog.Warn("QQ 音乐搜索失败", "query", query, "error", err)
		return []PlatformSong{}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("QQ 音乐读取响应失败", "error", err)
		return []PlatformSong{}
	}

	// 解析响应（QQ 音乐返回 JSONP，需要去除回调函数）
	jsonStr := strings.TrimPrefix(string(body), "callback(")
	jsonStr = strings.TrimSuffix(jsonStr, ")")

	var result struct {
		Data struct {
			Song struct {
				List []struct {
					SongID   int64  `json:"songid"`
					SongName string `json:"songname"`
					Singer   []struct {
						Name string `json:"name"`
					} `json:"singer"`
					AlbumName string `json:"albumname"`
					AlbumMid  string `json:"albummid"`
				} `json:"list"`
			} `json:"song"`
		} `json:"data"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		slog.Warn("QQ 音乐解析响应失败", "error", err)
		return []PlatformSong{}
	}

	songs := make([]PlatformSong, 0, len(result.Data.Song.List))
	for _, song := range result.Data.Song.List {
		artistNames := make([]string, len(song.Singer))
		for i, singer := range song.Singer {
			artistNames[i] = singer.Name
		}

		// QQ 音乐封面 URL
		coverURL := fmt.Sprintf("https://y.gtimg.cn/music/photo_new/T002R300x300M000%s.jpg", song.AlbumMid)

		songs = append(songs, PlatformSong{
			ID:       fmt.Sprintf("qq_%d", song.SongID),
			Title:    song.SongName,
			Artist:   strings.Join(artistNames, "/"),
			Album:    song.AlbumName,
			CoverURL: coverURL,
		})
	}

	return songs
}

// getSongDetail 获取歌曲详情（歌词等）
func (m *Manager) getSongDetail(songID, title string) *SongMetadata {
	// 根据歌曲 ID 前缀判断平台
	if strings.HasPrefix(songID, "netease_") {
		return m.getNeteaseLyric(songID)
	} else if strings.HasPrefix(songID, "qq_") {
		return m.getQQLyric(songID)
	}

	return &SongMetadata{}
}

// getNeteaseLyric 获取网易云音乐歌词
func (m *Manager) getNeteaseLyric(songID string) *SongMetadata {
	// 提取歌曲 ID
	id := strings.TrimPrefix(songID, "netease_")
	url := fmt.Sprintf("https://music.163.com/api/song/lyric?id=%s&lv=1", id)

	resp, err := http.Get(url)
	if err != nil {
		slog.Warn("网易云音乐获取歌词失败", "songID", songID, "error", err)
		return &SongMetadata{}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("网易云音乐读取歌词失败", "error", err)
		return &SongMetadata{}
	}

	var result struct {
		Lrc struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		slog.Warn("网易云音乐解析歌词失败", "error", err)
		return &SongMetadata{}
	}

	return &SongMetadata{
		Lyric: result.Lrc.Lyric,
	}
}

// getQQLyric 获取 QQ 音乐歌词
func (m *Manager) getQQLyric(songID string) *SongMetadata {
	// 提取歌曲 ID
	id := strings.TrimPrefix(songID, "qq_")
	url := fmt.Sprintf("https://c.y.qq.com/lyric/fcgi-bin/fcg_query_lyric_new.fcg?songmid=%s&format=json", id)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &SongMetadata{}
	}

	req.Header.Set("Referer", "https://y.qq.com/")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("QQ 音乐获取歌词失败", "songID", songID, "error", err)
		return &SongMetadata{}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("QQ 音乐读取歌词失败", "error", err)
		return &SongMetadata{}
	}

	var result struct {
		Lyric string `json:"lyric"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		slog.Warn("QQ 音乐解析歌词失败", "error", err)
		return &SongMetadata{}
	}

	// QQ 音乐歌词是 base64 编码的，这里简化处理，直接返回
	return &SongMetadata{
		Lyric: result.Lyric,
	}
}
