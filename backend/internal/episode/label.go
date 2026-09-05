package episode

import (
	"regexp"
	"strings"
)

var (
	seasonEpisodePattern = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])s\s*([0-9]{1,3})\s*e\s*([0-9]{1,4})(?:$|[^0-9])`)
	hashPattern          = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])#\s*([0-9]{1,4})(?:$|[^0-9])`)
	// 第N期 accepts optional spaces and needs both markers so years, durations,
	// and other bare numbers in a title are never read as an episode number.
	chineseEpisodePattern = regexp.MustCompile(`第\s*([0-9]{1,4})\s*期`)
	episodeMarkerPattern  = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(?:episode|ep|e|vol(?:ume)?[ ._-]*)\s*([0-9]{1,4})(?:$|[^0-9])`)
	episodePrefixPattern  = regexp.MustCompile(`^\s*([0-9]{1,4})(?:$|[\s.。、:：)\]】_︳|｜/／\-–—])`)
	storedMarkerPattern   = regexp.MustCompile(`(?i)^(?:#\s*|episode[ ._-]*|ep[ ._-]*|e[ ._-]*|vol(?:ume)?[ ._-]*)[0-9]{1,4}$`)
	seasonEpisodeValue    = regexp.MustCompile(`(?i)^s[0-9]{1,3}e[0-9]{1,4}$`)
)

// FromTitle extracts a high-confidence episode label from a title.
// Season/episode labels retain their season because "S10E24" is more precise
// than reducing it to "24".
func FromTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}

	if matches := seasonEpisodePattern.FindStringSubmatch(title); len(matches) == 3 {
		return "S" + canonicalDigits(matches[1]) + "E" + canonicalDigits(matches[2])
	}
	if matches := chineseEpisodePattern.FindStringSubmatch(title); len(matches) == 2 {
		return canonicalDigits(matches[1])
	}
	if matches := hashPattern.FindStringSubmatch(title); len(matches) == 2 {
		return canonicalDigits(matches[1])
	}
	if matches := episodeMarkerPattern.FindStringSubmatch(title); len(matches) == 2 {
		return canonicalDigits(matches[1])
	}
	if matches := episodePrefixPattern.FindStringSubmatch(title); len(matches) == 2 {
		return canonicalDigits(matches[1])
	}
	return ""
}

// Normalize resolves a display-safe label from the title first, then from a
// previously stored structured label. Bare stored numbers are not trusted
// because feeds may persist list positions or other source identifiers.
func Normalize(title, stored string) string {
	if titleLabel := FromTitle(title); titleLabel != "" {
		return titleLabel
	}

	stored = strings.TrimSpace(stored)
	if stored == "" {
		return ""
	}
	if seasonEpisodeValue.MatchString(stored) || storedMarkerPattern.MatchString(stored) {
		if storedLabel := FromTitle(stored); storedLabel != "" {
			return storedLabel
		}
	}
	return ""
}

func canonicalDigits(value string) string {
	value = strings.TrimLeft(strings.TrimSpace(value), "0")
	if value == "" {
		return "0"
	}
	return value
}
