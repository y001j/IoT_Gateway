# 企业级告警功能配置指南

## 概述

BuiltinAlertHandler已经为企业级功能预留了接口，包括邮件、短信、重试机制等。这些功能目前以占位符方式实现，可以在需要时进行开发。

## 当前支持的通道

### ✅ 已实现通道

#### 1. Console通道
```json
{
  "type": "alert",
  "config": {
    "level": "warning",
    "message": "设备{{.DeviceID}}温度异常: {{.Value}}°C",
    "channels": [
      {
        "type": "console"
      }
    ]
  }
}
```

#### 2. Webhook通道
```json
{
  "type": "alert", 
  "config": {
    "level": "error",
    "message": "设备{{.DeviceID}}故障: {{.Value}}",
    "channels": [
      {
        "type": "webhook",
        "config": {
          "url": "https://your-webhook.com/alerts",
          "token": "your-auth-token"
        }
      }
    ]
  }
}
```

#### 3. NATS通道
```json
{
  "type": "alert",
  "config": {
    "level": "critical", 
    "message": "关键设备{{.DeviceID}}离线",
    "channels": [
      {
        "type": "nats",
        "config": {
          "subject": "alerts.critical"
        }
      }
    ]
  }
}
```

### 🚧 待实现通道 (占位符)

#### 4. 邮件通道 (Email)
```json
{
  "type": "alert",
  "config": {
    "level": "warning",
    "message": "IoT设备告警通知",
    "channels": [
      {
        "type": "email",
        "config": {
          "smtp_host": "smtp.example.com",
          "smtp_port": 587,
          "username": "alert@company.com",
          "password": "app_password",
          "from": "IoT Gateway <alert@company.com>",
          "to": ["admin@company.com", "ops@company.com"],
          "subject": "【{{.Level}}】设备{{.DeviceID}}告警",
          "template": "html_email_template"
        }
      }
    ]
  }
}
```

**实现优先级**: 高  
**预期工作量**: 2-3天  
**依赖**: SMTP客户端库 (gomail/net/smtp)

#### 5. 短信通道 (SMS)
```json
{
  "type": "alert",
  "config": {
    "level": "critical",
    "message": "紧急告警",
    "channels": [
      {
        "type": "sms",
        "config": {
          "provider": "aliyun",
          "access_key": "your_access_key",
          "secret_key": "your_secret_key", 
          "sign_name": "IoT监控",
          "template_code": "SMS_001",
          "phone_numbers": ["+86138****8888"],
          "template_params": {
            "device": "{{.DeviceID}}",
            "level": "{{.Level}}"
          }
        }
      }
    ]
  }
}
```

**实现优先级**: 中  
**预期工作量**: 3-5天  
**依赖**: 短信服务商SDK (阿里云/腾讯云/Twilio)

## 高级功能配置

### 节流控制
```json
{
  "type": "alert",
  "config": {
    "level": "warning",
    "message": "设备{{.DeviceID}}温度告警",
    "throttle": "5m",
    "channels": [{"type": "console"}]
  }
}
```

### 多通道组合
```json
{
  "type": "alert",
  "config": {
    "level": "critical",
    "message": "设备{{.DeviceID}}严重故障",
    "channels": [
      {"type": "console"},
      {"type": "webhook", "config": {"url": "https://webhook.site/alert"}},
      {"type": "nats", "config": {"subject": "alerts.critical"}},
      {"type": "email", "config": {"to": ["admin@company.com"]}},
      {"type": "sms", "config": {"phone_numbers": ["+86138****8888"]}}
    ]
  }
}
```

## 待实现企业功能

### 1. 重试机制 (sendToChannelsWithRetry)

**功能描述**: 通道发送失败时自动重试

**配置示例**:
```json
{
  "type": "alert",
  "config": {
    "retry_enabled": true,
    "retry_count": 3,
    "retry_delay": "30s",
    "retry_strategy": "exponential_backoff"
  }
}
```

**实现优先级**: 中  
**预期工作量**: 2-3天

### 2. 故障转移 (enableChannelFailover)

**功能描述**: 主通道失败时自动切换备用通道

**配置示例**:
```json
{
  "type": "alert", 
  "config": {
    "failover_enabled": true,
    "primary_channels": [{"type": "webhook"}],
    "fallback_channels": [{"type": "email"}, {"type": "sms"}]
  }
}
```

**实现优先级**: 低  
**预期工作量**: 3-5天

### 3. 投递状态跟踪 (trackDeliveryStatus)

**功能描述**: 记录和查询告警投递状态

**功能特性**:
- 投递状态持久化
- 投递成功率统计
- 失败原因分析
- 历史查询API

**实现优先级**: 低  
**预期工作量**: 5-7天

### 4. 配置验证增强 (validateChannelConfig)

**功能描述**: 企业级配置验证和健康检查

**功能特性**:
- SMTP连接测试
- API端点验证
- 配置完整性检查
- 安全配置验证

**实现优先级**: 中  
**预期工作量**: 2-3天

## 开发指导

### 实现新通道的步骤

1. **在sendToChannels方法中添加case**:
```go
case "your_channel":
    err = h.sendYourChannelAlert(alert, channel.Config)
```

2. **实现具体的发送方法**:
```go
func (h *BuiltinAlertHandler) sendYourChannelAlert(alert *Alert, config map[string]interface{}) error {
    // 具体实现
    return nil
}
```

3. **添加配置验证**:
```go
func (h *BuiltinAlertHandler) validateYourChannelConfig(config map[string]interface{}) error {
    // 验证必需参数
    return nil
}
```

4. **编写单元测试**:
```go
func TestSendYourChannelAlert(t *testing.T) {
    // 测试用例
}
```

### 代码规范

- 所有新方法都要有详细的注释说明
- 错误处理要细致，提供有意义的错误信息
- 配置参数要有验证和默认值
- 要考虑并发安全和性能影响
- 要有对应的单元测试

### 优先级建议

1. **高优先级**: 邮件通道 (最常用的企业功能)
2. **中优先级**: 重试机制、配置验证增强
3. **低优先级**: 短信通道、故障转移、投递跟踪

## 测试方法

每个新功能都应该包含:

1. **单元测试**: 测试单个方法的功能
2. **集成测试**: 测试与规则引擎的集成
3. **端到端测试**: 测试实际的消息发送
4. **性能测试**: 确保不影响整体性能

## 总结

通过占位符方式，我们为BuiltinAlertHandler的企业级扩展提供了清晰的路径。开发团队可以根据业务需求的优先级来逐步实现这些功能，而不会影响现有的稳定功能。