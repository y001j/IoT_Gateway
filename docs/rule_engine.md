# IoT Gateway 规则引擎

## 概述

IoT Gateway规则引擎是一个强大的事件驱动数据处理系统，作为IoT Gateway的第五个核心模块，位于数据处理流水线的中间层。它提供了灵活的规则配置、实时数据处理和多样化的动作执行能力。

## 系统架构

### 整体架构

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│  Southbound     │    │   Plugin         │    │  Rule Engine    │
│  Adapters       │───▶│   Manager        │───▶│                 │
│                 │    │                  │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                                        │
                                                        ▼
┌─────────────────┐                            ┌─────────────────┐
│  Northbound     │◀───────────────────────────│   NATS          │
│  Sinks          │                            │   Message Bus   │
└─────────────────┘                            └─────────────────┘
```

### 数据流

1. **数据输入**: 通过NATS订阅`iot.processed.*`主题接收Plugin Manager处理后的IoT数据
2. **规则匹配**: 使用多维索引快速匹配适用的规则
3. **条件评估**: 评估规则条件是否满足
4. **动作执行**: 执行匹配规则的动作序列
5. **结果输出**: 通过NATS发布处理结果到`iot.rules.*`、`iot.alerts.*`等主题

### 核心组件

- **规则管理器 (Manager)**: 规则的加载、保存、验证和热更新
- **规则索引 (Index)**: 多维索引系统，提供快速规则匹配
- **条件评估器 (Evaluator)**: 条件逻辑评估和表达式计算
- **动作处理器 (Actions)**: 五大动作类型的执行器
- **数据类型 (Types)**: 完整的数据结构定义

## 功能特性

### 🎯 核心功能

- **事件驱动架构**: 基于NATS消息总线的实时处理，通过发布/订阅模式与其他模块通信
- **JSON规则配置**: 完全基于JSON的规则定义
- **热更新支持**: 运行时动态加载和更新规则
- **多维索引**: 按设备ID、数据key、优先级等维度建立索引
- **条件评估**: 支持简单条件、表达式和Lua脚本
- **动作执行**: 五大动作类型，满足各种处理需求
- **NATS集成**: 无缝集成NATS消息总线，支持消息持久化和集群部署

### 🚀 性能特性

- **高性能匹配**: 多维索引避免全量扫描
- **并发处理**: 支持并发规则执行
- **异步动作**: 支持异步动作执行
- **内存优化**: 环形缓冲区和缓存机制
- **批量处理**: 支持批量数据处理

### 🔧 扩展特性

- **插件化动作**: 易于扩展新的动作类型
- **模板系统**: Go模板语法支持
- **错误恢复**: 完善的错误处理和重试机制
- **监控指标**: 详细的执行统计和性能指标

## 规则定义

### 基本结构

```json
{
  "id": "rule_unique_id",
  "name": "规则名称",
  "description": "规则描述",
  "enabled": true,
  "priority": 1,
  "version": 1,
  "conditions": {
    // 条件定义
  },
  "actions": [
    // 动作列表
  ],
  "tags": {
    "category": "temperature",
    "environment": "production"
  },
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

### 字段说明

- **id**: 规则唯一标识符
- **name**: 规则名称
- **description**: 规则描述
- **enabled**: 是否启用（默认true）
- **priority**: 优先级（数字越小优先级越高）
- **version**: 版本号（自动管理）
- **conditions**: 触发条件
- **actions**: 执行动作列表
- **tags**: 规则标签（可选）
- **created_at/updated_at**: 时间戳（自动管理）

## 条件系统

### 条件类型

#### 1. 简单条件 (Simple)

```json
{
  "type": "simple",
  "field": "temperature",
  "operator": "gt",
  "value": 30.0
}
```

**支持的操作符**:
- `eq`: 等于
- `ne`: 不等于
- `gt`: 大于
- `gte`: 大于等于
- `lt`: 小于
- `lte`: 小于等于
- `contains`: 包含
- `startswith`: 开始于
- `endswith`: 结束于

#### 2. 逻辑条件

**AND条件**:
```json
{
  "type": "and",
  "conditions": [
    {
      "type": "simple",
      "field": "temperature",
      "operator": "gt",
      "value": 30
    },
    {
      "type": "simple",
      "field": "humidity",
      "operator": "lt",
      "value": 60
    }
  ]
}
```

**OR条件**:
```json
{
  "type": "or",
  "conditions": [
    {
      "type": "simple",
      "field": "status",
      "operator": "eq",
      "value": "error"
    },
    {
      "type": "simple",
      "field": "status",
      "operator": "eq",
      "value": "warning"
    }
  ]
}
```

**NOT条件**:
```json
{
  "type": "not",
  "condition": {
    "type": "simple",
    "field": "status",
    "operator": "eq",
    "value": "normal"
  }
}
```

#### 3. 表达式条件

```json
{
  "type": "expression",
  "expression": "temperature > 30 && humidity < 60"
}
```

#### 4. Lua脚本条件

```json
{
  "type": "lua",
  "script": "return point.temperature > 30 and point.humidity < 60"
}
```

### 字段引用

支持嵌套字段访问：
- `device_id`: 设备ID
- `key`: 数据键名
- `value`: 数据值
- `type`: 数据类型
- `timestamp`: 时间戳
- `tags.location`: 标签中的location字段

## 动作系统

规则引擎提供五大动作类型，每种动作都有丰富的配置选项。

### 1. Alert 动作 - 报警通知

发送多渠道报警通知，支持节流机制防止报警风暴。

```json
{
  "type": "alert",
  "config": {
    "level": "warning",
    "message": "温度异常: {{.DeviceID}} 当前温度 {{.Value}}°C",
    "channels": [
      {
        "type": "console",
        "enabled": true
      },
      {
        "type": "webhook",
        "enabled": true,
        "config": {
          "url": "https://hooks.slack.com/services/YOUR/WEBHOOK/URL",
          "method": "POST",
          "headers": {
            "Content-Type": "application/json"
          }
        }
      }
    ],
    "throttle": "5m",
    "tags": {
      "severity": "medium"
    }
  }
}
```

**支持的渠道**:
- `console`: 控制台输出
- `webhook`: HTTP Webhook
- `email`: 邮件通知（预留）
- `sms`: 短信通知（预留）

### 2. Transform 动作 - 数据转换

对数据进行各种转换操作，包括数值计算、单位转换、格式化等。

```json
{
  "type": "transform",
  "config": {
    "field": "temperature",
    "transforms": [
      {
        "type": "scale",
        "factor": 1.8
      },
      {
        "type": "offset",
        "value": 32
      },
      {
        "type": "unit_convert",
        "from": "celsius",
        "to": "fahrenheit"
      }
    ],
    "output_field": "temperature_f",
    "error_handling": "default",
    "default_value": 0
  }
}
```

**转换类型**:
- `scale`: 数值缩放
- `offset`: 数值偏移
- `unit_convert`: 单位转换
- `format`: 格式化
- `expression`: 表达式计算
- `lookup`: 查找表映射
- `round`: 四舍五入
- `clamp`: 数值限幅
- `map`: 值映射

### 3. Filter 动作 - 数据过滤

过滤数据，只有满足条件的数据才会继续传递。

```json
{
  "type": "filter",
  "config": {
    "filters": [
      {
        "type": "range",
        "field": "temperature",
        "min": -50,
        "max": 100
      },
      {
        "type": "duplicate",
        "field": "value",
        "window": "1m"
      },
      {
        "type": "rate_limit",
        "max_rate": 10,
        "window": "1s"
      }
    ],
    "action": "drop"
  }
}
```

**过滤类型**:
- `duplicate`: 重复数据过滤
- `range`: 范围过滤
- `rate_limit`: 速率限制
- `pattern`: 模式匹配
- `null`: 空值过滤
- `threshold`: 阈值过滤
- `time_window`: 时间窗口过滤

### 4. Aggregate 动作 - 数据聚合

对时间序列数据进行聚合计算。

```json
{
  "type": "aggregate",
  "config": {
    "window": {
      "type": "time",
      "size": "5m"
    },
    "functions": ["avg", "max", "min", "count"],
    "group_by": ["device_id"],
    "trigger": {
      "type": "time",
      "interval": "1m"
    },
    "output_subject": "aggregated.{{.device_id}}"
  }
}
```

**聚合函数**:
- `count`: 计数
- `sum`: 求和
- `avg`: 平均值
- `min`: 最小值
- `max`: 最大值
- `median`: 中位数
- `stddev`: 标准差
- `range`: 范围
- `first`: 第一个值
- `last`: 最后一个值

### 5. Forward 动作 - 数据转发

将数据转发到各种目标系统。

```json
{
  "type": "forward",
  "config": {
    "targets": [
      {
        "name": "api_server",
        "type": "http",
        "enabled": true,
        "async": false,
        "timeout": "10s",
        "retry": 3,
        "config": {
          "url": "https://api.example.com/data",
          "method": "POST",
          "content_type": "application/json",
          "auth": {
            "type": "bearer",
            "token": "your-token"
          }
        }
      }
    ],
    "add_rule_info": true,
    "data_transform": {
      "fields": ["device_id", "key", "value", "timestamp"],
      "constants": {
        "source": "iot-gateway"
      }
    }
  }
}
```

**转发目标**:
- `http`: HTTP API
- `file`: 本地文件
- `mqtt`: MQTT Broker
- `nats`: NATS消息系统

## 规则管理

### 规则文件格式

规则引擎支持JSON和YAML两种格式：

**JSON格式** (`rules.json`):
```json
[
  {
    "id": "temperature_alert",
    "name": "温度报警",
    "enabled": true,
    "conditions": {
      "type": "simple",
      "field": "temperature",
      "operator": "gt",
      "value": 35
    },
    "actions": [
      {
        "type": "alert",
        "config": {
          "level": "warning",
          "message": "温度过高: {{.Value}}°C"
        }
      }
    ]
  }
]
```

**YAML格式** (`rules.yaml`):
```yaml
- id: temperature_alert
  name: 温度报警
  enabled: true
  conditions:
    type: simple
    field: temperature
    operator: gt
    value: 35
  actions:
    - type: alert
      config:
        level: warning
        message: "温度过高: {{.Value}}°C"
```

### 热更新

规则引擎支持运行时热更新：

1. **文件监控**: 自动监控规则文件变化
2. **增量更新**: 只更新变化的规则
3. **版本管理**: 自动管理规则版本
4. **回滚支持**: 支持规则回滚
5. **验证检查**: 更新前进行规则验证

### 规则验证

规则加载时会进行全面验证：

- **结构验证**: JSON/YAML结构正确性
- **字段验证**: 必填字段完整性
- **条件验证**: 条件逻辑正确性
- **动作验证**: 动作配置有效性
- **引用验证**: 字段引用有效性

## 使用指南

### 快速开始

#### 1. 创建第一个规则

创建文件 `rules/temperature_monitor.json`:

```json
[
  {
    "id": "temp_high_alert",
    "name": "高温报警",
    "description": "当温度超过30度时发送报警",
    "enabled": true,
    "priority": 1,
    "conditions": {
      "type": "simple",
      "field": "temperature",
      "operator": "gt",
      "value": 30
    },
    "actions": [
      {
        "type": "alert",
        "config": {
          "level": "warning",
          "message": "设备 {{.DeviceID}} 温度过高: {{.Value}}°C",
          "channels": [
            {
              "type": "console",
              "enabled": true
            }
          ]
        }
      }
    ]
  }
]
```

#### 2. 启动规则引擎

```go
package main

import (
    "context"
    "log"
    
    "github.com/y001j/iot-gateway/internal/rules"
    "github.com/nats-io/nats.go"
)

func main() {
    // 连接NATS
    nc, err := nats.Connect("nats://localhost:4222")
    if err != nil {
        log.Fatal(err)
    }
    defer nc.Close()
    
    // 创建规则管理器
    manager := rules.NewManager("rules/")
    
    // 加载规则
    if err := manager.LoadRules(); err != nil {
        log.Fatal(err)
    }
    
    // 启动规则引擎
    ctx := context.Background()
    if err := manager.Start(ctx); err != nil {
        log.Fatal(err)
    }
    
    log.Println("规则引擎启动成功")
    
    // 等待退出信号
    select {}
}
```

### 常用规则模式

#### 1. 阈值监控

```json
{
  "id": "threshold_monitor",
  "name": "阈值监控",
  "conditions": {
    "type": "or",
    "conditions": [
      {
        "type": "simple",
        "field": "value",
        "operator": "gt",
        "value": 100
      },
      {
        "type": "simple",
        "field": "value",
        "operator": "lt",
        "value": 0
      }
    ]
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "level": "error",
        "message": "数值异常: {{.Value}}"
      }
    }
  ]
}
```

#### 2. 数据转换流水线

```json
{
  "id": "data_pipeline",
  "name": "数据处理流水线",
  "conditions": {
    "type": "simple",
    "field": "type",
    "operator": "eq",
    "value": "sensor_data"
  },
  "actions": [
    {
      "type": "transform",
      "config": {
        "field": "value",
        "transforms": [
          {
            "type": "scale",
            "factor": 0.01
          },
          {
            "type": "round",
            "precision": 2
          }
        ]
      }
    },
    {
      "type": "filter",
      "config": {
        "filters": [
          {
            "type": "range",
            "field": "value",
            "min": 0,
            "max": 100
          }
        ]
      }
    },
    {
      "type": "forward",
      "config": {
        "targets": [
          {
            "name": "database",
            "type": "http",
            "config": {
              "url": "http://localhost:8080/api/data",
              "method": "POST"
            }
          }
        ]
      }
    }
  ]
}
```

#### 3. 实时聚合

```json
{
  "id": "realtime_aggregation",
  "name": "实时数据聚合",
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "temperature"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window": {
          "type": "time",
          "size": "1m"
        },
        "functions": ["avg", "max", "min"],
        "group_by": ["device_id"],
        "trigger": {
          "type": "time",
          "interval": "30s"
        }
      }
    }
  ]
}
```

### 最佳实践

#### 1. 规则设计原则

- **单一职责**: 每个规则只处理一种业务逻辑
- **优先级设置**: 重要规则设置较高优先级
- **条件简化**: 避免过于复杂的条件逻辑
- **动作链**: 合理组织动作执行顺序
- **错误处理**: 为关键动作配置错误处理

#### 2. 性能优化

- **索引利用**: 充分利用设备ID和key索引
- **条件优化**: 将计算成本低的条件放在前面
- **异步动作**: 对非关键动作使用异步执行
- **缓存策略**: 合理设置缓存和缓冲区大小
- **批量处理**: 对高频数据使用批量处理

#### 3. 监控和调试

- **日志级别**: 合理设置日志级别
- **指标监控**: 监控规则执行统计
- **错误跟踪**: 跟踪规则执行错误
- **性能分析**: 分析规则执行性能
- **测试验证**: 充分测试规则逻辑

## API接口

### 规则管理API

```go
// 加载规则
func (m *Manager) LoadRules() error

// 保存规则
func (m *Manager) SaveRule(rule *Rule) error

// 删除规则
func (m *Manager) DeleteRule(id string) error

// 获取规则
func (m *Manager) GetRule(id string) (*Rule, error)

// 列出所有规则
func (m *Manager) ListRules() []*Rule

// 启用/禁用规则
func (m *Manager) EnableRule(id string, enabled bool) error

// 验证规则
func (m *Manager) ValidateRule(rule *Rule) error

// 获取统计信息
func (m *Manager) GetStats() ManagerStats
```

### 条件评估API

```go
// 评估条件
func (e *Evaluator) Evaluate(condition *Condition, point model.Point) (bool, error)

// 评估简单条件
func (e *Evaluator) evaluateSimple(condition *Condition, point model.Point) (bool, error)

// 评估表达式
func (e *Evaluator) evaluateExpression(expression string, point model.Point) (bool, error)

// 获取字段值
func (e *Evaluator) getFieldValue(field string, point model.Point) (interface{}, error)
```

## 配置参数

### 规则引擎配置

```yaml
rule_engine:
  # 规则文件目录
  rules_dir: "./rules"
  
  # 文件监控
  watch_files: true
  watch_interval: "1s"
  
  # 性能参数
  max_concurrent_rules: 100
  action_timeout: "30s"
  evaluation_timeout: "5s"
  
  # 缓存配置
  cache_size: 1000
  buffer_size: 10000
  
  # 日志配置
  log_level: "info"
  log_rule_execution: true
  
  # 指标配置
  metrics_enabled: true
  metrics_interval: "10s"
```

### NATS配置

```yaml
nats:
  # 输入主题
  input_subject: "iot.processed.*"
  
  # 输出主题
  output_subject: "iot.rules.*"
  
  # 错误主题
  error_subject: "iot.errors"
  
  # 队列组
  queue_group: "rule_engine"
  
  # 连接参数
  servers: ["nats://localhost:4222"]
  max_reconnect: -1
  reconnect_wait: "2s"
```

## 故障排除

### 常见问题

#### 1. 规则不生效

**可能原因**:
- 规则未启用 (`enabled: false`)
- 条件不匹配
- 优先级设置问题
- 索引未正确建立

**解决方法**:
```bash
# 检查规则状态
curl http://localhost:8080/api/rules

# 查看规则执行日志
tail -f logs/rule_engine.log

# 验证规则条件
curl -X POST http://localhost:8080/api/rules/validate
```

#### 2. 动作执行失败

**可能原因**:
- 目标系统不可达
- 认证信息错误
- 配置参数错误
- 超时设置过短

**解决方法**:
- 检查网络连通性
- 验证认证信息
- 调整超时和重试参数
- 查看详细错误日志

#### 3. 性能问题

**可能原因**:
- 规则数量过多
- 条件计算复杂
- 动作执行耗时
- 内存不足

**解决方法**:
- 优化规则条件
- 使用异步动作
- 增加系统资源
- 调整缓存大小

### 调试技巧

#### 1. 启用详细日志

```yaml
log_level: "debug"
log_rule_execution: true
```

#### 2. 使用测试工具

```bash
# 发送测试数据
nats pub iot.processed.test '{"device_id":"test","key":"temperature","value":35}'

# 监控输出
nats sub "iot.rules.>"
```

#### 3. 性能分析

```go
import _ "net/http/pprof"

go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

## 扩展开发

### 自定义动作处理器

```go
package actions

import (
    "context"
    "time"
    
    "github.com/y001j/iot-gateway/internal/model"
    "github.com/y001j/iot-gateway/internal/rules"
)

// CustomHandler 自定义动作处理器
type CustomHandler struct {
    // 处理器配置
}

// NewCustomHandler 创建自定义处理器
func NewCustomHandler() *CustomHandler {
    return &CustomHandler{}
}

// Name 返回处理器名称
func (h *CustomHandler) Name() string {
    return "custom"
}

// Execute 执行自定义动作
func (h *CustomHandler) Execute(ctx context.Context, point model.Point, rule *rules.Rule, config map[string]interface{}) (*rules.ActionResult, error) {
    start := time.Now()
    
    // 自定义处理逻辑
    
    return &rules.ActionResult{
        Type:     "custom",
        Success:  true,
        Duration: time.Since(start),
        Output:   "处理结果",
    }, nil
}
```

### 注册自定义处理器

```go
// 在规则引擎初始化时注册
engine.RegisterActionHandler("custom", NewCustomHandler())
```

## 监控指标

### 系统指标

- `rules_total`: 规则总数
- `rules_enabled`: 启用规则数
- `points_processed`: 处理数据点数
- `rules_matched`: 匹配规则数
- `actions_executed`: 执行动作数
- `actions_succeeded`: 成功动作数
- `actions_failed`: 失败动作数
- `processing_duration`: 处理耗时

### 动作指标

- `alert_sent`: 发送报警数
- `transform_executed`: 执行转换数
- `filter_dropped`: 过滤丢弃数
- `aggregate_computed`: 聚合计算数
- `forward_sent`: 转发发送数

## 版本历史

### v1.0.0 (当前版本)

**新功能**:
- ✅ 完整的规则引擎架构
- ✅ 五大动作处理器
- ✅ 多维索引系统
- ✅ 条件评估引擎
- ✅ 热更新支持
- ✅ NATS消息总线集成

**技术特性**:
- ✅ 事件驱动架构
- ✅ JSON/YAML规则配置
- ✅ 高性能数据处理
- ✅ 完善的错误处理
- ✅ 详细的文档和示例

## 许可证

本项目采用 MIT 许可证。详见 [LICENSE](../LICENSE) 文件。

## 贡献指南

欢迎贡献代码！请查看 [CONTRIBUTING.md](../CONTRIBUTING.md) 了解详细信息。

## 支持

如有问题或建议，请：

1. 查看文档和FAQ
2. 搜索已有的Issues
3. 创建新的Issue
4. 联系维护团队 