package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/middleware"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	imageFetchTimeout = 10 * time.Second
	imageDialTimeout  = 5 * time.Second
)

var (
	errInvalidImageURL   = errors.New("invalid image URL")
	errImageHostNotAllow = errors.New("image host is not allowed")
)

// ImageHandler 图片代理处理器
type ImageHandler struct {
	httpClient   *http.Client
	allowedHosts []string // 允许代理的域名白名单
}

// NewImageHandler 创建图片代理处理器
func NewImageHandler() *ImageHandler {
	return &ImageHandler{
		httpClient: &http.Client{
			Timeout: imageFetchTimeout,
			Transport: &http.Transport{
				// Direct connections keep the DNS/IP policy effective even if the
				// process inherits proxy environment variables.
				DialContext:           newSafeImageDialContext(defaultImageLookup, &net.Dialer{Timeout: imageDialTimeout}),
				TLSHandshakeTimeout:   imageDialTimeout,
				ResponseHeaderTimeout: imageFetchTimeout,
				MaxIdleConns:          8,
				MaxIdleConnsPerHost:   2,
				IdleConnTimeout:       30 * time.Second,
			},
			// 禁止自动跟随重定向，防止通过跳转绕过白名单和 DNS 检查。
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		allowedHosts: approvedImageHosts(),
	}
}

// approvedImageHosts is the reviewed set observed in the current podcast and
// episode data. Adding a host is an explicit security review decision.
func approvedImageHosts() []string {
	return []string{
		"assets.fireside.fm",
		"assets.pippa.io",
		"bts-image.xyzcdn.net",
		"cdn.justinbot.com",
		"cdn.lizhi.fm",
		"cdn.vistopia.com.cn",
		"cdn.wavpub.com",
		"cdn.wlz.danlirencomedy.com",
		"cdn2.jjldbk.com",
		"cdn2.wavpub.com",
		"cdn5.vistopia.com.cn",
		"content.production.cdn.art19.com",
		"crazy.capital",
		"d3t3ozftmdmh3i.cloudfront.net",
		"face.t.sinajs.cn",
		"fdfs.xmcdn.com",
		"files.fireside.fm",
		"host.podapi.xyz",
		"hosting.wavpub.cn",
		"i.typlog.com",
		"image-qiniu.jellow.site",
		"image.firstory-cdn.me",
		"image.xyzcdn.net",
		"images.pexels.com",
		"images.unsplash.com",
		"imagev2.xmcdn.com",
		"img.transistorcdn.com",
		"is1-ssl.mzstatic.com",
		"is2-ssl.mzstatic.com",
		"is3-ssl.mzstatic.com",
		"is4-ssl.mzstatic.com",
		"is5-ssl.mzstatic.com",
		"jsftwafp1d.feishu.cn",
		"justpodmedia.com",
		"lexfridman.com",
		"media.redcircle.com",
		"media.smfm2016.com",
		"media.wavpub.com",
		"media24.fireside.fm",
		"megaphone.imgix.net",
		"mmbiz.qpic.cn",
		"pan.icu",
		"pie.wetime.com",
		"radio-res.cgtn.com",
		"rio.xyzcdn.net",
		"s.anyway.red",
		"s.w.org",
		"static.storyfm.cn",
		"static2.ximalaya.com",
		"storage.buzzsprout.com",
		"uploader.shimo.im",
		"v2km9a2fuc.feishu.cn",
		"xueqiu.feishu.cn",
		// 兼容仍在使用的旧来源记录。
		"typlog.com",
		"a1.mzstatic.com",
		"a2.mzstatic.com",
		"a3.mzstatic.com",
		"a4.mzstatic.com",
		"a5.mzstatic.com",
	}
}

func normalizeImageHostname(hostname string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")
}

func (h *ImageHandler) isAllowedHost(hostname string) bool {
	normalized := normalizeImageHostname(hostname)
	for _, allowedHost := range h.allowedHosts {
		if normalized == normalizeImageHostname(allowedHost) {
			return true
		}
	}
	return false
}

func (h *ImageHandler) validateImageURL(rawURL string) (*url.URL, error) {
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, errInvalidImageURL
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, errInvalidImageURL
	}
	if parsedURL.User != nil || parsedURL.Port() != "" {
		return nil, errImageHostNotAllow
	}

	hostname := normalizeImageHostname(parsedURL.Hostname())
	if hostname == "" || net.ParseIP(hostname) != nil || !h.isAllowedHost(hostname) {
		return nil, errImageHostNotAllow
	}

	return parsedURL, nil
}

type imageLookupFunc func(context.Context, string) ([]net.IPAddr, error)

func defaultImageLookup(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

func newSafeImageDialContext(lookup imageLookupFunc, dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	if lookup == nil {
		lookup = defaultImageLookup
	}
	if dialer == nil {
		dialer = &net.Dialer{Timeout: imageDialTimeout}
	}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("split image address: %w", err)
		}

		ips, err := lookup(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve image host: %w", err)
		}
		if len(ips) == 0 {
			return nil, errors.New("image host resolved to no addresses")
		}

		var lastErr error
		for _, ipAddr := range ips {
			if isBlockedImageIP(ipAddr.IP) {
				return nil, fmt.Errorf("image host resolved to a private or local address: %s", ipAddr.IP)
			}

			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ipAddr.IP.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}

		return nil, fmt.Errorf("dial image host: %w", lastErr)
	}
}

func isBlockedImageIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}

	ip4 := ip.To4()
	return ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127
}

func normalizedImageContentType(raw string) (string, bool) {
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return "", false
	}

	switch strings.ToLower(mediaType) {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/avif":
		return strings.ToLower(mediaType), true
	default:
		return "", false
	}
}

func imageBodyMatchesContentType(body []byte, contentType string) bool {
	if len(body) == 0 {
		return false
	}

	// Go's content sniffer does not recognize every AVIF variant, so retain
	// the strict media-type check for AVIF and reject SVG entirely above.
	if contentType == "image/avif" {
		return bytes.Contains(body[:minInt(len(body), 64)], []byte("ftypavif")) ||
			bytes.Contains(body[:minInt(len(body), 64)], []byte("ftypavis"))
	}

	detected := http.DetectContentType(body)
	return detected == contentType
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ProxyImage 代理图片请求
func (h *ImageHandler) ProxyImage(c *gin.Context) {
	imageURL := c.Query("url")
	if imageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing url parameter"})
		return
	}

	parsedURL, err := h.validateImageURL(imageURL)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errImageHostNotAllow) {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		logger.Infof("[ImageProxy] 创建请求失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to create request"})
		return
	}
	req.Header.Set("User-Agent", "MagicPodcast/1.0")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*;q=0.8")
	req.Header.Set("Referer", parsedURL.Scheme+"://"+parsedURL.Hostname()+"/")

	startTime := time.Now()
	resp, err := h.httpClient.Do(req)
	if err != nil {
		logger.Infof("[ImageProxy] 请求失败 [%s]: %v", imageURL, err)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": "failed to fetch image"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
		logger.Infof("[ImageProxy] 拒绝重定向 [%s]: %d", imageURL, resp.StatusCode)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": "image redirects are not allowed"})
		return
	}
	if resp.StatusCode != http.StatusOK {
		logger.Infof("[ImageProxy] 非200状态码 [%s]: %d", imageURL, resp.StatusCode)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": fmt.Sprintf("upstream returned status %d", resp.StatusCode)})
		return
	}
	if resp.ContentLength > middleware.DefaultImageResponseLimitBytes {
		middleware.RequestTooLargeResponse(c, middleware.DefaultImageResponseLimitBytes)
		return
	}

	contentType, ok := normalizedImageContentType(resp.Header.Get("Content-Type"))
	if !ok {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"success": false, "error": "upstream content is not an allowed image type"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, middleware.DefaultImageResponseLimitBytes+1))
	if err != nil {
		logger.Infof("[ImageProxy] 传输图片失败 [%s]: %v", imageURL, err)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": "failed to read image"})
		return
	}
	if int64(len(body)) > middleware.DefaultImageResponseLimitBytes {
		middleware.RequestTooLargeResponse(c, middleware.DefaultImageResponseLimitBytes)
		return
	}
	if !imageBodyMatchesContentType(body, contentType) {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": "upstream body does not match image type"})
		return
	}

	// Do not create an unbounded browser, CDN, or proxy cache. A future bounded
	// cache can be added as a separate reviewed change with an explicit budget.
	c.Header("Cache-Control", "no-store, max-age=0")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Type", contentType)
	c.Data(http.StatusOK, contentType, body)

	logger.Infof("[ImageProxy] 传输完成 [%s] - %dms - %d bytes", imageURL, time.Since(startTime).Milliseconds(), len(body))
}

// Health 健康检查
func (h *ImageHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"service":       "image-proxy",
		"status":        "ok",
		"allowed_hosts": h.allowedHosts,
	})
}
