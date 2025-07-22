package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/web/models"
)

// SystemAlertConfig 系统告警配置
type SystemAlertConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	CheckInterval     time.Duration `yaml:"check_interval" json:"check_interval"`
	CPUThreshold      float64       `yaml:"cpu_threshold" json:"cpu_threshold"`
	MemoryThreshold   float64       `yaml:"memory_threshold" json:"memory_threshold"`
	GoroutineThreshold int          `yaml:"goroutine_threshold" json:"goroutine_threshold"`
	AlertCooldown     time.Duration `yaml:"alert_cooldown" json:"alert_cooldown"`
	AutoResolve       bool          `yaml:"auto_resolve" json:"auto_resolve"`
	ResolveTimeout    time.Duration `yaml:"resolve_timeout" json:"resolve_timeout"`
}

// SystemAlertService 系统告警服务
type SystemAlertService struct {
	config       *SystemAlertConfig
	alertService AlertService
	metrics      *metrics.LightweightMetrics
	
	// 状态管理
	mu            sync.RWMutex
	lastAlerts    map[string]*models.Alert // 缓存最后的告警以避免重复
	ctx           context.Context
	cancel        context.CancelFunc
	running       bool
}

// NewSystemAlertService 创建系统告警服务
func NewSystemAlertService(config *SystemAlertConfig, alertService AlertService, metrics *metrics.LightweightMetrics) *SystemAlertService {
	if config == nil {
		config = &SystemAlertConfig{
			Enabled:           true,
			CheckInterval:     30 * time.Second,
			CPUThreshold:      80.0,
			MemoryThreshold:   85.0,
			GoroutineThreshold: 1000,
			AlertCooldown:     5 * time.Minute,
			AutoResolve:       true,
			ResolveTimeout:    10 * time.Minute,
		}
	}
	
	return &SystemAlertService{
		config:       config,
		alertService: alertService,
		metrics:      metrics,
		lastAlerts:   make(map[string]*models.Alert),
	}
}

// Start 启动系统告警服务
func (s *SystemAlertService) Start() error {
	if !s.config.Enabled {
		log.Info().Msg("系统告警服务已禁用")
		return nil
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.running {
		return fmt.Errorf("系统告警服务已在运行")
	}
	
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.running = true
	
	go s.monitorLoop()
	
	log.Info().
		Dur("check_interval", s.config.CheckInterval).
		Float64("cpu_threshold", s.config.CPUThreshold).
		Float64("memory_threshold", s.config.MemoryThreshold).
		Int("goroutine_threshold", s.config.GoroutineThreshold).
		Msg("系统告警服务已启动")
	
	return nil
}

// Stop 停止系统告警服务
func (s *SystemAlertService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.running {
		return fmt.Errorf("系统告警服务未运行")
	}
	
	s.cancel()
	s.running = false
	
	log.Info().Msg("系统告警服务已停止")
	return nil
}

// monitorLoop 监控循环
func (s *SystemAlertService) monitorLoop() {
	ticker := time.NewTicker(s.config.CheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkSystemMetrics()
		}
	}
}

// checkSystemMetrics 检查系统指标
func (s *SystemAlertService) checkSystemMetrics() {
	if s.metrics == nil {
		log.Debug().Msg("系统指标服务未初始化")
		return
	}
	
	// 先更新系统指标
	s.metrics.UpdateSystemMetrics()
	
	// 获取当前指标
	currentMetrics := s.metrics.GetMetrics()
	systemMetrics := currentMetrics.SystemMetrics
	
	// 检查CPU使用率
	if systemMetrics.CPUUsagePercent > s.config.CPUThreshold {
		s.handleCPUAlert(systemMetrics.CPUUsagePercent)
	} else {
		s.resolveAlert("cpu_usage")
	}
	
	// 检查内存使用率
	memoryUsage := float64(systemMetrics.MemoryUsageBytes) / float64(systemMetrics.HeapSizeBytes) * 100
	if memoryUsage > s.config.MemoryThreshold {
		s.handleMemoryAlert(memoryUsage, systemMetrics.MemoryUsageBytes, systemMetrics.HeapSizeBytes)
	} else {
		s.resolveAlert("memory_usage")
	}
	
	// 检查协程数量
	if systemMetrics.GoroutineCount > s.config.GoroutineThreshold {
		s.handleGoroutineAlert(systemMetrics.GoroutineCount)
	} else {
		s.resolveAlert("goroutine_count")
	}
}

// handleCPUAlert 处理CPU告警
func (s *SystemAlertService) handleCPUAlert(cpuUsage float64) {
	alertKey := "cpu_usage"
	
	if s.shouldSkipAlert(alertKey) {
		return
	}
	
	alert, err := s.alertService.CreateAlert(&models.AlertCreateRequest{
		Title:       "CPU使用率过高",
		Description: fmt.Sprintf("当前CPU使用率为 %.2f%%，超过了配置的阈值 %.2f%%", cpuUsage, s.config.CPUThreshold),
		Level:       "warning",
		Source:      "system",
		Data: map[string]interface{}{
			"metric_type":      "cpu_usage",
			"current_value":    cpuUsage,
			"threshold_value":  s.config.CPUThreshold,
			"unit":            "percent",
			"timestamp":       time.Now().Unix(),
		},
	})
	
	if err != nil {
		log.Error().Err(err).Msg("创建CPU告警失败")
		return
	}
	
	s.mu.Lock()
	s.lastAlerts[alertKey] = alert
	s.mu.Unlock()
	
	log.Warn().
		Float64("cpu_usage", cpuUsage).
		Float64("threshold", s.config.CPUThreshold).
		Str("alert_id", alert.ID).
		Msg("CPU使用率告警已创建")
}

// handleMemoryAlert 处理内存告警
func (s *SystemAlertService) handleMemoryAlert(memoryUsage float64, usedBytes, totalBytes int64) {
	alertKey := "memory_usage"
	
	if s.shouldSkipAlert(alertKey) {
		return
	}
	
	alert, err := s.alertService.CreateAlert(&models.AlertCreateRequest{
		Title:       "内存使用率过高",
		Description: fmt.Sprintf("当前内存使用率为 %.2f%% (%d/%d bytes)，超过了配置的阈值 %.2f%%", 
			memoryUsage, usedBytes, totalBytes, s.config.MemoryThreshold),
		Level:       "warning",
		Source:      "system",
		Data: map[string]interface{}{
			"metric_type":      "memory_usage",
			"current_value":    memoryUsage,
			"threshold_value":  s.config.MemoryThreshold,
			"used_bytes":       usedBytes,
			"total_bytes":      totalBytes,
			"unit":            "percent",
			"timestamp":       time.Now().Unix(),
		},
	})
	
	if err != nil {
		log.Error().Err(err).Msg("创建内存告警失败")
		return
	}
	
	s.mu.Lock()
	s.lastAlerts[alertKey] = alert
	s.mu.Unlock()
	
	log.Warn().
		Float64("memory_usage", memoryUsage).
		Float64("threshold", s.config.MemoryThreshold).
		Int64("used_bytes", usedBytes).
		Int64("total_bytes", totalBytes).
		Str("alert_id", alert.ID).
		Msg("内存使用率告警已创建")
}

// handleGoroutineAlert 处理协程告警
func (s *SystemAlertService) handleGoroutineAlert(goroutineCount int) {
	alertKey := "goroutine_count"
	
	if s.shouldSkipAlert(alertKey) {
		return
	}
	
	alert, err := s.alertService.CreateAlert(&models.AlertCreateRequest{
		Title:       "协程数量过多",
		Description: fmt.Sprintf("当前协程数量为 %d，超过了配置的阈值 %d", goroutineCount, s.config.GoroutineThreshold),
		Level:       "warning",
		Source:      "system",
		Data: map[string]interface{}{
			"metric_type":      "goroutine_count",
			"current_value":    goroutineCount,
			"threshold_value":  s.config.GoroutineThreshold,
			"unit":            "count",
			"timestamp":       time.Now().Unix(),
		},
	})
	
	if err != nil {
		log.Error().Err(err).Msg("创建协程告警失败")
		return
	}
	
	s.mu.Lock()
	s.lastAlerts[alertKey] = alert
	s.mu.Unlock()
	
	log.Warn().
		Int("goroutine_count", goroutineCount).
		Int("threshold", s.config.GoroutineThreshold).
		Str("alert_id", alert.ID).
		Msg("协程数量告警已创建")
}

// shouldSkipAlert 检查是否应该跳过告警（冷却期）
func (s *SystemAlertService) shouldSkipAlert(alertKey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if lastAlert, exists := s.lastAlerts[alertKey]; exists {
		if lastAlert.Status == "active" && time.Since(lastAlert.CreatedAt) < s.config.AlertCooldown {
			return true
		}
	}
	
	return false
}

// resolveAlert 解决告警
func (s *SystemAlertService) resolveAlert(alertKey string) {
	if !s.config.AutoResolve {
		return
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if lastAlert, exists := s.lastAlerts[alertKey]; exists && lastAlert.Status == "active" {
		if err := s.alertService.ResolveAlert(lastAlert.ID, "system", "指标已恢复正常"); err != nil {
			log.Error().Err(err).Str("alert_id", lastAlert.ID).Msg("自动解决告警失败")
		} else {
			log.Info().Str("alert_id", lastAlert.ID).Str("metric", alertKey).Msg("系统告警已自动解决")
		}
	}
}

// GetStatus 获取服务状态
func (s *SystemAlertService) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return map[string]interface{}{
		"enabled":           s.config.Enabled,
		"running":          s.running,
		"check_interval":   s.config.CheckInterval.String(),
		"cpu_threshold":    s.config.CPUThreshold,
		"memory_threshold": s.config.MemoryThreshold,
		"goroutine_threshold": s.config.GoroutineThreshold,
		"alert_cooldown":   s.config.AlertCooldown.String(),
		"auto_resolve":     s.config.AutoResolve,
		"active_alerts":    len(s.lastAlerts),
	}
}

// IsRunning 检查服务是否运行
func (s *SystemAlertService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}