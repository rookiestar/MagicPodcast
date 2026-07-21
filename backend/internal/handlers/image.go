package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/middleware"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/image/draw"
)

const (
	imageFetchTimeout = 10 * time.Second
	imageDialTimeout  = 5 * time.Second
	// Cloudflare edge (Access / Tunnel) enforces an implicit response body
	// limit around 3–5 MiB that returns 413 regardless of plan. Images larger
	// than this threshold are re-encoded to fit through the edge.
	cloudflareSafeSize = 3 * 1024 * 1024
	// Cover art is displayed at ~256 px; 800 px covers 2× retina with margin.
	coverMaxDimension = 800
	// Image responses use a finite browser/CDN cache window. This avoids
	// repeated fetches after virtualized list remounts without making the
	// response permanently cacheable.
	imageProxyCacheControl = "public, max-age=86400, stale-while-revalidate=604800"
)

var (
	errInvalidImageURL   = errors.New("invalid image URL")
	errImageHostNotAllow = errors.New("image host is not allowed")
)

// ImageHandler 图片代理处理器
type ImageHandler struct {
	httpClient   *http.Client
	allowedHosts []string // 允许代理的域名白名单
	cache        *imageCache
	singleflight *imageSingleFlight
	sem          chan struct{}
}

// NewImageHandler 创建图片代理处理器
func NewImageHandler() *ImageHandler {
	h := &ImageHandler{
		allowedHosts: approvedImageHosts(),
		cache:        newImageCache(),
		singleflight: newImageSingleFlight(),
		sem:          make(chan struct{}, imageFetchConcurrency),
	}
	h.httpClient = &http.Client{
		Timeout: imageFetchTimeout,
		Transport: &http.Transport{
			// Direct connections keep the DNS/IP policy effective even if the
			// process inherits proxy environment variables.
			DialContext:           newSafeImageDialContext(defaultImageLookup, &net.Dialer{Timeout: imageDialTimeout}),
			TLSHandshakeTimeout:   imageDialTimeout,
			ResponseHeaderTimeout: imageFetchTimeout,
			// Connection pool sized to match imageOperation admission limits
			// (MaxConcurrent:32). Each concurrent proxy request needs one
			// outbound connection; undersizing causes requests to queue on
			// dial, which manifests as slow/stuck cover art during scroll.
			MaxIdleConns:        48,
			MaxIdleConnsPerHost: 16,
			IdleConnTimeout:     30 * time.Second,
		},
		// 每一跳重新执行完整目标校验，防止通过重定向绕过白名单、端口和私网地址限制。
		CheckRedirect: h.validateImageRedirect,
	}
	return h
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

func (h *ImageHandler) validateImageRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > imageRedirectLimit {
		return fmt.Errorf("stopped after %d redirects", imageRedirectLimit)
	}
	if _, err := h.validateImageURL(req.URL.String()); err != nil {
		return fmt.Errorf("image proxy: redirect target rejected: %w", err)
	}
	return nil
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
	case "image/jpeg", "image/jpg":
		return "image/jpeg", true
	case "image/png", "image/gif", "image/webp", "image/avif":
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

// maybeCompressImage decodes and re-encodes images that exceed the Cloudflare edge
// response body limit. Cover art displayed at ~256 px does not need multi-MiB
// originals; resizing to coverMaxDimension with JPEG quality 85 preserves visual
// quality while staying well under the ~3–5 MiB edge limit. Returns the original
// body unchanged if it is already small enough, cannot be decoded, or
// re-encoding does not reduce size.
func maybeCompressImage(body []byte, contentType string) ([]byte, string) {
	if int64(len(body)) <= cloudflareSafeSize {
		return body, contentType
	}

	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		// AVIF and some WebP variants cannot be decoded by the standard library;
		// return the original body rather than failing the request.
		return body, contentType
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// If dimensions are already small but file is still large (e.g. uncompressed
	// PNG), try re-encoding as JPEG without resizing.
	if w <= coverMaxDimension && h <= coverMaxDimension {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
			return body, contentType
		}
		if buf.Len() < len(body) {
			logger.Infof("[ImageProxy] 重编码 %dx%d (%d → %d bytes)", w, h, len(body), buf.Len())
			return buf.Bytes(), "image/jpeg"
		}
		return body, contentType
	}

	// Resize to coverMaxDimension preserving aspect ratio.
	ratio := float64(coverMaxDimension) / math.Max(float64(w), float64(h))
	nw := int(math.Round(float64(w) * ratio))
	nh := int(math.Round(float64(h) * ratio))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Rect, img, bounds, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return body, contentType
	}
	logger.Infof("[ImageProxy] 压缩图片 %dx%d → %dx%d (%d → %d bytes)",
		w, h, nw, nh, len(body), buf.Len())
	return buf.Bytes(), "image/jpeg"
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

	cacheKey := parsedURL.String()
	if entry, ok := h.cache.get(cacheKey); ok {
		h.serveImage(c, entry, imageURL, true)
		return
	}

	// 单飞合并相同 URL 的并发请求；有界信号量把上游抓取总量控制在固定范围。
	entry, failure, transport := h.singleflight.do(cacheKey, func() (imageCacheEntry, *imageProxyError, bool) {
		select {
		case h.sem <- struct{}{}:
			defer func() { <-h.sem }()
		case <-c.Request.Context().Done():
			return imageCacheEntry{}, nil, true
		}
		return h.fetchImage(c.Request.Context(), parsedURL)
	})

	if transport {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": "failed to fetch image"})
		return
	}
	if failure != nil {
		c.JSON(failure.status, gin.H{
			"success": false,
			"error":   gin.H{"code": failure.code, "message": failure.message},
		})
		return
	}

	h.cache.put(cacheKey, entry)
	h.serveImage(c, entry, imageURL, false)
}

// fetchImage 执行一次完整的上游校验，只有通过状态、大小、类型和主体嗅探的内容才会返回。
func (h *ImageHandler) fetchImage(ctx context.Context, parsedURL *url.URL) (imageCacheEntry, *imageProxyError, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		logger.Infof("[ImageProxy] 创建请求失败: %v", err)
		return imageCacheEntry{}, nil, true
	}
	req.Header.Set("User-Agent", "MagicPodcast/1.0")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*;q=0.8")
	req.Header.Set("Referer", parsedURL.Scheme+"://"+parsedURL.Hostname()+"/")

	startTime := time.Now()
	resp, err := h.httpClient.Do(req)
	if err != nil {
		logger.Infof("[ImageProxy] 请求失败 [%s]: %v", parsedURL.String(), err)
		return imageCacheEntry{}, nil, true
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
		logger.Infof("[ImageProxy] 拒绝重定向 [%s]: %d", parsedURL.String(), resp.StatusCode)
		return imageCacheEntry{}, &imageProxyError{
			status:  http.StatusBadGateway,
			code:    "REDIRECT_REJECTED",
			message: "image redirects are not allowed",
		}, false
	}
	if resp.StatusCode != http.StatusOK {
		logger.Infof("[ImageProxy] 非200状态码 [%s]: %d", parsedURL.String(), resp.StatusCode)
		return imageCacheEntry{}, &imageProxyError{
			status:  http.StatusBadGateway,
			code:    "UPSTREAM_ERROR",
			message: fmt.Sprintf("upstream returned status %d", resp.StatusCode),
		}, false
	}
	if resp.ContentLength > middleware.DefaultImageResponseLimitBytes {
		return imageCacheEntry{}, &imageProxyError{
			status:  http.StatusRequestEntityTooLarge,
			code:    "REQUEST_TOO_LARGE",
			message: fmt.Sprintf("image exceeds the %d byte limit", middleware.DefaultImageResponseLimitBytes),
		}, false
	}

	contentType, ok := normalizedImageContentType(resp.Header.Get("Content-Type"))
	if !ok {
		return imageCacheEntry{}, &imageProxyError{
			status:  http.StatusUnsupportedMediaType,
			code:    "UNSUPPORTED_IMAGE_TYPE",
			message: "upstream content is not an allowed image type",
		}, false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, middleware.DefaultImageResponseLimitBytes+1))
	if err != nil {
		logger.Infof("[ImageProxy] 传输图片失败 [%s]: %v", parsedURL.String(), err)
		return imageCacheEntry{}, nil, true
	}
	if int64(len(body)) > middleware.DefaultImageResponseLimitBytes {
		return imageCacheEntry{}, &imageProxyError{
			status:  http.StatusRequestEntityTooLarge,
			code:    "REQUEST_TOO_LARGE",
			message: fmt.Sprintf("image exceeds the %d byte limit", middleware.DefaultImageResponseLimitBytes),
		}, false
	}
	if !imageBodyMatchesContentType(body, contentType) {
		return imageCacheEntry{}, &imageProxyError{
			status:  http.StatusBadGateway,
			code:    "IMAGE_BODY_MISMATCH",
			message: "upstream body does not match image type",
		}, false
	}

	// 先压缩大图，再决定是否写入内存缓存；现有 Cloudflare 边界和图片安全保护保持不变。
	body, contentType = maybeCompressImage(body, contentType)
	logger.Infof("[ImageProxy] 传输完成 [%s] - %dms - %d bytes", parsedURL.String(), time.Since(startTime).Milliseconds(), len(body))

	return imageCacheEntry{
		body:        body,
		contentType: contentType,
		expiresAt:   time.Now().Add(imageCacheTTL),
	}, nil, false
}

func (h *ImageHandler) serveImage(c *gin.Context, entry imageCacheEntry, sourceURL string, fromCache bool) {
	c.Header("Cache-Control", imageProxyCacheControl)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Type", entry.contentType)
	c.Data(http.StatusOK, entry.contentType, entry.body)

	logger.Infof("[ImageProxy] 返回 %s [%s] - %d bytes - cache=%t", entry.contentType, sourceURL, len(entry.body), fromCache)
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
