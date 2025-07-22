# Alert System Architecture Proposal

## 当前问题分析

Alert Rule 独立实现 vs 集成到规则引擎的权衡分析。

## 建议的混合架构

### 方案一：Alert Rule 作为规则引擎的专门动作类型（推荐）

```go
// 在规则引擎中扩展 Action 类型
type Action struct {
    Type    string                 `json:"type" yaml:"type"`  // "alert", "transform", "filter", etc.
    Config  map[string]interface{} `json:"config" yaml:"config"`
    // Alert专用配置
    AlertConfig *AlertActionConfig `json:"alert_config,omitempty" yaml:"alert_config,omitempty"`
}

type AlertActionConfig struct {
    Level               string   `json:"level"`                // info, warning, error, critical
    NotificationChannels []string `json:"notification_channels"` // 通知渠道ID列表
    Throttle            string   `json:"throttle,omitempty"`    // 节流时间
    Template            string   `json:"template,omitempty"`    // 消息模板
}
```

### 架构优势

1. **统一的条件引擎**: 复用规则引擎的条件评估逻辑
2. **专业化的告警管理**: 独立的告警生命周期和通知系统
3. **减少代码重复**: 共享条件解析、验证、测试逻辑

### 系统分层

```
┌─────────────────────────────────────┐
│           Web UI Layer              │
├─────────────────────────────────────┤
│         Alert Management            │
│    (告警列表、确认、解决、统计)        │
├─────────────────────────────────────┤
│        Notification System          │
│     (通知渠道、消息发送、模板)         │
├─────────────────────────────────────┤
│         Rule Engine Core            │
│  (条件评估、动作执行、Alert Action)    │
├─────────────────────────────────────┤
│            Data Pipeline            │
│       (数据收集、实时处理)            │
└─────────────────────────────────────┘
```

## 重构建议

### 1. 保持当前告警服务的管理功能
- Alert CRUD 操作
- Alert 状态管理 (active -> acknowledged -> resolved)
- Alert 统计和历史

### 2. 将 Alert Rule 合并到规则引擎
```go
// 规则引擎中的告警规则示例
{
  "id": "cpu-alert-rule",
  "name": "CPU使用率告警",
  "conditions": {
    "type": "simple",
    "field": "cpu_usage",
    "operator": "gt",
    "value": 80.0
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "CPU使用率过高: {{cpu_usage}}%",
        "notification_channels": ["email-ops", "webhook-slack"],
        "throttle": "5m"
      }
    }
  ]
}
```

### 3. Alert Service 监听规则引擎事件
```go
type AlertService interface {
    // 现有的告警管理功能
    GetAlerts(req *AlertListRequest) ([]Alert, int, error)
    AcknowledgeAlert(id string, userID string, comment string) error
    
    // 新增：处理规则引擎产生的告警
    HandleRuleAlert(alert *rules.Alert) error
    
    // 保持通知渠道管理
    GetNotificationChannels() ([]NotificationChannel, error)
}
```

## 实施步骤

### 阶段一：保持当前实现
- 当前的独立 Alert Rule 继续工作
- 验证告警管理功能的完整性

### 阶段二：渐进式集成
1. 在规则引擎中添加 "alert" 动作类型
2. Alert Service 同时支持两种告警来源
3. 前端提供统一的规则创建界面

### 阶段三：统一架构
1. 迁移现有告警规则到规则引擎
2. 移除重复的告警规则管理代码
3. 优化性能和用户体验

## 架构优势总结

### 独立告警系统优势
- ✅ 专门的告警生命周期管理
- ✅ 丰富的通知渠道配置
- ✅ 告警统计和分析功能
- ✅ 清晰的用户界面和API

### 集成到规则引擎优势
- ✅ 统一的条件表达式语法
- ✅ 复用条件验证和测试逻辑
- ✅ 减少配置复杂度
- ✅ 更好的性能（统一处理流程）

### 混合架构优势
- ✅ 结合两者优势
- ✅ 渐进式迁移路径
- ✅ 最小化用户影响
- ✅ 保持系统稳定性

## 结论

建议采用混合架构，将告警规则的**条件定义**集成到规则引擎中，保持告警的**生命周期管理**和**通知系统**独立。这样既能减少重复代码，又能保持各模块的专业性。