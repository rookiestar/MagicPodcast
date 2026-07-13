package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ConfirmationRequest is shared by high-risk JSON endpoints. The confirmation
// is an explicit user-intent contract, not a second authentication mechanism.
type ConfirmationRequest struct {
	ConfirmationText string `json:"confirmation_text" form:"confirmation_text"`
}

// ConfirmationTextFromHeaderOrForm reads the fast-path header used by multipart
// requests and falls back to a form field for clients that cannot set headers.
func ConfirmationTextFromHeaderOrForm(c *gin.Context) string {
	if value := strings.TrimSpace(c.GetHeader("X-MagicPodcast-Confirmation")); value != "" {
		return value
	}
	return strings.TrimSpace(c.PostForm("confirmation_text"))
}

// RequireConfirmationText rejects an operation until the client sends the
// exact short phrase shown in the confirmation prompt.
func RequireConfirmationText(c *gin.Context, provided, expected, impact string) bool {
	if strings.TrimSpace(provided) == expected {
		return true
	}

	c.Abort()
	c.JSON(http.StatusPreconditionRequired, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "CONFIRMATION_REQUIRED",
			"message": "请先查看影响范围并完成二次确认",
			"details": gin.H{
				"action":        expected,
				"expected_text": expected,
				"impact":        impact,
			},
		},
	})
	return false
}
