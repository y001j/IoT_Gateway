package models

import "time"

// SystemStatus 系统状态
type SystemStatus struct {
	Status      string    `json:"status"`
	Uptime      string    `json:"uptime"`
	Version     string    `json:"version"`
	StartTime   time.Time `json:"start_time"`
	CPUUsage    float64   `json:"cpu_usage"`
	MemoryUsage float64   `json:"memory_usage"`
	DiskUsage   float64   `json:"disk_usage"`
	NetworkIn   int64     `json:"network_in"`
	NetworkOut  int64     `json:"network_out"`
	ActiveConns int       `json:"active_connections"`
	TotalConns  int64     `json:"total_connections"`
}

// SystemMetrics 系统指标
type SystemMetrics struct {
	Timestamp           time.Time `json:"timestamp"`
	DataPointsPerSecond int       `json:"data_points_per_second"`
	ActiveConnections   int       `json:"active_connections"`
	ErrorRate           float64   `json:"error_rate"`
	ResponseTimeAvg     float64   `json:"response_time_avg"`
	MemoryUsage         float64   `json:"memory_usage"`
	CPUUsage            float64   `json:"cpu_usage"`
	DiskUsage           float64   `json:"disk_usage"`
	NetworkInBytes      int64     `json:"network_in_bytes"`
	NetworkOutBytes     int64     `json:"network_out_bytes"`
}

// HealthCheck 健康检查
type HealthCheck struct {
	Service   string    `json:"service"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	Checks    []Check   `json:"checks"`
}

// Check 检查项
type Check struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Duration string `json:"duration,omitempty"`
}

// ServiceStatus 服务状态
type ServiceStatus struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Health      string    `json:"health"`
	Uptime      string    `json:"uptime"`
	LastCheck   time.Time `json:"last_check"`
	ErrorCount  int       `json:"error_count"`
	Description string    `json:"description"`
}

// SystemConfig 系统配置
type SystemConfig struct {
	Gateway  GatewayConfig  `json:"gateway"`
	NATS     NATSConfig     `json:"nats"`
	WebUI    WebUIConfig    `json:"web_ui"`
	Database DatabaseConfig `json:"database"`
	Security SecurityConfig `json:"security"`
	Rules    RulesConfig    `json:"rules"`
}

// GatewayConfig 网关配置
type GatewayConfig struct {
	ID         string        `json:"id"`
	HTTPPort   int           `json:"http_port"`
	LogLevel   string        `json:"log_level"`
	NATSURL    string        `json:"nats_url"`
	PluginsDir string        `json:"plugins_dir"`
	Metrics    MetricsConfig `json:"metrics"`
}

// MetricsConfig 指标配置
type MetricsConfig struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

// NATSConfig NATS配置
type NATSConfig struct {
	Enabled     bool               `json:"enabled"`
	Embedded    bool               `json:"embedded"`
	Host        string             `json:"host"`
	Port        int                `json:"port"`
	ClusterPort int                `json:"cluster_port"`
	MonitorPort int                `json:"monitor_port"`
	JetStream   JetStreamConfig    `json:"jetstream"`
	Cluster     ClusterConfig      `json:"cluster"`
	TLS         NATSTLSConfig      `json:"tls"`
}

// JetStreamConfig JetStream配置
type JetStreamConfig struct {
	Enabled   bool   `json:"enabled"`
	StoreDir  string `json:"store_dir"`
	MaxMemory int64  `json:"max_memory"`
	MaxFile   int64  `json:"max_file"`
}

// ClusterConfig 集群配置
type ClusterConfig struct {
	Enabled bool     `json:"enabled"`
	Name    string   `json:"name"`
	Routes  []string `json:"routes"`
}

// NATSTLSConfig NATS TLS配置
type NATSTLSConfig struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
	CAFile   string `json:"ca_file"`
}

// WebUIConfig WebUI配置
type WebUIConfig struct {
	Enabled   bool            `json:"enabled"`
	Port      int             `json:"port"`
	Auth      AuthConfig      `json:"auth"`
	WebSocket WebSocketConfig `json:"websocket"`
}

// WebSocketConfig WebSocket配置
type WebSocketConfig struct {
	MaxClients         int `json:"max_clients"`          // 最大客户端连接数
	MessageRate        int `json:"message_rate"`         // 消息发送速率限制 (每秒)
	ReadBufferSize     int `json:"read_buffer_size"`     // 读取缓冲区大小
	WriteBufferSize    int `json:"write_buffer_size"`    // 写入缓冲区大小
	CleanupInterval    int `json:"cleanup_interval"`     // 清理间隔 (秒)
	InactivityTimeout  int `json:"inactivity_timeout"`   // 不活跃超时 (秒)
	PingInterval       int `json:"ping_interval"`        // Ping间隔 (秒)
	PongTimeout        int `json:"pong_timeout"`         // Pong超时 (秒)
}


// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	SQLite SQLiteConfig `json:"sqlite"`
}

// SQLiteConfig SQLite配置
type SQLiteConfig struct {
	Path              string `json:"path"`
	MaxOpenConns      int    `json:"max_open_conns"`
	MaxIdleConns      int    `json:"max_idle_conns"`
	ConnMaxLifetime   string `json:"conn_max_lifetime"`
	ConnMaxIdleTime   string `json:"conn_max_idle_time"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	APIKeys APIKeysConfig `json:"api_keys"`
	HTTPS   HTTPSConfig   `json:"https"`
	CORS    CORSConfig    `json:"cors"`
}

// APIKeysConfig API密钥配置
type APIKeysConfig struct {
	Enabled bool     `json:"enabled"`
	Keys    []APIKey `json:"keys"`
}

// APIKey API密钥
type APIKey struct {
	Name      string `json:"name"`
	Key       string `json:"key"`
	Enabled   bool   `json:"enabled"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// HTTPSConfig HTTPS配置
type HTTPSConfig struct {
	Enabled      bool   `json:"enabled"`
	CertFile     string `json:"cert_file"`
	KeyFile      string `json:"key_file"`
	RedirectHTTP bool   `json:"redirect_http"`
}

// CORSConfig CORS配置
type CORSConfig struct {
	Enabled        bool     `json:"enabled"`
	AllowedOrigins []string `json:"allowed_origins"`
	AllowedMethods []string `json:"allowed_methods"`
	AllowedHeaders []string `json:"allowed_headers"`
	Credentials    bool     `json:"credentials"`
}

// RulesConfig 规则配置
type RulesConfig struct {
	Dir string `json:"dir"`
}

// ErrorDetail 错误详情
type ErrorDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ConfigValidationRequest 配置验证请求
type ConfigValidationRequest struct {
	Config SystemConfig `json:"config" binding:"required"`
}

// ConfigValidationResponse 配置验证响应
type ConfigValidationResponse struct {
	Valid  bool          `json:"valid"`
	Errors []ErrorDetail `json:"errors,omitempty"`
}

// RestartRequest 重启请求
type RestartRequest struct {
	Delay int `json:"delay" binding:"min=0,max=60"` // 延迟秒数
}
