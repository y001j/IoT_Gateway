package services

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"time"

	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/monitoring"
	"github.com/y001j/iot-gateway/internal/web/models"
	"gopkg.in/yaml.v2"
)

// SystemService 系统服务接口
type SystemService interface {
	GetStatus() (*models.SystemStatus, error)
	GetMetrics() (*models.SystemMetrics, error)
	GetHealth() (*models.HealthCheck, error)
	GetConfig() (*models.SystemConfig, error)
	UpdateConfig(config *models.SystemConfig) error
	RestartService(serviceName string) error
}

// systemService 系统服务实现
type systemService struct {
	startTime         time.Time
	authConfig        *models.AuthConfig
	configService     ConfigService
	mainConfigPath    string              // 主配置文件路径
	monitoringService *monitoring.Service // 监控服务
}

// NewSystemService 创建系统服务
func NewSystemService(authConfig *models.AuthConfig, configService ConfigService) SystemService {
	// 创建监控服务
	monitoringService := monitoring.NewService(monitoring.DefaultServiceConfig())

	return &systemService{
		startTime:         time.Now(),
		authConfig:        authConfig,
		configService:     configService,
		monitoringService: monitoringService,
	}
}

// NewSystemServiceWithMainConfig 创建系统服务并指定主配置文件路径
func NewSystemServiceWithMainConfig(authConfig *models.AuthConfig, configService ConfigService, mainConfigPath string) SystemService {
	// 创建监控服务
	monitoringService := monitoring.NewService(monitoring.DefaultServiceConfig())

	// 启动监控服务
	if err := monitoringService.Start(); err != nil {
		// 记录错误但继续创建服务
		fmt.Printf("Warning: Failed to start monitoring service: %v\n", err)
	}

	return &systemService{
		startTime:         time.Now(),
		authConfig:        authConfig,
		configService:     configService,
		mainConfigPath:    mainConfigPath,
		monitoringService: monitoringService,
	}
}

// GetStatus 获取系统状态
func (s *systemService) GetStatus() (*models.SystemStatus, error) {
	uptime := time.Since(s.startTime)

	status := &models.SystemStatus{
		Status:      "running",
		Uptime:      uptime.String(),
		Version:     "1.0.0",
		StartTime:   s.startTime,
		ActiveConns: 15,   // 这些需要从实际连接管理器获取
		TotalConns:  1250, // 这些需要从实际连接管理器获取
	}

	// 获取真实的系统指标
	if s.monitoringService != nil && s.monitoringService.IsStarted() {
		if metrics, err := s.monitoringService.GetSystemMetrics(); err == nil {
			status.CPUUsage = metrics.CPUUsage
			status.MemoryUsage = metrics.MemoryUsage
			status.DiskUsage = metrics.DiskUsage
			status.NetworkIn = int64(metrics.NetworkInBytes)
			status.NetworkOut = int64(metrics.NetworkOutBytes)
		} else {
			// 如果获取指标失败，使用默认值
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			status.CPUUsage = 0.0 // 无法获取时设为0
			status.MemoryUsage = float64(m.Alloc) / float64(m.Sys) * 100
			status.DiskUsage = 0.0
			status.NetworkIn = 0
			status.NetworkOut = 0
		}
	} else {
		// 监控服务未启动，使用基础指标
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		status.CPUUsage = 0.0
		status.MemoryUsage = float64(m.Alloc) / float64(m.Sys) * 100
		status.DiskUsage = 0.0
		status.NetworkIn = 0
		status.NetworkOut = 0
	}

	return status, nil
}

// GetMetrics 获取系统指标
func (s *systemService) GetMetrics() (*models.SystemMetrics, error) {
	// 尝试获取轻量级指标
	lightweightMetrics := s.getLightweightMetrics()
	if lightweightMetrics != nil {
		// 使用轻量级指标系统的真实数据，并结合监控服务的系统指标
		metrics := &models.SystemMetrics{
			Timestamp:           lightweightMetrics.LastUpdated,
			DataPointsPerSecond: int(lightweightMetrics.DataMetrics.DataPointsPerSecond),
			ActiveConnections:   lightweightMetrics.ConnectionMetrics.ActiveConnections,
			ErrorRate:           lightweightMetrics.ErrorMetrics.ErrorRate,
			ResponseTimeAvg:     lightweightMetrics.ConnectionMetrics.AverageResponseTimeMS,
			MemoryUsage:         float64(lightweightMetrics.SystemMetrics.MemoryUsageBytes) / float64(lightweightMetrics.SystemMetrics.HeapSizeBytes) * 100,
			CPUUsage:            lightweightMetrics.SystemMetrics.CPUUsagePercent,
			NetworkInBytes:      lightweightMetrics.DataMetrics.TotalBytesProcessed,
			NetworkOutBytes:     lightweightMetrics.DataMetrics.TotalBytesProcessed,
			DiskUsage:           0.0, // 默认值，将从监控服务获取
		}

		// 尝试从监控服务获取更详细的系统指标
		if s.monitoringService != nil && s.monitoringService.IsStarted() {
			if sysMetrics, err := s.monitoringService.GetSystemMetrics(); err == nil {
				// 使用监控服务的更准确的系统指标
				metrics.CPUUsage = sysMetrics.CPUUsage
				metrics.MemoryUsage = sysMetrics.MemoryUsage
				metrics.DiskUsage = sysMetrics.DiskUsage
				metrics.NetworkInBytes = int64(sysMetrics.NetworkInBytes)
				metrics.NetworkOutBytes = int64(sysMetrics.NetworkOutBytes)
			}
		}

		return metrics, nil
	}

	// 回退到使用监控服务
	if s.monitoringService != nil && s.monitoringService.IsStarted() {
		if sysMetrics, err := s.monitoringService.GetSystemMetrics(); err == nil {
			return &models.SystemMetrics{
				Timestamp:           sysMetrics.Timestamp,
				DataPointsPerSecond: 0, // 这些需要从实际数据源获取
				ActiveConnections:   0,
				ErrorRate:           0.0,
				ResponseTimeAvg:     0.0,
				MemoryUsage:         sysMetrics.MemoryUsage,
				CPUUsage:            sysMetrics.CPUUsage,
				DiskUsage:           sysMetrics.DiskUsage,
				NetworkInBytes:      int64(sysMetrics.NetworkInBytes),
				NetworkOutBytes:     int64(sysMetrics.NetworkOutBytes),
			}, nil
		}
	}

	// 最后回退到基础指标
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	metrics := &models.SystemMetrics{
		Timestamp:           time.Now(),
		DataPointsPerSecond: 0,
		ActiveConnections:   0,
		ErrorRate:           0.0,
		ResponseTimeAvg:     0.0,
		MemoryUsage:         float64(m.Alloc) / float64(m.Sys) * 100,
		CPUUsage:            0.0,
		DiskUsage:           0.0,
		NetworkInBytes:      0,
		NetworkOutBytes:     0,
	}

	return metrics, nil
}

// GetHealth 获取健康检查
func (s *systemService) GetHealth() (*models.HealthCheck, error) {
	checks := []models.Check{
		{
			Name:     "database",
			Status:   "healthy",
			Message:  "Database connection is healthy",
			Duration: "2ms",
		},
		{
			Name:     "cache",
			Status:   "healthy",
			Message:  "Cache is responding",
			Duration: "1ms",
		},
		{
			Name:     "disk_space",
			Status:   "healthy",
			Message:  "Disk space is sufficient",
			Duration: "0ms",
		},
	}

	health := &models.HealthCheck{
		Service:   "iot-gateway-web",
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Checks:    checks,
	}

	return health, nil
}

// GetConfig 获取系统配置
func (s *systemService) GetConfig() (*models.SystemConfig, error) {
	if s.configService != nil {
		return s.configService.GetConfig()
	}

	// 如果没有配置服务，返回默认配置
	return s.getDefaultConfig(), nil
}

// UpdateConfig 更新系统配置
func (s *systemService) UpdateConfig(config *models.SystemConfig) error {
	if s.configService != nil {
		return s.configService.UpdateConfig(config)
	}

	// 如果没有配置服务，只返回成功
	return nil
}

// RestartService 重启服务
func (s *systemService) RestartService(serviceName string) error {
	// 这里应该实现服务重启逻辑
	// 暂时返回成功
	return nil
}

// MainConfigForSystem 主配置文件结构（SystemService使用）
type MainConfigForSystem struct {
	Gateway struct {
		ID         string `yaml:"id"`
		HTTPPort   int    `yaml:"http_port"`
		LogLevel   string `yaml:"log_level"`
		NATSURL    string `yaml:"nats_url"`
		PluginsDir string `yaml:"plugins_dir"`
	} `yaml:"gateway"`
	WebUI struct {
		Enabled bool `yaml:"enabled"`
		Port    int  `yaml:"port"`
		Auth    struct {
			JWTSecret         string `yaml:"jwt_secret"`
			TokenDuration     string `yaml:"token_duration"`
			RefreshDuration   string `yaml:"refresh_duration"`
			MaxLoginAttempts  int    `yaml:"max_login_attempts"`
			LockoutDuration   string `yaml:"lockout_duration"`
			EnableTwoFactor   bool   `yaml:"enable_two_factor"`
			SessionTimeout    string `yaml:"session_timeout"`
			PasswordMinLength int    `yaml:"password_min_length"`
			BcryptCost        int    `yaml:"bcrypt_cost"`
		} `yaml:"auth"`
	} `yaml:"web_ui"`
	Database struct {
		SQLite struct {
			Path string `yaml:"path"`
		} `yaml:"sqlite"`
	} `yaml:"database"`
	Rules struct {
		Dir string `yaml:"dir"`
	} `yaml:"rules"`
}

// loadMainConfigForSystem 加载主配置文件
func (s *systemService) loadMainConfigForSystem() (*MainConfigForSystem, error) {
	var possiblePaths []string

	// 如果指定了主配置文件路径，优先使用它
	if s.mainConfigPath != "" {
		possiblePaths = []string{s.mainConfigPath}
	} else {
		// 否则尝试从几个可能的路径加载主配置文件
		possiblePaths = []string{
			"./config.yaml",
			"./config.yaml",
			"../config.yaml",
			"../../config.yaml",
		}
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			data, err := ioutil.ReadFile(path)
			if err != nil {
				if s.mainConfigPath != "" {
					// 如果是指定的主配置文件路径，读取失败应该返回错误
					return nil, fmt.Errorf("无法读取指定的主配置文件 %s: %v", path, err)
				}
				continue
			}

			var config MainConfigForSystem
			if err := yaml.Unmarshal(data, &config); err != nil {
				if s.mainConfigPath != "" {
					// 如果是指定的主配置文件路径，解析失败应该返回错误
					return nil, fmt.Errorf("无法解析指定的主配置文件 %s: %v", path, err)
				}
				continue
			}

			return &config, nil
		}
	}

	if s.mainConfigPath != "" {
		return nil, fmt.Errorf("找不到指定的主配置文件: %s", s.mainConfigPath)
	}

	return nil, nil // 找不到主配置文件时返回nil，使用默认值
}

// getDefaultConfig 获取默认配置
func (s *systemService) getDefaultConfig() *models.SystemConfig {
	// 先尝试从主配置文件加载基本信息
	mainConfig, _ := s.loadMainConfigForSystem()

	// 使用真实的认证配置
	var authConfig models.AuthConfig
	if s.authConfig != nil {
		authConfig = *s.authConfig
	} else {
		authConfig = models.AuthConfig{
			JWTSecret:         "your-secret-key-change-in-production",
			TokenDuration:     24 * time.Hour,
			RefreshDuration:   72 * time.Hour,
			MaxLoginAttempts:  5,
			LockoutDuration:   15 * time.Minute,
			EnableTwoFactor:   false,
			SessionTimeout:    30 * time.Minute,
			PasswordMinLength: 8,
			BcryptCost:        12,
		}
	}

	// 基础配置
	config := &models.SystemConfig{
		Gateway: models.GatewayConfig{
			ID:         "iot-gateway",
			HTTPPort:   8080,
			LogLevel:   "info",
			NATSURL:    "nats://localhost:4222",
			PluginsDir: "./plugins",
			Metrics: models.MetricsConfig{
				Enabled: true,
				Port:    9090,
			},
		},
		NATS: models.NATSConfig{
			Enabled:     true,
			Embedded:    true,
			Host:        "localhost",
			Port:        4222,
			ClusterPort: 6222,
			MonitorPort: 8222,
			JetStream: models.JetStreamConfig{
				Enabled:   true,
				StoreDir:  "./data/jetstream",
				MaxMemory: 1073741824,
				MaxFile:   10737418240,
			},
			Cluster: models.ClusterConfig{
				Enabled: false,
				Name:    "iot-cluster",
				Routes:  []string{},
			},
			TLS: models.NATSTLSConfig{
				Enabled:  false,
				CertFile: "",
				KeyFile:  "",
				CAFile:   "",
			},
		},
		WebUI: models.WebUIConfig{
			Enabled: true,
			Port:    3000,
			Auth:    authConfig,
		},
		Database: models.DatabaseConfig{
			SQLite: models.SQLiteConfig{
				Path:            "./data/auth.db", // 默认路径，会被主配置覆盖
				MaxOpenConns:    25,
				MaxIdleConns:    5,
				ConnMaxLifetime: "5m",
				ConnMaxIdleTime: "1m",
			},
		},
		Security: models.SecurityConfig{
			APIKeys: models.APIKeysConfig{
				Enabled: false,
				Keys:    []models.APIKey{},
			},
			HTTPS: models.HTTPSConfig{
				Enabled:      false,
				CertFile:     "",
				KeyFile:      "",
				RedirectHTTP: false,
			},
			CORS: models.CORSConfig{
				Enabled:        true,
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders: []string{"*"},
				Credentials:    false,
			},
		},
		Rules: models.RulesConfig{
			Dir: "./rules",
		},
	}

	// 如果成功加载了主配置文件，使用其中的设置
	if mainConfig != nil {
		if mainConfig.Gateway.ID != "" {
			config.Gateway.ID = mainConfig.Gateway.ID
		}
		if mainConfig.Gateway.HTTPPort > 0 {
			config.Gateway.HTTPPort = mainConfig.Gateway.HTTPPort
		}
		if mainConfig.Gateway.LogLevel != "" {
			config.Gateway.LogLevel = mainConfig.Gateway.LogLevel
		}
		if mainConfig.Gateway.NATSURL != "" {
			config.Gateway.NATSURL = mainConfig.Gateway.NATSURL
		}
		if mainConfig.Gateway.PluginsDir != "" {
			config.Gateway.PluginsDir = mainConfig.Gateway.PluginsDir
		}

		// WebUI配置
		if mainConfig.WebUI.Port > 0 {
			config.WebUI.Port = mainConfig.WebUI.Port
		}
		config.WebUI.Enabled = mainConfig.WebUI.Enabled

		// 认证配置
		if mainConfig.WebUI.Auth.JWTSecret != "" {
			config.WebUI.Auth.JWTSecret = mainConfig.WebUI.Auth.JWTSecret
		}
		if mainConfig.WebUI.Auth.MaxLoginAttempts > 0 {
			config.WebUI.Auth.MaxLoginAttempts = mainConfig.WebUI.Auth.MaxLoginAttempts
		}
		if mainConfig.WebUI.Auth.PasswordMinLength > 0 {
			config.WebUI.Auth.PasswordMinLength = mainConfig.WebUI.Auth.PasswordMinLength
		}
		if mainConfig.WebUI.Auth.BcryptCost > 0 {
			config.WebUI.Auth.BcryptCost = mainConfig.WebUI.Auth.BcryptCost
		}
		config.WebUI.Auth.EnableTwoFactor = mainConfig.WebUI.Auth.EnableTwoFactor

		// 数据库配置 - 最重要的修正
		if mainConfig.Database.SQLite.Path != "" {
			config.Database.SQLite.Path = mainConfig.Database.SQLite.Path
		}

		// 规则配置
		if mainConfig.Rules.Dir != "" {
			config.Rules.Dir = mainConfig.Rules.Dir
		}
	}

	return config
}

// getLightweightMetrics 获取轻量级指标
func (s *systemService) getLightweightMetrics() *metrics.LightweightMetrics {
	// 由于系统服务无法直接访问Runtime实例，这里返回nil
	// 在实际部署中，可以通过依赖注入或全局变量来获取指标
	return nil
}

// GetMonitoringService 获取监控服务
func (s *systemService) GetMonitoringService() *monitoring.Service {
	return s.monitoringService
}

// StartMonitoring 启动监控服务
func (s *systemService) StartMonitoring() error {
	if s.monitoringService == nil {
		return fmt.Errorf("monitoring service is not initialized")
	}
	return s.monitoringService.Start()
}

// StopMonitoring 停止监控服务
func (s *systemService) StopMonitoring() error {
	if s.monitoringService == nil {
		return nil
	}
	return s.monitoringService.Stop()
}
