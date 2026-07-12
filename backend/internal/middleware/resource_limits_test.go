package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestBodyLimitRejectsKnownOversizeBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	called := false
	router.POST("/", RequestBodyLimit(4), func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345"))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
	if called {
		t.Fatal("handler ran for a request rejected by the content-length limit")
	}
	if !strings.Contains(recorder.Body.String(), "REQUEST_TOO_LARGE") {
		t.Fatalf("response = %s, want stable error code", recorder.Body.String())
	}
}

func TestRequestBodyLimitMarksUnknownLengthOversize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/", RequestBodyLimit(4), func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		if RequestBodyLimitExceeded(c) {
			RequestTooLargeResponse(c, 4)
			return
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", io.NopCloser(strings.NewReader("12345")))
	req.ContentLength = -1
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
}

func TestOperationLimiterRateLimitHasStableResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewOperationLimiter()
	router := gin.New()
	router.GET("/", limiter.Middleware("test", OperationPolicy{
		MaxConcurrent: 1,
		MaxRequests:   1,
		Window:        time.Hour,
	}), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header")
	}
	if !strings.Contains(second.Body.String(), "RATE_LIMITED") {
		t.Fatalf("response = %s, want stable error code", second.Body.String())
	}
}

func TestOperationLimiterRejectsConcurrentOperation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewOperationLimiter()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	router := gin.New()
	router.GET("/", limiter.Middleware("test", OperationPolicy{
		MaxConcurrent: 1,
	}), func(c *gin.Context) {
		once.Do(func() { close(entered) })
		<-release
		c.Status(http.StatusOK)
	})

	firstDone := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		firstDone <- recorder.Code
	}()

	<-entered
	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/", nil))
	if second.Code != http.StatusConflict {
		t.Fatalf("second status = %d, want 409", second.Code)
	}
	if !strings.Contains(second.Body.String(), "OPERATION_IN_PROGRESS") {
		t.Fatalf("response = %s, want stable error code", second.Body.String())
	}

	close(release)
	if code := <-firstDone; code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", code)
	}
}
