package southbound

import (
	"context"
	"encoding/json"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

// HealthStatus 健康状态
type HealthStatus struct {
	Status    string    `json:"status"`     // "healthy", "degraded", "unhealthy"
	Message   string    `json:"message"`    // 状态描述
	LastCheck time.Time `json:"last_check"` // 最后检查时间
}

// AdapterMetrics 适配器指标
type AdapterMetrics struct {
	DataPointsCollected int64         `json:"data_points_collected"` // 采集的数据点总数
	ErrorsCount         int64         `json:"errors_count"`          // 错误总数
	LastDataPointTime   time.Time     `json:"last_data_point_time"`  // 最后数据点时间
	ConnectionUptime    time.Duration `json:"connection_uptime"`     // 连接正常运行时间
	LastError           string        `json:"last_error,omitempty"`  // 最后错误信息
	AverageResponseTime float64       `json:"average_response_time"` // 平均响应时间(毫秒)
}

// Adapter 定义了所有设备侧协议驱动必须实现的接口
type Adapter interface {
	// Name 返回适配器的唯一名称
	Name() string

	// Init 初始化适配器，传入JSON格式的配置
	Init(cfg json.RawMessage) error

	// Start 启动适配器，开始采集数据并通过channel发送
	Start(ctx context.Context, ch chan<- model.Point) error

	// Stop 停止适配器，释放资源
	Stop() error
}

// ExtendedAdapter 扩展适配器接口，包含健康检查和指标监控
type ExtendedAdapter interface {
	Adapter

	// Health 返回适配器健康状态
	Health() (HealthStatus, error)

	// GetMetrics 返回适配器运行指标（AdapterMetrics）。签名与 BaseAdapter 及各调用方一致；
	// 以前声明为 (AdapterMetrics, error)，没有任何适配器满足该接口，监控页的详细指标从未生效
	GetMetrics() (interface{}, error)

	// GetLastError 返回最后的错误信息
	GetLastError() error
}

// Config 是适配器配置的基础结构
type Config struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Params     json.RawMessage   `json:"params"`
	SampleRate int               `json:"sample_rate,omitempty"` // 采样率(ms)，默认由各适配器决定
	Tags       map[string]string `json:"tags,omitempty"`        // 附加标签
}

// AdapterFactory 定义了创建适配器实例的工厂函数类型
type AdapterFactory func() Adapter

// Registry 维护所有已注册的适配器工厂
var Registry = make(map[string]AdapterFactory)

// Register 注册一个适配器工厂到全局注册表
func Register(typeName string, factory AdapterFactory) {
	Registry[typeName] = factory
}

// Create 根据类型名创建适配器实例
func Create(typeName string) (Adapter, bool) {
	factory, exists := Registry[typeName]
	if !exists {
		return nil, false
	}
	return factory(), true
}
