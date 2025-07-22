package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/rules"
)

type Service interface {
	Name() string
	Init(cfg any) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type Runtime struct {
	V          *viper.Viper
	Bus        *nats.Conn
	Js         nats.JetStreamContext
	NatsServer *server.Server
	Svcs       []Service
	PluginMgr  *plugin.Manager
	Mu         sync.Mutex
	metrics    *metrics.LightweightMetrics
}

func NewRuntime(cfgPath string) (*Runtime, error) {
	v := viper.New()
	v.SetConfigFile(cfgPath)

	// 根据文件扩展名设置配置类型
	ext := filepath.Ext(cfgPath)
	switch ext {
	case ".yaml", ".yml":
		v.SetConfigType("yaml")
	case ".json":
		v.SetConfigType("json")
	case ".toml":
		v.SetConfigType("toml")
	default:
		// 默认尝试YAML
		v.SetConfigType("yaml")
	}

	v.AutomaticEnv()
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	// logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	level, _ := zerolog.ParseLevel(v.GetString("gateway.log_level"))
	zerolog.SetGlobalLevel(level)

	// nats embedded or external
	natsURL := v.GetString("gateway.nats_url")
	var nc *nats.Conn
	var js nats.JetStreamContext
	var natsServer *server.Server
	var err error

	if natsURL == "embedded" {
		// 使用更可靠的方法启动嵌入式 NATS
		port := 4222
		var serverReady bool

		// 先检查是否已有运行中的 NATS 服务器
		log.Info().Int("port", port).Msg("检查现有 NATS 服务器")

		// 尝试连接到现有服务器
		testConn, err := nats.Connect(fmt.Sprintf("nats://127.0.0.1:%d", port),
			nats.Timeout(1*time.Second))

		if err == nil {
			// 成功连接到现有服务器
			log.Info().Int("port", port).Msg("检测到现有 NATS 服务器")
			testConn.Close() // 关闭测试连接
			serverReady = true
		} else {
			// 没有现有服务器，检查端口是否可用
			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				// 端口被占用，尝试备用端口
				log.Warn().Int("port", port).Msg("端口被占用，尝试备用端口")
				port = 14222
				ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
				if err != nil {
					return nil, fmt.Errorf("无法找到可用的端口: %w", err)
				}
			}
			ln.Close() // 关闭测试监听器

			// 配置嵌入式 NATS 服务器
			log.Info().Int("port", port).Msg("配置嵌入式 NATS 服务器")
			opts := &server.Options{
				ServerName: "embedded-nats",
				Host:       "127.0.0.1",
				Port:       port,
				JetStream:  true,
				StoreDir:   "./data/jetstream",
				Debug:      false, // 设置为 true 可查看更多调试信息
				Trace:      false,
				NoLog:      false,
				LogFile:    "", // 留空表示使用标准输出
			}

			// 创建并启动服务器
			natsServer, err = server.NewServer(opts)
			if err != nil {
				return nil, fmt.Errorf("创建嵌入式 NATS 服务器失败: %w", err)
			}

			// 启动服务器
			log.Info().Int("port", port).Msg("启动嵌入式 NATS 服务器")
			go natsServer.Start()

			// 等待服务器就绪
			if !natsServer.ReadyForConnections(10 * time.Second) {
				if natsServer != nil {
					natsServer.Shutdown()
				}
				return nil, fmt.Errorf("嵌入式 NATS 服务器启动超时")
			}

			log.Info().Int("port", port).Msg("嵌入式 NATS 服务器已就绪")
			serverReady = true
		}

		// 当服务器就绪后连接到它
		if serverReady {
			natsURL = fmt.Sprintf("nats://127.0.0.1:%d", port)
			log.Info().Str("url", natsURL).Msg("连接到 NATS 服务器")

			// 使用重试机制连接
			var connectErr error
			for i := 0; i < 5; i++ {
				nc, connectErr = nats.Connect(natsURL,
					nats.Timeout(2*time.Second),
					nats.RetryOnFailedConnect(true),
					nats.MaxReconnects(5))

				if connectErr == nil {
					log.Info().Str("url", natsURL).Msg("成功连接到 NATS 服务器")
					break
				}

				log.Warn().Err(connectErr).Int("attempt", i+1).Msg("连接失败，重试")
				time.Sleep(time.Second)
			}

			if connectErr != nil {
				if natsServer != nil {
					natsServer.Shutdown()
				}
				return nil, fmt.Errorf("无法连接到 NATS 服务器: %w", connectErr)
			}
		} else {
			return nil, fmt.Errorf("无法启动或连接到 NATS 服务器")
		}

	} else if natsURL != "" {
		// 连接到外部 NATS 服务器
		log.Info().Str("url", natsURL).Msg("连接外部 NATS 服务器")
		nc, err = nats.Connect(natsURL)
	} else {
		// 默认连接
		log.Info().Str("url", nats.DefaultURL).Msg("使用默认 NATS 连接")
		nc, err = nats.Connect(nats.DefaultURL)
	}

	if err != nil {
		return nil, fmt.Errorf("连接 NATS 失败: %w", err)
	}

	// 创建 JetStream 上下文
	js, err = nc.JetStream()
	if err != nil {
		nc.Close()
		if natsServer != nil {
			natsServer.Shutdown()
		}
		return nil, fmt.Errorf("创建 JetStream 上下文失败: %w", err)
	}

	rt := &Runtime{V: v, Bus: nc, Js: js, NatsServer: natsServer}

	// 初始化插件管理器
	pluginManager := plugin.NewManager(v.GetString("gateway.plugins_dir"), rt.Bus, v)
	if err := pluginManager.Init(nil); err != nil {
		log.Fatal().Err(err).Msg("初始化插件管理器失败")
	}
	rt.PluginMgr = pluginManager

	// 注册规则引擎服务
	var ruleEngineService *rules.RuleEngineService
	if v.GetBool("rule_engine.enabled") {
		ruleEngineService = rules.NewRuleEngineService()
		// 传递Runtime连接到规则引擎服务
		ruleEngineService.SetRuntime(rt)
		
		// 注册Transform和Forward动作处理器
		// 注意：这些处理器需要NATS连接，将在Start阶段设置
		
		rt.RegisterService(ruleEngineService)
		log.Info().Msg("规则引擎服务已注册")
	}

	// 注册Web服务（稍后在Start阶段获取规则管理器）
	if v.GetBool("web_ui.enabled") {
		webService, err := NewWebService(v, pluginManager, nil, nc, rt.metrics)
		if err != nil {
			log.Error().Err(err).Msg("创建Web服务失败")
		} else {
			// 如果有规则引擎服务，设置其引用
			if ruleEngineService != nil {
				webService.SetRuleEngineService(ruleEngineService)
			}
			rt.RegisterService(webService)
			log.Info().Msg("Web服务已注册")
		}
	}

	// 初始化轻量级指标收集器
	metrics.InitLightweightMetrics() // 初始化全局单例
	rt.metrics = metrics.GetLightweightMetrics() // 使用全局单例
	
	// 更新网关基础指标
	rt.metrics.UpdateGatewayMetrics(
		"running",
		cfgPath,
		v.GetString("gateway.plugins_dir"),
		v.GetInt("web_ui.port"),
		v.GetInt("gateway.http_port"),
	)

	// 启动Gateway主服务HTTP服务器 (用于metrics和健康检查)
	gatewayPort := v.GetString("gateway.http_port")
	if gatewayPort == "" {
		gatewayPort = "8080"
	}

	go func() {
		mux := http.NewServeMux()

		// 轻量级指标端点 - JSON格式
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			format := r.URL.Query().Get("format")
			if format == "" {
				format = "json"
			}
			
			// 更新系统指标
			rt.UpdateMetrics()
			
			switch format {
			case "json":
				w.Header().Set("Content-Type", "application/json")
				data, err := rt.metrics.ToJSON()
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write(data)
			case "text", "plain":
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(rt.metrics.ToPlainText()))
			default:
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "不支持的格式，支持的格式: json, text, plain"})
			}
		})

		// 健康检查端点
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":    "healthy",
				"timestamp": time.Now().Format(time.RFC3339),
				"services": map[string]string{
					"nats":    "running",
					"gateway": "running",
				},
			})
		})

		// 系统信息端点
		mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			natsPort := "N/A"
			if natsServer != nil {
				natsPort = fmt.Sprintf("%d", natsServer.Addr().(*net.TCPAddr).Port)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name":         "IoT Gateway",
				"version":      "1.0.0",
				"nats_port":    natsPort,
				"gateway_port": gatewayPort,
			})
		})

		log.Info().Str("port", gatewayPort).Msg("启动Gateway主服务HTTP服务器")
		if err := http.ListenAndServe(":"+gatewayPort, mux); err != nil {
			log.Error().Err(err).Msg("Gateway主服务HTTP服务器启动失败")
		}
	}()

	// watch config
	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		log.Info().Str("file", e.Name).Msg("config changed (todo: hot reload)")
	})

	return rt, nil
}

func (r *Runtime) RegisterService(svc Service) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.Svcs = append(r.Svcs, svc)
}

func (r *Runtime) GetBus() *nats.Conn { return r.Bus }

func (r *Runtime) Start(ctx context.Context) error {
	// 启动插件管理器
	if err := r.PluginMgr.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("启动插件管理器失败")
	}

	// 初始化并启动所有服务
	for _, s := range r.Svcs {
		// 先初始化服务
		var cfg interface{}
		if s.Name() == "rule-engine" {
			// 为规则引擎服务获取配置
			cfg = r.V.Get("rule_engine")
			if cfg == nil {
				log.Warn().Msg("规则引擎配置为空，使用默认配置")
				cfg = map[string]interface{}{
					"enabled":   true,
					"rules_dir": "./data/rules",
					"subject":   "iot.data.>",
				}
			}
		}

		// 调用Init方法
		if err := s.Init(cfg); err != nil {
			log.Error().Err(err).Str("service", s.Name()).Msg("服务初始化失败")
			return fmt.Errorf("服务 %s 初始化失败: %w", s.Name(), err)
		}

		// 调用Start方法
		if err := s.Start(ctx); err != nil {
			log.Error().Err(err).Str("service", s.Name()).Msg("服务启动失败")
			return fmt.Errorf("服务 %s 启动失败: %w", s.Name(), err)
		}

		log.Info().Str("service", s.Name()).Msg("服务启动成功")
	}
	return nil
}

func (r *Runtime) Stop(ctx context.Context) {
	// 停止插件管理器
	if r.PluginMgr != nil {
		if err := r.PluginMgr.Stop(ctx); err != nil {
			log.Error().Err(err).Msg("停止插件管理器失败")
		}
	}

	for i := len(r.Svcs) - 1; i >= 0; i-- {
		_ = r.Svcs[i].Stop(ctx)
	}

	// 关闭 NATS 连接
	if r.Bus != nil {
		r.Bus.Close()
	}

	// 关闭嵌入式 NATS 服务器
	if r.NatsServer != nil {
		r.NatsServer.Shutdown()
		log.Info().Msg("嵌入式 NATS 服务器已关闭")
	}
}

// GetMetrics 获取轻量级指标收集器
func (r *Runtime) GetMetrics() *metrics.LightweightMetrics {
	return r.metrics
}

// UpdateMetrics 更新运行时指标
func (r *Runtime) UpdateMetrics() {
	if r.metrics != nil {
		// 更新适配器和连接器计数
		if r.PluginMgr != nil {
			plugins := r.PluginMgr.GetPlugins()
			var totalAdapters, runningAdapters, totalSinks, runningSinks int
			
			for _, plugin := range plugins {
				if plugin.Type == "adapter" {
					totalAdapters++
					if plugin.Status == "running" {
						runningAdapters++
					}
				} else if plugin.Type == "sink" {
					totalSinks++
					if plugin.Status == "running" {
						runningSinks++
					}
				}
			}
			
			// 更新网关指标
			r.metrics.GatewayMetrics.TotalAdapters = totalAdapters
			r.metrics.GatewayMetrics.RunningAdapters = runningAdapters
			r.metrics.GatewayMetrics.TotalSinks = totalSinks
			r.metrics.GatewayMetrics.RunningSinks = runningSinks
			r.metrics.GatewayMetrics.NATSConnected = (r.Bus != nil && r.Bus.IsConnected())
			
			if r.Bus != nil {
				r.metrics.GatewayMetrics.NATSConnectionURL = r.Bus.ConnectedUrl()
			}
		}
		
		// 更新系统指标
		r.metrics.UpdateSystemMetrics()
	}
}
