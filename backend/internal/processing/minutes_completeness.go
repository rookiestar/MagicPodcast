package processing

import (
	"errors"
	"regexp"
	"strings"
	"time"

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
	sections := splitNoteSections(document)
	checks := []struct {
		name  string
		count int
	}{
		{"关键决策", len(extractNoteDecisions(sections["关键决策"]))},
		{"金句时刻", len(extractNoteQuotes(firstNonEmptySection(sections, "金句时刻", "金句")))},
		{"相关链接", len(extractNoteLinks(firstNonEmptySection(sections, "相关链接", "相关外链")))},
	}
	rawByName := map[string]string{
		"关键决策": sections["关键决策"],
		"金句时刻": firstNonEmptySection(sections, "金句时刻", "金句"),
		"相关链接": firstNonEmptySection(sections, "相关链接", "相关外链"),
	}
	for _, check := range checks {
		raw := rawByName[check.name]
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if check.count > 0 ||
			(check.name == "相关链接" && noteLinksSectionIsFullyAccounted(raw)) {
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

// noteLinksSectionIsFullyAccounted distinguishes a link block whose links
// were all filtered for safety from a block whose non-empty content was not
// parsed at all. Internal Feishu links are intentionally omitted from the
// public snapshot, but unsupported residual content must still fail closed.
func noteLinksSectionIsFullyAccounted(section string) bool {
	fragments, err := nethtml.ParseFragment(
		strings.NewReader(section),
		&nethtml.Node{Type: nethtml.ElementNode, Data: "div", DataAtom: atom.Div},
	)
	if err != nil {
		return false
	}

	recognized := 0
	unaccounted := false
	for _, fragment := range fragments {
		scan := scanNoteLinkNode(fragment)
		if !scan.valid {
			return false
		}
		recognized += scan.recognized
		unaccounted = unaccounted || scan.unaccounted
	}
	return recognized > 0 && !unaccounted
}

type noteLinkNodeScan struct {
	valid       bool
	recognized  int
	unaccounted bool
}

var noteLinkContainerTags = map[string]struct{}{
	"b":      {},
	"br":     {},
	"div":    {},
	"em":     {},
	"li":     {},
	"ol":     {},
	"p":      {},
	"span":   {},
	"strong": {},
	"ul":     {},
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
		return noteLinkNodeScan{
			valid:       !plainURLPattern.MatchString(text),
			unaccounted: true,
		}
	case nethtml.ElementNode:
		name := strings.ToLower(node.Data)
		if name == "a" || name == "bookmark" {
			return noteLinkNodeScan{
				valid:      strings.TrimSpace(noteLinkNodeAttribute(node, "href")) != "",
				recognized: 1,
			}
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
		}
		// Text outside an explicit link is only a label when this container
		// also contains a recognized link. A URL-like text node is never a
		// label and remains an unparsed residual.
		if scan.recognized == 0 && scan.unaccounted {
			return noteLinkNodeScan{}
		}
		if scan.recognized > 0 {
			scan.unaccounted = false
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
		}
		return scan
	default:
		return noteLinkNodeScan{valid: true}
	}
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
