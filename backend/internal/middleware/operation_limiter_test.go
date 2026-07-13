package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }

func TestOperationLimiter_RateLimitsBurst(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	limiter := NewOperationLimiter()
	limiter.nowFunc = clock.now

	state := limiter.stateFor("op", OperationPolicy{MaxConcurrent: 0, MaxRequests: 2, Window: time.Minute})
	state.now = clock.now

	assert.True(t, state.allowRate(), "first request within window")
	assert.True(t, state.allowRate(), "second request within window")
	assert.False(t, state.allowRate(), "third request exceeds window budget")
}

func TestOperationLimiter_QueuesConcurrentRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewOperationLimiter()
	r := gin.New()
	release := make(chan struct{})
	r.GET("/op", limiter.Middleware("op", OperationPolicy{MaxConcurrent: 1, MaxRequests: 100, Window: time.Minute}), func(c *gin.Context) {
		select {
		case <-release:
		case <-c.Request.Context().Done():
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 第一个请求占用唯一槽位。
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/op", nil))
		firstDone <- w
	}()

	// 等待第一个请求进入处理器并持有槽位。
	time.Sleep(20 * time.Millisecond)

	// 第二个请求应排队而非立即 429。
	secondDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/op", nil))
		secondDone <- w
	}()

	select {
	case <-secondDone:
		t.Fatal("second request must queue, not return immediately")
	case <-time.After(30 * time.Millisecond):
	}

	// 释放槽位后，两个请求都应成功。
	close(release)

	select {
	case w := <-secondDone:
		assert.Equal(t, http.StatusOK, w.Code, "queued request should be served after slot frees")
	case <-time.After(time.Second):
		t.Fatal("queued request did not complete after release")
	}

	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first request did not complete after release")
	}
}

func TestOperationLimiter_RejectsRateBurstAtHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewOperationLimiter()
	r := gin.New()
	r.GET("/op", limiter.Middleware("op", OperationPolicy{MaxConcurrent: 0, MaxRequests: 1, Window: time.Minute}), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/op", nil))
	assert.Equal(t, http.StatusOK, w1.Code)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/op", nil))
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)
	assert.Contains(t, w2.Body.String(), operationRateLimitedCode)
}
