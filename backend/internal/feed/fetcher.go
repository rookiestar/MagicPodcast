package feed

import (
	"context"
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
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableKeepAlives:   false,
				// 连接池配置
				MaxConnsPerHost:     20, // 限制每个主机的最大连接数
			},
		},
	}
}

// FetchFeed 抓取RSS Feed（完整）
func (f *Fetcher) FetchFeed(feedURL string) (*gofeed.Feed, error) {
	return f.FetchFeedWithContext(context.Background(), feedURL)
}

// FetchFeedWithContext 抓取RSS Feed（支持context和超时控制）
func (f *Fetcher) FetchFeedWithContext(ctx context.Context, feedURL string) (*gofeed.Feed, error) {
	startTime := time.Now()
	log.Printf("  📡 HTTP GET: %s", feedURL)

	// 检查context是否已取消
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// 使用gofeed的ParseURL，它会自动处理HTTP请求
	// 注意：gofeed库本身不支持context，所以这里我们使用context作为超时控制的额外保障
	type result struct {
		feed *gofeed.Feed
		err  error
	}
	resultChan := make(chan result, 1)

	// 启动goroutine执行feed解析
	parseComplete := make(chan struct{})
	go func() {
		defer close(parseComplete) // 标记goroutine已完成
		feed, err := f.parser.ParseURL(feedURL)
		select {
		case resultChan <- result{feed, err}:
			// 正常发送结果
		case <-ctx.Done():
			// context已取消，不需要发送结果（避免阻塞）
			log.Printf("  ⚠️ Feed解析goroutine被取消: %s", feedURL)
		}
	}()

	// 等待完成或context取消
	select {
	case <-ctx.Done():
		duration := time.Since(startTime)
		log.Printf("  ⏱️ HTTP请求超时/取消: %s (耗时: %v): %v", feedURL, duration, ctx.Err())
		// 等待goroutine退出，但不阻塞太久
		select {
		case <-parseComplete:
			// goroutine已经退出
		case <-time.After(100 * time.Millisecond):
			// 如果100ms后还没退出，记录警告但不等待
			log.Printf("  ⚠️ ParseURL goroutine可能仍在运行: %s", feedURL)
		}
		return nil, ctx.Err()
	case res := <-resultChan:
		if res.err != nil {
			duration := time.Since(startTime)
			log.Printf("  ❌ HTTP请求失败: %s (耗时: %v): %v", feedURL, duration, res.err)
			return nil, WrapHTTPError(feedURL, res.err)
		}
		duration := time.Since(startTime)
		log.Printf("  ✅ HTTP请求成功: %s (耗时: %v, 标题: %s, 单集数: %d)",
			feedURL, duration, res.feed.Title, len(res.feed.Items))
		return res.feed, nil
	}
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
