# 动作类型详细说明

规则引擎支持五种核心动作类型，每种动作类型都有其特定的用途和配置选项。经过最新优化，特别是聚合动作获得了显著的性能提升。本文档详细说明每种动作类型的功能、配置和使用方法。

## 1. 聚合动作 (Aggregate) 🆕 高性能优化

聚合动作经过重大重构，采用增量统计算法，实现了O(1)复杂度的高性能数据聚合。

### 🚀 性能特性

- **增量统计**: O(1)复杂度的平均值、方差、标准差计算
- **滑动窗口**: 高效的固定大小窗口管理
- **智能缓存**: 统计结果缓存，避免重复计算
- **自动清理**: TTL-based状态管理，防止内存泄漏
- **并发安全**: 线程安全的状态管理

### 配置选项

```json
{
  "type": "aggregate",
  "config": {
    "window_size": 10,              // 滑动窗口大小（数据点数量）
    "functions": [                  // 聚合函数列表
      "avg", "max", "min", "sum", 
      "count", "stddev", "variance"
    ],
    "group_by": ["device_id", "key"], // 分组字段
    "output": {                     // 输出配置
      "key_template": "{{.key}}_stats",
      "forward": true
    },
    "ttl": "10m"                    // 状态存活时间
  }
}
```

### 支持的聚合函数 (共28个)

**基础统计函数** (13个):
| 函数 | 说明 | 计算复杂度 | 特性 |
|------|------|------------|------|
| `avg`/`mean`/`average` | 平均值 | O(1) | 增量计算 |
| `sum` | 求和 | O(1) | 增量计算 |
| `count` | 计数 | O(1) | 直接访问 |
| `min` | 最小值 | O(n) | 滑动窗口 |
| `max` | 最大值 | O(n) | 滑动窗口 |
| `stddev`/`std` | 标准差 | O(1) | 增量计算 |
| `variance` | 方差 | O(1) | 增量计算 |
| `median` | 中位数 | O(n log n) | 排序计算 |
| `first` | 第一个值 | O(1) | 直接访问 |
| `last` | 最后一个值 | O(1) | 直接访问 |

**百分位数函数** (6个):
| 函数 | 说明 | 计算复杂度 | 特性 |
|------|------|------------|------|
| `p25` | 25%分位数 | O(n log n) | 排序计算 |
| `p50` | 50%分位数 | O(n log n) | 排序计算 |
| `p75` | 75%分位数 | O(n log n) | 排序计算 |
| `p90` | 90%分位数 | O(n log n) | 排序计算 |
| `p95` | 95%分位数 | O(n log n) | 排序计算 |
| `p99` | 99%分位数 | O(n log n) | 排序计算 |

**数据质量函数** (3个):
| 函数 | 说明 | 计算复杂度 | 特性 |
|------|------|------------|------|
| `null_rate` | 空值率 | O(1) | 增量计算 |
| `completeness` | 完整性(1-空值率) | O(1) | 增量计算 |
| `outlier_count` | 异常值数量 | O(n) | 3σ检测 |

**变化检测函数** (4个):
| 函数 | 说明 | 计算复杂度 | 特性 |
|------|------|------------|------|
| `change` | 变化量 | O(1) | 当前值-上一个值 |
| `change_rate` | 变化率 | O(1) | 百分比变化 |
| `volatility` | 波动性 | O(1) | 标准差计算 |
| `cv` | 变异系数 | O(1) | 标准差/平均值 |

**阈值监控函数** (3个):
| 函数 | 说明 | 计算复杂度 | 特性 |
|------|------|------------|------|
| `above_count` | 超过阈值数量 | O(n) | 条件计数 |
| `below_count` | 低于阈值数量 | O(n) | 条件计数 |
| `in_range_count` | 范围内数量 | O(n) | 范围检查 |

### 配置选项详解

#### window_size
- **0**: 累积模式，计算所有历史数据
- **>0**: 滑动窗口模式，保持固定数量的最新数据点

#### functions
支持单个或多个聚合函数：
```json
// 单个函数
"functions": ["avg"]

// 多个函数
"functions": ["avg", "max", "min", "stddev"]

// 支持别名
"functions": ["mean", "std"]  // 等同于 ["avg", "stddev"]
```

#### group_by
支持的分组字段：
- `device_id`: 按设备分组
- `key`: 按数据键分组  
- `type`: 按数据类型分组
- `tags.{name}`: 按标签分组

#### ttl
状态存活时间，支持以下格式：
- `"5m"`: 5分钟
- `"1h"`: 1小时
- `"30s"`: 30秒
- `"2h30m"`: 2小时30分钟

### 使用示例

#### 1. 简单温度平均值计算
```json
{
  "type": "aggregate",
  "config": {
    "window_size": 10,
    "functions": ["avg"],
    "group_by": ["device_id"],
    "output": {
      "key_template": "{{.key}}_avg",
      "forward": true
    },
    "ttl": "5m"
  }
}
```

#### 2. 多维度统计分析
```json
{
  "type": "aggregate", 
  "config": {
    "window_size": 20,
    "functions": ["avg", "max", "min", "stddev"],
    "group_by": ["device_id", "key"],
    "output": {
      "key_template": "{{.key}}_stats",
      "forward": true
    },
    "ttl": "15m"
  }
}
```

#### 3. 累积统计（无窗口限制）
```json
{
  "type": "aggregate",
  "config": {
    "window_size": 0,  // 累积模式
    "functions": ["count", "sum", "avg"],
    "group_by": ["device_id"],
    "ttl": "1h"
  }
}
```

#### 4. 按标签分组的高级聚合
```json
{
  "type": "aggregate",
  "config": {
    "window_size": 15,
    "functions": ["avg", "variance"],
    "group_by": ["device_id", "tags.location", "tags.sensor_type"],
    "output": {
      "key_template": "{{.key}}_by_location",
      "forward": true
    },
    "ttl": "30m"
  }
}
```

### 聚合结果格式

聚合动作的输出结果格式：

```json
{
  "device_id": "sensor_001",
  "key": "temperature_stats",
  "window": "window_size:10",
  "group_by": {
    "device_id": "sensor_001",
    "key": "temperature"
  },
  "functions": {
    "avg": 25.5,
    "max": 30.2,
    "min": 20.1,
    "stddev": 2.3,
    "count": 10
  },
  "count": 10,
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## 2. 转换动作 (Transform)

转换动作用于对数据进行各种转换和处理操作。

### 功能特性

- **数值转换**: 缩放、偏移、精度控制
- **单位转换**: 温度、长度、重量等单位转换
- **表达式转换**: 使用表达式引擎进行复杂转换
- **数据类型转换**: 字符串、数字、布尔值转换
- **标签管理**: 添加、删除、修改标签

### 配置选项

```json
{
  "type": "transform",
  "config": {
    "type": "scale",              // 转换类型
    "factor": 1.8,                // 缩放因子
    "offset": 32,                 // 偏移量
    "field": "value",             // 目标字段
    "precision": 2,               // 精度
    "expression": "value * 1.8 + 32", // 表达式转换
    "add_tags": {                 // 添加标签
      "unit": "°F",
      "converted": "true"
    },
    "remove_tags": ["temp_unit"], // 移除标签
    "error_handling": "ignore"    // 错误处理：ignore, skip, fail
  }
}
```

### 转换类型

#### 数值转换

##### 缩放转换 (scale)
```json
{
  "type": "transform",
  "config": {
    "type": "scale",
    "factor": 1.8,
    "field": "value"
  }
}
```

##### 偏移转换 (offset)
```json
{
  "type": "transform",
  "config": {
    "type": "offset",
    "value": 32,
    "field": "value"
  }
}
```

##### 精度控制 (round)
```json
{
  "type": "transform",
  "config": {
    "type": "round",
    "precision": 2,
    "field": "value"
  }
}
```

#### 单位转换 (unit_convert)

##### 温度转换
```json
{
  "type": "transform",
  "config": {
    "type": "unit_convert",
    "from_unit": "celsius",
    "to_unit": "fahrenheit",
    "field": "value"
  }
}
```

支持的温度单位：
- `celsius` (°C)
- `fahrenheit` (°F)  
- `kelvin` (K)

##### 长度转换
```json
{
  "type": "transform",
  "config": {
    "type": "unit_convert",
    "from_unit": "meter",
    "to_unit": "feet",
    "field": "value"
  }
}
```

支持的长度单位：
- `meter`, `centimeter`, `millimeter`
- `feet`, `inch`, `yard`
- `kilometer`, `mile`

##### 重量转换
```json
{
  "type": "transform",
  "config": {
    "type": "unit_convert", 
    "from_unit": "kilogram",
    "to_unit": "pound",
    "field": "value"
  }
}
```

#### 表达式转换 (expression) 🆕
使用强大的表达式引擎进行复杂转换：

```json
{
  "type": "transform",
  "config": {
    "type": "expression",
    "expression": "sqrt(pow(value, 2) + pow(offset, 2))",
    "field": "value"
  }
}
```

支持的表达式功能：
- 数学函数：`sqrt()`, `pow()`, `abs()`, `floor()`, `ceil()`
- 三角函数：`sin()`, `cos()`, `tan()`
- 条件表达式：`value > 0 ? value : 0`

#### 查找表映射 (lookup)
```json
{
  "type": "transform",
  "config": {
    "type": "lookup",
    "table": {
      "0": "正常",
      "1": "警告", 
      "2": "错误"
    },
    "field": "status",
    "default": "未知"
  }
}
```

### 使用示例

#### 1. 摄氏度转华氏度
```json
{
  "type": "transform",
  "config": {
    "type": "unit_convert",
    "from_unit": "celsius",
    "to_unit": "fahrenheit",
    "field": "value",
    "precision": 1,
    "add_tags": {
      "unit": "°F",
      "converted": "true"
    },
    "remove_tags": ["original_unit"]
  }
}
```

#### 2. 复杂数学转换
```json
{
  "type": "transform",
  "config": {
    "type": "expression",
    "expression": "round(sqrt(value * 1000) / 10, 2)",
    "field": "value",
    "add_tags": {
      "processed": "sqrt_scaled"
    }
  }
}
```

#### 3. 多步骤转换
```json
{
  "type": "transform",
  "config": {
    "type": "scale",
    "factor": 1.8,
    "field": "value"
  }
},
{
  "type": "transform", 
  "config": {
    "type": "offset",
    "value": 32,
    "field": "value",
    "precision": 1
  }
}
```

## 3. 过滤动作 (Filter)

过滤动作用于筛选或丢弃特定的数据点，确保数据质量。

### 功能特性

- **范围过滤**: 数值范围检查
- **质量过滤**: 基于质量字段过滤
- **重复数据过滤**: 去重处理
- **速率限制**: 控制数据流速
- **模式匹配**: 基于模式的过滤
- **空值过滤**: 过滤空值或无效数据

### 配置选项

```json
{
  "type": "filter",
  "config": {
    "type": "range",              // 过滤类型
    "min": 0,                     // 最小值
    "max": 100,                   // 最大值
    "field": "value",             // 目标字段
    "drop_on_match": true,        // 匹配时是否丢弃
    "conditions": {               // 过滤条件
      "field": "quality",
      "operator": "eq", 
      "value": 0
    }
  }
}
```

### 过滤类型

#### 范围过滤 (range)
```json
{
  "type": "filter",
  "config": {
    "type": "range",
    "min": -50,
    "max": 100,
    "field": "value",
    "drop_on_match": false  // 保留范围内的数据
  }
}
```

#### 重复数据过滤 (duplicate)
```json
{
  "type": "filter",
  "config": {
    "type": "duplicate",
    "window_size": 10,
    "tolerance": 0.1,        // 容差值
    "field": "value",
    "drop_on_match": true
  }
}
```

#### 速率限制 (rate_limit)
```json
{
  "type": "filter",
  "config": {
    "type": "rate_limit",
    "max_rate": 10,          // 每秒最大数据点数
    "window": "1s",
    "drop_on_exceed": true
  }
}
```

#### 模式匹配 (pattern)
```json
{
  "type": "filter",
  "config": {
    "type": "pattern",
    "patterns": ["temp_*", "hum_*"],
    "field": "key",
    "drop_on_match": false   // 保留匹配的数据
  }
}
```

#### 阈值过滤 (threshold)
```json
{
  "type": "filter",
  "config": {
    "type": "threshold",
    "upper_threshold": 80,
    "lower_threshold": 20,
    "field": "value",
    "drop_on_exceed": true
  }
}
```

### 使用示例

#### 1. 数据质量过滤
```json
{
  "type": "filter",
  "config": {
    "type": "range",
    "min": -273.15,  // 绝对零度
    "max": 1000,     // 合理上限
    "field": "value",
    "drop_on_match": false,
    "add_tags": {
      "quality_checked": "true"
    }
  }
}
```

#### 2. 去重过滤
```json
{
  "type": "filter",
  "config": {
    "type": "duplicate",
    "window_size": 5,
    "tolerance": 0.01,
    "field": "value",
    "drop_on_match": true
  }
}
```

#### 3. 组合过滤条件
```json
{
  "type": "filter",
  "config": {
    "conditions": {
      "and": [
        {
          "field": "quality",
          "operator": "eq",
          "value": 1
        },
        {
          "field": "value",
          "operator": "gt",
          "value": 0
        }
      ]
    },
    "drop_on_match": false
  }
}
```

## 4. 转发动作 (Forward)

转发动作用于将数据发送到其他系统或服务，支持多种目标类型和格式。

### 功能特性

- **多目标支持**: 同时转发到多个目标
- **多协议支持**: HTTP, MQTT, NATS, File等
- **数据转换**: 灵活的模板系统
- **批量处理**: 提高转发效率
- **错误重试**: 可靠的错误处理机制
- **异步处理**: 支持异步转发

### 配置选项

```json
{
  "type": "forward",
  "config": {
    "targets": [                  // 转发目标列表
      {
        "type": "http",
        "url": "http://api.example.com/data",
        "method": "POST",
        "headers": {
          "Content-Type": "application/json",
          "Authorization": "Bearer {{.token}}"
        }
      }
    ],
    "template": {                 // 数据转换模板
      "device": "{{.device_id}}",
      "sensor": "{{.key}}",
      "value": "{{.value}}",
      "timestamp": "{{.timestamp}}"
    },
    "batch": {                    // 批量配置
      "size": 10,
      "timeout": "5s"
    },
    "retry": {                    // 重试配置
      "max_attempts": 3,
      "interval": "1s",
      "backoff": "exponential"
    },
    "async": true                 // 异步处理
  }
}
```

### 目标类型

#### HTTP目标
```json
{
  "type": "http",
  "url": "http://api.example.com/sensors",
  "method": "POST",
  "headers": {
    "Content-Type": "application/json",
    "X-API-Key": "your-api-key"
  },
  "timeout": "30s"
}
```

#### MQTT目标
```json
{
  "type": "mqtt",
  "broker": "mqtt://localhost:1883",
  "topic": "sensors/{{.device_id}}/{{.key}}",
  "qos": 1,
  "retained": false,
  "username": "mqtt_user",
  "password": "mqtt_pass"
}
```

#### NATS目标  
```json
{
  "type": "nats",
  "subject": "iot.processed.{{.key}}",
  "url": "nats://localhost:4222"
}
```

#### 文件目标
```json
{
  "type": "file",
  "path": "/data/sensors/{{.device_id}}_{{.date}}.jsonl",
  "format": "jsonl",            // json, jsonl, csv
  "rotation": {
    "size": "100MB",
    "time": "24h"
  }
}
```

### 数据格式转换

#### JSON格式
```json
{
  "template": {
    "deviceId": "{{.device_id}}",
    "measurement": "{{.key}}",
    "value": "{{.value}}",
    "timestamp": "{{.timestamp}}",
    "tags": "{{.tags}}"
  },
  "format": "json"
}
```

#### InfluxDB Line Protocol
```json
{
  "template": "{{.key}},device={{.device_id}} value={{.value}} {{.timestamp_ns}}",
  "format": "influx"
}
```

#### CSV格式
```json
{
  "template": "{{.device_id}},{{.key}},{{.value}},{{.timestamp}}",
  "format": "csv",
  "header": "device_id,key,value,timestamp"
}
```

### 使用示例

#### 1. 多目标转发
```json
{
  "type": "forward",
  "config": {
    "targets": [
      {
        "type": "http",
        "url": "http://analytics.example.com/api/data",
        "method": "POST"
      },
      {
        "type": "mqtt",
        "topic": "processed/{{.device_id}}/{{.key}}",
        "qos": 1
      },
      {
        "type": "file",
        "path": "/backup/{{.date}}/{{.device_id}}.jsonl"
      }
    ],
    "template": {
      "id": "{{.device_id}}-{{.timestamp}}",
      "metric": "{{.key}}", 
      "value": "{{.value}}",
      "ts": "{{.timestamp}}"
    }
  }
}
```

#### 2. 批量HTTP转发
```json
{
  "type": "forward",
  "config": {
    "targets": [
      {
        "type": "http",
        "url": "http://warehouse.example.com/api/batch",
        "method": "POST",
        "headers": {
          "Content-Type": "application/json"
        }
      }
    ],
    "batch": {
      "size": 50,
      "timeout": "10s"
    },
    "template": {
      "data": "{{.batch}}"  // 批量数据
    }
  }
}
```

## 5. 告警动作 (Alert)

告警动作用于生成和发送告警信息，支持多种通知通道和告警策略。

### 功能特性

- **多级别告警**: debug, info, warning, error, critical
- **多通道通知**: console, webhook, email, sms
- **告警模板**: 自定义消息格式
- **告警抑制**: 防止告警风暴
- **告警聚合**: 相同类型告警合并
- **条件告警**: 基于条件触发告警

### 配置选项

```json
{
  "type": "alert",
  "config": {
    "level": "warning",           // 告警级别
    "message": "设备 {{.device_id}} {{.key}} 异常值: {{.value}}",
    "channels": [                 // 通知通道
      {
        "type": "console",
        "enabled": true
      },
      {
        "type": "webhook",
        "url": "http://alert.example.com/webhook",
        "method": "POST"
      }
    ],
    "conditions": {               // 告警条件
      "field": "value",
      "operator": "gt",
      "value": 50
    },
    "throttle": {                 // 告警抑制
      "window": "5m",
      "max_count": 3
    },
    "tags": {                     // 告警标签
      "severity": "high",
      "component": "sensor"
    }
  }
}
```

### 告警级别

| 级别 | 说明 | 用途 |
|------|------|------|
| `debug` | 调试信息 | 开发调试 |
| `info` | 一般信息 | 状态通知 |
| `warning` | 警告 | 需要关注的问题 |
| `error` | 错误 | 需要处理的错误 |
| `critical` | 严重错误 | 紧急处理的问题 |

### 通知通道

#### 控制台输出 (console)
```json
{
  "type": "console",
  "enabled": true,
  "format": "text"  // text, json
}
```

#### Webhook通知 (webhook)
```json
{
  "type": "webhook",
  "url": "http://alert-service.example.com/alerts",
  "method": "POST",
  "headers": {
    "Content-Type": "application/json",
    "X-Alert-Source": "iot-gateway"
  },
  "template": {
    "alert_id": "{{.id}}",
    "level": "{{.level}}",
    "message": "{{.message}}",
    "device": "{{.device_id}}",
    "timestamp": "{{.timestamp}}"
  }
}
```

#### 邮件通知 (email)
```json
{
  "type": "email",
  "to": ["admin@example.com", "ops@example.com"],
  "cc": ["manager@example.com"],
  "subject": "IoT告警: {{.level}} - {{.device_id}}",
  "template": "alert_email.html",
  "smtp": {
    "server": "smtp.example.com:587",
    "username": "alerts@example.com",
    "password": "smtp_password",
    "tls": true
  }
}
```

#### 短信通知 (sms)
```json
{
  "type": "sms",
  "numbers": ["+1234567890", "+0987654321"],
  "message": "IoT告警: {{.device_id}} {{.message}}",
  "provider": {
    "type": "twilio",
    "account_sid": "your_account_sid",
    "auth_token": "your_auth_token",
    "from": "+1234567890"
  }
}
```

### 告警抑制和聚合

#### 告警抑制 (throttle)
防止告警风暴：
```json
{
  "throttle": {
    "window": "10m",      // 时间窗口
    "max_count": 5,       // 最大告警数
    "key": "{{.device_id}}-{{.key}}"  // 抑制键
  }
}
```

#### 告警聚合 (aggregate)
合并相同类型的告警：
```json
{
  "aggregate": {
    "window": "15m",
    "group_by": ["device_id", "level"],
    "max_alerts": 10,
    "summary_template": "{{.count}}个设备出现{{.level}}级别告警"
  }
}
```

### 使用示例

#### 1. 简单告警
```json
{
  "type": "alert",
  "config": {
    "level": "warning",
    "message": "温度过高: {{.value}}°C",
    "channels": [
      {
        "type": "console"
      }
    ]
  }
}
```

#### 2. 多渠道高级告警
```json
{
  "type": "alert",
  "config": {
    "level": "critical",
    "message": "设备 {{.device_id}} 在 {{.timestamp}} 出现严重故障: {{.key}}={{.value}}",
    "channels": [
      {
        "type": "webhook",
        "url": "http://alert.example.com/critical",
        "template": {
          "alert_type": "device_failure",
          "device_id": "{{.device_id}}",
          "metric": "{{.key}}",
          "value": "{{.value}}",
          "level": "{{.level}}",
          "timestamp": "{{.timestamp}}"
        }
      },
      {
        "type": "email",
        "to": ["oncall@example.com"],
        "subject": "CRITICAL: 设备故障 - {{.device_id}}"
      },
      {
        "type": "sms",
        "numbers": ["+1234567890"],
        "message": "CRITICAL: {{.device_id}} 故障"
      }
    ],
    "throttle": {
      "window": "30m",
      "max_count": 3
    },
    "tags": {
      "severity": "critical",
      "team": "ops",
      "escalation": "immediate"
    }
  }
}
```

#### 3. 条件告警
```json
{
  "type": "alert",
  "config": {
    "conditions": {
      "and": [
        {
          "field": "value",
          "operator": "gt", 
          "value": 80
        },
        {
          "field": "tags.priority",
          "operator": "eq",
          "value": "high"
        }
      ]
    },
    "level": "error",
    "message": "高优先级设备 {{.device_id}} 超过阈值: {{.value}}",
    "channels": [
      {
        "type": "webhook",
        "url": "http://priority-alerts.example.com/webhook"
      }
    ]
  }
}
```

## 动作执行控制

### 异步执行
```json
{
  "type": "alert",
  "config": {
    // ... 配置
  },
  "async": true,        // 异步执行，不阻塞后续动作
  "timeout": "30s"      // 超时时间
}
```

### 重试策略
```json
{
  "type": "forward",
  "config": {
    // ... 配置
  },
  "retry": {
    "max_attempts": 3,
    "interval": "1s",
    "backoff": "exponential"  // linear, exponential
  }
}
```

### 条件执行
```json
{
  "type": "transform",
  "config": {
    // ... 配置
  },
  "conditions": {           // 动作执行条件
    "field": "quality",
    "operator": "eq",
    "value": 1
  }
}
```

通过这些丰富的动作类型和配置选项，规则引擎能够处理各种复杂的IoT数据处理场景，同时保证高性能和可靠性。特别是经过优化的聚合动作，为大规模数据处理提供了强有力的支持。