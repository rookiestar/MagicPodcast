package utils

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type ShowNotesFormat string

const (
	ShowNotesFormatHTML     ShowNotesFormat = "html"
	ShowNotesFormatMarkdown ShowNotesFormat = "markdown"
)

type ShowNotesDocument struct {
	Content string          `json:"content"`
	Format  ShowNotesFormat `json:"format"`
}

var (
	markdownHeadingPattern          = regexp.MustCompile(`^[ \t]{0,3}#{1,6}[ \t]+\S`)
	markdownListPattern             = regexp.MustCompile(`^[ \t]{0,3}([-+*][ \t]+|[0-9]+[.)][ \t]+)\S`)
	markdownRulePattern             = regexp.MustCompile(`^[ \t]{0,3}((\*[ \t]*){3,}|(-[ \t]*){3,}|(_[ \t]*){3,})$`)
	markdownQuotePattern            = regexp.MustCompile(`^[ \t]{0,3}>[ \t]+\S`)
	markdownFencePattern            = regexp.MustCompile("^[ \\t]{0,3}(```|~~~)")
	markdownLinkPattern             = regexp.MustCompile(`!?\[[^]\n]+\]\((https?://|mailto:|tel:|/|#)[^)\n]+\)`)
	markdownStrongStarPattern       = regexp.MustCompile(`\*\*\S([^*\n]*\S)?\*\*`)
	markdownStrongUnderscorePattern = regexp.MustCompile(`__\S([^_\n]*\S)?__`)
	markdownEmphasisPattern         = regexp.MustCompile(`(^|[ \t(])\*[^* \t\n]([^*\n]*[^* \t\n])?\*([ \t).,!?;:]|$)`)
	htmlElementNamePattern          = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// BuildShowNotesDocument derives a deterministic display contract without
// changing the upstream Show Notes stored on the episode.
func BuildShowNotesDocument(source string) ShowNotesDocument {
	if strings.TrimSpace(source) == "" {
		return ShowNotesDocument{Content: "", Format: ShowNotesFormatMarkdown}
	}
	if !containsHTMLElement(source) {
		return ShowNotesDocument{Content: source, Format: ShowNotesFormatMarkdown}
	}

	doc, err := html.Parse(strings.NewReader(source))
	if err != nil || !containsMarkdownStructure(logicalHTMLText(doc)) {
		return ShowNotesDocument{Content: source, Format: ShowNotesFormatHTML}
	}
	return ShowNotesDocument{
		Content: htmlToMarkdownPreservingTextLineBreaks(source),
		Format:  ShowNotesFormatMarkdown,
	}
}

func containsHTMLElement(source string) bool {
	tokenizer := html.NewTokenizer(strings.NewReader(source))
	for {
		switch tokenizer.Next() {
		case html.StartTagToken, html.SelfClosingTagToken, html.EndTagToken:
			name, _ := tokenizer.TagName()
			if htmlElementNamePattern.Match(name) {
				return true
			}
		case html.ErrorToken:
			return false
		}
	}
}

func logicalHTMLText(root *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
			return
		}
		if node.Type == html.CommentNode {
			return
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "pre", "code", "script", "style", "head", "title", "noscript", "template":
				b.WriteByte('\n')
				return
			case "br", "hr":
				b.WriteByte('\n')
				return
			}
			if isHTMLBlock(node.Data) {
				b.WriteByte('\n')
				defer b.WriteByte('\n')
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return b.String()
}

func isHTMLBlock(tag string) bool {
	switch tag {
	case "address", "article", "aside", "blockquote", "dd", "div", "dl", "dt",
		"figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6",
		"header", "li", "main", "nav", "ol", "p", "section", "table", "tbody",
		"td", "tfoot", "th", "thead", "tr", "ul":
		return true
	default:
		return false
	}
}

func containsMarkdownStructure(logicalText string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(logicalText, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if markdownHeadingPattern.MatchString(trimmed) ||
			markdownListPattern.MatchString(trimmed) ||
			markdownRulePattern.MatchString(trimmed) ||
			markdownQuotePattern.MatchString(trimmed) ||
			markdownFencePattern.MatchString(trimmed) ||
			markdownLinkPattern.MatchString(trimmed) ||
			markdownStrongStarPattern.MatchString(trimmed) ||
			markdownStrongUnderscorePattern.MatchString(trimmed) ||
			markdownEmphasisPattern.MatchString(trimmed) {
			return true
		}
	}
	return false
}
