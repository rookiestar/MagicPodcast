package llm

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"magicpodcast/internal/logger"
	"net/http"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/config"
)

// Client LLM客户端
type Client struct {
	config      *config.LLMConfig
	httpClient  *http.Client
	rateLimiter *rateLimiter

	// 统计信息
	statsMutex sync.Mutex
	stats      UsageStats
}

// UsageStats 使用统计
type UsageStats struct {
	TotalRequests  int64
	TotalTokens    int64
	DailyTokens    int64
	DailyRequests  int64
	LastResetDate  time.Time
	DailyCostCents float64
}

// ChatCompletionRequest OpenAI兼容的请求格式
type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// Message 消息格式
type Message struct {
	Role    string `json:"role"` // system, user, assistant
	Content string `json:"content"`
}

// ChatCompletionResponse OpenAI兼容的响应格式
type ChatCompletionResponse struct {
	ID      string    `json:"id"`
	Object  string    `json:"object"`
	Created int64     `json:"created"`
	Model   string    `json:"model"`
	Choices []Choice  `json:"choices"`
	Usage   Usage     `json:"usage"`
	Error   *APIError `json:"error,omitempty"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// rateLimiter 简单的速率限制器
type rateLimiter struct {
	mutex  sync.Mutex
	tokens map[string][]time.Time
	maxReq int
	window time.Duration
}

func newRateLimiter(maxRequestsPerMinute int) *rateLimiter {
	return &rateLimiter{
		tokens: make(map[string][]time.Time),
		maxReq: maxRequestsPerMinute,
		window: time.Minute,
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// 清理过期记录
	var requests []time.Time
	for _, t := range rl.tokens[key] {
		if t.After(cutoff) {
			requests = append(requests, t)
		}
	}

	if len(requests) >= rl.maxReq {
		return false
	}

	requests = append(requests, now)
	rl.tokens[key] = requests
	return true
}

// NewClient 创建LLM客户端
func NewClient(cfg *config.LLMConfig) *Client {
	logger.Debugf("[LLM Client] Initializing with config:")
	logger.Debugf("  - Enabled: %v", cfg.Enabled)
	logger.Debugf("  - Provider: %s", cfg.Provider)
	logger.Debugf("  - API Key: %s", maskAPIKey(cfg.APIKey))
	logger.Debugf("  - Base URL: %s", cfg.BaseURL)
	logger.Debugf("  - Default Model: %s", cfg.DefaultModel)

	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		rateLimiter: newRateLimiter(cfg.RateLimitPerMinute),
		stats: UsageStats{
			LastResetDate: time.Now(),
		},
	}
}

// maskAPIKey 隐藏API Key的敏感信息
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// GenerateSummary 生成摘要
func (c *Client) GenerateSummary(ctx context.Context, systemPrompt, userPrompt string, options SummaryOptions) (*SummaryResult, error) {
	logger.Debugf("[LLM GenerateSummary] Called with config:")
	logger.Debugf("  - Enabled: %v", c.config.Enabled)
	logger.Debugf("  - API Key: %s", maskAPIKey(c.config.APIKey))

	// 检查总开关
	if !c.config.Enabled {
		return nil, fmt.Errorf("LLM功能未启用")
	}

	// 检查API Key
	apiKey := c.config.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("LLM API Key未配置")
	}

	// 速率限制
	if !c.rateLimiter.allow("global") {
		return nil, fmt.Errorf("达到速率限制，请稍后重试")
	}

	// 构建请求
	model := options.Model
	if model == "" {
		model = c.config.DefaultModel
	}
	model = c.normalizeModel(model)

	// 构建消息列表（支持system和user消息）
	messages := []Message{}
	if systemPrompt != "" {
		messages = append(messages, Message{Role: "system", Content: systemPrompt})
	}
	if userPrompt != "" {
		messages = append(messages, Message{Role: "user", Content: userPrompt})
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("至少需要提供system或user prompt")
	}

	req := ChatCompletionRequest{
		Model:       model,
		Messages:    messages,
		Temperature: options.Temperature,
		MaxTokens:   options.MaxTokens,
		Stream:      false,
	}

	// 序列化请求
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", c.config.BaseURL)
	authHeader := fmt.Sprintf("Bearer %s", apiKey)
	if c.config.Provider == "zhipuai" {
		token, tokenErr := generateZhipuJWTToken(apiKey)
		if tokenErr != nil {
			return nil, fmt.Errorf("生成智谱AI token失败: %w", tokenErr)
		}
		authHeader = fmt.Sprintf("Bearer %s", token)
	}

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("HTTP请求失败: %w", err)
			}
			logger.Infof("LLM请求重试 %d/%d", attempt, c.config.MaxRetries)
			if c.config.RetryInterval > 0 {
				time.Sleep(time.Duration(c.config.RetryInterval) * time.Millisecond)
			}
		}

		result, retryable, err := c.doChatCompletion(ctx, url, authHeader, reqBody)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryable || attempt == c.config.MaxRetries {
			if retryable && attempt > 0 {
				return nil, fmt.Errorf("%v（重试%d次后）", err, c.config.MaxRetries)
			}
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) doChatCompletion(ctx context.Context, url, authHeader string, reqBody []byte) (*SummaryResult, bool, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, false, fmt.Errorf("创建HTTP请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", authHeader)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, true, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("读取响应失败: %w", err)
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, false, fmt.Errorf("解析响应失败: %w, body: %s", err, string(body))
	}
	if chatResp.Error != nil {
		return nil, false, fmt.Errorf("LLM API错误: %s (code: %s)", chatResp.Error.Message, chatResp.Error.Code)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("LLM API返回错误状态: %d, body: %s", resp.StatusCode, string(body))
	}
	if len(chatResp.Choices) == 0 {
		return nil, false, fmt.Errorf("LLM API未返回任何结果")
	}

	c.updateStats(chatResp.Usage.TotalTokens)
	return &SummaryResult{
		Summary:      formatSummary(chatResp.Choices[0].Message.Content),
		ModelUsed:    chatResp.Model,
		TokensUsed:   chatResp.Usage.TotalTokens,
		PromptTokens: chatResp.Usage.PromptTokens,
		TotalTokens:  chatResp.Usage.TotalTokens,
	}, false, nil
}

// ValidateAPIKey sends a tiny request to verify that the configured provider,
// API key, and model can complete a chat request.
func (c *Client) ValidateAPIKey(ctx context.Context, apiKey, model string) (*SummaryResult, error) {
	testConfig := *c.config
	testConfig.APIKey = apiKey
	if model != "" {
		testConfig.DefaultModel = model
	}

	testClient := NewClient(&testConfig)
	return testClient.GenerateSummary(
		ctx,
		"You are a connection test endpoint. Reply with OK only.",
		"OK",
		SummaryOptions{
			Model:       model,
			Temperature: 0,
			MaxTokens:   32,
		},
	)
}

func (c *Client) normalizeModel(model string) string {
	if c.config.Provider != config.LLMProviderDeepSeek {
		return model
	}

	if model == "" || strings.HasPrefix(model, "glm-") {
		return "deepseek-v4-flash"
	}

	return model
}

// SummaryOptions 摘要选项
type SummaryOptions struct {
	Model       string
	Temperature float64
	MaxTokens   int
	MaxEpisodes int // 单次摘要最大单集数
}

// SummaryResult 摘要结果
type SummaryResult struct {
	Summary      string
	ModelUsed    string
	TokensUsed   int
	PromptTokens int
	TotalTokens  int
}

// updateStats 更新使用统计
func (c *Client) updateStats(tokensUsed int) {
	c.statsMutex.Lock()
	defer c.statsMutex.Unlock()

	// 检查是否需要重置每日统计
	now := time.Now()
	if now.Day() != c.stats.LastResetDate.Day() {
		c.stats.DailyTokens = 0
		c.stats.DailyRequests = 0
		c.stats.DailyCostCents = 0
		c.stats.LastResetDate = now
	}

	c.stats.TotalRequests++
	c.stats.TotalTokens += int64(tokensUsed)
	c.stats.DailyRequests++
	c.stats.DailyTokens += int64(tokensUsed)

	// 简单成本计算（硅基流动Qwen2.5-7B约 $0.06/1M tokens）
	// 这里用0.006 cents/1K tokens作为估算
	c.stats.DailyCostCents += float64(tokensUsed) * 0.006
}

// GetStats 获取使用统计
func (c *Client) GetStats() UsageStats {
	c.statsMutex.Lock()
	defer c.statsMutex.Unlock()
	return c.stats
}

// GetSystemPrompt 获取全局System Prompt
func (c *Client) GetSystemPrompt() string {
	return c.config.SystemPrompt
}

// formatSummary 格式化摘要内容，去除列表项后的多余换行
func formatSummary(summary string) string {
	lines := strings.Split(summary, "\n")
	result := make([]string, 0, len(lines))
	inList := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 跳过空行
		if trimmed == "" {
			inList = false
			continue
		}

		// 检测列表项
		isListItem := false
		runes := []rune(trimmed)
		if len(runes) > 0 {
			first := runes[0]
			// Bullet point: -, *, •
			if first == '-' || first == '*' || first == '•' {
				isListItem = true
			}
			// 数字序号: 1. 2. 3. 等
			if first >= '0' && first <= '9' && len(runes) > 1 && runes[1] == '.' {
				isListItem = true
			}
		}

		// 如果是连续的列表项，紧凑排列
		if isListItem || inList {
			result = append(result, line)
			inList = isListItem
		} else {
			// 非列表项之间添加空行
			if len(result) > 0 {
				result = append(result, "")
			}
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// generateZhipuJWTToken 生成智谱AI的JWT token
// 智谱AI的API Key格式为: id.secret
// 需要将id作为payload，使用secret作为HMAC-SHA256密钥生成JWT
func generateZhipuJWTToken(apiKey string) (string, error) {
	// 打印调试信息（隐藏敏感信息）
	maskedKey := maskAPIKey(apiKey)
	logger.Infof("[ZhipuAI] 生成JWT token，API Key: %s", maskedKey)

	// 解析API Key
	parts := strings.Split(apiKey, ".")
	logger.Infof("[ZhipuAI] API Key分割后: %d 个部分", len(parts))

	if len(parts) != 2 {
		return "", fmt.Errorf("invalid API key format, expected id.secret, got %d parts", len(parts))
	}

	id := parts[0]
	secret := parts[1]
	logger.Infof("[ZhipuAI] ID: %s, Secret: %s", id[:min(8, len(id))]+"...", maskAPIKey(secret))

	// 创建JWT payload
	now := time.Now()
	exp := now.Add(1 * time.Hour) // token有效期1小时

	payload := map[string]interface{}{
		"api_key":   id,
		"exp":       exp.Unix(),
		"timestamp": now.Unix(),
	}

	// 序列化payload
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Base64 URL encode payload
	payloadBase64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	// 创建签名
	// 格式: header.payload
	// 智谱AI使用固定的header: {"alg": "HS256", "sign_type": "SIGN"}
	header := map[string]string{
		"alg":       "HS256",
		"sign_type": "SIGN",
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal header: %w", err)
	}
	headerBase64 := base64.RawURLEncoding.EncodeToString(headerJSON)

	// 计算签名: HMAC-SHA256(secret, header.payload)
	signingInput := headerBase64 + "." + payloadBase64
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	// 组合完整的JWT token
	token := headerBase64 + "." + payloadBase64 + "." + signature

	return token, nil
}

// min 返回两个整数中的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
