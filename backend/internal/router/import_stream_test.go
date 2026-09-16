package router

import (
	"bufio"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"magicpodcast/internal/handlers"
)

func TestImportProgressArrivesBeforeWorkCompletes(t *testing.T) {
	router := gin.New()
	router.Use(responseCompression())
	release := make(chan struct{})
	router.POST("/api/v1/sync/import-sse", func(c *gin.Context) {
		reporter := handlers.NewSSEProgressReporter(c)
		defer reporter.Close()
		reporter.Report("first progress")
		<-release
	})
	server := httptest.NewServer(router)
	defer server.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/sync/import-sse", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Explicitly emulate browser compression negotiation, unlike Go's transparent client.
	req.Header.Set("Accept-Encoding", "gzip")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body io.Reader = response.Body
	if response.Header.Get("Content-Encoding") == "gzip" {
		compressed, err := gzip.NewReader(response.Body)
		if err != nil {
			t.Fatalf("cannot read stream: %v", err)
		}
		defer compressed.Close()
		body = compressed
	}
	line, err := bufio.NewReader(body).ReadString('\n')
	if err != nil {
		t.Fatalf("no progress before work finished: %v", err)
	}
	if len(line) < 5 || line[:5] != "data:" {
		t.Fatalf("unexpected event: %q", line)
	}
}
