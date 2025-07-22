让我查看一下当前项目中实现了哪些sink连接器：
Ran tool
Ran tool
Read file: internal/northbound/sink.go
现在让我查看每个连接器的具体功能描述：
Read file: internal/northbound/console/console.go
Read file: internal/northbound/jetstream/jetstream.go
Read file: internal/northbound/redis/redis.go
Read file: internal/northbound/influxdb/influxdb.go
Read file: internal/northbound/websocket/websocket.go
根据代码分析，目前IoT Gateway项目实现了以下**6个Sink连接器**：

## 🔗 **已实现的Sink连接器**

### 1. **Console Sink** 📺
- **功能**：将数据点输出到控制台，用于调试和监控
- **特性**：
  - 批量处理和缓冲
  - 格式化输出（时间戳、设备ID、数据点、标签）
  - 实时显示数据流
- **配置**：批处理大小、缓冲区大小、附加标签

### 2. **MQTT Sink** 📡
- **功能**：将数据点发布到MQTT代理
- **特性**：
  - 支持QoS设置
  - 主题模板配置
  - TLS/SSL连接支持
  - 断线重连机制

### 3. **JetStream Sink** 🚀
- **功能**：使用NATS JetStream进行数据持久化
- **特性**：
  - 消息持久化存储
  - 流式数据处理
  - 高可用性和容错性
  - 消费者管理
- **配置**：流名称、主题、保留策略、副本数

### 4. **Redis Sink** 🔴
- **功能**：将数据点存储到Redis数据库
- **特性**：
  - 多种存储格式（JSON、字符串、哈希）
  - 键模板配置（支持占位符）
  - 数据过期设置
  - 高性能缓存存储
- **配置**：连接参数、键格式、过期时间、存储格式

### 5. **InfluxDB Sink** 📊
- **功能**：将数据点存储到InfluxDB时序数据库
- **特性**：
  - 时序数据优化存储
  - 批量写入性能优化
  - 测量值和标签配置
  - 组织和桶管理
- **配置**：URL、认证令牌、组织、桶、批处理设置

### 6. **WebSocket Sink** 🌐
- **功能**：通过WebSocket实时推送数据到客户端
- **特性**：
  - 实时数据推送
  - 多客户端连接管理
  - CORS支持
  - 消息格式转换
  - 主题订阅机制
- **配置**：监听地址、路径、CORS设置、消息格式

## 📋 **连接器特性对比**

| 连接器 | 实时性 | 持久化 | 批处理 | 适用场景 |
|--------|--------|--------|--------|----------|
| Console | ✅ | ❌ | ✅ | 调试、监控 |
| MQTT | ✅ | ❌ | ✅ | IoT设备通信 |
| JetStream | ✅ | ✅ | ✅ | 消息队列、事件流 |
| Redis | ✅ | ✅ | ❌ | 缓存、会话存储 |
| InfluxDB | ✅ | ✅ | ✅ | 时序数据分析 |
| WebSocket | ✅ | ❌ | ❌ | 实时Web应用 |

## 🔧 **连接器注册机制**

所有连接器都通过统一的注册机制进行管理：

```go
// 在每个连接器的init()函数中注册
northbound.Register("连接器类型", func() northbound.Sink {
    return &具体连接器{}
})
```

这些连接器为IoT Gateway提供了丰富的数据输出选项，支持从实时监控到长期存储的各种应用场景！🎯