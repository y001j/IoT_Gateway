# IoT Gateway 规则引擎

[![Go Version](https://img.shields.io/badge/Go-1.24.3+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![NATS](https://img.shields.io/badge/NATS-Messaging-brightgreen)](https://nats.io/)

## 概述

IoT Gateway规则引擎是一个强大的事件驱动数据处理系统，提供灵活的规则配置、实时数据处理和多样化的动作执行能力。基于NATS消息总线，支持高性能并发处理和智能错误管理。

## 🚀 核心特性

### 📋 规则管理
- **JSON/YAML配置**: 完全基于配置文件的规则定义
- **热更新**: 运行时动态加载和更新规则
- **版本管理**: 自动规则版本控制和变更追踪
- **规则验证**: 完整的规则语法和逻辑验证
- **规则执行事件**: 自动发布规则执行状态到`iot.rules.*`主题

### 🎯 条件系统
- **简单条件**: 字段比较、逻辑操作符（eq, ne, gt, gte, lt, lte, contains, startswith, endswith, regex）
- **复合条件**: AND、OR、NOT逻辑组合，支持嵌套
- **表达式引擎**: 增强的数学表达式支持，包含递归下降解析器
- **内置函数**: 支持数学函数（abs, max, min, sqrt, pow等）、字符串函数、时间函数
- **正则表达式**: 带缓存的高性能正则匹配

### ⚡ 动作执行
- **Alert**: 多渠道报警通知（控制台、Webhook、邮件、短信、NATS发布）
- **Transform**: 数据转换（缩放、偏移、单位转换、格式化、表达式计算、查找表）
- **Filter**: 数据过滤（重复数据检测、范围过滤、速率限制、模式匹配）
- **Aggregate**: 数据聚合（统计函数、时间窗口、分组聚合、环形缓冲区）
- **Forward**: 数据转发（简化版，专注NATS转发，支持主题动态配置）

### 🔧 技术特性
- **事件驱动**: 基于NATS消息总线的实时处理
- **高性能**: 正则表达式缓存、字符串操作优化、并发处理
- **可扩展**: 插件化动作处理器，易于扩展
- **监控**: 详细的执行统计、性能指标和错误追踪
- **错误处理**: 分层错误管理系统，支持重试和错误分类

## 📁 项目结构

```
internal/rules/
├── types.go           # 数据类型定义
├── service.go         # 规则引擎服务
├── evaluator.go       # 条件评估器（增强的表达式支持）
├── expression.go      # 表达式引擎（Go AST + 自定义函数）
├── regex_cache.go     # 正则表达式缓存机制
├── errors.go          # 错误类型和错误处理
├── monitoring.go      # 监控和指标收集
└── actions/           # 动作处理器
    ├── alert.go       # 报警动作（增强通道支持）
    ├── transform.go   # 转换动作（增强表达式引擎）
    ├── filter.go      # 过滤动作
    ├── aggregate.go   # 聚合动作
    └── forward.go     # 转发动作（简化版）

examples/rules/
├── complete_examples.json    # 完整示例集合
└── forward_examples.json     # 转发动作示例

docs/
├── rule_engine.md           # 完整文档
├── forward_action.md        # Forward动作文档
└── quick_start.md          # 快速入门指南
```

## 🚀 快速开始

### 1. 启动服务

```bash
# 构建网关
go build -o bin/gateway cmd/gateway/main.go

# 启动网关（内置NATS服务器）
./bin/gateway -config config.yaml
```

### 2. 创建第一个规则

创建 `rules/temperature_alert.json`:

```json
[
  {
    "id": "temp_alert",
    "name": "温度报警",
    "enabled": true,
    "conditions": {
      "type": "and",
      "and": [
        {
          "type": "simple",
          "field": "key",
          "operator": "eq",
          "value": "temperature"
        },
        {
          "type": "simple",
          "field": "value",
          "operator": "gt",
          "value": 30
        }
      ]
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
            },
            {
              "type": "nats",
              "enabled": true,
              "config": {
                "subject": "iot.alerts.temperature"
              }
            }
          ]
        }
      }
    ]
  }
]
```

### 3. 表达式条件示例

```json
{
  "id": "complex_condition",
  "name": "复杂表达式条件",
  "enabled": true,
  "conditions": {
    "type": "expression",
    "expression": "value > 30 && contains(device_id, \"sensor\") && hour >= 9 && hour <= 17"
  },
  "actions": [
    {
      "type": "transform",
      "config": {
        "type": "expression",
        "parameters": {
          "expression": "(x - 32) * 5 / 9"
        },
        "output_key": "celsius_temp",
        "publish_subject": "iot.data.converted"
      }
    }
  ]
}
```

### 4. 测试规则

```bash
# 发送测试数据
nats pub iot.data.sensor001 '{
  "device_id": "sensor001",
  "key": "temperature",
  "value": 35.5,
  "type": "float",
  "timestamp": "2024-01-01T12:00:00Z"
}'

# 监控规则执行事件
nats sub "iot.rules.*"

# 监控告警
nats sub "iot.alerts.*"
```

## 📖 增强功能详解

### 🧮 表达式引擎

支持复杂的数学表达式和内置函数：

```json
{
  "type": "expression",
  "expression": "abs(value - 25) > 5 && len(device_id) > 5"
}
```

**支持的函数**：
- **数学函数**: `abs()`, `max()`, `min()`, `sqrt()`, `pow()`, `floor()`, `ceil()`
- **字符串函数**: `len()`, `upper()`, `lower()`, `contains()`, `startsWith()`, `endsWith()`
- **时间函数**: `now()`, `timeFormat()`, `timeDiff()`
- **类型转换**: `toString()`, `toNumber()`, `toBool()`

### 🔄 Transform动作增强

```json
{
  "type": "transform",
  "config": {
    "type": "expression",
    "parameters": {
      "expression": "sqrt(pow(x, 2) + pow(y, 2))"
    },
    "output_key": "magnitude",
    "output_type": "float",
    "precision": 2,
    "publish_subject": "iot.data.calculated",
    "error_action": "default",
    "default_value": 0
  }
}
```

**支持的转换类型**：
- `scale`: 数值缩放
- `offset`: 数值偏移
- `unit_convert`: 单位转换（温度、长度、重量）
- `format`: 格式化
- `expression`: 表达式计算
- `lookup`: 查找表
- `round`: 四舍五入
- `clamp`: 限幅
- `map`: 映射转换

### 🎯 Alert动作增强

```json
{
  "type": "alert",
  "config": {
    "level": "critical",
    "message": "设备 {{.DeviceID}} 异常，值: {{.Value}}",
    "throttle": "5m",
    "channels": [
      {
        "type": "console",
        "enabled": true
      },
      {
        "type": "webhook",
        "enabled": true,
        "config": {
          "url": "https://api.example.com/alerts",
          "method": "POST",
          "headers": {"Content-Type": "application/json"}
        }
      },
      {
        "type": "nats",
        "enabled": true,
        "config": {
          "subject": "iot.alerts.{{.Level}}"
        }
      }
    ]
  }
}
```

### 📊 Forward动作简化

简化后的Forward动作专注于NATS转发：

```json
{
  "type": "forward",
  "config": {
    "subject": "iot.data.processed.{{.DeviceID}}",
    "include_metadata": true,
    "transform_data": {
      "add_timestamp": true,
      "add_rule_info": true
    }
  }
}
```

## 🎯 实际使用场景

### 1. 智能温控系统

```json
{
  "id": "smart_thermostat",
  "name": "智能温控",
  "enabled": true,
  "conditions": {
    "type": "expression",
    "expression": "key == \"temperature\" && (value > 26 || value < 18) && time_range(9, 17)"
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "level": "info",
        "message": "自动调节温度：{{.Value}}°C → {{if gt .Value 26}}26{{else}}18{{end}}°C"
      }
    },
    {
      "type": "forward",
      "config": {
        "subject": "iot.control.{{.DeviceID}}.setpoint"
      }
    }
  ]
}
```

### 2. 设备健康监控

```json
{
  "id": "device_health",
  "name": "设备健康监控",
  "enabled": true,
  "conditions": {
    "type": "expression", 
    "expression": "regex(\"battery|power\", key) && toNumber(value) < 20"
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "设备{{.DeviceID}}电量不足：{{.Value}}%",
        "throttle": "1h"
      }
    }
  ]
}
```

### 3. 数据质量检查

```json
{
  "id": "data_quality",
  "name": "数据质量检查",
  "enabled": true,
  "conditions": {
    "type": "or",
    "or": [
      {
        "type": "expression",
        "expression": "value == null || value == \"\""
      },
      {
        "type": "expression", 
        "expression": "abs(value) > 1000"
      }
    ]
  },
  "actions": [
    {
      "type": "filter",
      "config": {
        "action": "discard",
        "reason": "数据质量异常"
      }
    }
  ]
}
```

## 📊 性能特性

- **高吞吐量**: 支持每秒数万条消息处理
- **低延迟**: 毫秒级规则匹配和执行
- **内存优化**: 正则表达式缓存、环形缓冲区
- **并发处理**: 支持并发规则执行
- **错误恢复**: 智能重试机制

## 🔧 配置选项

```yaml
rule_engine:
  enabled: true
  rules_dir: "./rules"
  watch_files: true
  max_concurrent_rules: 100
  action_timeout: "30s"
  enable_metrics: true
  
nats:
  servers: ["nats://localhost:4222"]
  input_subject: "iot.data.*"
  rule_events_subject: "iot.rules.*"
```

## 🛠️ 扩展开发

### 自定义动作处理器

```go
type CustomHandler struct {
    natsConn *nats.Conn
}

func (h *CustomHandler) Name() string {
    return "custom"
}

func (h *CustomHandler) Execute(ctx context.Context, point model.Point, rule *rules.Rule, config map[string]interface{}) (*rules.ActionResult, error) {
    start := time.Now()
    
    // 自定义处理逻辑
    result := processCustomLogic(point, config)
    
    return &rules.ActionResult{
        Type:     "custom",
        Success:  true,
        Duration: time.Since(start),
        Output:   result,
    }, nil
}

// 注册处理器
func init() {
    rules.RegisterActionHandler("custom", &CustomHandler{})
}
```

## 📈 监控指标

通过Web界面 `http://localhost:8081/metrics` 查看：

- `rules_total`: 规则总数
- `rules_enabled`: 启用规则数
- `points_processed`: 处理数据点数
- `rules_matched`: 匹配规则数
- `actions_executed`: 执行动作数
- `actions_succeeded`: 成功动作数
- `actions_failed`: 失败动作数
- `processing_duration`: 处理耗时

## 🐛 故障排除

### 常见问题

1. **规则不生效**: 
   - 检查规则JSON格式是否正确
   - 确认规则已启用 (`"enabled": true`)
   - 验证条件逻辑是否匹配数据

2. **表达式错误**:
   - 检查表达式语法
   - 确认变量名称正确
   - 使用内置函数检查参数类型

3. **性能问题**: 
   - 优化正则表达式
   - 使用简单条件替代复杂表达式
   - 启用动作异步执行

### 调试技巧

```bash
# 启用详细日志
export LOG_LEVEL=debug

# 监控规则执行
nats sub "iot.rules.*"

# 监控所有告警
nats sub "iot.alerts.*"

# 发送测试数据
nats pub iot.data.test '{
  "device_id": "test_device",
  "key": "temperature", 
  "value": 35.5,
  "type": "float",
  "timestamp": "2024-01-01T12:00:00Z"
}'
```

## 🔄 从旧版本升级

### 主要变更

1. **Forward动作简化**: 移除了HTTP、文件、MQTT转发，专注NATS
2. **表达式引擎增强**: 支持更多内置函数和复杂表达式
3. **错误处理改进**: 新增错误分类和重试机制
4. **性能优化**: 正则缓存、字符串操作优化

### 迁移指南

```json
// 旧版Forward配置
{
  "type": "forward",
  "config": {
    "targets": [
      {"type": "http", "url": "..."},
      {"type": "file", "path": "..."}
    ]
  }
}

// 新版Forward配置
{
  "type": "forward", 
  "config": {
    "subject": "iot.data.processed"
  }
}
```

## 🤝 贡献

欢迎贡献代码！请：

1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送分支 (`git push origin feature/amazing-feature`)
5. 发起 Pull Request

## 📄 许可证

本项目采用 [MIT 许可证](LICENSE)。

## 🙋‍♂️ 支持

- **文档**: [完整文档](docs/rule_engine.md)
- **示例**: [配置示例](examples/rules/)
- **Issues**: [GitHub Issues](https://github.com/y001j/iot-gateway/issues)
- **讨论**: [GitHub Discussions](https://github.com/y001j/iot-gateway/discussions)

---

**IoT Gateway 规则引擎** - 让IoT数据处理更智能、更灵活！ 🚀