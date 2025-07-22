package api

import (
	"github.com/y001j/iot-gateway/internal/web/middleware"
	"github.com/y001j/iot-gateway/internal/web/services"
	"github.com/nats-io/nats.go"

	"github.com/gin-gonic/gin"
)

// SetupRoutes 设置路由
func SetupRoutes(router *gin.Engine, svc *services.Services, natsConn *nats.Conn) {
	// 添加CORS中间件
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		
		c.Next()
	})
	
	// 创建处理器
	authHandler := NewAuthHandler(svc.Auth)
	systemHandler := NewSystemHandler(svc.System)
	pluginHandler := NewPluginHandler(svc.Plugin)
	ruleHandler := NewRuleHandler(svc.Rule)
	alertHandler := NewAlertHandler(svc.Alert)
	wsHandler := NewWebSocketHandler(svc, natsConn)
	
	// 创建适配器监控处理器（如果服务可用）
	var monitoringHandler *AdapterMonitoringHandler
	if svc.AdapterMonitoring != nil {
		monitoringHandler = NewAdapterMonitoringHandler(svc.AdapterMonitoring)
	}

	// 获取系统配置
	config, err := svc.System.GetConfig()
	if err != nil {
		panic("无法获取系统配置：" + err.Error())
	}

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
		wsProtected.Use(middleware.JWTMiddleware(config.WebUI.Auth.JWTSecret))
		{
			wsProtected.GET("/realtime", wsHandler.HandleWebSocket)
		}

		// 需要认证的路由
		protected := v1.Group("/")
		protected.Use(middleware.JWTMiddleware(config.WebUI.Auth.JWTSecret))
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
				plugins.POST("/:id/start", pluginHandler.StartPlugin)
				plugins.POST("/:id/stop", pluginHandler.StopPlugin)
				plugins.POST("/:id/restart", pluginHandler.RestartPlugin)

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
					rules.POST("", ruleHandler.CreateRule)
					rules.GET("/:id", ruleHandler.GetRule)
					rules.PUT("/:id", ruleHandler.UpdateRule)
					rules.DELETE("/:id", ruleHandler.DeleteRule)
					rules.POST("/:id/enable", ruleHandler.EnableRule)
					rules.POST("/:id/disable", ruleHandler.DisableRule)
				}
			}

			// 告警管理
			alerts := protected.Group("/alerts")
			{
				// 告警列表和基本操作
				alerts.GET("", alertHandler.GetAlerts)
				alerts.GET("/:id", alertHandler.GetAlert)
				alerts.POST("", alertHandler.CreateAlert)
				alerts.PUT("/:id", alertHandler.UpdateAlert)
				alerts.DELETE("/:id", alertHandler.DeleteAlert)
				alerts.POST("/:id/acknowledge", alertHandler.AcknowledgeAlert)
				alerts.POST("/:id/resolve", alertHandler.ResolveAlert)
				alerts.GET("/stats", alertHandler.GetAlertStats)

				// 告警规则管理
				alertRules := alerts.Group("/rules")
				{
					alertRules.GET("", alertHandler.GetAlertRules)
					alertRules.POST("", alertHandler.CreateAlertRule)
					alertRules.PUT("/:id", alertHandler.UpdateAlertRule)
					alertRules.DELETE("/:id", alertHandler.DeleteAlertRule)
					alertRules.POST("/:id/test", alertHandler.TestAlertRule)
				}

				// 通知渠道管理
				channels := alerts.Group("/channels")
				{
					channels.GET("", alertHandler.GetNotificationChannels)
					channels.POST("", alertHandler.CreateNotificationChannel)
					channels.PUT("/:id", alertHandler.UpdateNotificationChannel)
					channels.DELETE("/:id", alertHandler.DeleteNotificationChannel)
					channels.POST("/:id/test", alertHandler.TestNotificationChannel)
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
						adapters.POST("/:name/test-connection", monitoringHandler.TestAdapterConnection)
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

			// 日志管理（如果需要的话，可以后续添加）
			// logs := protected.Group("/logs")
			// {
			// 	// 日志相关的路由
			// }
		}
	}

	// 健康检查端点（无需认证）
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "iot-gateway-web",
		})
	})

	// Swagger 文档（开发环境）
	// router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
