package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Service 监控服务
type Service struct {
	systemCollector *SystemCollector
	config          ServiceConfig
	mutex           sync.RWMutex
	started         bool
	ctx             context.Context
	cancel          context.CancelFunc
}

// ServiceConfig 监控服务配置
type ServiceConfig struct {
	SystemCollector SystemCollectorConfig `json:"system_collector" yaml:"system_collector"`
}

// NewService 创建监控服务
func NewService(config ServiceConfig) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Service{
		config:          config,
		systemCollector: NewSystemCollector(config.SystemCollector),
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start 启动监控服务
func (s *Service) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.started {
		return fmt.Errorf("monitoring service is already started")
	}

	log.Info().Msg("Starting monitoring service...")

	// 启动系统收集器
	if err := s.systemCollector.Start(); err != nil {
		return fmt.Errorf("failed to start system collector: %w", err)
	}

	s.started = true
	log.Info().Msg("Monitoring service started successfully")
	
	return nil
}

// Stop 停止监控服务
func (s *Service) Stop() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.started {
		return nil
	}

	log.Info().Msg("Stopping monitoring service...")

	// 停止系统收集器
	if err := s.systemCollector.Stop(); err != nil {
		log.Error().Err(err).Msg("Failed to stop system collector")
	}

	s.cancel()
	s.started = false
	
	log.Info().Msg("Monitoring service stopped")
	return nil
}

// IsStarted 检查服务是否已启动
func (s *Service) IsStarted() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.started
}

// GetSystemMetrics 获取系统指标
func (s *Service) GetSystemMetrics() (*SystemMetrics, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if !s.started {
		return nil, fmt.Errorf("monitoring service is not started")
	}

	return s.systemCollector.GetMetrics()
}

// GetSystemMetricsForced 强制获取最新系统指标
func (s *Service) GetSystemMetricsForced() (*SystemMetrics, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if !s.started {
		return nil, fmt.Errorf("monitoring service is not started")
	}

	return s.systemCollector.GetMetricsForced()
}

// IsSystemHealthy 检查系统健康状态
func (s *Service) IsSystemHealthy() (bool, []string) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if !s.started {
		return false, []string{"monitoring service is not started"}
	}

	return s.systemCollector.IsHealthy()
}

// GetSystemCollector 获取系统收集器
func (s *Service) GetSystemCollector() *SystemCollector {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.systemCollector
}

// UpdateConfig 更新监控服务配置
func (s *Service) UpdateConfig(config ServiceConfig) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.config = config

	// 更新系统收集器配置
	if err := s.systemCollector.UpdateConfig(config.SystemCollector); err != nil {
		return fmt.Errorf("failed to update system collector config: %w", err)
	}

	log.Info().Msg("Monitoring service configuration updated")
	return nil
}

// GetConfig 获取监控服务配置
func (s *Service) GetConfig() ServiceConfig {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.config
}

// GetStatus 获取监控服务状态
func (s *Service) GetStatus() ServiceStatus {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	status := ServiceStatus{
		Started:   s.started,
		StartTime: time.Now(), // TODO: 记录实际启动时间
	}

	if s.started {
		// 获取系统健康状态
		healthy, issues := s.systemCollector.IsHealthy()
		status.SystemHealthy = healthy
		status.SystemIssues = issues

		// 获取系统收集器配置
		status.SystemCollectorConfig = s.systemCollector.GetConfig()
	}

	return status
}

// ServiceStatus 监控服务状态
type ServiceStatus struct {
	Started               bool                  `json:"started"`
	StartTime             time.Time             `json:"start_time"`
	SystemHealthy         bool                  `json:"system_healthy"`
	SystemIssues          []string              `json:"system_issues"`
	SystemCollectorConfig SystemCollectorConfig `json:"system_collector_config"`
}

// DefaultServiceConfig 获取默认监控服务配置
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		SystemCollector: SystemCollectorConfig{
			Enabled:         true,
			CollectInterval: 5 * time.Second,
			CacheDuration:   30 * time.Second,
			DiskPath:        "/",
			Thresholds: SystemThresholds{
				CPUWarning:     80.0,
				CPUCritical:    95.0,
				MemoryWarning:  85.0,
				MemoryCritical: 95.0,
				DiskWarning:    90.0,
				DiskCritical:   95.0,
			},
		},
	}
}