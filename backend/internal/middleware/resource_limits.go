package middleware

import (
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// DefaultUploadRequestLimitBytes is the complete request limit for OPML
// uploads, including multipart framing.
const DefaultUploadRequestLimitBytes int64 = 8 * 1024 * 1024

// DefaultImageResponseLimitBytes bounds the amount of data that the image
// proxy will retain and return for one request. Raised from 5 MiB to 20 MiB to
// accommodate large cover art (e.g. image.xyzcdn.net JPEGs ~7 MiB). The service
// is single-owner with no CDN cache, so this trades bandwidth for coverage.
const DefaultImageResponseLimitBytes int64 = 20 * 1024 * 1024

const requestBodyLimitExceededKey = "magicpodcast.request_body_limit_exceeded"

// OperationPolicy describes the admission limits for one expensive operation.
// The limiter is intentionally process-local: this service has one owner and
// one backend process, so it avoids trusting client-provided identity headers.
type OperationPolicy struct {
	MaxConcurrent int
	MaxRequests   int
	Window        time.Duration
}

type operationState struct {
	inFlight int
	requests []time.Time
}

// OperationLimiter applies process-local rate and concurrency limits.
type OperationLimiter struct {
	mu         sync.Mutex
	operations map[string]*operationState
}

// NewOperationLimiter creates an empty operation limiter.
func NewOperationLimiter() *OperationLimiter {
	return &OperationLimiter{
		operations: make(map[string]*operationState),
	}
}

// Middleware admits a request only when both the rate and concurrency policy
// allow it. Rate-limited requests receive 429; concurrent duplicate work
// receives 409. Both responses use the stable API error shape.
func (l *OperationLimiter) Middleware(name string, policy OperationPolicy) gin.HandlerFunc {
	if l == nil {
		return func(c *gin.Context) { c.Next() }
	}
	if policy.MaxConcurrent < 1 {
		policy.MaxConcurrent = 1
	}

	return func(c *gin.Context) {
		now := time.Now()

		l.mu.Lock()
		state := l.operations[name]
		if state == nil {
			state = &operationState{}
			l.operations[name] = state
		}

		if policy.MaxRequests > 0 && policy.Window > 0 {
			cutoff := now.Add(-policy.Window)
			firstValid := 0
			for firstValid < len(state.requests) && state.requests[firstValid].Before(cutoff) {
				firstValid++
			}
			if firstValid > 0 {
				state.requests = append([]time.Time(nil), state.requests[firstValid:]...)
			}
			if len(state.requests) >= policy.MaxRequests {
				retryAfter := policy.Window
				if len(state.requests) > 0 {
					retryAfter = policy.Window - now.Sub(state.requests[0])
				}
				l.mu.Unlock()
				c.Abort()
				RateLimitResponse(c, retryAfter)
				return
			}
		}

		if state.inFlight >= policy.MaxConcurrent {
			l.mu.Unlock()
			c.Abort()
			ConflictResponse(c, "OPERATION_IN_PROGRESS", "该操作正在进行中，请等待当前操作完成")
			return
		}

		state.inFlight++
		if policy.MaxRequests > 0 && policy.Window > 0 {
			state.requests = append(state.requests, now)
		}
		l.mu.Unlock()

		defer func() {
			l.mu.Lock()
			state.inFlight--
			l.mu.Unlock()
		}()

		c.Next()
	}
}

// RequestBodyLimit rejects a request before a handler parses or persists its
// body. Unknown-length requests are wrapped with MaxBytesReader and handlers
// can detect an over-limit read with RequestBodyLimitExceeded.
func RequestBodyLimit(maxBytes int64) gin.HandlerFunc {
	if maxBytes < 1 {
		maxBytes = DefaultUploadRequestLimitBytes
	}

	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			RequestTooLargeResponse(c, maxBytes)
			c.Abort()
			return
		}

		c.Request.Body = &trackingBody{
			ReadCloser: http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes),
			context:    c,
		}
		c.Next()
	}
}

type trackingBody struct {
	io.ReadCloser
	context *gin.Context
}

func (b *trackingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if _, exceeded := err.(*http.MaxBytesError); exceeded {
		b.context.Set(requestBodyLimitExceededKey, true)
	}
	return n, err
}

// RequestBodyLimitExceeded reports whether a wrapped unknown-length request
// crossed its configured limit while being parsed.
func RequestBodyLimitExceeded(c *gin.Context) bool {
	value, exists := c.Get(requestBodyLimitExceededKey)
	return exists && value == true
}

// RequestTooLargeResponse writes the stable 413 response used by upload and
// other bounded request-body entry points.
func RequestTooLargeResponse(c *gin.Context, maxBytes int64) {
	c.JSON(http.StatusRequestEntityTooLarge, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "REQUEST_TOO_LARGE",
			"message": "请求内容超过大小限制，请缩小后重试",
			"details": gin.H{"max_bytes": maxBytes},
		},
	})
}

// RateLimitResponse writes a stable 429 response and a standard Retry-After
// header. The body includes the same duration for frontend presentation.
func RateLimitResponse(c *gin.Context, retryAfter time.Duration) {
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	seconds := int(math.Ceil(retryAfter.Seconds()))
	c.Header("Retry-After", stringInt(seconds))
	c.JSON(http.StatusTooManyRequests, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "RATE_LIMITED",
			"message": "请求过于频繁，请稍后再试",
			"details": gin.H{"retry_after_seconds": seconds},
		},
	})
}

func stringInt(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 12)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
