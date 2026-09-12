package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"magicpodcast/internal/collection"

	"github.com/gin-gonic/gin"
)

// CollectionHandler 播客单集清单的读取与导入入口。
// 第 1 票只提供导入预览/确认与浏览；收录、刷新、删除由后续票交付。
type CollectionHandler struct {
	service *collection.Service
}

// NewCollectionHandler 创建清单处理器。
func NewCollectionHandler(service *collection.Service) *CollectionHandler {
	return &CollectionHandler{service: service}
}

// Preview POST /api/v1/collections/preview
// body: {"url": "..."}；读取源站清单并绑定服务端预览版本。
func (h *CollectionHandler) Preview(c *gin.Context) {
	var request struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, "INVALID_BODY", "请求体必须是包含 url 的 JSON")
		return
	}
	if strings.TrimSpace(request.URL) == "" {
		badRequest(c, "INVALID_URL", "请提供清单链接")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	result, err := h.service.Preview(ctx, request.URL)
	if err != nil {
		respondCollectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// ConfirmImport POST /api/v1/collections
// body: {"preview_id": "..."}；保存用户实际预览过的清单版本。
func (h *CollectionHandler) ConfirmImport(c *gin.Context) {
	var request struct {
		PreviewID string `json:"preview_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, "INVALID_BODY", "请求体必须是包含 preview_id 的 JSON")
		return
	}
	if strings.TrimSpace(request.PreviewID) == "" {
		badRequest(c, "INVALID_PREVIEW", "缺少预览标识，请重新预览")
		return
	}

	result, err := h.service.ConfirmImport(request.PreviewID)
	if err != nil {
		respondCollectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// List GET /api/v1/collections?search=
func (h *CollectionHandler) List(c *gin.Context) {
	summaries, err := h.service.ListCollections(c.Query("search"))
	if err != nil {
		internalError(c, "DATABASE_ERROR", "读取清单列表失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summaries})
}

// Get GET /api/v1/collections/:id
func (h *CollectionHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		badRequest(c, "INVALID_ID", "清单 ID 必须是正整数")
		return
	}
	detail, err := h.service.GetCollection(uint(id))
	if err != nil {
		if errors.Is(err, collection.ErrCollectionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   gin.H{"code": "COLLECTION_NOT_FOUND", "message": "清单不存在或已被删除"},
			})
			return
		}
		internalError(c, "DATABASE_ERROR", "读取清单详情失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": detail})
}

func respondCollectionError(c *gin.Context, err error) {
	code, status, message := mapCollectionError(err)
	c.JSON(status, gin.H{
		"success": false,
		"error":   gin.H{"code": code, "message": message},
	})
}

// mapCollectionError 把来源读取与解析失败映射为可区分的用户可见结果；
// 不冒充成功，也不把网络失败伪装成空清单。
func mapCollectionError(err error) (code string, status int, message string) {
	switch {
	case errors.Is(err, collection.ErrInvalidCollectionURL):
		return "UNSUPPORTED_SOURCE", http.StatusUnprocessableEntity,
			"仅支持小宇宙单集清单链接（www.xiaoyuzhoufm.com/collection/episode/…）"
	case errors.Is(err, collection.ErrSourceForbidden):
		return "SOURCE_FORBIDDEN", http.StatusForbidden,
			"来源拒绝了读取（可能需要登录或已限制访问），未保存任何清单"
	case errors.Is(err, collection.ErrSourceUnavailable):
		return "SOURCE_UNAVAILABLE", http.StatusBadGateway,
			"来源暂时无法读取，未保存任何清单，可稍后重试"
	case errors.Is(err, collection.ErrSourceTooLarge):
		return "SOURCE_TOO_LARGE", http.StatusUnprocessableEntity,
			"清单页面超过大小限制，已停止读取，未保存任何清单"
	case errors.Is(err, collection.ErrUnsupportedTargetType):
		return "UNSUPPORTED_SOURCE", http.StatusUnprocessableEntity,
			"这份清单不是单集清单，当前仅支持单集清单"
	case errors.Is(err, collection.ErrIncompleteSource):
		return "SOURCE_PARSE_FAILED", http.StatusUnprocessableEntity,
			"清单页面结构不完整，无法完整读取，未保存任何清单"
	case errors.Is(err, collection.ErrDuplicateItems):
		return "SOURCE_PARSE_FAILED", http.StatusUnprocessableEntity,
			"清单内出现重复单集，结构不可信，未保存任何清单"
	case errors.Is(err, collection.ErrEmptyCollection):
		return "EMPTY_COLLECTION", http.StatusUnprocessableEntity,
			"这份清单当前没有单集条目"
	case errors.Is(err, collection.ErrPreviewNotFound):
		return "PREVIEW_NOT_FOUND", http.StatusNotFound,
			"预览不存在，请重新预览后再导入"
	case errors.Is(err, collection.ErrPreviewExpired):
		return "PREVIEW_EXPIRED", http.StatusGone,
			"预览已过期，请重新预览后再导入"
	default:
		return "INTERNAL_ERROR", http.StatusInternalServerError, "处理清单请求失败"
	}
}

func badRequest(c *gin.Context, code, message string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"error":   gin.H{"code": code, "message": message},
	})
}

func internalError(c *gin.Context, code, message string) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"error":   gin.H{"code": code, "message": message},
	})
}
