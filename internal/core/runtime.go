package core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
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
	"github.com/y001j/iot-gateway/internal/rules/actions"
	"github.com/y001j/iot-gateway/internal/storage"
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
	Store      *storage.Store // 本地暂存（未启用时为 nil）
	Svcs       []Service
	PluginMgr  *plugin.Manager
	Mu         sync.Mutex
	metrics    *metrics.LightweightMetrics

	// NATS 异步错误统计（slow consumer 等），以前这类丢消息完全没有日志
	slowConsumerDrops atomic.Int64
	natsAsyncErrors   atomic.Int64
	lastNatsErrLog    atomic.Int64
	started           atomic.Bool

	// 内嵌 MQTT（gateway.mqtt）：启用时 mqttAddr 为实际监听地址，未能启动时 mqttProblem 说明原因
	mqttEnabled bool
	mqttAddr    string
	mqttProblem string
}

// 内嵌 NATS 的端口：先尝试 embeddedNATSPort（该端口上已有 NATS 时直接连接它），
// 端口被其他程序占用时改用 embeddedNATSFallbackPort。测试中改为随机空闲端口。
var (
	embeddedNATSPort         = 4222
	embeddedNATSFallbackPort = 14222
)

// natsOptions 返回网关使用的 NATS 连接参数
func (rt *Runtime) natsOptions(extra ...nats.Option) []nats.Option {
	opts := []nats.Option{
		nats.Name("iot-gateway"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.ErrorHandler(rt.handleNATSAsyncError),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				log.Warn().Err(err).Msg("NATS连接断开")
			}
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			log.Info().Str("url", c.ConnectedUrl()).Msg("NATS已重新连接")
		}),
	}
	return append(opts, extra...)
}

// handleNATSAsyncError 处理 NATS 异步错误（订阅回调处理不过来时的 slow consumer 丢消息等）
func (rt *Runtime) handleNATSAsyncError(_ *nats.Conn, sub *nats.Subscription, err error) {
	if err == nats.ErrSlowConsumer {
		rt.slowConsumerDrops.Add(1)
	} else {
		rt.natsAsyncErrors.Add(1)
	}
	now := time.Now().UnixNano()
	last := rt.lastNatsErrLog.Load()
	if now-last < int64(10*time.Second) || !rt.lastNatsErrLog.CompareAndSwap(last, now) {
		return
	}
	ev := log.Error().Err(err)
	if sub != nil {
		ev = ev.Str("subject", sub.Subject)
		if dropped, derr := sub.Dropped(); derr == nil {
			ev = ev.Int("dropped_total", dropped)
		}
	}
	ev.Int64("slow_consumer_events", rt.slowConsumerDrops.Load()).Msg("NATS异步错误（可能有消息被丢弃）")
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

	// 先创建 Runtime，NATS 连接参数中的错误回调需要引用它
	rtOpts := &Runtime{V: v}

	// 本地暂存（断网续传）配置
	storageCfg, err := storage.ConfigFromViper(v)
	if err != nil {
		return nil, fmt.Errorf("storage 配置错误: %w", err)
	}

	mqttCfg, err := mqttConfigFromViper(v)
	if err != nil {
		return nil, err
	}
	rtOpts.mqttEnabled = mqttCfg.Enabled

	// nats embedded or external
	natsURL := v.GetString("gateway.nats_url")
	var nc *nats.Conn
	var js nats.JetStreamContext
	var natsServer *server.Server

	if natsURL == "embedded" {
		// 使用更可靠的方法启动嵌入式 NATS
		port := embeddedNATSPort
		var serverReady bool

		// 先检查是否已有运行中的 NATS 服务器

		// 尝试连接到现有服务器
		testConn, err := nats.Connect(fmt.Sprintf("nats://127.0.0.1:%d", port),
			nats.Timeout(1*time.Second))

		if err == nil {
			// 成功连接到现有服务器
			testConn.Close() // 关闭测试连接
			serverReady = true
			if mqttCfg.Enabled {
				rtOpts.mqttProblem = fmt.Sprintf("端口 %d 上已有 NATS 服务器，网关连接到它而没有启动内嵌服务器，MQTT 未启动", port)
				log.Warn().Msg(rtOpts.mqttProblem)
			}
		} else {
			// 没有现有服务器，检查端口是否可用
			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				// 端口被占用，尝试备用端口
				log.Warn().Int("port", port).Msg("端口被占用，尝试备用端口")
				port = embeddedNATSFallbackPort
				ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
				if err != nil {
					return nil, fmt.Errorf("无法找到可用的端口: %w", err)
				}
			}
			_ = ln.Close() // 关闭测试监听器

			// 配置嵌入式 NATS 服务器
			opts := &server.Options{
				ServerName: "embedded-nats",
				Host:       "127.0.0.1",
				Port:       port,
				JetStream:  true,
				StoreDir:   storageDir(v),
				Debug:      false, // 设置为 true 可查看更多调试信息
				Trace:      false,
				NoLog:      false,
				LogFile:    "", // 留空表示使用标准输出
			}
			if storageCfg.Enabled {
				// 限制 JetStream 总磁盘占用；刷盘间隔决定掉电时最多丢失的数据窗口
				opts.JetStreamMaxStore = storageCfg.MaxDisk
				opts.SyncInterval = storageCfg.SyncInterval
			}
			mqttStarting := false
			if mqttCfg.Enabled {
				if err := applyMQTTOptions(opts, mqttCfg); err != nil {
					rtOpts.mqttProblem = err.Error()
					log.Error().Err(err).Msg("内嵌 MQTT 未启动")
				} else {
					mqttStarting = true
				}
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
			if mqttStarting {
				if err := waitMQTTListening(mqttCfg, 5*time.Second); err != nil {
					rtOpts.mqttProblem = err.Error()
					log.Error().Err(err).Msg("内嵌 MQTT 未启动")
				} else {
					rtOpts.mqttAddr = mqttCfg.addr()
					log.Info().Str("addr", rtOpts.mqttAddr).Msg("内嵌 MQTT 已启动（MQTT 主题 a/b 对应 NATS 主题 a.b）")
				}
			}

			serverReady = true
		}

		// 当服务器就绪后连接到它
		if serverReady {
			natsURL = fmt.Sprintf("nats://127.0.0.1:%d", port)

			// 使用重试机制连接
			var connectErr error
			for i := 0; i < 5; i++ {
				nc, connectErr = nats.Connect(natsURL, rtOpts.natsOptions(
					nats.Timeout(2*time.Second),
					nats.RetryOnFailedConnect(true))...)

				if connectErr == nil {
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
		nc, err = nats.Connect(natsURL, rtOpts.natsOptions()...)
		if mqttCfg.Enabled {
			rtOpts.mqttProblem = "使用外部 NATS 服务器，gateway.mqtt 只对内嵌服务器生效，请在外部服务器上配置 MQTT"
			log.Warn().Msg(rtOpts.mqttProblem)
		}
	} else {
		// 默认连接
		nc, err = nats.Connect(nats.DefaultURL, rtOpts.natsOptions()...)
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

	rt := rtOpts
	rt.Bus, rt.Js, rt.NatsServer = nc, js, natsServer

	// 初始化插件管理器
	pluginManager := plugin.NewManager(v.GetString("gateway.plugins_dir"), rt.Bus, v)
	if err := pluginManager.Init(nil); err != nil {
		log.Fatal().Err(err).Msg("初始化插件管理器失败")
	}
	rt.PluginMgr = pluginManager

	// 本地暂存：JetStream 不可用时只记录警告，开启缓冲的连接器退回直连模式
	if storageCfg.Enabled {
		store, err := storage.New(nc, storageCfg)
		if err != nil {
			log.Error().Err(err).Msg("本地暂存不可用，开启缓冲的连接器将退回直连模式")
		} else {
			rt.Store = store
			pluginManager.SetStore(store)
			if storageCfg.Rollup.Enabled {
				if _, err := store.StartRollup(context.Background()); err != nil {
					log.Error().Err(err).Msg("启动汇总层失败，长时间范围的查询不可用")
				}
			}
			if storageCfg.Alerts.Enabled {
				// 积压逼近保留上限、数据被淘汰、磁盘快满时发布告警（iot.alerts.triggered）
				store.StartAlertMonitor(context.Background(), nc, pluginManager.GetBufferStats)
			}
		}
	}

	// 注册规则引擎服务
	var ruleEngineService *rules.RuleEngineService
	if v.GetBool("rule_engine.enabled") {
		// 获取规则引擎配置并创建服务
		ruleEngineConfig := v.Get("rule_engine")
		if configMap, ok := ruleEngineConfig.(map[string]interface{}); ok {
			ruleEngineService = rules.NewRuleEngineServiceWithConfig(configMap)
		} else {
			ruleEngineService = rules.NewRuleEngineService()
		}
		// 传递Runtime连接到规则引擎服务
		ruleEngineService.SetRuntime(rt)
		// 注册完整的动作处理器（filter/transform/forward/aggregate），必须在服务启动前完成
		handlerOpts := actions.HandlerOptions{}
		// 聚合窗口状态持久化（默认开启，需要 JetStream）：重启后恢复，长时间窗口的统计不被打断
		v.SetDefault("rule_engine.aggregate_state.persist", true)
		if v.GetBool("rule_engine.aggregate_state.persist") && rt.Js != nil {
			handlerOpts.AggregateStateKV = rt.Js
			handlerOpts.AggregateSaveInterval = v.GetDuration("rule_engine.aggregate_state.save_interval")
		}
		actions.RegisterBuiltinHandlersWithOptions(ruleEngineService, nc, handlerOpts)

		// 注册Transform和Forward动作处理器
		// 注意：这些处理器需要NATS连接，将在Start阶段设置

		rt.RegisterService(ruleEngineService)
		log.Info().Msg("规则引擎服务已注册")
	}

	// 注册Web服务（稍后在Start阶段获取规则管理器）
	webOps := false // Web 端口是否提供运维接口
	if v.GetBool("web_ui.enabled") {
		webService, err := NewWebService(v, pluginManager, nil, nc, rt.metrics)
		if err != nil {
			log.Error().Err(err).Msg("创建Web服务失败")
		} else {
			// 如果有规则引擎服务，设置其引用
			if ruleEngineService != nil {
				webService.SetRuleEngineService(ruleEngineService)
			}
			// 运维接口同时由 Web 端口提供，部署时可以只开放一个端口
			webService.SetOpsHandler(rt.GatewayHTTPHandler(v.GetString("web_ui.port")))
			webOps = webService.config.Enabled
			rt.RegisterService(webService)
		}
	}

	// 初始化轻量级指标收集器
	metrics.InitLightweightMetrics()             // 初始化全局单例
	rt.metrics = metrics.GetLightweightMetrics() // 使用全局单例

	// 设置更新回调，用于同步规则引擎统计数据
	rt.metrics.SetUpdateCallback(func() {
		rt.updateRuleEngineMetrics()
	})

	// 更新网关基础指标
	rt.metrics.UpdateGatewayMetrics(
		"running",
		cfgPath,
		v.GetString("gateway.plugins_dir"),
		v.GetInt("web_ui.port"),
		v.GetInt("gateway.http_port"),
	)

	// 启动Gateway主服务HTTP服务器 (用于metrics和健康检查)
	gatewayPort := opsPort(v.GetString("gateway.http_port"), webOps)

	if gatewayPort != "" {
		go func() {
			srv := &http.Server{
				Addr:              ":" + gatewayPort,
				Handler:           rt.GatewayHTTPHandler(gatewayPort),
				ReadHeaderTimeout: 10 * time.Second,
			}
			log.Info().Str("port", gatewayPort).Msg("启动Gateway主服务HTTP服务器")
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error().Err(err).Msg("Gateway主服务HTTP服务器启动失败")
			}
		}()
	}

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
	log.Info().Msg("Runtime.Start() 被调用")

	// 启动插件管理器
	if err := r.PluginMgr.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("启动插件管理器失败")
	}
	// 启动注册的服务

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

	}
	r.started.Store(true)
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

	// 停止本地暂存的后台任务（汇总器）
	if r.Store != nil {
		r.Store.Close()
	}

	// 关闭 NATS 连接
	if r.Bus != nil {
		r.Bus.Close()
	}

	// 关闭嵌入式 NATS 服务器
	if r.NatsServer != nil {
		r.NatsServer.Shutdown()
	}
}

// updateRuleEngineMetrics 更新规则引擎指标到全局metrics系统
func (r *Runtime) updateRuleEngineMetrics() {
	if r.metrics == nil {
		return
	}

	// 找到规则引擎服务
	for _, s := range r.Svcs {
		if s.Name() == "rule-engine" {
			if ruleService, ok := s.(*rules.RuleEngineService); ok {
				// 获取规则引擎的监控指标
				engineMetrics := ruleService.GetMetrics()
				if engineMetrics != nil {
					// 详细调试日志：显示原始指标数据
					log.Debug().
						Int64("raw_rules_total", engineMetrics.RulesTotal).
						Int64("raw_rules_enabled", engineMetrics.RulesEnabled).
						Int64("raw_rules_matched", engineMetrics.RulesMatched).
						Int64("raw_actions_executed", engineMetrics.ActionsExecuted).
						Int64("raw_actions_succeeded", engineMetrics.ActionsSucceeded).
						Int64("raw_actions_failed", engineMetrics.ActionsFailed).
						Int64("raw_points_processed", engineMetrics.PointsProcessed).
						Time("raw_last_processed", engineMetrics.LastProcessedAt).
						Msg("🔍 规则引擎原始指标数据")

					// 同步统计数据到全局指标系统
					r.metrics.UpdateRuleMetrics(
						int(engineMetrics.RulesTotal),   // 总规则数
						int(engineMetrics.RulesEnabled), // 启用规则数
						engineMetrics.RulesMatched,      // 匹配次数
						engineMetrics.ActionsExecuted,   // 动作执行次数
						engineMetrics.ActionsSucceeded,  // 动作成功次数
						engineMetrics.ActionsFailed,     // 动作失败次数
					)

					log.Debug().
						Int("synced_total_rules", int(engineMetrics.RulesTotal)).
						Int("synced_enabled_rules", int(engineMetrics.RulesEnabled)).
						Int64("synced_rules_matched", engineMetrics.RulesMatched).
						Int64("synced_actions_executed", engineMetrics.ActionsExecuted).
						Msg("✅ 规则引擎指标已同步到全局metrics系统")
				} else {
					log.Warn().Msg("⚠️ 规则引擎GetMetrics返回nil")
				}
			}
			break
		}
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
			var totalAdapters, runningAdapters, totalSinks, runningSinks int
			for _, inst := range r.PluginMgr.InstanceStatuses() {
				if !inst.Counted() {
					continue
				}
				switch inst.Kind {
				case string(plugin.TypeAdapter):
					totalAdapters++
					if inst.Running {
						runningAdapters++
					}
				case string(plugin.TypeSink):
					totalSinks++
					if inst.Running {
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

// storageDir JetStream 存储目录（storage.store_dir，默认 ./data/jetstream）
func storageDir(v *viper.Viper) string {
	if d := v.GetString("storage.store_dir"); d != "" {
		return d
	}
	return "./data/jetstream"
}

// opsPort 返回单独的运维端口（为空表示不单独监听）：
// gateway.http_port 为 0 且 Web 端口已提供运维接口时不再单独监听；Web 未启用时仍使用 8080
func opsPort(configured string, webOps bool) string {
	switch configured {
	case "":
		return "8080"
	case "0":
		if webOps {
			log.Info().Msg("gateway.http_port 为 0：运维接口只由 Web 端口提供")
			return ""
		}
		log.Warn().Msg("gateway.http_port 为 0 但 Web 服务未启用，运维接口仍使用 8080 端口")
		return "8080"
	}
	return configured
}
