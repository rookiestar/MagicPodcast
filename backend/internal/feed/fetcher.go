package feed

import (
	"context"
	"errors"
	"io"
	"magicpodcast/internal/logger"
	"net/http"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
)

// Fetcher RSS Feed抓取器
type Fetcher struct {
	parser     *gofeed.Parser
	parserMu   sync.RWMutex
	httpClient *http.Client
	timeout    time.Duration
}

// FetchResult 抓取结果
type FetchResult struct {
	Feed          *gofeed.Feed
	NewItems      []*gofeed.Item
	Error         error
	IsIncremental bool // 是否为增量抓取
	Access        AccessOutcome
}

// NewFetcher 创建RSS Feed抓取器
func NewFetcher(timeout time.Duration) *Fetcher {
	return &Fetcher{
		parser:  gofeed.NewParser(),
		timeout: timeout,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableKeepAlives:   false,
				// 连接池配置
				MaxConnsPerHost: 20, // 限制每个主机的最大连接数
			},
		},
	}
}

func (f *Fetcher) newParser() *gofeed.Parser {
	parser := gofeed.NewParser()

	f.parserMu.RLock()
	parser.UserAgent = f.parser.UserAgent
	f.parserMu.RUnlock()

	return parser
}

func (f *Fetcher) userAgent() string {
	f.parserMu.RLock()
	defer f.parserMu.RUnlock()

	if f.parser.UserAgent != "" {
		return f.parser.UserAgent
	}

	return "MagicPodcast/1.0"
}

// FetchFeed 抓取RSS Feed（完整）
func (f *Fetcher) FetchFeed(feedURL string) (*gofeed.Feed, error) {
	return f.FetchFeedWithContext(context.Background(), feedURL)
}

// FetchFeedWithContext 抓取RSS Feed（支持context和超时控制）
func (f *Fetcher) FetchFeedWithContext(ctx context.Context, feedURL string) (*gofeed.Feed, error) {
	result, err := f.FetchFeedWithContextDetailed(ctx, feedURL)
	if result == nil {
		return nil, err
	}
	return result.Feed, err
}

// FetchFeedWithContextDetailed returns the parsed Feed together with a stable,
// bounded access outcome for workflow observability.
func (f *Fetcher) FetchFeedWithContextDetailed(ctx context.Context, feedURL string) (result *FetchResult, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	startTime := time.Now()
	result = &FetchResult{Access: newPrimaryAccessOutcome(feedURL)}
	setDuration := func() {
		result.Access.ResponseTimeMs = int(time.Since(startTime).Milliseconds())
		result.Error = err
	}
	defer setDuration()

	safeURL := SanitizeFeedURL(feedURL)
	logger.Infof("  📡 HTTP GET: %s", safeURL)
	if ctx.Err() != nil {
		result.Access.ErrorCategory = ErrorCategoryCancelled
		return result, ctx.Err()
	}

	requestCtx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	req, requestErr := http.NewRequestWithContext(requestCtx, http.MethodGet, feedURL, nil)
	if requestErr != nil {
		err = WrapHTTPError(feedURL, requestErr)
		result.Access.ErrorCategory = ErrorCategoryInvalidRequest
		logger.Infof("  ❌ HTTP请求创建失败: %s (耗时: %v): %v", safeURL, time.Since(startTime), err)
		return result, err
	}
	req.Header.Set("User-Agent", f.userAgent())

	resp, requestErr := f.httpClient.Do(req)
	if requestErr != nil {
		if requestCtx.Err() != nil {
			err = requestCtx.Err()
			if errorsIsDeadline(err) {
				result.Access.ErrorCategory = ErrorCategoryTimeout
			} else {
				result.Access.ErrorCategory = ErrorCategoryCancelled
			}
			logger.Infof("  ⏱️ HTTP请求超时/取消: %s (耗时: %v): %v", safeURL, time.Since(startTime), err)
			return result, err
		}
		err = WrapHTTPError(feedURL, requestErr)
		result.Access.ErrorCategory = ErrorCategoryNetwork
		logger.Infof("  ❌ HTTP请求失败: %s (耗时: %v): %v", safeURL, time.Since(startTime), err)
		return result, err
	}
	defer resp.Body.Close()

	status := resp.StatusCode
	result.Access.HTTPStatus = &status
	result.Access.RetryAfter = resp.Header.Get("Retry-After")
	result.Access.ETag = resp.Header.Get("ETag")
	result.Access.LastModified = resp.Header.Get("Last-Modified")
	result.Access.CacheControl = resp.Header.Get("Cache-Control")
	result.Access.Expires = resp.Header.Get("Expires")
	result.Access.Age = resp.Header.Get("Age")
	if resp.ContentLength > 0 {
		result.Access.ResponseBytes = resp.ContentLength
	}

	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		err = WrapHTTPError(feedURL, newHTTPStatusError(status))
		result.Access.ErrorCategory = errorCategoryForStatus(status)
		result.Access.Freshness = FreshnessUnknown
		logger.Infof("  ❌ HTTP请求失败: %s (耗时: %v): %v", safeURL, time.Since(startTime), err)
		return result, err
	}

	var counter byteCounter
	parsedFeed, parseErr := f.newParser().Parse(io.TeeReader(resp.Body, &counter))
	result.Access.ResponseBytes = counter.BytesRead
	if parseErr != nil {
		err = WrapFeedParseError(feedURL, parseErr)
		result.Access.ErrorCategory = ErrorCategoryParse
		result.Access.Freshness = FreshnessUnknown
		logger.Infof("  ❌ Feed解析失败: %s (耗时: %v): %v", safeURL, time.Since(startTime), err)
		return result, err
	}

	result.Feed = parsedFeed
	result.Access.ErrorCategory = ErrorCategoryNone
	result.Access.Freshness = FreshnessLive
	result.Access.EgressID = EgressDirect
	logger.Infof("  ✅ HTTP请求成功: %s (耗时: %v, 标题: %s, 单集数: %d)",
		safeURL, time.Since(startTime), parsedFeed.Title, len(parsedFeed.Items))
	return result, nil
}

// FetchFeedWithClient 使用自定义HTTP客户端抓取
func (f *Fetcher) FetchFeedWithClient(feedURL string, client *http.Client) (*gofeed.Feed, error) {
	// gofeed库不支持自定义HTTP客户端，所以这里我们只使用默认的FetchFeed
	// 如果需要自定义HTTP客户端（如代理、超时等），需要在更高层处理
	return f.FetchFeed(feedURL)
}

// CloseIdleConnections 关闭HTTP连接池中的空闲连接
func (f *Fetcher) CloseIdleConnections() {
	if f.httpClient != nil {
		f.httpClient.CloseIdleConnections()
	}
}

// FetchIncremental 增量抓取：只获取lastFetchTime之后的新单集
func (f *Fetcher) FetchIncremental(feedURL string, lastFetchTime time.Time) (*FetchResult, error) {
	return f.FetchIncrementalWithContext(context.Background(), feedURL, lastFetchTime)
}

// FetchIncrementalWithContext is the context-aware incremental variant used by
// workflow executions so access metadata survives both success and failure.
func (f *Fetcher) FetchIncrementalWithContext(ctx context.Context, feedURL string, lastFetchTime time.Time) (*FetchResult, error) {
	result, err := f.FetchFeedWithContextDetailed(ctx, feedURL)
	if result == nil {
		return nil, err
	}
	result.IsIncremental = true
	if err != nil {
		return result, err
	}

	var newItems []*gofeed.Item

	// 遍历所有item，只保留发布时间在lastFetchTime之后的
	for _, item := range result.Feed.Items {
		if item.PublishedParsed != nil {
			if item.PublishedParsed.After(lastFetchTime) {
				newItems = append(newItems, item)
			}
		}
	}

	result.NewItems = newItems
	return result, nil
}

// ValidateFeed 验证RSS Feed是否有效
func (f *Fetcher) ValidateFeed(feedURL string) error {
	_, err := f.FetchFeed(feedURL)
	return err
}

// SetUserAgent 设置User-Agent（某些feed可能需要）
func (f *Fetcher) SetUserAgent(userAgent string) {
	f.parserMu.Lock()
	defer f.parserMu.Unlock()

	f.parser.UserAgent = userAgent
}

type byteCounter struct {
	BytesRead int64
}

func (c *byteCounter) Write(p []byte) (int, error) {
	c.BytesRead += int64(len(p))
	return len(p), nil
}

func errorsIsDeadline(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}
