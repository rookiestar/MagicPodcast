package services

import "strings"

const (
	searchSnippetLength = 150
	searchSnippetPrefix = 35
)

// isPureNumber 检查字符串是否为纯数字
func isPureNumber(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isStandaloneNumber 检查数字是否独立出现（被空格、标点包围，或在开头/结尾）
func isStandaloneNumber(text, number string) bool {
	lowerText := strings.ToLower(text)
	lowerNumber := strings.ToLower(number)

	// 查找所有出现位置
	idx := 0
	for {
		pos := strings.Index(lowerText[idx:], lowerNumber)
		if pos == -1 {
			break
		}
		actualPos := idx + pos

		// 检查这个位置的数字是否是独立的
		before := ' '
		after := ' '

		if actualPos > 0 {
			before = rune(lowerText[actualPos-1])
		}
		if actualPos+len(number) < len(lowerText) {
			after = rune(lowerText[actualPos+len(number)])
		}

		// 如果前后都是非字母数字字符，则认为是独立的
		beforeIsAlphaNum := (before >= 'a' && before <= 'z') || (before >= '0' && before <= '9')
		afterIsAlphaNum := (after >= 'a' && after <= 'z') || (after >= '0' && after <= '9')

		if !beforeIsAlphaNum && !afterIsAlphaNum {
			return true
		}

		idx = actualPos + len(number)
	}

	return false
}

// stripHTML 移除 HTML 标签
func stripHTML(text string) string {
	// 简单的 HTML 标签移除
	var result strings.Builder
	inTag := false

	for _, r := range text {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// generateSnippet 生成匹配片段（用于高亮显示）
func generateSnippet(text, keyword string) string {
	// 清理文本：先移除 HTML 标签，再清理换行符
	text = stripHTML(text)
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.TrimSpace(text)

	textRunes := []rune(text)
	if len(textRunes) <= searchSnippetLength {
		return text
	}

	keywordLowerRunes := []rune(strings.ToLower(strings.TrimSpace(keyword)))
	if len(keywordLowerRunes) == 0 {
		return string(textRunes[:searchSnippetLength]) + "..."
	}
	textLowerRunes := []rune(strings.ToLower(text))

	// 查找关键词第一次出现的位置
	idx := indexRunes(textLowerRunes, keywordLowerRunes)
	if idx == -1 {
		return string(textRunes[:searchSnippetLength]) + "..."
	}

	// 生成以关键词为中心的片段
	// 策略：让关键词出现在片段的前 1/4 处，这样用户能更快看到关键词
	start := idx - searchSnippetPrefix
	if start < 0 {
		start = 0
	}

	end := start + searchSnippetLength
	if end > len(textRunes) {
		end = len(textRunes)
		// 如果接近文本末尾，调整 start 以保持 snippet 长度
		start = end - searchSnippetLength
		if start < 0 {
			start = 0
		}
	}

	// 最终验证：确保 snippet 包含完整的关键词
	snippet := string(textRunes[start:end])
	if !containsRunes(textLowerRunes[start:end], keywordLowerRunes) {
		// 如果因为某种原因 snippet 不包含关键词，使用最简单的策略
		start = idx - 20
		if start < 0 {
			start = 0
		}
		end = idx + len(keywordLowerRunes) + 100
		if end > len(textRunes) {
			end = len(textRunes)
		}
		snippet = string(textRunes[start:end])
	}

	// 添加省略号
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(textRunes) {
		snippet = snippet + "..."
	}

	return snippet
}

func indexRunes(text, keyword []rune) int {
	if len(keyword) == 0 || len(keyword) > len(text) {
		return -1
	}

	for i := 0; i <= len(text)-len(keyword); i++ {
		if sameRunes(text[i:i+len(keyword)], keyword) {
			return i
		}
	}

	return -1
}

func containsRunes(text, keyword []rune) bool {
	return indexRunes(text, keyword) >= 0
}

func sameRunes(left, right []rune) bool {
	if len(left) != len(right) {
		return false
	}

	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}

	return true
}
