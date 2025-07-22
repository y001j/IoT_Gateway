# IoT Gateway NATS 消息总线架构

## 概述

NATS是IoT Gateway的核心消息总线，所有模块间的通信都通过NATS进行。规则引擎作为数据处理流水线的中心环节，通过NATS与其他模块进行事件驱动的异步通信。

## 系统架构

### 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           IoT Gateway 系统架构                                │
│                                                                             │
│  ┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐        │
│  │  Southbound     │    │   Plugin         │    │  Rule Engine    │        │
│  │  Adapters       │───▶│   Manager        │───▶│                 │        │
│  │                 │    │                  │    │                 │        │
│  │ • Modbus        │    │ • ISP Protocol   │    │ • Alert         │        │
│  │ • MQTT Sub      │    │ • Data Routing   │    │ • Transform     │        │
│  │ • HTTP          │    │ • Format Convert │    │ • Filter        │        │
│  │ • Mock          │    │ • Validation     │    │ • Aggregate     │        │
│  └─────────────────┘    └──────────────────┘    │ • Forward       │        │
│                                                  └─────────────────┘        │
│                                                           │                 │
│                                                           ▼                 │
│                          ┌─────────────────────────────────────────────────┐│
│                          │            NATS Message Bus                     ││
│                          │                                                 ││
│                          │  ┌─────────────────────────────────────────────┐││
│                          │  │            消息主题 (Subjects)              │││
│                          │  │                                             │││
│                          │  │ • iot.raw.*        - 原始数据               │││
│                          │  │ • iot.processed.*  - 处理后数据             │││
│                          │  │ • iot.rules.*      - 规则处理结果           │││
│                          │  │ • iot.alerts.*     - 报警消息               │││
│                          │  │ • iot.aggregated.* - 聚合数据               │││
│                          │  │ • iot.errors.*     - 错误消息               │││
│                          │  │ • iot.metrics.*    - 系统指标               │││
│                          │  └─────────────────────────────────────────────┘││
│                          └─────────────────────────────────────────────────┘│
│                                                           │                 │
│                                                           ▼                 │
│  ┌─────────────────┐                            ┌─────────────────┐        │
│  │  Northbound     │◀───────────────────────────│   Data Sinks    │        │
│  │  Sinks          │                            │                 │        │
│  │                 │                            │ • Console       │        │
│  │ • InfluxDB      │                            │ • JetStream     │        │
│  │ • Redis         │                            │ • MQTT Pub      │        │
│  │ • WebSocket     │                            │ • WebSocket     │        │
│  │ • MQTT          │                            │ • File          │        │
│  └─────────────────┘                            └─────────────────┘        │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 数据流向

1. **数据采集**: Southbound Adapters 从设备采集原始数据
2. **数据预处理**: Plugin Manager 处理数据格式转换和验证
3. **规则处理**: Rule Engine 根据规则对数据进行处理
4. **数据输出**: Northbound Sinks 将处理结果发送到目标系统

## NATS 消息主题设计

### 主题层次结构

```
iot.
├── raw.                    # 原始数据
│   ├── modbus.{device_id}  # Modbus设备原始数据
│   ├── mqtt.{device_id}    # MQTT设备原始数据
│   └── http.{device_id}    # HTTP设备原始数据
│
├── processed.              # 处理后数据
│   ├── {device_id}.{key}   # 按设备和数据键分类
│   └── batch.{device_id}   # 批量处理数据
│
├── rules.                  # 规则处理结果
│   ├── {rule_id}.success   # 规则执行成功
│   ├── {rule_id}.failed    # 规则执行失败
│   └── {device_id}.{key}   # 按设备和数据键分类
│
├── alerts.                 # 报警消息
│   ├── {level}.{device_id} # 按报警级别和设备分类
│   └── {category}.*        # 按报警类别分类
│
├── aggregated.             # 聚合数据
│   ├── {device_id}.{key}   # 按设备和数据键分类
│   └── {window}.{function} # 按时间窗口和聚合函数分类
│
├── errors.                 # 错误消息
│   ├── {module}.{error_type} # 按模块和错误类型分类
│   └── {device_id}.errors    # 按设备分类的错误
│
└── metrics.                # 系统指标
    ├── performance.*       # 性能指标
    └── health.*           # 健康状态
```

### 消息格式

所有NATS消息都采用JSON格式，包含统一的元数据：

```json
{
  "timestamp": "2024-01-01T12:00:00Z",
  "source": "rule_engine",
  "version": "1.0.0",
  "trace_id": "uuid-trace-id",
  "data": {
    // 具体的数据内容
  }
}
```

## 规则引擎与NATS集成

### 消息订阅

规则引擎订阅以下主题接收数据：

```go
// 订阅处理后的数据
nc.Subscribe("iot.processed.*", func(msg *nats.Msg) {
    // 解析消息
    var point model.Point
    if err := json.Unmarshal(msg.Data, &point); err != nil {
        log.Error().Err(err).Msg("解析数据点失败")
        return
    }
    
    // 处理数据点
    ruleEngine.ProcessPoint(point)
})

// 订阅批量数据
nc.Subscribe("iot.processed.batch.*", func(msg *nats.Msg) {
    // 批量处理逻辑
})
```

### 消息发布

规则引擎根据处理结果发布到不同主题：

```go
// 发布规则处理结果
func (r *RuleEngine) publishResult(result ProcessedPoint) {
    // 成功处理的数据
    if result.Success {
        subject := fmt.Sprintf("iot.rules.%s", result.RuleID)
        r.natsConn.Publish(subject, result.ToJSON())
    }
    
    // 报警消息
    for _, alert := range result.Alerts {
        subject := fmt.Sprintf("iot.alerts.%s.%s", alert.Level, alert.DeviceID)
        r.natsConn.Publish(subject, alert.ToJSON())
    }
    
    // 聚合数据
    for _, agg := range result.Aggregates {
        subject := fmt.Sprintf("iot.aggregated.%s.%s", agg.DeviceID, agg.Key)
        r.natsConn.Publish(subject, agg.ToJSON())
    }
}
```

## 核心模块NATS集成

### 1. Core Runtime

Core Runtime负责NATS服务器的启动和连接管理：

```go
// 启动嵌入式NATS服务器或连接外部服务器
func NewRuntime(cfgPath string) (*Runtime, error) {
    // 配置NATS连接
    natsURL := v.GetString("gateway.nats_url")
    
    if natsURL == "embedded" {
        // 启动嵌入式NATS服务器
        natsServer := startEmbeddedNATS()
        nc, _ := nats.Connect("nats://localhost:4222")
    } else {
        // 连接外部NATS服务器
        nc, _ := nats.Connect(natsURL)
    }
    
    // 创建JetStream上下文
    js, _ := nc.JetStream()
    
    return &Runtime{
        Bus: nc,
        Js:  js,
        NatsServer: natsServer,
    }
}
```

### 2. Plugin Manager

Plugin Manager通过NATS分发处理后的数据：

```go
// 发布处理后的数据点
func (m *Manager) publishDataPoint(point model.Point) {
    subject := fmt.Sprintf("iot.processed.%s.%s", point.DeviceID, point.Key)
    
    data, _ := json.Marshal(point)
    m.bus.Publish(subject, data)
    
    log.Debug().
        Str("subject", subject).
        Str("device_id", point.DeviceID).
        Str("key", point.Key).
        Msg("发布数据点到NATS")
}
```

### 3. Northbound Sinks

各种Sink通过NATS接收处理后的数据：

```go
// WebSocket Sink订阅规则处理结果
func (s *WebSocketSink) Start() error {
    s.natsSubscription, err = s.natsConn.Subscribe("iot.rules.*", func(msg *nats.Msg) {
        // 解析消息
        var result ProcessedPoint
        json.Unmarshal(msg.Data, &result)
        
        // 广播到WebSocket客户端
        s.broadcast(result)
    })
    
    return err
}
```

## 消息队列和持久化

### JetStream 流配置

```go
// 创建数据流
_, err := js.AddStream(&nats.StreamConfig{
    Name:     "IOT_DATA",
    Subjects: []string{"iot.processed.*", "iot.rules.*"},
    Storage:  nats.FileStorage,
    MaxAge:   24 * time.Hour,
})

// 创建消费者
_, err = js.AddConsumer("IOT_DATA", &nats.ConsumerConfig{
    Durable:   "rule_engine_consumer",
    AckPolicy: nats.AckExplicitPolicy,
})
```

### 消息持久化

关键消息通过JetStream进行持久化：

```go
// 发布持久化消息
func (r *RuleEngine) publishPersistent(subject string, data []byte) {
    _, err := r.js.Publish(subject, data)
    if err != nil {
        log.Error().Err(err).Str("subject", subject).Msg("发布持久化消息失败")
    }
}
```

## 性能优化

### 1. 连接池管理

```go
type NATSPool struct {
    connections []*nats.Conn
    current     int
    mu          sync.Mutex
}

func (p *NATSPool) GetConnection() *nats.Conn {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    conn := p.connections[p.current]
    p.current = (p.current + 1) % len(p.connections)
    return conn
}
```

### 2. 批量发布

```go
// 批量发布消息
func (r *RuleEngine) publishBatch(messages []Message) {
    for _, msg := range messages {
        r.natsConn.PublishAsync(msg.Subject, msg.Data)
    }
    r.natsConn.FlushTimeout(time.Second)
}
```

### 3. 异步处理

```go
// 异步消息处理
func (r *RuleEngine) processAsync(msg *nats.Msg) {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                log.Error().Interface("panic", r).Msg("消息处理异常")
            }
        }()
        
        r.processMessage(msg)
    }()
}
```

## 监控和调试

### 1. 消息统计

```go
// 统计消息处理情况
type MessageStats struct {
    Published   int64
    Consumed    int64
    Failed      int64
    LastMessage time.Time
}

func (s *MessageStats) RecordPublish() {
    atomic.AddInt64(&s.Published, 1)
    s.LastMessage = time.Now()
}
```

### 2. 健康检查

```go
// NATS连接健康检查
func (r *Runtime) healthCheck() bool {
    if r.Bus == nil {
        return false
    }
    
    return r.Bus.IsConnected()
}
```

### 3. 调试工具

```bash
# 监控所有消息
nats sub "iot.>"

# 监控特定设备
nats sub "iot.processed.sensor_001.*"

# 监控规则处理结果
nats sub "iot.rules.*"

# 监控报警
nats sub "iot.alerts.*"

# 发送测试消息
nats pub iot.processed.test '{"device_id":"test","key":"temperature","value":25.5}'
```

## 配置示例

### 1. 嵌入式NATS配置

```yaml
gateway:
  nats_url: "embedded"  # 使用嵌入式NATS服务器
  jetstream:
    enabled: true
    store_dir: "./data/jetstream"
    max_memory: "1GB"
    max_file: "10GB"
```

### 2. 外部NATS配置

```yaml
gateway:
  nats_url: "nats://nats-cluster:4222"
  nats_options:
    max_reconnect: -1
    reconnect_wait: "2s"
    timeout: "5s"
```

### 3. 规则引擎NATS配置

```yaml
rule_engine:
  nats:
    input_subject: "iot.processed.*"
    output_subject: "iot.rules.*"
    error_subject: "iot.errors.rules"
    queue_group: "rule_engine_workers"
    max_pending: 1000
    ack_wait: "30s"
```

## 最佳实践

### 1. 主题命名规范

- 使用层次化主题结构
- 包含版本信息
- 使用通配符进行分组订阅
- 避免过深的主题层次

### 2. 消息设计

- 保持消息大小适中（< 1MB）
- 包含必要的元数据
- 使用统一的消息格式
- 考虑消息的可扩展性

### 3. 错误处理

- 实现消息重试机制
- 记录详细的错误日志
- 使用死信队列处理失败消息
- 监控消息处理延迟

### 4. 性能优化

- 使用连接池管理连接
- 批量发布消息
- 异步处理非关键消息
- 合理设置缓冲区大小

## 总结

NATS作为IoT Gateway的核心消息总线，提供了：

1. **统一通信**: 所有模块通过NATS进行通信
2. **事件驱动**: 基于发布/订阅的事件驱动架构
3. **高性能**: 低延迟、高吞吐量的消息传递
4. **可扩展**: 支持水平扩展和集群部署
5. **持久化**: 通过JetStream提供消息持久化
6. **监控**: 完整的消息统计和监控能力

规则引擎通过NATS与其他模块无缝集成，实现了松耦合、高性能的IoT数据处理架构。 