package personidentity

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type extractedCandidate struct {
	key             string
	SourceNames     []string
	DisplayName     string
	Aliases         []string
	IdentityNote    string
	Role            string
	Status          string
	StatusReason    string
	EvidenceKind    string
	EvidenceLocator string
	MentionedOnly   bool
}

type extractedFragment struct {
	Segment
	PersonName      string
	PersonKey       string
	Status          string
	EvidenceKind    string
	EvidenceLocator string
}

var (
	hostPattern           = regexp.MustCompile(`主播\s*[：:]?\s*([^\s，。,、（(：:]{1,20})`)
	guestPattern          = regexp.MustCompile(`嘉宾\s*[：:]?\s*([^\s，。,、（(：:]{1,20})`)
	reporterPattern       = regexp.MustCompile(`记者\s*[：:]?\s*([^\s，。,、（(：:]{1,20})`)
	alsoCalledPattern     = regexp.MustCompile(`(?:也叫[他她]|叫作|常被叫作|听众也叫[他她])\s*([^\s，。,、）)]{1,12})`)
	parenPattern          = regexp.MustCompile(`([^\s，。,、]{1,20})[（(]([^）)]{1,80})[）)]`)
	notPresentPattern     = regexp.MustCompile(`([^\s，。,、]{1,12})(?:并未到场|未到场|没有到场|未出席)`)
	selfIntroPattern      = regexp.MustCompile(`我是([^\s，。,、]{1,20})`)
	callMePattern         = regexp.MustCompile(`(?:大家叫我|叫我)([^\s，。,、也]{1,12})`)
	anonymousGuestPattern = regexp.MustCompile(`匿名[^，。]{0,12}`)
	speakerNumberPattern  = regexp.MustCompile(`(?i)^speaker\s*\d+$`)
)

func extractFromSources(sources EpisodeSources) ([]extractedCandidate, []extractedFragment) {
	candidates := parseShowNoteCandidates(sources.ShowNotes)
	selfIntros := parseSelfIntros(sources.Segments)
	candidates = mergeSelfIntroCandidates(candidates, selfIntros)
	fragments := bindFragments(sources.Segments, candidates)
	return candidates, fragments
}

func parseShowNoteCandidates(notes string) []extractedCandidate {
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return nil
	}
	mentionedOnly := map[string]string{}
	for _, match := range notPresentPattern.FindAllStringSubmatch(notes, -1) {
		name := cleanName(match[1])
		if name != "" {
			mentionedOnly[name] = match[0]
		}
	}
	seen := map[string]extractedCandidate{}
	orderNames := make([]string, 0)
	add := func(name, role, evidence, locator, identity string, aliases []string, pending bool) {
		name = cleanName(name)
		if name == "" || mentionedOnly[name] != "" {
			return
		}
		if current, ok := seen[name]; ok {
			if current.Role == RoleUnknown && role != RoleUnknown {
				current.Role = role
			}
			if current.IdentityNote == "" && identity != "" {
				current.IdentityNote = identity
			}
			current.Aliases = uniqueStrings(append(current.Aliases, aliases...))
			seen[name] = current
			return
		}
		status := StatusConfirmed
		reason := ""
		if pending || strings.Contains(name, "匿名") {
			status = StatusPending
			reason = "identity is not uniquely confirmed"
		}
		seen[name] = extractedCandidate{
			DisplayName:     name,
			Aliases:         uniqueStrings(aliases),
			IdentityNote:    strings.TrimSpace(identity),
			Role:            role,
			Status:          status,
			StatusReason:    reason,
			EvidenceKind:    evidence,
			EvidenceLocator: locator,
			MentionedOnly:   false,
		}
		orderNames = append(orderNames, name)
	}

	for _, match := range hostPattern.FindAllStringSubmatchIndex(notes, -1) {
		name := clipPersonName(notes[match[2]:match[3]])
		identity, aliases := identityFromFollowing(notes, match[3], name)
		add(name, RoleHost, "show_notes", "host_line", identity, aliases, false)
	}
	for _, match := range guestPattern.FindAllStringSubmatchIndex(notes, -1) {
		name := notes[match[2]:match[3]]
		if strings.Contains(name, "匿名") || strings.Contains(notes[match[0]:min(len(notes), match[3]+40)], "匿名") {
			add("匿名工程师", RoleGuest, "show_notes", "anonymous_guest", "不愿具名的从业者", nil, true)
			continue
		}
		name = clipPersonName(name)
		identity, aliases := identityFromFollowing(notes, match[3], name)
		add(name, RoleGuest, "show_notes", "guest_line", identity, aliases, false)
	}
	for _, match := range reporterPattern.FindAllStringSubmatchIndex(notes, -1) {
		name := clipPersonName(notes[match[2]:match[3]])
		identity, aliases := identityFromFollowing(notes, match[3], name)
		if identity == "" {
			identity = "记者"
		}
		add(name, RoleGuest, "show_notes", "reporter_line", identity, aliases, false)
	}
	if anonymousGuestPattern.MatchString(notes) {
		if _, exists := seen["匿名工程师"]; !exists {
			add("匿名工程师", RoleGuest, "show_notes", "anonymous_guest", "不愿具名的从业者", nil, true)
		}
	}
	order := make([]extractedCandidate, 0, len(orderNames)+len(mentionedOnly))
	for _, name := range orderNames {
		order = append(order, seen[name])
	}
	for name, locator := range mentionedOnly {
		if _, ok := seen[name]; ok {
			continue
		}
		order = append(order, extractedCandidate{
			DisplayName:     name,
			Role:            RoleUnknown,
			Status:          StatusPending,
			StatusReason:    "mentioned only; not an episode participant",
			EvidenceKind:    "show_notes_mention",
			EvidenceLocator: locator,
			MentionedOnly:   true,
		})
	}
	return order
}

func identityFromFollowing(notes string, nameEnd int, name string) (string, []string) {
	window := notes[nameEnd:]
	if cut := strings.IndexAny(window, "。；;\n"); cut >= 0 {
		window = window[:cut]
	}
	for _, separator := range []string{"和嘉宾", "和主播", "邀请", "以及"} {
		if cut := strings.Index(window, separator); cut >= 0 {
			window = window[:cut]
		}
	}
	if utf8.RuneCountInString(window) > 40 {
		window = string([]rune(window)[:40])
	}
	identity := ""
	aliases := []string{}
	trimmed := strings.TrimSpace(window)
	if match := regexp.MustCompile(`^[（(]([^）)]+)[）)]`).FindStringSubmatch(trimmed); len(match) == 2 {
		identity = strings.TrimSpace(match[1])
	}
	if identity != "" {
		for _, match := range alsoCalledPattern.FindAllStringSubmatch(identity, -1) {
			aliases = append(aliases, cleanName(match[1]))
		}
		if looksLikeNickname(identity) && !strings.Contains(identity, "，") && !strings.Contains(identity, "、") {
			aliases = append(aliases, cleanName(identity))
			identity = ""
		}
	}
	return identity, uniqueStrings(aliases)
}

var personNameStops = []string{
	"分享", "讲解", "讨论", "回顾", "邀请", "介绍", "带来", "一起", "今天", "本期",
}

func clipPersonName(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "匿名") {
		return raw
	}
	var collected []rune
	for _, r := range raw {
		if unicode.Is(unicode.Han, r) {
			collected = append(collected, r)
			if len(collected) >= 4 {
				break
			}
			continue
		}
		if len(collected) > 0 {
			break
		}
	}
	if len(collected) < 2 {
		return cleanName(raw)
	}
	for n := 2; n <= len(collected); n++ {
		rest := string(collected[n:])
		if rest == "" {
			return string(collected[:n])
		}
		for _, stop := range personNameStops {
			if strings.HasPrefix(rest, stop) || strings.HasPrefix(string(collected[n:]), stop) {
				return string(collected[:n])
			}
		}
	}

	return string(collected)
}

func looksLikeNickname(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > 4 {
		return false
	}
	for _, r := range trimmed {
		if unicode.IsDigit(r) {
			return false
		}
	}
	return !strings.Contains(trimmed, "记者") && !strings.Contains(trimmed, "主播") && !strings.Contains(trimmed, "架构")
}

func parseSelfIntros(segments []Segment) []extractedCandidate {
	out := make([]extractedCandidate, 0)
	for _, segment := range segments {
		name := ""
		if match := selfIntroPattern.FindStringSubmatch(segment.Text); len(match) == 2 {
			name = cleanName(match[1])
		}
		aliases := []string{}
		if match := callMePattern.FindStringSubmatch(segment.Text); len(match) == 2 {
			aliases = append(aliases, cleanName(match[1]))
		}
		if name == "" && len(aliases) == 0 {
			continue
		}
		if name == "" {
			name = cleanName(segment.SpeakerLabel)
		}
		out = append(out, extractedCandidate{
			DisplayName:     name,
			Aliases:         uniqueStrings(aliases),
			Role:            RoleUnknown,
			Status:          StatusConfirmed,
			EvidenceKind:    "self_intro",
			EvidenceLocator: fragmentLocator(segment),
		})
	}
	return out
}

func mergeSelfIntroCandidates(candidates []extractedCandidate, intros []extractedCandidate) []extractedCandidate {
	for _, intro := range intros {
		matched := false
		for i := range candidates {
			if candidates[i].MentionedOnly {
				continue
			}
			if namesEquivalent(candidates[i], intro.DisplayName) || namesEquivalent(intro, candidates[i].DisplayName) {
				candidates[i].Aliases = uniqueStrings(append(candidates[i].Aliases, intro.Aliases...))
				if candidates[i].EvidenceKind == "show_notes" {
					candidates[i].EvidenceKind = "show_notes+self_intro"
				}
				if candidates[i].Status != StatusPending {
					candidates[i].Status = StatusConfirmed
				}
				matched = true
				break
			}
		}
		if !matched && intro.DisplayName != "" {
			candidates = append(candidates, intro)
		}
	}
	return candidates
}

func bindFragments(segments []Segment, candidates []extractedCandidate) []extractedFragment {
	labelCounts := map[string]int{}
	for _, segment := range segments {
		labelCounts[strings.TrimSpace(segment.SpeakerLabel)]++
	}
	participants := participantCandidates(candidates)
	out := make([]extractedFragment, 0, len(segments))
	for _, segment := range segments {
		item := extractedFragment{
			Segment:         segment,
			Status:          StatusPending,
			EvidenceKind:    "unmatched_label",
			EvidenceLocator: fragmentLocator(segment),
		}
		label := strings.TrimSpace(segment.SpeakerLabel)
		if isAnonymousLabel(label) {
			item.EvidenceKind = "anonymous_speaker_label"
			out = append(out, item)
			continue
		}
		matches := matchingCandidates(participants, label)
		if intro := selfIntroPattern.FindStringSubmatch(segment.Text); len(intro) == 2 {
			introName := cleanName(intro[1])
			introMatches := matchingCandidates(participants, introName)
			if len(introMatches) == 1 {
				matches = introMatches
			}
		}
		if isGenericLabel(label) && labelCounts[label] > 0 {
			distinctPeople := map[string]struct{}{}
			for _, candidate := range participants {
				if candidate.Status == StatusConfirmed {
					distinctPeople[candidate.DisplayName] = struct{}{}
				}
			}
			if len(distinctPeople) > 1 && (label == "嘉宾" || isGenericLabel(label)) {
				item.EvidenceKind = "mixed_speaker_label"
				out = append(out, item)
				continue
			}
		}
		if len(matches) == 1 && matches[0].Status == StatusConfirmed && !isGenericLabel(label) {
			item.PersonName = matches[0].DisplayName
			item.Status = StatusConfirmed
			item.EvidenceKind = "speaker_label"
			if strings.Contains(segment.Text, "我是"+matches[0].DisplayName) {
				item.EvidenceKind = "self_intro"
			}
		}
		out = append(out, item)
	}
	return out
}

func participantCandidates(candidates []extractedCandidate) []extractedCandidate {
	out := make([]extractedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.MentionedOnly {
			out = append(out, candidate)
		}
	}
	return out
}

func matchingCandidates(candidates []extractedCandidate, name string) []extractedCandidate {
	name = cleanName(name)
	if name == "" {
		return nil
	}
	out := make([]extractedCandidate, 0)
	for _, candidate := range candidates {
		if namesEquivalent(candidate, name) {
			out = append(out, candidate)
		}
	}
	return out
}

func namesEquivalent(candidate extractedCandidate, name string) bool {
	name = cleanName(name)
	if name == "" {
		return false
	}
	if candidate.DisplayName == name {
		return true
	}
	for _, alias := range candidate.Aliases {
		if alias == name {
			return true
		}
	}
	return false
}

func isAnonymousLabel(label string) bool {
	normalized := strings.ToLower(strings.TrimSpace(label))
	return speakerNumberPattern.MatchString(normalized) || strings.Contains(label, "匿名")
}

func isGenericLabel(label string) bool {
	switch strings.TrimSpace(label) {
	case "嘉宾", "主播", "主持", "guest", "host", "speaker":
		return true
	default:
		return isAnonymousLabel(label)
	}
}

func cleanName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "（()）“”\"'：:，,。、")
	value = strings.TrimPrefix(value, "一位")
	if strings.HasPrefix(value, "是") {
		value = strings.TrimPrefix(value, "是")
	}
	if utf8.RuneCountInString(value) > 20 {
		value = string([]rune(value)[:20])
	}
	return strings.TrimSpace(value)
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanName(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsDisplayName(candidates []extractedCandidate, name string) bool {
	for _, candidate := range candidates {
		if candidate.DisplayName == name {
			return true
		}
	}
	return false
}

func fragmentLocator(segment Segment) string {
	return "transcript:" + strings.TrimSpace(segment.SpeakerLabel) + ":" + strconv.Itoa(segment.Order)
}

func identityTokens(note string) map[string]struct{} {
	tokens := map[string]struct{}{}
	normalized := strings.ToLower(strings.TrimSpace(note))
	var current strings.Builder
	flush := func() {
		token := strings.TrimSpace(current.String())
		current.Reset()
		if utf8.RuneCountInString(token) < 2 {
			return
		}
		tokens[token] = struct{}{}
	}
	runes := []rune(normalized)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if unicode.Is(unicode.Han, r) {
			flush()
			if i+1 < len(runes) && unicode.Is(unicode.Han, runes[i+1]) {
				tokens[string(runes[i:i+2])] = struct{}{}
			}
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}
