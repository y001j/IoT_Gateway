package config

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// ConfigManager manages all configuration operations
type ConfigManager interface {
	Load() error
	Validate() error
	Get(key string) interface{}
	GetAs(key string, target interface{}) error
	Watch(key string, callback func(interface{})) error
	Hot() HotReloadManager
	GetViper() *viper.Viper
}

// HotReloadManager manages hot configuration reloading
type HotReloadManager interface {
	Enable() error
	Disable() error
	IsEnabled() bool
	AddWatcher(key string, callback func(interface{})) error
}

// ConfigProvider provides configuration values
type ConfigProvider interface {
	GetString(key string) string
	GetInt(key string) int
	GetBool(key string) bool
	GetStringSlice(key string) []string
	GetStringMap(key string) map[string]interface{}
	IsSet(key string) bool
}

// Manager implements ConfigManager
type Manager struct {
	viper     *viper.Viper
	watchers  map[string][]func(interface{})
	hotReload *hotReloadManager
	mu        sync.RWMutex
}

// NewManager creates a new configuration manager
func NewManager(configPath string) (ConfigManager, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Set default values
	setDefaults(v)

	mgr := &Manager{
		viper:    v,
		watchers: make(map[string][]func(interface{})),
	}

	// 初始化热加载配置
	hotReloadConfig := &HotReloadConfig{
		Enabled:          true,  // 默认启用
		GracefulFallback: true,  // 默认优雅降级
		RetryInterval:    "30s",
		MaxRetries:       3,
	}

	mgr.hotReload = &hotReloadManager{
		manager: mgr,
		config:  hotReloadConfig,
		enabled: false,
	}

	return mgr, nil
}

// Load loads the configuration
func (m *Manager) Load() error {
	return m.viper.ReadInConfig()
}

// Validate validates the configuration
func (m *Manager) Validate() error {
	// Implement comprehensive validation logic
	return validateConfig(m.viper)
}

// Get gets a configuration value
func (m *Manager) Get(key string) interface{} {
	return m.viper.Get(key)
}

// GetAs gets a configuration value and unmarshals it into target
func (m *Manager) GetAs(key string, target interface{}) error {
	value := m.viper.Get(key)
	if value == nil {
		return fmt.Errorf("configuration key not found: %s", key)
	}

	// Convert to JSON and back to properly unmarshal
	jsonData, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal config value: %w", err)
	}

	if err := json.Unmarshal(jsonData, target); err != nil {
		return fmt.Errorf("failed to unmarshal config value: %w", err)
	}

	return nil
}

// Watch adds a watcher for configuration changes
func (m *Manager) Watch(key string, callback func(interface{})) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.watchers[key] == nil {
		m.watchers[key] = make([]func(interface{}), 0)
	}
	m.watchers[key] = append(m.watchers[key], callback)

	return nil
}

// Hot returns the hot reload manager
func (m *Manager) Hot() HotReloadManager {
	return m.hotReload
}

// GetViper returns the underlying viper instance for backward compatibility
func (m *Manager) GetViper() *viper.Viper {
	return m.viper
}

// notifyWatchers notifies all watchers for a key
func (m *Manager) notifyWatchers(key string, value interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if watchers, exists := m.watchers[key]; exists {
		for _, callback := range watchers {
			go callback(value)
		}
	}
}

// HotReloadConfig 热加载配置
type HotReloadConfig struct {
	Enabled          bool   `yaml:"enabled" json:"enabled"`
	GracefulFallback bool   `yaml:"graceful_fallback" json:"graceful_fallback"`
	RetryInterval    string `yaml:"retry_interval" json:"retry_interval"`
	MaxRetries       int    `yaml:"max_retries" json:"max_retries"`
}

// hotReloadManager implements HotReloadManager
type hotReloadManager struct {
	manager   *Manager
	config    *HotReloadConfig
	enabled   bool
	watcher   *fsnotify.Watcher
	retryCount int
	mu        sync.RWMutex
}

// Enable enables hot reloading
func (h *hotReloadManager) Enable() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 检查配置是否允许热加载
	if h.config != nil && !h.config.Enabled {
		log.Info().Msg("配置文件热加载已禁用")
		return nil
	}

	if h.enabled {
		return nil
	}

	// 尝试启动热加载
	err := h.startHotReload()
	if err != nil {
		log.Error().Err(err).Msg("启动配置文件热加载失败")
		
		// 是否优雅降级
		if h.config != nil && h.config.GracefulFallback {
			log.Warn().Msg("启用优雅降级，继续运行但不监控文件变更")
			return nil // 不返回错误，允许系统继续运行
		}
		return fmt.Errorf("配置文件热加载启动失败: %w", err)
	}

	h.enabled = true
	return nil
}

// startHotReload 启动热加载功能
func (h *hotReloadManager) startHotReload() error {
	// 尝试使用 viper 的内置热加载
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("viper WatchConfig panic")
		}
	}()

	h.manager.viper.WatchConfig()
	h.manager.viper.OnConfigChange(func(e fsnotify.Event) {
		log.Info().Str("file", e.Name).Str("op", e.Op.String()).Msg("检测到配置文件变更")
		
		// Notify all watchers
		h.manager.mu.RLock()
		for key := range h.manager.watchers {
			value := h.manager.viper.Get(key)
			go h.manager.notifyWatchers(key, value)
		}
		h.manager.mu.RUnlock()
	})

	return nil
}

// Disable disables hot reloading
func (h *hotReloadManager) Disable() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.enabled = false
	return nil
}

// IsEnabled returns whether hot reloading is enabled
func (h *hotReloadManager) IsEnabled() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.enabled
}

// AddWatcher adds a watcher for hot reload
func (h *hotReloadManager) AddWatcher(key string, callback func(interface{})) error {
	return h.manager.Watch(key, callback)
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Gateway defaults
	v.SetDefault("gateway.id", "gateway-001")
	v.SetDefault("gateway.http_port", 8082)
	v.SetDefault("gateway.log_level", "info")
	v.SetDefault("gateway.nats_url", "embedded")
	v.SetDefault("gateway.plugins_dir", "./plugins")

	// Web UI defaults
	v.SetDefault("web_ui.enabled", true)
	v.SetDefault("web_ui.port", 8081)

	// Database defaults
	v.SetDefault("database.sqlite.path", "./data/auth.db")

	// Rule engine defaults
	v.SetDefault("rule_engine.enabled", false)
	v.SetDefault("rule_engine.rules_dir", "./rules")
}

// validateConfig validates the entire configuration
func validateConfig(v *viper.Viper) error {
	// Validate required fields
	if !v.IsSet("gateway.id") || v.GetString("gateway.id") == "" {
		return fmt.Errorf("gateway.id is required")
	}

	// Validate port ranges
	if port := v.GetInt("gateway.http_port"); port < 1 || port > 65535 {
		return fmt.Errorf("gateway.http_port must be between 1 and 65535")
	}

	if port := v.GetInt("web_ui.port"); port < 1 || port > 65535 {
		return fmt.Errorf("web_ui.port must be between 1 and 65535")
	}

	// Validate log level
	logLevel := v.GetString("gateway.log_level")
	validLevels := map[string]bool{
		"trace": true, "debug": true, "info": true,
		"warn": true, "error": true, "fatal": true,
	}
	if !validLevels[logLevel] {
		return fmt.Errorf("gateway.log_level must be one of: trace, debug, info, warn, error, fatal")
	}

	return nil
}