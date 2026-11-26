package api

import (
	"github.com/nats-io/nats.go"
	"github.com/y001j/iot-gateway/internal/web/middleware"
	"github.com/y001j/iot-gateway/internal/web/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RouteOptions 路由选项
type RouteOptions struct {
	// AllowedOrigins 允许跨域访问的来源白名单（web_ui.cors.allowed_origins）。
	// 为空时不返回任何 CORS 头：同源访问（同域部署或通过 Vite 代理）不受影响，
	// 第三方网站无法跨域调用 API。包含 "*" 时允许任意来源，但不允许携带凭证。
	AllowedOrigins []string
	// OpsHandler 网关的运维接口（/health、/metrics、/info、/storage）。设置后这些路径也由 Web 端口提供，
	// 部署时只需要开放一个端口（gateway.http_port 设为 0 可关闭单独的运维端口）
	OpsHandler http.Handler
}

// SetupRoutes 设置路由
func SetupRoutes(router *gin.Engine, svc *services.Services, natsConn *nats.Conn, opts ...RouteOptions) {
	var opt RouteOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	// 以前固定返回 Access-Control-Allow-Origin: * 且 Allow-Credentials: true
	router.Use(middleware.GinCORS(opt.AllowedOrigins))

	// 创建处理器
	authHandler := NewAuthHandler(svc.Auth)
	systemHandler := NewSystemHandlerWithServices(svc)
	pluginHandler := NewPluginHandler(svc.Plugin)
	ruleHandler := NewRuleHandler(svc.Rule)
	alertHandler := NewAlertHandler(svc.Alert)
	wsHandler := NewWebSocketHandler(svc, natsConn)
	wsHandler.allowedOrigins = opt.AllowedOrigins

	// 创建适配器监控处理器（如果服务可用）
	var monitoringHandler *AdapterMonitoringHandler
	if svc.AdapterMonitoring != nil {
		monitoringHandler = NewAdapterMonitoringHandler(svc.AdapterMonitoring)
	}

	// 获取系统配置
	// 令牌签发和校验必须使用同一个密钥（配置未设置时由认证服务自动生成）
	jwtSecret := svc.Auth.JWTSecret()
	// 每个请求都校验会话仍然存在：登出、修改密码后旧令牌立即失效
	sessionCheck := middleware.TokenValidator(func(token string) error {
		_, err := svc.Auth.ValidateToken(token)
		return err
	})

	// API v1 路由组
	v1 := router.Group("/api/v1")
	{
		// 认证路由（无需认证）
		auth := v1.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.Refresh)
		}

		// WebSocket 路由（需要认证）
		wsProtected := v1.Group("/ws")
		wsProtected.Use(middleware.JWTMiddleware(jwtSecret, sessionCheck))
		{
			wsProtected.GET("/realtime", wsHandler.HandleWebSocket)
		}

		// WebSocket 管理路由（需要认证）
		wsManagement := v1.Group("/websocket")
		wsManagement.Use(middleware.JWTMiddleware(jwtSecret, sessionCheck))
		{
			// WebSocket 统计信息（所有用户）
			wsManagement.GET("/stats", wsHandler.GetWebSocketStats)

			// 管理员权限路由
			adminWs := wsManagement.Group("/")
			adminWs.Use(middleware.RequireRole("admin"))
			{
				// 全局推送控制
				adminWs.POST("/push/enable", wsHandler.SetGlobalPushEnabled)

				// 客户端管理
				adminWs.POST("/clients/:clientId/push", wsHandler.SetClientPushEnabled)
				adminWs.POST("/clients/:clientId/subscriptions", wsHandler.SetClientSubscriptions)
			}
		}

		// 需要认证的路由
		protected := v1.Group("/")
		protected.Use(middleware.JWTMiddleware(jwtSecret, sessionCheck))
		// 写操作需要 operator 或 admin 角色（以前任何已登录用户都能启停插件、修改规则）
		op := middleware.RequireRole("operator")
		// 通知渠道包含 webhook 地址、邮件密码等敏感信息，仅管理员可管理
		adm := middleware.RequireRole("admin")
		{
			// 认证相关（需要登录）
			authProtected := protected.Group("/auth")
			{
				authProtected.POST("/logout", authHandler.Logout)
				authProtected.GET("/profile", authHandler.GetProfile)
				authProtected.PUT("/profile", authHandler.UpdateProfile)
				authProtected.PUT("/password", authHandler.ChangePassword)
			}

			// 系统管理
			system := protected.Group("/system")
			{
				system.GET("/status", systemHandler.GetStatus)
				system.GET("/metrics", systemHandler.GetMetrics)
				system.GET("/metrics/lightweight", systemHandler.GetLightweightMetrics)
				system.GET("/health", systemHandler.GetHealth)
				system.GET("/alert-status", systemHandler.GetSystemAlertStatus)

				// 管理员权限
				adminSystem := system.Group("/")
				adminSystem.Use(middleware.RequireRole("admin"))
				{
					adminSystem.GET("/config", systemHandler.GetConfig)
					adminSystem.PUT("/config", systemHandler.UpdateConfig)
					adminSystem.POST("/config/validate", systemHandler.ValidateConfig)
					adminSystem.POST("/restart", systemHandler.Restart)
				}
			}

			// 插件管理
			plugins := protected.Group("/plugins")
			{
				plugins.GET("", pluginHandler.GetPlugins)
				plugins.GET("/:id", pluginHandler.GetPlugin)
				plugins.POST("/:id/start", op, pluginHandler.StartPlugin)
				plugins.POST("/:id/stop", op, pluginHandler.StopPlugin)
				plugins.POST("/:id/restart", op, pluginHandler.RestartPlugin)

				// 管理员权限
				adminPlugins := plugins.Group("/")
				adminPlugins.Use(middleware.RequireRole("admin"))
				{
					adminPlugins.DELETE("/:id", pluginHandler.DeletePlugin)
					adminPlugins.PUT("/:id/config", pluginHandler.UpdatePluginConfig)
					adminPlugins.POST("/:id/config/validate", pluginHandler.ValidatePluginConfig)
				}

				// 普通用户权限
				plugins.GET("/:id/config", pluginHandler.GetPluginConfig)
				plugins.GET("/:id/logs", pluginHandler.GetPluginLogs)
				plugins.GET("/:id/stats", pluginHandler.GetPluginStats)

				// 规则管理
				rules := plugins.Group("/rules")
				{
					rules.GET("", ruleHandler.GetRules)
					rules.POST("", op, ruleHandler.CreateRule)
					rules.GET("/:id", ruleHandler.GetRule)
					rules.PUT("/:id", op, ruleHandler.UpdateRule)
					rules.DELETE("/:id", op, ruleHandler.DeleteRule)
					rules.POST("/:id/enable", op, ruleHandler.EnableRule)
					rules.POST("/:id/disable", op, ruleHandler.DisableRule)
				}
			}

			// 告警管理
			alerts := protected.Group("/alerts")
			{
				// 告警列表和基本操作
				alerts.GET("", alertHandler.GetAlerts)
				alerts.GET("/:id", alertHandler.GetAlert)
				alerts.POST("", op, alertHandler.CreateAlert)
				alerts.PUT("/:id", op, alertHandler.UpdateAlert)
				alerts.DELETE("/:id", op, alertHandler.DeleteAlert)
				alerts.POST("/:id/acknowledge", op, alertHandler.AcknowledgeAlert)
				alerts.POST("/:id/resolve", op, alertHandler.ResolveAlert)
				alerts.GET("/stats", alertHandler.GetAlertStats)

				// 告警规则管理
				alertRules := alerts.Group("/rules")
				{
					alertRules.GET("", alertHandler.GetAlertRules)
					alertRules.POST("", op, alertHandler.CreateAlertRule)
					alertRules.PUT("/:id", op, alertHandler.UpdateAlertRule)
					alertRules.DELETE("/:id", op, alertHandler.DeleteAlertRule)
					alertRules.POST("/:id/test", op, alertHandler.TestAlertRule)
				}

				// 通知渠道管理
				channels := alerts.Group("/channels")
				{
					channels.GET("", adm, alertHandler.GetNotificationChannels)
					channels.POST("", adm, alertHandler.CreateNotificationChannel)
					channels.PUT("/:id", adm, alertHandler.UpdateNotificationChannel)
					channels.DELETE("/:id", adm, alertHandler.DeleteNotificationChannel)
					channels.POST("/:id/test", adm, alertHandler.TestNotificationChannel)
				}
			}

			// 适配器监控
			if monitoringHandler != nil {
				monitoring := protected.Group("/monitoring")
				{
					// 适配器状态监控
					adapters := monitoring.Group("/adapters")
					{
						adapters.GET("/status", monitoringHandler.GetAdapterStatus)
						adapters.GET("/data-flow", monitoringHandler.GetDataFlowMetrics)
						adapters.GET("/:name/diagnostics", monitoringHandler.GetAdapterDiagnostics)
						adapters.POST("/:name/test-connection", op, monitoringHandler.TestAdapterConnection)
						adapters.GET("/:name/performance", monitoringHandler.GetAdapterPerformance)

						// 管理员权限
						adminAdapters := adapters.Group("/")
						adminAdapters.Use(middleware.RequireRole("admin"))
						{
							adminAdapters.POST("/:name/restart", monitoringHandler.RestartAdapter)
						}
					}
				}
			}

			// 本地近期数据查询（需要 storage.enabled）
			dataHandler := NewDataHandler(svc)
			data := protected.Group("/data")
			{
				data.GET("/series", dataHandler.GetSeries)
				data.GET("/latest", dataHandler.GetLatest)
				data.GET("/history", dataHandler.GetHistory)
				data.GET("/export", dataHandler.ExportCSV)
			}
			storageGroup := protected.Group("/storage")
			{
				storageGroup.GET("/status", dataHandler.GetStorageStatus)
				// 暂停/恢复/跳过/重放会影响数据完整性，仅限管理员
				storageGroup.POST("/sinks/:name/:action", adm, dataHandler.ControlSink)
			}

			// 日志管理（如果需要的话，可以后续添加）
			// logs := protected.Group("/logs")
			// {
			// 	// 日志相关的路由
			// }
		}
	}

	// 健康检查端点（无需认证）：有运维接口时返回网关的真实健康状态（以前固定返回 ok）
	if opt.OpsHandler != nil {
		ops := gin.WrapH(opt.OpsHandler)
		for _, p := range []string{"/health", "/metrics", "/info", "/storage"} {
			router.GET(p, ops)
			router.OPTIONS(p, ops)
		}
	} else {
		router.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":  "ok",
				"service": "iot-gateway-web",
			})
		})
	}

	// Swagger 文档（开发环境）
	// router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
