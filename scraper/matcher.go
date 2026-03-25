//go:build wasip1
// +build wasip1

package scraper

import (
	"strings"
)

// CalculateScore 计算匹配度评分
// localTitle, localArtist: 本地歌曲的标题和艺术家
// platformTitle, platformArtist, platformAlbum: 平台歌曲的标题、艺术家、专辑
func CalculateScore(localTitle, localArtist, platformTitle, platformArtist, platformAlbum string) float64 {
	titleScore := calculateTextSimilarity(localTitle, platformTitle)
	artistScore := calculateTextSimilarity(localArtist, platformArtist)
	albumScore := calculateTextSimilarity("", platformAlbum)

	// 标题权重 50%，艺术家 30%，专辑 20%
	return titleScore*0.5 + artistScore*0.3 + albumScore*0.2
}

// calculateTextSimilarity 计算文本相似度 (0-1)
func calculateTextSimilarity(s1, s2 string) float64 {
	if s1 == "" && s2 == "" {
		return 1.0
	}
	if s1 == "" || s2 == "" {
		return 0.0
	}

	// 转换为小写进行比较
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	// 完全匹配
	if s1 == s2 {
		return 1.0
	}

	// 包含匹配
	if strings.Contains(s1, s2) || strings.Contains(s2, s1) {
		return 0.8
	}

	// 使用编辑距离计算相似度
	distance := levenshteinDistance(s1, s2)
	maxLen := max(len(s1), len(s2))
	if maxLen == 0 {
		return 1.0
	}

	similarity := 1.0 - float64(distance)/float64(maxLen)
	return similarity
}

// levenshteinDistance 计算编辑距离
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// 创建矩阵
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// 填充矩阵
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // 删除
				matrix[i][j-1]+1,      // 插入
				matrix[i-1][j-1]+cost, // 替换
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
