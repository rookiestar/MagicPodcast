package processing

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMinutesChaptersAndKeywordsPreserveOrder(t *testing.T) {
	chapters := parseMinutesChapters(json.RawMessage(`[
		{"start_time": 120000, "end_time": 240000, "title": "中段", "summary": "讨论方案"},
		{"start_ms": 0, "title": "开场", "abstract": "介绍背景"},
		{"title": "   ", "summary": ""}
	]`))
	require.Equal(t, []MinutesChapter{
		{Order: 1, StartMS: 120000, EndMS: 240000, Title: "中段", Summary: "讨论方案"},
		{Order: 2, StartMS: 0, Title: "开场", Summary: "介绍背景"},
	}, chapters)

	keywords := parseMinutesKeywords(json.RawMessage(`[
		"产品",
		{"name": "AI"},
		{"keyword": "产品"},
		"  "
	]`))
	require.Equal(t, []string{"产品", "AI"}, keywords)
}

func TestParseNoteSectionsExtractsKnownBlocksAndIgnoresUnknown(t *testing.T) {
	document := `
<title>智能纪要</title>
<h1>总结</h1>
<whiteboard token="wbcn_board_123"/>
<p>这段总结不得覆盖妙记 Summary。</p>
<h2>智能章节</h2>
<p>章节以妙记详情为准。</p>
<h1>关键决策</h1>
<ul>
  <li>采用方案 A</li>
  <li>暂无</li>
  <li>继续观察指标</li>
</ul>
<h1>金句时刻</h1>
<blockquote>真正重要的是长期主义</blockquote>
<p>这句话强调节奏。</p>
<blockquote> </blockquote>
<h1>相关链接</h1>
<p><a href="https://example.com/article">外部文章</a></p>
<p><a href="https://example.feishu.cn/minutes/obcn_secret">飞书内部</a></p>
<p><a href="javascript:alert(1)">危险协议</a></p>
<p><bookmark name="安全书签" href="https://example.org/notes"></bookmark></p>
<p><a href="https://cdn.example.com/file?minute_token=abc">带身份参数</a></p>
<h1>未知区块</h1>
<p>不渲染未知内容</p>
`
	decisions, quotes, links, token := parseNoteSections(document)
	require.Equal(t, "wbcn_board_123", token)
	require.Equal(t, []string{"采用方案 A", "继续观察指标"}, decisions)
	require.Equal(t, []MinutesQuote{{
		Quote:       "真正重要的是长期主义",
		Explanation: "这句话强调节奏。",
	}}, quotes)
	require.Equal(t, []MinutesLink{
		{Title: "外部文章", URL: "https://example.com/article"},
		{Title: "安全书签", URL: "https://example.org/notes"},
	}, links)
}

func TestPlaceholderOnlyDecisionsAreOmitted(t *testing.T) {
	decisions, quotes, links, token := parseNoteSections(`
<h1>关键决策</h1>
<p>暂无关键决策</p>
<h1>金句时刻</h1>
<p>无</p>
<h1>相关链接</h1>
<p><a href="https://feishu.cn/docx/abc">内部文档</a></p>
`)
	require.Empty(t, decisions)
	require.Empty(t, quotes)
	require.Empty(t, links)
	require.Empty(t, token)
}

func TestSniffManagedImageRejectsWrongTypeAndOversize(t *testing.T) {
	var pngBuf bytes.Buffer
	require.NoError(t, png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	parsed, err := sniffManagedImage(pngBuf.Bytes())
	require.NoError(t, err)
	require.Equal(t, "image/png", parsed.MediaType)
	require.Equal(t, 2, parsed.Width)
	require.Equal(t, 2, parsed.Height)

	_, err = sniffManagedImage([]byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>"))
	require.Error(t, err)
	_, err = sniffManagedImage(bytes.Repeat([]byte("x"), int(maxWhiteboardPreviewBytes)+1))
	require.Error(t, err)
}

func TestEncodeMinutesEnrichmentOmitsEmptyDocument(t *testing.T) {
	encoded, err := encodeMinutesEnrichment(MinutesEnrichment{})
	require.NoError(t, err)
	require.Nil(t, encoded)

	encoded, err = encodeMinutesEnrichment(MinutesEnrichment{
		Chapters: []MinutesChapter{{Title: "开场", StartMS: 0}},
		Keywords: []string{"AI"},
	})
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"schema_version": "1.0.0"`)
	require.NotContains(t, string(encoded), "minute_token")
	decoded, err := decodeMinutesEnrichment(encoded)
	require.NoError(t, err)
	require.Equal(t, "开场", decoded.Chapters[0].Title)
	require.Equal(t, []string{"AI"}, decoded.Keywords)
}

func TestSanitizePublicMinutesURLRejectsFeishuAndSecrets(t *testing.T) {
	_, ok := sanitizePublicMinutesURL("https://example.com/a")
	require.True(t, ok)
	for _, raw := range []string{
		"http://example.com/a",
		"https://feishu.cn/minutes/abc",
		"https://open.feishu.cn/open-apis/drive/v1/files",
		"https://example.com/doc?note_id=abc",
		"https://user:pass@example.com/a",
		"javascript:alert(1)",
	} {
		_, ok := sanitizePublicMinutesURL(raw)
		require.False(t, ok, raw)
	}
}

func TestMinutesEnrichmentJSONDoesNotEmbedSecrets(t *testing.T) {
	encoded, err := encodeMinutesEnrichment(MinutesEnrichment{
		Links: []MinutesLink{{Title: "外链", URL: "https://example.com/ok"}},
		Whiteboard: &MinutesWhiteboard{
			MediaID:   minutesWhiteboardMediaID,
			MediaType: "image/png",
			Width:     8,
			Height:    8,
			SHA256:    strings.Repeat("a", 64),
			Alt:       minutesWhiteboardAlt,
		},
	})
	require.NoError(t, err)
	for _, secret := range []string{"minute_token", "note_id", "file_token", "wbcn_", "/tmp/", "obcn_"} {
		require.NotContains(t, string(encoded), secret)
	}
}
