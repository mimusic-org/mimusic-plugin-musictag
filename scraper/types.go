//go:build wasip1
// +build wasip1

package scraper

import (
	"fmt"
	"time"
)

// TaskStatus 任务状态
type TaskStatus struct {
	Status      string     `json:"status"`
	Total       int        `json:"total"`
	Current     int        `json:"current"`
	Success     int        `json:"success"`
	Failed      int        `json:"failed"`
	CurrentSong string     `json:"current_song"`
	Error       string     `json:"error"`
	Songs       []SongInfo `json:"songs"`        // 待刮削的歌曲信息列表
	FailedSongs []SongInfo `json:"failed_songs"` // 刮削失败的歌曲列表
}

// SongInfo 歌曲信息
type SongInfo struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
}

// generateTaskID 生成唯一任务 ID
func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}

// SongMetadata 歌曲元数据
type SongMetadata struct {
	Title    string `json:"title"`     // 歌曲标题
	Artist   string `json:"artist"`    // 艺术家/歌手
	Album    string `json:"album"`     // 专辑名称
	Lyric    string `json:"lyric"`     // 歌词内容
	CoverURL string `json:"cover_url"` // 封面图片 URL
}

// ScrapeRequest 刮削请求
type ScrapeRequest struct {
	SongID      int64  `json:"song_id"`      // 歌曲 ID
	SearchQuery string `json:"search_query"` // 搜索关键词（可选，为空则使用歌曲文件名）
}

// ScrapeResult 刮削结果
type ScrapeResult struct {
	Success  bool          `json:"success"`
	Message  string        `json:"message"`
	Metadata *SongMetadata `json:"metadata,omitempty"`
}

// PlatformSong 平台歌曲信息
type PlatformSong struct {
	ID       string                 `json:"id"`        // 平台歌曲 ID
	Title    string                 `json:"title"`     // 歌曲标题
	Artist   string                 `json:"artist"`    // 艺术家
	Album    string                 `json:"album"`     // 专辑
	CoverURL string                 `json:"cover_url"` // 封面 URL
	Score    float64                `json:"score"`     // 匹配度评分
	Source   string                 `json:"source"`    // 平台标识 (wy/tx)
	rawInfo  map[string]interface{} // 原始平台信息（用于歌词获取）
}

// BatchScrapeRequest 批量刮削请求
type BatchScrapeRequest struct {
	SongIDs []int64 `json:"song_ids"`
}

// BatchScrapeResponse 批量刮削响应
type BatchScrapeResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

// ScrapeProgress 刮削进度
type ScrapeProgress struct {
	TaskID      string     `json:"task_id"`
	Status      string     `json:"status"` // pending, running, completed, failed
	Total       int        `json:"total"`
	Current     int        `json:"current"`
	Success     int        `json:"success"`
	Failed      int        `json:"failed"`
	CurrentSong string     `json:"current_song"`
	Error       string     `json:"error"`
	FailedSongs []SongInfo `json:"failed_songs"` // 刮削失败的歌曲列表
}
