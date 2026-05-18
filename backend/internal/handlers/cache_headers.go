package handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

func setPrivateCache(c *gin.Context, seconds int) {
	c.Header("Cache-Control", fmt.Sprintf("private, max-age=%d, stale-while-revalidate=%d", seconds, seconds*5))
}

func copyGinH(src gin.H) gin.H {
	dst := make(gin.H, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
