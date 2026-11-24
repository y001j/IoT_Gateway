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
	"github.com/y001j/iot-gateway/internal/monitoring"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/southbound"
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

// DataPointsTracker 数据点速率跟踪器
type DataPointsTracker struct {
	// 64-bit fields first for ARM32 alignment
	lastCount      int64
	lastBytesCount int64
	// Other fields
	lastTime        time.Time
	currentRate     float64
	currentByteRate float64
	initialized     bool
}

// LightweightMetrics 轻量级指标收集器
type LightweightMetrics struct {
	mu        sync.RWMutex
	startTime time.Time

	// 更新管理器
	updateCtx    context.Context
	updateCancel context.CancelFunc
	updateTicker *time.Ticker

	// 系统指标收集器
	systemCollector *monitoring.SystemCollector

	// 实时速率跟踪器
	dataTracker *DataPointsTracker

	// 缓存（由 cacheMu 保护）
	cache   *MetricsCache
	cacheMu sync.Mutex

	// 自定义更新回调
	updateCallback func()

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
	// 64-bit fields first for ARM32 alignment
	MemoryUsageBytes int64 `json:"memory_usage_bytes"`
	HeapSizeBytes    int64 `json:"heap_size_bytes"`
	HeapInUseBytes   int64 `json:"heap_in_use_bytes"`
	// 系统（整机）内存，来自系统采集器；采集不到时为 0
	SystemMemoryUsedBytes  int64 `json:"system_memory_used_bytes"`
	SystemMemoryTotalBytes int64 `json:"system_memory_total_bytes"`
	// 网络累计流量指标
	NetworkInBytes    int64 `json:"network_in_bytes"`
	NetworkOutBytes   int64 `json:"network_out_bytes"`
	NetworkInPackets  int64 `json:"network_in_packets"`
	NetworkOutPackets int64 `json:"network_out_packets"`
	// Other fields
	UptimeSeconds    float64 `json:"uptime_seconds"`
	CPUUsagePercent  float64 `json:"cpu_usage_percent"`
	DiskUsagePercent float64 `json:"disk_usage_percent"`
	// SystemMemoryPercent 系统内存使用率（%）
	SystemMemoryPercent float64 `json:"system_memory_percent"`
	GoroutineCount      int     `json:"goroutine_count"`
	GCPauseMS           float64 `json:"gc_pause_ms"`
	// 网络实时速率指标
	NetworkInBytesPerSec    float64 `json:"network_in_bytes_per_sec"`
	NetworkOutBytesPerSec   float64 `json:"network_out_bytes_per_sec"`
	NetworkInPacketsPerSec  float64 `json:"network_in_packets_per_sec"`
	NetworkOutPacketsPerSec float64 `json:"network_out_packets_per_sec"`
	Version                 string  `json:"version"`
	GoVersion               string  `json:"go_version"`
}

// GatewayMetrics 网关指标
type GatewayMetrics struct {
	Status            string    `json:"status"`
	StartTime         time.Time `json:"start_time"`
	ConfigFile        string    `json:"config_file"`
	PluginsDirectory  string    `json:"plugins_directory"`
	TotalAdapters     int       `json:"total_adapters"`
	RunningAdapters   int       `json:"running_adapters"`
	TotalSinks        int       `json:"total_sinks"`
	RunningSinks      int       `json:"running_sinks"`
	NATSConnected     bool      `json:"nats_connected"`
	NATSConnectionURL string    `json:"nats_connection_url"`
	WebUIPort         int       `json:"web_ui_port"`
	APIPort           int       `json:"api_port"`
}

// DataMetrics 数据处理指标
type DataMetrics struct {
	// 64-bit fields first for ARM32 alignment
	TotalDataPoints        int64 `json:"total_data_points"`
	TotalBytesProcessed    int64 `json:"total_bytes_processed"`
	ProcessingErrorsCount  int64 `json:"processing_errors_count"`
	TotalMessagesPublished int64 `json:"total_messages_published"`
	// Other fields
	DataPointsPerSecond float64   `json:"data_points_per_second"`
	BytesPerSecond      float64   `json:"bytes_per_second"`
	AverageLatencyMS    float64   `json:"average_latency_ms"`
	MaxLatencyMS        float64   `json:"max_latency_ms"`
	MinLatencyMS        float64   `json:"min_latency_ms"`
	LastDataPointTime   time.Time `json:"last_data_point_time"`
	DataQueueLength     int       `json:"data_queue_length"`
	PublishRate         float64   `json:"publish_rate"`
}

// ConnectionMetrics 连接指标
type ConnectionMetrics struct {
	// 64-bit fields first for ARM32 alignment
	TotalConnections  int64 `json:"total_connections"`
	FailedConnections int64 `json:"failed_connections"`
	ConnectionErrors  int64 `json:"connection_errors"`
	ReconnectionCount int64 `json:"reconnection_count"`
	// Other fields
	ActiveConnections     int            `json:"active_connections"`
	ConnectionsByType     map[string]int `json:"connections_by_type"`
	ConnectionsByStatus   map[string]int `json:"connections_by_status"`
	AverageResponseTimeMS float64        `json:"average_response_time_ms"`
}

// RuleMetrics 规则引擎指标
type RuleMetrics struct {
	// 64-bit fields first for ARM32 alignment
	RulesMatched     int64 `json:"rules_matched"`
	ActionsExecuted  int64 `json:"actions_executed"`
	ActionsSucceeded int64 `json:"actions_succeeded"`
	ActionsFailed    int64 `json:"actions_failed"`
	// Other fields
	TotalRules             int       `json:"total_rules"`
	EnabledRules           int       `json:"enabled_rules"`
	AverageExecutionTimeMS float64   `json:"average_execution_time_ms"`
	RuleEngineStatus       string    `json:"rule_engine_status"`
	LastRuleExecution      time.Time `json:"last_rule_execution"`
}

// PerformanceMetrics 性能指标
type PerformanceMetrics struct {
	ThroughputPerSecond   float64            `json:"throughput_per_second"`
	P50LatencyMS          float64            `json:"p50_latency_ms"`
	P95LatencyMS          float64            `json:"p95_latency_ms"`
	P99LatencyMS          float64            `json:"p99_latency_ms"`
	QueueLength           int                `json:"queue_length"`
	ProcessingTime        map[string]float64 `json:"processing_time"`
	ResourceUtilization   map[string]float64 `json:"resource_utilization"`
	AverageResponseTimeMS float64            `json:"average_response_time_ms"`
}

// ErrorMetrics 错误指标
type ErrorMetrics struct {
	// 64-bit fields first for ARM32 alignment
	TotalErrors   int64 `json:"total_errors"`
	RecoveryCount int64 `json:"recovery_count"`
	// Other fields
	ErrorsPerSecond float64          `json:"errors_per_second"`
	ErrorsByType    map[string]int64 `json:"errors_by_type"`
	ErrorsByLevel   map[string]int64 `json:"errors_by_level"`
	LastError       string           `json:"last_error"`
	LastErrorTime   time.Time        `json:"last_error_time"`
	ErrorRate       float64          `json:"error_rate"`
}

// NewLightweightMetrics 创建轻量级指标收集器
func NewLightweightMetrics() *LightweightMetrics {
	// 创建并启动系统指标收集器
	// 根据操作系统设置磁盘路径
	diskPath := "/"
	if runtime.GOOS == "windows" {
		diskPath = "C:\\"
	}

	systemCollectorConfig := monitoring.SystemCollectorConfig{
		Enabled:         true,
		CollectInterval: 5 * time.Second,
		CacheDuration:   10 * time.Second,
		DiskPath:        diskPath,
	}

	systemCollector := monitoring.NewSystemCollector(systemCollectorConfig)
	if err := systemCollector.Start(); err != nil {
		log.Warn().Err(err).Msg("系统指标收集器启动失败，将使用基本指标")
	} else {
		log.Info().Msg("系统指标收集器已启动")
	}

	return &LightweightMetrics{
		startTime:       time.Now(),
		systemCollector: systemCollector,
		dataTracker: &DataPointsTracker{
			lastTime:    time.Now(),
			initialized: false,
		},
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

	// 尝试从系统收集器获取真实的系统指标
	if m.systemCollector != nil {
		if systemMetrics, err := m.systemCollector.GetMetrics(); err == nil {
			// 使用系统收集器的CPU和磁盘使用率
			m.SystemMetrics.CPUUsagePercent = systemMetrics.CPUUsage
			m.SystemMetrics.DiskUsagePercent = systemMetrics.DiskUsage
			m.SystemMetrics.SystemMemoryPercent = systemMetrics.MemoryUsage
			m.SystemMetrics.SystemMemoryUsedBytes = int64(systemMetrics.MemoryUsageBytes)
			m.SystemMetrics.SystemMemoryTotalBytes = int64(systemMetrics.MemoryTotalBytes)

			// 网络累计流量指标
			m.SystemMetrics.NetworkInBytes = int64(systemMetrics.NetworkInBytes)
			m.SystemMetrics.NetworkOutBytes = int64(systemMetrics.NetworkOutBytes)
			m.SystemMetrics.NetworkInPackets = int64(systemMetrics.NetworkInPackets)
			m.SystemMetrics.NetworkOutPackets = int64(systemMetrics.NetworkOutPackets)

			// 网络实时速率指标
			m.SystemMetrics.NetworkInBytesPerSec = systemMetrics.NetworkInBytesPerSec
			m.SystemMetrics.NetworkOutBytesPerSec = systemMetrics.NetworkOutBytesPerSec
			m.SystemMetrics.NetworkInPacketsPerSec = systemMetrics.NetworkInPacketsPerSec
			m.SystemMetrics.NetworkOutPacketsPerSec = systemMetrics.NetworkOutPacketsPerSec

			// 系统指标已更新（移除频繁debug日志）
		} else {
			log.Warn().Err(err).Msg("无法获取系统指标，使用默认值")
			m.SystemMetrics.CPUUsagePercent = 0
			m.SystemMetrics.DiskUsagePercent = 0
			m.SystemMetrics.NetworkInBytes = 0
			m.SystemMetrics.NetworkOutBytes = 0
			m.SystemMetrics.NetworkInPackets = 0
			m.SystemMetrics.NetworkOutPackets = 0
			m.SystemMetrics.NetworkInBytesPerSec = 0
			m.SystemMetrics.NetworkOutBytesPerSec = 0
			m.SystemMetrics.NetworkInPacketsPerSec = 0
			m.SystemMetrics.NetworkOutPacketsPerSec = 0
		}
	} else {
		// 系统收集器未启用，使用默认值0
		m.SystemMetrics.CPUUsagePercent = 0
		m.SystemMetrics.DiskUsagePercent = 0
		m.SystemMetrics.NetworkInBytes = 0
		m.SystemMetrics.NetworkOutBytes = 0
		m.SystemMetrics.NetworkInPackets = 0
		m.SystemMetrics.NetworkOutPackets = 0
		m.SystemMetrics.NetworkInBytesPerSec = 0
		m.SystemMetrics.NetworkOutBytesPerSec = 0
		m.SystemMetrics.NetworkInPacketsPerSec = 0
		m.SystemMetrics.NetworkOutPacketsPerSec = 0
	}

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

// calculateRealTimeRate 计算实时速率
func (m *LightweightMetrics) calculateRealTimeRate(currentPoints, currentBytes int64, now time.Time) {
	if m.dataTracker == nil {
		return
	}

	if !m.dataTracker.initialized {
		// 首次初始化
		m.dataTracker.lastCount = currentPoints
		m.dataTracker.lastBytesCount = currentBytes
		m.dataTracker.lastTime = now
		m.dataTracker.initialized = true
		m.dataTracker.currentRate = 0
		m.dataTracker.currentByteRate = 0
		return
	}

	// 计算时间差值（秒）
	timeDiff := now.Sub(m.dataTracker.lastTime).Seconds()

	// 如果时间差小于1秒，使用上次的速率
	if timeDiff < 1.0 {
		m.DataMetrics.DataPointsPerSecond = m.dataTracker.currentRate
		m.DataMetrics.BytesPerSecond = m.dataTracker.currentByteRate
		return
	}

	// 计算数据点差值
	pointsDiff := currentPoints - m.dataTracker.lastCount
	bytesDiff := currentBytes - m.dataTracker.lastBytesCount

	// 计算实时速率
	if timeDiff > 0 {
		m.dataTracker.currentRate = float64(pointsDiff) / timeDiff
		m.dataTracker.currentByteRate = float64(bytesDiff) / timeDiff
	}

	// 更新指标
	m.DataMetrics.DataPointsPerSecond = m.dataTracker.currentRate
	m.DataMetrics.BytesPerSecond = m.dataTracker.currentByteRate

	// 更新跟踪器状态
	m.dataTracker.lastCount = currentPoints
	m.dataTracker.lastBytesCount = currentBytes
	m.dataTracker.lastTime = now
}

// UpdateDataMetrics 更新数据处理指标
func (m *LightweightMetrics) UpdateDataMetrics(totalPoints int64, bytesProcessed int64, latencyMS float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	m.DataMetrics.TotalDataPoints = totalPoints
	m.DataMetrics.TotalBytesProcessed = bytesProcessed
	m.DataMetrics.LastDataPointTime = now

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

	// 使用实时速率计算
	m.calculateRealTimeRate(totalPoints, bytesProcessed, now)

	m.LastUpdated = now
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

// GetMetrics 获取所有指标的快照。
// 以前直接 *metrics = *m 复制整个结构体（包括互斥锁和内部状态），
// 并且在只持有读锁时写缓存字段，存在数据竞争（go vet copylocks 也会报错）。
// 现在只复制导出的指标字段，缓存使用独立的互斥锁保护。
func (m *LightweightMetrics) GetMetrics() *LightweightMetrics {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	if m.cache != nil && m.cache.data != nil && time.Since(m.cache.lastUpdate) < m.cache.ttl {
		return m.cache.data
	}

	m.mu.RLock()
	snapshot := &LightweightMetrics{
		startTime:          m.startTime,
		SystemMetrics:      m.SystemMetrics,
		GatewayMetrics:     m.GatewayMetrics,
		DataMetrics:        m.DataMetrics,
		ConnectionMetrics:  m.ConnectionMetrics,
		RuleMetrics:        m.RuleMetrics,
		PerformanceMetrics: m.PerformanceMetrics,
		ErrorMetrics:       m.ErrorMetrics,
		LastUpdated:        m.LastUpdated,
	}
	m.mu.RUnlock()

	if m.cache != nil {
		m.cache.data = snapshot
		m.cache.lastUpdate = time.Now()
	}
	return snapshot
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
	result += fmt.Sprintf("iot_gateway_disk_usage_percent %.2f\n", m.SystemMetrics.DiskUsagePercent)
	result += fmt.Sprintf("iot_gateway_system_memory_percent %.2f\n", m.SystemMetrics.SystemMemoryPercent)
	result += fmt.Sprintf("iot_gateway_network_in_bytes %d\n", m.SystemMetrics.NetworkInBytes)
	result += fmt.Sprintf("iot_gateway_network_out_bytes %d\n", m.SystemMetrics.NetworkOutBytes)
	result += fmt.Sprintf("iot_gateway_network_in_packets %d\n", m.SystemMetrics.NetworkInPackets)
	result += fmt.Sprintf("iot_gateway_network_out_packets %d\n", m.SystemMetrics.NetworkOutPackets)
	result += fmt.Sprintf("iot_gateway_network_in_bytes_per_sec %.2f\n", m.SystemMetrics.NetworkInBytesPerSec)
	result += fmt.Sprintf("iot_gateway_network_out_bytes_per_sec %.2f\n", m.SystemMetrics.NetworkOutBytesPerSec)
	result += fmt.Sprintf("iot_gateway_network_in_packets_per_sec %.2f\n", m.SystemMetrics.NetworkInPacketsPerSec)
	result += fmt.Sprintf("iot_gateway_network_out_packets_per_sec %.2f\n", m.SystemMetrics.NetworkOutPacketsPerSec)
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

// SetUpdateCallback 设置自定义更新回调函数
func (m *LightweightMetrics) SetUpdateCallback(callback func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCallback = callback
	log.Info().Msg("🔗 更新回调函数已设置")
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
	now := time.Now()
	m.DataMetrics.TotalDataPoints = totalDataPoints

	// 使用实时速率计算而不是平均值
	m.calculateRealTimeRate(totalDataPoints, m.DataMetrics.TotalBytesProcessed, now)

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

				// 调用自定义更新回调（如规则引擎指标同步）
				m.mu.RLock()
				callback := m.updateCallback
				m.mu.RUnlock()

				if callback != nil {
					log.Debug().Msg("⚡ 调用更新回调函数")
					callback()
					log.Debug().Msg("✅ 更新回调函数执行完成")
				} else {
					log.Warn().Msg("⚠️ 更新回调函数为nil")
				}

				// 更新最后更新时间
				m.mu.Lock()
				m.LastUpdated = time.Now()
				m.mu.Unlock()

				// 轻量级metrics已更新（移除频繁debug日志）

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

	// 停止系统收集器
	if m.systemCollector != nil {
		if err := m.systemCollector.Stop(); err != nil {
			log.Warn().Err(err).Msg("停止系统收集器时出错")
		} else {
			log.Info().Msg("系统收集器已停止")
		}
	}

	log.Info().Msg("轻量级metrics自动更新已停止")
}
