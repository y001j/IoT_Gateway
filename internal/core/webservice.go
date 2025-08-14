package core

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/web/api"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"
)

// WebService provides HTTP API and web interface
type WebService struct {
	server             *http.Server
	services           *services.Services
	config             *WebConfig
	ruleEngineService  *rules.RuleEngineService
	natsConn           *nats.Conn
}

type WebConfig struct {
	Enabled bool              `mapstructure:"enabled"`
	Port    int               `mapstructure:"port"`
	Auth    models.AuthConfig `mapstructure:"auth"`
}

// NewWebService creates a new web service instance
func NewWebService(viper *viper.Viper, pluginMgr *plugin.Manager, ruleMgr rules.RuleManager, natsConn *nats.Conn, metrics *metrics.LightweightMetrics) (*WebService, error) {
	var config WebConfig
	if err := viper.UnmarshalKey("web_ui", &config); err != nil {
		return nil, fmt.Errorf("failed to parse web config: %w", err)
	}

	if !config.Enabled {
		log.Info().Msg("Web service is disabled")
		return &WebService{config: &config}, nil
	}

	// 确保数据目录存在
	dataDir := "./data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// 数据库路径
	dbPath := viper.GetString("database.sqlite.path")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "auth.db")
	}

	// 创建服务
	svc, err := services.NewServices(&services.ServiceConfig{
		Auth:          &config.Auth,
		DBPath:        dbPath,
		PluginManager: pluginMgr,
		RuleManager:   ruleMgr,
		NATSConn:      natsConn,
		Metrics:       metrics,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create web services: %w", err)
	}

	// 创建地址
	addr := fmt.Sprintf(":%d", config.Port)
	if config.Port == 0 {
		addr = ":8081"
	}

	return &WebService{
		services: svc,
		config:   &config,
		natsConn: natsConn,
		server: &http.Server{
			Addr:         addr,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
	}, nil
}

// SetRuleEngineService 设置规则引擎服务的引用
func (ws *WebService) SetRuleEngineService(ruleEngineService *rules.RuleEngineService) {
	ws.ruleEngineService = ruleEngineService
}

func (ws *WebService) Name() string {
	return "WebService"
}

func (ws *WebService) Init(cfg any) error {
	// Configuration is already loaded in NewWebService
	return nil
}

func (ws *WebService) Start(ctx context.Context) error {
	if !ws.config.Enabled {
		log.Info().Msg("Web service is disabled, skipping start")
		return nil
	}

	// 如果有规则引擎服务，获取其规则管理器并更新服务
	if ws.ruleEngineService != nil {
		ruleManager := ws.ruleEngineService.GetRuleManager()
		if ruleManager != nil {
			ws.services.RuleManager = ruleManager
			// 重新创建规则服务以使用新的规则管理器
			if newRuleService, err := services.NewRuleService(ruleManager); err == nil {
				ws.services.Rule = newRuleService
				log.Info().Msg("Web服务规则管理器集成成功")
			} else {
				log.Error().Err(err).Msg("重新创建规则服务失败")
			}
		} else {
			log.Warn().Msg("规则引擎服务返回的规则管理器为nil")
		}
	} else {
		log.Info().Msg("规则引擎服务未启用，Web服务使用空规则服务")
	}

	// 启动服务（包括AlertIntegrationService）
	if err := ws.services.Start(); err != nil {
		log.Error().Err(err).Msg("启动Web服务失败")
		return err
	}

	// 现在设置路由，此时规则服务已更新
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	api.SetupRoutes(router, ws.services, ws.natsConn)
	
	// 设置路由到服务器
	ws.server.Handler = router

	log.Info().Str("addr", ws.server.Addr).Msg("Starting web service")

	go func() {
		log.Info().Str("addr", ws.server.Addr).Msg("Web服务HTTP服务器正在启动...")
		if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Str("addr", ws.server.Addr).Msg("Web server failed")
		} else {
			log.Info().Str("addr", ws.server.Addr).Msg("Web服务HTTP服务器已停止")
		}
	}()

	// 等待一小段时间让服务器启动
	time.Sleep(100 * time.Millisecond)

	log.Info().Str("addr", ws.server.Addr).Msg("Web service started successfully")
	log.Info().Msg("Web服务已成功启动，可以通过以下地址访问:")
	log.Info().Msgf("  - 管理界面: http://localhost%s", ws.server.Addr)
	log.Info().Msgf("  - API文档: http://localhost%s/api/v1/swagger", ws.server.Addr)
	log.Info().Msgf("  - WebSocket: ws://localhost%s/ws", ws.server.Addr)

	return nil
}

func (ws *WebService) Stop(ctx context.Context) error {
	if !ws.config.Enabled || ws.server == nil {
		return nil
	}

	log.Info().Msg("Stopping web service")

	// 停止服务（包括AlertIntegrationService）
	if err := ws.services.Stop(); err != nil {
		log.Error().Err(err).Msg("停止Web服务失败")
	}

	// 创建带超时的上下文
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 优雅关闭服务器
	if err := ws.server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Web server shutdown failed")
		return err
	}

	// 关闭服务
	if ws.services != nil {
		ws.services.Close()
	}

	log.Info().Msg("Web service stopped")
	return nil
}
