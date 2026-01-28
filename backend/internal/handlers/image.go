package handlers

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/gin-gonic/gin"
)

// ImageHandler 图片代理处理器
type ImageHandler struct {
	httpClient *http.Client
	allowedHosts []string // 允许代理的域名白名单
}

// NewImageHandler 创建图片代理处理器
func NewImageHandler() *ImageHandler {
	return &ImageHandler{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			// 禁止自动跟随重定向，防止SSRF攻击
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		allowedHosts: []string{
			"i.typlog.com",
			"typlog.com",
			"image.xyzcdn.net",
			"fdfs.xmcdn.com",
			"cdn.lizhi.fm",
			"is1-ssl.mzstatic.com",
			"is2-ssl.mzstatic.com",
			"is3-ssl.mzstatic.com",
			"is4-ssl.mzstatic.com",
			"is5-ssl.mzstatic.com",
			"a5.mzstatic.com",
			"a1.mzstatic.com",
			"a2.mzstatic.com",
			"a3.mzstatic.com",
			"a4.mzstatic.com",
		},
	}
}

// ProxyImage 代理图片请求
func (h *ImageHandler) ProxyImage(c *gin.Context) {
	// 获取目标图片URL
	imageURL := c.Query("url")
	if imageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "missing url parameter",
		})
		return
	}

	// 解析URL验证
	parsedURL, err := url.Parse(imageURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid url format",
		})
		return
	}

	// 检查协议
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "only http and https protocols are allowed",
		})
		return
	}

	// 检查域名白名单
	allowed := false
	for _, allowedHost := range h.allowedHosts {
		if parsedURL.Host == allowedHost || parsedURL.Host == "."+allowedHost {
			allowed = true
			break
		}
	}

	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   fmt.Sprintf("domain %s is not in whitelist", parsedURL.Host),
		})
		return
	}

	// 创建代理请求
	req, err := http.NewRequestWithContext(context.Background(), "GET", imageURL, nil)
	if err != nil {
		log.Printf("[ImageProxy] 创建请求失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to create request",
		})
		return
	}

	// 设置请求头，模拟浏览器
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/webp,image/apng,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", parsedURL.Scheme+"://"+parsedURL.Host+"/")

	// 发送请求
	startTime := time.Now()
	resp, err := h.httpClient.Do(req)
	if err != nil {
		log.Printf("[ImageProxy] 请求失败 [%s]: %v", imageURL, err)
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   "failed to fetch image",
		})
		return
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		log.Printf("[ImageProxy] 非200状态码 [%s]: %d", imageURL, resp.StatusCode)
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   fmt.Sprintf("upstream returned status %d", resp.StatusCode),
		})
		return
	}

	// 检查Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		// 尝试从URL推断
		ext := path.Ext(parsedURL.Path)
		switch ext {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		case ".gif":
			contentType = "image/gif"
		case ".webp":
			contentType = "image/webp"
		default:
			contentType = "image/jpeg" // 默认
		}
	}

	// 读取图片内容到内存（用于计算ETag）
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[ImageProxy] 读取图片失败 [%s]: %v", imageURL, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to read image data",
		})
		return
	}

	// 计算ETag（基于内容的MD5哈希）
	hash := md5.Sum(imageData)
	etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:]))

	// 检查条件请求（If-None-Match）
	ifNoneMatch := c.GetHeader("If-None-Match")
	if ifNoneMatch != "" && ifNoneMatch == etag {
		// 内容未变化，返回304
		c.Header("ETag", etag)
		c.Header("Cache-Control", "public, max-age=2592000, immutable")
		c.Header("Last-Modified", time.Now().Format(time.RFC1123))
		c.Status(http.StatusNotModified)
		log.Printf("[ImageProxy] 304 Not Modified [%s] - ETag: %s", imageURL, etag)
		return
	}

	// 记录成功日志
	duration := time.Since(startTime).Milliseconds()
	log.Printf("[ImageProxy] 成功代理图片 [%s] - %dms - %s - %d bytes", imageURL, duration, contentType, len(imageData))

	// 设置增强的缓存头
	// 缓存30天（2592000秒）
	c.Header("Cache-Control", "public, max-age=2592000, immutable")
	c.Header("Content-Type", contentType)
	c.Header("ETag", etag)
	c.Header("Last-Modified", time.Now().Format(time.RFC1123))

	// 支持跨域
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET")
	c.Header("Access-Control-Allow-Headers", "Content-Type")

	// 传输图片到客户端
	written, err := c.Writer.Write(imageData)
	if err != nil {
		log.Printf("[ImageProxy] 传输图片失败 [%s]: %v", imageURL, err)
		return
	}

	log.Printf("[ImageProxy] 传输完成 [%s] - %d bytes - ETag: %s", imageURL, written, etag)
}

// Health 健康检查
func (h *ImageHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"service": "image-proxy",
		"status":  "ok",
		"allowed_hosts": h.allowedHosts,
	})
}
