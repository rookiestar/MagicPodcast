package utils

import (
	"strings"
	"testing"
)

func TestBuildShowNotesDocument_NormalizesMixedHTMLAndMarkdown(t *testing.T) {
	source := strings.Join([]string{
		"<p># 转型开场",
		"<br><br><br>**对齐事实**",
		"<br><br><br>---",
		"<br><br><br>- 建立共同语言",
		`<br><br><br>[延伸阅读](https://example.com/notes)`,
		`<br><br><br>![转型示意图](https://images.example.com/cover.png)</p>`,
	}, "")

	document := BuildShowNotesDocument(source)

	if document.Format != ShowNotesFormatMarkdown {
		t.Fatalf("format = %q, want %q", document.Format, ShowNotesFormatMarkdown)
	}
	for _, want := range []string{
		"# 转型开场",
		"**对齐事实**",
		"---",
		"- 建立共同语言",
		"[延伸阅读](https://example.com/notes)",
		"![转型示意图](https://images.example.com/cover.png)",
	} {
		if !strings.Contains(document.Content, want) {
			t.Errorf("content missing %q:\n%s", want, document.Content)
		}
	}
	if strings.Contains(document.Content, "<p>") {
		t.Fatalf("content retained HTML shell: %q", document.Content)
	}
	if strings.Contains(document.Content, "\n\n\n") {
		t.Fatalf("content retained three consecutive newlines: %q", document.Content)
	}
}

func TestBuildShowNotesDocument_ClassifiesConservatively(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		wantFormat  ShowNotesFormat
		wantContent string
	}{
		{
			name:        "empty",
			source:      " \n\t ",
			wantFormat:  ShowNotesFormatMarkdown,
			wantContent: "",
		},
		{
			name:        "pure HTML",
			source:      `<h2>结构标题</h2><p><strong>重点</strong></p><ul><li>项目</li></ul>`,
			wantFormat:  ShowNotesFormatHTML,
			wantContent: `<h2>结构标题</h2><p><strong>重点</strong></p><ul><li>项目</li></ul>`,
		},
		{
			name:        "pure Markdown",
			source:      "# 标题\n\n**重点**\n\n- 项目",
			wantFormat:  ShowNotesFormatMarkdown,
			wantContent: "# 标题\n\n**重点**\n\n- 项目",
		},
		{
			name:        "Markdown angle autolink is not HTML",
			source:      "来源：<https://example.com/article>",
			wantFormat:  ShowNotesFormatMarkdown,
			wantContent: "来源：<https://example.com/article>",
		},
		{
			name:        "plain text is a Markdown subset",
			source:      "普通文本可以直接阅读。",
			wantFormat:  ShowNotesFormatMarkdown,
			wantContent: "普通文本可以直接阅读。",
		},
		{
			name:        "code and pre contents do not trigger mixed detection",
			source:      `<p>示例：</p><pre># 不是标题\n**不是强调**</pre><p><code>- 不是列表</code></p>`,
			wantFormat:  ShowNotesFormatHTML,
			wantContent: `<p>示例：</p><pre># 不是标题\n**不是强调**</pre><p><code>- 不是列表</code></p>`,
		},
		{
			name:        "C sharp topic URL and lone star stay HTML",
			source:      `<p>C#、#话题、https://example.com/#anchor 和孤立 * 都是正文。</p>`,
			wantFormat:  ShowNotesFormatHTML,
			wantContent: `<p>C#、#话题、https://example.com/#anchor 和孤立 * 都是正文。</p>`,
		},
		{
			name:        "unpaired emphasis stays HTML",
			source:      `<p>这只是 *未闭合的强调。</p>`,
			wantFormat:  ShowNotesFormatHTML,
			wantContent: `<p>这只是 *未闭合的强调。</p>`,
		},
		{
			name:        "paired emphasis makes mixed content",
			source:      `<p>这是 *真正强调* 的内容。</p>`,
			wantFormat:  ShowNotesFormatMarkdown,
			wantContent: `这是 *真正强调* 的内容。`,
		},
		{
			name:        "malformed HTML remains readable",
			source:      `<p># 容错标题`,
			wantFormat:  ShowNotesFormatMarkdown,
			wantContent: `# 容错标题`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := BuildShowNotesDocument(test.source)
			if document.Format != test.wantFormat {
				t.Fatalf("format = %q, want %q", document.Format, test.wantFormat)
			}
			if document.Content != test.wantContent {
				t.Fatalf("content = %q, want %q", document.Content, test.wantContent)
			}
		})
	}
}

func TestBuildShowNotesDocument_RecognizesExplicitMarkdownStructuresOnLogicalLines(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"heading", `<p># 标题</p>`, "# 标题"},
		{"unordered list", `<div>- 列表项</div>`, "- 列表项"},
		{"ordered list", `<div>1. 第一步</div>`, "1. 第一步"},
		{"rule", `<p>前言<br>---</p>`, "---"},
		{"quote", `<p>&gt; 引用</p>`, "> 引用"},
		{"fence", "<p>```go<br>fmt.Println()</p>", "```go"},
		{"link", `<p>[来源](https://example.com/source)</p>`, "[来源](https://example.com/source)"},
		{"telephone link", `<p>[电话](tel:+8613800000000)</p>`, "[电话](tel:+8613800000000)"},
		{"image", `<p>![图](https://images.example.com/cover.png)</p>`, "![图](https://images.example.com/cover.png)"},
		{"strong stars", `<p>这是 **重点**。</p>`, "**重点**"},
		{"strong underscores", `<p>这是 __重点__。</p>`, "__重点__"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := BuildShowNotesDocument(test.source)
			if document.Format != ShowNotesFormatMarkdown {
				t.Fatalf("format = %q, want %q", document.Format, ShowNotesFormatMarkdown)
			}
			if !strings.Contains(document.Content, test.want) {
				t.Fatalf("content = %q, want it to contain %q", document.Content, test.want)
			}
		})
	}
}

func TestBuildShowNotesDocument_KeepsHTMLCodeExamplesLiteralInsideMixedContent(t *testing.T) {
	document := BuildShowNotesDocument(
		"<p># 正文标题</p><pre><code># 示例标题\n**示例强调**</code></pre>",
	)

	if document.Format != ShowNotesFormatMarkdown {
		t.Fatalf("format = %q, want %q", document.Format, ShowNotesFormatMarkdown)
	}
	if !strings.Contains(document.Content, "```\n# 示例标题\n**示例强调**\n```") {
		t.Fatalf("代码示例未保持字面量，输出:\n%s", document.Content)
	}
	if strings.Count(document.Content, "```") != 2 {
		t.Fatalf("代码示例生成了嵌套围栏，输出:\n%s", document.Content)
	}
}
