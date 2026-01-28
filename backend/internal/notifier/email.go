package notifier

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"strings"

	"magicpodcast/internal/config"

	"gopkg.in/gomail.v2"
)

// EmailNotifier 邮件通知器
type EmailNotifier struct {
	config *config.EmailConfig
	dialer *gomail.Dialer
}

// NewEmailNotifier 创建邮件通知器
func NewEmailNotifier(cfg *config.EmailConfig) *EmailNotifier {
	dialer := gomail.NewDialer(
		cfg.SMTPHost,
		cfg.SMTPPort,
		cfg.Username,
		cfg.Password,
	)

	// 如果使用SSL/TLS
	if cfg.UseTLS {
		dialer.TLSConfig = &tls.Config{
			InsecureSkipVerify: false, // 生产环境应验证证书
			ServerName:         cfg.SMTPHost,
		}
	}

	return &EmailNotifier{
		config: cfg,
		dialer: dialer,
	}
}

// SendReport 发送报告邮件
func (n *EmailNotifier) SendReport(title, content string) error {
	if !n.config.Enabled {
		log.Println("📧 邮件通知未启用，跳过发送")
		return nil
	}

	// 构建邮件内容
	subject := title
	body := n.buildEmailBody(content)

	// 创建邮件消息
	m := gomail.NewMessage()
	m.SetHeader("From", n.config.From)
	m.SetHeader("To", n.config.To)
	m.SetHeader("Subject", subject)

	// 设置邮件正文（HTML格式，支持Markdown基本渲染）
	m.SetBody("text/html", body)

	// 发送邮件
	log.Printf("📧 正在发送邮件 [SMTP: %s:%d, From: %s, To: %s]",
		n.config.SMTPHost, n.config.SMTPPort, n.config.From, n.config.To)

	if err := n.dialer.DialAndSend(m); err != nil {
		log.Printf("❌ SMTP发送失败: %v", err)
		return fmt.Errorf("发送邮件失败: %w", err)
	}

	log.Printf("✅ 报告邮件已发送 [To: %s, Title: %s]", n.config.To, title)
	log.Printf("💡 提示：如果没有收到邮件，请检查：")
	log.Printf("   1. 垃圾邮件/推广邮件文件夹")
	log.Printf("   2. 163邮箱的SMTP授权码是否有效")
	log.Printf("   3. 163邮箱是否开启了SMTP服务")
	return nil
}

// buildEmailBody 构建邮件正文（将Markdown转换为HTML）
func (n *EmailNotifier) buildEmailBody(markdownContent string) string {
	// 简单的Markdown到HTML转换
	htmlTemplate := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 800px; margin: 0 auto; padding: 20px; }
        h1, h2, h3 { color: #2c3e50; margin-top: 30px; }
        h2 { border-bottom: 2px solid #eee; padding-bottom: 10px; }
        h3 { color: #34495e; }
        code { background: #f4f4f4; padding: 2px 6px; border-radius: 3px; font-family: 'Monaco', 'Courier New', monospace; font-size: 0.9em; }
        pre { background: #f4f4f4; padding: 15px; border-radius: 5px; overflow-x: auto; border: 1px solid #ddd; }
        pre code { background: none; padding: 0; }
        blockquote { border-left: 4px solid #ddd; margin: 0; padding-left: 20px; color: #666; }
        hr { border: none; border-top: 1px solid #eee; margin: 20px 0; }
        img { max-width: 100%; height: auto; border-radius: 4px; }
        a { color: #3498db; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .footer { margin-top: 40px; padding-top: 20px; border-top: 2px solid #eee; color: #999; font-size: 12px; }
        .emoji { font-size: 1.2em; }
    </style>
</head>
<body>
    {{.Content}}
    <div class="footer">
        <p>📬 本邮件由 <strong>MagicPodcast</strong> 工作流自动生成，请勿回复。</p>
    </div>
</body>
</html>`

	// 将Markdown转换为简单的HTML
	htmlContent := n.markdownToHTML(markdownContent)

	// 替换模板变量
	t, err := template.New("email").Parse(htmlTemplate)
	if err != nil {
		log.Printf("❌ 解析邮件模板失败: %v", err)
		return fmt.Sprintf("<pre>%s</pre><div class='footer'><p>本邮件由 MagicPodcast 工作流自动生成，请勿回复。</p></div>", template.HTMLEscapeString(markdownContent))
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, map[string]interface{}{
		"Content": template.HTML(htmlContent),
	}); err != nil {
		log.Printf("❌ 渲染邮件模板失败: %v", err)
		return fmt.Sprintf("<pre>%s</pre><div class='footer'><p>本邮件由 MagicPodcast 工作流自动生成，请勿回复。</p></div>", template.HTMLEscapeString(markdownContent))
	}

	return buf.String()
}

// markdownToHTML 简单的Markdown到HTML转换
func (n *EmailNotifier) markdownToHTML(markdown string) string {
	lines := strings.Split(markdown, "\n")
	var html strings.Builder
	inCodeBlock := false
	inList := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 处理代码块
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				inCodeBlock = false
				html.WriteString("</pre></code>\n")
			} else {
				inCodeBlock = true
				html.WriteString("<pre><code>")
			}
			continue
		}

		// 在代码块内直接输出
		if inCodeBlock {
			html.WriteString(template.HTMLEscapeString(line) + "\n")
			continue
		}

		// 处理空行
		if trimmed == "" {
			if inList {
				inList = false
				html.WriteString("</ul>\n")
			}
			html.WriteString("<br>\n")
			continue
		}

		// 处理标题
		if strings.HasPrefix(trimmed, "# ") {
			html.WriteString(fmt.Sprintf("<h1>%s</h1>\n", strings.TrimSpace(trimmed[2:])))
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			html.WriteString(fmt.Sprintf("<h2>%s</h2>\n", strings.TrimSpace(trimmed[3:])))
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			html.WriteString(fmt.Sprintf("<h3>%s</h3>\n", strings.TrimSpace(trimmed[4:])))
			continue
		}
		if strings.HasPrefix(trimmed, "#### ") {
			html.WriteString(fmt.Sprintf("<h4>%s</h4>\n", strings.TrimSpace(trimmed[5:])))
			continue
		}

		// 处理水平线
		if strings.HasPrefix(trimmed, "---") {
			html.WriteString("<hr>\n")
			continue
		}

		// 处理列表
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				inList = true
				html.WriteString("<ul>\n")
			}
			html.WriteString(fmt.Sprintf("<li>%s</li>\n", strings.TrimSpace(trimmed[2:])))
			continue
		}

		// 处理图片
		if strings.HasPrefix(trimmed, "![") {
			html.WriteString(n.processImage(trimmed) + "\n")
			continue
		}

		// 处理链接
		if strings.Contains(trimmed, "[") && strings.Contains(trimmed, "](") {
			html.WriteString(n.processLink(line) + "\n")
			continue
		}

		// 处理粗体
		line = strings.ReplaceAll(line, "**", "<strong>")
		line = strings.ReplaceAll(line, "__", "<strong>")

		// 处理斜体
		line = strings.ReplaceAll(line, "*", "<em>")
		line = strings.ReplaceAll(line, "_", "<em>")

		// 处理行内代码
		line = n.processInlineCode(line)

		// 普通段落
		if inList {
			inList = false
			html.WriteString("</ul>\n")
		}
		html.WriteString(fmt.Sprintf("<p>%s</p>\n", line))
	}

	// 关闭未闭合的标签
	if inList {
		html.WriteString("</ul>\n")
	}

	return html.String()
}

// processImage 处理Markdown图片语法
func (n *EmailNotifier) processImage(line string) string {
	// 格式: ![alt](url)
	idx1 := strings.Index(line, "![")
	idx2 := strings.Index(line, "](")
	idx3 := strings.Index(line, ")")

	if idx1 != -1 && idx2 != -1 && idx3 != -1 {
		alt := line[idx1+2 : idx2]
		url := line[idx2+2 : idx3]
		return fmt.Sprintf(`<img src="%s" alt="%s">`, url, alt)
	}
	return line
}

// processLink 处理Markdown链接语法
func (n *EmailNotifier) processLink(line string) string {
	// 格式: [text](url)
	result := line
	for {
		idx1 := strings.Index(result, "[")
		idx2 := strings.Index(result, "](")
		idx3 := strings.Index(result, ")")

		if idx1 == -1 || idx2 == -1 || idx3 == -1 || idx2 < idx1 || idx3 < idx2 {
			break
		}

		text := result[idx1+1 : idx2]
		url := result[idx2+2 : idx3]
		link := fmt.Sprintf(`<a href="%s">%s</a>`, url, text)

		result = result[:idx1] + link + result[idx3+1:]
	}
	return result
}

// processInlineCode 处理行内代码
func (n *EmailNotifier) processInlineCode(line string) string {
	// 简单处理：查找 `code` 格式
	result := line
	for {
		idx1 := strings.Index(result, "`")
		if idx1 == -1 {
			break
		}
		idx2 := strings.Index(result[idx1+1:], "`")
		if idx2 == -1 {
			break
		}
		idx2 += idx1 + 1

		code := result[idx1+1 : idx2]
		escaped := template.HTMLEscapeString(code)
		codeTag := fmt.Sprintf("<code>%s</code>", escaped)

		result = result[:idx1] + codeTag + result[idx2+1:]
	}
	return result
}
