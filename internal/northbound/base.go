package northbound

import (
	//"context"

	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	//"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
)

// SinkStats 定义了连接器的统计信息
type SinkStats struct {
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	Running        bool      `json:"running"`
	MessagesTotal  int64     `json:"messages_total"`
	MessagesFailed int64     `json:"messages_failed"`
	LastError      string    `json:"last_error,omitempty"`
	LastMessage    time.Time `json:"last_message"`
}

// SinkHealthStatus 连接器健康状态
type SinkHealthStatus struct {
	Status    string    `json:"status"`     // "healthy", "degraded", "unhealthy"
	Message   string    `json:"message"`    // 状态描述
	LastCheck time.Time `json:"last_check"` // 最后检查时间
}

// SinkMetrics 连接器指标
type SinkMetrics struct {
	MessagesPublished   int64         `json:"messages_published"`   // 发布的消息总数
	ErrorsCount         int64         `json:"errors_count"`         // 错误总数
	LastPublishTime     time.Time     `json:"last_publish_time"`    // 最后发布时间
	ConnectionUptime    time.Duration `json:"connection_uptime"`    // 连接正常运行时间
	LastError           string        `json:"last_error,omitempty"` // 最后错误信息
	AverageResponseTime float64       `json:"average_response_time"` // 平均响应时间(毫秒)
}

// StandardConfig 是所有连接器的标准配置结构
type StandardConfig struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	BatchSize  int               `json:"batch_size,omitempty"`
	BufferSize int               `json:"buffer_size,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	Params     json.RawMessage   `json:"params"` // 连接器特定的参数
}

// BaseSink 提供了所有连接器的基础实现
type BaseSink struct {
	name       string
	sinkType   string
	running    int32 // 使用atomic操作
	stats      SinkStats
	statsMutex sync.RWMutex
	tags       map[string]string
	batchSize  int
	bufferSize int
}

// NewBaseSink 创建一个新的基础连接器
func NewBaseSink(sinkType string) *BaseSink {
	return &BaseSink{
		sinkType:   sinkType,
		batchSize:  10,   // 默认批处理大小
		bufferSize: 1000, // 默认缓冲区大小
		stats: SinkStats{
			Type: sinkType,
		},
	}
}

// Name 返回连接器名称
func (b *BaseSink) Name() string {
	return b.name
}

// IsRunning 检查连接器是否正在运行
func (b *BaseSink) IsRunning() bool {
	return atomic.LoadInt32(&b.running) == 1
}

// SetRunning 设置连接器运行状态
func (b *BaseSink) SetRunning(running bool) {
	if running {
		atomic.StoreInt32(&b.running, 1)
	} else {
		atomic.StoreInt32(&b.running, 0)
	}

	b.statsMutex.Lock()
	b.stats.Running = running
	b.statsMutex.Unlock()
}

// GetStats 获取连接器统计信息
func (b *BaseSink) GetStats() SinkStats {
	b.statsMutex.RLock()
	defer b.statsMutex.RUnlock()

	stats := b.stats
	stats.Running = b.IsRunning()
	return stats
}

// IncrementMessageCount 增加消息计数
func (b *BaseSink) IncrementMessageCount() {
	atomic.AddInt64(&b.stats.MessagesTotal, 1)
	b.statsMutex.Lock()
	b.stats.LastMessage = time.Now()
	b.statsMutex.Unlock()
}

// IncrementFailedCount 增加失败计数
func (b *BaseSink) IncrementFailedCount() {
	atomic.AddInt64(&b.stats.MessagesFailed, 1)
}

// SetLastError 设置最后一个错误
func (b *BaseSink) SetLastError(err error) {
	if err == nil {
		return
	}

	b.statsMutex.Lock()
	b.stats.LastError = err.Error()
	b.statsMutex.Unlock()
}

// HandleError 统一错误处理
func (b *BaseSink) HandleError(err error, context string) {
	if err == nil {
		return
	}

	b.IncrementFailedCount()
	b.SetLastError(err)

	log.Error().
		Err(err).
		Str("sink", b.name).
		Str("type", b.sinkType).
		Str("context", context).
		Msg("连接器操作失败")
}

// ParseStandardConfig 解析标准配置
func (b *BaseSink) ParseStandardConfig(cfg json.RawMessage) (*StandardConfig, error) {
	var config StandardConfig
	if err := json.Unmarshal(cfg, &config); err != nil {
		return nil, err
	}

	// 设置基础属性
	b.name = config.Name
	b.tags = config.Tags

	if config.BatchSize > 0 {
		b.batchSize = config.BatchSize
	}
	if config.BufferSize > 0 {
		b.bufferSize = config.BufferSize
	}

	// 更新统计信息
	b.statsMutex.Lock()
	b.stats.Name = b.name
	b.stats.Type = b.sinkType
	b.statsMutex.Unlock()

	return &config, nil
}

// GetBatchSize 获取批处理大小
func (b *BaseSink) GetBatchSize() int {
	return b.batchSize
}

// GetBufferSize 获取缓冲区大小
func (b *BaseSink) GetBufferSize() int {
	return b.bufferSize
}

// GetTags 获取标签
func (b *BaseSink) GetTags() map[string]string {
	return b.tags
}

// AddTags 添加标签到数据点
func (b *BaseSink) AddTags(points []model.Point) {
	if len(b.tags) == 0 {
		return
	}

	for i := range points {
		for k, v := range b.tags {
			points[i].AddTag(k, v)
		}
	}
}

// Health 返回连接器健康状态
func (b *BaseSink) Health() (SinkHealthStatus, error) {
	b.statsMutex.RLock()
	defer b.statsMutex.RUnlock()
	
	status := "healthy"
	message := "Sink is running normally"
	
	// 如果连接器未运行，标记为unhealthy
	if !b.IsRunning() {
		status = "unhealthy"
		message = "Sink is not running"
	} else if b.stats.LastError != "" {
		// 如果有错误，标记为degraded
		status = "degraded"
		message = b.stats.LastError
	}
	
	return SinkHealthStatus{
		Status:    status,
		Message:   message,
		LastCheck: time.Now(),
	}, nil
}

// GetMetrics 返回连接器运行指标
func (b *BaseSink) GetMetrics() (interface{}, error) {
	b.statsMutex.RLock()
	defer b.statsMutex.RUnlock()
	
	return SinkMetrics{
		MessagesPublished:   b.stats.MessagesTotal,
		ErrorsCount:         b.stats.MessagesFailed,
		LastPublishTime:     b.stats.LastMessage,
		ConnectionUptime:    0, // 基础实现暂时不提供运行时间
		LastError:           b.stats.LastError,
		AverageResponseTime: 0, // 基础实现暂时不提供响应时间
	}, nil
}

// GetLastError 返回最后的错误信息
func (b *BaseSink) GetLastError() error {
	b.statsMutex.RLock()
	defer b.statsMutex.RUnlock()
	
	if b.stats.LastError != "" {
		return fmt.Errorf(b.stats.LastError)
	}
	return nil
}

// SafePublishBatch 安全发布批量数据，包含错误处理和统计收集
func (b *BaseSink) SafePublishBatch(batch []model.Point, publishFunc func([]model.Point) error, operationStart time.Time) error {
	// 执行实际的发布操作
	err := publishFunc(batch)
	
	if err != nil {
		// 发布失败，记录错误
		b.SetLastError(err)
		log.Warn().
			Str("sink", b.name).
			Int("batch_size", len(batch)).
			Err(err).
			Msg("发布数据失败")
		return err
	}
	
	// 发布成功，更新统计信息
	b.IncrementMessageCount()
	b.statsMutex.Lock()
	b.stats.LastMessage = time.Now()
	b.statsMutex.Unlock()
	
	log.Debug().
		Str("sink", b.name).
		Int("batch_size", len(batch)).
		Float64("response_time_ms", float64(time.Since(operationStart).Nanoseconds())/1000000.0).
		Msg("发布数据成功")
	
	return nil
}

// ExtendedSink 定义了扩展的连接器接口（包含统计和健康检查）
type ExtendedSink interface {
	Sink

	// GetStats 获取连接器统计信息
	GetStats() SinkStats

	// Healthy 检查连接器健康状态
	Healthy() error
	
	// Health 返回连接器健康状态
	Health() (SinkHealthStatus, error)
	
	// GetMetrics 返回连接器运行指标
	GetMetrics() (SinkMetrics, error)
	
	// GetLastError 返回最后的错误信息
	GetLastError() error
}
