//go:build wasip1
// +build wasip1

// Package scraper 提供音乐标签刮削功能。
package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"mimusic-plugin-musictag/storage"

	"github.com/mimusic-org/musicsdk"
	"github.com/mimusic-org/plugin/api/pbplugin"
	"github.com/mimusic-org/plugin/api/plugin"
)

// Manager 管理刮削任务
type Manager struct {
	currentTask *TaskStatus        // 当前任务（WASM 单线程，只支持一个任务）
	registry    *musicsdk.Registry // 音乐平台搜索注册表
	storage     *storage.Storage   // 刮削记录持久化存储
}

// NewManager 创建一个新的刮削管理器
func NewManager(store *storage.Storage) *Manager {
	m := &Manager{
		registry: musicsdk.NewRegistry(),
		storage:  store,
	}
	// 注册搜索器
	m.registry.Register(musicsdk.NewWySearcher())
	m.registry.Register(musicsdk.NewTxSearcher())
	m.registry.Register(musicsdk.NewKwSearcher())
	m.registry.Register(musicsdk.NewKgSearcher())
	// 注册歌词获取器
	m.registry.RegisterLyricFetcher(musicsdk.NewWyLyricFetcher())
	m.registry.RegisterLyricFetcher(musicsdk.NewTxLyricFetcher())
	m.registry.RegisterLyricFetcher(musicsdk.NewKwLyricFetcher())
	m.registry.RegisterLyricFetcher(musicsdk.NewKgLyricFetcher())
	return m
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

	// 3. 从各平台搜索
	var allPlatformSongs []PlatformSong

	// wy 平台搜索
	wySearcher, ok := m.registry.Get("wy")
	if ok {
		result, err := wySearcher.Search(query, 1, 10)
		if err != nil {
			slog.Warn("搜索失败", "source", "wy", "query", query, "error", err)
		} else {
			for _, item := range result.List {
				allPlatformSongs = append(allPlatformSongs, PlatformSong{
					ID:       fmt.Sprintf("wy_%s", item.MusicID),
					Title:    item.Name,
					Artist:   item.Singer,
					Album:    item.Album,
					CoverURL: item.Img,
					Source:   "wy",
					// 保存原始信息用于歌词获取
					rawInfo: map[string]interface{}{
						"musicId": item.MusicID,
						"songmid": item.Songmid,
					},
				})
			}
		}
	}

	// tx 平台搜索
	txSearcher, ok := m.registry.Get("tx")
	if ok {
		result, err := txSearcher.Search(query, 1, 10)
		if err != nil {
			slog.Warn("搜索失败", "source", "tx", "query", query, "error", err)
		} else {
			for _, item := range result.List {
				allPlatformSongs = append(allPlatformSongs, PlatformSong{
					ID:       fmt.Sprintf("tx_%s", item.MusicID),
					Title:    item.Name,
					Artist:   item.Singer,
					Album:    item.Album,
					CoverURL: item.Img,
					Source:   "tx",
					// 保存原始信息用于歌词获取
					rawInfo: map[string]interface{}{
						"musicId":     item.MusicID,
						"songmid":     item.Songmid,
						"albummid":    item.AlbumMid,
						"strMediaMid": item.StrMediaMid,
					},
				})
			}
		}
	}

	// kw 平台搜索
	kwSearcher, ok := m.registry.Get("kw")
	if ok {
		result, err := kwSearcher.Search(query, 1, 10)
		if err != nil {
			slog.Warn("搜索失败", "source", "kw", "query", query, "error", err)
		} else {
			for _, item := range result.List {
				allPlatformSongs = append(allPlatformSongs, PlatformSong{
					ID:       fmt.Sprintf("kw_%s", item.MusicID),
					Title:    item.Name,
					Artist:   item.Singer,
					Album:    item.Album,
					CoverURL: item.Img,
					Source:   "kw",
					rawInfo: map[string]interface{}{
						"musicId": item.MusicID,
						"songmid": item.Songmid,
					},
				})
			}
		}
	}

	// kg 平台搜索
	kgSearcher, ok := m.registry.Get("kg")
	if ok {
		result, err := kgSearcher.Search(query, 1, 10)
		if err != nil {
			slog.Warn("搜索失败", "source", "kg", "query", query, "error", err)
		} else {
			for _, item := range result.List {
				allPlatformSongs = append(allPlatformSongs, PlatformSong{
					ID:       fmt.Sprintf("kg_%s", item.MusicID),
					Title:    item.Name,
					Artist:   item.Singer,
					Album:    item.Album,
					CoverURL: item.Img,
					Source:   "kg",
					rawInfo: map[string]interface{}{
						"musicId":  item.MusicID,
						"hash":     item.Hash,
						"name":     item.Name,
						"singer":   item.Singer,
						"duration": item.Duration,
					},
				})
			}
		}
	}

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
	metadata := m.getSongDetail(bestMatch)

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
		Source:   bestMatch.Source,
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
		// 记录刮削成功
		if m.storage != nil {
			m.storage.RecordScrape(songInfo.ID, true, result.Source)
		}
	} else {
		m.currentTask.Failed++
		// 记录失败的歌曲
		m.currentTask.FailedSongs = append(m.currentTask.FailedSongs, songInfo)
		slog.Warn("刮削失败", "songID", songInfo.ID, "error", result.Message)
		// 记录刮削失败
		if m.storage != nil {
			m.storage.RecordScrape(songInfo.ID, false, "")
		}
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

// getSongDetail 获取歌曲详情（歌词等）
func (m *Manager) getSongDetail(song PlatformSong) *SongMetadata {
	// 根据平台获取歌词
	source := song.Source
	fetcher, ok := m.registry.GetLyricFetcher(source)
	if !ok {
		slog.Warn("未找到歌词获取器", "source", source)
		return &SongMetadata{}
	}

	lyricResult, err := fetcher.GetLyric(song.rawInfo)
	if err != nil {
		slog.Warn("获取歌词失败", "source", source, "songID", song.ID, "error", err)
		return &SongMetadata{}
	}

	return &SongMetadata{
		Lyric: lyricResult.Lyric,
	}
}
