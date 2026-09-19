package services

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func hasSearchHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func searchASCII(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

// normalizedSearchRunes retains source rune offsets for snippets and highlighting.
func normalizedSearchRunes(s string) ([]rune, []int) {
	source := []rune(s)
	result := make([]rune, 0, len(source))
	positions := make([]int, 0, len(source))
	for i := 0; i < len(source); {
		if unicode.IsSpace(source[i]) {
			end := i + 1
			for end < len(source) && unicode.IsSpace(source[end]) {
				end++
			}
			boundary := i > 0 && end < len(source) && ((unicode.Is(unicode.Han, source[i-1]) && searchASCII(source[end])) || (searchASCII(source[i-1]) && unicode.Is(unicode.Han, source[end])))
			if i == 0 || end == len(source) || boundary {
				i = end
				continue
			}
			for ; i < end; i++ {
				result = append(result, source[i])
				positions = append(positions, i)
			}
			continue
		}
		result = append(result, unicode.ToLower(source[i]))
		positions = append(positions, i)
		i++
	}
	return result, positions
}

func normalizeSearchText(s string) string {
	s = strings.TrimSpace(s)
	var out strings.Builder
	out.Grow(len(s))
	var previous rune
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if unicode.IsSpace(r) {
			end := i + size
			for end < len(s) {
				next, width := utf8.DecodeRuneInString(s[end:])
				if !unicode.IsSpace(next) {
					break
				}
				end += width
			}
			next, _ := utf8.DecodeRuneInString(s[end:])
			if (unicode.Is(unicode.Han, previous) && searchASCII(next)) || (searchASCII(previous) && unicode.Is(unicode.Han, next)) {
				i = end
				continue
			}
			out.WriteString(s[i:end])
			previous = r
			i = end
			continue
		}
		out.WriteRune(unicode.ToLower(r))
		previous = r
		i += size
	}
	return out.String()
}

func searchContains(text, keyword string) bool {
	if hasSearchHan(keyword) {
		return strings.Contains(normalizeSearchText(text), normalizeSearchText(keyword))
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(keyword))
}

// A bounded fractional relevance preserves existing field weights within each tier.
func searchTitlePriority(title, keyword string, relevance float64, rawOtherMatch bool) float64 {
	if relevance <= 0 {
		return 0
	}
	t, q := normalizeSearchText(title), normalizeSearchText(keyword)
	tier := 1.0
	if t == q {
		tier = 7
	} else if strings.HasPrefix(t, q) {
		tier = 5
	} else if strings.Contains(t, q) {
		tier = 3
	}
	rawTitle, rawQuery := strings.ToLower(title), strings.ToLower(strings.TrimSpace(keyword))
	if (tier == 1 && rawOtherMatch) || (tier == 7 && rawTitle == rawQuery) || (tier == 5 && strings.HasPrefix(rawTitle, rawQuery)) || (tier == 3 && strings.Contains(rawTitle, rawQuery)) {
		tier++
	}
	return tier + relevance/(1+relevance)
}
