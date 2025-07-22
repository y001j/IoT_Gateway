package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
)

// RuleEngineConfig 规则引擎配置
type RuleEngineConfig struct {
	Enabled  bool    `yaml:"enabled" json:"enabled"`
	RulesDir string  `yaml:"rules_dir" json:"rules_dir"`
	Rules    []*Rule `yaml:"rules" json:"rules"`
	Subject  string  `yaml:"subject" json:"subject"`
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

	// Runtime引用
	runtime interface{}
	
	// 并行处理优化
	workerPool    *WorkerPool
	ruleTaskQueue chan RuleTask
	maxWorkers    int
	queueSize     int
	
	// 监控和错误处理
	monitor       *RuleMonitor
	enableMetrics bool
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
	service := &RuleEngineService{
		actionHandlers:  make(map[string]ActionHandler),
		aggregateStates: make(map[string]*AggregateState),
		aggregateMutex:  sync.RWMutex{},
		maxWorkers:      4,    // 默认4个工作协程
		queueSize:       1000, // 默认队列大小
		enableMetrics:   true, // 默认启用监控
	}
	
	// 初始化监控器
	service.monitor = NewRuleMonitor(1000) // 保留最近1000个错误
	
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
	log.Info().Msg("规则引擎工作池停止")
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
	
	log.Debug().Int("worker_id", workerID).Msg("规则引擎工作协程启动")
	
	for {
		select {
		case <-wp.ctx.Done():
			log.Debug().Int("worker_id", workerID).Msg("规则引擎工作协程退出")
			return
		case task, ok := <-wp.taskQueue:
			if !ok {
				log.Debug().Int("worker_id", workerID).Msg("任务队列已关闭，工作协程退出")
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

	// 创建聚合结果数据点
	resultPoint := model.Point{
		DeviceID:  aggregateResult.DeviceID,
		Key:       outputKey,
		Value:     aggregatedValue,
		Type:      model.TypeFloat,
		Timestamp: aggregateResult.Timestamp,
		Tags:      originalPoint.Tags,
	}

	// 添加聚合标签
	if resultPoint.Tags == nil {
		resultPoint.Tags = make(map[string]string)
	}
	resultPoint.Tags["aggregated"] = "true"
	resultPoint.Tags["source_rule"] = rule.ID
	resultPoint.Tags["window_count"] = fmt.Sprintf("%d", aggregateResult.Count)

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
	s.evaluator = NewEvaluator()

	log.Info().
		Str("rules_dir", s.config.RulesDir).
		Str("subject", s.config.Subject).
		Bool("enabled", s.config.Enabled).
		Msg("规则引擎服务初始化完成")

	return nil
}

// Start 启动服务
func (s *RuleEngineService) Start(ctx context.Context) error {
	log.Info().Msg("开始启动规则引擎服务...")

	if !s.config.Enabled {
		log.Info().Msg("规则引擎服务已禁用")
		return nil
	}

	log.Info().Msg("规则引擎服务已启用，继续启动...")
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 加载规则
	log.Info().Msg("开始加载规则文件...")
	if err := s.manager.LoadRules(); err != nil {
		log.Error().Err(err).Msg("加载规则文件失败")
		return fmt.Errorf("加载规则失败: %w", err)
	}
	log.Info().Msg("规则文件加载成功")

	// 加载配置中的内联规则
	log.Info().Msg("开始加载内联规则...")
	if err := s.loadInlineRules(); err != nil {
		log.Error().Err(err).Msg("加载内联规则失败")
		return fmt.Errorf("加载内联规则失败: %w", err)
	}
	log.Info().Msg("内联规则加载成功")

	// 获取NATS连接
	log.Info().Msg("开始设置NATS连接...")
	if err := s.setupNATSConnection(ctx); err != nil {
		log.Error().Err(err).Msg("设置NATS连接失败")
		return fmt.Errorf("设置NATS连接失败: %w", err)
	}
	log.Info().Msg("NATS连接设置成功")

	// 创建并启动工作池
	log.Info().Msg("启动规则引擎工作池...")
	s.workerPool = NewWorkerPool(s.maxWorkers, s.queueSize, s)
	s.workerPool.Start()
	log.Info().Int("workers", s.maxWorkers).Int("queue_size", s.queueSize).Msg("规则引擎工作池启动成功")

	// 注册动作处理器
	log.Info().Msg("注册动作处理器...")
	
	// 注册内建的动作处理器
	s.RegisterActionHandler("alert", &BuiltinAlertHandler{natsConn: s.bus})
	
	// Transform和Forward处理器需要在外部注册，以避免循环导入
	// 这些处理器应该在main函数或runtime中注册
	
	log.Info().Int("handlers", len(s.actionHandlers)).Msg("动作处理器注册完成")

	// 订阅数据主题
	log.Info().Msg("开始订阅数据流...")
	if err := s.subscribeToDataStream(); err != nil {
		log.Error().Err(err).Msg("订阅数据流失败")
		return fmt.Errorf("订阅数据流失败: %w", err)
	}
	log.Info().Msg("数据流订阅成功")

	// 启动规则监控
	log.Info().Msg("启动规则监控...")
	s.wg.Add(1)
	go s.watchRuleChanges()

	// 启动聚合状态清理器
	log.Info().Msg("启动聚合状态清理器...")
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
	if s.workerPool != nil {
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
	log.Debug().
		Str("subject", msg.Subject).
		Msg("规则引擎收到数据点消息")

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

	// 获取启用的规则
	rules := s.manager.GetEnabledRules()
	if len(rules) == 0 {
		log.Debug().Msg("没有启用的规则")
		return
	}

	log.Debug().Int("rules_count", len(rules)).Msg("开始评估规则")

	// 并行评估规则
	successCount := 0
	failCount := 0
	
	for _, rule := range rules {
		task := RuleTask{Rule: rule, Point: point}
		
		// 尝试提交到工作池进行并行处理
		if s.workerPool != nil && s.workerPool.SubmitTask(task) {
			successCount++
		} else {
			// 工作池满或不可用，回退到同步处理
			s.processRule(rule, point)
			failCount++
		}
	}
	
	if successCount > 0 {
		log.Debug().
			Int("parallel_tasks", successCount).
			Int("sync_tasks", failCount).
			Msg("规则任务分发完成")
	}
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
	
	// 记录规则执行统计
	if s.enableMetrics {
		s.monitor.RecordRuleExecution(rule.ID, duration, matched, err)
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
			s.monitor.RecordActionExecution(action.Type, actionDuration, err == nil, err)
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

// executeAction 执行动作
func (s *RuleEngineService) executeAction(action *Action, point model.Point, rule *Rule) error {
	handler, exists := s.actionHandlers[action.Type]
	if exists {
		// 使用新的动作处理器
		result, err := handler.Execute(context.Background(), point, rule, action.Config)
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
	switch action.Type {
	case "aggregate":
		return s.executeAggregateAction(action, point, rule)
	case "transform":
		return s.executeTransformAction(action, point, rule)
	case "filter":
		return s.executeFilterAction(action, point, rule)
	case "forward":
		return s.executeForwardAction(action, point, rule)
	case "alert":
		return s.executeAlertAction(action, point, rule)
	default:
		return fmt.Errorf("不支持的动作类型: %s", action.Type)
	}
}

// executeAggregateAction 执行聚合动作
func (s *RuleEngineService) executeAggregateAction(action *Action, point model.Point, rule *Rule) error {
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

	s.aggregateMutex.Lock()
	defer s.aggregateMutex.Unlock()

	// 获取或创建聚合状态
	state, exists := s.aggregateStates[stateKey]
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

	// 检查是否达到窗口大小
	if len(state.Buffer) >= windowSize {
		// 计算聚合结果
		result, err := s.calculateAggregateResult(state.Buffer, functions)
		if err != nil {
			return fmt.Errorf("计算聚合结果失败: %w", err)
		}

		// 创建结果数据点
		resultPoint := model.Point{
			DeviceID:  point.DeviceID,
			Key:       s.formatOutputKey(outputKey, point),
			Value:     result,
			Type:      model.TypeFloat,
			Timestamp: time.Now(),
			Tags:      point.Tags,
		}

		// 添加聚合标签
		if resultPoint.Tags == nil {
			resultPoint.Tags = make(map[string]string)
		}
		resultPoint.Tags["aggregated"] = "true"
		resultPoint.Tags["window_size"] = fmt.Sprintf("%d", windowSize)
		resultPoint.Tags["source_rule"] = rule.ID

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
		state.Buffer = state.Buffer[:0]
	}

	return nil
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
			if point.Tags != nil {
				if tagValue, exists := point.Tags[fieldStr]; exists {
					keyParts = append(keyParts, tagValue)
				}
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

// convertToFloat64 将值转换为float64
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
			"tags":      SafeValueForJSON(transformedPoint.Tags),
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
		"tags":      point.Tags,
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
		}
	}

	return nil
}

// watchRuleChanges 监控规则变化
func (s *RuleEngineService) watchRuleChanges() {
	defer s.wg.Done()

	changesChan, err := s.manager.WatchChanges()
	if err != nil {
		log.Error().Err(err).Msg("监控规则变化失败")
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
	s.aggregateMutex.Lock()
	defer s.aggregateMutex.Unlock()

	expireTime := time.Now().Add(-10 * time.Minute)
	cleanedCount := 0
	for key, state := range s.aggregateStates {
		if state.LastUpdate.Before(expireTime) {
			delete(s.aggregateStates, key)
			cleanedCount++
		}
	}
	
	if cleanedCount > 0 {
		log.Debug().
			Int("cleaned_count", cleanedCount).
			Int("remaining_count", len(s.aggregateStates)).
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

// BuiltinAlertHandler 内建告警处理器
type BuiltinAlertHandler struct {
	natsConn *nats.Conn
}

// Name 返回处理器名称
func (h *BuiltinAlertHandler) Name() string {
	return "BuiltinAlertHandler"
}

// Execute 执行告警动作
func (h *BuiltinAlertHandler) Execute(ctx context.Context, point model.Point, rule *Rule, config map[string]interface{}) (*ActionResult, error) {
	// 解析告警配置
	level, ok := config["level"].(string)
	if !ok {
		level = "info"
	}
	
	message, ok := config["message"].(string)
	if !ok {
		message = "触发告警"
	}
	
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
		Tags:      point.Tags,
	}
	
	// 发布到NATS
	if h.natsConn != nil {
		data, err := json.Marshal(alert)
		if err == nil {
			// 只发布到一个主题以避免重复
			subject := "iot.alerts.triggered"
			if err := h.natsConn.Publish(subject, data); err != nil {
				log.Error().Err(err).Str("subject", subject).Msg("发布告警到NATS失败")
			} else {
				log.Info().Str("alert_id", alert.ID).Str("subject", subject).Str("level", level).Msg("告警发布到NATS成功")
			}
		}
	}
	
	return &ActionResult{
		Type:     "alert",
		Success:  true,
		Duration: 0,
		Output:   "告警发送成功",
	}, nil
}

// generateAlertID 生成告警ID
func generateAlertID() string {
	return fmt.Sprintf("alert_%d", time.Now().UnixNano())
}


// publishRuleEvent 发布规则执行事件到NATS
func (s *RuleEngineService) publishRuleEvent(eventType string, rule *Rule, point model.Point, eventData map[string]interface{}) {
	if s.bus == nil {
		return
	}

	// 构建事件数据
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
			"tags":      SafeValueForJSON(point.Tags),
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
