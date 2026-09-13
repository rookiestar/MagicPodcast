package opml

import (
	"io"
	"os"
	"regexp"
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

	// 如果 title 为空，使用 text 字段作为备用，并按 rune 截取过长文本，
	// 避免按字节截断切开中文/emoji 等多字节字符（#398 R8）。
	text := strings.TrimSpace(o.Text)
	if text == "" {
		return "Unknown Podcast"
	}
	runes := []rune(text)
	if len(runes) <= fallbackTitleMaxRunes {
		return text
	}
	head := runes[:fallbackTitleMaxRunes]
	// 优先在截断范围内的首个换行或句读处收尾，保持与原语义一致。
	for i := 1; i < len(head); i++ {
		switch head[i] {
		case '\n', '。', '！', '？', '.':
			return strings.TrimSpace(string(head[:i]))
		}
	}
	return string(head) + "..."
}

// fallbackTitleMaxRunes 限制 text 兜底标题的最大 rune 数。
const fallbackTitleMaxRunes = 100

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

// ParseError 标记输入文件本身无法解析（空文件、畸形 XML、非 OPML 内容）。
// 调用方据此把「文件非法」与「导入过程失败」区分开（#398 R11）。
type ParseError struct {
	Err error
}

func (e *ParseError) Error() string { return e.Err.Error() }
func (e *ParseError) Unwrap() error { return e.Err }

// ParseFile 从文件解析OPML
func (p *Parser) ParseFile(filePath string) ([]Outline, error) {
	// 读取文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return p.ParseBytes(data)
}

// ParseReader 从io.Reader解析OPML
func (p *Parser) ParseReader(reader io.Reader) ([]Outline, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return p.ParseBytes(data)
}

// ParseBytes 解析OPML内容。解析失败一律包装为 *ParseError。
func (p *Parser) ParseBytes(data []byte) ([]Outline, error) {
	// 预处理：仅转义确实非法的裸 &
	data = p.preprocessXML(data)

	doc, err := opml.NewOPML(data)
	if err != nil {
		return nil, &ParseError{Err: err}
	}

	return p.extractOutlines(*doc)
}

// ampersandOrEntity 匹配一个 & 及其可选的实体主体。Go 的 RE2 不支持负向
// 前瞻，因此用 ReplaceAllStringFunc 判断：带完整实体形态（命名实体或
// 十进制/十六进制数字实体）的原样保留，其余裸 & 转义。旧的五实体白名单
// 会把 &#39; 改写成 &amp;#39;，破坏数字实体和中文（#398 R8）。
var ampersandOrEntity = regexp.MustCompile(`&(?:#x?[0-9A-Fa-f]+;|[a-zA-Z][a-zA-Z0-9]*;)?`)

// preprocessXML 预处理XML，仅转义确实非法的裸 &
func (p *Parser) preprocessXML(data []byte) []byte {
	return []byte(ampersandOrEntity.ReplaceAllStringFunc(string(data), func(match string) string {
		if strings.HasSuffix(match, ";") {
			return match
		}
		return "&amp;"
	}))
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
