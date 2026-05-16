package feed

import (
	"context"
	"fmt"
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
	startTime := time.Now()
	logger.Infof("  📡 HTTP GET: %s", feedURL)

	// 检查context是否已取消
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// 创建带超时的子context，确保goroutine不会无限期运行
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, WrapHTTPError(feedURL, err)
	}
	req.Header.Set("User-Agent", f.userAgent())

	resp, err := f.httpClient.Do(req)
	if err != nil {
		duration := time.Since(startTime)
		if ctx.Err() != nil {
			logger.Infof("  ⏱️ HTTP请求超时/取消: %s (耗时: %v): %v", feedURL, duration, ctx.Err())
			return nil, ctx.Err()
		}
		logger.Infof("  ❌ HTTP请求失败: %s (耗时: %v): %v", feedURL, duration, err)
		return nil, WrapHTTPError(feedURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		err := fmt.Errorf("HTTP status code %d", resp.StatusCode)
		duration := time.Since(startTime)
		logger.Infof("  ❌ HTTP请求失败: %s (耗时: %v): %v", feedURL, duration, err)
		return nil, WrapHTTPError(feedURL, err)
	}

	feed, err := f.newParser().Parse(resp.Body)
	if err != nil {
		duration := time.Since(startTime)
		logger.Infof("  ❌ Feed解析失败: %s (耗时: %v): %v", feedURL, duration, err)
		return nil, WrapHTTPError(feedURL, err)
	}

	duration := time.Since(startTime)
	logger.Infof("  ✅ HTTP请求成功: %s (耗时: %v, 标题: %s, 单集数: %d)",
		feedURL, duration, feed.Title, len(feed.Items))
	return feed, nil
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
	feed, err := f.FetchFeed(feedURL)
	if err != nil {
		return &FetchResult{Error: err}, err
	}

	var newItems []*gofeed.Item

	// 遍历所有item，只保留发布时间在lastFetchTime之后的
	for _, item := range feed.Items {
		if item.PublishedParsed != nil {
			if item.PublishedParsed.After(lastFetchTime) {
				newItems = append(newItems, item)
			}
		}
	}

	return &FetchResult{
		Feed:          feed,
		NewItems:      newItems,
		IsIncremental: true,
	}, nil
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
