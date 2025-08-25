package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
)

// RuleEngineConfig 规则引擎配置
type RuleEngineConfig struct {
	Enabled   bool              `yaml:"enabled" json:"enabled"`
	RulesDir  string            `yaml:"rules_dir" json:"rules_dir"`
	Rules     []*Rule           `yaml:"rules" json:"rules"`
	Subject   string            `yaml:"subject" json:"subject"`
	HotReload *HotReloadConfig  `yaml:"hot_reload" json:"hot_reload"` // 热加载配置
}

// RuleEngineService 规则引擎服务
type RuleEngineService struct {
	config    *RuleEngineConfig
	manager   RuleManager
	evaluator *Evaluator
	bus       *nats.Conn
	js        nats.JetStreamContext
	sub       *nats.Subscription
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// 动作处理器
	actionHandlers map[string]ActionHandler

	// 聚合状态管理（保留旧版本兼容性）
	aggregateStates map[string]*AggregateState
	aggregateMutex  sync.RWMutex
	
	// 新的分片聚合状态管理器（高性能）
	shardedAggregates *ShardedAggregateStates
	useShardedAggregates bool

	// Runtime引用
	runtime interface{}
	
	// 并行处理优化
	workerPool    *WorkerPool
	ruleTaskQueue chan RuleTask
	maxWorkers    int
	queueSize     int
	
	// 新的优化工作池
	optimizedPool *OptimizedWorkerPool
	useOptimizedPool bool
	
	// 监控和错误处理
	monitor       *RuleMonitor
	enableMetrics bool
	
	// 高性能聚合处理器
	optimizedAggregateHandler OptimizedAggregateHandler
	
	// 规则索引系统
	ruleIndex *Index
	useRuleIndex bool
}

// GetRuleManager 获取规则管理器实例
func (s *RuleEngineService) GetRuleManager() RuleManager {
	return s.manager
}

// RuleTask 规则处理任务
type RuleTask struct {
	Rule  *Rule
	Point model.Point
}

// WorkerPool 工作池
type WorkerPool struct {
	workers    int
	taskQueue  chan RuleTask
	service    *RuleEngineService
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// AggregateState 聚合状态
type AggregateState struct {
	Buffer     []model.Point
	GroupKey   string
	Count      int
	WindowSize int
	LastUpdate time.Time
}

// NewRuleEngineService 创建规则引擎服务
func NewRuleEngineService() *RuleEngineService {
	// 使用更大的默认值以应对高负载场景
	service := &RuleEngineService{
		actionHandlers:  make(map[string]ActionHandler),
		aggregateStates: make(map[string]*AggregateState),
		aggregateMutex:  sync.RWMutex{},
		maxWorkers:      16,   // 增大默认worker数量
		queueSize:       5000, // 增大默认队列大小
		enableMetrics:   true, // 默认启用监控
		
		// 启用新的优化组件
		useShardedAggregates: true,
		useOptimizedPool:     true,
		useRuleIndex:         true, // 启用规则索引系统
	}
	
	// 初始化监控器
	service.monitor = NewRuleMonitor(1000) // 保留最近1000个错误
	
	// 初始化分片聚合状态管理器
	service.shardedAggregates = NewShardedAggregateStates(16) // 16个分片
	
	// 初始化规则索引
	service.ruleIndex = NewIndex()
	
	return service
}

// NewRuleEngineServiceWithConfig 使用配置创建规则引擎服务
func NewRuleEngineServiceWithConfig(config map[string]interface{}) *RuleEngineService {
	service := NewRuleEngineService()
	
	// 解析工作池配置
	if poolConfig, ok := config["worker_pool"].(map[string]interface{}); ok {
		if maxWorkers, ok := poolConfig["max_workers"].(int); ok && maxWorkers > 0 {
			service.maxWorkers = maxWorkers
		}
		
		if queueSize, ok := poolConfig["queue_size"].(int); ok && queueSize > 0 {
			service.queueSize = queueSize
		}
		
		if useOptimized, ok := poolConfig["use_optimized"].(bool); ok {
			service.useOptimizedPool = useOptimized
		}
	}
	
	// 解析热加载配置
	if hotReloadConfig, ok := config["hot_reload"].(map[string]interface{}); ok {
		if enabled, ok := hotReloadConfig["enabled"].(bool); ok {
			if service.config == nil {
				service.config = &RuleEngineConfig{}
			}
			if service.config.HotReload == nil {
				service.config.HotReload = &HotReloadConfig{}
			}
			service.config.HotReload.Enabled = enabled
		}
		
		if gracefulFallback, ok := hotReloadConfig["graceful_fallback"].(bool); ok {
			service.config.HotReload.GracefulFallback = gracefulFallback
		}
		
		if retryInterval, ok := hotReloadConfig["retry_interval"].(string); ok {
			service.config.HotReload.RetryInterval = retryInterval
		}
		
		if maxRetries, ok := hotReloadConfig["max_retries"].(int); ok {
			service.config.HotReload.MaxRetries = maxRetries
		}
		
		if debounceDelay, ok := hotReloadConfig["debounce_delay"].(string); ok {
			service.config.HotReload.DebounceDelay = debounceDelay
		}
	}
	
	return service
}

// NewWorkerPool 创建工作池
func NewWorkerPool(workers int, queueSize int, service *RuleEngineService) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		workers:   workers,
		taskQueue: make(chan RuleTask, queueSize),
		service:   service,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start 启动工作池
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
	log.Info().Int("workers", wp.workers).Msg("规则引擎工作池启动")
}

// Stop 停止工作池
func (wp *WorkerPool) Stop() {
	wp.cancel()
	close(wp.taskQueue)
	wp.wg.Wait()
}

// SubmitTask 提交任务到工作池
func (wp *WorkerPool) SubmitTask(task RuleTask) bool {
	select {
	case wp.taskQueue <- task:
		return true
	case <-wp.ctx.Done():
		return false
	default:
		// 队列满了，返回false让调用者决定处理方式
		return false
	}
}

// worker 工作协程
func (wp *WorkerPool) worker(workerID int) {
	defer wp.wg.Done()
	
	
	for {
		select {
		case <-wp.ctx.Done():
			return
		case task, ok := <-wp.taskQueue:
			if !ok {
				return
			}
			
			// 处理规则任务
			wp.service.processRuleTask(task)
		}
	}
}

// 动作处理器将在运行时注册

// RegisterActionHandler 注册动作处理器
func (s *RuleEngineService) RegisterActionHandler(actionType string, handler ActionHandler) {
	s.actionHandlers[actionType] = handler
	log.Info().Str("type", actionType).Str("name", handler.Name()).Msg("动作处理器已注册")
}

// handleAggregateResult 处理聚合结果并转发
func (s *RuleEngineService) handleAggregateResult(aggregateResult *AggregateResult, originalPoint model.Point, rule *Rule, action *Action) error {
	// 从聚合结果创建新的数据点
	config := action.Config
	outputKey := "aggregated_result"
	forward := false

	// 解析输出配置
	if output, ok := config["output"].(map[string]interface{}); ok {
		if keyTemplate, ok := output["key_template"].(string); ok {
			outputKey = s.formatOutputKey(keyTemplate, originalPoint)
		}
		if forwardFlag, ok := output["forward"].(bool); ok {
			forward = forwardFlag
		}
	}

	// 获取聚合函数的第一个结果作为值
	var aggregatedValue interface{} = 0.0
	if len(aggregateResult.Functions) > 0 {
		for _, value := range aggregateResult.Functions {
			aggregatedValue = value
			break
		}
	}

	// 创建聚合结果数据点，使用安全的Tags复制
	resultPoint := model.Point{
		DeviceID:  aggregateResult.DeviceID,
		Key:       outputKey,
		Value:     aggregatedValue,
		Type:      model.TypeFloat,
		Timestamp: aggregateResult.Timestamp,
	}
	// Go 1.24安全：复制原始数据点的安全标签
	originalTags := originalPoint.GetTagsCopy()
	for k, v := range originalTags {
		resultPoint.AddTag(k, v)
	}
	// 添加聚合标签（Tags字段已通过AddTag方法初始化）
	// Go 1.24安全：使用AddTag方法替代直接Tags[]访问
	resultPoint.AddTag("aggregated", "true")
	resultPoint.AddTag("source_rule", rule.ID)
	resultPoint.AddTag("window_count", fmt.Sprintf("%d", aggregateResult.Count))

	log.Info().
		Str("rule_id", rule.ID).
		Str("output_key", resultPoint.Key).
		Interface("result", aggregatedValue).
		Int64("window_count", aggregateResult.Count).
		Msg("聚合计算完成，准备转发")

	// 如果配置了转发，发送结果到数据总线
	if forward {
		if err := s.publishPoint(resultPoint); err != nil {
			return fmt.Errorf("发布聚合结果失败: %w", err)
		}
	}

	return nil
}

// SetRuntime 设置Runtime引用
func (s *RuleEngineService) SetRuntime(runtime interface{}) {
	s.runtime = runtime
}

// Name 返回服务名称
func (s *RuleEngineService) Name() string {
	return "rule-engine"
}

// Init 初始化服务
func (s *RuleEngineService) Init(cfg any) error {
	// 解析配置
	configData, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	var config RuleEngineConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return fmt.Errorf("解析规则引擎配置失败: %w", err)
	}

	s.config = &config

	// 设置默认值
	if s.config.RulesDir == "" {
		s.config.RulesDir = "./data/rules"
	}
	if s.config.Subject == "" {
		s.config.Subject = "iot.data.>"
	}

	// 创建规则管理器
	s.manager = NewManager(s.config.RulesDir)
	
	// 传递热加载配置给规则管理器
	if s.config.HotReload != nil {
		s.manager.SetHotReloadConfig(s.config.HotReload)
		log.Info().
			Bool("hot_reload_enabled", s.config.HotReload.Enabled).
			Bool("graceful_fallback", s.config.HotReload.GracefulFallback).
			Str("retry_interval", s.config.HotReload.RetryInterval).
			Int("max_retries", s.config.HotReload.MaxRetries).
			Msg("规则文件热加载配置已设置")
	} else {
		log.Info().Msg("使用默认热加载配置")
	}
	
	s.evaluator = NewEvaluator()
	
	// 如果启用了规则索引，重新构建索引
	if s.useRuleIndex && s.ruleIndex != nil {
		s.rebuildRuleIndex()
	}

	log.Info().
		Str("rules_dir", s.config.RulesDir).
		Str("subject", s.config.Subject).
		Bool("enabled", s.config.Enabled).
		Msg("规则引擎服务初始化完成")

	return nil
}

// Start 启动服务
func (s *RuleEngineService) Start(ctx context.Context) error {

	if !s.config.Enabled {
		log.Info().Msg("规则引擎服务已禁用")
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	// 加载规则
	if err := s.manager.LoadRules(); err != nil {
		log.Error().Err(err).Msg("加载规则文件失败")
		return fmt.Errorf("加载规则失败: %w", err)
	}

	// 加载配置中的内联规则
	if err := s.loadInlineRules(); err != nil {
		log.Error().Err(err).Msg("加载内联规则失败")
		return fmt.Errorf("加载内联规则失败: %w", err)
	}
	
	// 构建规则索引
	if s.useRuleIndex && s.ruleIndex != nil {
		s.rebuildRuleIndex()
		log.Info().Msg("🔍 规则索引系统已启用")
	}

	// 获取NATS连接
	log.Info().Msg("开始设置NATS连接...")
	if err := s.setupNATSConnection(ctx); err != nil {
		log.Error().Err(err).Msg("设置NATS连接失败")
		return fmt.Errorf("设置NATS连接失败: %w", err)
	}

	// 创建并启动工作池
	
	if s.useOptimizedPool {
		// 使用优化的工作池，支持动态配置
		config := WorkerPoolConfig{
			NumWorkers:   s.maxWorkers,
			QueueSize:    s.queueSize,
			BatchSize:    20,                    // 增大批处理大小
			BatchTimeout: 10 * time.Millisecond, // 增加批处理超时
		}
		s.optimizedPool = NewOptimizedWorkerPool(config, s)
		if err := s.optimizedPool.Start(); err != nil {
			log.Error().Err(err).Msg("启动优化工作池失败")
			return fmt.Errorf("启动优化工作池失败: %w", err)
		}
	} else {
		// 使用原始工作池
		s.workerPool = NewWorkerPool(s.maxWorkers, s.queueSize, s)
		s.workerPool.Start()
		log.Info().Int("workers", s.maxWorkers).Int("queue_size", s.queueSize).Msg("规则引擎工作池启动成功")
	}

	// 注册动作处理器
	// 注册动作处理器
	
	// 注册内建的动作处理器
	builtinAlertHandler := &BuiltinAlertHandler{
		natsConn:    s.bus,
		throttleMap: make(map[string]time.Time),
	}
	// 启动清理协程
	go builtinAlertHandler.startCleanupRoutine()
	s.RegisterActionHandler("alert", builtinAlertHandler)
	
	// Transform和Forward处理器需要在外部注册，以避免循环导入
	// 这些处理器应该在main函数或runtime中注册
	

	// 订阅数据主题
	if err := s.subscribeToDataStream(); err != nil {
		log.Error().Err(err).Msg("订阅数据流失败")
		return fmt.Errorf("订阅数据流失败: %w", err)
	}

	// 启动规则监控
	s.wg.Add(1)
	go s.watchRuleChanges()

	// 启动聚合状态清理器
	s.wg.Add(1)
	go s.aggregateStatesCleaner()

	log.Info().
		Int("rules_count", len(s.manager.GetEnabledRules())).
		Msg("规则引擎服务启动成功")

	return nil
}

// Stop 停止服务
func (s *RuleEngineService) Stop(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}

	// 停止工作池
	if s.useOptimizedPool && s.optimizedPool != nil {
		log.Info().Msg("停止优化规则引擎工作池...")
		if err := s.optimizedPool.Stop(); err != nil {
			log.Error().Err(err).Msg("停止优化工作池失败")
		} else {
			log.Info().Msg("优化规则引擎工作池已停止")
		}
	} else if s.workerPool != nil {
		log.Info().Msg("停止规则引擎工作池...")
		s.workerPool.Stop()
		log.Info().Msg("规则引擎工作池已停止")
	}

	// 取消订阅
	if s.sub != nil {
		s.sub.Unsubscribe()
	}

	// 等待所有goroutine完成
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("规则引擎服务停止成功")
	case <-time.After(5 * time.Second):
		log.Warn().Msg("规则引擎服务停止超时")
	}

	// 关闭监控器
	if s.monitor != nil {
		s.monitor.Close()
	}

	// 关闭规则管理器
	if s.manager != nil {
		s.manager.Close()
	}

	return nil
}

// setupNATSConnection 设置NATS连接
func (s *RuleEngineService) setupNATSConnection(ctx context.Context) error {
	// 从Runtime获取NATS连接
	if s.runtime != nil {
		// 使用类型断言获取Runtime的NATS连接
		if runtime, ok := s.runtime.(interface {
			GetBus() *nats.Conn
		}); ok {
			s.bus = runtime.GetBus()
			if s.bus != nil {
				// 创建JetStream上下文
				var err error
				s.js, err = s.bus.JetStream()
				if err != nil {
					return fmt.Errorf("创建JetStream上下文失败: %w", err)
				}
				log.Info().Str("server", s.bus.ConnectedUrl()).Msg("规则引擎使用Runtime的NATS连接")
				return nil
			}
		}
	}

	// 如果无法从Runtime获取连接，则创建新连接
	// 尝试连接到本地嵌入式服务器
	var err error
	s.bus, err = nats.Connect("nats://127.0.0.1:4222")
	if err != nil {
		// 尝试连接到默认NATS服务器
		s.bus, err = nats.Connect(nats.DefaultURL)
		if err != nil {
			return fmt.Errorf("无法连接到NATS服务器: %w", err)
		}
	}

	// 创建JetStream上下文
	s.js, err = s.bus.JetStream()
	if err != nil {
		return fmt.Errorf("创建JetStream上下文失败: %w", err)
	}

	log.Info().Str("server", s.bus.ConnectedUrl()).Msg("规则引擎已连接到NATS服务器")
	return nil
}

// subscribeToDataStream 订阅数据流
func (s *RuleEngineService) subscribeToDataStream() error {
	var err error
	s.sub, err = s.bus.Subscribe(s.config.Subject, s.handleDataPoint)
	if err != nil {
		return fmt.Errorf("订阅数据主题失败: %w", err)
	}

	log.Info().Str("subject", s.config.Subject).Msg("已订阅数据流")
	return nil
}

// handleDataPoint 处理数据点
func (s *RuleEngineService) handleDataPoint(msg *nats.Msg) {
	log.Info().
		Str("subject", msg.Subject).
		Int("data_size", len(msg.Data)).
		Msg("🎯 规则引擎收到数据点消息")

	// 解析数据点
	var point model.Point
	if err := json.Unmarshal(msg.Data, &point); err != nil {
		if s.enableMetrics {
			s.monitor.RecordError(ErrorTypeValidation, ErrorLevelError, 
				"解析数据点失败", err.Error(), 
				map[string]string{"subject": msg.Subject})
		}
		log.Error().Err(err).Str("subject", msg.Subject).Msg("解析数据点失败")
		return
	}

	log.Debug().
		Str("key", point.Key).
		Str("device_id", point.DeviceID).
		Interface("value", point.Value).
		Msg("开始处理数据点")

	// 获取候选规则（使用索引优化）
	var rules []*Rule
	if s.useRuleIndex && s.ruleIndex != nil {
		// 使用规则索引获取候选规则
		rules = s.ruleIndex.Match(point)
		log.Debug().Int("indexed_rules", len(rules)).Msg("🔍 使用规则索引获取候选规则")
	} else {
		// 回退到获取所有启用的规则
		rules = s.manager.GetEnabledRules()
		log.Debug().Int("all_rules", len(rules)).Msg("📝 使用所有启用规则")
	}
	
	if len(rules) == 0 {
		log.Warn().Msg("⚠️ 没有匹配的规则")
		return
	}

	log.Info().
		Int("rules_count", len(rules)).
		Bool("use_index", s.useRuleIndex && s.ruleIndex != nil).
		Msg("🔢 开始评估规则")

	// 并行评估规则
	successCount := 0
	failCount := 0
	
	for _, rule := range rules {
		task := RuleTask{Rule: rule, Point: point}
		
		var submitted bool
		if s.useOptimizedPool && s.optimizedPool != nil {
			// 使用优化工作池
			submitted = s.optimizedPool.SubmitTask(task)
		} else if s.workerPool != nil {
			// 使用原始工作池
			submitted = s.workerPool.SubmitTask(task)
		}
		
		if submitted {
			successCount++
		} else {
			// 工作池满或不可用，回退到同步处理
			s.processRule(rule, point)
			failCount++
		}
	}
	
	log.Info().
		Int("parallel_tasks", successCount).
		Int("sync_tasks", failCount).
		Bool("useOptimizedPool", s.useOptimizedPool).
		Bool("optimizedPool_nil", s.optimizedPool == nil).
		Bool("workerPool_nil", s.workerPool == nil).
		Msg("📋 规则任务分发完成")
}

// processRuleTask 处理规则任务（由工作池调用）
func (s *RuleEngineService) processRuleTask(task RuleTask) {
	s.processRule(task.Rule, task.Point)
}

// processRule 处理单个规则
func (s *RuleEngineService) processRule(rule *Rule, point model.Point) {
	start := time.Now()
	
	// 评估条件
	matched, err := s.evaluator.Evaluate(rule.Conditions, point)
	duration := time.Since(start)
	
	// 临时调试：记录规则评估详情
	log.Info().
		Str("rule_id", rule.ID).
		Str("rule_name", rule.Name).
		Str("device_id", point.DeviceID).
		Str("key", point.Key).
		Interface("value", point.Value).
		Bool("matched", matched).
		Err(err).
		Msg("规则评估结果")
	
	// 记录规则执行统计
	if s.enableMetrics {
		if s.monitor == nil {
			log.Error().Msg("❌ s.monitor是nil但enableMetrics是true")
		} else {
			log.Debug().
				Bool("enableMetrics", s.enableMetrics).
				Msg("📈 准备调用RecordRuleExecution")
			s.monitor.RecordRuleExecution(rule.ID, duration, matched, err)
		}
	} else {
		log.Warn().
			Bool("enableMetrics", s.enableMetrics).
			Msg("⚠️ enableMetrics是false，跳过统计记录")
	}
	
	if err != nil {
		if s.enableMetrics {
			s.monitor.RecordError(ErrorTypeCondition, ErrorLevelError,
				"规则条件评估失败", err.Error(),
				map[string]string{
					"rule_id": rule.ID,
					"rule_name": rule.Name,
					"device_id": point.DeviceID,
					"key": point.Key,
				})
		}
		log.Error().
			Err(err).
			Str("rule_id", rule.ID).
			Str("rule_name", rule.Name).
			Msg("规则条件评估失败")
		return
	}

	// 发布规则评估事件
	var errorStr *string
	if err != nil {
		s := err.Error()
		errorStr = &s
	}
	s.publishRuleEvent("evaluated", rule, point, map[string]interface{}{
		"matched": matched,
		"duration_ns": duration.Nanoseconds(),
		"error": errorStr,
	})

	if !matched {
		log.Debug().
			Str("rule_id", rule.ID).
			Str("point_key", point.Key).
			Msg("规则条件不匹配")
		return
	}

	log.Debug().
		Str("rule_id", rule.ID).
		Str("rule_name", rule.Name).
		Str("point_key", point.Key).
		Msg("规则条件匹配，开始执行动作")

	// 发布规则匹配事件
	s.publishRuleEvent("matched", rule, point, map[string]interface{}{
		"matched": true,
		"duration_ns": duration.Nanoseconds(),
		"actions_count": len(rule.Actions),
	})

	// 执行动作
	executedActions := make([]map[string]interface{}, 0, len(rule.Actions))
	totalDuration := time.Duration(0)
	successCount := 0
	errorCount := 0

	for i, action := range rule.Actions {
		actionStart := time.Now()
		err := s.executeAction(&action, point, rule)
		actionDuration := time.Since(actionStart)
		totalDuration += actionDuration
		
		// 记录动作执行统计
		if s.enableMetrics {
			if s.monitor == nil {
				log.Error().Msg("❌ s.monitor是nil但enableMetrics是true (动作)")
			} else {
				log.Debug().
					Str("action_type", action.Type).
					Bool("success", err == nil).
					Msg("🎯 准备调用RecordActionExecution")
				s.monitor.RecordActionExecution(action.Type, actionDuration, err == nil, err)
			}
		} else {
			log.Warn().Msg("⚠️ enableMetrics是false，跳过动作统计记录")
		}
		
		actionResult := map[string]interface{}{
			"type": action.Type,
			"index": i,
			"duration_ns": actionDuration.Nanoseconds(),
			"success": err == nil,
		}
		
		if err != nil {
			actionResult["error"] = err.Error()
			errorCount++
			
			if s.enableMetrics {
				s.monitor.RecordError(ErrorTypeAction, ErrorLevelError,
					fmt.Sprintf("动作执行失败: %s", action.Type), err.Error(),
					map[string]string{
						"rule_id": rule.ID,
						"rule_name": rule.Name,
						"action_type": action.Type,
						"action_index": fmt.Sprintf("%d", i),
						"device_id": point.DeviceID,
						"key": point.Key,
					})
			}
			log.Error().
				Err(err).
				Str("rule_id", rule.ID).
				Str("action_type", action.Type).
				Msg("执行规则动作失败")
		} else {
			successCount++
		}

		executedActions = append(executedActions, actionResult)
		
		// 发布单个动作执行事件
		s.publishRuleEvent("action_executed", rule, point, map[string]interface{}{
			"action": actionResult,
		})
	}
	
	// 发布规则执行完成事件
	s.publishRuleEvent("executed", rule, point, map[string]interface{}{
		"matched": true,
		"total_duration_ns": (duration + totalDuration).Nanoseconds(),
		"evaluation_duration_ns": duration.Nanoseconds(),
		"actions_duration_ns": totalDuration.Nanoseconds(),
		"actions": executedActions,
		"actions_total": len(rule.Actions),
		"actions_success": successCount,
		"actions_error": errorCount,
	})
}

// processRuleTaskInternal 内部规则处理方法（供优化工作池调用）
func (s *RuleEngineService) processRuleTaskInternal(rule *Rule, point model.Point) error {
	if !rule.Enabled {
		return nil
	}
	
	startTime := time.Now()
	
	// 评估条件
	matched, err := s.evaluator.Evaluate(rule.Conditions, point)
	if err != nil {
		if s.enableMetrics {
			s.monitor.RecordError(ErrorTypeCondition, ErrorLevelError,
				"条件评估失败", err.Error(),
				map[string]string{
					"rule_id": rule.ID,
					"rule_name": rule.Name,
					"device_id": point.DeviceID,
					"key": point.Key,
				})
		}
		return fmt.Errorf("规则条件评估失败: %w", err)
	}
	
	duration := time.Since(startTime)
	
	// *** 修复：添加规则执行统计记录 ***
	if s.enableMetrics {
		if s.monitor == nil {
			log.Error().Msg("❌ s.monitor是nil但enableMetrics是true")
		} else {
			log.Debug().
				Bool("enableMetrics", s.enableMetrics).
				Str("rule_id", rule.ID).
				Bool("matched", matched).
				Msg("📈 准备调用RecordRuleExecution（优化工作池）")
			s.monitor.RecordRuleExecution(rule.ID, duration, matched, err)
		}
	} else {
		log.Warn().
			Bool("enableMetrics", s.enableMetrics).
			Msg("⚠️ enableMetrics是false，跳过统计记录（优化工作池）")
	}
	
	// 发布条件评估事件
	s.publishRuleEvent("evaluated", rule, point, map[string]interface{}{
		"matched": matched,
		"duration_ns": duration.Nanoseconds(),
	})
	
	// 如果条件匹配，执行动作
	if matched {
		// 简化的动作执行，避免循环依赖
		for _, action := range rule.Actions {
			if err := s.executeAction(&action, point, rule); err != nil {
				log.Error().
					Err(err).
					Str("rule_id", rule.ID).
					Str("action_type", action.Type).
					Msg("执行规则动作失败")
			}
		}
	}
	
	return nil
}

// executeAction 执行动作
func (s *RuleEngineService) executeAction(action *Action, point model.Point, rule *Rule) error {
	actionStart := time.Now()
	
	handler, exists := s.actionHandlers[action.Type]
	if exists {
		// 使用新的动作处理器
		result, err := handler.Execute(context.Background(), point, rule, action.Config)
		actionDuration := time.Since(actionStart)
		
		// *** 修复：记录动作执行统计 ***
		if s.enableMetrics && s.monitor != nil {
			s.monitor.RecordActionExecution(action.Type, actionDuration, err == nil, err)
		}
		
		if err != nil {
			return err
		}

		// 处理聚合结果，如果需要转发
		if action.Type == "aggregate" && result.Success {
			if output, ok := result.Output.(map[string]interface{}); ok {
				if aggregated, ok := output["aggregated"].(bool); ok && aggregated {
					if aggregateResult, ok := output["aggregate_result"].(*AggregateResult); ok {
						// 创建聚合结果数据点并转发
						if err := s.handleAggregateResult(aggregateResult, point, rule, action); err != nil {
							log.Error().Err(err).Msg("处理聚合结果失败")
						}
					}
				}
			}
		}

		return nil
	}

	// 回退到旧的内置实现
	var err error
	switch action.Type {
	case "aggregate":
		err = s.executeAggregateAction(action, point, rule)
	case "transform":
		err = s.executeTransformAction(action, point, rule)
	case "filter":
		err = s.executeFilterAction(action, point, rule)
	case "forward":
		err = s.executeForwardAction(action, point, rule)
	case "alert":
		err = s.executeAlertAction(action, point, rule)
	default:
		err = fmt.Errorf("不支持的动作类型: %s", action.Type)
	}
	
	// *** 修复：记录旧实现的动作执行统计 ***
	actionDuration := time.Since(actionStart)
	if s.enableMetrics && s.monitor != nil {
		s.monitor.RecordActionExecution(action.Type, actionDuration, err == nil, err)
	}
	
	return err
}

// executeAggregateAction 执行聚合动作 - 高性能优化版本
func (s *RuleEngineService) executeAggregateAction(action *Action, point model.Point, rule *Rule) error {
	// 检查是否启用高性能聚合引擎
	useOptimized := os.Getenv("IOT_GATEWAY_ENABLE_OPTIMIZED_AGGREGATE") == "true"
	
	if useOptimized {
		return s.executeOptimizedAggregateAction(action, point, rule)
	}
	
	// 回退到原始实现
	return s.executeLegacyAggregateAction(action, point, rule)
}

// executeOptimizedAggregateAction 执行优化版聚合动作
func (s *RuleEngineService) executeOptimizedAggregateAction(action *Action, point model.Point, rule *Rule) error {
	// 懒加载优化聚合处理器
	if s.optimizedAggregateHandler == nil {
		if OptimizedAggregateHandlerFactory == nil {
			log.Error().Msg("优化聚合处理器工厂未注册，回退到传统实现")
			return s.executeLegacyAggregateAction(action, point, rule)
		}
		s.optimizedAggregateHandler = OptimizedAggregateHandlerFactory()
		log.Info().Msg("高性能聚合引擎已启动")
	}
	
	// 使用优化处理器处理
	result, err := s.optimizedAggregateHandler.Execute(context.Background(), point, rule, action.Config)
	if err != nil {
		log.Error().Err(err).Msg("优化聚合处理失败，回退到传统实现")
		return s.executeLegacyAggregateAction(action, point, rule)
	}
	
	// 处理聚合结果转发
	if result.Success && result.Output != nil {
		if outputMap, ok := result.Output.(map[string]interface{}); ok {
			if aggregated, ok := outputMap["aggregated"].(bool); ok && aggregated {
				if aggregateResultData, ok := outputMap["aggregate_result"]; ok {
					if aggResult, ok := aggregateResultData.(map[string]interface{}); ok {
						if err := s.handleOptimizedAggregateResult(aggResult, point, rule, action); err != nil {
							log.Error().Err(err).Msg("处理优化聚合结果失败")
						}
					}
				}
			}
		}
	}
	
	return nil
}

// executeLegacyAggregateAction 执行传统聚合动作（保持向后兼容）
func (s *RuleEngineService) executeLegacyAggregateAction(action *Action, point model.Point, rule *Rule) error {
	config := action.Config

	// 获取窗口配置
	windowSize, _ := config["count"].(int) // 新配置使用count而不是size
	if windowSize <= 0 {
		if size, ok := config["size"].(int); ok {
			windowSize = size // 兼容旧版本
		}
	}
	functions, _ := config["functions"].([]interface{})
	groupBy, _ := config["group_by"].([]interface{})

	// 处理新的嵌套output配置
	var outputKey string
	var forward bool
	if output, ok := config["output"].(map[string]interface{}); ok {
		if keyTemplate, ok := output["key_template"].(string); ok {
			outputKey = keyTemplate
		}
		if forwardFlag, ok := output["forward"].(bool); ok {
			forward = forwardFlag
		}
	} else {
		// 兼容旧版本配置
		outputKey, _ = config["output_key"].(string)
		forward, _ = config["forward"].(bool)
	}

	if windowSize <= 0 {
		windowSize = 10 // 默认窗口大小
	}

	// 生成分组键
	groupKey := s.generateGroupKey(point, groupBy)
	stateKey := fmt.Sprintf("%s:%s", rule.ID, groupKey)

	var state *AggregateState
	var windowReady bool
	
	if s.useShardedAggregates {
		// 使用分片聚合状态管理器（高性能）
		state, windowReady = s.shardedAggregates.UpdateState(stateKey, point, windowSize)
	} else {
		// 使用原始聚合状态管理器（向后兼容）
		s.aggregateMutex.Lock()
		var exists bool
		state, exists = s.aggregateStates[stateKey]
		if !exists {
			state = &AggregateState{
				Buffer:     make([]model.Point, 0, windowSize),
				GroupKey:   groupKey,
				WindowSize: windowSize,
			}
			s.aggregateStates[stateKey] = state
		}
		
		// 添加数据点到缓冲区
		state.Buffer = append(state.Buffer, point)
		state.LastUpdate = time.Now()
		windowReady = len(state.Buffer) >= windowSize
		s.aggregateMutex.Unlock()
	}

	// 检查是否达到窗口大小
	if windowReady {
		// 计算聚合结果
		result, err := s.calculateAggregateResult(state.Buffer, functions)
		if err != nil {
			return fmt.Errorf("计算聚合结果失败: %w", err)
		}

		// 创建结果数据点
		// 创建安全的Tags副本 - 使用GetTagsCopy()获取SafeTags
		safeTags := point.GetTagsCopy()
		
		resultPoint := model.Point{
			DeviceID:  point.DeviceID,
			Key:       s.formatOutputKey(outputKey, point),
			Value:     result,
			Type:      model.TypeFloat,
			Timestamp: time.Now(),
		}
		// Go 1.24安全：复制安全标签到结果数据点
		for k, v := range safeTags {
			resultPoint.AddTag(k, v)
		}
		// 添加聚合标签
		// Go 1.24安全：使用AddTag方法替代直接Tags[]访问
		resultPoint.AddTag("aggregated", "true")
		resultPoint.AddTag("window_size", fmt.Sprintf("%d", windowSize))
		resultPoint.AddTag("source_rule", rule.ID)

		log.Info().
			Str("rule_id", rule.ID).
			Str("output_key", resultPoint.Key).
			Interface("result", result).
			Int("window_size", windowSize).
			Msg("聚合计算完成")

		// 如果配置了转发，发送结果到数据总线
		if forward {
			if err := s.publishPoint(resultPoint); err != nil {
				log.Error().Err(err).Msg("发布聚合结果失败")
			}
		}

		// 清空缓冲区（滑动窗口）
		if s.useShardedAggregates {
			s.shardedAggregates.ClearStateBuffer(stateKey)
		} else {
			state.Buffer = state.Buffer[:0]
		}
	}

	return nil
}

// handleOptimizedAggregateResult 处理优化聚合结果
func (s *RuleEngineService) handleOptimizedAggregateResult(aggregateResult map[string]interface{}, originalPoint model.Point, rule *Rule, action *Action) error {
	// 提取聚合结果信息
	deviceID, _ := aggregateResult["device_id"].(string)
	functions, _ := aggregateResult["functions"].(map[string]interface{})
	timestamp, _ := aggregateResult["timestamp"].(time.Time)
	
	// 处理输出配置
	config := action.Config
	var outputKey string
	var forward bool
	
	if output, ok := config["output"].(map[string]interface{}); ok {
		if keyTemplate, ok := output["key_template"].(string); ok {
			outputKey = s.formatOutputKey(keyTemplate, originalPoint)
		}
		if forwardFlag, ok := output["forward"].(bool); ok {
			forward = forwardFlag
		}
	} else {
		outputKey, _ = config["output_key"].(string)
		forward, _ = config["forward"].(bool)
	}
	
	if outputKey == "" {
		outputKey = "aggregated_result"
	}
	
	// 获取聚合函数的第一个结果作为值
	var aggregatedValue interface{} = 0.0
	if len(functions) > 0 {
		for _, value := range functions {
			aggregatedValue = value
			break
		}
	}
	
	// 创建结果数据点，使用安全的Tags复制
	resultPoint := model.Point{
		DeviceID:  deviceID,
		Key:       outputKey,
		Value:     aggregatedValue,
		Type:      model.TypeFloat,
		Timestamp: timestamp,
	}
	// Go 1.24安全：复制原始数据点的安全标签
	originalTags := originalPoint.GetTagsCopy()
	for k, v := range originalTags {
		resultPoint.AddTag(k, v)
	}
	// 添加聚合标签（Tags字段已通过AddTag方法初始化）
	// Go 1.24安全：使用AddTag方法替代直接Tags[]访问
	resultPoint.AddTag("aggregated", "true")
	resultPoint.AddTag("source_rule", rule.ID)
	resultPoint.AddTag("optimized", "true")
	
	log.Info().
		Str("rule_id", rule.ID).
		Str("output_key", resultPoint.Key).
		Interface("result", aggregatedValue).
		Str("engine", "optimized").
		Msg("优化聚合计算完成")
	
	// 如果配置了转发，发送结果到数据总线
	if forward {
		if err := s.publishPoint(resultPoint); err != nil {
			log.Error().Err(err).Msg("发布优化聚合结果失败")
			return err
		}
	}
	
	return nil
}

// OptimizedAggregateHandler 优化聚合处理器接口声明
type OptimizedAggregateHandler interface {
	Execute(ctx context.Context, point model.Point, rule *Rule, config map[string]interface{}) (*ActionResult, error)
	Close()
	GetMetrics() map[string]interface{}
}

// OptimizedAggregateHandlerFactory 优化聚合处理器工厂函数
var OptimizedAggregateHandlerFactory func() OptimizedAggregateHandler

// SetOptimizedAggregateHandlerFactory 设置优化聚合处理器工厂
func SetOptimizedAggregateHandlerFactory(factory func() OptimizedAggregateHandler) {
	OptimizedAggregateHandlerFactory = factory
}

// generateGroupKey 生成分组键
func (s *RuleEngineService) generateGroupKey(point model.Point, groupBy []interface{}) string {
	if len(groupBy) == 0 {
		return "default"
	}

	var keyParts []string
	for _, field := range groupBy {
		fieldStr := fmt.Sprintf("%v", field)
		switch fieldStr {
		case "key":
			keyParts = append(keyParts, point.Key)
		case "device_id":
			keyParts = append(keyParts, fmt.Sprintf("%d", point.DeviceID))
		case "type":
			keyParts = append(keyParts, string(point.Type))
		default:
			// Go 1.24安全：使用GetTag方法替代直接Tags[]访问
			if tagValue, exists := point.GetTag(fieldStr); exists {
				keyParts = append(keyParts, tagValue)
			}
		}
	}

	return fmt.Sprintf("%v", keyParts)
}

// formatOutputKey 格式化输出键
func (s *RuleEngineService) formatOutputKey(template string, point model.Point) string {
	if template == "" {
		return point.Key + "_processed"
	}

	// 支持{{.Key}}和{{.key}}格式的模板替换
	result := template
	if strings.Contains(result, "{{.Key}}") {
		result = strings.ReplaceAll(result, "{{.Key}}", point.Key)
	} else if strings.Contains(result, "{{.key}}") {
		result = strings.ReplaceAll(result, "{{.key}}", point.Key)
	} else if strings.Contains(result, "{{key}}") {
		result = strings.ReplaceAll(result, "{{key}}", point.Key)
	} else {
		// 如果没有模板标记，直接使用sprintf格式
		result = fmt.Sprintf(template, point.Key)
	}

	return result
}

// calculateAggregateResult 计算聚合结果
func (s *RuleEngineService) calculateAggregateResult(buffer []model.Point, functions []interface{}) (interface{}, error) {
	if len(buffer) == 0 {
		return nil, fmt.Errorf("缓冲区为空")
	}

	// 提取数值
	var values []float64
	for _, point := range buffer {
		if val, ok := s.convertToFloat64(point.Value); ok {
			values = append(values, val)
		}
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("没有有效的数值")
	}

	// 默认计算平均值
	if len(functions) == 0 {
		functions = []interface{}{"avg"}
	}

	// 计算第一个函数的结果（简化版本）
	function := fmt.Sprintf("%v", functions[0])
	switch function {
	case "avg", "average":
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return sum / float64(len(values)), nil
	case "sum":
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return sum, nil
	case "max":
		max := values[0]
		for _, v := range values {
			if v > max {
				max = v
			}
		}
		return max, nil
	case "min":
		min := values[0]
		for _, v := range values {
			if v < min {
				min = v
			}
		}
		return min, nil
	case "count":
		return float64(len(values)), nil
	default:
		return nil, fmt.Errorf("不支持的聚合函数: %s", function)
	}
}

// convertToFloat64 将值转换为float64，支持复杂数据类型
func (s *RuleEngineService) convertToFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		// 尝试解析字符串
		if f, err := fmt.Sscanf(v, "%f", new(float64)); err == nil && f == 1 {
			var result float64
			fmt.Sscanf(v, "%f", &result)
			return result, true
		}
	case map[string]interface{}:
		// 处理复杂数据类型
		return s.extractNumericFromComplexType(v)
	}
	return 0, false
}

// extractNumericFromComplexType 从复杂数据类型中提取数值用于聚合
func (s *RuleEngineService) extractNumericFromComplexType(data map[string]interface{}) (float64, bool) {
	// 1. 数组数据类型 - 取第一个数值元素
	if elements, ok := data["elements"]; ok {
		if elemArray, ok := elements.([]interface{}); ok && len(elemArray) > 0 {
			if val, ok := s.convertToFloat64(elemArray[0]); ok {
				return val, true
			}
		}
	}
	
	// 2. 向量数据类型 - 取第一个数值或计算向量模长
	if values, ok := data["values"]; ok {
		if valArray, ok := values.([]interface{}); ok && len(valArray) > 0 {
			if val, ok := s.convertToFloat64(valArray[0]); ok {
				return val, true
			}
		}
		if valArray, ok := values.([]float64); ok && len(valArray) > 0 {
			return valArray[0], true
		}
	}
	
	// 3. 3D向量 - 计算向量模长
	if x, okX := data["x"]; okX {
		if y, okY := data["y"]; okY {
			if z, okZ := data["z"]; okZ {
				if xVal, ok := s.convertToFloat64(x); ok {
					if yVal, ok := s.convertToFloat64(y); ok {
						if zVal, ok := s.convertToFloat64(z); ok {
							// 计算3D向量模长 sqrt(x² + y² + z²)
							magnitude := math.Sqrt(xVal*xVal + yVal*yVal + zVal*zVal)
							return magnitude, true
						}
					}
				}
			}
		}
	}
	
	// 4. GPS位置数据 - 优先使用速度或海拔
	if speed, ok := data["speed"]; ok {
		if val, ok := s.convertToFloat64(speed); ok {
			return val, true
		}
	}
	if altitude, ok := data["altitude"]; ok {
		if val, ok := s.convertToFloat64(altitude); ok {
			return val, true
		}
	}
	
	// 5. 颜色数据 - 使用亮度值
	if lightness, ok := data["lightness"]; ok {
		if val, ok := s.convertToFloat64(lightness); ok {
			return val, true
		}
	}
	if brightness, ok := data["brightness"]; ok {
		if val, ok := s.convertToFloat64(brightness); ok {
			return val, true
		}
	}
	
	// 6. 通用数值字段检查
	numericFields := []string{"value", "magnitude", "norm", "length", "size", "count", "temperature", "humidity", "pressure"}
	for _, field := range numericFields {
		if fieldValue, exists := data[field]; exists {
			if val, ok := s.convertToFloat64(fieldValue); ok {
				return val, true
			}
		}
	}
	
	return 0, false
}

// executeTransformAction 执行转换动作
func (s *RuleEngineService) executeTransformAction(action *Action, point model.Point, rule *Rule) error {
	config := action.Config
	
	// 简单的转换实现
	transformedPoint := point
	transformedPoint.Timestamp = time.Now()
	
	// 应用简单的转换
	if scale, ok := config["scale"].(float64); ok {
		if val, ok := point.Value.(float64); ok {
			transformedPoint.Value = val * scale
		}
	}
	
	if offset, ok := config["offset"].(float64); ok {
		if val, ok := transformedPoint.Value.(float64); ok {
			transformedPoint.Value = val + offset
		}
	}
	
	// 发布转换后的数据到NATS
	if s.bus != nil {
		subject := fmt.Sprintf("transformed.%s.%s", transformedPoint.DeviceID, transformedPoint.Key)
		if subjectCfg, ok := config["publish_subject"].(string); ok && subjectCfg != "" {
			subject = subjectCfg
		}
		
		publishData := map[string]interface{}{
			"device_id": transformedPoint.DeviceID,
			"key":       transformedPoint.Key,
			"value":     SafeValueForJSON(transformedPoint.Value),
			"type":      string(transformedPoint.Type),
			"timestamp": transformedPoint.Timestamp,
			"tags":      SafeValueForJSON(transformedPoint.GetTagsCopy()),
			"transform_info": map[string]interface{}{
				"rule_id":        rule.ID,
				"rule_name":      rule.Name,
				"action":         "transform",
				"original_value": SafeValueForJSON(point.Value),
			},
			"processed_at": time.Now(),
		}
		
		if jsonData, err := json.Marshal(publishData); err == nil {
			if err := s.bus.Publish(subject, jsonData); err != nil {
				log.Error().Err(err).Str("subject", subject).Msg("发布转换数据失败")
			} else {
				log.Debug().
					Str("rule_id", rule.ID).
					Str("subject", subject).
					Msg("转换数据已发布到NATS")
			}
		}
	}
	
	log.Info().
		Str("rule_id", rule.ID).
		Str("point_key", point.Key).
		Interface("original_value", point.Value).
		Interface("transformed_value", transformedPoint.Value).
		Msg("执行转换动作")
	
	return nil
}

// executeFilterAction 执行过滤动作
func (s *RuleEngineService) executeFilterAction(action *Action, point model.Point, rule *Rule) error {
	// 简化的过滤实现
	log.Info().
		Str("rule_id", rule.ID).
		Str("point_key", point.Key).
		Msg("执行过滤动作")
	return nil
}

// executeForwardAction 执行转发动作
func (s *RuleEngineService) executeForwardAction(action *Action, point model.Point, rule *Rule) error {
	config := action.Config
	
	if s.bus == nil {
		return fmt.Errorf("NATS连接未初始化")
	}
	
	// 获取目标主题
	subject, ok := config["subject"].(string)
	if !ok || subject == "" {
		// 使用默认主题格式
		subject = fmt.Sprintf("iot.data.%s.%s", point.DeviceID, point.Key)
	}
	
	// 准备转发数据（增加规则信息）
	forwardData := map[string]interface{}{
		"device_id": point.DeviceID,
		"key":       point.Key,
		"value":     point.Value,
		"type":      string(point.Type),
		"timestamp": point.Timestamp,
		"tags":      SafeValueForJSON(point.GetTagsCopy()), // 使用安全的JSON转换
		"rule_info": map[string]interface{}{
			"rule_id":   rule.ID,
			"rule_name": rule.Name,
			"action":    "forward",
		},
		"processed_at": time.Now(),
	}
	
	// 序列化并发送
	jsonData, err := json.Marshal(forwardData)
	if err != nil {
		return fmt.Errorf("序列化转发数据失败: %w", err)
	}
	
	if err := s.bus.Publish(subject, jsonData); err != nil {
		return fmt.Errorf("发送NATS消息失败: %w", err)
	}
	
	log.Info().
		Str("rule_id", rule.ID).
		Str("point_key", point.Key).
		Str("subject", subject).
		Int("bytes", len(jsonData)).
		Msg("执行转发动作")
		
	return nil
}

// executeAlertAction 执行告警动作
func (s *RuleEngineService) executeAlertAction(action *Action, point model.Point, rule *Rule) error {
	log.Warn().
		Str("rule_id", rule.ID).
		Str("point_key", point.Key).
		Interface("point_value", point.Value).
		Msg("规则告警触发")
	return nil
}

// publishPoint 发布数据点到总线
func (s *RuleEngineService) publishPoint(point model.Point) error {
	data, err := json.Marshal(point)
	if err != nil {
		return fmt.Errorf("序列化数据点失败: %w", err)
	}

	subject := fmt.Sprintf("iot.data.%s", point.Key)
	if err := s.bus.Publish(subject, data); err != nil {
		return fmt.Errorf("发布数据点失败: %w", err)
	}

	log.Debug().
		Str("subject", subject).
		Str("key", point.Key).
		Msg("数据点已发布到总线")

	return nil
}

// loadInlineRules 加载配置中的内联规则
func (s *RuleEngineService) loadInlineRules() error {
	if len(s.config.Rules) == 0 {
		return nil
	}

	for _, rule := range s.config.Rules {
		if err := s.manager.SaveRule(rule); err != nil {
			log.Error().
				Err(err).
				Str("rule_id", rule.ID).
				Msg("保存内联规则失败")
		} else {
			log.Info().
				Str("rule_id", rule.ID).
				Msg("内联规则加载成功")
			
			// 更新规则索引
			if s.useRuleIndex && s.ruleIndex != nil {
				s.updateRuleIndex(rule, "add")
			}
		}
	}

	return nil
}

// watchRuleChanges 监控规则变化
func (s *RuleEngineService) watchRuleChanges() {
	defer s.wg.Done()

	// 检查热加载是否允许
	if s.config != nil && s.config.HotReload != nil && !s.config.HotReload.Enabled {
		log.Info().Msg("规则文件热加载已禁用，跳过文件监控")
		return
	}

	changesChan, err := s.manager.WatchChanges()
	if err != nil {
		log.Error().Err(err).Msg("监控规则变化失败")
		
		// 检查是否优雅降级
		if s.config != nil && s.config.HotReload != nil && s.config.HotReload.GracefulFallback {
			log.Warn().Msg("启用优雅降级，继续运行但不监控规则文件变更")
			return
		}
		
		// 非优雅模式下，记录错误后继续
		return
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case event := <-changesChan:
			ruleID := ""
			if event.Rule != nil {
				ruleID = event.Rule.ID
			}
			log.Info().
				Str("event_type", event.Type).
				Str("rule_id", ruleID).
				Msg("规则变化事件")
			
			// 更新规则索引
			if event.Rule != nil {
				switch event.Type {
				case "created", "updated":
					s.updateRuleIndex(event.Rule, "update")
				case "deleted":
					s.updateRuleIndex(event.Rule, "remove")
				}
			}
		}
	}
}

// aggregateStatesCleaner 清理过期的聚合状态
func (s *RuleEngineService) aggregateStatesCleaner() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanExpiredAggregateStates()
		}
	}
}

// cleanExpiredAggregateStates 清理过期的聚合状态
func (s *RuleEngineService) cleanExpiredAggregateStates() {
	expireTime := time.Now().Add(-10 * time.Minute)
	var cleanedCount int
	
	if s.useShardedAggregates {
		// 使用分片清理，支持并行
		cleanedCount = s.shardedAggregates.CleanExpiredStates(10 * time.Minute)
	} else {
		// 使用原始清理方式
		s.aggregateMutex.Lock()
		defer s.aggregateMutex.Unlock()
		
		cleanedCount = 0
		for key, state := range s.aggregateStates {
			if state.LastUpdate.Before(expireTime) {
				delete(s.aggregateStates, key)
				cleanedCount++
			}
		}
	}
	
	if cleanedCount > 0 {
		log.Debug().
			Int("cleaned_count", cleanedCount).
			Msg("清理过期聚合状态")
	}
}

// GetMonitor 获取监控器实例
func (s *RuleEngineService) GetMonitor() *RuleMonitor {
	return s.monitor
}

// GetMetrics 获取引擎监控指标
func (s *RuleEngineService) GetMetrics() *EngineMetrics {
	if s.monitor == nil {
		return &EngineMetrics{}
	}
	
	metrics := s.monitor.GetMetrics()
	
	// 更新当前规则统计
	if s.manager != nil {
		allRules := s.manager.ListRules()
		enabledRules := s.manager.GetEnabledRules()
		metrics.RulesTotal = int64(len(allRules))
		metrics.RulesEnabled = int64(len(enabledRules))
	}
	
	return metrics
}

// GetHealthStatus 获取健康状态
func (s *RuleEngineService) GetHealthStatus() HealthStatus {
	if s.monitor == nil {
		return HealthStatus{
			Status:  "unknown",
			Message: "监控器未初始化",
		}
	}
	return s.monitor.GetHealthStatus()
}

// GetErrors 获取最近的错误列表
func (s *RuleEngineService) GetErrors(limit int) []*RuleError {
	if s.monitor == nil {
		return []*RuleError{}
	}
	
	// 将MonitoringRuleError转换为RuleError
	monitoringErrors := s.monitor.GetErrors(limit)
	errors := make([]*RuleError, len(monitoringErrors))
	for i, monitoringError := range monitoringErrors {
		errors[i] = monitoringError.RuleError
	}
	return errors
}

// SetMetricsEnabled 设置是否启用监控
func (s *RuleEngineService) SetMetricsEnabled(enabled bool) {
	s.enableMetrics = enabled
}

// RegisterHealthChecker 注册健康检查器
func (s *RuleEngineService) RegisterHealthChecker(checker HealthChecker) {
	if s.monitor != nil {
		s.monitor.RegisterHealthChecker(checker)
	}
}

// GetMonitoringJSON 获取监控数据的JSON表示
func (s *RuleEngineService) GetMonitoringJSON() ([]byte, error) {
	if s.monitor == nil {
		return []byte("{}"), nil
	}
	return s.monitor.ToJSON()
}

// BuiltinAlertHandler 内建告警处理器 - 增强版
type BuiltinAlertHandler struct {
	natsConn    *nats.Conn
	throttleMap map[string]time.Time  // 节流控制
	mu          sync.RWMutex          // 并发安全
}

// Name 返回处理器名称
func (h *BuiltinAlertHandler) Name() string {
	return "BuiltinAlertHandler"
}

// InitializeForTesting 为测试初始化处理器
func (h *BuiltinAlertHandler) InitializeForTesting() {
	if h.throttleMap == nil {
		h.throttleMap = make(map[string]time.Time)
	}
}

// Execute 执行告警动作 - 增强版
func (h *BuiltinAlertHandler) Execute(ctx context.Context, point model.Point, rule *Rule, config map[string]interface{}) (*ActionResult, error) {
	start := time.Now()
	
	// 解析告警配置
	level, ok := config["level"].(string)
	if !ok {
		level = "info"
	}
	
	message, ok := config["message"].(string)
	if !ok {
		message = "触发告警"
	}
	
	// 解析节流配置
	var throttleDuration time.Duration
	if throttleStr, ok := config["throttle"].(string); ok {
		if duration, err := time.ParseDuration(throttleStr); err == nil {
			throttleDuration = duration
		}
	}
	
	// 处理消息模板
	message = h.parseMessageTemplate(message, point, rule)
	
	// 创建告警消息
	alert := &Alert{
		ID:        generateAlertID(),
		RuleID:    rule.ID,
		RuleName:  rule.Name,
		Level:     level,
		Message:   message,
		DeviceID:  point.DeviceID,
		Key:       point.Key,
		Value:     point.Value,
		Timestamp: time.Now(),
		Tags:      point.GetTagsCopy(),
	}
	
	// 检查节流
	if throttleDuration > 0 && h.shouldThrottle(alert, throttleDuration) {
		return &ActionResult{
			Type:     "alert",
			Success:  true,
			Error:    "告警被节流跳过",
			Duration: time.Since(start),
			Output:   map[string]interface{}{"throttled": true},
		}, nil
	}
	
	// 记录节流时间
	if throttleDuration > 0 {
		h.recordThrottle(alert)
	}
	
	// 发送告警到多个通道
	results := h.sendToChannels(ctx, alert, config)
	
	// 发布到NATS
	h.publishToNATS(alert, level)
	
	// 统计结果
	successCount := 0
	var errors []string
	for channel, result := range results {
		if result.Success {
			successCount++
		} else {
			errors = append(errors, fmt.Sprintf("%s: %s", channel, result.Error))
		}
	}
	
	success := successCount > 0 || len(results) == 0 // 如果没有配置通道，默认成功
	errorMsg := ""
	if len(errors) > 0 {
		errorMsg = strings.Join(errors, "; ")
	}
	
	return &ActionResult{
		Type:     "alert",
		Success:  success,
		Error:    errorMsg,
		Duration: time.Since(start),
		Output: map[string]interface{}{
			"alert_id":       alert.ID,
			"channels_sent":  successCount,
			"channels_total": len(results),
			"results":        results,
			"message":        alert.Message,
			"level":          alert.Level,
			"device_id":      alert.DeviceID,
			"key":            alert.Key,
			"value":          alert.Value,
		},
	}, nil
}

// parseMessageTemplate 解析消息模板，支持简单的占位符替换
func (h *BuiltinAlertHandler) parseMessageTemplate(templateStr string, point model.Point, rule *Rule) string {
	if templateStr == "" {
		return templateStr
	}
	
	message := templateStr
	
	// 替换基本变量
	replacements := map[string]string{
		"{{.RuleName}}":  rule.Name,
		"{{.RuleID}}":    rule.ID,
		"{{.DeviceID}}":  point.DeviceID,
		"{{.Key}}":       point.Key,
		"{{.Value}}":     fmt.Sprintf("%v", point.Value),
		"{{.Type}}":      string(point.Type),
		"{{.Timestamp}}": point.Timestamp.Format("2006-01-02 15:04:05"),
	}
	
	for placeholder, value := range replacements {
		message = strings.ReplaceAll(message, placeholder, value)
	}
	
	// 处理复杂值的嵌套路径 (如 {{.value.speed}}, {{.value.magnitude}})
	message = h.replaceNestedValuePaths(message, point.Value)
	
	// 替换标签
	pointTags := point.GetTagsSafe()
	for key, value := range pointTags {
		placeholder := fmt.Sprintf("{{.Tags.%s}}", key)
		message = strings.ReplaceAll(message, placeholder, value)
	}
	
	return message
}

// replaceNestedValuePaths 处理嵌套值路径的替换，支持{{.value.field}}格式
func (h *BuiltinAlertHandler) replaceNestedValuePaths(message string, value interface{}) string {
	// 使用正则表达式匹配 {{.value.xxx}} 模式
	re := regexp.MustCompile(`\{\{\.value\.([^}]+)\}\}`)
	matches := re.FindAllStringSubmatch(message, -1)
	
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		
		placeholder := match[0] // 完整的占位符，如 {{.value.speed}}
		fieldPath := match[1]   // 字段路径，如 speed
		
		// 尝试从value中提取字段值
		fieldValue := h.extractFieldFromValue(value, fieldPath)
		if fieldValue != nil {
			message = strings.ReplaceAll(message, placeholder, fmt.Sprintf("%v", fieldValue))
		}
	}
	
	return message
}

// extractFieldFromValue 从复杂值中提取指定字段
func (h *BuiltinAlertHandler) extractFieldFromValue(value interface{}, fieldPath string) interface{} {
	if value == nil {
		return nil
	}
	
	// 尝试将value转换为map[string]interface{}
	if valueMap, ok := value.(map[string]interface{}); ok {
		if fieldValue, exists := valueMap[fieldPath]; exists {
			return fieldValue
		}
		// 尝试不区分大小写的匹配
		for key, val := range valueMap {
			if strings.EqualFold(key, fieldPath) {
				return val
			}
		}
	}
	
	// 尝试JSON解析
	// 情况1: value是JSON字符串
	if jsonStr, ok := value.(string); ok {
		var valueMap map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &valueMap); err == nil {
			if fieldValue, exists := valueMap[fieldPath]; exists {
				return fieldValue
			}
			// 尝试不区分大小写的匹配
			for key, val := range valueMap {
				if strings.EqualFold(key, fieldPath) {
					return val
				}
			}
		}
	}
	
	// 情况2: value是其他类型，尝试通过Marshal/Unmarshal处理
	if valueBytes, err := json.Marshal(value); err == nil {
		var valueMap map[string]interface{}
		if err := json.Unmarshal(valueBytes, &valueMap); err == nil {
			if fieldValue, exists := valueMap[fieldPath]; exists {
				return fieldValue
			}
			// 尝试不区分大小写的匹配
			for key, val := range valueMap {
				if strings.EqualFold(key, fieldPath) {
					return val
				}
			}
		}
	}
	
	// 使用反射处理结构体字段
	return h.extractFieldUsingReflection(value, fieldPath)
}

// extractFieldUsingReflection 使用反射从结构体中提取字段
func (h *BuiltinAlertHandler) extractFieldUsingReflection(value interface{}, fieldPath string) interface{} {
	if value == nil {
		return nil
	}
	
	v := reflect.ValueOf(value)
	
	// 如果是指针，解引用
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	
	// 只处理结构体
	if v.Kind() != reflect.Struct {
		return nil
	}
	
	// 查找字段（不区分大小写）
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldName := field.Name
		
		// 检查字段名（不区分大小写）
		if strings.EqualFold(fieldName, fieldPath) {
			fieldValue := v.Field(i)
			if fieldValue.CanInterface() {
				return fieldValue.Interface()
			}
		}
		
		// 检查JSON标签
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			jsonName := strings.Split(jsonTag, ",")[0]
			if strings.EqualFold(jsonName, fieldPath) {
				fieldValue := v.Field(i)
				if fieldValue.CanInterface() {
					return fieldValue.Interface()
				}
			}
		}
	}
	
	return nil
}

// shouldThrottle 检查是否应该节流
func (h *BuiltinAlertHandler) shouldThrottle(alert *Alert, throttleDuration time.Duration) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	throttleKey := fmt.Sprintf("%s:%s:%s", alert.RuleID, alert.DeviceID, alert.Key)
	
	if lastTime, exists := h.throttleMap[throttleKey]; exists {
		return time.Since(lastTime) < throttleDuration
	}
	
	return false
}

// recordThrottle 记录节流时间
func (h *BuiltinAlertHandler) recordThrottle(alert *Alert) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	throttleKey := fmt.Sprintf("%s:%s:%s", alert.RuleID, alert.DeviceID, alert.Key)
	h.throttleMap[throttleKey] = time.Now()
}

// sendToChannels 发送到多个通道
func (h *BuiltinAlertHandler) sendToChannels(ctx context.Context, alert *Alert, config map[string]interface{}) map[string]ChannelResult {
	results := make(map[string]ChannelResult)
	
	// 解析通道配置
	channels := h.parseChannelConfig(config)
	
	for i, channel := range channels {
		channelKey := fmt.Sprintf("%s_%d", channel.Type, i)
		start := time.Now()
		var err error
		
		switch channel.Type {
		case "console":
			err = h.sendConsoleAlert(alert)
		case "webhook":
			err = h.sendWebhookAlert(ctx, alert, channel.Config)
		case "nats":
			err = h.sendNATSAlert(alert, channel.Config)
		case "email":
			err = h.sendEmailAlert(alert, channel.Config)
		case "sms":
			err = h.sendSMSAlert(alert, channel.Config)
		default:
			err = fmt.Errorf("不支持的通知渠道: %s", channel.Type)
		}
		
		results[channelKey] = ChannelResult{
			Success:  err == nil,
			Error:    func() string { if err != nil { return err.Error() }; return "" }(),
			Duration: time.Since(start),
		}
	}
	
	return results
}

// parseChannelConfig 解析通道配置
func (h *BuiltinAlertHandler) parseChannelConfig(config map[string]interface{}) []ChannelConfig {
	channels := []ChannelConfig{}
	
	if channelsData, ok := config["channels"]; ok {
		channelsBytes, _ := json.Marshal(channelsData)
		json.Unmarshal(channelsBytes, &channels)
	}
	
	// 如果没有配置通道，默认添加console通道
	if len(channels) == 0 {
		channels = []ChannelConfig{
			{Type: "console", Config: map[string]interface{}{}},
		}
	}
	
	return channels
}

// publishToNATS 发布到NATS
func (h *BuiltinAlertHandler) publishToNATS(alert *Alert, level string) {
	if h.natsConn != nil {
		data, err := json.Marshal(alert)
		if err == nil {
			subjects := []string{
				"iot.alerts.triggered",
				fmt.Sprintf("iot.alerts.triggered.%s", level),
			}
			
			for _, subject := range subjects {
				if err := h.natsConn.Publish(subject, data); err != nil {
					log.Error().Err(err).Str("subject", subject).Msg("发布告警到NATS失败")
				} else {
					log.Info().Str("alert_id", alert.ID).Str("subject", subject).Str("level", level).Msg("告警发布到NATS成功")
				}
			}
		}
	}
}

// sendConsoleAlert 发送控制台告警
func (h *BuiltinAlertHandler) sendConsoleAlert(alert *Alert) error {
	switch alert.Level {
	case "critical", "error":
		log.Error().
			Str("alert_id", alert.ID).
			Str("rule_id", alert.RuleID).
			Str("rule_name", alert.RuleName).
			Str("device_id", alert.DeviceID).
			Str("key", alert.Key).
			Interface("value", alert.Value).
			Interface("tags", alert.Tags).
			Msg(alert.Message)
	case "warning":
		log.Warn().
			Str("alert_id", alert.ID).
			Str("rule_id", alert.RuleID).
			Str("rule_name", alert.RuleName).
			Str("device_id", alert.DeviceID).
			Str("key", alert.Key).
			Interface("value", alert.Value).
			Interface("tags", alert.Tags).
			Msg(alert.Message)
	default:
		log.Info().
			Str("alert_id", alert.ID).
			Str("rule_id", alert.RuleID).
			Str("rule_name", alert.RuleName).
			Str("device_id", alert.DeviceID).
			Str("key", alert.Key).
			Interface("value", alert.Value).
			Interface("tags", alert.Tags).
			Msg(alert.Message)
	}
	
	return nil
}

// sendWebhookAlert 发送Webhook告警
func (h *BuiltinAlertHandler) sendWebhookAlert(ctx context.Context, alert *Alert, config map[string]interface{}) error {
	url, ok := config["url"].(string)
	if !ok || url == "" {
		return fmt.Errorf("webhook URL未配置")
	}
	
	payload := map[string]interface{}{
		"alert_id":  alert.ID,
		"rule_id":   alert.RuleID,
		"rule_name": alert.RuleName,
		"level":     alert.Level,
		"message":   alert.Message,
		"device_id": alert.DeviceID,
		"key":       alert.Key,
		"value":     alert.Value,
		"tags":      alert.Tags,
		"timestamp": alert.Timestamp.Unix(),
	}
	
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化数据失败: %w", err)
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "IoT-Gateway-Rules-Engine")
	
	if token, ok := config["token"].(string); ok && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Webhook响应错误: %d", resp.StatusCode)
	}
	
	return nil
}

// sendNATSAlert 发送NATS告警
func (h *BuiltinAlertHandler) sendNATSAlert(alert *Alert, config map[string]interface{}) error {
	subject, ok := config["subject"].(string)
	if !ok || subject == "" {
		subject = "alerts.default"
	}
	
	data, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("序列化NATS消息失败: %w", err)
	}
	
	if h.natsConn != nil {
		return h.natsConn.Publish(subject, data)
	}
	
	return fmt.Errorf("NATS连接未初始化")
}

// sendEmailAlert 发送邮件告警 (企业功能 - 占位符实现)
func (h *BuiltinAlertHandler) sendEmailAlert(alert *Alert, config map[string]interface{}) error {
	// TODO: 实现邮件发送功能
	// 预期配置参数:
	// - smtp_host: SMTP服务器地址
	// - smtp_port: SMTP端口 (25/465/587)
	// - username: 邮箱用户名
	// - password: 邮箱密码或应用密码
	// - from: 发件人邮箱
	// - to: 收件人邮箱列表
	// - subject: 邮件主题模板
	// - template: 邮件内容模板 (支持HTML)
	
	log.Info().
		Str("alert_id", alert.ID).
		Str("type", "email").
		Interface("config", config).
		Msg("邮件告警发送 (占位符实现 - 待开发)")
	
	// 返回成功以避免影响其他通道
	// 在实际实现时，应该返回真实的错误
	return nil
}

// sendSMSAlert 发送短信告警 (企业功能 - 占位符实现)
func (h *BuiltinAlertHandler) sendSMSAlert(alert *Alert, config map[string]interface{}) error {
	// TODO: 实现短信发送功能
	// 预期配置参数:
	// - provider: 短信服务商 (aliyun/tencent/twilio)
	// - access_key: 访问密钥
	// - secret_key: 密钥
	// - sign_name: 短信签名
	// - template_code: 短信模板代码
	// - phone_numbers: 接收手机号列表
	// - template_params: 模板参数
	
	log.Info().
		Str("alert_id", alert.ID).
		Str("type", "sms").
		Interface("config", config).
		Msg("短信告警发送 (占位符实现 - 待开发)")
	
	// 返回成功以避免影响其他通道
	// 在实际实现时，应该返回真实的错误
	return nil
}

// sendToChannelsWithRetry 带重试机制的多通道发送 (企业功能 - 占位符实现)
func (h *BuiltinAlertHandler) sendToChannelsWithRetry(ctx context.Context, alert *Alert, config map[string]interface{}) map[string]ChannelResult {
	// TODO: 实现重试机制
	// 预期功能:
	// - 指数退避重试策略
	// - 每个通道独立重试计数
	// - 重试间隔可配置
	// - 最大重试次数限制
	// - 失败时的故障转移通道
	// - 重试状态跟踪和日志
	
	// 目前回退到标准发送方式
	// 当需要重试功能时，可以在Execute方法中调用此方法
	log.Debug().
		Str("alert_id", alert.ID).
		Msg("重试机制发送 (占位符实现 - 使用标准发送)")
	
	return h.sendToChannels(ctx, alert, config)
}

// enableChannelFailover 启用通道故障转移 (企业功能 - 占位符实现)
func (h *BuiltinAlertHandler) enableChannelFailover(config map[string]interface{}) []ChannelConfig {
	// TODO: 实现故障转移机制
	// 预期功能:
	// - 主通道失败时自动切换到备用通道
	// - 通道健康检查和监控
	// - 故障转移策略配置 (立即/延迟/条件)
	// - 故障恢复后的回切机制
	// - 故障转移事件记录和告警
	
	// 目前返回标准通道配置
	log.Debug().Msg("通道故障转移 (占位符实现 - 使用标准通道)")
	
	return h.parseChannelConfig(config)
}

// trackDeliveryStatus 跟踪投递状态 (企业功能 - 占位符实现)
func (h *BuiltinAlertHandler) trackDeliveryStatus(alert *Alert, channel string, result ChannelResult) {
	// TODO: 实现投递状态跟踪
	// 预期功能:
	// - 投递状态持久化存储
	// - 投递成功率统计
	// - 通道性能监控
	// - 投递失败原因分析
	// - 投递历史查询接口
	// - 投递状态回调通知
	
	log.Debug().
		Str("alert_id", alert.ID).
		Str("channel", channel).
		Bool("success", result.Success).
		Dur("duration", result.Duration).
		Msg("投递状态跟踪 (占位符实现 - 待开发)")
}

// validateChannelConfig 验证通道配置 (企业功能增强 - 占位符实现)
func (h *BuiltinAlertHandler) validateChannelConfig(channels []ChannelConfig) error {
	// TODO: 实现企业级配置验证
	// 预期功能:
	// - 邮件SMTP连接测试
	// - 短信服务商API验证
	// - Webhook端点可达性检查
	// - 配置参数完整性验证
	// - 安全配置检查 (SSL/TLS)
	// - 配置模板语法验证
	
	for _, channel := range channels {
		log.Debug().
			Str("type", channel.Type).
			Msg("通道配置验证 (占位符实现 - 跳过)")
	}
	
	return nil
}

// startCleanupRoutine 启动清理协程
func (h *BuiltinAlertHandler) startCleanupRoutine() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		h.mu.Lock()
		now := time.Now()
		for key, lastTime := range h.throttleMap {
			// 清理超过1小时的记录
			if now.Sub(lastTime) > time.Hour {
				delete(h.throttleMap, key)
			}
		}
		h.mu.Unlock()
	}
}

// ChannelConfig 通知渠道配置
type ChannelConfig struct {
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config"`
}

// ChannelResult 渠道发送结果
type ChannelResult struct {
	Success  bool          `json:"success"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
}

// generateAlertID 生成告警ID
func generateAlertID() string {
	return fmt.Sprintf("alert_%d", time.Now().UnixNano())
}


// rebuildRuleIndex 重新构建规则索引
func (s *RuleEngineService) rebuildRuleIndex() {
	if s.ruleIndex == nil {
		log.Warn().Msg("规则索引未初始化，跳过重建")
		return
	}
	
	start := time.Now()
	
	// 清空现有索引
	s.ruleIndex.Clear()
	
	// 获取所有启用的规则
	rules := s.manager.GetEnabledRules()
	
	// 添加规则到索引
	for _, rule := range rules {
		s.ruleIndex.AddRule(rule)
	}
	
	duration := time.Since(start)
	stats := s.ruleIndex.GetStats()
	
	log.Info().
		Int("rules_indexed", len(rules)).
		Dur("build_time", duration).
		Interface("index_stats", stats).
		Msg("🔍 规则索引重建完成")
}

// updateRuleIndex 更新规则索引（当规则变化时调用）
func (s *RuleEngineService) updateRuleIndex(rule *Rule, operation string) {
	if !s.useRuleIndex || s.ruleIndex == nil {
		return
	}
	
	switch operation {
	case "add", "update":
		s.ruleIndex.AddRule(rule)
		log.Debug().
			Str("rule_id", rule.ID).
			Str("operation", operation).
			Msg("更新规则索引")
	case "remove":
		s.ruleIndex.RemoveRule(rule)
		log.Debug().
			Str("rule_id", rule.ID).
			Str("operation", operation).
			Msg("从规则索引中移除")
	}
}

// GetRuleIndexStats 获取规则索引统计信息
func (s *RuleEngineService) GetRuleIndexStats() map[string]interface{} {
	if !s.useRuleIndex || s.ruleIndex == nil {
		return map[string]interface{}{
			"enabled": false,
			"reason": "规则索引未启用或未初始化",
		}
	}
	
	stats := s.ruleIndex.GetStats()
	stats["enabled"] = true
	stats["use_rule_index"] = s.useRuleIndex
	
	return stats
}

// publishRuleEvent 发布规则执行事件到NATS
func (s *RuleEngineService) publishRuleEvent(eventType string, rule *Rule, point model.Point, eventData map[string]interface{}) {
	if s.bus == nil {
		return
	}

	// 构建事件数据 - Go 1.24增强版：在调用点增加额外保护
	// 使用专门的安全包装器处理潜在的并发map访问
	safePointTags := safeExtractMapForEventPublishing(point.GetTagsCopy())
	
	event := map[string]interface{}{
		"event_type": eventType,
		"timestamp":  time.Now(),
		"rule": map[string]interface{}{
			"id":       rule.ID,
			"name":     rule.Name,
			"priority": rule.Priority,
			"enabled":  rule.Enabled,
		},
		"data_point": map[string]interface{}{
			"device_id": point.DeviceID,
			"key":       point.Key,
			"value":     SafeValueForJSON(point.Value),
			"type":      string(point.Type),
			"timestamp": point.Timestamp,
			"tags":      safePointTags, // 使用预处理的安全tags
		},
	}

	// 合并事件特定数据，确保安全
	for key, value := range eventData {
		event[key] = SafeValueForJSON(value)
	}

	// 序列化事件数据
	eventJSON, err := json.Marshal(event)
	if err != nil {
		log.Error().Err(err).Str("event_type", eventType).Msg("序列化规则事件失败")
		return
	}

	// 确定发布主题
	subject := fmt.Sprintf("iot.rules.%s", eventType)

	// 发布事件
	if err := s.bus.Publish(subject, eventJSON); err != nil {
		log.Error().
			Err(err).
			Str("subject", subject).
			Str("event_type", eventType).
			Str("rule_id", rule.ID).
			Msg("发布规则事件失败")
	} else {
		log.Debug().
			Str("subject", subject).
			Str("event_type", eventType).
			Str("rule_id", rule.ID).
			Int("bytes", len(eventJSON)).
			Msg("规则事件发布成功")
	}
}

// SetHotReloadEnabled 动态设置热加载状态
func (s *RuleEngineService) SetHotReloadEnabled(enabled bool) error {
	if s.config == nil {
		s.config = &RuleEngineConfig{}
	}
	if s.config.HotReload == nil {
		s.config.HotReload = &HotReloadConfig{
			Enabled:          true,
			GracefulFallback: true,
			RetryInterval:    "30s",
			MaxRetries:       3,
			DebounceDelay:    "100ms",
		}
	}
	
	oldEnabled := s.config.HotReload.Enabled
	s.config.HotReload.Enabled = enabled
	
	// 通知规则管理器
	if s.manager != nil {
		s.manager.SetHotReloadConfig(s.config.HotReload)
	}
	
	log.Info().
		Bool("old_enabled", oldEnabled).
		Bool("new_enabled", enabled).
		Msg("规则文件热加载状态已更新")
	
	return nil
}

// GetHotReloadStatus 获取热加载状态
func (s *RuleEngineService) GetHotReloadStatus() map[string]interface{} {
	status := map[string]interface{}{
		"enabled":           false,
		"graceful_fallback": true,
		"retry_interval":    "30s",
		"max_retries":       3,
		"debounce_delay":    "100ms",
	}
	
	if s.config != nil && s.config.HotReload != nil {
		status["enabled"] = s.config.HotReload.Enabled
		status["graceful_fallback"] = s.config.HotReload.GracefulFallback
		status["retry_interval"] = s.config.HotReload.RetryInterval
		status["max_retries"] = s.config.HotReload.MaxRetries
		status["debounce_delay"] = s.config.HotReload.DebounceDelay
	}
	
	return status
}
