package errors

import (
	"net/http"
	"testing"
)

func TestBaseError(t *testing.T) {
	tests := []struct {
		name       string
		error      AppError
		wantCode   string
		wantStatus int
		wantMsg    string
	}{
		{
			name: "Validation Error",
			error: ValidationError("email", "is required"),
			wantCode: "VALIDATION_ERROR",
			wantStatus: http.StatusBadRequest,
			wantMsg: "email is required",
		},
		{
			name: "Not Found Error",
			error: NotFoundError("podcast", 123),
			wantCode: "NOT_FOUND",
			wantStatus: http.StatusNotFound,
			wantMsg: "podcast with id '123' not found",
		},
		{
			name: "Conflict Error",
			error: ConflictError("tag", "already exists"),
			wantCode: "CONFLICT",
			wantStatus: http.StatusConflict,
			wantMsg: "tag: already exists",
		},
		{
			name: "Internal Error",
			error: InternalError("database connection failed"),
			wantCode: "INTERNAL_ERROR",
			wantStatus: http.StatusInternalServerError,
			wantMsg: "database connection failed",
		},
		{
			name: "Invalid Cron Expression",
			error: InvalidCronExpressionError("invalid"),
			wantCode: "INVALID_CRON_EXPRESSION",
			wantStatus: http.StatusBadRequest,
			wantMsg: "Invalid cron expression: invalid",
		},
		{
			name: "Workflow Execution Error",
			error: WorkflowExecutionError(5, "timeout"),
			wantCode: "WORKFLOW_EXECUTION_ERROR",
			wantStatus: http.StatusInternalServerError,
			wantMsg: "Workflow 5 execution failed: timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.error.Code() != tt.wantCode {
				t.Errorf("Code() = %v, want %v", tt.error.Code(), tt.wantCode)
			}
			if tt.error.StatusCode() != tt.wantStatus {
				t.Errorf("StatusCode() = %v, want %v", tt.error.StatusCode(), tt.wantStatus)
			}
			if tt.error.Message() != tt.wantMsg {
				t.Errorf("Message() = %v, want %v", tt.error.Message(), tt.wantMsg)
			}
			if tt.error.Error() != tt.wantMsg {
				t.Errorf("Error() = %v, want %v", tt.error.Error(), tt.wantMsg)
			}
		})
	}
}

func TestValidationErrorWithDetails(t *testing.T) {
	details := map[string]string{
		"title": "is required",
		"url":   "must be a valid URL",
	}
	err := ValidationErrorWithDetails(details)

	if err.Code() != "VALIDATION_ERROR" {
		t.Errorf("Code() = %v, want %v", err.Code(), "VALIDATION_ERROR")
	}
	if err.StatusCode() != http.StatusBadRequest {
		t.Errorf("StatusCode() = %v, want %v", err.StatusCode(), http.StatusBadRequest)
	}

	// 检查 details
	errDetails, ok := err.Details().(map[string]string)
	if !ok {
		t.Fatalf("Details() is not map[string]string")
	}

	if errDetails["title"] != "is required" {
		t.Errorf("Details()[title] = %v, want %v", errDetails["title"], "is required")
	}
	if errDetails["url"] != "must be a valid URL" {
		t.Errorf("Details()[url] = %v, want %v", errDetails["url"], "must be a valid URL")
	}
}

func TestNew(t *testing.T) {
	customErr := New(http.StatusTeapot, "CUSTOM_ERROR", "I'm a teapot", nil)

	if customErr.Code() != "CUSTOM_ERROR" {
		t.Errorf("Code() = %v, want %v", customErr.Code(), "CUSTOM_ERROR")
	}
	if customErr.StatusCode() != http.StatusTeapot {
		t.Errorf("StatusCode() = %v, want %v", customErr.StatusCode(), http.StatusTeapot)
	}
	if customErr.Message() != "I'm a teapot" {
		t.Errorf("Message() = %v, want %v", customErr.Message(), "I'm a teapot")
	}
}

func TestExternalServiceError(t *testing.T) {
	err := ExternalServiceError("xyz-api", "connection refused")

	if err.Code() != "EXTERNAL_SERVICE_ERROR" {
		t.Errorf("Code() = %v, want %v", err.Code(), "EXTERNAL_SERVICE_ERROR")
	}
	if err.StatusCode() != http.StatusBadGateway {
		t.Errorf("StatusCode() = %v, want %v", err.StatusCode(), http.StatusBadGateway)
	}

	// 检查 details
	errDetails, ok := err.Details().(map[string]interface{})
	if !ok {
		t.Fatalf("Details() is not map[string]interface{}")
	}

	if errDetails["service"] != "xyz-api" {
		t.Errorf("Details()[service] = %v, want %v", errDetails["service"], "xyz-api")
	}
	if errDetails["message"] != "connection refused" {
		t.Errorf("Details()[message] = %v, want %v", errDetails["message"], "connection refused")
	}
}

func TestUnauthorizedAndForbidden(t *testing.T) {
	t.Run("Unauthorized", func(t *testing.T) {
		err := UnauthorizedError("Authentication required")
		if err.Code() != "UNAUTHORIZED" {
			t.Errorf("Code() = %v, want %v", err.Code(), "UNAUTHORIZED")
		}
		if err.StatusCode() != http.StatusUnauthorized {
			t.Errorf("StatusCode() = %v, want %v", err.StatusCode(), http.StatusUnauthorized)
		}
	})

	t.Run("Forbidden", func(t *testing.T) {
		err := ForbiddenError("Access denied")
		if err.Code() != "FORBIDDEN" {
			t.Errorf("Code() = %v, want %v", err.Code(), "FORBIDDEN")
		}
		if err.StatusCode() != http.StatusForbidden {
			t.Errorf("StatusCode() = %v, want %v", err.StatusCode(), http.StatusForbidden)
		}
	})
}
