package models

import "time"

// Plugin 插件模型
type Plugin struct {
	BaseModel
	Name        string                 `json:"name"`
	Type        string                 `json:"type"` // adapter, sink
	Version     string                 `json:"version"`
	Status      string                 `json:"status"` // running, stopped, error
	Description string                 `json:"description"`
	Author      string                 `json:"author"`
	Config      map[string]interface{} `json:"config"`
	Enabled     bool                   `json:"enabled"`
	Path        string                 `json:"path"`
	Port        int                    `json:"port"`
	LastStart   *time.Time             `json:"last_start,omitempty"`
	LastStop    *time.Time             `json:"last_stop,omitempty"`
	ErrorCount  int                    `json:"error_count"`
}

// PluginListRequest 插件列表请求
type PluginListRequest struct {
	Page     int    `form:"page" binding:"required,min=1"`
	PageSize int    `form:"page_size" binding:"required,min=1,max=100"`
	Type     string `form:"type"`
	Status   string `form:"status"`
	Search   string `form:"search"`
}

// PluginLogRequest 插件日志请求
type PluginLogRequest struct {
	Page      int       `form:"page" binding:"required,min=1"`
	PageSize  int       `form:"page_size" binding:"required,min=1,max=100"`
	Level     string    `form:"level"`
	Source    string    `form:"source"`
	From      string    `form:"from"`
	To        string    `form:"to"`
	StartTime time.Time `form:"start_time"`
	EndTime   time.Time `form:"end_time"`
	Search    string    `form:"search"`
}

// PluginLog 插件日志
type PluginLog struct {
	ID        int64     `json:"id"`
	PluginID  int       `json:"plugin_id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
}

// PluginStats 插件统计
type PluginStats struct {
	PluginID        int       `json:"plugin_id"`
	DataPointsTotal int64     `json:"data_points_total"`
	DataPointsHour  int64     `json:"data_points_hour"`
	ErrorsTotal     int64     `json:"errors_total"`
	ErrorsHour      int64     `json:"errors_hour"`
	UptimeSeconds   int64     `json:"uptime_seconds"`
	AverageLatency  float64   `json:"average_latency"`
	MemoryUsage     int64     `json:"memory_usage"`
	CPUUsage        float64   `json:"cpu_usage"`
	LastUpdate      time.Time `json:"last_update"`
}

// PluginConfigRequest 插件配置请求
type PluginConfigRequest struct {
	Config map[string]interface{} `json:"config" binding:"required"`
}

// PluginConfigValidationResponse 插件配置验证响应
type PluginConfigValidationResponse struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// PluginOperationResponse 插件操作响应
type PluginOperationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// PluginOperationRequest 插件操作请求
type PluginOperationRequest struct {
	Action string `json:"action" binding:"required,oneof=start stop restart reload"`
}

// PluginInstallRequest 插件安装请求
type PluginInstallRequest struct {
	Name      string `json:"name" binding:"required"`
	Source    string `json:"source" binding:"required"`
	Version   string `json:"version"`
	AutoStart bool   `json:"auto_start"`
}

// PluginDependency 插件依赖
type PluginDependency struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
	Status  string `json:"status"`
}

// PluginMetadata 插件元数据
type PluginMetadata struct {
	Name         string             `json:"name"`
	Version      string             `json:"version"`
	Description  string             `json:"description"`
	Author       string             `json:"author"`
	License      string             `json:"license"`
	Homepage     string             `json:"homepage"`
	Repository   string             `json:"repository"`
	Keywords     []string           `json:"keywords"`
	Dependencies []PluginDependency `json:"dependencies"`
	MinVersion   string             `json:"min_version"`
	MaxVersion   string             `json:"max_version"`
}

// PluginCreateRequest 创建插件请求
type PluginCreateRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Type        string                 `json:"type" binding:"required"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config"`
	Enabled     bool                   `json:"enabled"`
	AutoStart   bool                   `json:"auto_start"`
	Path        string                 `json:"path" binding:"required"`
}

// PluginUpdateRequest 更新插件请求
type PluginUpdateRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config"`
	Enabled     *bool                  `json:"enabled"`
	AutoStart   *bool                  `json:"auto_start"`
}

// PluginConfigValidationRequest 插件配置验证请求
type PluginConfigValidationRequest struct {
	Config map[string]interface{} `json:"config" binding:"required"`
}

// PluginError 插件错误
type PluginError struct {
	PluginID   string    `json:"plugin_id"`
	PluginName string    `json:"plugin_name"`
	Error      string    `json:"error"`
	Timestamp  time.Time `json:"timestamp"`
	Count      int       `json:"count"`
}

// PluginHealthCheck 插件健康检查
type PluginHealthCheck struct {
	PluginID     string                 `json:"plugin_id"`
	Healthy      bool                   `json:"healthy"`
	LastCheck    time.Time              `json:"last_check"`
	ResponseTime int64                  `json:"response_time"` // 毫秒
	Error        string                 `json:"error,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
}
