# Forward 动作处理器

Forward动作处理器是IoT Gateway规则引擎的核心组件之一，负责将处理后的数据转发到各种目标系统。它支持多种转发协议和格式，提供了灵活的数据转换和路由能力。

## 功能特性

- **多协议支持**: NATS、HTTP、文件、MQTT
- **多格式支持**: JSON、XML、CSV、自定义模板
- **数据转换**: 字段过滤、映射、常量添加
- **异步处理**: 支持异步转发，提高性能
- **重试机制**: 可配置重试次数和超时时间
- **认证支持**: 支持多种认证方式
- **模板引擎**: 支持Go模板语法的自定义格式化

## 基本配置结构

```json
{
  "type": "forward",
  "config": {
    "targets": [
      {
        "name": "target_name",
        "type": "http|nats|file|mqtt",
        "enabled": true,
        "async": false,
        "timeout": "10s",
        "retry": 3,
        "config": {
          // 特定于转发类型的配置
        }
      }
    ],
    "add_rule_info": true,
    "add_timestamp": true,
    "extra_fields": {
      "key": "value"
    },
    "data_transform": {
      // 数据转换配置
    }
  }
}
```

## 转发目标类型

### 1. HTTP 转发

将数据发送到HTTP端点，支持多种认证方式和内容格式。

```json
{
  "name": "http_target",
  "type": "http",
  "enabled": true,
  "async": false,
  "timeout": "10s",
  "retry": 3,
  "config": {
    "url": "https://api.example.com/webhook",
    "method": "POST",
    "content_type": "application/json",
    "headers": {
      "X-Custom-Header": "value",
      "User-Agent": "IoT-Gateway"
    },
    "auth": {
      "type": "bearer|basic|api_key",
      "token": "bearer_token",
      "username": "user",
      "password": "pass",
      "key": "api_key",
      "header": "X-API-Key"
    },
    "template": "自定义模板（当content_type为text/plain时）"
  }
}
```

**认证类型**:
- `bearer`: Bearer Token认证
- `basic`: HTTP Basic认证
- `api_key`: API Key认证（默认使用X-API-Key头）

**支持的内容类型**:
- `application/json`: JSON格式（默认）
- `application/xml`: XML格式
- `text/csv`: CSV格式
- `text/plain`: 自定义模板格式

### 2. 文件转发

将数据写入本地文件，支持多种格式和追加模式。

```json
{
  "name": "file_target",
  "type": "file",
  "enabled": true,
  "config": {
    "path": "/var/log/iot-gateway/data.log",
    "format": "json|csv|xml|text",
    "append": true,
    "template": "{{.timestamp}} [{{.device_id}}] {{.key}}={{.value}}\n"
  }
}
```

**格式说明**:
- `json`: JSON格式，每行一个JSON对象
- `csv`: CSV格式，包含表头
- `xml`: XML格式
- `text`: 自定义模板格式

### 3. MQTT 转发

将数据发布到MQTT broker，支持QoS、retain和TLS。

```json
{
  "name": "mqtt_target",
  "type": "mqtt",
  "enabled": true,
  "async": true,
  "timeout": "15s",
  "retry": 2,
  "config": {
    "broker": "tcp://localhost:1883",
    "topic": "iot/sensors/{{.device_id}}/{{.key}}",
    "client_id": "iot-gateway-forward",
    "qos": 1,
    "retain": false,
    "format": "json|xml|csv|text",
    "username": "mqtt_user",
    "password": "mqtt_pass",
    "tls": false,
    "template": "自定义消息模板"
  }
}
```

**QoS级别**:
- `0`: 最多一次投递
- `1`: 至少一次投递
- `2`: 恰好一次投递

### 4. NATS 转发

将数据发布到NATS消息系统。

```json
{
  "name": "nats_target",
  "type": "nats",
  "enabled": true,
  "async": false,
  "config": {
    "subject": "iot.processed.{{.device_id}}"
  }
}
```

## 数据转换

Forward动作支持灵活的数据转换功能，可以在转发前对数据进行处理。

```json
{
  "data_transform": {
    "format": "json|csv|xml|custom",
    "template": "Go模板字符串",
    "fields": ["device_id", "key", "value"],
    "exclude": ["internal_field"],
    "mapping": {
      "old_field": "new_field",
      "device_id": "sensor_id"
    },
    "constants": {
      "version": "1.0",
      "source": "rule-engine"
    }
  }
}
```

**转换步骤**:
1. 字段过滤（如果指定了fields）
2. 排除字段（exclude中的字段）
3. 字段映射（重命名字段）
4. 添加常量字段

## 模板语法

Forward动作支持Go模板语法，可以在以下地方使用：

- HTTP请求的URL和模板
- 文件路径和内容模板
- MQTT主题和消息模板
- NATS主题
- 自定义格式模板

**可用变量**:
- `{{.device_id}}`: 设备ID
- `{{.key}}`: 数据键名
- `{{.value}}`: 数据值
- `{{.type}}`: 数据类型
- `{{.timestamp}}`: 时间戳
- `{{.tags.xxx}}`: 标签值
- `{{.rule_id}}`: 规则ID（如果启用add_rule_info）
- `{{.rule_name}}`: 规则名称

**示例模板**:
```
{{.timestamp}} [{{.device_id}}] {{.key}}={{.value}} ({{.type}})
```

## 错误处理

Forward动作提供了完善的错误处理机制：

1. **超时控制**: 每个目标可以设置独立的超时时间
2. **重试机制**: 失败时自动重试，支持指数退避
3. **部分失败**: 即使某些目标失败，其他目标仍会继续处理
4. **错误日志**: 详细的错误信息和调试日志

## 性能优化

1. **异步处理**: 设置`async: true`启用异步转发
2. **连接复用**: MQTT和HTTP客户端自动复用连接
3. **模板缓存**: 模板编译结果会被缓存
4. **批量操作**: 文件写入使用缓冲区优化

## 使用示例

### 多目标转发示例

```json
{
  "id": "multi_target_forward",
  "name": "多目标转发",
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "temperature"
  },
  "actions": [
    {
      "type": "forward",
      "config": {
        "targets": [
          {
            "name": "api_server",
            "type": "http",
            "config": {
              "url": "https://api.example.com/data",
              "method": "POST",
              "auth": {
                "type": "bearer",
                "token": "your-token"
              }
            }
          },
          {
            "name": "local_log",
            "type": "file",
            "config": {
              "path": "/var/log/temperature.json",
              "format": "json",
              "append": true
            }
          },
          {
            "name": "mqtt_broker",
            "type": "mqtt",
            "async": true,
            "config": {
              "broker": "tcp://localhost:1883",
              "topic": "sensors/{{.device_id}}/temperature",
              "qos": 1
            }
          }
        ],
        "add_rule_info": true,
        "data_transform": {
          "constants": {
            "unit": "celsius",
            "source": "iot-gateway"
          }
        }
      }
    }
  ]
}
```

### 条件转发示例

```json
{
  "id": "conditional_forward",
  "name": "条件转发",
  "conditions": {
    "type": "and",
    "conditions": [
      {
        "type": "simple",
        "field": "key",
        "operator": "eq",
        "value": "alert"
      },
      {
        "type": "simple",
        "field": "value",
        "operator": "eq",
        "value": "critical"
      }
    ]
  },
  "actions": [
    {
      "type": "forward",
      "config": {
        "targets": [
          {
            "name": "emergency_webhook",
            "type": "http",
            "config": {
              "url": "https://alerts.example.com/emergency",
              "method": "POST",
              "content_type": "application/json"
            }
          },
          {
            "name": "alert_file",
            "type": "file",
            "config": {
              "path": "/var/log/critical_alerts.log",
              "format": "text",
              "template": "{{.timestamp}} CRITICAL ALERT: {{.device_id}} - {{.value}}\n",
              "append": true
            }
          }
        ]
      }
    }
  ]
}
```

## 最佳实践

1. **合理设置超时**: 根据网络环境和目标系统响应时间设置合适的超时值
2. **使用异步转发**: 对于非关键路径的转发，启用异步模式提高性能
3. **配置重试策略**: 为不稳定的网络环境配置适当的重试次数
4. **监控转发状态**: 通过日志和指标监控转发成功率
5. **数据转换优化**: 只转换必要的字段，减少处理开销
6. **文件路径管理**: 使用合理的文件路径和轮转策略避免磁盘空间问题

## 故障排除

### 常见问题

1. **HTTP转发失败**
   - 检查URL是否正确
   - 验证认证信息
   - 确认网络连通性

2. **文件写入失败**
   - 检查文件路径权限
   - 确认磁盘空间充足
   - 验证目录是否存在

3. **MQTT连接失败**
   - 检查broker地址和端口
   - 验证认证信息
   - 确认网络防火墙设置

4. **模板解析错误**
   - 检查模板语法
   - 确认变量名称正确
   - 验证数据字段存在

### 调试技巧

1. 启用详细日志记录
2. 使用小数据集测试配置
3. 逐步增加转发目标
4. 监控系统资源使用情况 