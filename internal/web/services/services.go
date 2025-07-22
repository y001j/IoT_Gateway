package services

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/nats-io/nats.go"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/metrics"

	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/rules"
)

// Services 服务容器
type Services struct {
	System              SystemService
	Auth                AuthService
	Plugin              PluginService
	Rule                RuleService
	Log                 LogService
	Alert               AlertService
	Notification        NotificationService
	AlertIntegration    *AlertIntegrationService
	AdapterMonitoring   *AdapterMonitoringService
	SystemAlert         *SystemAlertService
	store               models.UserStore
	PluginManager       plugin.PluginManager
	RuleManager         rules.RuleManager
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	Auth           *models.AuthConfig
	DBPath         string
	ConfigPath     string
	MainConfigPath string // 主配置文件路径（通过--config参数指定）
	PluginManager  plugin.PluginManager
	RuleManager    rules.RuleManager
	NATSConn       *nats.Conn
	Metrics        *metrics.LightweightMetrics
}

// NewServices 创建服务容器
func NewServices(config *ServiceConfig) (*Services, error) {
	if config == nil {
		config = &ServiceConfig{}
	}

	// 创建配置服务
	var configService ConfigService
	if config.ConfigPath != "" {
		if config.MainConfigPath != "" {
			configService = NewConfigServiceWithMainConfig(config.ConfigPath, config.MainConfigPath)
		} else {
			configService = NewConfigService(config.ConfigPath)
		}
	}

	// 创建存储 - 优先使用SQLite，失败时使用内存存储
	var store models.UserStore
	var err error
	
	if config.DBPath != "" {
		// 如果有配置服务，尝试从配置中获取数据库配置
		if configService != nil {
			if systemConfig, err := configService.GetConfig(); err == nil {
				store, err = NewSQLiteStoreWithConfig(&systemConfig.Database.SQLite)
				if err != nil {
					// SQLite失败时使用内存存储
					store = NewMemoryStore()
					fmt.Printf("Warning: SQLite with config failed (%v), using memory store\n", err)
				}
			} else {
				// 配置服务失败时使用默认配置
				store, err = NewSQLiteStore(config.DBPath)
				if err != nil {
					store = NewMemoryStore()
					fmt.Printf("Warning: SQLite failed (%v), using memory store\n", err)
				}
			}
		} else {
			store, err = NewSQLiteStore(config.DBPath)
			if err != nil {
				// SQLite失败时使用内存存储
				store = NewMemoryStore()
				fmt.Printf("Warning: SQLite failed (%v), using memory store\n", err)
			}
		}
	} else {
		// 没有配置数据库路径时使用内存存储
		store = NewMemoryStore()
	}

	// 初始化插件管理器
	if err := config.PluginManager.Init(nil); err != nil {
		return nil, fmt.Errorf("初始化插件管理器失败: %v", err)
	}

	// 创建插件服务
	pluginService, err := NewPluginService(config.PluginManager)
	if err != nil {
		return nil, fmt.Errorf("创建插件服务失败: %v", err)
	}

	var ruleService RuleService
	if config.RuleManager != nil {
		ruleService, err = NewRuleService(config.RuleManager) // 使用新的RuleManager
		if err != nil {
			return nil, fmt.Errorf("创建规则服务失败: %v", err)
		}
	} else {
		// 创建一个空的规则服务，稍后再设置规则管理器
		ruleService = &emptyRuleService{}
	}

	// 创建告警服务
	var alertService AlertService
	if config.NATSConn != nil {
		alertService = NewAlertServiceWithNATS(config.NATSConn)
	} else {
		alertService = NewAlertService()
	}
	
	// 创建通知服务
	notificationService := NewNotificationService(alertService)
	
	// 创建告警集成服务（如果有NATS连接）
	var alertIntegration *AlertIntegrationService
	if config.NATSConn != nil {
		alertIntegration = NewAlertIntegrationService(alertService, notificationService, config.NATSConn, config.RuleManager)
	}
	
	// 创建适配器监控服务（如果有NATS连接和插件管理器）
	var adapterMonitoring *AdapterMonitoringService
	if config.NATSConn != nil && config.PluginManager != nil {
		adapterMonitoring = NewAdapterMonitoringService(config.PluginManager, config.NATSConn)
	}
	
	// 创建系统告警服务（如果有指标服务）
	var systemAlert *SystemAlertService
	if config.Metrics != nil {
		systemAlert = NewSystemAlertService(nil, alertService, config.Metrics)
	}

	// 创建SystemService
	var systemService SystemService
	if config.MainConfigPath != "" {
		systemService = NewSystemServiceWithMainConfig(config.Auth, configService, config.MainConfigPath)
	} else {
		systemService = NewSystemService(config.Auth, configService)
	}

	services := &Services{
		System:            systemService,
		Auth:              NewAuthService(store, config.Auth),
		Plugin:            pluginService,
		Rule:              ruleService,
		Log:               NewLogService(),
		Alert:             alertService,
		Notification:      notificationService,
		AlertIntegration:  alertIntegration,
		AdapterMonitoring: adapterMonitoring,
		SystemAlert:       systemAlert,
		store:             store,
		PluginManager:     config.PluginManager,
		RuleManager:       config.RuleManager,
	}

	return services, nil
}

// Start 启动服务
func (s *Services) Start() error {
	log.Info().Msg("启动Web服务")
	
	// 启动告警集成服务
	if s.AlertIntegration != nil {
		log.Info().Msg("启动告警集成服务")
		if err := s.AlertIntegration.Start(); err != nil {
			log.Error().Err(err).Msg("启动告警集成服务失败")
			return fmt.Errorf("启动告警集成服务失败: %w", err)
		}
	} else {
		log.Warn().Msg("告警集成服务未配置，跳过启动")
	}

	// 适配器监控服务在构造函数中自动启动
	if s.AdapterMonitoring != nil {
		log.Info().Msg("适配器监控服务已启动")
	} else {
		log.Warn().Msg("适配器监控服务未配置，跳过启动")
	}
	
	// 启动系统告警服务
	if s.SystemAlert != nil {
		log.Info().Msg("启动系统告警服务")
		if err := s.SystemAlert.Start(); err != nil {
			log.Error().Err(err).Msg("启动系统告警服务失败")
			return fmt.Errorf("启动系统告警服务失败: %w", err)
		}
	} else {
		log.Warn().Msg("系统告警服务未配置，跳过启动")
	}

	log.Info().Msg("Web服务启动成功")
	return nil
}

// Stop 停止服务
func (s *Services) Stop() error {
	log.Info().Msg("停止Web服务")
	
	// 停止告警集成服务
	if s.AlertIntegration != nil {
		log.Info().Msg("停止告警集成服务")
		if err := s.AlertIntegration.Stop(); err != nil {
			log.Error().Err(err).Msg("停止告警集成服务失败")
		}
	}

	// 停止适配器监控服务
	if s.AdapterMonitoring != nil {
		log.Info().Msg("停止适配器监控服务")
		s.AdapterMonitoring.Stop()
	}
	
	// 停止系统告警服务
	if s.SystemAlert != nil {
		log.Info().Msg("停止系统告警服务")
		if err := s.SystemAlert.Stop(); err != nil {
			log.Error().Err(err).Msg("停止系统告警服务失败")
		}
	}

	log.Info().Msg("Web服务停止成功")
	return nil
}

// Close 关闭服务
func (s *Services) Close() error {
	// 先停止服务
	s.Stop()
	
	// 关闭数据库连接
	if store, ok := s.store.(*SQLiteStore); ok {
		return store.Close()
	}
	return nil
}

// Response 通用响应类型
type Response struct {
	Success bool        `json:"success"`
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// SuccessResponse 成功响应
func SuccessResponse(data interface{}, message string) *Response {
	return &Response{
		Success: true,
		Code:    200,
		Message: message,
		Data:    data,
	}
}

// ErrorResponseWithCode 带状态码的错误响应
func ErrorResponseWithCode(code int, message string, err error) *ErrorResponse {
	response := &ErrorResponse{
		Success: false,
		Code:    code,
		Message: message,
	}

	if err != nil {
		response.Error = err.Error()
	}

	return response
}

// PagedResponse 分页响应
type PagedResponse struct {
	Success bool        `json:"success"`
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
	Total   int         `json:"total"`
	Page    int         `json:"page"`
	Size    int         `json:"size"`
}

// NewPagedResponse 创建分页响应
func NewPagedResponse(data interface{}, total, page, size int) *PagedResponse {
	return &PagedResponse{
		Success: true,
		Code:    200,
		Message: "success",
		Data:    data,
		Total:   total,
		Page:    page,
		Size:    size,
	}
}

// emptyRuleService 空规则服务实现，用于在规则管理器不可用时提供默认行为
type emptyRuleService struct{}

func (e *emptyRuleService) GetRules(req *models.RuleListRequest) ([]models.Rule, int, error) {
	log.Warn().Msg("使用空规则服务返回空结果")
	return []models.Rule{}, 0, nil
}

func (e *emptyRuleService) GetRule(id string) (*models.Rule, error) {
	return nil, fmt.Errorf("规则服务不可用")
}

func (e *emptyRuleService) CreateRule(rule *models.RuleCreateRequest) (*models.Rule, error) {
	return nil, fmt.Errorf("规则服务不可用")
}

func (e *emptyRuleService) UpdateRule(id string, rule *models.RuleUpdateRequest) (*models.Rule, error) {
	return nil, fmt.Errorf("规则服务不可用")
}

func (e *emptyRuleService) DeleteRule(id string) error {
	return fmt.Errorf("规则服务不可用")
}

func (e *emptyRuleService) EnableRule(id string) error {
	return fmt.Errorf("规则服务不可用")
}

func (e *emptyRuleService) DisableRule(id string) error {
	return fmt.Errorf("规则服务不可用")
}

func (e *emptyRuleService) ValidateRule(rule *models.Rule) (*models.RuleValidationResponseExtended, error) {
	return nil, fmt.Errorf("规则服务不可用")
}

func (e *emptyRuleService) TestRule(req *models.RuleTestRequestExtended) (*models.RuleTestResponseExtended, error) {
	return nil, fmt.Errorf("规则服务不可用")
}

func (e *emptyRuleService) GetRuleStats(id string) (*models.RuleStatsExtended, error) {
	return nil, fmt.Errorf("规则服务不可用")
}

func (e *emptyRuleService) GetRuleExecutionHistory(id string, req *models.RuleHistoryRequest) ([]models.RuleExecution, int, error) {
	return nil, 0, fmt.Errorf("规则服务不可用")
}

func (e *emptyRuleService) GetRuleTemplates() ([]models.RuleTemplate, error) {
	return []models.RuleTemplate{}, nil
}

func (e *emptyRuleService) CreateRuleFromTemplate(templateID string, req *models.RuleFromTemplateRequest) (*models.Rule, error) {
	return nil, fmt.Errorf("规则服务不可用")
}
