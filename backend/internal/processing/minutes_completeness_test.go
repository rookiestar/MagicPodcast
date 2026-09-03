package processing

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvaluateReadableNoteDocumentAllowsEmptyKnownSections(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><p></p><h1>关键决策</h1><p>暂无</p><h1>金句时刻</h1><p>无</p><h1>相关链接</h1><p></p>`,
		"",
		false,
		false,
	)
	require.True(t, decision.Complete)
	require.False(t, decision.Wait)
	require.Empty(t, decision.Code)
}

func TestEvaluateReadableNoteDocumentIgnoresFilteredInternalLinks(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><ul><li><a href="https://bytedance.larkoffice.com/minutes/obcn_internal">妙记</a></li><li><a href="https://bytedance.larkoffice.com/docx/internal">文字记录</a></li></ul>`,
		"",
		false,
		false,
	)
	require.True(t, decision.Complete)
	require.False(t, decision.Wait)
	require.Empty(t, decision.Code)
}

func TestEvaluateReadableNoteDocumentAllowsFilteredLinksWithPlaceholders(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><ul><li><a href="https://bytedance.larkoffice.com/minutes/obcn_internal">妙记</a></li><li>暂无</li></ul>`,
		"",
		false,
		false,
	)
	require.True(t, decision.Complete)
	require.Empty(t, decision.Code)
}

func TestEvaluateReadableNoteDocumentRejectsResidualRelatedLinkContent(t *testing.T) {
	for _, test := range []struct {
		name     string
		document string
	}{
		{name: "plain URL residual", document: `<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><ul><li><a href="https://bytedance.larkoffice.com/minutes/obcn_internal">妙记</a></li><li>https://example.com/unparsed</li></ul>`},
		{name: "unknown link element", document: `<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><ul><li><a href="https://bytedance.larkoffice.com/minutes/obcn_internal">妙记</a><provider-link href="https://example.com/provider">外部</provider-link></li></ul>`},
		{name: "unknown nested link element", document: `<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><ul><li><a href="https://bytedance.larkoffice.com/docx/internal"><provider-link href="https://example.com/provider">外部</provider-link></a></li></ul>`},
		{name: "unsupported URL-bearing element", document: `<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><ul><li><a href="https://bytedance.larkoffice.com/docx/internal">内部</a></li><li><provider-link url="https://example.com/provider"/></li><li><img src="https://example.com/image"/></li></ul>`},
		{name: "public URL label on filtered link", document: `<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><ul><li><a href="https://bytedance.larkoffice.com/docx/internal">https://example.com/provider</a></li></ul>`},
		{name: "text residual in sibling container", document: `<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><ul><li><a href="https://bytedance.larkoffice.com/minutes/obcn_internal">妙记</a></li><li>documentation pending</li></ul>`},
		{name: "text residual beside public link", document: `<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><ul><li><a href="https://example.com/guide">指南</a></li><li>documentation pending</li></ul>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			decision := evaluateReadableNoteDocument(test.document, "", false, false)
			require.False(t, decision.Complete)
			require.Equal(t, minutesEnrichmentSectionCode, decision.Code)
			require.Equal(t, "note_section_unparsed:相关链接", decision.Diagnostic)
		})
	}
}

func TestEvaluateReadableNoteDocumentValidatesRelatedLinkAliases(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><ul><li><a href="https://bytedance.larkoffice.com/minutes/obcn_internal">妙记</a></li></ul><h1>相关外链</h1><p>https://example.com/unparsed</p>`,
		"",
		false,
		false,
	)
	require.False(t, decision.Complete)
	require.Equal(t, minutesEnrichmentSectionCode, decision.Code)
	require.Equal(t, "note_section_unparsed:相关外链", decision.Diagnostic)
}

func TestEvaluateReadableNoteDocumentRejectsDuplicateRelatedLinkSections(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><p><a href="https://bytedance.larkoffice.com/docx/internal">内部</a></p><h1>相关链接</h1><p><a href="https://example.com/guide">指南</a></p>`,
		"",
		false,
		false,
	)
	require.False(t, decision.Complete)
	require.Equal(t, minutesEnrichmentSectionCode, decision.Code)
	require.Equal(t, "note_section_unparsed:相关链接", decision.Diagnostic)
}

func TestEvaluateReadableNoteDocumentAllowsPublicURLLabel(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><p><a href="https://example.com/guide">https://example.com/guide</a></p>`,
		"",
		false,
		false,
	)
	require.True(t, decision.Complete)
	require.Empty(t, decision.Code)
}

func TestEvaluateReadableNoteDocumentAllowsSelfClosingBookmarkLinks(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><p><bookmark name="指南" href="https://example.com/guide"/><bookmark name="参考" href="https://example.com/reference"/></p>`,
		"",
		false,
		false,
	)
	require.True(t, decision.Complete)
	require.Empty(t, decision.Code)
}

func TestEvaluateReadableNoteDocumentAllowsSelfClosingPublicLink(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><p><a href="https://example.com/guide"/></p>`,
		"",
		false,
		false,
	)
	require.True(t, decision.Complete)
	require.Empty(t, decision.Code)
}

func TestEvaluateReadableNoteDocumentIgnoresCommentedRelatedLinkHeading(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><p><a href="https://example.com/guide">指南</a></p><!-- <h1>相关链接</h1><p>旧区块</p> -->`,
		"",
		false,
		false,
	)
	require.True(t, decision.Complete)
	require.Empty(t, decision.Code)
}

func TestEvaluateReadableNoteDocumentAllowsFormattingAroundPublicLink(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><p>总结正文</p><h1>相关链接</h1><p><u><a href="https://example.com/guide">指南</a></u></p>`,
		"",
		false,
		false,
	)
	require.True(t, decision.Complete)
	require.Empty(t, decision.Code)
}

func TestEvaluateReadableNoteDocumentAllowsEmptyDocument(t *testing.T) {
	decision := evaluateReadableNoteDocument("   ", "", false, false)
	require.True(t, decision.Complete)
	require.False(t, decision.Wait)
}

func TestEvaluateReadableNoteDocumentRejectsUnknownTemplate(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>未知模板</h1><p>不能当作成功</p>`,
		"",
		false,
		false,
	)
	require.False(t, decision.Complete)
	require.Equal(t, minutesEnrichmentTemplateCode, decision.Code)
	require.Equal(t, "note_sections_unrecognized", decision.Diagnostic)
}

func TestEvaluateReadableNoteDocumentRejectsUnknownEmptyShell(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<weird><unknown-block token="abc"/></weird>`,
		"",
		false,
		false,
	)
	require.False(t, decision.Complete)
	require.Equal(t, minutesEnrichmentTemplateCode, decision.Code)
}

func TestEvaluateReadableNoteDocumentWaitsForTemporaryWhiteboard(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><whiteboard token="wbcn_board_123"/><h1>关键决策</h1><ul><li>继续推进</li></ul>`,
		"wbcn_board_123",
		false,
		true,
	)
	require.True(t, decision.Wait)
	require.False(t, decision.Complete)
	require.Equal(t, "whiteboard_pending", decision.Diagnostic)
}

func TestEvaluateReadableNoteDocumentFailsPermanentWhiteboard(t *testing.T) {
	decision := evaluateReadableNoteDocument(
		`<h1>总结</h1><whiteboard token="wbcn_board_123"/><h1>关键决策</h1><ul><li>继续推进</li></ul>`,
		"wbcn_board_123",
		false,
		false,
	)
	require.False(t, decision.Complete)
	require.False(t, decision.Wait)
	require.Equal(t, minutesEnrichmentWhiteboardCode, decision.Code)
}

func TestMinutesEnrichmentDeadlineIsFixedFromCoreReady(t *testing.T) {
	coreReady := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	deadline := minutesEnrichmentDeadline(coreReady, coreReady.Add(2*time.Minute))
	require.Equal(t, coreReady.Add(30*time.Minute), deadline)
	require.False(t, enrichmentWaitExpired(deadline, coreReady.Add(29*time.Minute)))
	require.True(t, enrichmentWaitExpired(deadline, coreReady.Add(30*time.Minute)))
}

func TestMinutesErrorIsWaitableForPendingAndUnknownReads(t *testing.T) {
	require.True(t, minutesErrorIsWaitable(errLarkMinutesPending))
	require.True(t, minutesErrorIsWaitable(errors.New("network down")))
	require.True(t, minutesErrorIsWaitable(NewUnknownExternalResultError(
		"lark_result_unknown",
		"Feishu CLI result is unknown",
	)))
	require.True(t, minutesErrorIsWaitable(NewAdapterError(
		"lark_rate_limited",
		"Feishu request rate is limited",
		true,
	)))
	require.False(t, minutesErrorIsWaitable(NewAdapterError(
		"lark_permission_denied",
		"Feishu user permission is insufficient",
		false,
	)))
}
