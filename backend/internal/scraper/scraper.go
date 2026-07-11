package scraper

import (
	"fmt"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

// Scraper 网页抓取器（兜底方案）
type Scraper struct {
	collector *colly.Collector
}

// ScrapedPodcast 抓取的播客信息
type ScrapedPodcast struct {
	Title       string
	Author      string
	Description string
	CoverURL    string
	FeedURL     string
	WebsiteURL  string
}

// NewScraper 创建网页抓取器
func NewScraper() *Scraper {
	c := colly.NewCollector(
		// 限制并发请求
		colly.Async(true),
		// 限制并发数量
		colly.MaxDepth(2),
	)

	// 设置限速（每秒1个请求，避免被封）
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Delay:       1 * time.Second,
		RandomDelay: 500 * time.Millisecond,
	})

	// 设置User-Agent
	c.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	return &Scraper{
		collector: c,
	}
}

// ScrapeXiaoyuzhou 从小宇宙网页版抓取播客信息
// URL格式: https://web.xiaoyuzhoufm.com/podcast/xxxxx
func (s *Scraper) ScrapeXiaoyuzhou(podcastURL string) (*ScrapedPodcast, error) {
	c := colly.NewCollector()

	var podcast ScrapedPodcast

	// 抓取标题
	c.OnXML(`//h1[contains(@class, "title")]`, func(e *colly.XMLElement) {
		podcast.Title = strings.TrimSpace(e.Text)
	})

	// 抓取作者
	c.OnXML(`//a[contains(@class, "author")]`, func(e *colly.XMLElement) {
		podcast.Author = strings.TrimSpace(e.Text)
	})

	// 抓取描述
	c.OnXML(`//div[contains(@class, "description")]`, func(e *colly.XMLElement) {
		podcast.Description = strings.TrimSpace(e.Text)
	})

	// 抓取封面
	c.OnXML(`//img[contains(@class, "cover")]`, func(e *colly.XMLElement) {
		podcast.CoverURL = e.Attr("src")
	})

	err := c.Visit(podcastURL)
	if err != nil {
		return nil, fmt.Errorf("failed to scrape xiaoyuzhou: %w", err)
	}

	podcast.WebsiteURL = podcastURL

	return &podcast, nil
}

// ScrapeGenericWebsite 通用网站抓取（使用meta标签和结构化数据）
func (s *Scraper) ScrapeGenericWebsite(url string) (*ScrapedPodcast, error) {
	c := colly.NewCollector()

	var podcast ScrapedPodcast

	// 尝试从Open Graph meta标签抓取
	c.OnXML(`//meta[contains(@property, "og:")]`, func(e *colly.XMLElement) {
		property := e.Attr("property")
		content := e.Attr("content")

		switch property {
		case "og:title":
			podcast.Title = content
		case "og:description":
			podcast.Description = content
		case "og:image":
			podcast.CoverURL = content
		case "og:audio":
			podcast.FeedURL = content
		}
	})

	// 尝试从Twitter Card meta标签抓取
	c.OnXML(`//meta[contains(@name, "twitter:")]`, func(e *colly.XMLElement) {
		name := e.Attr("name")
		content := e.Attr("content")

		switch name {
		case "twitter:title":
			if podcast.Title == "" {
				podcast.Title = content
			}
		case "twitter:description":
			if podcast.Description == "" {
				podcast.Description = content
			}
		case "twitter:image":
			if podcast.CoverURL == "" {
				podcast.CoverURL = content
			}
		}
	})

	// 尝试抓取RSS feed链接
	c.OnXML(`//link[@rel="alternate" and @type="application/rss+xml"]`, func(e *colly.XMLElement) {
		podcast.FeedURL = e.Attr("href")
	})

	// 抓取页面标题作为最后手段
	c.OnXML(`//title`, func(e *colly.XMLElement) {
		if podcast.Title == "" {
			podcast.Title = strings.TrimSpace(e.Text)
		}
	})

	err := c.Visit(url)
	if err != nil {
		return nil, fmt.Errorf("failed to scrape website: %w", err)
	}

	podcast.WebsiteURL = url

	return &podcast, nil
}

// ValidateURL 验证URL是否可访问
func (s *Scraper) ValidateURL(url string) error {
	c := colly.NewCollector()
	c.OnHTML("html", func(e *colly.HTMLElement) {
		// 页面可访问
	})

	return c.Visit(url)
}

// SetProxy 设置代理（如果需要）
func (s *Scraper) SetProxy(proxyURL string) error {
	return s.collector.SetProxy(proxyURL)
}

// SetDebugMode 设置调试模式
func (s *Scraper) SetDebugMode(enabled bool) {
	// Colly的调试模式设置较为复杂，暂时留空
	// 如需调试，可以使用Colly的Debug回调函数
	_ = enabled
}
