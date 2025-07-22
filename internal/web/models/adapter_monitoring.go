package models

import (
	"time"
)

// AdapterStatus 适配器状态信息
type AdapterStatus struct {
	Name             string                 `json:"name"`              // 适配器名称
	Type             string                 `json:"type"`              // 适配器类型
	Status           string                 `json:"status"`            // 运行状态: "running", "stopped", "error"
	Health           string                 `json:"health"`            // 健康状态: "healthy", "degraded", "unhealthy"
	HealthMessage    string                 `json:"health_message"`    // 健康状态描述
	StartTime        *time.Time             `json:"start_time"`        // 启动时间
	LastDataTime     *time.Time             `json:"last_data_time"`    // 最后数据时间
	ConnectionUptime int64                  `json:"connection_uptime"` // 连接正常运行时间(秒)
	DataPointsCount  int64                  `json:"data_points_count"` // 数据点总数
	ErrorsCount      int64                  `json:"errors_count"`      // 错误总数
	LastError        string                 `json:"last_error"`        // 最后错误信息
	ResponseTimeMS   float64                `json:"response_time_ms"`  // 平均响应时间(毫秒)
	Config           map[string]interface{} `json:"config,omitempty"`  // 配置信息(敏感信息已脱敏)
	Tags             map[string]string      `json:"tags"`              // 标签信息
}

// SinkStatus 输出连接器状态信息
type SinkStatus struct {
	Name              string                 `json:"name"`               // 连接器名称
	Type              string                 `json:"type"`               // 连接器类型
	Status            string                 `json:"status"`             // 运行状态: "running", "stopped", "error"
	Health            string                 `json:"health"`             // 健康状态: "healthy", "degraded", "unhealthy"
	HealthMessage     string                 `json:"health_message"`     // 健康状态描述
	StartTime         *time.Time             `json:"start_time"`         // 启动时间
	LastPublishTime   *time.Time             `json:"last_publish_time"`  // 最后发布时间
	ConnectionUptime  int64                  `json:"connection_uptime"`  // 连接正常运行时间(秒)
	MessagesPublished int64                  `json:"messages_published"` // 发布消息总数
	ErrorsCount       int64                  `json:"errors_count"`       // 错误总数
	LastError         string                 `json:"last_error"`         // 最后错误信息
	ResponseTimeMS    float64                `json:"response_time_ms"`   // 平均响应时间(毫秒)
	Config            map[string]interface{} `json:"config,omitempty"`   // 配置信息(敏感信息已脱敏)
	Tags              map[string]string      `json:"tags"`               // 标签信息
}

// DataFlowMetrics 数据流指标
type DataFlowMetrics struct {
	AdapterName      string      `json:"adapter_name"`        // 适配器名称
	DeviceID         string      `json:"device_id"`           // 设备ID
	Key              string      `json:"key"`                 // 数据点关键字
	DataPointsPerSec float64     `json:"data_points_per_sec"` // 每秒数据点数
	BytesPerSec      float64     `json:"bytes_per_sec"`       // 每秒字节数
	LatencyMS        float64     `json:"latency_ms"`          // 延迟(毫秒)
	ErrorRate        float64     `json:"error_rate"`          // 错误率(0-1)
	LastValue        interface{} `json:"last_value"`          // 最后数据值
	LastTimestamp    time.Time   `json:"last_timestamp"`      // 最后时间戳
}

// ConnectionOverview 连接概览信息
type ConnectionOverview struct {
	TotalAdapters         int               `json:"total_adapters"`            // 总适配器数
	RunningAdapters       int               `json:"running_adapters"`          // 运行中适配器数
	HealthyAdapters       int               `json:"healthy_adapters"`          // 健康适配器数
	TotalSinks            int               `json:"total_sinks"`               // 总连接器数
	RunningSinks          int               `json:"running_sinks"`             // 运行中连接器数
	HealthySinks          int               `json:"healthy_sinks"`             // 健康连接器数
	TotalDataPointsPerSec float64           `json:"total_data_points_per_sec"` // 总数据点/秒
	TotalErrorsPerSec     float64           `json:"total_errors_per_sec"`      // 总错误/秒
	ActiveConnections     int               `json:"active_connections"`        // 活跃连接数
	SystemHealth          string            `json:"system_health"`             // 系统健康状态
	TopAdaptersByTraffic  []DataFlowMetrics `json:"top_adapters_by_traffic"`   // 流量最高的适配器
}

// AdapterDiagnostics 适配器诊断信息
type AdapterDiagnostics struct {
	AdapterName      string                  `json:"adapter_name"`      // 适配器名称
	ConnectionTest   *ConnectionTestResult   `json:"connection_test"`   // 连接测试结果
	ConfigValidation *ConfigValidationResult `json:"config_validation"` // 配置验证结果
	HealthChecks     []HealthCheckResult     `json:"health_checks"`     // 健康检查结果
	PerformanceTest  *PerformanceTestResult  `json:"performance_test"`  // 性能测试结果
	Recommendations  []string                `json:"recommendations"`   // 优化建议
}

// ConnectionTestResult 连接测试结果
type ConnectionTestResult struct {
	Success      bool                   `json:"success"`       // 测试是否成功
	ResponseTime time.Duration          `json:"response_time"` // 响应时间
	Error        string                 `json:"error"`         // 错误信息
	Details      map[string]interface{} `json:"details"`       // 详细信息
	Timestamp    time.Time              `json:"timestamp"`     // 测试时间
}

// ConfigValidationResult 配置验证结果
type ConfigValidationResult struct {
	Valid       bool      `json:"valid"`       // 配置是否有效
	Errors      []string  `json:"errors"`      // 错误列表
	Warnings    []string  `json:"warnings"`    // 警告列表
	Suggestions []string  `json:"suggestions"` // 建议列表
	Timestamp   time.Time `json:"timestamp"`   // 验证时间
}

// HealthCheckResult 健康检查结果
type HealthCheckResult struct {
	CheckName string                 `json:"check_name"` // 检查项名称
	Status    string                 `json:"status"`     // 状态: "pass", "warn", "fail"
	Message   string                 `json:"message"`    // 状态消息
	Duration  time.Duration          `json:"duration"`   // 检查耗时
	Timestamp time.Time              `json:"timestamp"`  // 检查时间
	Details   map[string]interface{} `json:"details"`    // 详细信息
}

// PerformanceTestResult 性能测试结果
type PerformanceTestResult struct {
	ThroughputPerSec float64       `json:"throughput_per_sec"` // 吞吐量/秒
	AvgLatency       time.Duration `json:"avg_latency"`        // 平均延迟
	MaxLatency       time.Duration `json:"max_latency"`        // 最大延迟
	MinLatency       time.Duration `json:"min_latency"`        // 最小延迟
	ErrorRate        float64       `json:"error_rate"`         // 错误率
	TestDuration     time.Duration `json:"test_duration"`      // 测试持续时间
	SampleCount      int           `json:"sample_count"`       // 采样数量
	Timestamp        time.Time     `json:"timestamp"`          // 测试时间
}

// AdapterStatusListResponse 适配器状态列表响应
type AdapterStatusListResponse struct {
	BaseResponse
	Data struct {
		Adapters []AdapterStatus    `json:"adapters"`
		Sinks    []SinkStatus       `json:"sinks"`
		Overview ConnectionOverview `json:"overview"`
	} `json:"data"`
}

// DataFlowMetricsResponse 数据流指标响应
type DataFlowMetricsResponse struct {
	BaseResponse
	Data struct {
		Metrics     []DataFlowMetrics `json:"metrics"`
		TimeRange   string            `json:"time_range"`   // 时间范围
		Granularity string            `json:"granularity"`  // 粒度
		TotalPoints int               `json:"total_points"` // 总数据点数
	} `json:"data"`
}

// AdapterDiagnosticsResponse 适配器诊断响应
type AdapterDiagnosticsResponse struct {
	BaseResponse
	Data AdapterDiagnostics `json:"data"`
}
