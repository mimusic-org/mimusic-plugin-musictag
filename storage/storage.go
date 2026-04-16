//go:build wasip1
// +build wasip1

package storage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

const (
	DataVersion = "1.0"
	FileName    = "scrape_records.json"
)

// ScrapeRecord 刮削记录
type ScrapeRecord struct {
	SongID    int64  `json:"song_id"`
	Success   bool   `json:"success"`
	Source    string `json:"source"`     // 匹配平台 wy/tx/kw/kg
	ScrapedAt string `json:"scraped_at"` // ISO 8601 时间
}

// ScrapeData 持久化数据结构
type ScrapeData struct {
	Version string                  `json:"version"`
	Records map[string]ScrapeRecord `json:"records"` // key: songID 字符串
}

// Storage 刮削记录存储
type Storage struct {
	data     *ScrapeData
	filePath string
}

// NewStorage 创建存储实例
func NewStorage(baseDir string) (*Storage, error) {
	// 确保目录存在
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w, baseDir: %s", err, baseDir)
	}

	filePath := baseDir + "/" + FileName
	s := &Storage{
		filePath: filePath,
		data: &ScrapeData{
			Version: DataVersion,
			Records: make(map[string]ScrapeRecord),
		},
	}

	// 尝试加载现有数据
	if err := s.Load(); err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，创建新文件
			if err := s.Save(); err != nil {
				return nil, fmt.Errorf("failed to create storage file: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to load storage: %w", err)
		}
	}

	slog.Info("刮削记录存储已初始化", "filePath", filePath, "records", len(s.data.Records))
	return s, nil
}

// Load 加载数据文件
func (s *Storage) Load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	var scrapeData ScrapeData
	if err := json.Unmarshal(data, &scrapeData); err != nil {
		return fmt.Errorf("failed to parse storage data: %w", err)
	}

	if scrapeData.Records == nil {
		scrapeData.Records = make(map[string]ScrapeRecord)
	}

	s.data = &scrapeData
	return nil
}

// Save 保存数据文件
func (s *Storage) Save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal storage data: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write storage file: %w", err)
	}

	return nil
}

// RecordScrape 记录刮削结果
func (s *Storage) RecordScrape(songID int64, success bool, source string) {
	key := strconv.FormatInt(songID, 10)
	s.data.Records[key] = ScrapeRecord{
		SongID:    songID,
		Success:   success,
		Source:    source,
		ScrapedAt: time.Now().Format(time.RFC3339),
	}

	if err := s.Save(); err != nil {
		slog.Error("保存刮削记录失败", "songID", songID, "error", err)
	}
}

// IsScraped 查询歌曲是否已刮削
func (s *Storage) IsScraped(songID int64) bool {
	key := strconv.FormatInt(songID, 10)
	_, exists := s.data.Records[key]
	return exists
}

// GetRecord 获取单条刮削记录
func (s *Storage) GetRecord(songID int64) (ScrapeRecord, bool) {
	key := strconv.FormatInt(songID, 10)
	record, exists := s.data.Records[key]
	return record, exists
}

// GetAllRecords 获取所有刮削记录
func (s *Storage) GetAllRecords() map[string]ScrapeRecord {
	result := make(map[string]ScrapeRecord, len(s.data.Records))
	for k, v := range s.data.Records {
		result[k] = v
	}
	return result
}

// RemoveRecords 移除指定歌曲的刮削记录
func (s *Storage) RemoveRecords(songIDs []int64) error {
	for _, id := range songIDs {
		key := strconv.FormatInt(id, 10)
		delete(s.data.Records, key)
	}
	return s.Save()
}

// ClearRecords 清空所有刮削记录
func (s *Storage) ClearRecords() error {
	s.data.Records = make(map[string]ScrapeRecord)
	return s.Save()
}
