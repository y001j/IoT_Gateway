package services

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/y001j/iot-gateway/internal/web/models"
	"gopkg.in/yaml.v2"
)

// ConfigService 配置服务接口
type ConfigService interface {
	LoadConfig() (*models.SystemConfig, error)
	SaveConfig(config *models.SystemConfig) error
	GetConfig() (*models.SystemConfig, error)
	UpdateConfig(config *models.SystemConfig) error
	ApplyConfig(config *models.SystemConfig) error
}

// configService 配置服务实现
type configService struct {
	configPath     string
	mainConfigPath string // 主配置文件路径（用于读取基础配置）
	currentConfig  *models.SystemConfig
	mu             sync.RWMutex
}

// MainConfig 主配置文件结构（用于读取现有配置）
type MainConfig struct {
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

// NewConfigService 创建配置服务
func NewConfigService(configPath string) ConfigService {
	return &configService{
		configPath: configPath,
	}
}

// NewConfigServiceWithMainConfig 创建配置服务并指定主配置文件路径
func NewConfigServiceWithMainConfig(configPath, mainConfigPath string) ConfigService {
	return &configService{
		configPath:     configPath,
		mainConfigPath: mainConfigPath,
	}
}

// LoadConfig 加载配置
func (s *configService) LoadConfig() (*models.SystemConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查配置文件是否存在
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		// 如果不存在，创建默认配置
		config := s.getDefaultConfig()
		if err := s.saveConfigFile(config); err != nil {
			return nil, fmt.Errorf("创建默认配置失败: %v", err)
		}
		s.currentConfig = config
		return config, nil
	}

	// 读取配置文件
	data, err := ioutil.ReadFile(s.configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析配置
	var config models.SystemConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	s.currentConfig = &config
	return &config, nil
}

// SaveConfig 保存配置
func (s *configService) SaveConfig(config *models.SystemConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.saveConfigFile(config); err != nil {
		return err
	}

	s.currentConfig = config
	return nil
}

// GetConfig 获取当前配置
func (s *configService) GetConfig() (*models.SystemConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentConfig == nil {
		return s.LoadConfig()
	}

	return s.currentConfig, nil
}

// UpdateConfig 更新配置
func (s *configService) UpdateConfig(config *models.SystemConfig) error {
	// 验证配置
	if err := s.validateConfig(config); err != nil {
		return fmt.Errorf("配置验证失败: %v", err)
	}

	// 保存配置
	if err := s.SaveConfig(config); err != nil {
		return fmt.Errorf("保存配置失败: %v", err)
	}

	// 应用配置
	if err := s.ApplyConfig(config); err != nil {
		return fmt.Errorf("应用配置失败: %v", err)
	}

	return nil
}

// ApplyConfig 应用配置（重启相关服务）
func (s *configService) ApplyConfig(config *models.SystemConfig) error {
	// 这里可以实现配置的热重载逻辑
	// 例如：重新初始化数据库连接池、更新NATS配置等
	
	// 对于数据库配置，可以重新创建连接池
	// 对于NATS配置，可以重新连接NATS服务器
	// 对于安全配置，可以更新API密钥等
	
	// 配置应用的逻辑应该在调用者处理，这里只是验证并保存配置
	// 实际的服务重启和配置应用应该在上层服务中处理
	
	fmt.Printf("配置已更新并应用: 网关端口=%d, 数据库路径=%s, NATS端口=%d\n", 
		config.Gateway.HTTPPort, config.Database.SQLite.Path, config.NATS.Port)
	return nil
}

// saveConfigFile 保存配置到文件
func (s *configService) saveConfigFile(config *models.SystemConfig) error {
	// 确保目录存在
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %v", err)
	}

	// 序列化配置
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	// 写入文件
	if err := ioutil.WriteFile(s.configPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}

// validateConfig 验证配置
func (s *configService) validateConfig(config *models.SystemConfig) error {
	// 验证端口范围
	if config.Gateway.HTTPPort < 1 || config.Gateway.HTTPPort > 65535 {
		return fmt.Errorf("HTTP端口必须在1-65535范围内")
	}

	if config.NATS.Port < 1 || config.NATS.Port > 65535 {
		return fmt.Errorf("NATS端口必须在1-65535范围内")
	}

	// 验证目录路径
	if config.Gateway.PluginsDir == "" {
		return fmt.Errorf("插件目录不能为空")
	}

	if config.Database.SQLite.Path == "" {
		return fmt.Errorf("数据库路径不能为空")
	}

	// 验证数据库连接池配置
	if config.Database.SQLite.MaxOpenConns < 1 {
		return fmt.Errorf("最大连接数必须大于0")
	}

	if config.Database.SQLite.MaxIdleConns < 1 {
		return fmt.Errorf("最大空闲连接数必须大于0")
	}

	if config.Database.SQLite.MaxIdleConns > config.Database.SQLite.MaxOpenConns {
		return fmt.Errorf("最大空闲连接数不能大于最大连接数")
	}

	// 验证认证配置
	if config.WebUI.Auth.PasswordMinLength < 4 {
		return fmt.Errorf("密码最小长度不能少于4位")
	}

	if config.WebUI.Auth.BcryptCost < 4 || config.WebUI.Auth.BcryptCost > 31 {
		return fmt.Errorf("Bcrypt代价必须在4-31范围内")
	}

	return nil
}

// loadMainConfig 加载主配置文件
func (s *configService) loadMainConfig() (*MainConfig, error) {
	var possiblePaths []string
	
	// 如果指定了主配置文件路径，优先使用它
	if s.mainConfigPath != "" {
		possiblePaths = []string{s.mainConfigPath}
	} else {
		// 否则尝试从几个可能的路径加载主配置文件
		possiblePaths = []string{
			"./config.yaml",
			"./config_rule_engine_test.yaml",
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
			
			var config MainConfig
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
	
	return nil, fmt.Errorf("未找到主配置文件")
}

// getDefaultConfig 获取默认配置
func (s *configService) getDefaultConfig() *models.SystemConfig {
	// 先尝试从主配置文件加载基本信息
	mainConfig, err := s.loadMainConfig()
	
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
			Auth: models.AuthConfig{
				JWTSecret:         "your-secret-key-change-in-production",
				TokenDuration:     24 * 60 * 60 * 1000000000, // 24小时（纳秒）
				RefreshDuration:   72 * 60 * 60 * 1000000000, // 72小时（纳秒）
				MaxLoginAttempts:  5,
				LockoutDuration:   15 * 60 * 1000000000, // 15分钟（纳秒）
				EnableTwoFactor:   false,
				SessionTimeout:    30 * 60 * 1000000000, // 30分钟（纳秒）
				PasswordMinLength: 8,
				BcryptCost:        12,
			},
		},
		Database: models.DatabaseConfig{
			SQLite: models.SQLiteConfig{
				Path:              "./data/auth.db", // 默认路径
				MaxOpenConns:      25,
				MaxIdleConns:      5,
				ConnMaxLifetime:   "5m",
				ConnMaxIdleTime:   "1m",
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
	if err == nil && mainConfig != nil {
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
		
		// 数据库配置 - 这是最重要的
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