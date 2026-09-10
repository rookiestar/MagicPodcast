package contentsearch

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var chineseStopwords = map[string]struct{}{
	"的": {}, "了": {}, "吗": {}, "呢": {}, "啊": {}, "吧": {}, "是": {}, "不": {},
	"我": {}, "你": {}, "他": {}, "她": {}, "它": {}, "们": {}, "对": {}, "在": {},
	"和": {}, "与": {}, "或": {}, "也": {}, "就": {}, "都": {}, "很": {}, "还": {},
	"这": {}, "那": {}, "有": {}, "没": {}, "怎么": {}, "什么": {}, "如何": {}, "怎样": {},
	"看": {}, "看待": {}, "认为": {}, "觉得": {}, "一下": {}, "一个": {}, "这个": {}, "那个": {},
}

func tokenize(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}
	seen := map[string]struct{}{}
	tokens := make([]string, 0)
	add := func(token string) {
		token = strings.TrimSpace(token)
		if token == "" {
			return
		}
		if _, stop := chineseStopwords[token]; stop {
			return
		}
		if utf8.RuneCountInString(token) == 1 && unicode.Is(unicode.Han, []rune(token)[0]) {
			return
		}
		if _, ok := seen[token]; ok {
			return
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	var latin strings.Builder
	flushLatin := func() {
		if latin.Len() == 0 {
			return
		}
		add(latin.String())
		latin.Reset()
	}
	runes := []rune(text)
	for i, r := range runes {
		if unicode.Is(unicode.Han, r) {
			flushLatin()
			if i+1 < len(runes) && unicode.Is(unicode.Han, runes[i+1]) {
				add(string(runes[i : i+2]))
			}
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			latin.WriteRune(r)
			continue
		}
		flushLatin()
	}
	flushLatin()
	return tokens
}

func tokenString(tokens []string) string {
	copied := append([]string(nil), tokens...)
	sort.Strings(copied)
	return strings.Join(copied, " ")
}

func HasContentOverlap(query, text string) bool {
	return overlapScore(tokenize(query), tokenize(text)) > 0 ||
		strings.Contains(text, strings.TrimSpace(query))
}

func overlapScore(queryTokens []string, fragmentTokens []string) int {
	if len(queryTokens) == 0 || len(fragmentTokens) == 0 {
		return 0
	}
	set := map[string]struct{}{}
	for _, token := range fragmentTokens {
		set[token] = struct{}{}
	}
	score := 0
	for _, token := range queryTokens {
		if _, ok := set[token]; ok {
			score++
			if utf8.RuneCountInString(token) >= 2 {
				score++
			}
		}
	}
	return score
}
