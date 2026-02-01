package middleware

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	apperrors "magicpodcast/internal/errors"
	"magicpodcast/internal/logger"

	"github.com/gin-gonic/gin"
)

// ErrorDetail 错误详情
type ErrorDetail struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// APIErrorResponse 统一的API错误响应格式
type APIErrorResponse struct {
	Success bool                   `json:"success"`
	Error   ErrorDetail            `json:"error,omitempty"`
	Request map[string]interface{} `json:"request,omitempty"`
}

// ErrorHandlerMiddleware 错误处理中间件
func ErrorHandlerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// 如果没有错误，直接返回
		if len(c.Errors) == 0 {
			return
		}

		// 获取第一个错误
		err := c.Errors.Last().Err

		// 处理不同类型的错误
		var appErr apperrors.AppError
		var statusCode int
		var response APIErrorResponse

		// 尝试转换为 AppError
		if errors.As(err, &appErr) {
			statusCode = appErr.StatusCode()
			response = APIErrorResponse{
				Success: false,
				Error: ErrorDetail{
					Code:    appErr.Code(),
					Message: appErr.Message(),
					Details: appErr.Details(),
				},
			}
		} else {
			// 未知错误，返回500
			statusCode = http.StatusInternalServerError
			response = APIErrorResponse{
				Success: false,
				Error: ErrorDetail{
					Code:    "INTERNAL_ERROR",
					Message: "An unexpected error occurred",
					Details: err.Error(),
				},
			}
		}

		// 添加请求信息（仅在开发环境）
		if gin.Mode() == gin.DebugMode {
			response.Request = map[string]interface{}{
				"method": c.Request.Method,
				"path":   c.Request.URL.Path,
				"query":  c.Request.URL.RawQuery,
			}
		}

		// 记录错误日志
		logError(c, statusCode, response)

		// 返回JSON响应
		c.JSON(statusCode, response)
		c.Abort()
	}
}

// SuccessResponse 成功响应辅助函数
func SuccessResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// SuccessResponseWithMessage 带消息的成功响应
func SuccessResponseWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
		"data":    data,
	})
}

// CreatedResponse 创建成功响应（201）
func CreatedResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    data,
	})
}

// NoContentResponse 无内容响应（204）
func NoContentResponse(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// HandleError 直接返回错误响应
func HandleError(c *gin.Context, err error) {
	// 如果是 AppError，使用中间件处理
	var appErr apperrors.AppError
	if errors.As(err, &appErr) {
		c.Error(appErr)
		return
	}

	// 其他错误包装为内部错误
	c.Error(apperrors.InternalErrorWithErr(err, "Internal server error"))
}

// logError 记录错误日志
func logError(c *gin.Context, statusCode int, response APIErrorResponse) {
	// 对于4xx错误，记录为警告级别
	// 对于5xx错误，记录为错误级别
	logFields := map[string]interface{}{
		"method":       c.Request.Method,
		"path":         c.Request.URL.Path,
		"status":       statusCode,
		"error_code":   response.Error.Code,
		"error_message": response.Error.Message,
	}

	if statusCode >= 400 && statusCode < 500 {
		logger.WithFields(logFields).Warn("Request failed with client error")
	} else if statusCode >= 500 {
		if response.Error.Details != nil {
			logFields["error_details"] = response.Error.Details
		}
		logger.WithFields(logFields).Error("Request failed with server error")
	}

	// 同时打印到控制台（开发环境）
	if gin.Mode() == gin.DebugMode {
		log.Printf("[ERROR] %s %s - %d - %s: %s",
			c.Request.Method,
			c.Request.URL.Path,
			statusCode,
			response.Error.Code,
			response.Error.Message,
		)

		// 如果有详情，打印详情
		if response.Error.Details != nil {
			detailsJSON, _ := json.MarshalIndent(response.Error.Details, "", "  ")
			log.Printf("[ERROR] Details: %s", string(detailsJSON))
		}
	}
}

// ValidationErrorResponse 验证错误响应
func ValidationErrorResponse(c *gin.Context, field string, message string) {
	HandleError(c, apperrors.ValidationError(field, message))
}

// NotFoundErrorResponse 未找到错误响应
func NotFoundErrorResponse(c *gin.Context, resource string, id interface{}) {
	HandleError(c, apperrors.NotFoundError(resource, id))
}

// InternalErrorResponse 内部错误响应
func InternalErrorResponse(c *gin.Context, message string) {
	HandleError(c, apperrors.InternalError(message))
}
