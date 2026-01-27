package llm

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"
)

// PromptManager Prompt模板管理器
type PromptManager struct {
	promptsDir string                 // 模板文件目录
	templates  map[string]*template.Template // 缓存的编译后模板
	mutex      sync.RWMutex           // 读写锁
}

// PromptFileInfo Prompt文件信息
type PromptFileInfo struct {
	Name        string    `json:"name"`         // 模板名称（文件名，不含.txt）
	Description string    `json:"description"`  // 模板描述（从文件首行注释提取）
	IsDefault   bool      `json:"is_default"`   // 是否为默认模板
	Content     string    `json:"content"`      // 模板内容
	ModifiedAt  time.Time `json:"modified_at"`  // 最后修改时间
}

// NewPromptManager 创建Prompt模板管理器
func NewPromptManager(promptsDir string) *PromptManager {
	pm := &PromptManager{
		promptsDir: promptsDir,
		templates:  make(map[string]*template.Template),
	}

	// 确保目录存在
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		fmt.Printf("⚠️  创建prompts目录失败: %v\n", err)
	}

	// 加载所有模板
	if err := pm.reloadTemplates(); err != nil {
		fmt.Printf("⚠️  加载prompt模板失败: %v\n", err)
	}

	return pm
}

// GetTemplate 获取编译后的模板
func (m *PromptManager) GetTemplate(name string) (*template.Template, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	tpl, ok := m.templates[name]
	if !ok {
		return nil, fmt.Errorf("模板不存在: %s", name)
	}

	return tpl, nil
}

// RenderTemplate 渲染模板
func (m *PromptManager) RenderTemplate(name string, data interface{}) (string, error) {
	tpl, err := m.GetTemplate(name)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("模板渲染失败: %w", err)
	}

	return buf.String(), nil
}

// ListTemplates 列出所有模板
func (m *PromptManager) ListTemplates() ([]PromptFileInfo, error) {
	entries, err := os.ReadDir(m.promptsDir)
	if err != nil {
		return nil, fmt.Errorf("读取prompts目录失败: %w", err)
	}

	var templates []PromptFileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// 只处理.txt文件
		if !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".txt")
		info, err := m.getTemplateInfo(name)
		if err != nil {
			continue // 跳过无法读取的文件
		}

		templates = append(templates, info)
	}

	return templates, nil
}

// getTemplateInfo 获取模板信息
func (m *PromptManager) getTemplateInfo(name string) (PromptFileInfo, error) {
	filePath := m.getFilePath(name)

	info, err := os.Stat(filePath)
	if err != nil {
		return PromptFileInfo{}, err
	}

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return PromptFileInfo{}, err
	}

	// 提取描述（首行注释）
	description := ""
	scanner := bufio.NewScanner(bytes.NewReader(content))
	if scanner.Scan() {
		firstLine := scanner.Text()
		if strings.HasPrefix(firstLine, "#") {
			description = strings.TrimSpace(strings.TrimPrefix(firstLine, "#"))
		}
	}

	return PromptFileInfo{
		Name:        name,
		Description: description,
		IsDefault:   name == "default_summary",
		Content:     string(content),
		ModifiedAt:  info.ModTime(),
	}, nil
}

// SaveTemplate 保存模板
func (m *PromptManager) SaveTemplate(name, content string) error {
	// 验证文件名
	if !isValidTemplateName(name) {
		return fmt.Errorf("无效的模板名称: %s (只允许字母、数字、下划线、短横线)", name)
	}

	filePath := m.getFilePath(name)

	// 写入文件
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("保存模板失败: %w", err)
	}

	// 重新加载模板
	if err := m.reloadTemplate(name); err != nil {
		return fmt.Errorf("重新加载模板失败: %w", err)
	}

	return nil
}

// DeleteTemplate 删除模板
func (m *PromptManager) DeleteTemplate(name string) error {
	// 不允许删除默认模板
	if name == "default_summary" {
		return fmt.Errorf("不能删除默认模板")
	}

	filePath := m.getFilePath(name)

	// 删除文件
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("删除模板失败: %w", err)
	}

	// 从缓存中移除
	m.mutex.Lock()
	delete(m.templates, name)
	m.mutex.Unlock()

	return nil
}

// ResetToDefault 重置为默认模板
func (m *PromptManager) ResetToDefault(name string) error {
	if name != "default_summary" {
		return fmt.Errorf("只有默认模板可以重置")
	}

	// 内置的默认模板内容
	defaultContent := `# 默认摘要模板 - 混合模式
你是一个专业的播客内容摘要助手。请为以下工作流执行结果生成一份简洁的中文摘要。

工作流名称: {{.WorkflowName}}
匹配的单集总数: {{.TotalEpisodes}}
节目数量: {{.NumPodcasts}}

{{range .Podcasts}}
### {{.PodcastTitle}}
单集数: {{len .Episodes}}
{{range .Episodes}}
- **{{.Title}}** ({{.PublishedDate.Format "2006-01-02"}})
  {{if ne .ShowNotes ""}}{{.ShowNotes}}{{end}}
{{end}}
{{end}}

请按以下格式生成摘要：

## 📊 总体概览
简要描述本次抓取的整体情况（1-2句话）

## 🎯 核心内容
按节目分类列出重要单集的要点

## 💡 关键洞察
提炼3-5个关键主题或趋势，指出值得关注的亮点

请用简洁专业的语言生成摘要，避免过度解读。
`

	return m.SaveTemplate(name, defaultContent)
}

// reloadTemplates 重新加载所有模板
func (m *PromptManager) reloadTemplates() error {
	entries, err := os.ReadDir(m.promptsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".txt")
		if err := m.reloadTemplate(name); err != nil {
			fmt.Printf("⚠️  加载模板 %s 失败: %v\n", name, err)
		}
	}

	return nil
}

// reloadTemplate 重新加载单个模板
func (m *PromptManager) reloadTemplate(name string) error {
	filePath := m.getFilePath(name)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// 编译模板
	tpl, err := template.New(name).Parse(string(content))
	if err != nil {
		return fmt.Errorf("模板编译失败: %w", err)
	}

	// 缓存模板
	m.mutex.Lock()
	m.templates[name] = tpl
	m.mutex.Unlock()

	return nil
}

// getFilePath 获取模板文件路径
func (m *PromptManager) getFilePath(name string) string {
	return filepath.Join(m.promptsDir, name+".txt")
}

// isValidTemplateName 验证模板名称是否有效
func isValidTemplateName(name string) bool {
	if name == "" {
		return false
	}

	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}

	return true
}
