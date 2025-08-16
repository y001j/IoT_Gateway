# API 接口参考

规则引擎提供了丰富的REST API接口，用于规则管理、监控、调试和系统管理。所有API接口都支持JSON格式的请求和响应，经过最新优化后提供了全面的监控和管理功能。

## 基础信息

- **Base URL**: `http://localhost:8081/api/v1`
- **Content-Type**: `application/json`
- **认证**: 根据配置可能需要API密钥或身份认证

## REST API 接口

### 规则管理 API

#### 1. 获取规则列表

获取所有规则的列表信息。

```http
GET /api/rules
```

**查询参数**:
- `enabled` (boolean, 可选): 过滤启用状态
- `category` (string, 可选): 按分类过滤
- `priority` (int, 可选): 按优先级过滤
- `limit` (int, 可选): 限制返回数量，默认100
- `offset` (int, 可选): 分页偏移量，默认0

**响应示例**:
```json
{
  "success": true,
  "data": {
    "rules": [
      {
        "id": "temp_monitor",
        "name": "温度监控",
        "description": "监控温度传感器数据",
        "enabled": true,
        "priority": 100,
        "version": 1,
        "tags": {
          "category": "monitoring",
          "type": "temperature"
        },
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
      }
    ],
    "total": 1,
    "limit": 100,
    "offset": 0
  }
}
```

#### 2. 获取规则详情

获取特定规则的完整信息。

```http
GET /api/v1/rules/{rule_id}
```

**路径参数**:
- `rule_id` (string): 规则ID

**响应示例**:
```json
{
  "success": true,
  "data": {
    "id": "temp_monitor",
    "name": "温度监控",
    "description": "监控温度传感器数据",
    "enabled": true,
    "priority": 100,
    "version": 1,
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
          "window_size": 10,
          "functions": ["avg", "max", "min"],
          "group_by": ["device_id"],
          "ttl": "10m"
        }
      }
    ],
    "tags": {
      "category": "monitoring",
      "type": "temperature"
    },
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

#### 3. 创建规则

创建新的规则。

```http
POST /api/v1/rules
```

**请求体示例**:
```json
{
  "id": "new_rule",
  "name": "新规则",
  "description": "规则描述",
  "enabled": true,
  "priority": 100,
  "conditions": {
    "type": "simple",
    "field": "value",
    "operator": "gt",
    "value": 30
  },
  "actions": [
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
  ],
  "tags": {
    "category": "alert"
  }
}
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "id": "new_rule",
    "version": 1,
    "created_at": "2024-01-01T12:00:00Z"
  },
  "message": "规则创建成功"
}
```

#### 4. 更新规则

更新现有规则。

```http
PUT /api/v1/rules/{rule_id}
```

**路径参数**:
- `rule_id` (string): 规则ID

**请求体**: 与创建规则相同的格式

**响应示例**:
```json
{
  "success": true,
  "data": {
    "id": "new_rule",
    "version": 2,
    "updated_at": "2024-01-01T12:30:00Z"
  },
  "message": "规则更新成功"
}
```

#### 5. 删除规则

删除指定规则。

```http
DELETE /api/v1/rules/{rule_id}
```

**路径参数**:
- `rule_id` (string): 规则ID

**响应示例**:
```json
{
  "success": true,
  "message": "规则删除成功"
}
```

#### 6. 启用/禁用规则

切换规则的启用状态。

```http
PATCH /api/v1/rules/{rule_id}/toggle
```

**路径参数**:
- `rule_id` (string): 规则ID

**请求体**:
```json
{
  "enabled": true
}
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "id": "new_rule",
    "enabled": true
  },
  "message": "规则状态已更新"
}
```

### 监控和统计 API 🆕

#### 1. 获取系统健康状态

获取规则引擎整体健康状态。

```http
GET /api/v1/rules/health
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "status": "healthy",
    "uptime": "2h30m",
    "components": {
      "rule_manager": "healthy",
      "nats_connection": "healthy", 
      "worker_pool": "healthy",
      "aggregate_manager": "healthy"
    },
    "resource_usage": {
      "memory_usage": "45%",
      "cpu_usage": "23%",
      "goroutines": 156,
      "active_rules": 25
    },
    "last_check": "2024-01-01T12:00:00Z"
  }
}
```

#### 2. 获取规则执行统计

获取规则执行的统计信息。

```http
GET /api/v1/rules/metrics
```

**查询参数**:
- `time_range` (string, 可选): 时间范围，如 "1h", "24h", "7d"
- `rule_id` (string, 可选): 特定规则ID
- `group_by` (string, 可选): 分组维度，如 "rule", "action", "hour"

**响应示例**:
```json
{
  "success": true,
  "data": {
    "summary": {
      "total_rules": 25,
      "enabled_rules": 23,
      "total_data_points": 1250000,
      "total_rule_matches": 45000,
      "total_actions_executed": 12500,
      "avg_processing_time_ms": 2.5,
      "error_rate": 0.001
    },
    "performance": {
      "throughput_per_second": 250,
      "p50_latency_ms": 1.2,
      "p95_latency_ms": 4.8,
      "p99_latency_ms": 12.5,
      "memory_usage_mb": 145,
      "cpu_usage_percent": 23
    },
    "rule_stats": [
      {
        "rule_id": "temp_monitor",
        "rule_name": "温度监控",
        "matches": 1500,
        "actions_executed": 450,
        "avg_execution_time_ms": 3.2,
        "last_execution": "2024-01-01T12:00:00Z",
        "error_count": 0
      }
    ],
    "action_stats": [
      {
        "action_type": "aggregate",
        "execution_count": 8500,
        "avg_execution_time_ms": 1.8,
        "error_count": 2,
        "success_rate": 0.9998
      }
    ],
    "time_range": "1h",
    "generated_at": "2024-01-01T12:00:00Z"
  }
}
```

#### 3. 获取错误信息

获取规则执行过程中的错误信息。

```http
GET /api/v1/rules/errors
```

**查询参数**:
- `limit` (int, 可选): 限制返回数量，默认50
- `error_type` (string, 可选): 错误类型过滤
- `error_level` (string, 可选): 错误级别过滤
- `rule_id` (string, 可选): 特定规则ID
- `since` (string, 可选): 时间过滤，ISO格式

**响应示例**:
```json
{
  "success": true,
  "data": {
    "errors": [
      {
        "id": "error_001",
        "rule_id": "temp_monitor",
        "rule_name": "温度监控",
        "error_type": "execution",
        "error_level": "warning",
        "message": "数据点解析失败",
        "details": "无法解析JSON数据: unexpected end of JSON input",
        "context": {
          "action_type": "transform",
          "device_id": "sensor_001",
          "data_point": "{\"device_id\":\"sensor_001\",\"key\""
        },
        "timestamp": "2024-01-01T11:58:30Z",
        "retry_count": 1,
        "resolved": false
      }
    ],
    "summary": {
      "total_errors": 12,
      "by_type": {
        "validation": 3,
        "execution": 6,
        "timeout": 2,
        "configuration": 1
      },
      "by_level": {
        "warning": 8,
        "error": 3,
        "critical": 1
      }
    },
    "limit": 50,
    "has_more": false
  }
}
```

#### 4. 获取聚合状态

获取聚合动作的状态信息。

```http
GET /api/v1/rules/aggregates
```

**查询参数**:
- `rule_id` (string, 可选): 特定规则ID
- `group_key` (string, 可选): 特定分组键
- `active_only` (boolean, 可选): 只返回活跃状态

**响应示例**:
```json
{
  "success": true,
  "data": {
    "aggregates": [
      {
        "rule_id": "temp_monitor",
        "group_key": "device_id=sensor_001,key=temperature",
        "window_size": 10,
        "current_count": 8,
        "statistics": {
          "avg": 25.5,
          "max": 30.2,
          "min": 20.1,
          "sum": 204.0,
          "stddev": 2.3,
          "count": 8
        },
        "created_at": "2024-01-01T11:50:00Z",
        "last_updated": "2024-01-01T11:59:30Z",
        "ttl_expires_at": "2024-01-01T12:00:00Z"
      }
    ],
    "summary": {
      "total_aggregates": 156,
      "active_aggregates": 142,
      "memory_usage_mb": 12.5,
      "oldest_created": "2024-01-01T10:30:00Z"
    }
  }
}
```

### 调试和管理 API

#### 1. 测试规则

测试规则条件和动作，不实际执行。

```http
POST /api/v1/rules/test
```

**请求体**:
```json
{
  "rule": {
    "conditions": {
      "type": "simple",
      "field": "value",
      "operator": "gt", 
      "value": 30
    },
    "actions": [
      {
        "type": "alert",
        "config": {
          "level": "warning",
          "message": "测试告警"
        }
      }
    ]
  },
  "data_point": {
    "device_id": "test_device",
    "key": "temperature",
    "value": 35.5,
    "timestamp": "2024-01-01T12:00:00Z"
  }
}
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "condition_result": {
      "matched": true,
      "evaluation_time_ms": 0.5,
      "details": "value (35.5) > 30 = true"
    },
    "action_results": [
      {
        "action_type": "alert",
        "would_execute": true,
        "config_valid": true,
        "estimated_time_ms": 2.1,
        "preview": {
          "level": "warning",
          "message": "测试告警",
          "channels": ["console"]
        }
      }
    ],
    "total_time_ms": 2.6
  }
}
```

#### 2. 验证规则配置

验证规则配置的正确性。

```http
POST /api/v1/rules/validate
```

**请求体**:
```json
{
  "id": "test_rule",
  "name": "测试规则",
  "conditions": {
    "type": "expression",
    "expression": "value > 30 && device_id == 'sensor_001'"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window_size": 10,
        "functions": ["avg", "max"],
        "group_by": ["device_id"]
      }
    }
  ]
}
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "valid": true,
    "validation_results": {
      "structure": {
        "valid": true,
        "errors": []
      },
      "conditions": {
        "valid": true,
        "expression_parsed": true,
        "syntax_errors": []
      },
      "actions": [
        {
          "action_type": "aggregate",
          "valid": true,
          "config_errors": []
        }
      ]
    },
    "warnings": [],
    "suggestions": [
      "考虑为规则添加描述信息",
      "建议设置聚合TTL避免内存泄漏"
    ]
  }
}
```

#### 3. 重新加载规则

重新加载指定文件或所有规则文件。

```http
POST /api/v1/rules/reload
```

**请求体**:
```json
{
  "file_path": "/path/to/rules/file.json",
  "force": false
}
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "reloaded_files": [
      "/path/to/rules/file.json"
    ],
    "rules_loaded": 5,
    "rules_updated": 2,
    "rules_added": 1,
    "rules_removed": 0,
    "errors": []
  },
  "message": "规则重新加载完成"
}
```

#### 4. 清理聚合状态

清理过期或特定的聚合状态。

```http
POST /api/v1/rules/aggregates/cleanup
```

**请求体**:
```json
{
  "rule_id": "temp_monitor",
  "group_key": "device_id=sensor_001",
  "force": false,
  "older_than": "1h"
}
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "cleaned_count": 25,
    "memory_freed_mb": 2.1,
    "remaining_aggregates": 131
  },
  "message": "聚合状态清理完成"
}
```

### 配置管理 API

#### 1. 获取引擎配置

获取规则引擎的当前配置。

```http
GET /api/v1/rules/config
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "enabled": true,
    "worker_pool_size": 4,
    "batch_size": 100,
    "batch_timeout": "5s",
    "max_rules": 1000,
    "rule_directories": [
      "/etc/iot-gateway/rules",
      "./rules"
    ],
    "monitoring": {
      "enabled": true,
      "metrics_retention": "24h",
      "error_retention": "7d"
    },
    "performance": {
      "enable_parallel_processing": true,
      "enable_object_pooling": true,
      "gc_interval": "30s"
    }
  }
}
```

#### 2. 更新引擎配置

更新规则引擎配置（需要重启）。

```http
PUT /api/v1/rules/config
```

**请求体**:
```json
{
  "worker_pool_size": 8,
  "batch_size": 200,
  "monitoring": {
    "enabled": true,
    "metrics_retention": "48h"
  }
}
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "updated_fields": [
      "worker_pool_size",
      "batch_size", 
      "monitoring.metrics_retention"
    ],
    "restart_required": true
  },
  "message": "配置更新成功，需要重启服务生效"
}
```

## 错误响应格式

所有API的错误响应都遵循统一格式：

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "请求参数验证失败",
    "details": {
      "field": "window_size",
      "reason": "必须是正整数"
    }
  },
  "request_id": "req_123456789"
}
```

### 常见错误代码

| 错误代码 | HTTP状态码 | 说明 |
|----------|------------|------|
| `VALIDATION_ERROR` | 400 | 请求参数验证失败 |
| `RULE_NOT_FOUND` | 404 | 规则不存在 |
| `RULE_ALREADY_EXISTS` | 409 | 规则已存在 |
| `INVALID_CONFIGURATION` | 400 | 配置格式错误 |
| `EXECUTION_ERROR` | 500 | 执行错误 |
| `SERVICE_UNAVAILABLE` | 503 | 服务不可用 |
| `RATE_LIMITED` | 429 | 请求频率限制 |
| `UNAUTHORIZED` | 401 | 未授权访问 |
| `FORBIDDEN` | 403 | 权限不足 |

## 使用示例

### 监控规则执行情况

```bash
# 获取系统健康状态
curl http://localhost:8081/api/v1/rules/health

# 获取最近1小时的执行统计
curl "http://localhost:8081/api/v1/rules/metrics?time_range=1h"

# 获取最近错误信息
curl "http://localhost:8081/api/v1/rules/errors?limit=10&error_level=error"
```

### 规则管理操作

```bash
# 获取所有启用的规则
curl "http://localhost:8081/api/rules?enabled=true"

# 创建新规则
curl -X POST http://localhost:8081/api/rules \
  -H "Content-Type: application/json" \
  -d @new_rule.json

# 禁用特定规则
curl -X PATCH http://localhost:8081/api/v1/rules/temp_monitor/toggle \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}'
```

### 调试和测试

```bash
# 验证规则配置
curl -X POST http://localhost:8081/api/v1/rules/validate \
  -H "Content-Type: application/json" \
  -d @test_rule.json

# 测试规则执行
curl -X POST http://localhost:8081/api/v1/rules/test \
  -H "Content-Type: application/json" \
  -d @test_data.json
```

## 编程接口参考 🆕

### 条件评估API

### Evaluator 接口

```go
type Evaluator interface {
    Evaluate(condition *Condition, point model.Point) (bool, error)
    RegisterFunction(name string, fn Function) error
}
```

#### 方法说明

1. **Evaluate**
   - 功能：评估条件
   - 参数：
     - condition *Condition
     - point model.Point
   - 返回：(bool, error)
   - 说明：评估数据点是否满足条件

2. **RegisterFunction**
   - 功能：注册自定义函数
   - 参数：
     - name string
     - fn Function
   - 返回：error
   - 说明：注册自定义评估函数

### Function 接口

```go
type Function interface {
    Name() string
    Call(args []interface{}) (interface{}, error)
}
```

## 动作处理API

### ActionHandler 接口

```go
type ActionHandler interface {
    Name() string
    Execute(ctx context.Context, point model.Point, rule *Rule, config map[string]interface{}) (*ActionResult, error)
}
```

#### 方法说明

1. **Name**
   - 功能：获取处理器名称
   - 参数：无
   - 返回：string
   - 说明：返回动作处理器的名称

2. **Execute**
   - 功能：执行动作
   - 参数：
     - ctx context.Context
     - point model.Point
     - rule *Rule
     - config map[string]interface{}
   - 返回：(*ActionResult, error)
   - 说明：执行动作处理逻辑

## 数据结构

### Rule

```go
type Rule struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Description string            `json:"description"`
    Enabled     bool              `json:"enabled"`
    Priority    int               `json:"priority"`
    Version     int              `json:"version"`
    Conditions  *Condition        `json:"conditions"`
    Actions     []Action          `json:"actions"`
    Tags        map[string]string `json:"tags,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

### Condition

```go
type Condition struct {
    Type       string       `json:"type,omitempty"`
    Field      string       `json:"field,omitempty"`
    Operator   string       `json:"operator,omitempty"`
    Value      interface{}  `json:"value,omitempty"`
    Expression string       `json:"expression,omitempty"`
    Script     string       `json:"script,omitempty"`
    And        []*Condition `json:"and,omitempty"`
    Or         []*Condition `json:"or,omitempty"`
    Not        *Condition   `json:"not,omitempty"`
}
```

### Action

```go
type Action struct {
    Type   string                 `json:"type"`
    Config map[string]interface{} `json:"config"`
}
```

### Point

```go
type Point struct {
    DeviceID  string                 `json:"device_id"`
    Key       string                 `json:"key"`
    Value     interface{}            `json:"value"`
    Timestamp time.Time              `json:"timestamp"`
    Quality   int                    `json:"quality"`
    Tags      map[string]string      `json:"tags,omitempty"`
    Metadata  map[string]interface{} `json:"metadata,omitempty"`
}
```

## HTTP API

### 规则管理

#### 1. 获取规则列表

```http
GET /api/rules
```

参数：
- page: 页码
- page_size: 每页数量
- enabled: 是否启用
- tag: 标签过滤

响应：
```json
{
    "total": 100,
    "rules": [
        {
            "id": "rule_001",
            "name": "温度监控",
            "enabled": true,
            "priority": 100
        }
    ]
}
```

#### 2. 获取规则详情

```http
GET /api/v1/rules/{id}
```

响应：
```json
{
    "id": "rule_001",
    "name": "温度监控",
    "description": "监控温度变化",
    "enabled": true,
    "priority": 100,
    "conditions": {},
    "actions": []
}
```

#### 3. 创建规则

```http
POST /api/rules
```

请求体：
```json
{
    "name": "新规则",
    "description": "规则描述",
    "conditions": {},
    "actions": []
}
```

#### 4. 更新规则

```http
PUT /api/v1/rules/{id}
```

#### 5. 删除规则

```http
DELETE /api/v1/rules/{id}
```

#### 6. 启用/禁用规则

```http
POST /api/v1/rules/{id}/enable
POST /api/v1/rules/{id}/disable
```

### 规则验证

```http
POST /api/v1/rules/validate
```

请求体：
```json
{
    "conditions": {},
    "actions": []
}
```

响应：
```json
{
    "valid": true,
    "errors": []
}
```

### 规则测试

```http
POST /api/v1/rules/test
```

请求体：
```json
{
    "rule": {
        "conditions": {},
        "actions": []
    },
    "points": [
        {
            "device_id": "device_001",
            "key": "temperature",
            "value": 25.5
        }
    ]
}
```

响应：
```json
{
    "results": [
        {
            "matched": true,
            "actions": [
                {
                    "type": "alert",
                    "success": true
                }
            ]
        }
    ]
}
```

通过这些API接口，您可以全面管理和监控规则引擎的运行状态，确保系统的高效运行和及时问题排查。最新优化版本提供了完整的监控和调试能力，大大提升了运维效率和问题定位能力。 