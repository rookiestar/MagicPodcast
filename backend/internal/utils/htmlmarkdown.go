package utils

import (
	stdhtml "html"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

var (
	htmlMarkdownBlankLines = regexp.MustCompile(`\n{3,}`)
	htmlMarkdownSpaceRun   = regexp.MustCompile(`[ \t\r\n\f\v]+`)
	htmlMarkdownTagStrip   = regexp.MustCompile(`<[^>]+>`)
)

// HTMLToMarkdown 把常见 show-notes HTML 转成 GitHub Flavored Markdown，保留
// 链接、图片、强调与列表；无法解析的输入降级为“去标签 + 反转义 + 折叠空白”。
//
// 设计目的：让工作流报告里的“节目详情”段是干净的 Markdown，使在线预览
// （react-markdown 无 rehypeRaw）、下载的 .md、邮件（goldmark）和 LLM 摘要
// 输入看到同一份内容，而不是一坨 raw HTML 标签。ShowNotes 来自 RSS/小宇宙的
// description，本身就是 HTML。
func HTMLToMarkdown(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return stripHTMLToPlainText(s)
	}

	var b strings.Builder
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		walkHTMLToMarkdown(&b, c)
	}

	out := normalizeMarkdown(b.String())
	return strings.TrimSpace(out)
}

// walkHTMLToMarkdown 递归把 HTML 节点树写成 Markdown 片段。
func walkHTMLToMarkdown(b *strings.Builder, n *html.Node) {
	if n == nil {
		return
	}

	switch n.Type {
	case html.TextNode:
		// 非保留空白语义下折叠连续空白为单空格；解析器已反转义实体。
		b.WriteString(htmlMarkdownSpaceRun.ReplaceAllString(n.Data, " "))
		return
	case html.CommentNode:
		return
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkHTMLToMarkdown(b, c)
		}
		return
	}

	// 整棵子树都不要的内容。
	switch n.Data {
	case "script", "style", "head", "title", "noscript", "template":
		return
	}

	switch n.Data {
	case "br":
		b.WriteString("\n")
		return
	case "hr":
		b.WriteString("\n\n---\n\n")
		return
	}

	switch n.Data {
	case "a":
		href := getHTMLAttr(n, "href")
		text := childMarkdown(n)
		if isSafeMarkdownURL(href) {
			b.WriteString("[" + text + "](" + href + ")")
		} else {
			b.WriteString(text)
		}
	case "img":
		src := getHTMLAttr(n, "src")
		alt := strings.TrimSpace(getHTMLAttr(n, "alt"))
		if isSafeMarkdownURL(src) {
			b.WriteString("![" + alt + "](" + src + ")")
		}
	case "strong", "b":
		text := strings.TrimSpace(childMarkdown(n))
		if text != "" {
			b.WriteString("**" + text + "**")
		}
	case "em", "i":
		text := strings.TrimSpace(childMarkdown(n))
		if text != "" {
			b.WriteString("*" + text + "*")
		}
	case "code":
		text := htmlTextContent(n)
		if strings.ContainsAny(text, "\n`") {
			b.WriteString(markdownCodeBlock(text))
		} else {
			b.WriteString("`" + text + "`")
		}
	case "pre":
		text := strings.TrimSpace(htmlTextContent(n))
		if text != "" {
			b.WriteString(markdownCodeBlock(text))
		}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		text := strings.TrimSpace(childMarkdown(n))
		if text != "" {
			b.WriteString("\n\n" + strings.Repeat("#", level) + " " + text + "\n\n")
		}
	case "p", "div", "section", "article":
		text := strings.TrimSpace(childMarkdown(n))
		if text != "" {
			b.WriteString("\n\n" + text + "\n\n")
		}
	case "ul":
		if rendered := renderHTMLList(n, false); rendered != "" {
			b.WriteString("\n" + rendered + "\n")
		}
	case "ol":
		if rendered := renderHTMLList(n, true); rendered != "" {
			b.WriteString("\n" + rendered + "\n")
		}
	case "blockquote":
		text := strings.TrimSpace(childMarkdown(n))
		if text != "" {
			b.WriteString("\n\n")
			for _, line := range strings.Split(text, "\n") {
				b.WriteString("> " + line + "\n")
			}
		}
	default:
		// span / u / font / 未识别标签：透传子节点。
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkHTMLToMarkdown(b, c)
		}
	}
}

// renderHTMLList 把 <ul>/<ol> 渲染为 Markdown 列表，支持嵌套。
func renderHTMLList(n *html.Node, ordered bool) string {
	var b strings.Builder
	idx := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "li" {
			continue
		}
		idx++
		var cb strings.Builder
		for gc := c.FirstChild; gc != nil; gc = gc.NextSibling {
			walkHTMLToMarkdown(&cb, gc)
		}
		item := strings.TrimSpace(cb.String())
		if item == "" {
			continue
		}
		marker := "- "
		if ordered {
			marker = strconv.Itoa(idx) + ". "
		}
		lines := strings.Split(item, "\n")
		b.WriteString(marker + strings.TrimSpace(lines[0]) + "\n")
		for _, line := range lines[1:] {
			b.WriteString("  " + strings.TrimSpace(line) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func childMarkdown(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkHTMLToMarkdown(&b, c)
	}
	return b.String()
}

func htmlTextContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil || node.Type == html.CommentNode {
			return
		}
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
			return
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "script", "style", "head", "title", "noscript", "template":
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return b.String()
}

func markdownCodeBlock(text string) string {
	text = strings.TrimSpace(text)
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	return "\n" + fence + "\n" + text + "\n" + fence + "\n"
}

func getHTMLAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// isSafeMarkdownURL 限制输出链接/图片协议，避免 javascript:/data: 等被写进报告。
func isSafeMarkdownURL(u string) bool {
	u = strings.TrimSpace(u)
	if u == "" {
		return false
	}
	lower := strings.ToLower(u)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "tel:") ||
		strings.HasPrefix(lower, "/")
}

func normalizeMarkdown(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	out := strings.Join(lines, "\n")
	out = htmlMarkdownBlankLines.ReplaceAllString(out, "\n\n")
	return out
}

// stripHTMLToPlainText 是解析失败时的降级路径，与 handlers.episodeShowNotesPreview
// 保持一致的“去标签 + 反转义 + 折叠空白”语义。
func stripHTMLToPlainText(s string) string {
	stripped := htmlMarkdownTagStrip.ReplaceAllString(s, " ")
	stripped = stdhtml.UnescapeString(stripped)
	return strings.Join(strings.Fields(stripped), " ")
}
