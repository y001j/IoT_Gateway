package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/southbound"
	"github.com/y001j/iot-gateway/internal/northbound"
)

// MetricsProvider 定义了metrics提供者接口
type MetricsProvider interface {
	GetAdapters() []southbound.Adapter
	GetSinks() []northbound.Sink
}

// 全局metrics提供者
var globalMetricsProvider MetricsProvider

// MetricsCache 缓存结构
type MetricsCache struct {
	data       *LightweightMetrics
	lastUpdate time.Time
	ttl        time.Duration
}

// LightweightMetrics 轻量级指标收集器
type LightweightMetrics struct {
	mu        sync.RWMutex
	startTime time.Time
	
	// 更新管理器
	updateCtx    context.Context
	updateCancel context.CancelFunc
	updateTicker *time.Ticker
	
	// 缓存
	cache *MetricsCache
	
	// 系统指标
	SystemMetrics SystemMetrics `json:"system"`
	
	// 网关指标
	GatewayMetrics GatewayMetrics `json:"gateway"`
	
	// 数据处理指标
	DataMetrics DataMetrics `json:"data"`
	
	// 连接指标
	ConnectionMetrics ConnectionMetrics `json:"connections"`
	
	// 规则引擎指标
	RuleMetrics RuleMetrics `json:"rules"`
	
	// 性能指标
	PerformanceMetrics PerformanceMetrics `json:"performance"`
	
	// 错误指标
	ErrorMetrics ErrorMetrics `json:"errors"`
	
	// 最后更新时间
	LastUpdated time.Time `json:"last_updated"`
}

// SystemMetrics 系统指标
type SystemMetrics struct {
	UptimeSeconds     float64 `json:"uptime_seconds"`
	MemoryUsageBytes  int64   `json:"memory_usage_bytes"`
	CPUUsagePercent   float64 `json:"cpu_usage_percent"`
	GoroutineCount    int     `json:"goroutine_count"`
	GCPauseMS         float64 `json:"gc_pause_ms"`
	HeapSizeBytes     int64   `json:"heap_size_bytes"`
	HeapInUseBytes    int64   `json:"heap_in_use_bytes"`
	Version           string  `json:"version"`
	GoVersion         string  `json:"go_version"`
}

// GatewayMetrics 网关指标
type GatewayMetrics struct {
	Status              string    `json:"status"`
	StartTime           time.Time `json:"start_time"`
	ConfigFile          string    `json:"config_file"`
	PluginsDirectory    string    `json:"plugins_directory"`
	TotalAdapters       int       `json:"total_adapters"`
	RunningAdapters     int       `json:"running_adapters"`
	TotalSinks          int       `json:"total_sinks"`
	RunningSinks        int       `json:"running_sinks"`
	NATSConnected       bool      `json:"nats_connected"`
	NATSConnectionURL   string    `json:"nats_connection_url"`
	WebUIPort          int       `json:"web_ui_port"`
	APIPort            int       `json:"api_port"`
}

// DataMetrics 数据处理指标
type DataMetrics struct {
	TotalDataPoints         int64   `json:"total_data_points"`
	DataPointsPerSecond     float64 `json:"data_points_per_second"`
	TotalBytesProcessed     int64   `json:"total_bytes_processed"`
	BytesPerSecond          float64 `json:"bytes_per_second"`
	AverageLatencyMS        float64 `json:"average_latency_ms"`
	MaxLatencyMS            float64 `json:"max_latency_ms"`
	MinLatencyMS            float64 `json:"min_latency_ms"`
	LastDataPointTime       time.Time `json:"last_data_point_time"`
	DataQueueLength         int     `json:"data_queue_length"`
	ProcessingErrorsCount   int64   `json:"processing_errors_count"`
	TotalMessagesPublished  int64   `json:"total_messages_published"`
	PublishRate             float64 `json:"publish_rate"`
}

// ConnectionMetrics 连接指标
type ConnectionMetrics struct {
	ActiveConnections    int                    `json:"active_connections"`
	TotalConnections     int64                  `json:"total_connections"`
	FailedConnections    int64                  `json:"failed_connections"`
	ConnectionsByType    map[string]int         `json:"connections_by_type"`
	ConnectionsByStatus  map[string]int         `json:"connections_by_status"`
	AverageResponseTimeMS float64               `json:"average_response_time_ms"`
	ConnectionErrors     int64                  `json:"connection_errors"`
	ReconnectionCount    int64                  `json:"reconnection_count"`
}

// RuleMetrics 规则引擎指标
type RuleMetrics struct {
	TotalRules           int       `json:"total_rules"`
	EnabledRules         int       `json:"enabled_rules"`
	RulesMatched         int64     `json:"rules_matched"`
	ActionsExecuted      int64     `json:"actions_executed"`
	ActionsSucceeded     int64     `json:"actions_succeeded"`
	ActionsFailed        int64     `json:"actions_failed"`
	AverageExecutionTimeMS float64 `json:"average_execution_time_ms"`
	RuleEngineStatus     string    `json:"rule_engine_status"`
	LastRuleExecution    time.Time `json:"last_rule_execution"`
}

// PerformanceMetrics 性能指标
type PerformanceMetrics struct {
	ThroughputPerSecond     float64           `json:"throughput_per_second"`
	P50LatencyMS            float64           `json:"p50_latency_ms"`
	P95LatencyMS            float64           `json:"p95_latency_ms"`
	P99LatencyMS            float64           `json:"p99_latency_ms"`
	QueueLength             int               `json:"queue_length"`
	ProcessingTime          map[string]float64 `json:"processing_time"`
	ResourceUtilization     map[string]float64 `json:"resource_utilization"`
	AverageResponseTimeMS   float64           `json:"average_response_time_ms"`
}

// ErrorMetrics 错误指标
type ErrorMetrics struct {
	TotalErrors         int64            `json:"total_errors"`
	ErrorsPerSecond     float64          `json:"errors_per_second"`
	ErrorsByType        map[string]int64 `json:"errors_by_type"`
	ErrorsByLevel       map[string]int64 `json:"errors_by_level"`
	LastError           string           `json:"last_error"`
	LastErrorTime       time.Time        `json:"last_error_time"`
	ErrorRate           float64          `json:"error_rate"`
	RecoveryCount       int64            `json:"recovery_count"`
}

// NewLightweightMetrics 创建轻量级指标收集器
func NewLightweightMetrics() *LightweightMetrics {
	return &LightweightMetrics{
		startTime: time.Now(),
		cache: &MetricsCache{
			ttl: 3 * time.Second, // 缓存3秒
		},
		SystemMetrics: SystemMetrics{
			Version:   "1.0.0",
			GoVersion: runtime.Version(),
		},
		GatewayMetrics: GatewayMetrics{
			Status:    "starting",
			StartTime: time.Now(),
		},
		ConnectionMetrics: ConnectionMetrics{
			ConnectionsByType:   make(map[string]int),
			ConnectionsByStatus: make(map[string]int),
		},
		PerformanceMetrics: PerformanceMetrics{
			ProcessingTime:      make(map[string]float64),
			ResourceUtilization: make(map[string]float64),
		},
		ErrorMetrics: ErrorMetrics{
			ErrorsByType:  make(map[string]int64),
			ErrorsByLevel: make(map[string]int64),
		},
		LastUpdated: time.Now(),
	}
}

// UpdateSystemMetrics 更新系统指标
func (m *LightweightMetrics) UpdateSystemMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 更新运行时间
	m.SystemMetrics.UptimeSeconds = time.Since(m.startTime).Seconds()
	
	// 获取内存统计
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	m.SystemMetrics.MemoryUsageBytes = int64(memStats.Alloc)
	m.SystemMetrics.HeapSizeBytes = int64(memStats.HeapSys)
	m.SystemMetrics.HeapInUseBytes = int64(memStats.HeapInuse)
	m.SystemMetrics.GCPauseMS = float64(memStats.PauseNs[(memStats.NumGC+255)%256]) / 1e6
	m.SystemMetrics.GoroutineCount = runtime.NumGoroutine()
	
	// CPU使用率需要在实际使用中计算
	// 这里设置为0，实际实现时可以使用系统调用获取
	m.SystemMetrics.CPUUsagePercent = 0
	
	m.LastUpdated = time.Now()
}

// UpdateGatewayMetrics 更新网关指标
func (m *LightweightMetrics) UpdateGatewayMetrics(status, configFile, pluginsDir string, webPort, apiPort int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.GatewayMetrics.Status = status
	m.GatewayMetrics.ConfigFile = configFile
	m.GatewayMetrics.PluginsDirectory = pluginsDir
	m.GatewayMetrics.WebUIPort = webPort
	m.GatewayMetrics.APIPort = apiPort
	
	m.LastUpdated = time.Now()
}

// UpdateDataMetrics 更新数据处理指标
func (m *LightweightMetrics) UpdateDataMetrics(totalPoints int64, bytesProcessed int64, latencyMS float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.DataMetrics.TotalDataPoints = totalPoints
	m.DataMetrics.TotalBytesProcessed = bytesProcessed
	m.DataMetrics.LastDataPointTime = time.Now()
	
	// 更新延迟统计
	if latencyMS > 0 {
		if m.DataMetrics.MaxLatencyMS == 0 || latencyMS > m.DataMetrics.MaxLatencyMS {
			m.DataMetrics.MaxLatencyMS = latencyMS
		}
		if m.DataMetrics.MinLatencyMS == 0 || latencyMS < m.DataMetrics.MinLatencyMS {
			m.DataMetrics.MinLatencyMS = latencyMS
		}
		
		// 简单的平均延迟计算
		m.DataMetrics.AverageLatencyMS = (m.DataMetrics.AverageLatencyMS + latencyMS) / 2
	}
	
	// 计算每秒处理量
	uptime := time.Since(m.startTime).Seconds()
	if uptime > 0 {
		m.DataMetrics.DataPointsPerSecond = float64(totalPoints) / uptime
		m.DataMetrics.BytesPerSecond = float64(bytesProcessed) / uptime
	}
	
	m.LastUpdated = time.Now()
}

// UpdateConnectionMetrics 更新连接指标
func (m *LightweightMetrics) UpdateConnectionMetrics(activeConns int, totalConns, failedConns int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.ConnectionMetrics.ActiveConnections = activeConns
	m.ConnectionMetrics.TotalConnections = totalConns
	m.ConnectionMetrics.FailedConnections = failedConns
	
	m.LastUpdated = time.Now()
}

// UpdateRuleMetrics 更新规则引擎指标
func (m *LightweightMetrics) UpdateRuleMetrics(totalRules, enabledRules int, matched, executed, succeeded, failed int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.RuleMetrics.TotalRules = totalRules
	m.RuleMetrics.EnabledRules = enabledRules
	m.RuleMetrics.RulesMatched = matched
	m.RuleMetrics.ActionsExecuted = executed
	m.RuleMetrics.ActionsSucceeded = succeeded
	m.RuleMetrics.ActionsFailed = failed
	m.RuleMetrics.LastRuleExecution = time.Now()
	
	m.LastUpdated = time.Now()
}

// UpdateErrorMetrics 更新错误指标
func (m *LightweightMetrics) UpdateErrorMetrics(totalErrors int64, errorType, errorLevel, lastError string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.ErrorMetrics.TotalErrors = totalErrors
	m.ErrorMetrics.LastError = lastError
	m.ErrorMetrics.LastErrorTime = time.Now()
	
	// 更新错误分类统计
	if errorType != "" {
		m.ErrorMetrics.ErrorsByType[errorType]++
	}
	if errorLevel != "" {
		m.ErrorMetrics.ErrorsByLevel[errorLevel]++
	}
	
	// 计算错误率
	if m.DataMetrics.TotalDataPoints > 0 {
		m.ErrorMetrics.ErrorRate = float64(totalErrors) / float64(m.DataMetrics.TotalDataPoints)
	}
	
	// 计算每秒错误数
	uptime := time.Since(m.startTime).Seconds()
	if uptime > 0 {
		m.ErrorMetrics.ErrorsPerSecond = float64(totalErrors) / uptime
	}
	
	m.LastUpdated = time.Now()
}

// UpdatePerformanceMetrics 更新性能指标
func (m *LightweightMetrics) UpdatePerformanceMetrics(throughput, p50, p95, p99, avgResponseTime float64, queueLength int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.PerformanceMetrics.ThroughputPerSecond = throughput
	m.PerformanceMetrics.P50LatencyMS = p50
	m.PerformanceMetrics.P95LatencyMS = p95
	m.PerformanceMetrics.P99LatencyMS = p99
	m.PerformanceMetrics.AverageResponseTimeMS = avgResponseTime
	m.PerformanceMetrics.QueueLength = queueLength
	
	m.LastUpdated = time.Now()
}

// GetMetrics 获取所有指标
func (m *LightweightMetrics) GetMetrics() *LightweightMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// 检查缓存是否有效
	if m.cache.data != nil && time.Since(m.cache.lastUpdate) < m.cache.ttl {
		return m.cache.data
	}
	
	// 缓存无效，创建新的副本
	metrics := &LightweightMetrics{}
	*metrics = *m
	
	// 更新缓存
	m.cache.data = metrics
	m.cache.lastUpdate = time.Now()
	
	return metrics
}

// ToJSON 转换为JSON格式
func (m *LightweightMetrics) ToJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return json.MarshalIndent(m, "", "  ")
}

// ToPlainText 转换为纯文本格式
func (m *LightweightMetrics) ToPlainText() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var result string
	
	result += fmt.Sprintf("# IoT Gateway Metrics\n")
	result += fmt.Sprintf("# Generated at: %s\n\n", m.LastUpdated.Format(time.RFC3339))
	
	// 系统指标
	result += fmt.Sprintf("# System Metrics\n")
	result += fmt.Sprintf("iot_gateway_uptime_seconds %.2f\n", m.SystemMetrics.UptimeSeconds)
	result += fmt.Sprintf("iot_gateway_memory_usage_bytes %d\n", m.SystemMetrics.MemoryUsageBytes)
	result += fmt.Sprintf("iot_gateway_cpu_usage_percent %.2f\n", m.SystemMetrics.CPUUsagePercent)
	result += fmt.Sprintf("iot_gateway_goroutine_count %d\n", m.SystemMetrics.GoroutineCount)
	result += fmt.Sprintf("iot_gateway_gc_pause_ms %.2f\n", m.SystemMetrics.GCPauseMS)
	result += fmt.Sprintf("iot_gateway_heap_size_bytes %d\n", m.SystemMetrics.HeapSizeBytes)
	result += fmt.Sprintf("iot_gateway_heap_in_use_bytes %d\n", m.SystemMetrics.HeapInUseBytes)
	result += fmt.Sprintf("\n")
	
	// 网关指标
	result += fmt.Sprintf("# Gateway Metrics\n")
	result += fmt.Sprintf("iot_gateway_status{status=\"%s\"} 1\n", m.GatewayMetrics.Status)
	result += fmt.Sprintf("iot_gateway_total_adapters %d\n", m.GatewayMetrics.TotalAdapters)
	result += fmt.Sprintf("iot_gateway_running_adapters %d\n", m.GatewayMetrics.RunningAdapters)
	result += fmt.Sprintf("iot_gateway_total_sinks %d\n", m.GatewayMetrics.TotalSinks)
	result += fmt.Sprintf("iot_gateway_running_sinks %d\n", m.GatewayMetrics.RunningSinks)
	result += fmt.Sprintf("iot_gateway_nats_connected %t\n", m.GatewayMetrics.NATSConnected)
	result += fmt.Sprintf("iot_gateway_web_ui_port %d\n", m.GatewayMetrics.WebUIPort)
	result += fmt.Sprintf("iot_gateway_api_port %d\n", m.GatewayMetrics.APIPort)
	result += fmt.Sprintf("\n")
	
	// 数据处理指标
	result += fmt.Sprintf("# Data Processing Metrics\n")
	result += fmt.Sprintf("iot_gateway_total_data_points %d\n", m.DataMetrics.TotalDataPoints)
	result += fmt.Sprintf("iot_gateway_data_points_per_second %.2f\n", m.DataMetrics.DataPointsPerSecond)
	result += fmt.Sprintf("iot_gateway_total_bytes_processed %d\n", m.DataMetrics.TotalBytesProcessed)
	result += fmt.Sprintf("iot_gateway_bytes_per_second %.2f\n", m.DataMetrics.BytesPerSecond)
	result += fmt.Sprintf("iot_gateway_average_latency_ms %.2f\n", m.DataMetrics.AverageLatencyMS)
	result += fmt.Sprintf("iot_gateway_max_latency_ms %.2f\n", m.DataMetrics.MaxLatencyMS)
	result += fmt.Sprintf("iot_gateway_min_latency_ms %.2f\n", m.DataMetrics.MinLatencyMS)
	result += fmt.Sprintf("iot_gateway_data_queue_length %d\n", m.DataMetrics.DataQueueLength)
	result += fmt.Sprintf("iot_gateway_processing_errors_count %d\n", m.DataMetrics.ProcessingErrorsCount)
	result += fmt.Sprintf("\n")
	
	// 连接指标
	result += fmt.Sprintf("# Connection Metrics\n")
	result += fmt.Sprintf("iot_gateway_active_connections %d\n", m.ConnectionMetrics.ActiveConnections)
	result += fmt.Sprintf("iot_gateway_total_connections %d\n", m.ConnectionMetrics.TotalConnections)
	result += fmt.Sprintf("iot_gateway_failed_connections %d\n", m.ConnectionMetrics.FailedConnections)
	result += fmt.Sprintf("iot_gateway_average_response_time_ms %.2f\n", m.ConnectionMetrics.AverageResponseTimeMS)
	result += fmt.Sprintf("iot_gateway_connection_errors %d\n", m.ConnectionMetrics.ConnectionErrors)
	result += fmt.Sprintf("iot_gateway_reconnection_count %d\n", m.ConnectionMetrics.ReconnectionCount)
	result += fmt.Sprintf("\n")
	
	// 规则引擎指标
	result += fmt.Sprintf("# Rule Engine Metrics\n")
	result += fmt.Sprintf("iot_gateway_total_rules %d\n", m.RuleMetrics.TotalRules)
	result += fmt.Sprintf("iot_gateway_enabled_rules %d\n", m.RuleMetrics.EnabledRules)
	result += fmt.Sprintf("iot_gateway_rules_matched %d\n", m.RuleMetrics.RulesMatched)
	result += fmt.Sprintf("iot_gateway_actions_executed %d\n", m.RuleMetrics.ActionsExecuted)
	result += fmt.Sprintf("iot_gateway_actions_succeeded %d\n", m.RuleMetrics.ActionsSucceeded)
	result += fmt.Sprintf("iot_gateway_actions_failed %d\n", m.RuleMetrics.ActionsFailed)
	result += fmt.Sprintf("iot_gateway_average_execution_time_ms %.2f\n", m.RuleMetrics.AverageExecutionTimeMS)
	result += fmt.Sprintf("iot_gateway_rule_engine_status{status=\"%s\"} 1\n", m.RuleMetrics.RuleEngineStatus)
	result += fmt.Sprintf("\n")
	
	// 错误指标
	result += fmt.Sprintf("# Error Metrics\n")
	result += fmt.Sprintf("iot_gateway_total_errors %d\n", m.ErrorMetrics.TotalErrors)
	result += fmt.Sprintf("iot_gateway_errors_per_second %.2f\n", m.ErrorMetrics.ErrorsPerSecond)
	result += fmt.Sprintf("iot_gateway_error_rate %.4f\n", m.ErrorMetrics.ErrorRate)
	result += fmt.Sprintf("iot_gateway_recovery_count %d\n", m.ErrorMetrics.RecoveryCount)
	
	// 按类型分类的错误
	for errType, count := range m.ErrorMetrics.ErrorsByType {
		result += fmt.Sprintf("iot_gateway_errors_by_type{type=\"%s\"} %d\n", errType, count)
	}
	
	// 按级别分类的错误
	for errLevel, count := range m.ErrorMetrics.ErrorsByLevel {
		result += fmt.Sprintf("iot_gateway_errors_by_level{level=\"%s\"} %d\n", errLevel, count)
	}
	
	return result
}

// HTTPHandler HTTP处理器
func (m *LightweightMetrics) HTTPHandler(format string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 更新系统指标
		m.UpdateSystemMetrics()
		
		switch format {
		case "json":
			c.Header("Content-Type", "application/json")
			data, err := m.ToJSON()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.Data(http.StatusOK, "application/json", data)
		case "text", "plain":
			c.Header("Content-Type", "text/plain")
			c.String(http.StatusOK, m.ToPlainText())
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的格式，支持的格式: json, text, plain"})
		}
	}
}

// 全局指标收集器实例
var globalLightweightMetrics *LightweightMetrics

// InitLightweightMetrics 初始化轻量级指标收集器
func InitLightweightMetrics() {
	globalLightweightMetrics = NewLightweightMetrics()
	// 启动自动更新，默认5秒间隔
	globalLightweightMetrics.StartAutoUpdate(5 * time.Second)
	log.Info().Msg("轻量级指标收集器已初始化并启动自动更新")
}

// GetLightweightMetrics 获取全局轻量级指标收集器
func GetLightweightMetrics() *LightweightMetrics {
	return globalLightweightMetrics
}

// ShutdownLightweightMetrics 关闭轻量级指标收集器
func ShutdownLightweightMetrics() {
	if globalLightweightMetrics != nil {
		globalLightweightMetrics.StopAutoUpdate()
		log.Info().Msg("轻量级指标收集器已关闭")
	}
}

// SetMetricsProvider 设置全局metrics提供者
func SetMetricsProvider(provider MetricsProvider) {
	globalMetricsProvider = provider
	log.Info().Msg("metrics提供者已设置")
}

// AggregateAdapterMetrics 汇总所有adapter的metrics
func (m *LightweightMetrics) AggregateAdapterMetrics() {
	if globalMetricsProvider == nil {
		return
	}
	
	adapters := globalMetricsProvider.GetAdapters()
	var totalDataPoints int64
	var totalErrors int64
	var totalResponseTime float64
	var adapterCount int
	
	for _, adapter := range adapters {
		if metricsInterface, ok := adapter.(interface {
			GetMetrics() (interface{}, error)
		}); ok {
			if metrics, err := metricsInterface.GetMetrics(); err == nil {
				if adapterMetrics, ok := metrics.(southbound.AdapterMetrics); ok {
					totalDataPoints += adapterMetrics.DataPointsCollected
					totalErrors += adapterMetrics.ErrorsCount
					totalResponseTime += adapterMetrics.AverageResponseTime
					adapterCount++
				}
			}
		}
	}
	
	// 更新数据处理指标
	m.mu.Lock()
	m.DataMetrics.TotalDataPoints = totalDataPoints
	m.DataMetrics.DataPointsPerSecond = float64(totalDataPoints) / time.Since(m.startTime).Seconds()
	
	// 更新错误指标
	m.ErrorMetrics.TotalErrors = totalErrors
	
	// 更新性能指标
	if adapterCount > 0 {
		m.PerformanceMetrics.AverageResponseTimeMS = totalResponseTime / float64(adapterCount)
	}
	m.mu.Unlock()
}

// AggregateSinkMetrics 汇总所有sink的metrics
func (m *LightweightMetrics) AggregateSinkMetrics() {
	if globalMetricsProvider == nil {
		return
	}
	
	sinks := globalMetricsProvider.GetSinks()
	var totalMessages int64
	var totalErrors int64
	var totalResponseTime float64
	var sinkCount int
	
	for _, sink := range sinks {
		if metricsInterface, ok := sink.(interface {
			GetMetrics() (interface{}, error)
		}); ok {
			if metrics, err := metricsInterface.GetMetrics(); err == nil {
				if sinkMetrics, ok := metrics.(northbound.SinkMetrics); ok {
					totalMessages += sinkMetrics.MessagesPublished
					totalErrors += sinkMetrics.ErrorsCount
					totalResponseTime += sinkMetrics.AverageResponseTime
					sinkCount++
				}
			}
		}
	}
	
	// 更新数据处理指标
	m.mu.Lock()
	m.DataMetrics.TotalMessagesPublished = totalMessages
	m.DataMetrics.PublishRate = float64(totalMessages) / time.Since(m.startTime).Seconds()
	
	// 更新错误指标
	m.ErrorMetrics.TotalErrors += totalErrors
	
	// 更新性能指标
	if sinkCount > 0 {
		currentAvg := m.PerformanceMetrics.AverageResponseTimeMS
		m.PerformanceMetrics.AverageResponseTimeMS = (currentAvg + totalResponseTime/float64(sinkCount)) / 2
	}
	m.mu.Unlock()
}

// StartAutoUpdate 启动自动更新
func (m *LightweightMetrics) StartAutoUpdate(interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second // 默认5秒更新一次
	}
	
	m.updateCtx, m.updateCancel = context.WithCancel(context.Background())
	m.updateTicker = time.NewTicker(interval)
	
	go func() {
		log.Info().Dur("interval", interval).Msg("轻量级metrics自动更新已启动")
		
		for {
			select {
			case <-m.updateTicker.C:
				m.UpdateSystemMetrics()
				
				// 汇总adapter和sink的metrics
				m.AggregateAdapterMetrics()
				m.AggregateSinkMetrics()
				
				// 更新最后更新时间
				m.mu.Lock()
				m.LastUpdated = time.Now()
				m.mu.Unlock()
				
				log.Debug().Msg("轻量级metrics已更新")
				
			case <-m.updateCtx.Done():
				log.Info().Msg("轻量级metrics自动更新已停止")
				return
			}
		}
	}()
}

// StopAutoUpdate 停止自动更新
func (m *LightweightMetrics) StopAutoUpdate() {
	if m.updateCancel != nil {
		m.updateCancel()
	}
	if m.updateTicker != nil {
		m.updateTicker.Stop()
	}
	
	log.Info().Msg("轻量级metrics自动更新已停止")
}