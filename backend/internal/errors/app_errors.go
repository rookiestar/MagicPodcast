package errors

import (
	"fmt"
	"net/http"
)

// AppError 应用错误接口
type AppError interface {
	error
	StatusCode() int
	Code() string
	Message() string
	Details() interface{}
}

// BaseError 基础错误结构
type BaseError struct {
	httpStatusCode int
	errCode        string
	errMsg         string
	errDetails     interface{}
}

// Error 实现 error 接口
func (e *BaseError) Error() string {
	return e.errMsg
}

// StatusCode 返回HTTP状态码
func (e *BaseError) StatusCode() int {
	return e.httpStatusCode
}

// Code 返回错误码
func (e *BaseError) Code() string {
	return e.errCode
}

// Message 返回错误消息
func (e *BaseError) Message() string {
	return e.errMsg
}

// Details 返回错误详情
func (e *BaseError) Details() interface{} {
	return e.errDetails
}

// New 创建新的应用错误
func New(statusCode int, code string, message string, details interface{}) AppError {
	return &BaseError{
		httpStatusCode: statusCode,
		errCode:        code,
		errMsg:         message,
		errDetails:     details,
	}
}

// Wrap 包装已有错误
func Wrap(err error, code string, message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusInternalServerError,
		errCode:        code,
		errMsg:         fmt.Sprintf("%s: %v", message, err),
		errDetails:     err.Error(),
	}
}

// ========== 预定义错误类型 ==========

// ValidationError 验证错误
func ValidationError(field string, message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusBadRequest,
		errCode:        "VALIDATION_ERROR",
		errMsg:         fmt.Sprintf("%s %s", field, message),
		errDetails: map[string]interface{}{
			"field":   field,
			"message": message,
		},
	}
}

// ValidationErrorWithDetails 带详情的验证错误
func ValidationErrorWithDetails(details map[string]string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusBadRequest,
		errCode:        "VALIDATION_ERROR",
		errMsg:         "Validation failed",
		errDetails:     details,
	}
}

// NotFoundError 未找到错误
func NotFoundError(resource string, id interface{}) AppError {
	return &BaseError{
		httpStatusCode: http.StatusNotFound,
		errCode:        "NOT_FOUND",
		errMsg:         fmt.Sprintf("%s with id '%v' not found", resource, id),
		errDetails: map[string]interface{}{
			"resource": resource,
			"id":       fmt.Sprintf("%v", id),
		},
	}
}

// ConflictError 冲突错误
func ConflictError(resource string, message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusConflict,
		errCode:        "CONFLICT",
		errMsg:         fmt.Sprintf("%s: %s", resource, message),
		errDetails: map[string]interface{}{
			"resource": resource,
			"message":  message,
		},
	}
}

// UnauthorizedError 未授权错误
func UnauthorizedError(message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusUnauthorized,
		errCode:        "UNAUTHORIZED",
		errMsg:         message,
		errDetails:     nil,
	}
}

// ForbiddenError 禁止访问错误
func ForbiddenError(message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusForbidden,
		errCode:        "FORBIDDEN",
		errMsg:         message,
		errDetails:     nil,
	}
}

// InternalError 内部服务器错误
func InternalError(message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusInternalServerError,
		errCode:        "INTERNAL_ERROR",
		errMsg:         message,
		errDetails:     nil,
	}
}

// InternalErrorWithErr 带原始错误的内部错误
func InternalErrorWithErr(err error, message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusInternalServerError,
		errCode:        "INTERNAL_ERROR",
		errMsg:         fmt.Sprintf("%s: %v", message, err),
		errDetails:     err.Error(),
	}
}

// ServiceUnavailableError 服务不可用错误
func ServiceUnavailableError(message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusServiceUnavailable,
		errCode:        "SERVICE_UNAVAILABLE",
		errMsg:         message,
		errDetails:     nil,
	}
}

// BadRequestError 错误的请求
func BadRequestError(message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusBadRequest,
		errCode:        "BAD_REQUEST",
		errMsg:         message,
		errDetails:     nil,
	}
}

// ========== 业务特定错误 ==========

// InvalidCronExpressionError 无效的Cron表达式
func InvalidCronExpressionError(expression string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusBadRequest,
		errCode:        "INVALID_CRON_EXPRESSION",
		errMsg:         fmt.Sprintf("Invalid cron expression: %s", expression),
		errDetails: map[string]interface{}{
			"expression": expression,
		},
	}
}

// InvalidWorkflowConfigError 无效的工作流配置
func InvalidWorkflowConfigError(message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusBadRequest,
		errCode:        "INVALID_WORKFLOW_CONFIG",
		errMsg:         fmt.Sprintf("Invalid workflow config: %s", message),
		errDetails: map[string]interface{}{
			"message": message,
		},
	}
}

// WorkflowExecutionError 工作流执行错误
func WorkflowExecutionError(workflowID uint, message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusInternalServerError,
		errCode:        "WORKFLOW_EXECUTION_ERROR",
		errMsg:         fmt.Sprintf("Workflow %d execution failed: %s", workflowID, message),
		errDetails: map[string]interface{}{
			"workflow_id": workflowID,
			"message":     message,
		},
	}
}

// SyncError 同步错误
func SyncError(message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusInternalServerError,
		errCode:        "SYNC_ERROR",
		errMsg:         fmt.Sprintf("Sync failed: %s", message),
		errDetails: map[string]interface{}{
			"message": message,
		},
	}
}

// ExternalServiceError 外部服务错误
func ExternalServiceError(service string, message string) AppError {
	return &BaseError{
		httpStatusCode: http.StatusBadGateway,
		errCode:        "EXTERNAL_SERVICE_ERROR",
		errMsg:         fmt.Sprintf("External service '%s' error: %s", service, message),
		errDetails: map[string]interface{}{
			"service": service,
			"message": message,
		},
	}
}
