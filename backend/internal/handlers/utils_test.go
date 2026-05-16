package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"magicpodcast/internal/handlers"

	"github.com/gin-gonic/gin"
)

func newQueryContext(t *testing.T, target string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req
	return c
}

func TestParsePaginationParamsWithKeys(t *testing.T) {
	c := newQueryContext(t, "/search?page=9&page_size=9&episode_page=3&episode_page_size=40")

	params := handlers.ParsePaginationParamsWithKeys(c, "episode_page", "episode_page_size", 20)

	if params.Page != 3 {
		t.Fatalf("expected page 3, got %d", params.Page)
	}
	if params.PageSize != 40 {
		t.Fatalf("expected page size 40, got %d", params.PageSize)
	}
}

func TestParsePaginationParamsWithKeysBounds(t *testing.T) {
	c := newQueryContext(t, "/search?episode_page=-1&episode_page_size=500")

	params := handlers.ParsePaginationParamsWithKeys(c, "episode_page", "episode_page_size", 20)

	if params.Page != 1 {
		t.Fatalf("expected page to clamp to 1, got %d", params.Page)
	}
	if params.PageSize != 100 {
		t.Fatalf("expected page size to clamp to 100, got %d", params.PageSize)
	}
}
