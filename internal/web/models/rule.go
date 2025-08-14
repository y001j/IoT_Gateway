package models

import (
	"time"
)

// Rule 规则定义
type Rule struct {
	ID          string            `json:"id"`           // String ID from rule manager
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Enabled     bool              `json:"enabled"`
	Priority    int               `json:"priority"`
	Version     int               `json:"version"`
	DataType    interface{}       `json:"data_type,omitempty"` // 数据类型：字符串或详细定义
	Conditions  *RuleCondition    `json:"conditions"`
	Actions     []RuleAction      `json:"actions"`
	Tags        map[string]string `json:"tags,omitempty"`
	Stats       *RuleStats        `json:"stats,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// RuleCondition 条件定义
type RuleCondition struct {
	Type       string          `json:"type,omitempty"`       // "simple", "expression", "lua"
	Field      string          `json:"field,omitempty"`      // 字段名
	Operator   string          `json:"operator,omitempty"`   // 操作符
	Value      interface{}     `json:"value,omitempty"`      // 比较值
	Expression string          `json:"expression,omitempty"` // 表达式
	Script     string          `json:"script,omitempty"`     // 脚本
	And        []RuleCondition `json:"and,omitempty"`        // AND条件
	Or         []RuleCondition `json:"or,omitempty"`         // OR条件
	Not        *RuleCondition  `json:"not,omitempty"`        // NOT条件
}

// RuleAction 动作定义
type RuleAction struct {
	Type    string                 `json:"type"`    // forward, transform, alert, aggregate, filter
	Config  map[string]interface{} `json:"config"`  // 动作配置
	Async   bool                   `json:"async"`   // 是否异步执行
	Timeout string                 `json:"timeout"` // 超时时间
	Retry   int                    `json:"retry"`   // 重试次数
}

// RuleStats 规则统计
type RuleStats struct {
	MatchedTotal     int64     `json:"matched_total"`     // 总匹配次数
	MatchedHour      int64     `json:"matched_hour"`      // 每小时匹配次数
	ActionsTotal     int64     `json:"actions_total"`     // 总动作执行次数
	ActionsSucceeded int64     `json:"actions_succeeded"` // 成功的动作次数
	ActionsFailed    int64     `json:"actions_failed"`    // 失败的动作次数
	LastMatched      time.Time `json:"last_matched"`      // 最后匹配时间
	LastSuccess      time.Time `json:"last_success"`      // 最后成功时间
	LastError        string    `json:"last_error"`        // 最后错误信息
	AverageLatency   float64   `json:"average_latency"`   // 平均延迟
}

// RuleListRequest 规则列表请求
type RuleListRequest struct {
	Page     int    `form:"page" binding:"required,min=1"`
	PageSize int    `form:"page_size" binding:"required,min=1,max=100"`
	Type     string `form:"type"`
	Status   string `form:"status"`
	Search   string `form:"search"`
}

// RuleCreateRequest 创建规则请求
type RuleCreateRequest struct {
	Name        string            `json:"name" binding:"required"`
	Description string            `json:"description"`
	Priority    int               `json:"priority"`
	Conditions  *RuleCondition    `json:"conditions" binding:"required"`
	Actions     []RuleAction      `json:"actions" binding:"required,min=1"`
	Tags        map[string]string `json:"tags"`
	Enabled     bool              `json:"enabled"`
}

// RuleUpdateRequest 更新规则请求
type RuleUpdateRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Priority    int               `json:"priority"`
	Conditions  *RuleCondition    `json:"conditions"`
	Actions     []RuleAction      `json:"actions"`
	Tags        map[string]string `json:"tags"`
	Enabled     *bool             `json:"enabled"`
}

// RuleValidationResponse 规则验证响应
type RuleValidationResponse struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

// RuleOperationResponse 规则操作响应
type RuleOperationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// RuleTestRequest 规则测试请求
type RuleTestRequest struct {
	Rule  *Rule                  `json:"rule" binding:"required"`
	Point map[string]interface{} `json:"point" binding:"required"`
}

// RuleTestResponse 规则测试响应
type RuleTestResponse struct {
	Matched  bool                   `json:"matched"`
	Actions  []RuleActionTestResult `json:"actions"`
	Duration int64                  `json:"duration"`
}

// RuleActionTestResult 规则动作测试结果
type RuleActionTestResult struct {
	Type     string        `json:"type"`
	Success  bool          `json:"success"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
	Output   interface{}   `json:"output,omitempty"`
}

// Additional missing types for rule service

// Condition is an alias for RuleCondition for backwards compatibility
type Condition = RuleCondition

// Action is an alias for RuleAction for backwards compatibility  
type Action = RuleAction

// RuleHistoryRequest 规则执行历史请求
type RuleHistoryRequest struct {
	Page      int       `form:"page" binding:"required,min=1"`
	PageSize  int       `form:"page_size" binding:"required,min=1,max=100"`
	StartTime time.Time `form:"start_time"`
	EndTime   time.Time `form:"end_time"`
	Status    string    `form:"status"` // success, error
}

// RuleExecution 规则执行记录
type RuleExecution struct {
	ID         string                 `json:"id"`
	RuleID     string                 `json:"rule_id"`
	ExecutedAt time.Time              `json:"executed_at"`
	Success    bool                   `json:"success"`
	Duration   time.Duration          `json:"duration"`
	InputData  map[string]interface{} `json:"input_data"`
	OutputData map[string]interface{} `json:"output_data"`
	Error      string                 `json:"error,omitempty"`
}

// RuleTemplate 规则模板
type RuleTemplate struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Tags        []string               `json:"tags"`
	Template    map[string]interface{} `json:"template"`
}

// RuleFromTemplateRequest 从模板创建规则请求
type RuleFromTemplateRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Enabled     bool                   `json:"enabled"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Updated RuleStats to match service usage
type RuleStatsExtended struct {
	RuleID                string                 `json:"rule_id"`
	RuleName              string                 `json:"rule_name"`
	ExecutionCount        int                    `json:"execution_count"`
	SuccessCount          int                    `json:"success_count"`
	ErrorCount            int                    `json:"error_count"`
	AverageExecutionTime  time.Duration          `json:"average_execution_time"`
	LastExecuted          time.Time              `json:"last_executed"`
	Status                string                 `json:"status"`
	TriggerRate           float64                `json:"trigger_rate"`
	ActionsTriggered      map[string]int         `json:"actions_triggered"`
}

// Updated RuleTestRequest to match service usage
type RuleTestRequestExtended struct {
	Rule     Rule                     `json:"rule" binding:"required"`
	TestData []map[string]interface{} `json:"test_data" binding:"required"`
}

// Updated RuleTestResponse to match service usage
type RuleTestResponseExtended struct {
	Success    bool             `json:"success"`
	Results    []RuleTestResult `json:"results"`
	Errors     []string         `json:"errors"`
	ExecutedAt time.Time        `json:"executed_at"`
}

// RuleTestResult 规则测试结果
type RuleTestResult struct {
	TestCaseIndex int                    `json:"test_case_index"`
	Input         map[string]interface{} `json:"input"`
	Matched       bool                   `json:"matched"`
	Actions       []string               `json:"actions"`
	Error         string                 `json:"error,omitempty"`
}

// RuleValidationResponseExtended 扩展的规则验证响应
type RuleValidationResponseExtended struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}
