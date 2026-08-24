package episodecopilot

import (
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	maxNestedURLDepth       = 4
	maxURLDecodePasses      = 8
	maxBufferedAnswerRunes  = 64_000
	noEvidenceAnswerMessage = "当前单集没有可核验的 Show Notes、逐字稿、私有备注或公开来源，无法形成事实性回答。"
)

var (
	modelControlledLinkPattern = regexp.MustCompile(
		`(?i)(?:https?://|file://|mailto:|tel:|data:|javascript:|www\.)[^\s<>"']*`,
	)
	modelControlledSchemeRelativePattern = regexp.MustCompile(
		`(?i)(?:^|[\s=("'[])//[^\s<>"']+`,
	)
	modelControlledEmailPattern = regexp.MustCompile(
		`(?i)\b[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+\b`,
	)
	nestedHTTPURLPattern = regexp.MustCompile(
		`(?i)https?://[^\s<>"']+`,
	)
	nestedSchemeRelativeURLPattern = regexp.MustCompile(
		`(?i)(?:^|[\s=("'[])//[^\s<>"']+`,
	)
	nestedDisallowedSchemePattern = regexp.MustCompile(
		`(?i)(?:^|[^a-z0-9])(?:file|data|javascript):`,
	)
	legacyIPv4HostPattern = regexp.MustCompile(
		`(?i)^(?:0x[0-9a-f]+|[0-9]+)(?:\.(?:0x[0-9a-f]+|[0-9]+)){0,3}$`,
	)
	showNotesCitationPattern = regexp.MustCompile(
		`(?i)\[Show Notes L([1-9][0-9]*)(?:-L?([1-9][0-9]*))?\]`,
	)
	transcriptCitationPattern = regexp.MustCompile(
		`\[逐字稿 L([1-9][0-9]*)(?:-L?([1-9][0-9]*))?\]`,
	)
	externalCitationPattern = regexp.MustCompile(
		`\[E([1-9][0-9]*)\]`,
	)
	cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")
)

var blockedAnswerLinkPrefixes = []string{
	"javascript:",
	"https://",
	"http://",
	"file://",
	"mailto:",
	"data:",
	"tel:",
	"www.",
	"](",
	"//",
}

type answerOutputFilter struct {
	pending string
}

type answerCitationGate struct {
	buffer          strings.Builder
	bufferedRunes   int
	tooLarge        bool
	open            bool
	hasEvidence     bool
	selectedSource  SelectionSource
	showNotesLines  int
	transcriptLines int
	externalSources int
	privateNote     bool
}

func newAnswerCitationGate(
	request QuestionRequest,
	episodeContext EpisodeContext,
	research researchResult,
) *answerCitationGate {
	showNotesLines := documentLineCount(
		episodeContext.ShowNotes,
		request.Selection,
		maxShowNotesRunes,
	)
	transcriptLines := documentLineCount(
		episodeContext.Transcript,
		request.Selection,
		maxTranscriptRunes,
	)
	privateNote := request.IncludePrivateNote &&
		strings.TrimSpace(episodeContext.PrivateNotes) != ""
	return &answerCitationGate{
		hasEvidence: showNotesLines > 0 ||
			transcriptLines > 0 ||
			len(research.Resources) > 0 ||
			privateNote,
		selectedSource:  request.SelectionSource,
		showNotesLines:  showNotesLines,
		transcriptLines: transcriptLines,
		externalSources: len(research.Resources),
		privateNote:     privateNote,
	}
}

func (g *answerCitationGate) Write(value string) string {
	if value == "" || g.tooLarge || !g.hasEvidence {
		return ""
	}
	if g.open {
		return value
	}
	g.bufferedRunes += len([]rune(value))
	if g.bufferedRunes > maxBufferedAnswerRunes {
		g.tooLarge = true
		g.buffer.Reset()
		return ""
	}
	g.buffer.WriteString(value)
	if !g.hasValidCitation(g.buffer.String()) {
		return ""
	}
	g.open = true
	output := g.buffer.String()
	g.buffer.Reset()
	return output
}

func (g *answerCitationGate) Flush() (string, bool) {
	if g.tooLarge {
		return "", false
	}
	if !g.hasEvidence {
		return noEvidenceAnswerMessage, true
	}
	if !g.open {
		return "", false
	}
	return "", true
}

func (g *answerCitationGate) hasValidCitation(value string) bool {
	showNotesValid := citationLineRangeValid(
		showNotesCitationPattern,
		value,
		g.showNotesLines,
	)
	transcriptValid := citationLineRangeValid(
		transcriptCitationPattern,
		value,
		g.transcriptLines,
	)
	switch g.selectedSource {
	case SelectionSourceShowNotes:
		return showNotesValid
	case SelectionSourceTranscript:
		return transcriptValid
	}
	if showNotesValid || transcriptValid {
		return true
	}
	for _, match := range externalCitationPattern.FindAllStringSubmatch(
		value,
		-1,
	) {
		index, err := strconv.Atoi(match[1])
		if err == nil && index <= g.externalSources {
			return true
		}
	}
	return g.privateNote && strings.Contains(value, "[私有备注]")
}

func citationLineRangeValid(
	pattern *regexp.Regexp,
	value string,
	lineCount int,
) bool {
	if lineCount < 1 {
		return false
	}
	for _, match := range pattern.FindAllStringSubmatch(value, -1) {
		start, startErr := strconv.Atoi(match[1])
		end := start
		var endErr error
		if match[2] != "" {
			end, endErr = strconv.Atoi(match[2])
		}
		if startErr == nil &&
			endErr == nil &&
			start <= end &&
			end <= lineCount {
			return true
		}
	}
	return false
}

func documentLineCount(value string, selection string, limit int) int {
	document := numberedDocument(value, selection, limit)
	if document == "无" {
		return 0
	}
	return strings.Count(document, "\n") + 1
}

func (f *answerOutputFilter) Write(value string) string {
	f.pending += strings.Map(func(character rune) rune {
		if character == '\x00' ||
			(unicode.IsControl(character) &&
				character != '\n' &&
				character != '\r' &&
				character != '\t') {
			return -1
		}
		if character == '@' {
			return '＠'
		}
		return character
	}, value)
	return f.drain(false)
}

func (f *answerOutputFilter) Flush() string {
	return f.drain(true)
}

func (f *answerOutputFilter) drain(final bool) string {
	var output strings.Builder
	for f.pending != "" {
		start, prefixLength := earliestBlockedLink(f.pending)
		if start < 0 {
			if final {
				output.WriteString(f.pending)
				f.pending = ""
				break
			}
			holdAt := possibleBlockedPrefixStart(f.pending)
			output.WriteString(f.pending[:holdAt])
			f.pending = f.pending[holdAt:]
			break
		}
		output.WriteString(f.pending[:start])
		end := answerLinkEnd(f.pending, start+prefixLength)
		if end == len(f.pending) && !final {
			f.pending = f.pending[start:]
			break
		}
		output.WriteString("[链接由服务端来源区提供]")
		f.pending = f.pending[end:]
	}
	return output.String()
}

func earliestBlockedLink(value string) (int, int) {
	lower := strings.ToLower(value)
	start := -1
	prefixLength := 0
	for _, prefix := range blockedAnswerLinkPrefixes {
		index := strings.Index(lower, prefix)
		if index >= 0 && (start < 0 || index < start) {
			start = index
			prefixLength = len(prefix)
		}
	}
	return start, prefixLength
}

func possibleBlockedPrefixStart(value string) int {
	lower := strings.ToLower(value)
	boundaries := make([]int, 0, len(value)+1)
	for index := range value {
		boundaries = append(boundaries, index)
	}
	boundaries = append(boundaries, len(value))
	for _, index := range boundaries {
		suffix := lower[index:]
		if suffix == "" {
			continue
		}
		for _, prefix := range blockedAnswerLinkPrefixes {
			if strings.HasPrefix(prefix, suffix) {
				return index
			}
		}
	}
	return len(value)
}

func answerLinkEnd(value string, start int) int {
	for offset, character := range value[start:] {
		if unicode.IsSpace(character) ||
			strings.ContainsRune(`"'<>[]{}()，。；！？,;!?`, character) {
			return start + offset
		}
	}
	return len(value)
}

func stripModelControlledLinks(value string) string {
	value = modelControlledLinkPattern.ReplaceAllString(
		value,
		"[链接省略]",
	)
	value = modelControlledSchemeRelativePattern.ReplaceAllString(
		value,
		" [链接省略]",
	)
	return modelControlledEmailPattern.ReplaceAllString(
		value,
		"[邮箱省略]",
	)
}

func publicHTTPURL(raw string) (string, bool) {
	normalized, err := normalizePublicHTTPURL(raw, 0)
	return normalized, err == nil
}

func normalizePublicHTTPURL(raw string, depth int) (string, error) {
	if depth > maxNestedURLDepth {
		return "", fmt.Errorf("URL nesting exceeds the safety limit")
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return "", fmt.Errorf("URL must be valid")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") ||
		parsed.Host == "" ||
		parsed.User != nil ||
		strings.HasSuffix(parsed.Host, ":") {
		return "", fmt.Errorf("URL must be HTTP(S) without credentials")
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "" ||
		hostname == "localhost" ||
		strings.HasSuffix(hostname, ".localhost") ||
		strings.HasSuffix(hostname, ".local") ||
		strings.HasSuffix(hostname, ".localdomain") ||
		strings.HasSuffix(hostname, ".lan") ||
		strings.HasSuffix(hostname, ".internal") ||
		strings.HasSuffix(hostname, ".home.arpa") ||
		strings.HasSuffix(hostname, ".test") ||
		strings.HasSuffix(hostname, ".invalid") ||
		strings.HasSuffix(hostname, ".onion") {
		return "", fmt.Errorf("URL host must be public")
	}
	address, addressErr := netip.ParseAddr(hostname)
	if addressErr == nil {
		address = address.Unmap()
		if !address.IsGlobalUnicast() ||
			address.IsPrivate() ||
			address.IsLoopback() ||
			address.IsLinkLocalUnicast() ||
			address.IsLinkLocalMulticast() ||
			address.IsUnspecified() ||
			cgnatPrefix.Contains(address) {
			return "", fmt.Errorf("URL address must be public")
		}
	} else {
		if legacyIPv4HostPattern.MatchString(hostname) ||
			!strings.Contains(hostname, ".") {
			return "", fmt.Errorf("URL host must be a public DNS name")
		}
	}
	port := parsed.Port()
	if port != "" {
		number, parseErr := strconv.Atoi(port)
		if parseErr != nil || number < 1 || number > 65535 {
			return "", fmt.Errorf("URL port must be valid")
		}
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", fmt.Errorf("URL query must be valid")
	}
	for key, values := range query {
		if sensitiveURLKey(key) {
			return "", fmt.Errorf("URL query contains credentials")
		}
		for _, value := range values {
			if err := validateNestedPublicURLs(value, depth+1); err != nil {
				return "", err
			}
		}
	}
	if parsed.Fragment != "" {
		if err := validateNestedPublicURLs(parsed.Fragment, depth+1); err != nil {
			return "", err
		}
		fragment, fragmentErr := url.ParseQuery(parsed.Fragment)
		if fragmentErr != nil {
			return "", fmt.Errorf("URL fragment must be valid")
		}
		for key, values := range fragment {
			if sensitiveURLKey(key) {
				return "", fmt.Errorf("URL fragment contains credentials")
			}
			for _, value := range values {
				if err := validateNestedPublicURLs(value, depth+1); err != nil {
					return "", err
				}
			}
		}
	}
	parsed.Scheme = scheme
	normalizedHost := hostname
	if strings.Contains(hostname, ":") {
		normalizedHost = "[" + hostname + "]"
	}
	if port != "" {
		normalizedHost += ":" + port
	}
	parsed.Host = normalizedHost
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	parsed.RawFragment = ""
	normalized := parsed.String()
	normalized = strings.ReplaceAll(normalized, "(", "%28")
	normalized = strings.ReplaceAll(normalized, ")", "%29")
	normalized = strings.ReplaceAll(normalized, "\\", "%5C")
	return normalized, nil
}

func validateNestedPublicURLs(value string, depth int) error {
	decodedValues := []string{value}
	for index := 0; index < maxURLDecodePasses; index++ {
		decoded, err := url.QueryUnescape(
			decodedValues[len(decodedValues)-1],
		)
		if err != nil {
			return fmt.Errorf("nested URL encoding must be valid")
		}
		if decoded == decodedValues[len(decodedValues)-1] {
			break
		}
		decodedValues = append(decodedValues, decoded)
	}
	lastDecoded := decodedValues[len(decodedValues)-1]
	if decoded, err := url.QueryUnescape(lastDecoded); err != nil {
		return fmt.Errorf("nested URL encoding must be valid")
	} else if decoded != lastDecoded {
		return fmt.Errorf("nested URL encoding exceeds the safety limit")
	}
	for _, decoded := range decodedValues {
		if nestedDisallowedSchemePattern.MatchString(decoded) {
			return fmt.Errorf("nested URL uses a disallowed scheme")
		}
		for _, raw := range nestedHTTPURLPattern.FindAllString(decoded, -1) {
			candidate := strings.TrimRight(raw, ".,;:!?)]}")
			if _, err := normalizePublicHTTPURL(candidate, depth); err != nil {
				return fmt.Errorf("nested URL is unsafe: %w", err)
			}
		}
		for _, raw := range nestedSchemeRelativeURLPattern.FindAllString(
			decoded,
			-1,
		) {
			candidate := strings.TrimLeft(raw, " \t\r\n=(\"'[")
			candidate = strings.TrimRight(candidate, ".,;:!?)]}")
			if _, err := normalizePublicHTTPURL(
				"https:"+candidate,
				depth,
			); err != nil {
				return fmt.Errorf("nested URL is unsafe: %w", err)
			}
		}
	}
	return nil
}

func sensitiveURLKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if strings.HasPrefix(normalized, "x-amz-") ||
		strings.HasPrefix(normalized, "x-goog-") {
		return true
	}
	compact := strings.Map(func(character rune) rune {
		if character >= 'a' && character <= 'z' {
			return character
		}
		return -1
	}, normalized)
	for _, marker := range []string{
		"accesstoken",
		"apikey",
		"accesskey",
		"signature",
		"credential",
		"password",
		"sessionid",
		"authkey",
	} {
		if strings.Contains(compact, marker) {
			return true
		}
	}
	parts := strings.FieldsFunc(normalized, func(character rune) bool {
		return character < 'a' || character > 'z'
	})
	for _, part := range parts {
		switch part {
		case "token", "key", "secret", "signature", "sig", "auth",
			"authorization", "credential", "password", "passwd", "cookie",
			"session", "jwt":
			return true
		}
	}
	return false
}
