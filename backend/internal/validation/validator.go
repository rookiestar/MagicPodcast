package validation

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"unicode/utf8"
)

var (
	// URL验证正则（支持http/https）
	urlRegex = regexp.MustCompile(`^https?://[a-zA-Z0-9\-._~:/?#\[\]@!$&'()*+,;=]+$`)
	// Feed URL正则（更宽松，支持各种feed格式）
	feedURLRegex = regexp.MustCompile(`^https?://.+\.(xml|rss|atom|json|opml)$|^https?://.+/.+$`)
)

// ValidationError 验证错误
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error 实现error接口
func (ve ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", ve.Field, ve.Message)
}

// Validator 验证器
type Validator struct {
	errors []ValidationError
}

// New 创建新的验证器
func New() *Validator {
	return &Validator{
		errors: make([]ValidationError, 0),
	}
}

// ValidateURL 验证URL格式
func (v *Validator) ValidateURL(field, value string) *Validator {
	if value == "" {
		return v
	}

	if !urlRegex.MatchString(value) {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "无效的URL格式，必须以http://或https://开头",
		})
		return v
	}

	// 尝试解析URL
	if _, err := url.Parse(value); err != nil {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "URL解析失败",
		})
	}

	return v
}

// ValidateFeedURL 验证Feed URL格式
func (v *Validator) ValidateFeedURL(field, value string) *Validator {
	if value == "" {
		return v
	}

	if !feedURLRegex.MatchString(value) {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "无效的Feed URL格式",
		})
	}

	return v
}

// ValidateRequired 验证必填字段
func (v *Validator) ValidateRequired(field, value string) *Validator {
	if value == "" {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "不能为空",
		})
	}
	return v
}

// ValidateStringLength 验证字符串长度
func (v *Validator) ValidateStringLength(field, value string, min, max int) *Validator {
	length := utf8.RuneCountInString(value)
	if length < min {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: fmt.Sprintf("长度不能少于%d个字符", min),
		})
	} else if length > max {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: fmt.Sprintf("长度不能超过%d个字符", max),
		})
	}
	return v
}

// ValidateIntRange 验证整数范围
func (v *Validator) ValidateIntRange(field string, value int64, min, max int64) *Validator {
	if value < min || value > max {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: fmt.Sprintf("必须在%d到%d之间", min, max),
		})
	}
	return v
}

// ValidateUint 验证无符号整数（用于ID等）
func (v *Validator) ValidateUint(field, value string) *Validator {
	if value == "" {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "不能为空",
		})
		return v
	}

	if _, err := strconv.ParseUint(value, 10, 64); err != nil {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "必须是有效的正整数",
		})
	}

	return v
}

// ValidateEmail 验证邮箱格式（基础验证）
func (v *Validator) ValidateEmail(field, value string) *Validator {
	if value == "" {
		return v
	}

	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(value) {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "无效的邮箱格式",
		})
	}

	return v
}

// ValidateEnum 验证枚举值
func (v *Validator) ValidateEnum(field, value string, allowedValues []string) *Validator {
	if value == "" {
		return v
	}

	for _, allowed := range allowedValues {
		if value == allowed {
			return v
		}
	}

	v.errors = append(v.errors, ValidationError{
		Field:   field,
		Message: fmt.Sprintf("必须是以下值之一: %v", allowedValues),
	})

	return v
}

// HasErrors 检查是否有验证错误
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors 获取所有验证错误
func (v *Validator) Errors() []ValidationError {
	return v.errors
}

// Clear 清空验证错误
func (v *Validator) Clear() *Validator {
	v.errors = make([]ValidationError, 0)
	return v
}

// Error 实现error接口
func (v *Validator) Error() string {
	if len(v.errors) == 0 {
		return ""
	}
	return v.errors[0].Error()
}
