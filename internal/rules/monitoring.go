package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// MonitoringRuleError 扩展规则错误（用于监控）
type MonitoringRuleError struct {
	*RuleError
	ID          string            `json:"id"`
	RuleID      string            `json:"rule_id,omitempty"`
	ActionType  string            `json:"action_type,omitempty"`
	StackTrace  string            `json:"stack_trace,omitempty"`
	Retry       bool              `json:"retry"`
	RetryCount  int               `json:"retry_count,omitempty"`
	LastRetryAt time.Time         `json:"last_retry_at,omitempty"`
}

// RuleMonitor 规则引擎监控器
type RuleMonitor struct {
	mu                sync.RWMutex
	metrics           *EngineMetrics
	errors            []*MonitoringRuleError
	maxErrors         int
	errorsByType      map[ErrorType]int64
	errorsByLevel     map[ErrorLevel]int64
	actionStats       map[string]*ActionStats
	ruleStats         map[string]*RuleStats
	performanceStats  *PerformanceStats
	healthStatus      HealthStatus
	healthChecks      map[string]HealthChecker
	alertThresholds   *AlertThresholds
	notificationChan  chan *MonitoringRuleError
	ctx               context.Context
	cancel            context.CancelFunc
	logger            zerolog.Logger
}

// ActionStats 动作统计
type ActionStats struct {
	TotalExecutions int64         `json:"total_executions"`
	SuccessCount    int64         `json:"success_count"`
	ErrorCount      int64         `json:"error_count"`
	TotalDuration   time.Duration `json:"total_duration"`
	AvgDuration     time.Duration `json:"avg_duration"`
	MaxDuration     time.Duration `json:"max_duration"`
	MinDuration     time.Duration `json:"min_duration"`
	LastExecuted    time.Time     `json:"last_executed"`
	LastError       string        `json:"last_error,omitempty"`
}

// RuleStats 规则统计
type RuleStats struct {
	TotalEvaluations int64         `json:"total_evaluations"`
	MatchCount       int64         `json:"match_count"`
	ErrorCount       int64         `json:"error_count"`
	TotalDuration    time.Duration `json:"total_duration"`
	AvgDuration      time.Duration `json:"avg_duration"`
	LastEvaluated    time.Time     `json:"last_evaluated"`
	LastMatched      time.Time     `json:"last_matched"`
	LastError        string        `json:"last_error,omitempty"`
}

// PerformanceStats 性能统计
type PerformanceStats struct {
	ThroughputPerSecond float64       `json:"throughput_per_second"`
	P50Duration         time.Duration `json:"p50_duration"`
	P95Duration         time.Duration `json:"p95_duration"`
	P99Duration         time.Duration `json:"p99_duration"`
	MemoryUsage         int64         `json:"memory_usage"`
	GoroutineCount      int           `json:"goroutine_count"`
	QueueLength         int           `json:"queue_length"`
	LastUpdated         time.Time     `json:"last_updated"`
}

// HealthStatus 健康状态
type HealthStatus struct {
	Status       string            `json:"status"` // healthy, degraded, unhealthy
	Message      string            `json:"message"`
	LastChecked  time.Time         `json:"last_checked"`
	CheckResults map[string]string `json:"check_results"`
}

// HealthChecker 健康检查器接口
type HealthChecker interface {
	Name() string
	Check(ctx context.Context) error
}

// AlertThresholds 报警阈值
type AlertThresholds struct {
	ErrorRateThreshold    float64       `json:"error_rate_threshold"`
	LatencyThreshold      time.Duration `json:"latency_threshold"`
	ThroughputThreshold   float64       `json:"throughput_threshold"`
	MemoryThreshold       int64         `json:"memory_threshold"`
	QueueLengthThreshold  int           `json:"queue_length_threshold"`
	ConsecutiveErrors     int           `json:"consecutive_errors"`
	CheckInterval         time.Duration `json:"check_interval"`
}

// NewRuleMonitor 创建规则监控器
func NewRuleMonitor(maxErrors int) *RuleMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	
	monitor := &RuleMonitor{
		metrics:          &EngineMetrics{},
		errors:           make([]*MonitoringRuleError, 0, maxErrors),
		maxErrors:        maxErrors,
		errorsByType:     make(map[ErrorType]int64),
		errorsByLevel:    make(map[ErrorLevel]int64),
		actionStats:      make(map[string]*ActionStats),
		ruleStats:        make(map[string]*RuleStats),
		performanceStats: &PerformanceStats{},
		healthStatus: HealthStatus{
			Status:       "healthy",
			Message:      "系统正常运行",
			CheckResults: make(map[string]string),
		},
		healthChecks:    make(map[string]HealthChecker),
		alertThresholds: getDefaultAlertThresholds(),
		notificationChan: make(chan *MonitoringRuleError, 100),
		ctx:             ctx,
		cancel:          cancel,
		logger:          log.With().Str("component", "rule_monitor").Logger(),
	}
	
	// 启动监控协程
	go monitor.run()
	
	return monitor
}

// RecordError 记录错误
func (m *RuleMonitor) RecordError(errType ErrorType, level ErrorLevel, message, details string, context map[string]string) {
	baseError := NewRuleError(errType, level, "", message).
		WithDetails(details).
		SetRetryable(shouldRetry(errType, level))
	
	// 添加上下文信息
	for k, v := range context {
		baseError.WithContext(k, v)
	}
	
	monitoringError := &MonitoringRuleError{
		RuleError: baseError,
		ID:        generateErrorID(),
		Retry:     shouldRetry(errType, level),
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 添加到错误列表（保持最大数量限制）
	if len(m.errors) >= m.maxErrors {
		// 移除最旧的错误
		m.errors = m.errors[1:]
	}
	m.errors = append(m.errors, monitoringError)
	
	// 更新统计
	m.errorsByType[errType]++
	m.errorsByLevel[level]++
	
	// 记录日志
	m.logError(monitoringError)
	
	// 发送通知（非阻塞）
	select {
	case m.notificationChan <- monitoringError:
	default:
		m.logger.Warn().Msg("错误通知队列已满，丢弃错误通知")
	}
}

// RecordRuleExecution 记录规则执行
func (m *RuleMonitor) RecordRuleExecution(ruleID string, duration time.Duration, matched bool, err error) {
	log.Debug().
		Str("rule_id", ruleID).
		Bool("matched", matched).
		Err(err).
		Msg("📊 RuleMonitor.RecordRuleExecution被调用")
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 增加调试：记录原子操作前后的值
	oldValue := atomic.LoadInt64(&m.metrics.PointsProcessed)
	atomic.AddInt64(&m.metrics.PointsProcessed, 1)
	newValue := atomic.LoadInt64(&m.metrics.PointsProcessed)
	
	log.Debug().
		Int64("points_processed_before", oldValue).
		Int64("points_processed_after", newValue).
		Msg("🔢 PointsProcessed原子计数器更新")
	
	if stats, exists := m.ruleStats[ruleID]; exists {
		stats.TotalEvaluations++
		stats.TotalDuration += duration
		stats.AvgDuration = stats.TotalDuration / time.Duration(stats.TotalEvaluations)
		stats.LastEvaluated = time.Now()
		
		if matched {
			stats.MatchCount++
			stats.LastMatched = time.Now()
			atomic.AddInt64(&m.metrics.RulesMatched, 1)
		}
		
		if err != nil {
			stats.ErrorCount++
			stats.LastError = err.Error()
		}
	} else {
		stats := &RuleStats{
			TotalEvaluations: 1,
			TotalDuration:    duration,
			AvgDuration:      duration,
			LastEvaluated:    time.Now(),
		}
		
		if matched {
			stats.MatchCount = 1
			stats.LastMatched = time.Now()
			atomic.AddInt64(&m.metrics.RulesMatched, 1)
		}
		
		if err != nil {
			stats.ErrorCount = 1
			stats.LastError = err.Error()
		}
		
		m.ruleStats[ruleID] = stats
	}
	
	m.metrics.LastProcessedAt = time.Now()
}

// RecordActionExecution 记录动作执行
func (m *RuleMonitor) RecordActionExecution(actionType string, duration time.Duration, success bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	atomic.AddInt64(&m.metrics.ActionsExecuted, 1)
	
	if stats, exists := m.actionStats[actionType]; exists {
		stats.TotalExecutions++
		stats.TotalDuration += duration
		stats.AvgDuration = stats.TotalDuration / time.Duration(stats.TotalExecutions)
		stats.LastExecuted = time.Now()
		
		if duration > stats.MaxDuration {
			stats.MaxDuration = duration
		}
		if stats.MinDuration == 0 || duration < stats.MinDuration {
			stats.MinDuration = duration
		}
		
		if success {
			stats.SuccessCount++
			atomic.AddInt64(&m.metrics.ActionsSucceeded, 1)
		} else {
			stats.ErrorCount++
			atomic.AddInt64(&m.metrics.ActionsFailed, 1)
			if err != nil {
				stats.LastError = err.Error()
			}
		}
	} else {
		stats := &ActionStats{
			TotalExecutions: 1,
			TotalDuration:   duration,
			AvgDuration:     duration,
			MaxDuration:     duration,
			MinDuration:     duration,
			LastExecuted:    time.Now(),
		}
		
		if success {
			stats.SuccessCount = 1
			atomic.AddInt64(&m.metrics.ActionsSucceeded, 1)
		} else {
			stats.ErrorCount = 1
			atomic.AddInt64(&m.metrics.ActionsFailed, 1)
			if err != nil {
				stats.LastError = err.Error()
			}
		}
		
		m.actionStats[actionType] = stats
	}
}

// GetMetrics 获取监控指标
func (m *RuleMonitor) GetMetrics() *EngineMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// 创建副本以避免并发访问问题
	metrics := &EngineMetrics{
		RulesTotal:         m.metrics.RulesTotal,
		RulesEnabled:       m.metrics.RulesEnabled,
		PointsProcessed:    atomic.LoadInt64(&m.metrics.PointsProcessed),
		RulesMatched:       atomic.LoadInt64(&m.metrics.RulesMatched),
		ActionsExecuted:    atomic.LoadInt64(&m.metrics.ActionsExecuted),
		ActionsSucceeded:   atomic.LoadInt64(&m.metrics.ActionsSucceeded),
		ActionsFailed:      atomic.LoadInt64(&m.metrics.ActionsFailed),
		ProcessingDuration: m.metrics.ProcessingDuration,
		LastProcessedAt:    m.metrics.LastProcessedAt,
	}
	
	return metrics
}

// GetErrors 获取错误列表
func (m *RuleMonitor) GetErrors(limit int) []*MonitoringRuleError {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if limit <= 0 || limit > len(m.errors) {
		limit = len(m.errors)
	}
	
	// 返回最新的错误（倒序）
	result := make([]*MonitoringRuleError, limit)
	start := len(m.errors) - limit
	for i := 0; i < limit; i++ {
		result[i] = m.errors[start+i]
	}
	
	return result
}

// GetHealthStatus 获取健康状态
func (m *RuleMonitor) GetHealthStatus() HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthStatus
}

// RegisterHealthChecker 注册健康检查器
func (m *RuleMonitor) RegisterHealthChecker(checker HealthChecker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthChecks[checker.Name()] = checker
}

// run 运行监控器
func (m *RuleMonitor) run() {
	ticker := time.NewTicker(m.alertThresholds.CheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.performHealthChecks()
			m.updatePerformanceStats()
			m.checkAlerts()
		case err := <-m.notificationChan:
			m.handleErrorNotification(err)
		}
	}
}

// performHealthChecks 执行健康检查
func (m *RuleMonitor) performHealthChecks() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	checkResults := make(map[string]string)
	overallStatus := "healthy"
	var messages []string
	
	for name, checker := range m.healthChecks {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		err := checker.Check(ctx)
		cancel()
		
		if err != nil {
			checkResults[name] = fmt.Sprintf("FAIL: %s", err.Error())
			overallStatus = "unhealthy"
			messages = append(messages, fmt.Sprintf("%s检查失败: %s", name, err.Error()))
		} else {
			checkResults[name] = "PASS"
		}
	}
	
	m.healthStatus.CheckResults = checkResults
	m.healthStatus.LastChecked = time.Now()
	
	if overallStatus == "unhealthy" {
		m.healthStatus.Status = "unhealthy"
		m.healthStatus.Message = fmt.Sprintf("健康检查失败: %v", messages)
	} else {
		m.healthStatus.Status = "healthy"
		m.healthStatus.Message = "所有健康检查通过"
	}
}

// updatePerformanceStats 更新性能统计
func (m *RuleMonitor) updatePerformanceStats() {
	// 这里可以添加性能统计的更新逻辑
	// 例如：计算吞吐量、延迟百分位数等
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.performanceStats.LastUpdated = time.Now()
	// TODO: 实现详细的性能统计计算
}

// checkAlerts 检查报警条件
func (m *RuleMonitor) checkAlerts() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// 检查错误率
	totalActions := atomic.LoadInt64(&m.metrics.ActionsExecuted)
	failedActions := atomic.LoadInt64(&m.metrics.ActionsFailed)
	
	if totalActions > 0 {
		errorRate := float64(failedActions) / float64(totalActions)
		if errorRate > m.alertThresholds.ErrorRateThreshold {
			m.logger.Warn().
				Float64("error_rate", errorRate).
				Float64("threshold", m.alertThresholds.ErrorRateThreshold).
				Msg("动作错误率超过阈值")
		}
	}
	
	// 检查队列长度
	if m.performanceStats.QueueLength > m.alertThresholds.QueueLengthThreshold {
		m.logger.Warn().
			Int("queue_length", m.performanceStats.QueueLength).
			Int("threshold", m.alertThresholds.QueueLengthThreshold).
			Msg("队列长度超过阈值")
	}
}

// handleErrorNotification 处理错误通知
func (m *RuleMonitor) handleErrorNotification(ruleError *MonitoringRuleError) {
	// 这里可以添加错误通知的处理逻辑
	// 例如：发送到外部监控系统、触发报警等
	
	if ruleError.Level == ErrorLevelCritical || ruleError.Level == ErrorLevelError {
		m.logger.Error().
			Str("error_id", ruleError.ID).
			Str("type", string(ruleError.Type)).
			Str("message", ruleError.Message).
			Msg("严重错误通知")
	}
}

// logError 记录错误日志
func (m *RuleMonitor) logError(ruleError *MonitoringRuleError) {
	logEvent := m.logger.With().
		Str("error_id", ruleError.ID).
		Str("type", string(ruleError.Type)).
		Str("level", string(ruleError.Level)).
		Str("message", ruleError.Message).
		Time("timestamp", ruleError.Timestamp)
	
	if ruleError.RuleID != "" {
		logEvent = logEvent.Str("rule_id", ruleError.RuleID)
	}
	
	if ruleError.ActionType != "" {
		logEvent = logEvent.Str("action_type", ruleError.ActionType)
	}
	
	if ruleError.Details != "" {
		logEvent = logEvent.Str("details", ruleError.Details)
	}
	
	logger := logEvent.Logger()
	
	switch ruleError.Level {
	case ErrorLevelInfo:
		logger.Info().Msg("规则引擎信息")
	case ErrorLevelWarning:
		logger.Warn().Msg("规则引擎警告")
	case ErrorLevelError:
		logger.Error().Msg("规则引擎错误")
	case ErrorLevelCritical:
		logger.Error().Msg("规则引擎严重错误")
	}
}

// Close 关闭监控器
func (m *RuleMonitor) Close() {
	m.cancel()
	close(m.notificationChan)
}

// 辅助函数

func generateErrorID() string {
	return fmt.Sprintf("err_%d", time.Now().UnixNano())
}

func shouldRetry(errType ErrorType, level ErrorLevel) bool {
	// 确定哪些错误类型和级别应该重试
	if level == ErrorLevelCritical || level == ErrorLevelError {
		return errType == ErrorTypeTimeout || errType == ErrorTypeSystem
	}
	return false
}

func getDefaultAlertThresholds() *AlertThresholds {
	return &AlertThresholds{
		ErrorRateThreshold:    0.05, // 5%
		LatencyThreshold:      1 * time.Second,
		ThroughputThreshold:   100, // 每秒处理数量
		MemoryThreshold:       1024 * 1024 * 1024, // 1GB
		QueueLengthThreshold:  1000,
		ConsecutiveErrors:     5,
		CheckInterval:         30 * time.Second,
	}
}

// ToJSON 将监控数据转换为JSON
func (m *RuleMonitor) ToJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	data := map[string]interface{}{
		"metrics":           m.GetMetrics(),
		"health_status":     m.healthStatus,
		"performance_stats": m.performanceStats,
		"error_stats": map[string]interface{}{
			"by_type":  m.errorsByType,
			"by_level": m.errorsByLevel,
		},
		"action_stats": m.actionStats,
		"rule_stats":   m.ruleStats,
		"recent_errors": m.GetErrors(10),
	}
	
	return json.Marshal(data)
}