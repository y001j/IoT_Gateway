package rules

import (
	"fmt"
	"time"
)

// ErrorType 错误类型
type ErrorType string

const (
	ErrorTypeRule       ErrorType = "rule"
	ErrorTypeCondition  ErrorType = "condition"
	ErrorTypeAction     ErrorType = "action"
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeSystem     ErrorType = "system"
	ErrorTypeNetwork    ErrorType = "network"
	ErrorTypeTimeout    ErrorType = "timeout"
	ErrorTypeConfig     ErrorType = "config"
)

// ErrorLevel 错误级别
type ErrorLevel string

const (
	ErrorLevelInfo    ErrorLevel = "info"
	ErrorLevelWarning ErrorLevel = "warning"
	ErrorLevelError   ErrorLevel = "error"
	ErrorLevelCritical ErrorLevel = "critical"
)

// RuleError 规则引擎专用错误
type RuleError struct {
	Type        ErrorType              `json:"type"`
	Level       ErrorLevel             `json:"level"`
	Code        string                 `json:"code"`
	Message     string                 `json:"message"`
	Details     string                 `json:"details,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Retryable   bool                   `json:"retryable"`
	Cause       error                  `json:"-"`
}

// Error 实现error接口
func (e *RuleError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("[%s:%s] %s: %s", e.Type, e.Level, e.Message, e.Details)
	}
	return fmt.Sprintf("[%s:%s] %s", e.Type, e.Level, e.Message)
}

// Unwrap 支持错误链
func (e *RuleError) Unwrap() error {
	return e.Cause
}

// NewRuleError 创建规则错误
func NewRuleError(errorType ErrorType, level ErrorLevel, code, message string) *RuleError {
	return &RuleError{
		Type:      errorType,
		Level:     level,
		Code:      code,
		Message:   message,
		Timestamp: time.Now(),
		Context:   make(map[string]interface{}),
	}
}

// WithDetails 添加详细信息
func (e *RuleError) WithDetails(details string) *RuleError {
	e.Details = details
	return e
}

// WithContext 添加上下文信息
func (e *RuleError) WithContext(key string, value interface{}) *RuleError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithCause 添加原因错误
func (e *RuleError) WithCause(cause error) *RuleError {
	e.Cause = cause
	return e
}

// SetRetryable 设置是否可重试
func (e *RuleError) SetRetryable(retryable bool) *RuleError {
	e.Retryable = retryable
	return e
}

// 常用错误构造函数

// NewConditionError 创建条件评估错误
func NewConditionError(code, message string, cause error) *RuleError {
	return NewRuleError(ErrorTypeCondition, ErrorLevelError, code, message).
		WithCause(cause).
		SetRetryable(false)
}

// NewActionError 创建动作执行错误
func NewActionError(code, message string, cause error) *RuleError {
	return NewRuleError(ErrorTypeAction, ErrorLevelError, code, message).
		WithCause(cause).
		SetRetryable(true)
}

// NewValidationError 创建验证错误
func NewValidationError(code, message string) *RuleError {
	return NewRuleError(ErrorTypeValidation, ErrorLevelError, code, message).
		SetRetryable(false)
}

// NewSystemError 创建系统错误
func NewSystemError(code, message string, cause error) *RuleError {
	return NewRuleError(ErrorTypeSystem, ErrorLevelCritical, code, message).
		WithCause(cause).
		SetRetryable(true)
}

// NewNetworkError 创建网络错误
func NewNetworkError(code, message string, cause error) *RuleError {
	return NewRuleError(ErrorTypeNetwork, ErrorLevelWarning, code, message).
		WithCause(cause).
		SetRetryable(true)
}

// NewTimeoutError 创建超时错误
func NewTimeoutError(code, message string) *RuleError {
	return NewRuleError(ErrorTypeTimeout, ErrorLevelWarning, code, message).
		SetRetryable(true)
}

// NewConfigError 创建配置错误
func NewConfigError(code, message string) *RuleError {
	return NewRuleError(ErrorTypeConfig, ErrorLevelError, code, message).
		SetRetryable(false)
}

// 错误码常量
const (
	// 条件评估错误码
	ErrCodeConditionParse    = "COND_PARSE"
	ErrCodeConditionEval     = "COND_EVAL"
	ErrCodeConditionType     = "COND_TYPE"
	ErrCodeConditionField    = "COND_FIELD"
	ErrCodeConditionOperator = "COND_OPERATOR"

	// 动作执行错误码
	ErrCodeActionParse     = "ACTION_PARSE"
	ErrCodeActionExec      = "ACTION_EXEC"
	ErrCodeActionType      = "ACTION_TYPE"
	ErrCodeActionConfig    = "ACTION_CONFIG"
	ErrCodeActionTimeout   = "ACTION_TIMEOUT"
	ErrCodeActionTransform = "ACTION_TRANSFORM"
	ErrCodeActionForward   = "ACTION_FORWARD"
	ErrCodeActionAlert     = "ACTION_ALERT"
	ErrCodeActionFilter    = "ACTION_FILTER"
	ErrCodeActionAggregate = "ACTION_AGGREGATE"

	// 规则错误码
	ErrCodeRuleLoad     = "RULE_LOAD"
	ErrCodeRuleParse    = "RULE_PARSE"
	ErrCodeRuleValidate = "RULE_VALIDATE"
	ErrCodeRuleExec     = "RULE_EXEC"

	// 系统错误码
	ErrCodeSystemInit     = "SYS_INIT"
	ErrCodeSystemStart    = "SYS_START"
	ErrCodeSystemStop     = "SYS_STOP"
	ErrCodeSystemResource = "SYS_RESOURCE"

	// 网络错误码
	ErrCodeNetworkConn    = "NET_CONN"
	ErrCodeNetworkTimeout = "NET_TIMEOUT"
	ErrCodeNetworkAuth    = "NET_AUTH"
	ErrCodeNetworkSend    = "NET_SEND"

	// 配置错误码
	ErrCodeConfigFormat   = "CFG_FORMAT"
	ErrCodeConfigMissing  = "CFG_MISSING"
	ErrCodeConfigInvalid  = "CFG_INVALID"
	ErrCodeConfigRequired = "CFG_REQUIRED"
)

// IsRetryableError 检查错误是否可重试
func IsRetryableError(err error) bool {
	if ruleErr, ok := err.(*RuleError); ok {
		return ruleErr.Retryable
	}
	return false
}

// GetErrorType 获取错误类型
func GetErrorType(err error) ErrorType {
	if ruleErr, ok := err.(*RuleError); ok {
		return ruleErr.Type
	}
	return ErrorTypeSystem
}

// GetErrorLevel 获取错误级别
func GetErrorLevel(err error) ErrorLevel {
	if ruleErr, ok := err.(*RuleError); ok {
		return ruleErr.Level
	}
	return ErrorLevelError
}