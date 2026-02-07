package notifier

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"magicpodcast/internal/logger"

	"magicpodcast/internal/config"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
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
		logger.Info("📧 邮件通知未启用，跳过发送")
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
	logger.Infof("📧 正在发送邮件 [SMTP: %s:%d, From: %s, To: %s]",
		n.config.SMTPHost, n.config.SMTPPort, n.config.From, n.config.To)

	if err := n.dialer.DialAndSend(m); err != nil {
		logger.Infof("❌ SMTP发送失败: %v", err)
		return fmt.Errorf("发送邮件失败: %w", err)
	}

	logger.Infof("✅ 报告邮件已发送 [To: %s, Title: %s]", n.config.To, title)
	logger.Infof("💡 提示：如果没有收到邮件，请检查：")
	logger.Infof("   1. 垃圾邮件/推广邮件文件夹")
	logger.Infof("   2. 163邮箱的SMTP授权码是否有效")
	logger.Infof("   3. 163邮箱是否开启了SMTP服务")
	return nil
}

// buildEmailBody 构建邮件正文（使用goldmark将Markdown转换为HTML）
func (n *EmailNotifier) buildEmailBody(markdownContent string) string {
	// 创建goldmark实例，启用GFM扩展（GitHub Flavored Markdown）
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)

	var buf bytes.Buffer
	context := parser.NewContext()
	if err := md.Convert([]byte(markdownContent), &buf, parser.WithContext(context)); err != nil {
		logger.Infof("❌ Markdown转换失败: %v", err)
		// 降级到纯文本
		return fmt.Sprintf("<pre>%s</pre><div class='footer'><p>本邮件由 MagicPodcast 工作流自动生成，请勿回复。</p></div>", template.HTMLEscapeString(markdownContent))
	}

	htmlContent := buf.String()

	// 构建完整的HTML邮件模板
	htmlTemplate := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Arial, sans-serif; font-size: 14px; line-height: 1.5; color: #333; max-width: 800px; margin: 0 auto; padding: 20px; }
        h1, h2, h3 { color: #2c3e50; margin-top: 20px; margin-bottom: 12px; line-height: 1.3; }
        h1 { font-size: 1.6em; }
        h2 { border-bottom: 2px solid #eee; padding-bottom: 8px; font-size: 1.4em; }
        h3 { color: #34495e; font-size: 1.2em; }
        p { margin: 8px 0; }
        ul, ol { padding-left: 20px; margin: 8px 0; }
        ul ul, ol ol, ul ol, ol ul { margin: 4px 0; }
        li { margin: 3px 0; }
        code { background: #f4f4f4; padding: 2px 5px; border-radius: 3px; font-family: 'Monaco', 'Courier New', monospace; font-size: 0.9em; }
        pre { background: #f4f4f4; padding: 12px; border-radius: 5px; overflow-x: auto; border: 1px solid #ddd; font-size: 0.85em; line-height: 1.4; }
        pre code { background: none; padding: 0; }
        blockquote { border-left: 4px solid #ddd; margin: 10px 0; padding-left: 15px; color: #666; }
        hr { border: none; border-top: 1px solid #eee; margin: 15px 0; }
        img { max-width: 100%%; height: auto; border-radius: 4px; }
        a { color: #3498db; text-decoration: none; }
        a:hover { text-decoration: underline; }
        strong { font-weight: 600; }
        .footer { margin-top: 30px; padding-top: 15px; border-top: 2px solid #eee; color: #999; font-size: 12px; }
        .emoji { font-size: 1.1em; }
    </style>
</head>
<body>
    %s
    <div class="footer">
        <p>📬 本邮件由 <strong>MagicPodcast</strong> 工作流自动生成，请勿回复。</p>
    </div>
</body>
</html>`

	return fmt.Sprintf(htmlTemplate, htmlContent)
}
