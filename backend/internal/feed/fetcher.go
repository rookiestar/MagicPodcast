package feed

import (
	"log"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"
)

// Fetcher RSS Feed抓取器
type Fetcher struct {
	parser     *gofeed.Parser
	httpClient *http.Client
	timeout    time.Duration
}

// FetchResult 抓取结果
type FetchResult struct {
	Feed        *gofeed.Feed
	NewItems    []*gofeed.Item
	Error       error
	IsIncremental bool // 是否为增量抓取
}

// NewFetcher 创建RSS Feed抓取器
func NewFetcher(timeout time.Duration) *Fetcher {
	return &Fetcher{
		parser:  gofeed.NewParser(),
		timeout: timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// FetchFeed 抓取RSS Feed（完整）
func (f *Fetcher) FetchFeed(feedURL string) (*gofeed.Feed, error) {
	startTime := time.Now()
	log.Printf("  📡 HTTP GET: %s", feedURL)

	// 使用gofeed的ParseURL，它会自动处理HTTP请求
	feed, err := f.parser.ParseURL(feedURL)
	if err != nil {
		duration := time.Since(startTime)
		log.Printf("  ❌ HTTP请求失败: %s (耗时: %v): %v", feedURL, duration, err)
		return nil, WrapHTTPError(feedURL, err)
	}

	duration := time.Since(startTime)
	log.Printf("  ✅ HTTP请求成功: %s (耗时: %v, 标题: %s, 单集数: %d)",
		feedURL, duration, feed.Title, len(feed.Items))

	return feed, nil
}

// FetchFeedWithClient 使用自定义HTTP客户端抓取
func (f *Fetcher) FetchFeedWithClient(feedURL string, client *http.Client) (*gofeed.Feed, error) {
	// gofeed库不支持自定义HTTP客户端，所以这里我们只使用默认的FetchFeed
	// 如果需要自定义HTTP客户端（如代理、超时等），需要在更高层处理
	return f.FetchFeed(feedURL)
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
		Feed:        feed,
		NewItems:    newItems,
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
	f.parser.UserAgent = userAgent
}
