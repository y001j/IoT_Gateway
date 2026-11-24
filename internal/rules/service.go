package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
)

// RuleEngineConfig 规则引擎配置
type RuleEngineConfig struct {
	Enabled   bool             `yaml:"enabled" json:"enabled"`
	RulesDir  string           `yaml:"rules_dir" json:"rules_dir"`
	Rules     []*Rule          `yaml:"rules" json:"rules"`
	Subject   string           `yaml:"subject" json:"subject"`
	HotReload *HotReloadConfig `yaml:"hot_reload" json:"hot_reload"` // 热加载配置

	// PublishRuleEvents 是否为每次条件评估发布 iot.rules.evaluated / action_executed / executed 事件。
	// 这些事件的数量与 "数据点数 × 候选规则数" 成正比，默认关闭；
	// 规则命中事件 iot.rules.matched 始终发布。
	PublishRuleEvents bool `yaml:"publish_rule_events" json:"publish_rule_events"`
	// PendingLimitMsgs / PendingLimitBytes NATS 订阅的本地缓冲上限
	PendingLimitMsgs  int `yaml:"pending_limit_msgs" json:"pending_limit_msgs"`
	PendingLimitBytes int `yaml:"pending_limit_bytes" json:"pending_limit_bytes"`
}

// RuleOutputHeader 规则引擎发布的数据（转发、聚合结果等）携带的消息头。
// 规则引擎订阅数据流时会忽略带有该消息头的消息，防止规则输出被再次处理形成环路。
const RuleOutputHeader = "Iot-Origin"

// RuleOutputHeaderValue RuleOutputHeader 的取值
const RuleOutputHeaderValue = "rule-engine"

// PublishRuleOutput 发布规则输出消息（带环路保护消息头）
func PublishRuleOutput(nc *nats.Conn, subject string, data []byte) error {
	if nc == nil {
		return fmt.Errorf("NATS连接未初始化")
	}
	msg := nats.NewMsg(subject)
	msg.Data = data
	msg.Header.Set(RuleOutputHeader, RuleOutputHeaderValue)
	return nc.PublishMsg(msg)
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
	handlersMu     sync.RWMutex

	// Runtime引用
	runtime interface{}

	// 并行处理
	maxWorkers    int
	queueSize     int
	submitTimeout time.Duration
	optimizedPool *OptimizedWorkerPool

	// 统计
	droppedPoints     atomic.Int64
	loopGuardSkipped  atomic.Int64
	publishRuleEvents bool

	// 监控和错误处理
	monitor       *RuleMonitor
	enableMetrics bool

	// 规则索引系统
	ruleIndex    *Index
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

// NewRuleEngineService 创建规则引擎服务
func NewRuleEngineService() *RuleEngineService {
	// 使用更大的默认值以应对高负载场景
	service := &RuleEngineService{
		actionHandlers: make(map[string]ActionHandler),
		maxWorkers:     16,   // 增大默认worker数量
		queueSize:      5000, // 增大默认队列大小
		enableMetrics:  true, // 默认启用监控

		// 启用新的优化组件
		useRuleIndex: true, // 启用规则索引系统
	}

	// 初始化监控器
	service.monitor = NewRuleMonitor(1000) // 保留最近1000个错误

	// 初始化规则索引
	service.ruleIndex = NewIndex()

	return service
}

// NewRuleEngineServiceWithConfig 使用配置创建规则引擎服务
func NewRuleEngineServiceWithConfig(config map[string]interface{}) *RuleEngineService {
	service := NewRuleEngineService()

	// 解析工作池配置（viper 解析出的数字可能是 int 或 float64）
	if poolConfig, ok := config["worker_pool"].(map[string]interface{}); ok {
		if n, ok := toInt(poolConfig["max_workers"]); ok && n > 0 {
			service.maxWorkers = n
		}
		if n, ok := toInt(poolConfig["queue_size"]); ok && n > 0 {
			service.queueSize = n
		}
		if v, ok := poolConfig["submit_timeout"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil {
				service.submitTimeout = d
			}
		}
	}

	// 解析热加载配置
	if hotReloadConfig, ok := config["hot_reload"].(map[string]interface{}); ok {
		if service.config == nil {
			service.config = &RuleEngineConfig{}
		}
		if service.config.HotReload == nil {
			service.config.HotReload = &HotReloadConfig{}
		}
		hr := service.config.HotReload
		if enabled, ok := hotReloadConfig["enabled"].(bool); ok {
			hr.Enabled = enabled
		}
		if gracefulFallback, ok := hotReloadConfig["graceful_fallback"].(bool); ok {
			hr.GracefulFallback = gracefulFallback
		}
		if retryInterval, ok := hotReloadConfig["retry_interval"].(string); ok {
			hr.RetryInterval = retryInterval
		}
		if maxRetries, ok := toInt(hotReloadConfig["max_retries"]); ok {
			hr.MaxRetries = maxRetries
		}
		if debounceDelay, ok := hotReloadConfig["debounce_delay"].(string); ok {
			hr.DebounceDelay = debounceDelay
		}
	}

	return service
}

// toInt 把配置中的数字（int/int64/float64/字符串）转换为 int
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	case string:
		var i int
		if _, err := fmt.Sscanf(n, "%d", &i); err == nil {
			return i, true
		}
	}
	return 0, false
}

// 动作处理器将在运行时注册

// RegisterActionHandler 注册动作处理器（并发安全，可在运行时调用）
func (s *RuleEngineService) RegisterActionHandler(actionType string, handler ActionHandler) {
	s.handlersMu.Lock()
	s.actionHandlers[actionType] = handler
	s.handlersMu.Unlock()
	log.Info().Str("type", actionType).Str("name", handler.Name()).Msg("动作处理器已注册")
}

// GetActionHandler 返回已注册的动作处理器
func (s *RuleEngineService) GetActionHandler(actionType string) (ActionHandler, bool) {
	return s.getActionHandler(actionType)
}

// getActionHandler 获取动作处理器
func (s *RuleEngineService) getActionHandler(actionType string) (ActionHandler, bool) {
	s.handlersMu.RLock()
	h, ok := s.actionHandlers[actionType]
	s.handlersMu.RUnlock()
	return h, ok
}

// handleAggregateResult 根据聚合结果生成数据点，并按配置转发到 iot.rules.aggregate.<device>.<key>。
//
// 输出配置支持两种写法：
//
//	{"output": {"key_template": "{{.Key}}_avg", "forward": true}}
//	{"output_key": "temp_avg", "forward": true}
//
// 数据点的值取 functions 配置中第一个已计算出的函数（以前遍历 map 取"第一个"，
// Go 的 map 遍历顺序随机，配置多个函数时输出值不确定）。
func (s *RuleEngineService) handleAggregateResult(aggregateResult *AggregateResult, originalPoint model.Point, rule *Rule, action *Action) error {
	if aggregateResult == nil {
		return nil
	}
	config := action.Config
	outputKey := ""
	forward := false
	if output, ok := config["output"].(map[string]interface{}); ok {
		if keyTemplate, ok := output["key_template"].(string); ok {
			outputKey = s.formatOutputKey(keyTemplate, originalPoint)
		}
		forward, _ = output["forward"].(bool)
	} else {
		outputKey, _ = config["output_key"].(string)
		forward, _ = config["forward"].(bool)
	}
	if outputKey == "" {
		outputKey = "aggregated_result"
	}

	fnName, value := primaryAggregateValue(aggregateResult.Functions, config)
	deviceID := aggregateResult.DeviceID
	if deviceID == "" {
		deviceID = originalPoint.DeviceID
	}
	ts := aggregateResult.Timestamp
	if ts.IsZero() {
		ts = originalPoint.Timestamp
	}

	resultPoint := model.Point{
		DeviceID:  deviceID,
		Key:       outputKey,
		Value:     value,
		Type:      model.TypeFloat,
		Timestamp: ts,
	}
	resultPoint.SetTagsSafe(originalPoint.GetTagsCopy())
	resultPoint.AddTag("aggregated", "true")
	resultPoint.AddTag("source_rule", rule.ID)
	resultPoint.AddTag("window_count", fmt.Sprintf("%d", aggregateResult.Count))
	if fnName != "" {
		resultPoint.AddTag("function", fnName)
	}

	if forward {
		if err := s.publishPoint(resultPoint); err != nil {
			return fmt.Errorf("发布聚合结果失败: %w", err)
		}
	}
	return nil
}

// primaryAggregateValue 按 functions 配置的顺序选出第一个已计算出的函数值
func primaryAggregateValue(functions map[string]interface{}, config map[string]interface{}) (string, interface{}) {
	if len(functions) == 0 {
		return "", 0.0
	}
	var order []string
	switch f := config["functions"].(type) {
	case []interface{}:
		for _, v := range f {
			if name, ok := v.(string); ok {
				order = append(order, name)
			}
		}
	case []string:
		order = f
	case string:
		order = []string{f}
	}
	order = append(order, "avg")
	for _, name := range order {
		if v, ok := functions[name]; ok {
			return name, v
		}
	}
	keys := make([]string, 0, len(functions))
	for k := range functions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys[0], functions[keys[0]]
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
	if config.HotReload == nil && s.config != nil {
		config.HotReload = s.config.HotReload
	}

	s.config = &config
	s.publishRuleEvents = config.PublishRuleEvents

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

	// 创建并启动工作池（按数据序列分片，保证同一序列的处理顺序）
	s.optimizedPool = NewOptimizedWorkerPool(WorkerPoolConfig{
		NumWorkers:    s.maxWorkers,
		QueueSize:     s.queueSize,
		SubmitTimeout: s.submitTimeout,
	}, s)
	if err := s.optimizedPool.Start(); err != nil {
		return fmt.Errorf("启动工作池失败: %w", err)
	}

	// 注册内建的告警处理器（如果外部没有注册其他实现）
	if _, exists := s.getActionHandler("alert"); !exists {
		builtinAlertHandler := &BuiltinAlertHandler{
			natsConn:    s.bus,
			throttleMap: make(map[string]time.Time),
		}
		go builtinAlertHandler.startCleanupRoutine()
		s.RegisterActionHandler("alert", builtinAlertHandler)
	}

	// 订阅数据主题
	if err := s.subscribeToDataStream(); err != nil {
		log.Error().Err(err).Msg("订阅数据流失败")
		return fmt.Errorf("订阅数据流失败: %w", err)
	}

	// 启动规则监控
	s.wg.Add(1)
	go s.watchRuleChanges()

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

	// 先取消订阅，不再接收新数据；再停止工作池（处理完队列中的任务）
	if s.sub != nil {
		_ = s.sub.Unsubscribe()
	}
	if s.optimizedPool != nil {
		_ = s.optimizedPool.Stop()
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
	// 默认的本地缓冲上限（65536 条 / 64MB）在高频场景下很容易触发 slow consumer 丢消息
	msgs, bytes := s.config.PendingLimitMsgs, s.config.PendingLimitBytes
	if msgs <= 0 {
		msgs = 500000
	}
	if bytes <= 0 {
		bytes = 256 * 1024 * 1024
	}
	if err := s.sub.SetPendingLimits(msgs, bytes); err != nil {
		log.Warn().Err(err).Msg("设置订阅缓冲上限失败")
	}

	log.Info().Str("subject", s.config.Subject).Int("pending_limit_msgs", msgs).Msg("已订阅数据流")
	return nil
}

// handleDataPoint 处理数据点（NATS 订阅回调）
func (s *RuleEngineService) handleDataPoint(msg *nats.Msg) {
	// 环路保护：忽略规则引擎自己发布的输出
	if msg.Header != nil && msg.Header.Get(RuleOutputHeader) == RuleOutputHeaderValue {
		s.loopGuardSkipped.Add(1)
		return
	}

	var point model.Point
	if err := json.Unmarshal(msg.Data, &point); err != nil {
		if s.enableMetrics && s.monitor != nil {
			s.monitor.RecordError(ErrorTypeValidation, ErrorLevelError,
				"解析数据点失败", err.Error(),
				map[string]string{"subject": msg.Subject})
		}
		log.Debug().Err(err).Str("subject", msg.Subject).Msg("解析数据点失败")
		return
	}
	s.dispatchPoint(point)
}

// dispatchPoint 为数据点匹配候选规则并提交给工作池
func (s *RuleEngineService) dispatchPoint(point model.Point) {
	var rules []*Rule
	if s.useRuleIndex && s.ruleIndex != nil {
		rules = s.ruleIndex.Match(point)
	} else if s.manager != nil {
		rules = s.manager.GetEnabledRules()
	}

	for _, rule := range rules {
		if rule == nil || !rule.Enabled {
			continue
		}
		if s.optimizedPool == nil {
			// 没有工作池（例如测试或尚未启动）时同步处理
			_ = s.processRule(rule, point)
			continue
		}
		if !s.optimizedPool.SubmitTask(RuleTask{Rule: rule, Point: point}) {
			// 不再回退为同步处理：同步处理会和队列中较早的点乱序，
			// 丢弃数量通过工作池统计和 GetDroppedPoints 观测
			s.droppedPoints.Add(1)
		}
	}
}

// GetDroppedPoints 返回因工作池过载而未处理的 (数据点, 规则) 任务数
func (s *RuleEngineService) GetDroppedPoints() int64 {
	return s.droppedPoints.Load()
}

// GetLoopGuardSkipped 返回被环路保护忽略的消息数
func (s *RuleEngineService) GetLoopGuardSkipped() int64 {
	return s.loopGuardSkipped.Load()
}

// GetWorkerPoolStats 返回工作池统计
func (s *RuleEngineService) GetWorkerPoolStats() *WorkerPoolStats {
	if s.optimizedPool == nil {
		return nil
	}
	st := s.optimizedPool.GetStats()
	return &st
}

// processRuleTask 处理规则任务
func (s *RuleEngineService) processRuleTask(task RuleTask) {
	_ = s.processRule(task.Rule, task.Point)
}

// processRule 处理单个规则：评估条件，命中后按顺序执行动作管道。
//
// 动作管道语义：
//   - filter 动作判定需要过滤时，终止后续动作；
//   - transform 动作的输出（transformed_point）作为后续动作的输入；
//   - 某个动作失败不影响后续动作执行（例如告警仍然要发出）。
func (s *RuleEngineService) processRule(rule *Rule, point model.Point) error {
	if rule == nil || !rule.Enabled {
		return nil
	}

	start := time.Now()
	matched, err := s.evaluator.Evaluate(rule.Conditions, point)
	duration := time.Since(start)

	if s.enableMetrics && s.monitor != nil {
		s.monitor.RecordRuleExecution(rule.ID, duration, matched, err)
	}
	if err != nil {
		if s.enableMetrics && s.monitor != nil {
			s.monitor.RecordError(ErrorTypeCondition, ErrorLevelError,
				"规则条件评估失败", err.Error(),
				map[string]string{
					"rule_id":   rule.ID,
					"rule_name": rule.Name,
					"device_id": point.DeviceID,
					"key":       point.Key,
				})
		}
		log.Debug().Err(err).Str("rule_id", rule.ID).Msg("规则条件评估失败")
		return fmt.Errorf("规则条件评估失败: %w", err)
	}

	if s.publishRuleEvents {
		s.publishRuleEvent("evaluated", rule, point, map[string]interface{}{
			"matched":     matched,
			"duration_ns": duration.Nanoseconds(),
		})
	}
	if !matched {
		return nil
	}

	s.publishRuleEvent("matched", rule, point, map[string]interface{}{
		"matched":       true,
		"duration_ns":   duration.Nanoseconds(),
		"actions_count": len(rule.Actions),
	})

	current := point
	executed := make([]map[string]interface{}, 0, len(rule.Actions))
	var totalDuration time.Duration
	successCount, errorCount := 0, 0
	filtered := false

	for i := range rule.Actions {
		action := &rule.Actions[i]
		actionStart := time.Now()
		result, err := s.executeAction(action, current, rule)
		actionDuration := time.Since(actionStart)
		totalDuration += actionDuration

		entry := map[string]interface{}{
			"type":        action.Type,
			"index":       i,
			"duration_ns": actionDuration.Nanoseconds(),
			"success":     err == nil,
		}
		if err != nil {
			errorCount++
			entry["error"] = err.Error()
			if s.enableMetrics && s.monitor != nil {
				s.monitor.RecordError(ErrorTypeAction, ErrorLevelError,
					fmt.Sprintf("动作执行失败: %s", action.Type), err.Error(),
					map[string]string{
						"rule_id":      rule.ID,
						"rule_name":    rule.Name,
						"action_type":  action.Type,
						"action_index": fmt.Sprintf("%d", i),
						"device_id":    current.DeviceID,
						"key":          current.Key,
					})
			}
			log.Debug().Err(err).Str("rule_id", rule.ID).Str("action_type", action.Type).Msg("执行规则动作失败")
		} else {
			successCount++
			if next, stop := applyActionResult(action.Type, result, current); stop {
				filtered = true
				entry["filtered"] = true
			} else {
				current = next
			}
		}
		executed = append(executed, entry)

		if s.publishRuleEvents {
			s.publishRuleEvent("action_executed", rule, current, map[string]interface{}{"action": entry})
		}
		if filtered {
			break
		}
	}

	if s.publishRuleEvents {
		s.publishRuleEvent("executed", rule, current, map[string]interface{}{
			"matched":                true,
			"filtered":               filtered,
			"total_duration_ns":      (duration + totalDuration).Nanoseconds(),
			"evaluation_duration_ns": duration.Nanoseconds(),
			"actions_duration_ns":    totalDuration.Nanoseconds(),
			"actions":                executed,
			"actions_total":          len(rule.Actions),
			"actions_success":        successCount,
			"actions_error":          errorCount,
		})
	}
	return nil
}

// applyActionResult 根据动作结果决定管道的下一步：
// 返回 (下一个动作的输入, 是否终止管道)
func applyActionResult(actionType string, result *ActionResult, current model.Point) (model.Point, bool) {
	if result == nil {
		return current, false
	}
	output, ok := result.Output.(map[string]interface{})
	if !ok {
		return current, false
	}
	if f, ok := output["filtered"].(bool); ok && f {
		return current, true
	}
	switch tp := output["transformed_point"].(type) {
	case model.Point:
		return tp, false
	case *model.Point:
		if tp != nil {
			return *tp, false
		}
	}
	return current, false
}

// processRuleTaskInternal 工作池调用的处理入口
func (s *RuleEngineService) processRuleTaskInternal(rule *Rule, point model.Point) error {
	return s.processRule(rule, point)
}

// executeAction 执行单个动作，返回动作结果。
// 处理器返回 Success=false 的结果时视为执行失败。动作统计只在这里记录一次。
func (s *RuleEngineService) executeAction(action *Action, point model.Point, rule *Rule) (*ActionResult, error) {
	actionStart := time.Now()
	var result *ActionResult
	var err error

	if handler, exists := s.getActionHandler(action.Type); exists {
		result, err = handler.Execute(context.Background(), point, rule, action.Config)
		if err == nil && result != nil && !result.Success {
			msg := result.Error
			if msg == "" {
				msg = "动作执行失败"
			}
			err = fmt.Errorf("%s", msg)
		}
		if err == nil && action.Type == "aggregate" && result != nil {
			if output, ok := result.Output.(map[string]interface{}); ok {
				if aggregated, ok := output["aggregated"].(bool); ok && aggregated {
					var herr error
					switch ar := output["aggregate_result"].(type) {
					case *AggregateResult: // actions.AggregateHandler
						herr = s.handleAggregateResult(ar, point, rule, action)
					case map[string]interface{}: // actions.OptimizedAggregateHandler
						herr = s.handleOptimizedAggregateResult(ar, point, rule, action)
					}
					if herr != nil {
						log.Error().Err(herr).Msg("处理聚合结果失败")
					}
				}
			}
		}
	} else {
		// 回退到内置的简化实现
		switch action.Type {
		case "aggregate":
			err = fmt.Errorf("aggregate 动作处理器未注册")
		case "transform":
			err = s.executeTransformAction(action, point, rule)
		case "filter":
			err = fmt.Errorf("filter 动作处理器未注册")
		case "forward":
			err = s.executeForwardAction(action, point, rule)
		case "alert":
			err = s.executeAlertAction(action, point, rule)
		default:
			err = fmt.Errorf("不支持的动作类型: %s", action.Type)
		}
		result = &ActionResult{Type: action.Type, Success: err == nil}
		if err != nil {
			result.Error = err.Error()
		}
	}

	if result != nil {
		result.Duration = time.Since(actionStart)
	}
	if s.enableMetrics && s.monitor != nil {
		s.monitor.RecordActionExecution(action.Type, time.Since(actionStart), err == nil, err)
	}
	return result, err
}

// handleOptimizedAggregateResult 处理 OptimizedAggregateHandler 输出的 map 格式聚合结果
func (s *RuleEngineService) handleOptimizedAggregateResult(aggregateResult map[string]interface{}, originalPoint model.Point, rule *Rule, action *Action) error {
	return s.handleAggregateResult(aggregateResultFromMap(aggregateResult), originalPoint, rule, action)
}

// aggregateResultFromMap 把 map 格式的聚合结果转换为 AggregateResult。
// functions 可能是 map[string]interface{} 或 map[string]float64（以前只识别前者，后者的结果值会丢失）。
func aggregateResultFromMap(m map[string]interface{}) *AggregateResult {
	r := &AggregateResult{Functions: map[string]interface{}{}}
	r.DeviceID, _ = m["device_id"].(string)
	r.Key, _ = m["key"].(string)
	r.Window, _ = m["window"].(string)
	r.Timestamp, _ = m["timestamp"].(time.Time)
	r.StartTime, _ = m["start_time"].(time.Time)
	r.EndTime, _ = m["end_time"].(time.Time)
	if c, ok := toInt(m["count"]); ok {
		r.Count = int64(c)
	} else if c, ok := m["count"].(int64); ok {
		r.Count = c
	}
	switch f := m["functions"].(type) {
	case map[string]interface{}:
		r.Functions = f
	case map[string]float64:
		for k, v := range f {
			r.Functions[k] = v
		}
	}
	return r
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

	log.Debug().
		Str("rule_id", rule.ID).
		Str("point_key", point.Key).
		Interface("original_value", point.Value).
		Interface("transformed_value", transformedPoint.Value).
		Msg("执行转换动作")

	return nil
}

// executeForwardAction 执行转发动作
func (s *RuleEngineService) executeForwardAction(action *Action, point model.Point, rule *Rule) error {
	config := action.Config

	if s.bus == nil {
		return fmt.Errorf("NATS连接未初始化")
	}

	// 获取目标主题。默认发布到 iot.rules.forward.<device>.<key>；
	// 以前默认发布到 iot.data.*，会被规则引擎再次消费形成死循环。
	subject, ok := config["subject"].(string)
	if !ok || subject == "" {
		subject = model.RuleOutputSubject("forward", point.DeviceID, point.Key)
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

	if err := PublishRuleOutput(s.bus, subject, jsonData); err != nil {
		return fmt.Errorf("发送NATS消息失败: %w", err)
	}
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

// publishPoint 发布规则产生的数据点（例如聚合结果）到 iot.rules.aggregate.<device>.<key>
func (s *RuleEngineService) publishPoint(point model.Point) error {
	data, err := json.Marshal(point)
	if err != nil {
		return fmt.Errorf("序列化数据点失败: %w", err)
	}
	subject := model.RuleOutputSubject("aggregate", point.DeviceID, point.Key)
	if err := PublishRuleOutput(s.bus, subject, data); err != nil {
		return fmt.Errorf("发布数据点失败: %w", err)
	}
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
	throttleMap map[string]time.Time // 节流控制
	mu          sync.RWMutex         // 并发安全
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
			Success: err == nil,
			Error: func() string {
				if err != nil {
					return err.Error()
				}
				return ""
			}(),
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
		Interface("config", RedactConfig(config)).
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
		Interface("config", RedactConfig(config)).
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
			"reason":  "规则索引未启用或未初始化",
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
