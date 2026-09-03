package processing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	minutesEnrichmentSchemaVersion = "1.0.0"
	minutesEnrichmentFileName      = "minutes-enrichment.json"
	minutesWhiteboardMediaID       = "whiteboard"
	maxWhiteboardPreviewBytes      = 20 << 20
	maxWhiteboardPreviewDimension  = 8192
	maxWhiteboardPreviewPixels     = 32 << 20
	minutesWhiteboardAlt           = "飞书智能纪要画板"
)

var (
	headingSplitPattern       = regexp.MustCompile(`(?is)<h([1-6])\b[^>]*>(.*?)</h[1-6]>`)
	whiteboardTokenPattern    = regexp.MustCompile(`(?is)<whiteboard\b[^>]*\b(?:token|whiteboard-token|whiteboard_token)="([^"]+)"`)
	anchorPattern             = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)
	bookmarkPattern           = regexp.MustCompile(`(?is)<bookmark\b([^>]*)>`)
	blockquotePattern         = regexp.MustCompile(`(?is)<blockquote\b[^>]*>(.*?)</blockquote>`)
	listItemPattern           = regexp.MustCompile(`(?is)<li\b[^>]*>(.*?)</li>`)
	paragraphPattern          = regexp.MustCompile(`(?is)<p\b[^>]*>(.*?)</p>`)
	hrefPattern               = regexp.MustCompile(`(?i)(?:^|[\t\n\f\r ])href\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'<>\x60]+))`)
	namePattern               = regexp.MustCompile(`(?i)(?:^|[\t\n\f\r ])(?:name|title)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'<>\x60]+))`)
	tagPattern                = regexp.MustCompile(`(?s)<[^>]+>`)
	mediaIDPattern            = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
	feishuIdentityPattern     = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:(?:obcn|wbcn|boxcn|doxcn)[a-z0-9_-]{4,}|docx_[a-z0-9_-]{4,})`)
	feishuURLHostPattern      = regexp.MustCompile(`(?i)(?:https?:)?//(?:[^/?#@\s]+@)?(?:[a-z0-9-]+\.)*(?:feishu\.cn|feishu\.com|larksuite\.com|larksuite\.cn|larkoffice\.com|larkoffice\.cn)(?::[0-9]+)?(?:[/?#\s]|$)`)
	nestedUnsafeSchemePattern = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:javascript|data|file):`)
)

var feishuLinkHosts = []string{
	"feishu.cn",
	"feishu.com",
	"larksuite.com",
	"larksuite.cn",
	"larkoffice.com",
	"larkoffice.cn",
}

var minutesLinkSecretKeys = []string{
	"minute_token",
	"note_id",
	"file_token",
	"whiteboard_token",
	"doc_token",
	"token",
}

var placeholderTexts = map[string]struct{}{
	"":       {},
	"-":      {},
	"—":      {},
	"n/a":    {},
	"na":     {},
	"无":      {},
	"暂无":     {},
	"无内容":    {},
	"暂无内容":   {},
	"没有内容":   {},
	"无关键决策":  {},
	"暂无关键决策": {},
	"没有关键决策": {},
	"无金句":    {},
	"暂无金句":   {},
	"没有金句":   {},
}

type MinutesChapter struct {
	Order   int    `json:"order"`
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms,omitempty"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
}

type MinutesQuote struct {
	Quote       string `json:"quote"`
	Explanation string `json:"explanation,omitempty"`
}

type MinutesLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type MinutesWhiteboard struct {
	MediaID   string `json:"media_id"`
	MediaType string `json:"media_type"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	SHA256    string `json:"sha256"`
	Alt       string `json:"alt"`
}

type MinutesEnrichment struct {
	SchemaVersion string             `json:"schema_version"`
	Chapters      []MinutesChapter   `json:"chapters,omitempty"`
	Keywords      []string           `json:"keywords,omitempty"`
	Decisions     []string           `json:"decisions,omitempty"`
	Quotes        []MinutesQuote     `json:"quotes,omitempty"`
	Links         []MinutesLink      `json:"links,omitempty"`
	Whiteboard    *MinutesWhiteboard `json:"whiteboard,omitempty"`
}

type ManagedImage struct {
	MediaType string
	Width     int
	Height    int
	Bytes     []byte
}

func (e MinutesEnrichment) Empty() bool {
	return len(e.Chapters) == 0 &&
		len(e.Keywords) == 0 &&
		len(e.Decisions) == 0 &&
		len(e.Quotes) == 0 &&
		len(e.Links) == 0 &&
		e.Whiteboard == nil
}

func (e MinutesEnrichment) Public() MinutesEnrichment {
	public := MinutesEnrichment{
		SchemaVersion: minutesEnrichmentSchemaVersion,
		Chapters:      append([]MinutesChapter(nil), e.Chapters...),
		Keywords:      append([]string(nil), e.Keywords...),
		Decisions:     append([]string(nil), e.Decisions...),
		Quotes:        append([]MinutesQuote(nil), e.Quotes...),
		Links:         append([]MinutesLink(nil), e.Links...),
	}
	if e.Whiteboard != nil {
		copied := *e.Whiteboard
		public.Whiteboard = &copied
	}
	return public.normalized()
}

func (e MinutesEnrichment) normalized() MinutesEnrichment {
	e.SchemaVersion = minutesEnrichmentSchemaVersion
	e.Chapters = normalizeMinutesChapters(e.Chapters)
	e.Keywords = uniqueNonEmptyStrings(e.Keywords)
	e.Decisions = filterMeaningfulTexts(e.Decisions)
	e.Quotes = normalizeMinutesQuotes(e.Quotes)
	e.Links = normalizeMinutesLinks(e.Links)
	if e.Whiteboard != nil && !validMinutesWhiteboard(*e.Whiteboard) {
		e.Whiteboard = nil
	}
	return e
}

func encodeMinutesEnrichment(enrichment MinutesEnrichment) ([]byte, error) {
	normalized := enrichment.Public()
	if normalized.Empty() {
		return nil, nil
	}
	encoded, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode minutes enrichment: %w", err)
	}
	return append(encoded, '\n'), nil
}

func decodeMinutesEnrichment(raw []byte) (MinutesEnrichment, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return MinutesEnrichment{}, fmt.Errorf("minutes enrichment is empty")
	}
	var decoded MinutesEnrichment
	if err := strictJSONDecode(raw, &decoded); err != nil {
		return MinutesEnrichment{}, err
	}
	if decoded.SchemaVersion != "" && decoded.SchemaVersion != minutesEnrichmentSchemaVersion {
		return MinutesEnrichment{}, fmt.Errorf("unsupported minutes enrichment schema")
	}
	return decoded.normalized(), nil
}

func parseMinutesChapters(raw json.RawMessage) []MinutesChapter {
	chapters, _ := parseMinutesChaptersStrict(raw)
	return chapters
}

func parseMinutesChaptersStrict(raw json.RawMessage) ([]MinutesChapter, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("chapters must be an array: %w", err)
	}
	chapters := make([]MinutesChapter, 0, len(items))
	for index, item := range items {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(item, &object); err != nil || object == nil {
			return nil, fmt.Errorf("chapter %d must be an object", index+1)
		}
		title, titlePresent, err := firstJSONStringStrict(object, "title", "topic", "name", "heading")
		if err != nil {
			return nil, fmt.Errorf("chapter %d title is invalid: %w", index+1, err)
		}
		summary, summaryPresent, err := firstJSONStringStrict(object, "summary", "abstract", "description", "content")
		if err != nil {
			return nil, fmt.Errorf("chapter %d summary is invalid: %w", index+1, err)
		}
		startMS, startPresent, startOK := firstJSONMillisecondsStrict(
			object,
			"start_ms", "start_time", "startTime", "start",
		)
		if startPresent && (!startOK || startMS < 0) {
			return nil, fmt.Errorf("chapter %d start time is invalid", index+1)
		}
		if strings.TrimSpace(title) == "" && strings.TrimSpace(summary) == "" && !startOK {
			if titlePresent || summaryPresent {
				continue
			}
			return nil, fmt.Errorf("chapter %d has no recognized content", index+1)
		}
		chapter := MinutesChapter{
			StartMS: startMS,
			Title:   strings.TrimSpace(title),
			Summary: strings.TrimSpace(summary),
		}
		if endMS, endPresent, endOK := firstJSONMillisecondsStrict(
			object,
			"end_ms", "end_time", "endTime", "end",
		); endPresent {
			if !endOK || endMS < chapter.StartMS {
				return nil, fmt.Errorf("chapter %d end time is invalid", index+1)
			}
			chapter.EndMS = endMS
		}
		chapters = append(chapters, chapter)
	}
	return normalizeMinutesChapters(chapters), nil
}

func parseMinutesKeywords(raw json.RawMessage) []string {
	keywords, _ := parseMinutesKeywordsStrict(raw)
	return keywords
}

func parseMinutesKeywordsStrict(raw json.RawMessage) ([]string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("keywords must be an array: %w", err)
	}
	keywords := make([]string, 0, len(items))
	for index, item := range items {
		var asString string
		if err := json.Unmarshal(item, &asString); err == nil {
			keywords = append(keywords, asString)
			continue
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(item, &object); err != nil || object == nil {
			return nil, fmt.Errorf("keyword %d must be text or an object", index+1)
		}
		value, present, err := firstJSONStringStrict(
			object,
			"keyword", "name", "text", "title", "value",
		)
		if err != nil || !present {
			return nil, fmt.Errorf("keyword %d has invalid content", index+1)
		}
		keywords = append(keywords, value)
	}
	return uniqueNonEmptyStrings(keywords), nil
}

func parseNoteSections(document string) (decisions []string, quotes []MinutesQuote, links []MinutesLink, whiteboardToken string) {
	sections := splitNoteSections(document)
	if token := firstWhiteboardToken(sections["总结"]); token != "" {
		whiteboardToken = token
	} else {
		whiteboardToken = firstWhiteboardToken(document)
	}
	decisions = extractNoteDecisions(sections["关键决策"])
	quotes = extractNoteQuotes(sections["金句时刻"])
	if len(quotes) == 0 {
		quotes = extractNoteQuotes(sections["金句"])
	}
	links = extractNoteLinks(sections["相关链接"])
	if len(links) == 0 {
		links = extractNoteLinks(sections["相关外链"])
	}
	return decisions, quotes, links, whiteboardToken
}

func splitNoteSections(document string) map[string]string {
	matches := headingSplitPattern.FindAllStringSubmatchIndex(document, -1)
	sections := make(map[string]string)
	for index, loc := range matches {
		if len(loc) < 6 {
			continue
		}
		title := normalizeNoteHeading(stripXMLTags(document[loc[4]:loc[5]]))
		start := loc[1]
		end := len(document)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		if title == "" || start >= end {
			continue
		}
		if _, exists := sections[title]; !exists {
			sections[title] = document[start:end]
		}
	}
	return sections
}

func hasKnownTopLevelNoteSection(document string) bool {
	for _, loc := range headingSplitPattern.FindAllStringSubmatchIndex(document, -1) {
		if len(loc) < 6 || document[loc[2]:loc[3]] != "1" {
			continue
		}
		if knownNoteSection(normalizeNoteHeading(stripXMLTags(document[loc[4]:loc[5]]))) {
			return true
		}
	}
	return false
}

func hasUnknownTopLevelNoteSection(document string) bool {
	for _, loc := range headingSplitPattern.FindAllStringSubmatchIndex(document, -1) {
		if len(loc) < 6 || document[loc[2]:loc[3]] != "1" {
			continue
		}
		if !knownNoteSection(normalizeNoteHeading(stripXMLTags(document[loc[4]:loc[5]]))) {
			return true
		}
	}
	return false
}

func knownNoteSection(title string) bool {
	switch title {
	case "总结", "智能章节", "关键决策", "金句时刻", "金句", "相关链接", "相关外链":
		return true
	default:
		return false
	}
}

func extractNoteDecisions(section string) []string {
	if strings.TrimSpace(section) == "" {
		return nil
	}
	items := collectXMLListItems(section)
	if len(items) == 0 {
		items = collectXMLParagraphs(section)
	}
	return filterMeaningfulTexts(items)
}

func extractNoteQuotes(section string) []MinutesQuote {
	if strings.TrimSpace(section) == "" {
		return nil
	}
	quotes := make([]MinutesQuote, 0)
	matches := blockquotePattern.FindAllStringSubmatchIndex(section, -1)
	for _, loc := range matches {
		quote := strings.TrimSpace(stripXMLTags(section[loc[2]:loc[3]]))
		if isPlaceholderText(quote) {
			continue
		}
		explanation := ""
		rest := section[loc[1]:]
		nextQuote := blockquotePattern.FindStringIndex(rest)
		nextHeading := headingSplitPattern.FindStringIndex(rest)
		limit := len(rest)
		if nextQuote != nil && nextQuote[0] < limit {
			limit = nextQuote[0]
		}
		if nextHeading != nil && nextHeading[0] < limit {
			limit = nextHeading[0]
		}
		if paragraphs := collectXMLParagraphs(rest[:limit]); len(paragraphs) > 0 {
			explanation = paragraphs[0]
		}
		quotes = append(quotes, MinutesQuote{Quote: quote, Explanation: explanation})
	}
	if len(quotes) > 0 {
		return normalizeMinutesQuotes(quotes)
	}
	if paragraphQuotes := extractParagraphQuotes(section); len(paragraphQuotes) > 0 {
		return paragraphQuotes
	}
	for _, item := range collectXMLListItems(section) {
		quote, explanation := splitQuoteAndExplanation(item)
		if isPlaceholderText(quote) {
			continue
		}
		quotes = append(quotes, MinutesQuote{Quote: quote, Explanation: explanation})
	}
	return normalizeMinutesQuotes(quotes)
}

func extractParagraphQuotes(section string) []MinutesQuote {
	paragraphs := collectXMLParagraphs(section)
	quotes := make([]MinutesQuote, 0, len(paragraphs)/2)
	for index := 0; index < len(paragraphs); index++ {
		quote := strings.TrimSpace(paragraphs[index])
		if !looksLikeQuotedText(quote) || isPlaceholderText(quote) {
			continue
		}
		explanation := ""
		if index+1 < len(paragraphs) {
			if parsed, ok := quoteExplanation(paragraphs[index+1]); ok {
				explanation = parsed
				index++
			}
		}
		quotes = append(quotes, MinutesQuote{Quote: quote, Explanation: explanation})
	}
	return normalizeMinutesQuotes(quotes)
}

func looksLikeQuotedText(value string) bool {
	value = strings.TrimSpace(value)
	for _, pair := range [][2]string{{"「", "」"}, {"『", "』"}, {"“", "”"}} {
		if strings.HasPrefix(value, pair[0]) && strings.HasSuffix(value, pair[1]) {
			return true
		}
	}
	return false
}

func quoteExplanation(value string) (string, bool) {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"——", "—", "--"} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(value, prefix)), true
		}
	}
	return "", false
}

func extractNoteLinks(section string) []MinutesLink {
	if strings.TrimSpace(section) == "" {
		return nil
	}
	links := make([]MinutesLink, 0)
	for _, match := range anchorPattern.FindAllStringSubmatch(section, -1) {
		href := firstAttribute(match[1], hrefPattern)
		title := strings.TrimSpace(stripXMLTags(match[2]))
		if title == "" {
			title = firstAttribute(match[1], namePattern)
		}
		if link, ok := publicMinutesLink(title, href); ok {
			links = append(links, link)
		}
	}
	for _, match := range bookmarkPattern.FindAllStringSubmatch(section, -1) {
		href := firstAttribute(match[1], hrefPattern)
		title := firstAttribute(match[1], namePattern)
		if link, ok := publicMinutesLink(title, href); ok {
			links = append(links, link)
		}
	}
	return normalizeMinutesLinks(links)
}

func firstWhiteboardToken(content string) string {
	match := whiteboardTokenPattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	token := strings.TrimSpace(match[1])
	if !larkTokenPattern.MatchString(token) {
		return ""
	}
	return token
}

func sniffManagedImage(content []byte) (ManagedImage, error) {
	if len(content) == 0 || int64(len(content)) > maxWhiteboardPreviewBytes {
		return ManagedImage{}, fmt.Errorf("whiteboard preview size is invalid")
	}
	detected := http.DetectContentType(content)
	mediaType := ""
	switch {
	case strings.HasPrefix(detected, "image/png"):
		mediaType = "image/png"
	case strings.HasPrefix(detected, "image/jpeg"):
		mediaType = "image/jpeg"
	case strings.HasPrefix(detected, "image/gif"):
		mediaType = "image/gif"
	default:
		return ManagedImage{}, fmt.Errorf("whiteboard preview media type is unsupported")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return ManagedImage{}, fmt.Errorf("whiteboard preview is not a valid image")
	}
	if !validManagedImageDimensions(config.Width, config.Height) {
		return ManagedImage{}, fmt.Errorf("whiteboard preview dimensions exceed the safety limit")
	}
	switch format {
	case "png":
		if mediaType != "image/png" {
			return ManagedImage{}, fmt.Errorf("whiteboard preview media type mismatch")
		}
	case "jpeg":
		if mediaType != "image/jpeg" {
			return ManagedImage{}, fmt.Errorf("whiteboard preview media type mismatch")
		}
	case "gif":
		if mediaType != "image/gif" {
			return ManagedImage{}, fmt.Errorf("whiteboard preview media type mismatch")
		}
	default:
		return ManagedImage{}, fmt.Errorf("whiteboard preview format is unsupported")
	}
	return ManagedImage{
		MediaType: mediaType,
		Width:     config.Width,
		Height:    config.Height,
		Bytes:     append([]byte(nil), content...),
	}, nil
}

func validManagedImageDimensions(width, height int) bool {
	return width > 0 && height > 0 &&
		width <= maxWhiteboardPreviewDimension &&
		height <= maxWhiteboardPreviewDimension &&
		int64(width)*int64(height) <= maxWhiteboardPreviewPixels
}

func publicMinutesLink(title, rawURL string) (MinutesLink, bool) {
	safeURL, ok := sanitizePublicMinutesURL(rawURL)
	if !ok {
		return MinutesLink{}, false
	}
	title = strings.TrimSpace(title)
	if title == "" || containsSensitiveMinutesText(title) {
		title = safeURL
	}
	return MinutesLink{Title: title, URL: safeURL}, true
}

func containsSensitiveMinutesText(value string) bool {
	if containsUnsafeMinutesURLData(value) {
		return true
	}
	lower := strings.ToLower(value)
	for _, key := range minutesLinkSecretKeys {
		if strings.Contains(lower, key) {
			return true
		}
	}
	return false
}

func sanitizePublicMinutesURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if err := validateSafeHTTPURL(raw); err != nil || containsUnsafeMinutesURLData(raw) {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil {
		return "", false
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, suffix := range feishuLinkHosts {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return "", false
		}
	}
	path := strings.ToLower(parsed.EscapedPath())
	for _, segment := range []string{"/minutes/", "/docx/", "/wiki/", "/drive/", "/whiteboard/", "/notes/"} {
		if strings.Contains(path, segment) {
			return "", false
		}
	}
	query := parsed.Query()
	for _, key := range minutesLinkSecretKeys {
		if _, found := query[key]; found {
			return "", false
		}
	}
	haystack := strings.ToLower(parsed.RawQuery + parsed.Fragment + parsed.Path)
	for _, key := range minutesLinkSecretKeys {
		if strings.Contains(haystack, key+"=") || strings.Contains(haystack, key+"%3d") {
			return "", false
		}
	}
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String(), true
}

func containsUnsafeMinutesURLData(raw string) bool {
	decoded := raw
	for range imaMaxURLDecodePasses + 1 {
		if feishuIdentityPattern.MatchString(decoded) ||
			feishuURLHostPattern.MatchString(decoded) ||
			nestedUnsafeSchemePattern.MatchString(decoded) {
			return true
		}
		next, err := url.QueryUnescape(decoded)
		if err != nil || next == decoded {
			return false
		}
		decoded = next
	}
	return true
}

func validMinutesWhiteboard(meta MinutesWhiteboard) bool {
	return mediaIDPattern.MatchString(meta.MediaID) &&
		sha256Pattern.MatchString(meta.SHA256) &&
		(meta.MediaType == "image/png" || meta.MediaType == "image/jpeg" || meta.MediaType == "image/gif") &&
		strings.TrimSpace(meta.Alt) != "" &&
		meta.Width >= 0 &&
		meta.Height >= 0
}

func normalizeMinutesChapters(chapters []MinutesChapter) []MinutesChapter {
	normalized := make([]MinutesChapter, 0, len(chapters))
	for _, chapter := range chapters {
		title := strings.TrimSpace(chapter.Title)
		summary := strings.TrimSpace(chapter.Summary)
		if title == "" && summary == "" {
			continue
		}
		if chapter.StartMS < 0 {
			chapter.StartMS = 0
		}
		if chapter.EndMS < chapter.StartMS {
			chapter.EndMS = 0
		}
		chapter.Title = title
		chapter.Summary = summary
		chapter.Order = len(normalized) + 1
		normalized = append(normalized, chapter)
	}
	return normalized
}

func normalizeMinutesQuotes(quotes []MinutesQuote) []MinutesQuote {
	normalized := make([]MinutesQuote, 0, len(quotes))
	for _, quote := range quotes {
		body := strings.TrimSpace(quote.Quote)
		if isPlaceholderText(body) {
			continue
		}
		normalized = append(normalized, MinutesQuote{
			Quote:       body,
			Explanation: strings.TrimSpace(quote.Explanation),
		})
	}
	return normalized
}

func normalizeMinutesLinks(links []MinutesLink) []MinutesLink {
	normalized := make([]MinutesLink, 0, len(links))
	seen := make(map[string]struct{}, len(links))
	for _, link := range links {
		safe, ok := publicMinutesLink(link.Title, link.URL)
		if !ok {
			continue
		}
		if _, exists := seen[safe.URL]; exists {
			continue
		}
		seen[safe.URL] = struct{}{}
		normalized = append(normalized, safe)
	}
	return normalized
}

func uniqueNonEmptyStrings(values []string) []string {
	output := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		output = append(output, value)
	}
	return output
}

func filterMeaningfulTexts(values []string) []string {
	output := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if isPlaceholderText(value) {
			continue
		}
		output = append(output, value)
	}
	return output
}

func isPlaceholderText(value string) bool {
	compact := strings.ToLower(strings.TrimSpace(value))
	compact = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, compact)
	_, found := placeholderTexts[compact]
	return found
}

func firstJSONString(object map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := object[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func firstJSONStringStrict(
	object map[string]json.RawMessage,
	keys ...string,
) (string, bool, error) {
	for _, key := range keys {
		raw, ok := object[key]
		if !ok {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return "", true, nil
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", true, err
		}
		return strings.TrimSpace(value), true, nil
	}
	return "", false, nil
}

func firstJSONMilliseconds(object map[string]json.RawMessage, keys ...string) (int64, bool) {
	for _, key := range keys {
		raw, ok := object[key]
		if !ok {
			continue
		}
		if ms, ok := jsonTimeToMilliseconds(raw); ok {
			return ms, true
		}
	}
	return 0, false
}

func firstJSONMillisecondsStrict(
	object map[string]json.RawMessage,
	keys ...string,
) (int64, bool, bool) {
	for _, key := range keys {
		raw, ok := object[key]
		if !ok {
			continue
		}
		milliseconds, valid := jsonTimeToMilliseconds(raw)
		return milliseconds, true, valid
	}
	return 0, false, false
}

func jsonTimeToMilliseconds(raw json.RawMessage) (int64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, false
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		if number < 0 {
			return 0, true
		}
		return int64(number), true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	if asNumber, err := strconv.ParseFloat(text, 64); err == nil {
		if asNumber < 0 {
			return 0, true
		}
		return int64(asNumber), true
	}
	parts := strings.Split(text, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	total := int64(0)
	for _, part := range parts {
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil || value < 0 {
			return 0, false
		}
		total = total*60 + value
	}
	return total * 1000, true
}

func collectXMLListItems(section string) []string {
	items := make([]string, 0)
	for _, match := range listItemPattern.FindAllStringSubmatch(section, -1) {
		text := strings.TrimSpace(stripXMLTags(match[1]))
		if text != "" {
			items = append(items, text)
		}
	}
	return items
}

func collectXMLParagraphs(section string) []string {
	items := make([]string, 0)
	for _, match := range paragraphPattern.FindAllStringSubmatch(section, -1) {
		text := strings.TrimSpace(stripXMLTags(match[1]))
		if text != "" {
			items = append(items, text)
		}
	}
	return items
}

func splitQuoteAndExplanation(item string) (string, string) {
	separators := []string{"\n", " —— ", " -- ", "：", ":"}
	for _, separator := range separators {
		if quote, explanation, ok := strings.Cut(item, separator); ok {
			quote = strings.TrimSpace(quote)
			explanation = strings.TrimSpace(explanation)
			if quote != "" {
				return quote, explanation
			}
		}
	}
	return strings.TrimSpace(item), ""
}

func stripXMLTags(value string) string {
	stripped := tagPattern.ReplaceAllString(value, " ")
	stripped = html.UnescapeString(stripped)
	stripped = strings.Join(strings.Fields(stripped), " ")
	return stripped
}

func normalizeNoteHeading(value string) string {
	return strings.TrimSpace(strings.TrimRightFunc(value, func(r rune) bool {
		return r == ':' || r == '：' || unicode.IsSpace(r)
	}))
}

func firstAttribute(attributes string, pattern *regexp.Regexp) string {
	match := pattern.FindStringSubmatch(attributes)
	if len(match) < 2 {
		return ""
	}
	for _, value := range match[1:] {
		if value = strings.TrimSpace(value); value != "" {
			return html.UnescapeString(value)
		}
	}
	return ""
}

func extractJSONString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
