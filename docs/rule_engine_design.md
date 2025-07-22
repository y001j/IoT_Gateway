# IoT Gateway 规则引擎设计文档

> **版本**: v1.0  
> **作者**: IoT Gateway Team  
> **日期**: 2025-06-30  
> **状态**: 设计阶段  
> **优先级**: ★★★★☆

## 📋 **概述**

规则引擎是IoT Gateway的第5个核心模块，位于数据处理流水线的中间层，负责对实时数据流进行过滤、转换、聚合和报警处理。它将显著增强系统的数据处理能力，使网关不仅仅是数据传输通道，更成为智能的边缘计算节点。

### **🎯 核心目标**
- **实时处理**: 对高频数据流进行实时规则评估
- **灵活配置**: 支持多种规则语法，满足不同复杂度需求
- **高性能**: 处理能力>10,000 rules/second
- **易扩展**: 支持自定义函数和动作插件
- **热更新**: 支持运行时规则动态更新

---

## 🏗️ **系统架构**

### **整体架构图**
```
┌─────────────────────────────────────────────────────────────────┐
│                        Rule Engine                              │
├─────────────────┬───────────────────┬───────────────────────────┤
│   Rule Manager  │  Expression Engine │     Action Executor      │
│                 │                   │                           │
│ • 规则CRUD      │ • JSON规则解析     │ • 内置动作                │
│ • 热更新        │ • 表达式评估       │ • 自定义动作              │
│ • 版本管理      │ • Lua脚本执行      │ • 异步执行                │
│ • 依赖检查      │ • 条件匹配         │ • 错误处理                │
└─────────────────┴───────────────────┴───────────────────────────┘
                              │
                    ┌─────────┼─────────┐
                    │         │         │
            ┌───────▼───┐ ┌───▼───┐ ┌───▼────┐
            │   NATS    │ │Config │ │Metrics │
            │Message Bus│ │ Mgmt  │ │Monitor │
            └───────────┘ └───────┘ └────────┘
```

### **数据流架构**
```
原始数据流:
Southbound Adapters → Plugin Manager → [原始数据通道]
                                           ↓
规则处理流:                              Rule Engine
                                           ↓
处理后数据流:                          [处理后数据通道] → Northbound Sinks
```

### **模块依赖关系**
- **依赖**: Core Runtime (NATS总线、配置系统、日志系统)
- **被依赖**: 无直接依赖，通过消息总线与其他模块通信
- **可选依赖**: Plugin Manager (自定义动作插件)

---

## 🔧 **核心组件设计**

### **1. Rule Manager - 规则管理器**

```go
type RuleManager struct {
    rules       map[string]*Rule
    ruleIndex   *RuleIndex
    watcher     *fsnotify.Watcher
    validator   *RuleValidator
    mu          sync.RWMutex
}

type Rule struct {
    ID          string                 `json:"id"`
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Enabled     bool                   `json:"enabled"`
    Priority    int                    `json:"priority"`
    Version     int                    `json:"version"`
    Conditions  *Condition             `json:"conditions"`
    Actions     []Action               `json:"actions"`
    Tags        map[string]string      `json:"tags,omitempty"`
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"`
}
```

**核心功能**:
- ✅ 规则的增删改查操作
- ✅ 规则文件监控和热更新
- ✅ 规则优先级管理和排序
- ✅ 规则版本控制和回滚
- ✅ 规则依赖关系检查
- ✅ 规则语法验证

### **2. Expression Engine - 表达式引擎**

```go
type ExpressionEngine struct {
    celEnv      *cel.Env
    luaState    *lua.LState
    functions   map[string]Function
    operators   map[string]Operator
}

type Condition struct {
    Type       string                 `json:"type"`        // "simple", "expression", "lua"
    Field      string                 `json:"field,omitempty"`
    Operator   string                 `json:"operator,omitempty"`
    Value      interface{}            `json:"value,omitempty"`
    Expression string                 `json:"expression,omitempty"`
    Script     string                 `json:"script,omitempty"`
    And        []*Condition           `json:"and,omitempty"`
    Or         []*Condition           `json:"or,omitempty"`
    Not        *Condition             `json:"not,omitempty"`
}
```

**支持的规则语法**:

**1. JSON配置规则 (基础版)**
```json
{
  "id": "temp_alert_001",
  "name": "温度报警规则",
  "enabled": true,
  "priority": 100,
  "conditions": {
    "and": [
      {"field": "device_id", "operator": "eq", "value": "sensor001"},
      {"field": "key", "operator": "eq", "value": "temperature"},
      {"field": "value", "operator": "gt", "value": 35.0}
    ]
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "设备{{device_id}}温度过高: {{value}}°C"
      }
    }
  ]
}
```

**2. CEL表达式规则 (高级版)**
```json
{
  "id": "complex_rule_001",
  "name": "复杂条件规则",
  "conditions": {
    "type": "expression",
    "expression": "device_id == 'sensor001' && key == 'temperature' && value > avg(device_id, 'temperature', '5m') + 2 * stddev(device_id, 'temperature', '5m')"
  }
}
```

**3. Lua脚本规则 (专业版)**
```json
{
  "id": "lua_rule_001",
  "name": "Lua脚本规则",
  "conditions": {
    "type": "lua",
    "script": "return point.device_id == 'sensor001' and point.value > get_threshold(point.device_id, point.key)"
  }
}
```

### **3. Action Executor - 动作执行器**

```go
type ActionExecutor struct {
    actions     map[string]ActionHandler
    bus         *nats.Conn
    asyncPool   *ants.Pool
}

type Action struct {
    Type     string                 `json:"type"`
    Config   map[string]interface{} `json:"config"`
    Async    bool                   `json:"async"`
    Timeout  time.Duration          `json:"timeout"`
    Retry    int                    `json:"retry"`
}

type ActionHandler interface {
    Name() string
    Execute(ctx context.Context, point model.Point, config map[string]interface{}) error
}
```

**内置动作类型**:

**1. Forward Action - 数据转发**
```json
{
  "type": "forward",
  "config": {
    "sink": "mqtt-alerts",
    "topic": "alerts/{{device_id}}/{{key}}",
    "transform": {
      "add_tags": {"rule_id": "temp_alert_001"},
      "set_priority": "high"
    }
  }
}
```

**2. Alert Action - 报警通知**
```json
{
  "type": "alert",
  "config": {
    "level": "warning",
    "message": "设备{{device_id}}的{{key}}值{{value}}超过阈值",
    "channels": ["email", "webhook"],
    "throttle": "5m"
  }
}
```

**3. Transform Action - 数据转换**
```json
{
  "type": "transform",
  "config": {
    "operations": [
      {"type": "scale", "factor": 0.1},
      {"type": "unit_convert", "from": "celsius", "to": "fahrenheit"},
      {"type": "add_tag", "key": "processed", "value": "true"}
    ]
  }
}
```

**4. Aggregate Action - 数据聚合**
```json
{
  "type": "aggregate",
  "config": {
    "window": "5m",
    "functions": ["avg", "max", "min"],
    "group_by": ["device_id", "location"],
    "output_topic": "aggregated/{{device_id}}"
  }
}
```

**5. Filter Action - 数据过滤**
```json
{
  "type": "filter",
  "config": {
    "action": "drop",
    "reason": "duplicate_data"
  }
}
```

### **4. Built-in Functions - 内置函数库**

```go
// 数学函数
func avg(deviceID, key string, window string) float64
func max(deviceID, key string, window string) float64
func min(deviceID, key string, window string) float64
func sum(deviceID, key string, window string) float64
func count(deviceID, key string, window string) int64
func stddev(deviceID, key string, window string) float64

// 时间函数
func now() time.Time
func ago(duration string) time.Time
func timeWindow(start, end time.Time) bool
func weekday() int
func hour() int

// 字符串函数
func contains(str, substr string) bool
func startsWith(str, prefix string) bool
func endsWith(str, suffix string) bool
func regex(str, pattern string) bool
func length(str string) int

// 设备状态函数
func deviceOnline(deviceID string) bool
func lastSeen(deviceID string) time.Time
func getTag(deviceID, tagKey string) string
func getThreshold(deviceID, key string) float64
```

---

## 📊 **性能优化设计**

### **1. 规则匹配优化**

```go
type RuleIndex struct {
    deviceIndex    map[string][]*Rule  // 按设备ID索引
    keyIndex       map[string][]*Rule  // 按数据key索引
    priorityIndex  []*Rule             // 按优先级排序
    typeIndex      map[string][]*Rule  // 按数据类型索引
}

// 快速规则匹配算法
func (idx *RuleIndex) Match(point model.Point) []*Rule {
    candidates := make([]*Rule, 0)
    
    // 1. 设备ID索引匹配
    if rules, exists := idx.deviceIndex[point.DeviceID]; exists {
        candidates = append(candidates, rules...)
    }
    
    // 2. 通用规则匹配
    if rules, exists := idx.deviceIndex["*"]; exists {
        candidates = append(candidates, rules...)
    }
    
    // 3. 按优先级排序
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].Priority > candidates[j].Priority
    })
    
    return candidates
}
```

### **2. 数据处理优化**

```go
type RuleEngine struct {
    batchSize     int
    workerPool    *ants.Pool
    dataBuffer    chan model.Point
    resultBuffer  chan ProcessedPoint
    cache         *cache.LRU
}

// 批量处理数据点
func (re *RuleEngine) ProcessBatch(points []model.Point) []ProcessedPoint {
    results := make([]ProcessedPoint, 0, len(points))
    
    // 并发处理批次数据
    var wg sync.WaitGroup
    resultChan := make(chan ProcessedPoint, len(points))
    
    for _, point := range points {
        wg.Add(1)
        re.workerPool.Submit(func() {
            defer wg.Done()
            result := re.ProcessPoint(point)
            resultChan <- result
        })
    }
    
    wg.Wait()
    close(resultChan)
    
    for result := range resultChan {
        results = append(results, result)
    }
    
    return results
}
```

### **3. 缓存策略**

```go
type CacheManager struct {
    ruleCache      *cache.LRU  // 编译后的规则缓存
    dataCache      *cache.LRU  // 历史数据缓存
    aggregateCache *cache.LRU  // 聚合结果缓存
    thresholdCache *cache.LRU  // 阈值配置缓存
}

// 时间窗口数据缓存
type TimeWindowCache struct {
    data     map[string]*CircularBuffer
    windows  map[string]time.Duration
    mu       sync.RWMutex
}

func (twc *TimeWindowCache) Add(deviceID, key string, value float64, timestamp time.Time) {
    twc.mu.Lock()
    defer twc.mu.Unlock()
    
    cacheKey := fmt.Sprintf("%s:%s", deviceID, key)
    if buffer, exists := twc.data[cacheKey]; exists {
        buffer.Add(value, timestamp)
    } else {
        buffer := NewCircularBuffer(1000) // 保持最近1000个数据点
        buffer.Add(value, timestamp)
        twc.data[cacheKey] = buffer
    }
}
```

---

## 🔌 **系统集成设计**

### **1. 与Core Runtime集成**

```go
// 规则引擎作为Core Runtime的服务
type RuleEngineService struct {
    engine     *RuleEngine
    manager    *RuleManager
    executor   *ActionExecutor
    bus        *nats.Conn
    config     *viper.Viper
}

func (res *RuleEngineService) Name() string {
    return "rule-engine"
}

func (res *RuleEngineService) Init(cfg any) error {
    // 初始化规则引擎组件
    res.engine = NewRuleEngine(res.config)
    res.manager = NewRuleManager(res.config.GetString("rules.dir"))
    res.executor = NewActionExecutor(res.bus)
    
    // 加载规则配置
    return res.manager.LoadRules()
}

func (res *RuleEngineService) Start(ctx context.Context) error {
    // 订阅原始数据
    _, err := res.bus.Subscribe("iot.data.raw", res.handleDataPoint)
    if err != nil {
        return fmt.Errorf("订阅数据失败: %w", err)
    }
    
    // 启动规则处理协程
    go res.engine.Run(ctx)
    
    return nil
}
```

### **2. 消息总线集成**

```go
// NATS主题设计
const (
    TopicRawData       = "iot.data.raw"        // 原始数据
    TopicProcessedData = "iot.data.processed"  // 处理后数据
    TopicAlerts        = "iot.alerts"          // 报警消息
    TopicRuleEvents    = "iot.rules.events"   // 规则事件
    TopicMetrics       = "iot.metrics"         // 指标数据
)

func (re *RuleEngine) handleDataPoint(msg *nats.Msg) {
    var point model.Point
    if err := json.Unmarshal(msg.Data, &point); err != nil {
        log.Error().Err(err).Msg("解析数据点失败")
        return
    }
    
    // 处理数据点
    processedPoints := re.ProcessPoint(point)
    
    // 发布处理结果
    for _, processed := range processedPoints {
        data, _ := json.Marshal(processed)
        re.bus.Publish(TopicProcessedData, data)
    }
}
```

### **3. REST API集成**

```go
// HTTP API路由
func (res *RuleEngineService) RegisterRoutes(router *gin.Engine) {
    api := router.Group("/api/v1/rules")
    {
        api.GET("", res.ListRules)           // 获取规则列表
        api.POST("", res.CreateRule)         // 创建规则
        api.GET("/:id", res.GetRule)         // 获取规则详情
        api.PUT("/:id", res.UpdateRule)      // 更新规则
        api.DELETE("/:id", res.DeleteRule)   // 删除规则
        api.POST("/:id/enable", res.EnableRule)   // 启用规则
        api.POST("/:id/disable", res.DisableRule) // 禁用规则
        api.POST("/:id/test", res.TestRule)       // 测试规则
    }
    
    metrics := router.Group("/api/v1/rules/metrics")
    {
        metrics.GET("", res.GetMetrics)      // 获取规则指标
        metrics.GET("/stats", res.GetStats)  // 获取统计信息
    }
}
```

---

## 📁 **文件结构设计**

```
internal/rules/
├── engine.go              # 规则引擎主逻辑
├── manager.go             # 规则管理器
├── parser.go              # 规则解析器
├── executor.go            # 动作执行器
├── index.go               # 规则索引
├── cache.go               # 缓存管理
├── service.go             # 服务集成
├── api/                   # REST API
│   ├── handlers.go        # HTTP处理器
│   ├── middleware.go      # 中间件
│   └── dto.go            # 数据传输对象
├── actions/               # 动作实现
│   ├── alert.go          # 报警动作
│   ├── forward.go        # 转发动作
│   ├── transform.go      # 转换动作
│   ├── aggregate.go      # 聚合动作
│   ├── filter.go         # 过滤动作
│   └── custom.go         # 自定义动作
├── functions/             # 内置函数
│   ├── math.go           # 数学函数
│   ├── string.go         # 字符串函数
│   ├── time.go           # 时间函数
│   └── device.go         # 设备函数
├── expressions/           # 表达式引擎
│   ├── cel.go            # CEL表达式
│   ├── lua.go            # Lua脚本
│   └── json.go           # JSON规则
└── test/                  # 测试文件
    ├── engine_test.go
    ├── manager_test.go
    └── integration_test.go
```

---

## ⚙️ **配置设计**

### **主配置文件**
```yaml
# config.yaml
rules:
  enabled: true
  rules_dir: "./rules"
  api_enabled: true
  api_port: 8082
  
  # 性能配置
  batch_size: 100
  worker_pool_size: 10
  cache_size: 10000
  
  # 表达式引擎配置
  engines:
    cel:
      enabled: true
      max_cost: 1000000
    lua:
      enabled: true
      max_memory: "10MB"
      timeout: "1s"
  
  # 动作配置
  actions:
    alert:
      channels:
        email:
          smtp_server: "smtp.example.com"
          smtp_port: 587
          username: "alerts@example.com"
          password: "password"
        webhook:
          url: "https://hooks.example.com/alerts"
          timeout: "5s"
```

### **规则配置文件**
```yaml
# rules/temperature_rules.yaml
rules:
  - id: "temp_high_alert"
    name: "温度过高报警"
    description: "当温度超过35度时发送报警"
    enabled: true
    priority: 100
    conditions:
      and:
        - field: "key"
          operator: "eq"
          value: "temperature"
        - field: "value"
          operator: "gt"
          value: 35.0
    actions:
      - type: "alert"
        config:
          level: "warning"
          message: "设备{{device_id}}温度过高: {{value}}°C"
          throttle: "5m"
      - type: "forward"
        config:
          sink: "mqtt-alerts"
          topic: "alerts/temperature/{{device_id}}"
    tags:
      category: "temperature"
      severity: "high"

  - id: "temp_trend_analysis"
    name: "温度趋势分析"
    description: "分析温度变化趋势"
    enabled: true
    priority: 50
    conditions:
      type: "expression"
      expression: "key == 'temperature' && abs(value - avg(device_id, 'temperature', '10m')) > 2 * stddev(device_id, 'temperature', '10m')"
    actions:
      - type: "transform"
        config:
          add_tags:
            trend: "anomaly"
            analysis_time: "{{now()}}"
      - type: "forward"
        config:
          sink: "influxdb-analytics"
```

---

## 🧪 **测试策略**

### **1. 单元测试**
```go
func TestRuleEngine_ProcessPoint(t *testing.T) {
    engine := NewRuleEngine(testConfig)
    
    // 添加测试规则
    rule := &Rule{
        ID: "test_rule",
        Conditions: &Condition{
            Field: "value",
            Operator: "gt",
            Value: 30.0,
        },
        Actions: []Action{
            {Type: "alert", Config: map[string]interface{}{"level": "warning"}},
        },
    }
    engine.AddRule(rule)
    
    // 测试数据点
    point := model.Point{
        DeviceID: "test_device",
        Key: "temperature",
        Value: 35.0,
        Timestamp: time.Now(),
    }
    
    results := engine.ProcessPoint(point)
    assert.Len(t, results, 1)
    assert.Equal(t, "alert", results[0].Action.Type)
}
```

### **2. 集成测试**
```go
func TestRuleEngine_Integration(t *testing.T) {
    // 启动测试环境
    runtime := setupTestRuntime(t)
    defer runtime.Stop(context.Background())
    
    // 创建规则引擎服务
    ruleService := NewRuleEngineService(runtime.Bus, runtime.Config)
    runtime.RegisterService(ruleService)
    
    ctx := context.Background()
    require.NoError(t, runtime.Start(ctx))
    
    // 发送测试数据
    testPoint := model.Point{
        DeviceID: "sensor001",
        Key: "temperature",
        Value: 40.0,
        Timestamp: time.Now(),
    }
    
    data, _ := json.Marshal(testPoint)
    runtime.Bus.Publish("iot.data.raw", data)
    
    // 验证处理结果
    sub, _ := runtime.Bus.Subscribe("iot.alerts", func(msg *nats.Msg) {
        var alert Alert
        json.Unmarshal(msg.Data, &alert)
        assert.Equal(t, "warning", alert.Level)
    })
    defer sub.Unsubscribe()
    
    time.Sleep(100 * time.Millisecond)
}
```

### **3. 性能测试**
```go
func BenchmarkRuleEngine_ProcessPoint(b *testing.B) {
    engine := NewRuleEngine(testConfig)
    
    // 添加100个规则
    for i := 0; i < 100; i++ {
        rule := createTestRule(fmt.Sprintf("rule_%d", i))
        engine.AddRule(rule)
    }
    
    point := model.Point{
        DeviceID: "test_device",
        Key: "temperature",
        Value: 25.0,
        Timestamp: time.Now(),
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        engine.ProcessPoint(point)
    }
}
```

---

## 📈 **监控和指标**

### **关键指标**
```go
// Prometheus指标定义
var (
    rulesTotal = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "iot_rules_total",
            Help: "Total number of rules",
        },
        []string{"status"}, // enabled, disabled
    )
    
    ruleExecutionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "iot_rule_execution_duration_seconds",
            Help: "Rule execution duration",
            Buckets: prometheus.DefBuckets,
        },
        []string{"rule_id", "action_type"},
    )
    
    ruleMatchesTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "iot_rule_matches_total",
            Help: "Total number of rule matches",
        },
        []string{"rule_id", "device_id"},
    )
    
    actionExecutionsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "iot_action_executions_total",
            Help: "Total number of action executions",
        },
        []string{"action_type", "status"}, // success, error
    )
)
```

### **监控面板**
- 规则执行统计（成功率、错误率、执行时间）
- 规则匹配统计（匹配次数、热点规则）
- 动作执行统计（各类动作的执行情况）
- 系统性能指标（内存使用、CPU使用、处理延迟）

---

## 🚀 **实施计划**

### **Phase 1: 基础框架 (2周)**
- ✅ 规则引擎核心架构
- ✅ 基础规则管理器
- ✅ JSON规则解析器
- ✅ 简单动作执行器
- ✅ 与Core Runtime集成

### **Phase 2: 核心功能 (3周)**
- ✅ 完整的动作系统（alert, forward, transform, filter）
- ✅ 规则索引和优化
- ✅ 批量处理和并发
- ✅ 基础内置函数库
- ✅ 单元测试覆盖

### **Phase 3: 高级特性 (3周)**
- ✅ CEL表达式引擎集成
- ✅ 时间窗口和聚合功能
- ✅ 缓存系统
- ✅ REST API
- ✅ 配置热更新

### **Phase 4: 扩展功能 (2周)**
- ✅ Lua脚本支持
- ✅ 自定义动作插件
- ✅ 高级内置函数
- ✅ 性能优化
- ✅ 集成测试

### **Phase 5: 生产就绪 (2周)**
- ✅ 监控指标完善
- ✅ 错误处理和恢复
- ✅ 文档和示例
- ✅ 性能测试和调优
- ✅ 部署和运维工具

---

## 💡 **最佳实践**

### **规则设计原则**
1. **简单优先**: 优先使用JSON配置，复杂逻辑才用表达式
2. **性能考虑**: 避免复杂的嵌套条件，使用索引友好的字段
3. **可维护性**: 规则命名规范，添加详细描述和标签
4. **测试驱动**: 每个规则都应该有对应的测试用例

### **开发建议**
1. **渐进式开发**: 先实现基础功能，再添加高级特性
2. **性能优先**: 关注规则匹配和执行的性能
3. **扩展性设计**: 为自定义函数和动作预留接口
4. **错误处理**: 完善的错误处理和降级机制

### **运维建议**
1. **监控告警**: 关键指标的监控和告警
2. **规则审计**: 规则变更的审计日志
3. **性能调优**: 定期的性能分析和优化
4. **容量规划**: 根据业务增长规划系统容量

---

## 🔗 **相关文档**

- [Core Runtime 设计文档](./core_runtime.md)
- [Plugin Manager 设计文档](./plugin_manager.md)
- [已完成模块文档](./completed_modules.md)
- [API 参考文档](./api_reference.md)

---

**🎯 规则引擎将显著增强IoT Gateway的数据处理能力，使其成为真正的智能边缘计算平台！** 