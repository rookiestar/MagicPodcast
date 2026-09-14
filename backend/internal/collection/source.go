package collection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// 来源读取失败分类：与解析失败共同构成可区分的失败结果。
var (
	ErrInvalidCollectionURL = errors.New("not a supported collection URL")
	ErrSourceForbidden      = errors.New("collection source refused access")
	ErrSourceUnavailable    = errors.New("collection source is unreachable")
	ErrSourceTooLarge       = errors.New("collection page exceeds size limit")
)

const (
	// collectionHost 与 campaignHost 是仅有的两个允许的清单来源主机；
	// 新增主机属于明确的安全评审决定。
	collectionHost = "www.xiaoyuzhoufm.com"
	campaignHost   = "collection.xiaoyuzhoufm.com"

	// campaignAPIHost/Path 是专题页客户端渲染所用的公开数据接口；
	// 专题抓取始终使用由 slug 构造的该地址，不接受任意 URL。
	campaignAPIHost = "api.xiaoyuzhoufm.com"
	campaignAPIPath = "/v1/campaign/get"

	// collectionPathPattern 匹配 /collection/episode/{id}；id 为 24 位十六进制。
	collectionPathPattern = `^/collection/episode/([0-9a-f]{24})$`

	// campaignSlugBody 限定专题 slug 字符集；campaignPathPattern 与接口查询
	// 参数校验共用同一份字符集，避免两处规则漂移。
	campaignSlugBody      = `[0-9a-zA-Z][0-9a-zA-Z_-]{0,63}`
	campaignPathPattern   = `^/(` + campaignSlugBody + `)$`
	campaignSlugPattern   = `^(` + campaignSlugBody + `)$`
	campaignComponentKind = "EPISODE_LIST"

	// maxCollectionPageBytes 限制清单页响应大小，防止异常来源耗尽内存。
	maxCollectionPageBytes = 3 << 20

	// collectionFetchTimeout 单次读取清单页的 总超时。
	collectionFetchTimeout = 20 * time.Second

	// collectionRedirectLimit 逐跳重定向上限；每一跳都重新走完整 URL 校验。
	collectionRedirectLimit = 3
)

var (
	collectionPathRegexp = regexp.MustCompile(collectionPathPattern)
	campaignPathRegexp   = regexp.MustCompile(campaignPathPattern)
	campaignSlugRegexp   = regexp.MustCompile(campaignSlugPattern)
)

// sourceKind 区分同平台内两种已核实的来源形态；来源身份、抓取目标与解析器都由它决定。
type sourceKind int

const (
	sourceEpisodeCollection sourceKind = iota
	sourceCampaign
)

// CollectionURL 是通过校验的清单地址，ExternalID 从路径中确定性识别，
// Kind 决定抓取与解析方式。
type CollectionURL struct {
	Raw        string
	ExternalID string
	Kind       sourceKind
}

// ParseCollectionURL 校验用户提交的清单链接：仅接受经过核实的源站主机与
// 清单路径，拒绝端口、用户信息与 IP 直连；查询参数和片段在识别时剥离，
// 抓取始终使用规范化地址，避免把任意网页读取能力扩大成通用抓取入口。
func ParseCollectionURL(rawURL string) (*CollectionURL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty url", ErrInvalidCollectionURL)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCollectionURL, err)
	}
	if parsed.Scheme != "https" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("%w: scheme/host", ErrInvalidCollectionURL)
	}
	if parsed.User != nil || parsed.Port() != "" {
		return nil, fmt.Errorf("%w: port/userinfo", ErrInvalidCollectionURL)
	}
	if net.ParseIP(parsed.Hostname()) != nil {
		return nil, fmt.Errorf("%w: host %q", ErrInvalidCollectionURL, parsed.Hostname())
	}
	switch normalizeHost(parsed.Hostname()) {
	case collectionHost:
		match := collectionPathRegexp.FindStringSubmatch(parsed.Path)
		if match == nil {
			return nil, fmt.Errorf("%w: path %q", ErrInvalidCollectionURL, parsed.Path)
		}
		return &CollectionURL{
			Raw:        canonicalCollectionURL(match[1]),
			ExternalID: match[1],
			Kind:       sourceEpisodeCollection,
		}, nil
	case campaignHost:
		match := campaignPathRegexp.FindStringSubmatch(parsed.Path)
		if match == nil {
			return nil, fmt.Errorf("%w: path %q", ErrInvalidCollectionURL, parsed.Path)
		}
		return &CollectionURL{
			Raw:        canonicalCampaignURL(match[1]),
			ExternalID: match[1],
			Kind:       sourceCampaign,
		}, nil
	default:
		return nil, fmt.Errorf("%w: host %q", ErrInvalidCollectionURL, parsed.Hostname())
	}
}

// canonicalCollectionURL 由清单 ID 构造规范化地址；抓取始终使用该地址。
func canonicalCollectionURL(externalID string) string {
	return "https://" + collectionHost + "/collection/episode/" + externalID
}

// IdentityKey 返回清单的稳定来源身份。清单 ID 是 24 位十六进制，与专题 slug
// 字符集有理论交集；带来源形态命名空间可避免两个无关来源在
// (source_platform, external_id) 唯一约束下被合并为同一份清单。
func (c *CollectionURL) IdentityKey() string {
	if c.Kind == sourceCampaign {
		return "campaign:" + c.ExternalID
	}
	return c.ExternalID
}

// canonicalCampaignURL 由专题 slug 构造规范化页面地址，作为清单的来源身份保存。
func canonicalCampaignURL(slug string) string {
	return "https://" + campaignHost + "/" + slug
}

// campaignAPIURL 由 slug 构造专题数据接口地址；slug 已通过字符集校验并经
// url.Values 转义，抓取地址完全由代码构造。
func campaignAPIURL(slug string) string {
	return "https://" + campaignAPIHost + campaignAPIPath + "?" + url.Values{"slug": {slug}}.Encode()
}

// campaignAPIHopAllowed 校验专题抓取的重定向目标必须仍是同一 slug 的接口地址，
// 防止重定向把读取扩大到其他地址空间。
func campaignAPIHopAllowed(target *url.URL, slug string) bool {
	if target.Scheme != "https" || target.User != nil || target.Port() != "" {
		return false
	}
	if normalizeHost(target.Hostname()) != campaignAPIHost || target.Path != campaignAPIPath {
		return false
	}
	query := target.Query()
	return len(query) == 1 && query.Get("slug") == slug
}

func normalizeHost(hostname string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")
}

// fetchFunc 抓取一个清单页并返回 HTML 字节。生产实现自带私网隔离与
// 逐跳校验；测试注入可控来源响应，不需要真实网络。
type fetchFunc func(ctx context.Context, pageURL string) ([]byte, error)

// newProductionFetcher 构造带安全边界的清单来源抓取函数：
// 解析后直连（绕过继承的代理环境变量）、阻止私网/环回/CGNAT 地址、
// 每一跳重定向重新校验目标、限制响应大小与总超时。
// 单集清单读取规范化页面；专题读取由 slug 构造的公开数据接口。
func newProductionFetcher() fetchFunc {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           newSafeDialContext(dialer),
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: collectionFetchTimeout,
		MaxIdleConns:          2,
		IdleConnTimeout:       time.Minute,
	}
	client := &http.Client{
		Timeout:   collectionFetchTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= collectionRedirectLimit {
				return fmt.Errorf("stopped after %d redirects", collectionRedirectLimit)
			}
			// 每一跳都必须仍然属于起始请求同一来源形态的已核实地址空间，
			// 防止重定向绕过主机与路径边界。
			source, err := ParseCollectionURL(via[0].URL.String())
			if err != nil {
				// 起始地址不是清单页面时，只可能是本包构造的专题接口地址。
				source, err = campaignSourceFromAPIURL(via[0].URL)
				if err != nil {
					return fmt.Errorf("collection fetch origin rejected: %w", err)
				}
			}
			switch source.Kind {
			case sourceCampaign:
				if !campaignAPIHopAllowed(req.URL, source.ExternalID) {
					return fmt.Errorf("collection redirect target rejected: %q", req.URL)
				}
			default:
				if _, err := ParseCollectionURL(req.URL.String()); err != nil {
					return fmt.Errorf("collection redirect target rejected: %w", err)
				}
			}
			return nil
		},
	}
	return func(ctx context.Context, pageURL string) ([]byte, error) {
		source, err := ParseCollectionURL(pageURL)
		if err != nil {
			return nil, err
		}
		fetchURL := source.Raw
		accept := "text/html,application/xhtml+xml"
		if source.Kind == sourceCampaign {
			fetchURL = campaignAPIURL(source.ExternalID)
			accept = "application/json"
		}

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidCollectionURL, err)
		}
		request.Header.Set("User-Agent", collectionUserAgent)
		request.Header.Set("Accept", accept)

		response, err := client.Do(request)
		if err != nil {
			// 客户端超时与网络错误都归入来源不可用；调用方保留既有数据可重试。
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w: timeout", ErrSourceUnavailable)
			}
			return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
		}
		defer func() { _ = response.Body.Close() }()

		switch {
		case response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusUnauthorized:
			return nil, fmt.Errorf("%w: http %d", ErrSourceForbidden, response.StatusCode)
		case response.StatusCode >= 400:
			return nil, fmt.Errorf("%w: http %d", ErrSourceUnavailable, response.StatusCode)
		case response.StatusCode < 200 || response.StatusCode >= 300:
			return nil, fmt.Errorf("%w: unexpected http %d", ErrSourceUnavailable, response.StatusCode)
		}

		body, err := io.ReadAll(io.LimitReader(response.Body, maxCollectionPageBytes+1))
		if err != nil {
			return nil, fmt.Errorf("%w: read body: %v", ErrSourceUnavailable, err)
		}
		if len(body) > maxCollectionPageBytes {
			return nil, ErrSourceTooLarge
		}
		return body, nil
	}
}

// campaignSourceFromAPIURL 识别本包 campaignAPIURL 构造的接口地址，
// 用于重定向校验时恢复来源身份；其他地址一律拒绝。
func campaignSourceFromAPIURL(parsed *url.URL) (*CollectionURL, error) {
	if parsed == nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" ||
		normalizeHost(parsed.Hostname()) != campaignAPIHost || parsed.Path != campaignAPIPath {
		return nil, fmt.Errorf("%w: not a campaign api url %q", ErrInvalidCollectionURL, parsed)
	}
	slug := parsed.Query().Get("slug")
	if !campaignSlugRegexp.MatchString(slug) || len(parsed.Query()) != 1 {
		return nil, fmt.Errorf("%w: campaign api slug %q", ErrInvalidCollectionURL, slug)
	}
	return &CollectionURL{
		Raw:        canonicalCampaignURL(slug),
		ExternalID: slug,
		Kind:       sourceCampaign,
	}, nil
}

const collectionUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func newSafeDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("split collection address: %w", err)
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve collection host: %w", err)
		}
		if len(ips) == 0 {
			return nil, errors.New("collection host resolved to no addresses")
		}
		var lastErr error
		for _, ipAddr := range ips {
			if isBlockedCollectionIP(ipAddr.IP) {
				return nil, fmt.Errorf("collection host resolved to a private or local address: %s", ipAddr.IP)
			}
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ipAddr.IP.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		return nil, fmt.Errorf("dial collection host: %w", lastErr)
	}
}

func isBlockedCollectionIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// CGNAT 100.64.0.0/10 同样视为私网边界。
	ip4 := ip.To4()
	return ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127
}
