package feed

import (
	"context"
	"errors"
	"io"
	"magicpodcast/internal/logger"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
)

// Feed HTTP defaults keep the fetcher honest and low-load: a stable product
// User-Agent with a project contact URL, a standard RSS/XML Accept header,
// and layered connect / TLS-handshake / response-header timeouts so a slow
// server cannot consume the whole request budget by trickling bytes.
// Accept-Encoding is intentionally never set, so Go's http.Transport
// transparently negotiates and decompresses gzip responses.
const (
	defaultFeedUserAgent = "MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)"
	defaultFeedAccept    = "application/rss+xml, application/atom+xml, application/xml;q=0.9, */*;q=0.8"

	defaultFeedConnectTimeout        = 10 * time.Second
	defaultFeedTLSHandshakeTimeout   = 10 * time.Second
	defaultFeedResponseHeaderTimeout = 15 * time.Second
	defaultFeedOverallTimeout        = 30 * time.Second
)

// FeedHTTPConfig is the single source of truth for outbound Feed HTTP behavior.
type FeedHTTPConfig struct {
	UserAgent             string
	Accept                string
	ConnectTimeout        time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	OverallTimeout        time.Duration
	// ConfiguredEgressLabel records which egress path the application is
	// configured to use. It is a configuration tag ONLY — not proof of the real
	// public egress — and is emitted as configured_egress_label in failure logs
	// and carried on AccessOutcome.EgressID for execution history. Defaults to
	// "direct"; #22/#24 egress experiments must pair it with network-side
	// evidence before drawing any conclusion.
	ConfiguredEgressLabel string
}

// DefaultFeedHTTPConfig returns honest, low-load defaults for the given overall
// timeout. It lets callers keep passing a single timeout while still getting
// layered connect/TLS/response-header behavior.
func DefaultFeedHTTPConfig(overall time.Duration) FeedHTTPConfig {
	if overall <= 0 {
		overall = defaultFeedOverallTimeout
	}
	return FeedHTTPConfig{
		UserAgent:             defaultFeedUserAgent,
		Accept:                defaultFeedAccept,
		ConnectTimeout:        defaultFeedConnectTimeout,
		TLSHandshakeTimeout:   defaultFeedTLSHandshakeTimeout,
		ResponseHeaderTimeout: defaultFeedResponseHeaderTimeout,
		OverallTimeout:        overall,
		ConfiguredEgressLabel: EgressDirect,
	}
}

// Fetcher RSS Feed抓取器
type Fetcher struct {
	mu          sync.RWMutex
	httpClient  *http.Client
	httpConfig  FeedHTTPConfig
	coordinator *Coordinator
}

// FetchResult 抓取结果
type FetchResult struct {
	Feed          *gofeed.Feed
	RawContent    []byte
	NewItems      []*gofeed.Item
	Error         error
	IsIncremental bool // 是否为增量抓取
	Access        AccessOutcome
}

// NewFetcher 创建RSS Feed抓取器
func NewFetcher(timeout time.Duration) *Fetcher {
	return NewFetcherWithCoordinator(timeout, SharedCoordinator())
}

// NewFetcherWithCoordinator is the injection seam for isolated tests and
// staged policies. Normal application code should use NewFetcher so all
// workflow fetches share the process-wide coordinator.
func NewFetcherWithCoordinator(timeout time.Duration, coordinator *Coordinator) *Fetcher {
	return NewFetcherWithHTTPConfig(DefaultFeedHTTPConfig(timeout), coordinator)
}

// NewFetcherWithHTTPConfig allows tests and staged policies to override the
// layered HTTP timeouts and headers without changing the shared coordinator.
func NewFetcherWithHTTPConfig(config FeedHTTPConfig, coordinator *Coordinator) *Fetcher {
	if config.OverallTimeout <= 0 {
		config.OverallTimeout = defaultFeedOverallTimeout
	}
	if config.ConnectTimeout <= 0 {
		config.ConnectTimeout = defaultFeedConnectTimeout
	}
	if config.TLSHandshakeTimeout <= 0 {
		config.TLSHandshakeTimeout = defaultFeedTLSHandshakeTimeout
	}
	if config.ResponseHeaderTimeout <= 0 {
		config.ResponseHeaderTimeout = defaultFeedResponseHeaderTimeout
	}
	if config.UserAgent == "" {
		config.UserAgent = defaultFeedUserAgent
	}
	if config.Accept == "" {
		config.Accept = defaultFeedAccept
	}
	if config.ConfiguredEgressLabel == "" {
		config.ConfiguredEgressLabel = EgressDirect
	}
	return &Fetcher{
		httpConfig:  config,
		coordinator: coordinator,
		httpClient: &http.Client{
			Timeout:   config.OverallTimeout,
			Transport: newFeedHTTPTransport(config),
		},
	}
}

// newFeedHTTPTransport builds a layered transport: a bounded connect dialer,
// separate TLS-handshake and response-header timeouts, and a conservative
// connection pool. The transport intentionally does not set Accept-Encoding,
// so Go transparently negotiates and decompresses gzip.
func newFeedHTTPTransport(config FeedHTTPConfig) *http.Transport {
	dialer := &net.Dialer{Timeout: config.ConnectTimeout}
	return &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
		ExpectContinueTimeout: 10 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		DisableKeepAlives:     false,
		MaxConnsPerHost:       20,
		ForceAttemptHTTP2:     true,
	}
}

func (f *Fetcher) newParser() *gofeed.Parser {
	return gofeed.NewParser()
}

func (f *Fetcher) userAgent() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.httpConfig.UserAgent
}

func (f *Fetcher) accept() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.httpConfig.Accept
}

// configuredEgressLabel returns the configured egress tag (default "direct").
// It is a configuration label only and must never be presented as proof of the
// real public egress.
func (f *Fetcher) configuredEgressLabel() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.httpConfig.ConfiguredEgressLabel
}

// SetConfiguredEgressLabel overrides the configured egress tag so #22/#24
// egress experiments can label their requests. It does not change any network
// behavior, only the observation tag emitted in logs, execution history, and
// the admin diagnostics view.
func (f *Fetcher) SetConfiguredEgressLabel(label string) {
	f.mu.Lock()
	if label != "" {
		f.httpConfig.ConfiguredEgressLabel = label
	}
	f.mu.Unlock()
	// Mirror the tag onto the process-wide metrics registry so the admin
	// diagnostics view reports the same egress label seen in failure logs.
	if label != "" {
		SharedFeedMetrics().SetConfiguredEgressLabel(label)
	}
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
	return f.fetchFeedWithContextDetailed(ctx, feedURL, AccessSourcePrimary)
}

// FetchFeedWithContextDetailedAsSource is the source-aware seam used by a
// verified alternative Feed. Marking the source before Coordinator.Do returns
// ensures metrics and the returned AccessOutcome agree on the selected source.
func (f *Fetcher) FetchFeedWithContextDetailedAsSource(ctx context.Context, feedURL string, source AccessSource) (result *FetchResult, err error) {
	return f.fetchFeedWithContextDetailed(ctx, feedURL, source)
}

func (f *Fetcher) fetchFeedWithContextDetailed(ctx context.Context, feedURL string, source AccessSource) (result *FetchResult, err error) {
	if source == AccessSourceUnknown || source == "" {
		source = AccessSourcePrimary
	}
	if f.coordinator == nil {
		result, err = f.fetchFeedWithContextDirect(ctx, feedURL, RequestValidators{})
		if result != nil {
			result.Access.SourceType = source
		}
		return result, err
	}
	return f.coordinator.Do(ctx, feedURL, func(operationCtx context.Context, validators RequestValidators) (*FetchResult, error) {
		result, err := f.fetchFeedWithContextDirect(operationCtx, feedURL, validators)
		if result != nil {
			result.Access.SourceType = source
		}
		return result, err
	})
}

func (f *Fetcher) fetchFeedWithContextDirect(ctx context.Context, feedURL string, validators RequestValidators) (result *FetchResult, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	startTime := time.Now()
	result = &FetchResult{Access: newPrimaryAccessOutcome(feedURL)}
	result.Access.EgressID = f.configuredEgressLabel()
	safeURL := SanitizeFeedURL(feedURL)
	// phase tracks the connection stage reached so far via httptrace; it is
	// stamped onto the outcome (and the structured failure log) only when err
	// != nil. A status-level refusal (incl. 403) is always response_header or
	// body_read — never connect.
	var phase FailurePhase
	setDuration := func() {
		result.Access.ResponseTimeMs = int(time.Since(startTime).Milliseconds())
		result.Error = err
		if err != nil && !errors.Is(err, ErrFeedNotModified) {
			result.Access.FailurePhase = phase
			logger.WithFields(feedFailureLogFields(result, safeURL)).Warn("feed fetch failed")
		}
	}
	defer setDuration()

	logger.Infof("  📡 HTTP GET: %s", safeURL)
	if ctx.Err() != nil {
		result.Access.ErrorCategory = ErrorCategoryCancelled
		return result, ctx.Err()
	}

	requestCtx, cancel := context.WithTimeout(ctx, f.httpConfig.OverallTimeout)
	defer cancel()

	tracedCtx := httptrace.WithClientTrace(requestCtx, newFeedFetchTrace(&phase))
	req, requestErr := http.NewRequestWithContext(tracedCtx, http.MethodGet, feedURL, nil)
	if requestErr != nil {
		err = WrapHTTPError(feedURL, requestErr)
		result.Access.ErrorCategory = ErrorCategoryInvalidRequest
		return result, err
	}
	req.Header.Set("User-Agent", f.userAgent())
	req.Header.Set("Accept", f.accept())
	// Conditional GET: only attach validators the Coordinator loaded from a
	// fingerprint-validated snapshot, so they describe exactly the content we
	// can recover on a 304.
	if validators.Present() {
		if validators.IfNoneMatch != "" {
			req.Header.Set("If-None-Match", validators.IfNoneMatch)
		}
		if validators.IfModifiedSince != "" {
			req.Header.Set("If-Modified-Since", validators.IfModifiedSince)
		}
	}

	resp, requestErr := f.httpClient.Do(req)
	if requestErr != nil {
		if requestCtx.Err() != nil {
			err = requestCtx.Err()
			if errorsIsDeadline(err) {
				result.Access.ErrorCategory = ErrorCategoryTimeout
			} else {
				result.Access.ErrorCategory = ErrorCategoryCancelled
			}
			return result, err
		}
		err = WrapHTTPError(feedURL, requestErr)
		result.Access.ErrorCategory = ErrorCategoryNetwork
		return result, err
	}
	defer resp.Body.Close()

	status := resp.StatusCode
	// Headers received: any subsequent failure (status refusal, body parse) is
	// by definition response_header or body_read, never connect.
	phase = FailurePhaseResponseHeader
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

	if status == http.StatusNotModified {
		// 304 Not Modified: the conditional check succeeded. There is no body
		// to parse; surface the outcome as not_modified and let the Coordinator
		// recover the Feed from the persisted snapshot. ErrFeedNotModified never
		// escapes callers — the Coordinator converts it to a recovered success.
		result.Access.CacheStatus = CacheStatusNotModified
		result.Access.Freshness = FreshnessFresh
		result.Access.ErrorCategory = ErrorCategoryNone
		err = ErrFeedNotModified
		logger.Infof("  ⏭️ HTTP 304 Not Modified: %s (耗时: %v)", safeURL, time.Since(startTime))
		return result, err
	}

	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		err = WrapHTTPError(feedURL, newHTTPStatusError(status))
		result.Access.ErrorCategory = errorCategoryForStatus(status)
		result.Access.Freshness = FreshnessUnknown
		return result, err
	}

	// Body read/parse phase: a parse failure from here is body_read.
	phase = FailurePhaseBodyRead
	var counter byteCounter
	capture := &boundedFeedCapture{maxBytes: maxCapturedFeedSnapshotBytes}
	parsedFeed, parseErr := f.newParser().Parse(io.TeeReader(resp.Body, io.MultiWriter(&counter, capture)))
	result.Access.ResponseBytes = counter.BytesRead
	if parseErr != nil {
		err = WrapFeedParseError(feedURL, parseErr)
		result.Access.ErrorCategory = ErrorCategoryParse
		result.Access.Freshness = FreshnessUnknown
		return result, err
	}

	result.Feed = parsedFeed
	result.RawContent = capture.Bytes()
	result.Access.ErrorCategory = ErrorCategoryNone
	result.Access.Freshness = FreshnessLive
	result.Access.EgressID = f.configuredEgressLabel()
	retrievedAt := time.Now()
	result.Access.RetrievedAt = &retrievedAt
	logger.Infof("  ✅ HTTP请求成功: %s (耗时: %v, 标题: %s, 单集数: %d)",
		safeURL, time.Since(startTime), parsedFeed.Title, len(parsedFeed.Items))
	return result, nil
}

// FetchLastGoodWithContext returns a validated local snapshot after a live
// request has already failed. It is explicit so a later verified alternative
// Feed can be attempted before stale last-good content.
func (f *Fetcher) FetchLastGoodWithContext(ctx context.Context, feedURL string, failure *FetchResult) (*FetchResult, bool) {
	if f == nil || f.coordinator == nil {
		return nil, false
	}
	return f.coordinator.LastGood(ctx, feedURL, failure)
}

// SetIncrementalItems derives the incremental view from an already selected
// Feed, including a local last-good result.
func (r *FetchResult) SetIncrementalItems(lastFetchTime time.Time) {
	if r == nil || r.Feed == nil {
		return
	}
	r.IsIncremental = true
	r.NewItems = r.NewItems[:0]
	for _, item := range r.Feed.Items {
		if item.PublishedParsed != nil && item.PublishedParsed.After(lastFetchTime) {
			r.NewItems = append(r.NewItems, item)
		}
	}
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
	return f.fetchIncrementalWithContext(ctx, feedURL, lastFetchTime, AccessSourcePrimary)
}

// FetchIncrementalWithContextAsSource preserves the semantic source label for
// verified alternative Feed requests all the way through Coordinator metrics.
func (f *Fetcher) FetchIncrementalWithContextAsSource(ctx context.Context, feedURL string, lastFetchTime time.Time, source AccessSource) (*FetchResult, error) {
	return f.fetchIncrementalWithContext(ctx, feedURL, lastFetchTime, source)
}

func (f *Fetcher) fetchIncrementalWithContext(ctx context.Context, feedURL string, lastFetchTime time.Time, source AccessSource) (*FetchResult, error) {
	result, err := f.FetchFeedWithContextDetailedAsSource(ctx, feedURL, source)
	if result == nil {
		return nil, err
	}
	result.IsIncremental = true
	if err != nil {
		return result, err
	}

	result.SetIncrementalItems(lastFetchTime)
	return result, nil
}

// ValidateFeed 验证RSS Feed是否有效
func (f *Fetcher) ValidateFeed(feedURL string) error {
	_, err := f.FetchFeed(feedURL)
	return err
}

// SetUserAgent overrides the product User-Agent. The override is the single
// source of truth: it flows into every outbound request header and is never
// duplicated onto the parser (gofeed ignores User-Agent on Parse).
func (f *Fetcher) SetUserAgent(userAgent string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if userAgent != "" {
		f.httpConfig.UserAgent = userAgent
	}
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
