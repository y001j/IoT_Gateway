package models

import "time"

// Alert 告警信息
type Alert struct {
	ID                   string                 `json:"id"`
	Title                string                 `json:"title"`
	Description          string                 `json:"description"`
	Level                string                 `json:"level"`        // info, warning, error, critical
	Status               string                 `json:"status"`       // active, acknowledged, resolved
	Source               string                 `json:"source"`       // 告警来源
	Data                 map[string]interface{} `json:"data"`         // 告警相关数据
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
	AcknowledgedAt       *time.Time             `json:"acknowledged_at,omitempty"`
	AcknowledgedBy       string                 `json:"acknowledged_by,omitempty"`
	AcknowledgedComment  string                 `json:"acknowledged_comment,omitempty"`
	ResolvedAt           *time.Time             `json:"resolved_at,omitempty"`
	ResolvedBy           string                 `json:"resolved_by,omitempty"`
	ResolvedComment      string                 `json:"resolved_comment,omitempty"`
	
	// 规则引擎相关字段
	RuleID               string            `json:"rule_id,omitempty"`
	RuleName             string            `json:"rule_name,omitempty"`
	DeviceID             string            `json:"device_id,omitempty"`
	Key                  string            `json:"key,omitempty"`
	Value                interface{}       `json:"value,omitempty"`
	Tags                 map[string]string `json:"tags,omitempty"`
	NotificationChannels []string          `json:"notification_channels,omitempty"`
	Priority             int               `json:"priority,omitempty"`
	AutoResolve          bool              `json:"auto_resolve,omitempty"`
}

// AlertListRequest 告警列表请求
type AlertListRequest struct {
	Page      int       `json:"page" form:"page"`
	PageSize  int       `json:"page_size" form:"page_size"`
	Level     string    `json:"level" form:"level"`
	Status    string    `json:"status" form:"status"`
	Source    string    `json:"source" form:"source"`
	Search    string    `json:"search" form:"search"`
	StartTime time.Time `json:"start_time" form:"start_time"`
	EndTime   time.Time `json:"end_time" form:"end_time"`
}

// AlertCreateRequest 创建告警请求
type AlertCreateRequest struct {
	Title       string                 `json:"title" binding:"required"`
	Description string                 `json:"description"`
	Level       string                 `json:"level" binding:"required"`
	Source      string                 `json:"source" binding:"required"`
	Data        map[string]interface{} `json:"data"`
}

// AlertUpdateRequest 更新告警请求
type AlertUpdateRequest struct {
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Level       string                 `json:"level"`
	Status      string                 `json:"status"`
	Data        map[string]interface{} `json:"data"`
}

// AlertStats 告警统计
type AlertStats struct {
	Total        int                 `json:"total"`
	Active       int                 `json:"active"`
	Acknowledged int                 `json:"acknowledged"`
	Resolved     int                 `json:"resolved"`
	ByLevel      map[string]int      `json:"by_level"`
	BySource     map[string]int      `json:"by_source"`
	RecentTrends []AlertTrend        `json:"recent_trends"`
}

// AlertTrend 告警趋势
type AlertTrend struct {
	Date  time.Time `json:"date"`
	Count int       `json:"count"`
}

// AlertRule 告警规则
type AlertRule struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	Description          string           `json:"description"`
	Enabled              bool             `json:"enabled"`
	Level                string           `json:"level"`
	Condition            *AlertCondition  `json:"condition"`
	NotificationChannels []string         `json:"notification_channels"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

// AlertCondition 告警条件
type AlertCondition struct {
	Type     string      `json:"type"`     // threshold, absence, expression
	Field    string      `json:"field"`    // 字段名
	Operator string      `json:"operator"` // gt, lt, eq, ne, contains, etc.
	Value    interface{} `json:"value"`    // 比较值
}

// AlertRuleCreateRequest 创建告警规则请求
type AlertRuleCreateRequest struct {
	Name                 string           `json:"name" binding:"required"`
	Description          string           `json:"description"`
	Enabled              bool             `json:"enabled"`
	Level                string           `json:"level" binding:"required"`
	Condition            *AlertCondition  `json:"condition" binding:"required"`
	NotificationChannels []string         `json:"notification_channels"`
}

// AlertRuleUpdateRequest 更新告警规则请求
type AlertRuleUpdateRequest struct {
	Name                 string           `json:"name"`
	Description          string           `json:"description"`
	Enabled              bool             `json:"enabled"`
	Level                string           `json:"level"`
	Condition            *AlertCondition  `json:"condition"`
	NotificationChannels []string         `json:"notification_channels"`
}

// AlertRuleTestResponse 告警规则测试响应
type AlertRuleTestResponse struct {
	RuleID    string    `json:"rule_id"`
	Triggered bool      `json:"triggered"`
	Message   string    `json:"message"`
	TestedAt  time.Time `json:"tested_at"`
}

// NotificationChannel 通知渠道
type NotificationChannel struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`    // email, webhook, sms, slack, etc.
	Enabled   bool                   `json:"enabled"`
	Config    map[string]interface{} `json:"config"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// NotificationChannelCreateRequest 创建通知渠道请求
type NotificationChannelCreateRequest struct {
	Name    string                 `json:"name" binding:"required"`
	Type    string                 `json:"type" binding:"required"`
	Enabled bool                   `json:"enabled"`
	Config  map[string]interface{} `json:"config"`
}

// NotificationChannelUpdateRequest 更新通知渠道请求
type NotificationChannelUpdateRequest struct {
	Name    string                 `json:"name"`
	Type    string                 `json:"type"`
	Enabled bool                   `json:"enabled"`
	Config  map[string]interface{} `json:"config"`
}