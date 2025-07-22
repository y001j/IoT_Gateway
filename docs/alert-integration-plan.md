# Alert System Integration Plan

## 集成目标

将Alert功能深度集成到IoT Gateway系统中，实现：
1. 规则引擎统一处理告警规则
2. 保持独立的告警生命周期管理
3. 实时告警触发和通知
4. Web UI统一管理界面

## 架构设计

### 整体架构
```
┌─────────────────────────────────────────────────────────┐
│                    Web UI Layer                         │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────┐ │
│  │  Dashboard  │ │    Rules    │ │       Alerts        │ │
│  │             │ │             │ │  (管理、统计、历史)    │ │
│  └─────────────┘ └─────────────┘ └─────────────────────┘ │
├─────────────────────────────────────────────────────────┤
│                   API Gateway                           │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────┐ │
│  │Rule Engine  │ │Alert Service│ │Notification Service │ │
│  │(条件评估)    │ │(生命周期)    │ │   (通知发送)         │ │
│  └─────────────┘ └─────────────┘ └─────────────────────┘ │
├─────────────────────────────────────────────────────────┤
│                 NATS Message Bus                        │
│  iot.data.* │ iot.rules.* │ iot.alerts.* │ iot.notify.* │
├─────────────────────────────────────────────────────────┤
│               Data Collection Layer                     │
│    Adapters (Modbus, MQTT, HTTP) → Sinks               │
└─────────────────────────────────────────────────────────┘
```

### 数据流设计
```
Data → Rule Engine → Alert Action → Alert Service → Notification Service
                 ↓
            NATS: iot.alerts.triggered
                 ↓
         WebSocket → Frontend (实时更新)
```

## 实施计划

### Phase 1: 规则引擎Alert Action集成
### Phase 2: Alert Service事件驱动重构
### Phase 3: 实时通知系统
### Phase 4: Web UI统一管理
### Phase 5: 高级功能扩展