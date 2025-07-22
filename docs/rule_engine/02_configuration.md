# 规则配置和使用指南

## 规则定义

规则是规则引擎的核心概念，每个规则包含条件和动作两个主要部分。当数据点满足条件时，将执行相应的动作。最新的规则引擎支持更强大的表达式系统和高性能的处理能力。

### 规则结构

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

- **id**: 规则唯一标识符（必填）
- **name**: 规则名称（必填）
- **description**: 规则描述（可选）
- **enabled**: 是否启用（默认true）
- **priority**: 优先级，数字越大优先级越高（默认0）
- **version**: 规则版本号（自动管理）
- **conditions**: 条件定义（必填）
- **actions**: 动作列表（必填）
- **tags**: 规则标签，用于分类和查询（可选）
- **created_at**: 创建时间（自动管理）
- **updated_at**: 更新时间（自动管理）

## 条件配置

规则引擎支持四种类型的条件：

1. **简单条件** - 基础字段比较
2. **复合条件** - 逻辑组合
3. **表达式条件** - 复杂表达式计算 🆕
4. **Lua脚本条件** - 脚本评估

### 1. 简单条件

基础的字段比较操作：

```json
{
  "conditions": {
    "type": "simple",
    "field": "value",
    "operator": "gt",
    "value": 30
  }
}
```

#### 支持的操作符

| 操作符 | 说明 | 示例 |
|--------|------|------|
| `eq` | 等于 | `"value": 30` |
| `ne`/`neq` | 不等于 | `"value": 30` |
| `gt` | 大于 | `"value": 30` |
| `gte` | 大于等于 | `"value": 30` |
| `lt` | 小于 | `"value": 30` |
| `lte` | 小于等于 | `"value": 30` |
| `contains` | 包含子字符串 | `"value": "temp"` |
| `startswith` | 以...开始 | `"value": "sensor"` |
| `endswith` | 以...结束 | `"value": "_001"` |
| `regex` | 正则表达式匹配 | `"value": "^temp_.*"` |

#### 支持的字段

- `device_id` - 设备ID
- `key` - 数据键
- `value` - 数据值
- `type` - 数据类型
- `timestamp` - 时间戳
- `tags.{tag_name}` - 标签字段（嵌套访问）

### 2. 复合条件

使用逻辑操作符组合多个条件：

```json
{
  "conditions": {
    "and": [
      {
        "field": "device_id",
        "operator": "eq",
        "value": "device_001"
      },
      {
        "field": "value",
        "operator": "gt",
        "value": 30
      }
    ]
  }
}
```

#### 逻辑操作符

- **and**: 所有条件都必须满足
- **or**: 任一条件满足即可
- **not**: 条件不满足

#### 复杂嵌套示例

```json
{
  "conditions": {
    "and": [
      {
        "field": "key",
        "operator": "eq",
        "value": "temperature"
      },
      {
        "or": [
          {
            "field": "device_id",
            "operator": "startswith",
            "value": "sensor_"
          },
          {
            "field": "tags.location",
            "operator": "eq",
            "value": "building_1"
          }
        ]
      },
      {
        "not": {
          "field": "tags.quality",
          "operator": "eq",
          "value": "bad"
        }
      }
    ]
  }
}
```

### 3. 表达式条件 🆕

使用强大的表达式引擎进行复杂条件评估：

```json
{
  "conditions": {
    "type": "expression",
    "expression": "value > 30 && device_id == 'sensor_001'"
  }
}
```

#### 支持的表达式语法

##### 基础运算符
```javascript
// 算术运算
value * 1.8 + 32
sqrt(value) + abs(offset)
pow(value, 2)

// 比较运算
value > 30
value >= min_threshold && value <= max_threshold

// 逻辑运算
value > 30 && humidity < 60
temperature > 25 || pressure < 1000

// 字符串运算
contains(device_id, "sensor")
startsWith(key, "temp")
endsWith(device_id, "_001")
```

##### 内置函数

###### 数学函数
- `abs(x)` - 绝对值
- `max(x, y, ...)` - 最大值
- `min(x, y, ...)` - 最小值
- `sqrt(x)` - 平方根
- `pow(x, y)` - x的y次幂
- `floor(x)` - 向下取整
- `ceil(x)` - 向上取整

###### 字符串函数
- `len(str)` - 字符串长度
- `upper(str)` - 转大写
- `lower(str)` - 转小写
- `contains(str, substr)` - 包含检查
- `startsWith(str, prefix)` - 前缀检查
- `endsWith(str, suffix)` - 后缀检查

###### 时间函数
- `now()` - 当前时间戳
- `timeFormat(time, format)` - 时间格式化
- `timeDiff(time1, time2)` - 时间差（秒）

###### 类型转换函数
- `toString(value)` - 转换为字符串
- `toNumber(value)` - 转换为数字
- `toBool(value)` - 转换为布尔值

#### 高级表达式示例

```json
{
  "conditions": {
    "type": "expression",
    "expression": "sqrt(pow(value - 20, 2)) > 5 && contains(upper(device_id), 'SENSOR')"
  }
}
```

##### 时间范围检查
```json
{
  "conditions": {
    "type": "expression", 
    "expression": "time_range(9, 17)"  // 工作时间 9:00-17:00
  }
}
```

##### 正则表达式匹配
```json
{
  "conditions": {
    "type": "expression",
    "expression": "regex('temp_.*', key)"
  }
}
```

##### 复杂业务逻辑
```json
{
  "conditions": {
    "type": "expression",
    "expression": "value > (avg_value * 1.2) && timeDiff(now(), timestamp) < 300"
  }
}
```

### 4. Lua脚本条件

使用Lua脚本进行复杂逻辑评估（功能开发中）：

```json
{
  "conditions": {
    "type": "lua",
    "script": "return point.value > 30 and string.find(point.device_id, 'sensor') ~= nil"
  }
}
```

## 动作配置

每个规则可以配置多个动作，支持串行和并行执行。最新版本对聚合动作进行了重大优化。

### 动作通用配置

```json
{
  "type": "action_type",
  "config": {
    // 动作特定配置
  },
  "async": false,           // 是否异步执行
  "timeout": "30s",         // 超时时间
  "retry": 3                // 重试次数
}
```

### 1. 聚合动作 🆕 (性能优化)

经过重大优化的聚合动作，支持增量统计和高性能处理：

```json
{
  "type": "aggregate",
  "config": {
    "window_size": 10,              // 窗口大小（数据点数量）
    "functions": ["avg", "max", "min", "sum", "count", "stddev"],
    "group_by": ["device_id", "key"],
    "output": {
      "key_template": "{{.key}}_stats",
      "forward": true
    },
    "ttl": "10m"                    // 状态存活时间
  }
}
```

#### 支持的聚合函数

| 函数 | 说明 | 备注 |
|------|------|------|
| `avg`/`mean` | 平均值 | 增量计算 |
| `sum` | 求和 | 增量计算 |
| `count` | 计数 | O(1)复杂度 |
| `min` | 最小值 | 滑动窗口 |
| `max` | 最大值 | 滑动窗口 |
| `stddev` | 标准差 | 增量计算 |
| `variance` | 方差 | 增量计算 |
| `median` | 中位数 | 排序计算 |
| `first` | 第一个值 | 窗口首值 |
| `last` | 最后一个值 | 窗口尾值 |

#### 配置选项详解

- **window_size**: 滑动窗口大小，0表示累积模式
- **functions**: 要计算的聚合函数列表
- **group_by**: 分组字段，支持 device_id, key, type 或 tags.{name}
- **ttl**: 聚合状态的生存时间，超时自动清理
- **output**: 输出配置，支持模板化

#### 聚合示例

```json
{
  "type": "aggregate",
  "config": {
    "window_size": 5,
    "functions": ["avg", "stddev"],
    "group_by": ["device_id"],
    "output": {
      "key_template": "{{.key}}_stats",
      "forward": true
    },
    "ttl": "5m"
  }
}
```

### 2. 转换动作

数据转换和格式化：

```json
{
  "type": "transform",
  "config": {
    "type": "scale",              // 转换类型
    "factor": 1.8,                // 缩放因子
    "offset": 32,                 // 偏移量
    "field": "value",             // 目标字段
    "precision": 2,               // 精度
    "add_tags": {                 // 添加标签
      "unit": "°F",
      "converted": "true"
    },
    "remove_tags": ["temp_unit"]  // 移除标签
  }
}
```

#### 转换类型

- **scale**: 数值缩放
- **offset**: 数值偏移
- **unit_convert**: 单位转换
- **expression**: 表达式转换
- **lookup**: 查找表映射

### 3. 过滤动作

数据筛选和质量控制：

```json
{
  "type": "filter",
  "config": {
    "type": "range",              // 过滤类型
    "min": 0,                     // 最小值
    "max": 100,                   // 最大值
    "drop_on_match": true,        // 匹配时是否丢弃
    "deduplicate": {              // 去重配置
      "window_size": 10,
      "tolerance": 0.1
    }
  }
}
```

#### 过滤类型

- **range**: 范围过滤
- **quality**: 质量过滤
- **duplicate**: 重复数据过滤
- **rate_limit**: 速率限制
- **null_filter**: 空值过滤

### 4. 转发动作

多目标数据转发：

```json
{
  "type": "forward",
  "config": {
    "targets": [                  // 多目标支持
      {
        "type": "http",
        "url": "http://api.example.com/data",
        "method": "POST",
        "headers": {
          "Content-Type": "application/json"
        }
      },
      {
        "type": "mqtt", 
        "topic": "sensors/{{.device_id}}/{{.key}}",
        "qos": 1
      }
    ],
    "template": {                 // 数据模板
      "device": "{{.device_id}}",
      "sensor": "{{.key}}",
      "value": "{{.value}}",
      "timestamp": "{{.timestamp}}"
    },
    "batch": {                    // 批量配置
      "size": 10,
      "timeout": "5s"
    }
  }
}
```

### 5. 告警动作

多通道告警通知：

```json
{
  "type": "alert",
  "config": {
    "level": "warning",           // 告警级别
    "message": "设备 {{.device_id}} {{.key}} 值为 {{.value}}，超过阈值",
    "channels": [                 // 通知通道
      {
        "type": "console",
        "enabled": true
      },
      {
        "type": "webhook",
        "url": "http://alert.example.com/webhook",
        "method": "POST"
      },
      {
        "type": "email",
        "to": ["admin@example.com"]
      }
    ],
    "throttle": {                 // 告警抑制
      "window": "5m",
      "max_count": 3
    }
  }
}
```

## 完整配置示例

### 1. 高性能温度监控规则

```json
{
  "id": "temp_monitor_optimized",
  "name": "优化温度监控",
  "description": "使用增量统计的高性能温度监控",
  "enabled": true,
  "priority": 100,
  "conditions": {
    "type": "expression",
    "expression": "key == 'temperature' && value > -50 && value < 100"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window_size": 10,
        "functions": ["avg", "max", "min", "stddev"],
        "group_by": ["device_id"],
        "output": {
          "key_template": "{{.key}}_stats",
          "forward": true
        },
        "ttl": "10m"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "设备 {{.device_id}} 温度异常: {{.value}}°C",
        "channels": [
          {
            "type": "console"
          }
        ]
      },
      "async": true
    }
  ],
  "tags": {
    "category": "monitoring",
    "type": "temperature",
    "optimized": "true"
  }
}
```

### 2. 复杂业务逻辑规则

```json
{
  "id": "complex_business_rule",
  "name": "复杂业务逻辑",
  "description": "使用表达式引擎的复杂业务规则",
  "enabled": true,
  "priority": 200,
  "conditions": {
    "type": "expression",
    "expression": "contains(device_id, 'sensor') && time_range(8, 18) && (value > avg_threshold * 1.2 || abs(value - last_value) > 10)"
  },
  "actions": [
    {
      "type": "transform",
      "config": {
        "type": "expression",
        "expression": "value * 1.8 + 32",
        "field": "value",
        "add_tags": {
          "unit": "fahrenheit",
          "processed": "true"
        }
      }
    },
    {
      "type": "forward",
      "config": {
        "targets": [
          {
            "type": "influxdb",
            "measurement": "{{.key}}_processed"
          }
        ]
      }
    }
  ]
}
```

### 3. 数据质量控制规则

```json
{
  "id": "data_quality_control",
  "name": "数据质量控制",
  "description": "多层次数据质量过滤",
  "enabled": true,
  "priority": 500,
  "conditions": {
    "type": "simple",
    "field": "value",
    "operator": "neq",
    "value": null
  },
  "actions": [
    {
      "type": "filter",
      "config": {
        "type": "range",
        "min": -100,
        "max": 200,
        "drop_on_match": false
      }
    },
    {
      "type": "filter", 
      "config": {
        "type": "duplicate",
        "window_size": 5,
        "tolerance": 0.01,
        "drop_on_match": true
      }
    },
    {
      "type": "transform",
      "config": {
        "type": "round",
        "precision": 2,
        "add_tags": {
          "quality_checked": "true"
        }
      }
    }
  ]
}
```

## 配置验证和最佳实践

### 配置验证

规则引擎提供完整的配置验证：

1. **结构验证**
   - JSON/YAML格式正确性
   - 必填字段完整性检查
   - 字段类型验证

2. **条件验证**
   - 条件语法正确性
   - 操作符有效性验证
   - 字段引用有效性检查
   - 表达式语法验证

3. **动作验证**
   - 动作类型有效性
   - 配置参数完整性
   - 模板语法正确性

### 性能最佳实践

#### 1. 条件优化
```json
// 推荐：简单条件优先
{
  "and": [
    {"field": "key", "operator": "eq", "value": "temperature"},  // 快速过滤
    {"type": "expression", "expression": "complex_calculation(value)"}  // 复杂计算
  ]
}

// 避免：复杂条件在前
{
  "and": [
    {"type": "expression", "expression": "complex_calculation(value)"},
    {"field": "key", "operator": "eq", "value": "temperature"}
  ]
}
```

#### 2. 聚合优化
```json
// 推荐：设置合理的TTL
{
  "type": "aggregate",
  "config": {
    "window_size": 10,
    "functions": ["avg"],
    "ttl": "5m"  // 避免内存泄漏
  }
}

// 推荐：使用分组减少状态数量
{
  "type": "aggregate", 
  "config": {
    "group_by": ["device_id"],  // 合理分组
    "window_size": 20
  }
}
```

#### 3. 规则优先级
```text
高优先级 (900-1000): 数据质量过滤
中高优先级 (700-899): 业务规则
中优先级 (400-699): 数据转换
低优先级 (100-399): 数据转发
最低优先级 (1-99): 统计和监控
```

### 监控和调试

#### 获取规则执行统计
```bash
curl http://localhost:8081/api/rules/metrics
```

#### 获取错误信息
```bash
curl http://localhost:8081/api/rules/errors?limit=10
```

#### 健康检查
```bash
curl http://localhost:8081/api/rules/health
```

通过这些配置和最佳实践，您可以充分利用规则引擎的强大功能和优化性能，构建高效、可靠的IoT数据处理流水线。