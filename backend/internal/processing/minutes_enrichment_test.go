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

func TestParseMinutesMetadataRejectsNonEmptyFormatDrift(t *testing.T) {
	_, err := parseMinutesChaptersStrict(json.RawMessage(`{"items":[]}`))
	require.Error(t, err)
	_, err = parseMinutesChaptersStrict(json.RawMessage(`[{"unknown":"value"}]`))
	require.Error(t, err)
	_, err = parseMinutesChaptersStrict(json.RawMessage(`[{"title":"章节","start_time":{"bad":true}}]`))
	require.Error(t, err)

	_, err = parseMinutesKeywordsStrict(json.RawMessage(`{"items":[]}`))
	require.Error(t, err)
	_, err = parseMinutesKeywordsStrict(json.RawMessage(`[{"unknown":"value"}]`))
	require.Error(t, err)
}

func TestPublicMinutesLinkReplacesSensitiveTitle(t *testing.T) {
	link, ok := publicMinutesLink(
		"minute_token=obcn_secret_123",
		"https://example.com/guide",
	)
	require.True(t, ok)
	require.Equal(t, "https://example.com/guide", link.Title)
	require.NotContains(t, link.Title, "obcn_secret_123")
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

func TestParseNoteSectionsExtractsSafeLinksWithHTMLAttributeSyntax(t *testing.T) {
	_, _, links, _ := parseNoteSections(`
<h1>相关链接</h1>
<p><a href='https://example.com/single'>单引号</a></p>
<p><a href=https://example.com/unquoted?x=1&amp;y=2>无引号</a></p>
<p><bookmark name='安全书签' href=https://example.org/bookmark></bookmark></p>
`)
	require.Equal(t, []MinutesLink{
		{Title: "单引号", URL: "https://example.com/single"},
		{Title: "无引号", URL: "https://example.com/unquoted?x=1&y=2"},
		{Title: "安全书签", URL: "https://example.org/bookmark"},
	}, links)
}

func TestParseNoteSectionsIgnoresSimilarAttributeNames(t *testing.T) {
	_, _, links, _ := parseNoteSections(`
<h1>相关链接</h1>
<p><a data-href='https://example.com/wrong' href='https://feishu.cn/docx/internal'>内部</a></p>
<p><a title="junk href=https://example.com/wrong" href="https://example.com/right">真实链接</a></p>
`)
	require.Equal(t, []MinutesLink{{Title: "真实链接", URL: "https://example.com/right"}}, links)
}

func TestParseNoteSectionsIgnoresCommentedLinks(t *testing.T) {
	_, _, links, _ := parseNoteSections(`
<h1>相关链接</h1>
<p><a href="https://example.com/live">Live</a><!-- <a href='https://example.com/old'>Old</a> --></p>
<div data-template="<a href='https://example.com/phantom'>Phantom</a>"><a href="https://example.com/real">Real</a></div>
`)
	require.Equal(t, []MinutesLink{
		{Title: "Live", URL: "https://example.com/live"},
		{Title: "Real", URL: "https://example.com/real"},
	}, links)
}

func TestParseNoteSectionsMergesRelatedLinkAliases(t *testing.T) {
	_, _, links, _ := parseNoteSections(`
<h1>相关链接</h1><p><a href="https://example.com/primary">主链接</a></p>
<h1>相关外链</h1><p><a href="https://example.com/alternate">备用链接</a></p>
`)
	require.Equal(t, []MinutesLink{
		{Title: "主链接", URL: "https://example.com/primary"},
		{Title: "备用链接", URL: "https://example.com/alternate"},
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

func TestParseNoteSectionsExtractsProviderParagraphQuotes(t *testing.T) {
	_, quotes, _, _ := parseNoteSections(`
<h1>金句时刻</h1>
<p>「所以最终你发现他其实不是一个技术的判断，最后是个经济学的判断。」</p>
<p>—— 精准点出了技术路线选择背后的核心逻辑。</p>
<p></p>
<p>「这是第一次，我觉得工作不再占据人的主要时间了。」</p>
<p>—— 指出了 AI 对社会结构的长期影响。</p>
`)
	require.Equal(t, []MinutesQuote{
		{
			Quote:       "「所以最终你发现他其实不是一个技术的判断，最后是个经济学的判断。」",
			Explanation: "精准点出了技术路线选择背后的核心逻辑。",
		},
		{
			Quote:       "「这是第一次，我觉得工作不再占据人的主要时间了。」",
			Explanation: "指出了 AI 对社会结构的长期影响。",
		},
	}, quotes)
}

func TestParseNoteVisualSourcesPreservesOrderAndMetadata(t *testing.T) {
	sources, err := parseNoteVisualSources(`
<h1>总结</h1>
<img token="filecn_image_one" type="image/png" width="2" height="2" alt="第一张" summary="开场图"/>
<div><image file_token='filecn_image_two' media-type='image/jpeg' aria-label='第二张'></image></div>
`)
	require.NoError(t, err)
	require.Equal(t, []minutesVisualSource{
		{
			Token:         "filecn_image_one",
			MediaType:     "image/png",
			Alt:           "第一张",
			Summary:       "开场图",
			Width:         2,
			Height:        2,
			WidthPresent:  true,
			HeightPresent: true,
		},
		{Token: "filecn_image_two", MediaType: "image/jpeg", Alt: "第二张"},
	}, sources)
}

func TestParseNoteVisualSourcesRejectsUnmanagedImages(t *testing.T) {
	for _, document := range []string{
		`<h1>总结</h1><img src="https://example.com/image.png"/>`,
		`<h1>总结</h1><img token="filecn_image_bad" type="image/svg+xml"/>`,
		`<h1>总结</h1><img token="filecn_image_bad" width="0"/>`,
	} {
		_, err := parseNoteVisualSources(document)
		require.Error(t, err, document)
	}
}

func TestNoteSectionDiagnosticsDistinguishKnownAndUnknownTopLevelSections(t *testing.T) {
	require.True(t, hasKnownTopLevelNoteSection(`<h1>总结</h1><p>正文</p>`))
	require.False(t, hasUnknownTopLevelNoteSection(`<h1>总结</h1><h2>子标题</h2>`))
	require.True(t, hasUnknownTopLevelNoteSection(`<h1>总结</h1><h1>供应商新增模块</h1>`))
	require.False(t, hasKnownTopLevelNoteSection(`<weird><unknown-block/></weird>`))
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
	require.False(t, validManagedImageDimensions(maxWhiteboardPreviewDimension+1, 1))
	require.False(t, validManagedImageDimensions(8192, 4097))
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
	for _, raw := range []string{
		"https://example.com/a",
		"https://example.com/guides/docx-format",
	} {
		_, ok := sanitizePublicMinutesURL(raw)
		require.True(t, ok, raw)
	}
	for _, raw := range []string{
		"http://example.com/a",
		"https://feishu.cn/minutes/abc",
		"https://open.feishu.cn/open-apis/drive/v1/files",
		"https://example.com/doc?note_id=abc",
		"https://example.com/resource/obcn_secret_123",
		"https://example.com/redirect?next=https%3A%2F%2Flocalhost%2Fadmin",
		"https://example.com/redirect?next=https%3A%2F%2Fexample.feishu.com%2Fpublic",
		"https://example.com/redirect?next=%2F%2Fexample.larksuite.cn%2Fpublic",
		"https://example.com/redirect?next=javascript%3Aalert%281%29",
		"https://localhost/a",
		"https://127.0.0.1/a",
		"https://10.0.0.7/a",
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
