//go:build wasip1
// +build wasip1

// Package scraper 提供音乐标签刮削功能。
package scraper

import (
	"strings"
)

// mergeTitle 智能合并文件名和刮削标题
// 使用最长公共子串去重，避免信息冗余
// 修复：基于rune处理中文字符，避免乱码；移除中文的大小写转换
func mergeTitle(fileName, scrapedTitle string) string {
	// 如果刮削标题为空，只使用文件名
	if scrapedTitle == "" {
		return fileName
	}

	// 如果文件名为空，只使用刮削标题
	if fileName == "" {
		return scrapedTitle
	}

	// 直接使用原始字符串比较（中文无大小写）
	if fileName == scrapedTitle {
		return fileName
	}

	// 如果存在包含关系，返回更长的那个
	if strings.Contains(fileName, scrapedTitle) {
		return fileName
	}
	if strings.Contains(scrapedTitle, fileName) {
		return scrapedTitle
	}

	// 计算最长公共子串（基于rune处理，避免乱码）
	commonSubstr, _, _ := longestCommonSubstring(fileName, scrapedTitle)

	// 如果最长公共子串长度 > 0，去除该子串后拼接剩余部分
	if len(commonSubstr) > 0 {
		// 去除公共子串（使用原始字符串，避免重复处理）
		fileNameRemaining := strings.Replace(fileName, commonSubstr, "", 1)
		scrapedTitleRemaining := strings.Replace(scrapedTitle, commonSubstr, "", 1)

		// 清理多余的分隔符和空格
		fileNameRemaining = strings.TrimSpace(strings.Trim(fileNameRemaining, "-_"))
		scrapedTitleRemaining = strings.TrimSpace(strings.Trim(scrapedTitleRemaining, "-_"))

		// 拼接剩余部分
		var parts []string
		if fileNameRemaining != "" {
			parts = append(parts, fileNameRemaining)
		}
		if scrapedTitleRemaining != "" {
			parts = append(parts, scrapedTitleRemaining)
		}

		if len(parts) > 0 {
			return strings.Join(parts, " - ")
		}

		// 如果去除公共子串后都为空，返回刮削标题
		return scrapedTitle
	}

	// 没有公共子串，直接拼接
	return fileName + " - " + scrapedTitle
}

// longestCommonSubstring 计算两个字符串的最长公共子串（修复中文乱码问题）
// 基于rune处理字符，避免字节截断导致的乱码
// 返回公共子串及其在两个字符串中的起始字符位置（非字节位置）
func longestCommonSubstring(s1, s2 string) (string, int, int) {
	// 转换为rune切片，按字符处理
	r1 := []rune(s1)
	r2 := []rune(s2)

	if len(r1) == 0 || len(r2) == 0 {
		return "", 0, 0
	}

	// 创建动态规划表（基于字符数）
	dp := make([][]int, len(r1)+1)
	for i := range dp {
		dp[i] = make([]int, len(r2)+1)
	}

	maxLen := 0
	endPos1 := 0 // 字符索引
	endPos2 := 0 // 字符索引

	// 填充动态规划表（比较rune，而非byte）
	for i := 1; i <= len(r1); i++ {
		for j := 1; j <= len(r2); j++ {
			if r1[i-1] == r2[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
				if dp[i][j] > maxLen {
					maxLen = dp[i][j]
					endPos1 = i
					endPos2 = j
				}
			}
		}
	}

	if maxLen == 0 {
		return "", 0, 0
	}

	// 基于rune切片截取公共子串，再转回string
	startIdx := endPos1 - maxLen
	commonRune := r1[startIdx:endPos1]
	commonSubstr := string(commonRune)

	return commonSubstr, startIdx, endPos2 - maxLen
}
