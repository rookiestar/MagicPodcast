package handlers

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"magicpodcast/internal/logger"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/opml"

	"github.com/gin-gonic/gin"
)

// uploadValidation 描述统一的上传校验失败响应。两个导入入口（普通/SSE）
// 在发送任何 SSE 响应头之前返回同一 JSON 错误结构，保证前后端和入口之间
// 校验结果一致（#398 R11）。
type uploadValidation struct {
	status  int
	code    string
	message string
}

func (v *uploadValidation) respond(c *gin.Context) {
	if v.status == http.StatusRequestEntityTooLarge {
		middleware.RequestTooLargeResponse(c, middleware.MaxOPMLFileBytes)
		return
	}
	c.JSON(v.status, gin.H{
		"success": false,
		"error": gin.H{
			"code":    v.code,
			"message": v.message,
		},
	})
}

// validateOPMLUpload 校验上传文件：扩展名忽略大小写（.opml/.xml），文件
// 内容上限与前端一致（8 MiB），MIME 只作辅助、不参与拦截——伪装 MIME 与
// 空 MIME 的结果完全由扩展名和大小决定。
func validateOPMLUpload(file *multipart.FileHeader) *uploadValidation {
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".opml" && ext != ".xml" {
		return &uploadValidation{
			status:  http.StatusBadRequest,
			code:    "INVALID_FILE_FORMAT",
			message: "OPML文件格式不正确，请上传.opml或.xml文件",
		}
	}
	if file.Size > middleware.MaxOPMLFileBytes {
		return &uploadValidation{status: http.StatusRequestEntityTooLarge}
	}
	return nil
}

// respondOPMLParseError 统一把解析类失败映射为 400，导入过程失败映射为
// 500，两个入口使用同一契约。
func respondOPMLParseError(c *gin.Context, err error) {
	var parseErr *opml.ParseError
	if errors.As(err, &parseErr) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_OPML",
				"message": "OPML文件无法解析，请确认文件为合法的 OPML/XML 内容",
			},
		})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "IMPORT_ERROR",
			"message": fmt.Sprintf("导入失败: %v", err),
		},
	})
}

// saveOPMLUpload 把上传文件写入临时目录并返回路径；调用方负责用返回的
// cleanup 释放临时文件。
func saveOPMLUpload(c *gin.Context, file *multipart.FileHeader) (tempFilePath string, cleanup func(), ok bool) {
	tempDir := filepath.Join(".", "data", "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		logger.Infof("创建临时目录失败: %v", err)
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "创建临时目录失败")
		return "", nil, false
	}

	tempFileName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(file.Filename))
	tempFilePath = filepath.Join(tempDir, tempFileName)
	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		logger.Infof("保存文件失败: %v", err)
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "保存文件失败")
		return "", nil, false
	}

	cleanup = func() {
		if err := os.Remove(tempFilePath); err != nil {
			logger.Infof("⚠️  清理临时OPML文件失败: %v", err)
		}
	}
	return tempFilePath, cleanup, true
}
