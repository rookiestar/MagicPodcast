package opml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseOPMLBytes(t *testing.T, content string) []Outline {
	t.Helper()
	parser := NewParser()
	outlines, err := parser.ParseBytes([]byte(content))
	require.NoError(t, err)
	return outlines
}

func wrapOPML(outlineXML string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0"><head><title>t</title></head><body>` + outlineXML + `</body></opml>`
}

func TestParseBytesPreservesNumericEntities(t *testing.T) {
	outlines := parseOPMLBytes(t, wrapOPML(
		`<outline title="Tom&#39;s Show &#39;直播&#39;" text="简介&#x27;A&#x27;" type="rss" xmlUrl="https://example.com/feed"/>`,
	))
	require.Len(t, outlines, 1)
	assert.Equal(t, "Tom's Show '直播'", outlines[0].GetTitle())
	assert.Equal(t, "简介'A'", outlines[0].GetDescription())
}

func TestParseBytesPreservesNamedEntitiesAndEscapesBareAmpersand(t *testing.T) {
	outlines := parseOPMLBytes(t, wrapOPML(
		`<outline title="A &amp; B" text="x &lt;tag&gt; y" type="rss" xmlUrl="https://example.com/feed?a=1&amp;b=2&amp;c=3"/>`,
	))
	require.Len(t, outlines, 1)
	assert.Equal(t, "A & B", outlines[0].GetTitle())
	assert.Equal(t, "x <tag> y", outlines[0].GetDescription())
	assert.Equal(t, "https://example.com/feed?a=1&b=2&c=3", outlines[0].XMLURL)
}

func TestParseBytesPreservesEmojiAndMultibyteURLQuery(t *testing.T) {
	outlines := parseOPMLBytes(t, wrapOPML(
		`<outline title="🎙️ 中文播客" text="😀 emoji 简介保持原样" type="rss" xmlUrl="https://example.com/feed?uto=1034&utm_source=xiaoyuzhou&episode_id=abc"/>`,
	))
	require.Len(t, outlines, 1)
	assert.Equal(t, "🎙️ 中文播客", outlines[0].Title)
	assert.Equal(t, "😀 emoji 简介保持原样", outlines[0].Text)
	assert.Equal(t, "https://example.com/feed?uto=1034&utm_source=xiaoyuzhou&episode_id=abc", outlines[0].XMLURL)
}

func TestGetTitleFallbackTruncatesByRune(t *testing.T) {
	longChinese := strings.Repeat("播客简介内容较长", 30) // 240 个 rune，含多字节字符
	title := (&Outline{Text: longChinese}).GetTitle()
	assert.True(t, len([]rune(title)) <= fallbackTitleMaxRunes+3)
	// 截断结果必须仍是合法字符串边界：逐 rune 转换不报错且不含乱码替换符。
	assert.NotContains(t, title, "�")

	emojiText := strings.Repeat("🎙", 150)
	emojiTitle := (&Outline{Text: emojiText}).GetTitle()
	assert.NotContains(t, emojiTitle, "�")
	assert.True(t, len([]rune(emojiTitle)) <= fallbackTitleMaxRunes+3)
}

func TestParseBytesEscapesUndeclaredNamedEntities(t *testing.T) {
	// &nbsp; 等未在 DTD 声明的 HTML 实体必须转义为字面文本，
	// 否则整个文件会被 XML 解析器拒绝（PR #407 review）。
	outlines := parseOPMLBytes(t, wrapOPML(
		`<outline title="A&nbsp;B &copy; 2024" text="x" type="rss" xmlUrl="https://example.com/f"/>`,
	))
	require.Len(t, outlines, 1)
	assert.Equal(t, "A&nbsp;B &copy; 2024", outlines[0].Title)
	// 转义后作为字面文本保留，与既有预处理行为一致。
	assert.Equal(t, "A&nbsp;B &copy; 2024", outlines[0].GetTitle())
}

func TestParseBytesMalformedXMLReturnsParseError(t *testing.T) {
	parser := NewParser()
	_, err := parser.ParseBytes([]byte(`<?xml version="1.0"?><opml><body><outline`))
	require.Error(t, err)
	var parseErr *ParseError
	require.ErrorAs(t, err, &parseErr)

	_, err = parser.ParseBytes([]byte(""))
	require.Error(t, err)
	require.ErrorAs(t, err, &parseErr)
}

func TestParseBytesExtractsNestedOutlines(t *testing.T) {
	outlines := parseOPMLBytes(t, wrapOPML(
		`<outline text="分类文件夹"><outline title="Show A" text="a" type="rss" xmlUrl="https://example.com/a.xml"/></outline>`,
	))
	require.Len(t, outlines, 1)
	assert.Equal(t, "https://example.com/a.xml", outlines[0].XMLURL)
}
