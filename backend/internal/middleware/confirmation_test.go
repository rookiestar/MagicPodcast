package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireConfirmationTextRejectsMissingOrMismatchedText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, provided := range []string{"", "DELETE WORKFLOW 2"} {
		router := gin.New()
		router.POST("/", func(c *gin.Context) {
			if RequireConfirmationText(c, provided, "DELETE WORKFLOW 1", "删除工作流 1") {
				c.Status(http.StatusNoContent)
			}
		})

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))

		if recorder.Code != http.StatusPreconditionRequired {
			t.Fatalf("provided=%q status=%d, want 428", provided, recorder.Code)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "CONFIRMATION_REQUIRED") ||
			!strings.Contains(body, "DELETE WORKFLOW 1") {
			t.Fatalf("response=%s, want stable confirmation contract", body)
		}
	}
}

func TestRequireConfirmationTextAllowsExactText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/", func(c *gin.Context) {
		if RequireConfirmationText(c, "CLEAR CACHE", "CLEAR CACHE", "清空内存缓存") {
			c.Status(http.StatusNoContent)
		}
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", recorder.Code)
	}
}

func TestConfirmationTextFromHeaderPrecedesForm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/", func(c *gin.Context) {
		if !RequireConfirmationText(c, ConfirmationTextFromHeaderOrForm(c), "SYNC ALL", "同步全部订阅") {
			return
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("confirmation_text=wrong"))
	req.Header.Set("X-MagicPodcast-Confirmation", "SYNC ALL")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", recorder.Code)
	}
}
