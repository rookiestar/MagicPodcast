package middleware

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 这些常量集中管理请求体大小上限。数值为经过评审的保守上限，修改需同步更新
// docs/BACKUP_RECOVERY.md 与发布检查清单中的安全约束说明。
const (
	// DefaultImageResponseLimitBytes 限制图片代理从单张上游图片读取或回写的最大字节数。
	// 在图片处理器中同时用于响应头 Content-Length 预检、有界读取以及缓存入库判断。
	DefaultImageResponseLimitBytes int64 = 10 * 1024 * 1024 // 10 MiB

	// DefaultUploadRequestLimitBytes 限制上传类接口（OPML 导入等）的请求体大小。
	DefaultUploadRequestLimitBytes int64 = 10 * 1024 * 1024 // 10 MiB
)

const requestBodyLimitExceededKey = "magicpodcastRequestBodyLimitExceeded"

// boundedBodyReader 包裹 http.MaxBytesReader，并在触发大小上限时记录标志位，
// 供下游处理器通过 RequestBodyLimitExceeded 判断 FormFile 等读取失败是否由超限引起。
type boundedBodyReader struct {
	rc       io.ReadCloser
	exceeded *bool
}

func (r *boundedBodyReader) Read(p []byte) (int, error) {
	n, err := r.rc.Read(p)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			*r.exceeded = true
		}
	}
	return n, err
}

func (r *boundedBodyReader) Close() error {
	return r.rc.Close()
}

// RequestBodyLimit 返回一个限制请求体大小的中间件。超过 limit 时读取会失败，
// 并通过 RequestBodyLimitExceeded 暴露给下游处理器，由其统一返回 413。
func RequestBodyLimit(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		exceeded := new(bool)
		c.Request.Body = &boundedBodyReader{
			rc:       http.MaxBytesReader(c.Writer, c.Request.Body, limit),
			exceeded: exceeded,
		}
		c.Set(requestBodyLimitExceededKey, exceeded)
		c.Next()
	}
}

// RequestBodyLimitExceeded 返回最近一次请求体读取是否触及大小上限。
// 仅在 RequestBodyLimit 中间件生效且发生过超限读取时为 true。
func RequestBodyLimitExceeded(c *gin.Context) bool {
	value, ok := c.Get(requestBodyLimitExceededKey)
	if !ok {
		return false
	}
	flag, ok := value.(*bool)
	if !ok || flag == nil {
		return false
	}
	return *flag
}

// RequestTooLargeResponse 统一返回 413 与 REQUEST_TOO_LARGE 错误码。
func RequestTooLargeResponse(c *gin.Context, limit int64) {
	c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "REQUEST_TOO_LARGE",
			"message": fmt.Sprintf("request body exceeds %d bytes", limit),
		},
	})
}
