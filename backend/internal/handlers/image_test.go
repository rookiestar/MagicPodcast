package handlers

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"magicpodcast/internal/middleware"
)

type imageRoundTripper func(*http.Request) (*http.Response, error)

func (f imageRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newImageTestHandler(response *http.Response) *ImageHandler {
	handler := NewImageHandler()
	handler.httpClient = &http.Client{
		Transport: imageRoundTripper(func(req *http.Request) (*http.Response, error) {
			return response, nil
		}),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return handler
}

func serveImageRequest(handler *ImageHandler, rawURL string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/images/proxy", handler.ProxyImage)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/images/proxy?url="+rawURL, nil)
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestImageHandler_Health(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	imageHandler := NewImageHandler()
	router.GET("/images/health", imageHandler.Health)

	req, _ := http.NewRequest("GET", "/images/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "image-proxy")
}

func TestImageHandlerConfiguresBoundedNetworkTimeouts(t *testing.T) {
	handler := NewImageHandler()
	transport, ok := handler.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("image proxy must use an explicit HTTP transport")
	}

	assert.Equal(t, imageFetchTimeout, handler.httpClient.Timeout)
	assert.Equal(t, imageFetchTimeout, transport.ResponseHeaderTimeout)
	assert.Equal(t, imageDialTimeout, transport.TLSHandshakeTimeout)
}

func TestImageHandler_AllowsReviewedPNGWithBoundedCaching(t *testing.T) {
	response := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"image/png; charset=binary"}},
		Body:          io.NopCloser(bytes.NewReader([]byte("\x89PNG\r\n\x1a\n"))),
		ContentLength: 8,
	}
	recorder := serveImageRequest(
		newImageTestHandler(response),
		"https%3A%2F%2Fi.typlog.com%2Fcover.png",
	)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "public, max-age=86400, stale-while-revalidate=604800", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, []byte("\x89PNG\r\n\x1a\n"), recorder.Body.Bytes())
}

func TestImageHandler_ReusesValidatedImageFromBoundedCache(t *testing.T) {
	var upstreamCalls atomic.Int32
	handler := NewImageHandler()
	handler.httpClient = &http.Client{Transport: imageRoundTripper(func(*http.Request) (*http.Response, error) {
		upstreamCalls.Add(1)
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"image/png"}},
			Body:          io.NopCloser(bytes.NewReader([]byte("\x89PNG\r\n\x1a\n"))),
			ContentLength: 8,
		}, nil
	})}

	first := serveImageRequest(handler, "https%3A%2F%2Fi.typlog.com%2Fcover.png")
	second := serveImageRequest(handler, "https%3A%2F%2Fi.typlog.com%2Fcover.png")

	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, int32(1), upstreamCalls.Load())
}

func TestImageHandler_CoalescesConcurrentValidatedFetches(t *testing.T) {
	var upstreamCalls atomic.Int32
	var releaseOnce sync.Once
	started := make(chan struct{})
	release := make(chan struct{})
	handler := NewImageHandler()
	handler.httpClient = &http.Client{Transport: imageRoundTripper(func(*http.Request) (*http.Response, error) {
		upstreamCalls.Add(1)
		releaseOnce.Do(func() { close(started) })
		<-release
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"image/png"}},
			Body:          io.NopCloser(bytes.NewReader([]byte("\x89PNG\r\n\x1a\n"))),
			ContentLength: 8,
		}, nil
	})}

	results := make(chan *httptest.ResponseRecorder, 2)
	go func() { results <- serveImageRequest(handler, "https%3A%2F%2Fi.typlog.com%2Fcover.png") }()
	<-started
	go func() { results <- serveImageRequest(handler, "https%3A%2F%2Fi.typlog.com%2Fcover.png") }()
	close(release)

	first, second := <-results, <-results
	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, int32(1), upstreamCalls.Load())
}

func TestImageHandler_FollowsOnlyValidatedRedirects(t *testing.T) {
	handler := NewImageHandler()
	handler.httpClient.Transport = imageRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "i.typlog.com" {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"https://image.xyzcdn.net/real.png"}},
				Body:       io.NopCloser(strings.NewReader("redirect")),
			}, nil
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"image/png"}},
			Body:          io.NopCloser(bytes.NewReader([]byte("\x89PNG\r\n\x1a\n"))),
			ContentLength: 8,
		}, nil
	})

	recorder := serveImageRequest(handler, "https%3A%2F%2Fi.typlog.com%2Fcover.png")
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
}

func TestImageHandler_RejectsUnreviewedAndPrivateTargets(t *testing.T) {
	for _, rawURL := range []string{
		"https%3A%2F%2Fevil.example%2Fcover.png",
		"https%3A%2F%2F127.0.0.1%2Fadmin",
		"https%3A%2F%2F%5B%3A%3A1%5D%2Fadmin",
		"https%3A%2F%2Fi.typlog.com%40127.0.0.1%2Fadmin",
		"https%3A%2F%2Fi.typlog.com%3A8443%2Fcover.png",
	} {
		recorder := serveImageRequest(NewImageHandler(), rawURL)
		assert.Equal(t, http.StatusForbidden, recorder.Code, rawURL)
	}
}

func TestImageHandler_RejectsRedirects(t *testing.T) {
	recorder := serveImageRequest(
		newImageTestHandler(&http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"http://127.0.0.1:8080/health"}},
			Body:       io.NopCloser(strings.NewReader("redirect")),
		}),
		"https%3A%2F%2Fi.typlog.com%2Fcover.png",
	)

	assert.Equal(t, http.StatusBadGateway, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "redirects are not allowed")
}

func TestImageHandler_RejectsMissingOrUnsupportedContentType(t *testing.T) {
	for _, contentType := range []string{"", "text/html", "image/svg+xml"} {
		recorder := serveImageRequest(
			newImageTestHandler(&http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader("not an image")),
			}),
			"https%3A%2F%2Fi.typlog.com%2Fcover.png",
		)

		assert.Equal(t, http.StatusUnsupportedMediaType, recorder.Code, contentType)
	}
}

func TestImageHandler_RejectsBodyThatDoesNotMatchDeclaredImageType(t *testing.T) {
	recorder := serveImageRequest(
		newImageTestHandler(&http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(strings.NewReader("this is html")),
		}),
		"https%3A%2F%2Fi.typlog.com%2Fcover.png",
	)

	assert.Equal(t, http.StatusBadGateway, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "does not match image type")
}

func TestImageHandler_RejectsOversizedUnknownLengthResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	imageHandler := NewImageHandler()
	// Use a custom body so net/http cannot rely on Content-Length; the handler
	// must still stop after the configured bounded read.
	imageHandler.httpClient = &http.Client{Transport: imageRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"image/png"}},
			Body:          io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("x"), int(middleware.DefaultImageResponseLimitBytes)+1))),
			ContentLength: -1,
		}, nil
	})}

	recorder := serveImageRequest(imageHandler, "https%3A%2F%2Fi.typlog.com%2Fcover.png")

	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "REQUEST_TOO_LARGE")
}

func TestSafeImageDialContextRejectsPrivateResolvedAddresses(t *testing.T) {
	for _, resolved := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "::1", "fd00::1"} {
		dial := newSafeImageDialContext(
			func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP(resolved)}}, nil
			},
			&net.Dialer{},
		)

		_, err := dial(context.Background(), "tcp", "reviewed.example:443")
		assert.Error(t, err, resolved)
	}
}
