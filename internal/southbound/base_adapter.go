package southbound

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
)

// BaseAdapter 提供适配器的基础实现，包含通用的监控和错误处理功能
type BaseAdapter struct {
	name           string
	adapterType    string
	running        int32 // 使用atomic操作
	startTime      time.Time
	mu             sync.RWMutex
	
	// 指标相关
	dataPointsCollected int64
	errorsCount         int64
	lastDataPointTime   time.Time
	lastError           error
	
	// 响应时间统计
	responseTimes       []float64 // 最近的响应时间记录
	avgResponseTime     float64   // 平均响应时间(毫秒)
	maxResponseTimes    int       // 最多保存的响应时间记录数
	
	// 健康状态
	healthStatus   string
	healthMessage  string
	lastHealthCheck time.Time
}

// NewBaseAdapter 创建新的基础适配器
func NewBaseAdapter(name, adapterType string) *BaseAdapter {
	return &BaseAdapter{
		name:           name,
		adapterType:    adapterType,
		healthStatus:   "healthy",
		healthMessage:  "Adapter initialized",
		lastHealthCheck: time.Now(),
		responseTimes:   make([]float64, 0, 100),
		maxResponseTimes: 100, // 保存最近100个响应时间
	}
}

// Name 返回适配器名称
func (b *BaseAdapter) Name() string {
	return b.name
}

// IsRunning 检查适配器是否正在运行
func (b *BaseAdapter) IsRunning() bool {
	return atomic.LoadInt32(&b.running) == 1
}

// SetRunning 设置适配器运行状态
func (b *BaseAdapter) SetRunning(running bool) {
	if running {
		atomic.StoreInt32(&b.running, 1)
		b.startTime = time.Now()
	} else {
		atomic.StoreInt32(&b.running, 0)
	}
}

// IncrementDataPoints 增加数据点计数
func (b *BaseAdapter) IncrementDataPoints() {
	atomic.AddInt64(&b.dataPointsCollected, 1)
	b.mu.Lock()
	b.lastDataPointTime = time.Now()
	b.mu.Unlock()
}

// IncrementDataPointsWithTiming 增加数据点计数并记录响应时间
func (b *BaseAdapter) IncrementDataPointsWithTiming(operationStart time.Time) {
	atomic.AddInt64(&b.dataPointsCollected, 1)
	b.mu.Lock()
	b.lastDataPointTime = time.Now()
	b.mu.Unlock()
	
	// 记录响应时间
	b.RecordDataOperation(operationStart)
}

// IncrementErrors 增加错误计数
func (b *BaseAdapter) IncrementErrors() {
	atomic.AddInt64(&b.errorsCount, 1)
}

// SetLastError 设置最后的错误
func (b *BaseAdapter) SetLastError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastError = err
	if err != nil {
		b.IncrementErrors()
		b.healthStatus = "degraded"
		b.healthMessage = err.Error()
	}
	b.lastHealthCheck = time.Now()
}

// SetHealthStatus 设置健康状态
func (b *BaseAdapter) SetHealthStatus(status, message string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.healthStatus = status
	b.healthMessage = message
	b.lastHealthCheck = time.Now()
}

// Health 返回健康状态
func (b *BaseAdapter) Health() (HealthStatus, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	// 如果适配器正在运行但很久没有数据，标记为降级
	if b.IsRunning() && !b.lastDataPointTime.IsZero() {
		if time.Since(b.lastDataPointTime) > 2*time.Minute {
			return HealthStatus{
				Status:    "degraded",
				Message:   "No data received for more than 2 minutes",
				LastCheck: time.Now(),
			}, nil
		}
	}
	
	return HealthStatus{
		Status:    b.healthStatus,
		Message:   b.healthMessage,
		LastCheck: b.lastHealthCheck,
	}, nil
}

// GetMetrics 返回适配器指标
func (b *BaseAdapter) GetMetrics() (interface{}, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	var uptime time.Duration
	if b.IsRunning() {
		uptime = time.Since(b.startTime)
	}
	
	var lastErrorMsg string
	if b.lastError != nil {
		lastErrorMsg = b.lastError.Error()
	}
	
	return AdapterMetrics{
		DataPointsCollected: atomic.LoadInt64(&b.dataPointsCollected),
		ErrorsCount:         atomic.LoadInt64(&b.errorsCount),
		LastDataPointTime:   b.lastDataPointTime,
		ConnectionUptime:    uptime,
		LastError:           lastErrorMsg,
		AverageResponseTime: b.avgResponseTime,
	}, nil
}

// GetLastError 返回最后的错误
func (b *BaseAdapter) GetLastError() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastError
}

// AddResponseTime 添加响应时间记录
func (b *BaseAdapter) AddResponseTime(responseTimeMs float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// 添加新的响应时间
	b.responseTimes = append(b.responseTimes, responseTimeMs)
	
	// 如果超过最大记录数，移除最旧的记录
	if len(b.responseTimes) > b.maxResponseTimes {
		b.responseTimes = b.responseTimes[1:]
	}
	
	// 重新计算平均响应时间
	if len(b.responseTimes) > 0 {
		var sum float64
		for _, rt := range b.responseTimes {
			sum += rt
		}
		b.avgResponseTime = sum / float64(len(b.responseTimes))
	}
}

// GetAverageResponseTime 获取平均响应时间
func (b *BaseAdapter) GetAverageResponseTime() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.avgResponseTime
}

// RecordDataOperation 记录数据操作的响应时间
func (b *BaseAdapter) RecordDataOperation(startTime time.Time) {
	elapsed := time.Since(startTime)
	responseTimeMs := float64(elapsed.Nanoseconds()) / 1000000.0 // 转换为毫秒
	b.AddResponseTime(responseTimeMs)
}

// SafeSendDataPoint 安全发送数据点，包含错误处理和响应时间统计
func (b *BaseAdapter) SafeSendDataPoint(ch chan<- model.Point, point model.Point, operationStart time.Time) {
	select {
	case ch <- point:
		// 成功发送，更新指标并记录响应时间
		b.IncrementDataPointsWithTiming(operationStart)
	default:
		// 缓冲区已满，记录错误
		err := fmt.Errorf("buffer full, dropped data point for key %s", point.Key)
		b.SetLastError(err)
		log.Warn().Str("key", point.Key).Msg("缓冲区已满，丢弃数据点")
	}
}