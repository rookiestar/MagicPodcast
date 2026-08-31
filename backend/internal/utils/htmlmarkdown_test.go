package utils

import (
	"strings"
	"testing"
)

func TestHTMLToMarkdown_XiaoyuzhouShowNotes(t *testing.T) {
	// 用户反馈的真实小宇宙节目详情：纯 HTML，含 <p> 与带一堆属性的 <a>。
	in := `<p>📝 本期播客简介</p>` +
		`<p>本期我们克隆了：知名播客 Axios <a target="_blank" rel="noopener noreferrer nofollow" href="https://www.youtube.com/watch?v=fr1IQspixmM">Jensen Huang says the AI doomers have it wrong</a></p>` +
		`<p>原内容更新时间：2026-07-23</p>`

	out := HTMLToMarkdown(in)

	if strings.Contains(out, "<p>") || strings.Contains(out, "<a ") || strings.Contains(out, "target=") {
		t.Fatalf("残留 raw HTML 标签或属性，输出:\n%s", out)
	}
	wantLink := "[Jensen Huang says the AI doomers have it wrong](https://www.youtube.com/watch?v=fr1IQspixmM)"
	if !strings.Contains(out, wantLink) {
		t.Fatalf("未保留链接为 markdown，期望包含 %q，输出:\n%s", wantLink, out)
	}
	if !strings.Contains(out, "📝 本期播客简介") {
		t.Fatalf("丢失正文文本，输出:\n%s", out)
	}
	if !strings.Contains(out, "2026-07-23") {
		t.Fatalf("丢失正文文本，输出:\n%s", out)
	}
}

func TestHTMLToMarkdown_Basics(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plaintext", "  hello world  ", "hello world"},
		{"entity", "Tom &amp; Jerry", "Tom & Jerry"},
		{"strong", "<strong>x</strong>", "**x**"},
		{"em", "<em>y</em>", "*y*"},
		{"inline code", "<code>go test</code>", "`go test`"},
		{"heading", "<h2>标题</h2>", "## 标题"},
		{"linebreak", "a<br>b", "a\nb"},
		{"hr", "<hr>", "---"},
		{"telephone link", `<a href="tel:+8613800000000">电话</a>`, "[电话](tel:+8613800000000)"},
		{"unsafe link dropped", `<a href="javascript:alert(1)">x</a>`, "x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.TrimSpace(HTMLToMarkdown(c.in))
			if got != c.want {
				t.Fatalf("HTMLToMarkdown(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestHTMLToMarkdown_Lists(t *testing.T) {
	out := HTMLToMarkdown("<ul><li>苹果</li><li>香蕉</li></ul>")
	if !strings.Contains(out, "- 苹果") || !strings.Contains(out, "- 香蕉") {
		t.Fatalf("无序列表未正确渲染，输出:\n%s", out)
	}

	out = HTMLToMarkdown("<ol><li>第一步</li><li>第二步</li></ol>")
	if !strings.Contains(out, "1. 第一步") || !strings.Contains(out, "2. 第二步") {
		t.Fatalf("有序列表未正确渲染，输出:\n%s", out)
	}
}

func TestHTMLToMarkdown_Malformed(t *testing.T) {
	// 未闭合标签 + 裸文本：解析器容错，正文不应丢失。
	out := HTMLToMarkdown("<p>未闭合的段落，仍可读")
	if !strings.Contains(out, "未闭合的段落，仍可读") {
		t.Fatalf("容错失败，正文丢失，输出:%q", out)
	}
	if strings.Contains(out, "<p>") {
		t.Fatalf("未剥离残留标签，输出:%q", out)
	}
}

func TestHTMLToMarkdown_StripsScriptAndStyle(t *testing.T) {
	out := HTMLToMarkdown(`<script>alert(1)</script><style>.x{}</style>可见文本`)
	if strings.Contains(out, "alert") || strings.Contains(out, ".x{}") {
		t.Fatalf("script/style 未被剔除，输出:%q", out)
	}
	if !strings.Contains(out, "可见文本") {
		t.Fatalf("正常文本丢失，输出:%q", out)
	}
}

func TestHTMLToMarkdown_PreservesPreformattedMarkdownAsCode(t *testing.T) {
	in := "<p># 正文标题</p><pre><code># 示例标题\n**示例强调**</code></pre>"

	out := HTMLToMarkdown(in)

	if !strings.Contains(out, "```\n# 示例标题\n**示例强调**\n```") {
		t.Fatalf("预格式内容未作为单一代码块保留，输出:\n%s", out)
	}
	if strings.Count(out, "```") != 2 {
		t.Fatalf("预格式内容生成了嵌套代码围栏，输出:\n%s", out)
	}
}
