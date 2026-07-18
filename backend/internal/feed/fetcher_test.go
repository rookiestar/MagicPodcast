package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchFeedWithContextDetailedRecordsHTTPAccessOutcome(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.Header().Set("ETag", `"feed-v1"`)
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	feedURL := server.URL + "/feed.xml?access_token=super-secret"
	result, err := NewFetcher(2*time.Second).FetchFeedWithContextDetailed(context.Background(), feedURL)
	if err == nil {
		t.Fatal("expected the HTTP refusal to be returned")
	}
	if result == nil {
		t.Fatal("expected an access result even when the request fails")
	}

	if result.Access.HTTPStatus == nil || *result.Access.HTTPStatus != http.StatusForbidden {
		t.Fatalf("expected HTTP status 403, got %#v", result.Access.HTTPStatus)
	}
	if result.Access.ErrorCategory != ErrorCategoryAccessDenied {
		t.Fatalf("expected access_denied, got %q", result.Access.ErrorCategory)
	}
	if result.Access.TargetDomain != "127.0.0.1" {
		t.Fatalf("expected the target domain to be recorded, got %q", result.Access.TargetDomain)
	}
	if result.Access.ResponseTimeMs < 0 {
		t.Fatalf("expected a non-negative response duration, got %d", result.Access.ResponseTimeMs)
	}
	if result.Access.RetryAfter != "120" || result.Access.ETag != `"feed-v1"` {
		t.Fatalf("expected whitelisted response metadata, got retry_after=%q etag=%q", result.Access.RetryAfter, result.Access.ETag)
	}
	if result.Access.SourceType != AccessSourcePrimary || result.Access.CacheStatus != CacheStatusNotUsed {
		t.Fatalf("unexpected source/cache defaults: source=%q cache=%q", result.Access.SourceType, result.Access.CacheStatus)
	}
	if result.Access.Freshness != FreshnessUnknown || result.Access.EgressID != EgressDirect {
		t.Fatalf("unexpected failure freshness/egress: freshness=%q egress=%q", result.Access.Freshness, result.Access.EgressID)
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("error should not expose query credentials: %v", err)
	}
}

func TestFetchFeedWithContextDetailedRecordsParseOutcomeWithoutBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.Header().Set("Last-Modified", "Sat, 19 Jul 2026 00:00:00 GMT")
		_, _ = w.Write([]byte("not a feed"))
	}))
	t.Cleanup(server.Close)

	result, err := NewFetcher(2*time.Second).FetchFeedWithContextDetailed(context.Background(), server.URL+"/feed.xml")
	if err == nil {
		t.Fatal("expected invalid feed content to fail")
	}
	if result == nil || result.Access.HTTPStatus == nil || *result.Access.HTTPStatus != http.StatusOK {
		t.Fatalf("expected a recorded HTTP 200 response, got %#v", result)
	}
	if result.Access.ErrorCategory != ErrorCategoryParse {
		t.Fatalf("expected parse, got %q", result.Access.ErrorCategory)
	}
	if result.Access.LastModified == "" || result.Access.CacheControl == "" {
		t.Fatalf("expected whitelisted cache metadata, got %#v", result.Access)
	}
	if result.Access.ResponseBytes <= 0 {
		t.Fatalf("expected response bytes to be observable, got %d", result.Access.ResponseBytes)
	}
	if result.Feed != nil {
		t.Fatal("invalid content must not produce a feed")
	}
}
