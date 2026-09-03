package processing

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	minutesEnrichmentWaitDuration        = 30 * time.Minute
	minutesEnrichmentExpiredProbeTimeout = 5 * time.Second

	minutesEnrichmentTimeoutCode        = "minutes_enrichment_timeout"
	minutesEnrichmentTemplateCode       = "minutes_template_unrecognized"
	minutesEnrichmentSectionCode        = "minutes_section_unparsed"
	minutesEnrichmentWhiteboardCode     = "minutes_whiteboard_unavailable"
	minutesEnrichmentNoteUnreadableCode = "minutes_note_unreadable"
	minutesEnrichmentSnapshotWriteCode  = "minutes_enrichment_snapshot_write_failed"
	minutesEnrichmentSnapshotStoredCode = "stored_enrichment_unavailable"

	minutesEnrichmentTimeoutMessage    = "Feishu intelligent minutes did not become complete before the wait ended"
	minutesEnrichmentTemplateMessage   = "Feishu intelligent minutes template is unrecognized"
	minutesEnrichmentSectionMessage    = "Feishu intelligent minutes section could not be parsed"
	minutesEnrichmentWhiteboardMessage = "Feishu intelligent minutes whiteboard could not be captured"
	minutesEnrichmentNoteUnreadableMsg = "Feishu intelligent minutes could not be read"
)

// isMinutesEnrichmentResyncError identifies failures for which an explicit
// retry should discard the enrichment snapshot and read the mutable Feishu
// Minute again. Other failures must keep a completed local snapshot intact.
func isMinutesEnrichmentResyncError(code string) bool {
	switch strings.TrimSpace(code) {
	case minutesEnrichmentTimeoutCode,
		minutesEnrichmentTemplateCode,
		minutesEnrichmentSectionCode,
		minutesEnrichmentWhiteboardCode,
		minutesEnrichmentNoteUnreadableCode,
		minutesEnrichmentSnapshotWriteCode,
		minutesEnrichmentSnapshotStoredCode:
		return true
	default:
		return false
	}
}

func isMinutesEnrichmentCredentialError(code string) bool {
	switch strings.TrimSpace(code) {
	case "lark_auth_expired", "lark_permission_denied":
		return true
	default:
		return false
	}
}

type minutesEnrichmentDecision struct {
	Complete   bool
	Wait       bool
	Code       string
	Message    string
	Diagnostic string
	Retryable  bool
}

func minutesEnrichmentDeadline(coreReady, now time.Time) time.Time {
	if coreReady.IsZero() {
		coreReady = now
	}
	return coreReady.UTC().Add(minutesEnrichmentWaitDuration)
}

func enrichmentWaitExpired(deadline, now time.Time) bool {
	if deadline.IsZero() {
		return false
	}
	return !now.UTC().Before(deadline.UTC())
}

func parseCheckpointTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func formatCheckpointTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func evaluateReadableNoteDocument(
	document string,
	whiteboardToken string,
	whiteboardCaptured bool,
	whiteboardWaitable bool,
) minutesEnrichmentDecision {
	hasKnown := hasKnownTopLevelNoteSection(document)
	hasUnknown := hasUnknownTopLevelNoteSection(document)
	if !hasKnown {
		if strings.TrimSpace(document) == "" {
			return minutesEnrichmentDecision{Complete: true}
		}
		return minutesEnrichmentDecision{
			Code:       minutesEnrichmentTemplateCode,
			Message:    minutesEnrichmentTemplateMessage,
			Diagnostic: "note_sections_unrecognized",
		}
	}
	if name, unparsed := firstUnparsedTargetSection(document); unparsed {
		return minutesEnrichmentDecision{
			Code:       minutesEnrichmentSectionCode,
			Message:    minutesEnrichmentSectionMessage,
			Diagnostic: "note_section_unparsed:" + name,
		}
	}
	if strings.TrimSpace(whiteboardToken) != "" && !whiteboardCaptured {
		if whiteboardWaitable {
			return minutesEnrichmentDecision{
				Wait:       true,
				Diagnostic: "whiteboard_pending",
			}
		}
		return minutesEnrichmentDecision{
			Code:       minutesEnrichmentWhiteboardCode,
			Message:    minutesEnrichmentWhiteboardMessage,
			Diagnostic: "whiteboard_unavailable",
		}
	}
	decision := minutesEnrichmentDecision{Complete: true}
	if hasUnknown {
		decision.Diagnostic = "note_sections_ignored"
	}
	return decision
}

func firstUnparsedTargetSection(document string) (string, bool) {
	if name, duplicate := duplicateRelatedLinkSection(document); duplicate {
		return name, true
	}
	sections := splitNoteSections(document)
	quoteSection := firstNonEmptySection(sections, "金句时刻", "金句")
	checks := []struct {
		name  string
		count int
		raw   string
	}{
		{"关键决策", len(extractNoteDecisions(sections["关键决策"])), sections["关键决策"]},
		{"金句时刻", len(extractNoteQuotes(quoteSection)), quoteSection},
		{"相关链接", len(extractNoteLinks(sections["相关链接"])), sections["相关链接"]},
		{"相关外链", len(extractNoteLinks(sections["相关外链"])), sections["相关外链"]},
	}
	for _, check := range checks {
		raw := check.raw
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if check.name == "相关链接" || check.name == "相关外链" {
			if noteLinksSectionIsFullyAccounted(raw) {
				continue
			}
			return check.name, true
		} else if check.count > 0 {
			continue
		}
		visible := strings.TrimSpace(stripXMLTags(raw))
		if visible == "" || isPlaceholderText(visible) {
			continue
		}
		return check.name, true
	}
	return "", false
}

func duplicateRelatedLinkSection(document string) (string, bool) {
	parsed, err := nethtml.Parse(strings.NewReader(document))
	if err != nil {
		return "", false
	}
	counts := make(map[string]int)
	duplicate := ""
	var visit func(*nethtml.Node)
	visit = func(node *nethtml.Node) {
		if node == nil || duplicate != "" {
			return
		}
		if node.Type == nethtml.ElementNode {
			name := strings.ToLower(node.Data)
			if len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6' {
				title := normalizeNoteHeading(noteLinkNodeText(node))
				if title == "相关链接" || title == "相关外链" {
					counts[title]++
					if counts[title] > 1 {
						duplicate = title
						return
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(parsed)
	return duplicate, duplicate != ""
}

// noteLinksSectionIsFullyAccounted distinguishes a link block whose links
// were all filtered for safety from a block whose non-empty content was not
// parsed at all. Internal Feishu links are intentionally omitted from the
// public snapshot, but unsupported residual content must still fail closed.
func noteLinksSectionIsFullyAccounted(section string) bool {
	section = normalizeSelfClosingBookmarks(section)
	fragments, err := nethtml.ParseFragment(
		strings.NewReader(section),
		&nethtml.Node{Type: nethtml.ElementNode, Data: "div", DataAtom: atom.Div},
	)
	if err != nil {
		return false
	}

	recognized := 0
	unaccounted := false
	hrefs := make([]string, 0)
	for _, fragment := range fragments {
		scan := scanNoteLinkNode(fragment)
		if !scan.valid {
			return false
		}
		recognized += scan.recognized
		unaccounted = unaccounted || scan.unaccounted
		hrefs = append(hrefs, scan.hrefs...)
	}
	if recognized == 0 && !unaccounted {
		visible := strings.TrimSpace(stripXMLTags(section))
		if visible == "" || isPlaceholderText(visible) {
			return true
		}
	}
	if recognized == 0 || unaccounted {
		return false
	}
	extracted := make(map[string]struct{})
	for _, link := range extractNoteLinks(section) {
		extracted[link.URL] = struct{}{}
	}
	for _, href := range hrefs {
		safeURL, ok := sanitizePublicMinutesURL(href)
		if !ok {
			continue
		}
		if _, ok := extracted[safeURL]; !ok {
			return false
		}
	}
	return true
}

var htmlCommentPattern = regexp.MustCompile(`(?is)<!--.*?-->`)

func stripHTMLComments(value string) string {
	return htmlCommentPattern.ReplaceAllString(value, "")
}

func normalizeSelfClosingBookmarks(section string) string {
	var normalized strings.Builder
	for index := 0; index < len(section); {
		if section[index] != '<' {
			normalized.WriteByte(section[index])
			index++
			continue
		}
		if strings.HasPrefix(section[index:], "<!--") {
			end := strings.Index(section[index+4:], "-->")
			if end < 0 {
				normalized.WriteString(section[index:])
				break
			}
			end += index + 7
			normalized.WriteString(section[index:end])
			index = end
			continue
		}
		end, ok := findHTMLTagEnd(section, index+1)
		if !ok {
			normalized.WriteString(section[index:])
			break
		}
		tag := section[index : end+1]
		if isSelfClosingBookmarkTag(tag) {
			inner := strings.TrimSpace(tag[len("<bookmark") : len(tag)-1])
			inner = strings.TrimSpace(strings.TrimSuffix(inner, "/"))
			normalized.WriteString("<bookmark")
			if inner != "" {
				normalized.WriteByte(' ')
			}
			normalized.WriteString(inner)
			normalized.WriteString("></bookmark>")
		} else {
			normalized.WriteString(tag)
		}
		index = end + 1
	}
	return normalized.String()
}

func isSelfClosingBookmarkTag(tag string) bool {
	if len(tag) < len("<bookmark>") || !strings.EqualFold(tag[:len("<bookmark")], "<bookmark") {
		return false
	}
	if len(tag) > len("<bookmark>") {
		next := tag[len("<bookmark")]
		if next != '/' && next != '>' && next != ' ' && next != '\t' && next != '\n' && next != '\r' && next != '\f' {
			return false
		}
	}
	inner := strings.TrimSpace(tag[len("<bookmark") : len(tag)-1])
	return strings.HasSuffix(inner, "/")
}

func findHTMLTagEnd(value string, start int) (int, bool) {
	var quote byte
	for index := start; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if character == '>' {
			return index, true
		}
	}
	return 0, false
}

type noteLinkNodeScan struct {
	valid       bool
	recognized  int
	unaccounted bool
	hrefs       []string
}

var noteLinkContainerTags = map[string]struct{}{
	"b":          {},
	"blockquote": {},
	"br":         {},
	"cite":       {},
	"code":       {},
	"del":        {},
	"div":        {},
	"em":         {},
	"font":       {},
	"i":          {},
	"ins":        {},
	"kbd":        {},
	"label":      {},
	"li":         {},
	"mark":       {},
	"ol":         {},
	"p":          {},
	"pre":        {},
	"q":          {},
	"s":          {},
	"section":    {},
	"small":      {},
	"span":       {},
	"strike":     {},
	"strong":     {},
	"sub":        {},
	"sup":        {},
	"time":       {},
	"tt":         {},
	"u":          {},
	"ul":         {},
	"var":        {},
}

var plainURLPattern = regexp.MustCompile(`(?i)(?:https?://|www\.)`)

func scanNoteLinkNode(node *nethtml.Node) noteLinkNodeScan {
	if node == nil {
		return noteLinkNodeScan{}
	}
	switch node.Type {
	case nethtml.TextNode:
		text := strings.TrimSpace(node.Data)
		if text == "" {
			return noteLinkNodeScan{valid: true}
		}
		if isPlaceholderText(text) {
			return noteLinkNodeScan{valid: true}
		}
		if isNoteLinkSeparatorText(text) {
			return noteLinkNodeScan{valid: true}
		}
		return noteLinkNodeScan{
			valid:       !plainURLPattern.MatchString(text),
			unaccounted: true,
		}
	case nethtml.ElementNode:
		name := strings.ToLower(node.Data)
		if name == "a" || name == "bookmark" {
			href := strings.TrimSpace(noteLinkNodeAttribute(node, "href"))
			if href == "" || noteLinkNodeHasAdditionalURLAttribute(node) {
				return noteLinkNodeScan{}
			}
			_, public := sanitizePublicMinutesURL(href)
			if noteLinkNodeHasUnsupportedDescendant(node, !public) {
				return noteLinkNodeScan{}
			}
			return noteLinkNodeScan{
				valid:      true,
				recognized: 1,
				hrefs:      []string{href},
			}
		}
		if noteLinkNodeHasURLAttribute(node) {
			return noteLinkNodeScan{}
		}
		if _, ok := noteLinkContainerTags[name]; !ok {
			return noteLinkNodeScan{}
		}

		scan := noteLinkNodeScan{valid: true}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			childScan := scanNoteLinkNode(child)
			if !childScan.valid {
				return noteLinkNodeScan{}
			}
			scan.recognized += childScan.recognized
			scan.unaccounted = scan.unaccounted || childScan.unaccounted
			scan.hrefs = append(scan.hrefs, childScan.hrefs...)
		}
		// Text outside an explicit link remains residual content even when the
		// same container also contains a recognized link.
		if scan.recognized == 0 && scan.unaccounted {
			return noteLinkNodeScan{}
		}
		return scan
	case nethtml.DocumentNode:
		scan := noteLinkNodeScan{valid: true}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			childScan := scanNoteLinkNode(child)
			if !childScan.valid {
				return noteLinkNodeScan{}
			}
			scan.recognized += childScan.recognized
			scan.unaccounted = scan.unaccounted || childScan.unaccounted
			scan.hrefs = append(scan.hrefs, childScan.hrefs...)
		}
		return scan
	default:
		return noteLinkNodeScan{valid: true}
	}
}

func isNoteLinkSeparatorText(text string) bool {
	for _, character := range text {
		if unicode.IsSpace(character) || unicode.IsPunct(character) || unicode.IsSymbol(character) {
			continue
		}
		return false
	}
	return text != ""
}

func noteLinkNodeHasUnsupportedDescendant(node *nethtml.Node, inspectTextURLs bool) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.TextNode {
			if inspectTextURLs && plainURLPattern.MatchString(strings.TrimSpace(child.Data)) {
				return true
			}
			continue
		}
		if child.Type != nethtml.ElementNode {
			continue
		}
		name := strings.ToLower(child.Data)
		if name == "a" || name == "bookmark" || noteLinkNodeHasURLAttribute(child) {
			return true
		}
		if _, ok := noteLinkContainerTags[name]; !ok {
			return true
		}
		if noteLinkNodeHasUnsupportedDescendant(child, inspectTextURLs) {
			return true
		}
	}
	return false
}

func noteLinkNodeHasURLAttribute(node *nethtml.Node) bool {
	for _, attribute := range node.Attr {
		if isNoteLinkURLAttribute(attribute.Key) && strings.TrimSpace(attribute.Val) != "" {
			return true
		}
	}
	return false
}

func noteLinkNodeHasAdditionalURLAttribute(node *nethtml.Node) bool {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, "href") {
			continue
		}
		if isNoteLinkURLAttribute(attribute.Key) && strings.TrimSpace(attribute.Val) != "" {
			return true
		}
	}
	return false
}

func isNoteLinkURLAttribute(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "href" || name == "src" || name == "url" || name == "link" ||
		strings.HasSuffix(name, "-href") || strings.HasSuffix(name, "_href") ||
		strings.HasSuffix(name, "-url") || strings.HasSuffix(name, "_url")
}

func noteLinkNodeAttribute(node *nethtml.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func firstNonEmptySection(sections map[string]string, names ...string) string {
	for _, name := range names {
		if strings.TrimSpace(sections[name]) != "" {
			return sections[name]
		}
	}
	return ""
}

func minutesErrorIsWaitable(err error) bool {
	if err == nil {
		return false
	}
	mapped := err
	if classified, _ := classifyLarkCommandError(err); classified != nil {
		mapped = classified
	}
	if errors.Is(mapped, errLarkMinutesPending) {
		return true
	}
	var adapterErr *AdapterError
	if !errors.As(mapped, &adapterErr) {
		return true
	}
	if adapterErr.ResultUnknown || adapterErr.CanRetry {
		return true
	}
	switch adapterErr.ErrorCode {
	case "lark_permission_denied",
		"lark_auth_expired",
		"lark_protocol_error",
		"lark_request_rejected",
		"lark_resource_not_found",
		"invalid_external_checkpoint":
		return false
	default:
		return false
	}
}

func minutesEnrichmentTimeoutDecision() minutesEnrichmentDecision {
	return minutesEnrichmentDecision{
		Code:       minutesEnrichmentTimeoutCode,
		Message:    minutesEnrichmentTimeoutMessage,
		Diagnostic: "note_not_ready",
	}
}
