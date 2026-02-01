package opml

import (
	"io"
	"os"
	"strings"

	"github.com/gilliek/go-opml/opml"
)

// Outline OPML outline结构（小宇宙格式）
// 注意：小宇宙的 OPML 格式与标准 OPML 规范不同：
// - title: 播客标题（短文本，如 "无时差研究所"）
// - text: 播客描述（长文本，完整的节目介绍）
// - xmlUrl: RSS Feed URL
// - htmlUrl: 网站链接（可选）
type Outline struct {
	Text    string `xml:"text,attr"`    // 播客描述（长文本）
	Title   string `xml:"title,attr"`   // 播客标题（短文本）
	XMLURL  string `xml:"xmlUrl,attr"`  // Feed URL
	HTMLURL string `xml:"htmlUrl,attr"` // 网站链接（可选）
	Type    string `xml:"type,attr"`    // 类型（通常为 "rss"）
}

// GetTitle 获取播客标题（优先使用 title，为空时使用 text）
func (o *Outline) GetTitle() string {
	// 优先使用 title 字段
	if o.Title != "" {
		return o.Title
	}

	// 如果 title 为空，使用 text 字段作为备用
	// 并智能截取过长的文本
	text := strings.TrimSpace(o.Text)
	if text != "" {
		if len(text) > 100 {
			// 尝试在换行符或句号处截断
			if idx := strings.IndexAny(text, "\n。！？."); idx > 0 && idx < 100 {
				return strings.TrimSpace(text[:idx])
			}
			// 如果没有合适的截断点，截取前100个字符
			return text[:100] + "..."
		}
		return text
	}

	return "Unknown Podcast"
}

// GetDescription 获取播客描述（从 text 字段）
func (o *Outline) GetDescription() string {
	return o.Text
}

// Parser OPML解析器
type Parser struct{}

// NewParser 创建OPML解析器
func NewParser() *Parser {
	return &Parser{}
}

// ParseFile 从文件解析OPML
func (p *Parser) ParseFile(filePath string) ([]Outline, error) {
	// 读取文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// 预处理：修复常见的XML转义问题
	data = p.preprocessXML(data)

	doc, err := opml.NewOPML(data)
	if err != nil {
		return nil, err
	}

	return p.extractOutlines(*doc)
}

// ParseReader 从io.Reader解析OPML
func (p *Parser) ParseReader(reader io.Reader) ([]Outline, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	// 预处理：修复常见的XML转义问题
	data = p.preprocessXML(data)

	doc, err := opml.NewOPML(data)
	if err != nil {
		return nil, err
	}

	return p.extractOutlines(*doc)
}

// preprocessXML 预处理XML，修复常见的转义问题
func (p *Parser) preprocessXML(data []byte) []byte {
	content := string(data)

	// 修复未转义的 & 字符（最常见的XML问题）
	// 策略：先保护已转义的实体，然后替换剩余的&，最后恢复

	// 1. 保护已正确转义的实体
	content = strings.ReplaceAll(content, "&amp;", "\x00AMP;")
	content = strings.ReplaceAll(content, "&lt;", "\x00LT;")
	content = strings.ReplaceAll(content, "&gt;", "\x00GT;")
	content = strings.ReplaceAll(content, "&quot;", "\x00QUOT;")
	content = strings.ReplaceAll(content, "&apos;", "\x00APOS;")

	// 2. 替换剩余未转义的 &
	content = strings.ReplaceAll(content, "&", "&amp;")

	// 3. 恢复已转义的实体
	content = strings.ReplaceAll(content, "\x00AMP;", "&amp;")
	content = strings.ReplaceAll(content, "\x00LT;", "&lt;")
	content = strings.ReplaceAll(content, "\x00GT;", "&gt;")
	content = strings.ReplaceAll(content, "\x00QUOT;", "&quot;")
	content = strings.ReplaceAll(content, "\x00APOS;", "&apos;")

	return []byte(content)
}

// extractOutlines 从OPML文档提取RSS URL列表
func (p *Parser) extractOutlines(doc opml.OPML) ([]Outline, error) {
	var outlines []Outline

	for _, outline := range doc.Body.Outlines {
		// 只保留有XMLURL的outline（即RSS feed）
		if outline.XMLURL != "" {
			outlines = append(outlines, Outline{
				Text:    outline.Text,
				Title:   outline.Title,
				XMLURL:  outline.XMLURL,
				HTMLURL: outline.HTMLURL,
				Type:    outline.Type,
			})
		}

		// 递归处理嵌套的outlines
		if len(outline.Outlines) > 0 {
			childOutlines, err := p.extractChildOutlines(outline)
			if err != nil {
				return nil, err
			}
			outlines = append(outlines, childOutlines...)
		}
	}

	return outlines, nil
}

// extractChildOutlines 递归提取子outline
func (p *Parser) extractChildOutlines(outline opml.Outline) ([]Outline, error) {
	var outlines []Outline

	for _, child := range outline.Outlines {
		if child.XMLURL != "" {
			outlines = append(outlines, Outline{
				Text:    child.Text,
				Title:   child.Title,
				XMLURL:  child.XMLURL,
				HTMLURL: child.HTMLURL,
				Type:    child.Type,
			})
		}

		if len(child.Outlines) > 0 {
			childOutlines, err := p.extractChildOutlines(child)
			if err != nil {
				return nil, err
			}
			outlines = append(outlines, childOutlines...)
		}
	}

	return outlines, nil
}
