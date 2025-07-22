# Sidecar 实现和使用指南

本文档提供了IoT Gateway系统中Sidecar模式插件的完整实现指南和使用说明。

## 概述

Sidecar模式是IoT Gateway支持的插件架构之一，通过独立进程和ISP (IoT Sidecar Protocol)协议实现设备适配器的解耦。这种模式具有以下优势：

- **进程隔离**: Sidecar运行在独立进程中，故障不会影响主Gateway
- **语言无关**: 可以使用任何语言实现Sidecar
- **易于调试**: 可独立启动和调试Sidecar进程
- **高可用性**: 支持故障恢复和自动重连

## ISP 协议规范

### 协议特性

- **传输协议**: TCP
- **消息格式**: 行分隔的JSON消息
- **默认端口**: 50052 (可配置)
- **编码**: UTF-8
- **架构**: 客户端-服务器 (Gateway为客户端，Sidecar为服务器)

### 消息类型

| 类型 | 方向 | 描述 |
|------|------|------|
| `CONFIG` | Gateway → Sidecar | 配置消息 |
| `DATA` | Sidecar → Gateway | 数据消息 |
| `STATUS` | Gateway → Sidecar | 状态查询 |
| `RESPONSE` | 双向 | 响应消息 |
| `HEARTBEAT` | 双向 | 心跳消息 |

### 消息结构

```go
type ISPMessage struct {
    Type      string          `json:"type"`              // 消息类型
    ID        string          `json:"id,omitempty"`      // 消息ID（用于请求/响应匹配）
    Timestamp int64           `json:"timestamp"`         // 时间戳（纳秒）
    Payload   json.RawMessage `json:"payload,omitempty"` // 消息载荷
}
```

## 标准实现模式

### 1. Sidecar服务器实现

#### 基本结构

```go
package main

import (
    "context"
    "net"
    "sync"
    "time"
    "encoding/json"
    "bufio"
)

type SidecarServer struct {
    address    string
    listener   net.Listener
    clients    map[string]*ClientConn
    clientsMu  sync.RWMutex
    running    bool
    ctx        context.Context
    cancel     context.CancelFunc
    config     *ConfigPayload  // 使用标准ISP配置结构
}

type ClientConn struct {
    conn    net.Conn
    scanner *bufio.Scanner
    writer  *bufio.Writer
    id      string
    server  *SidecarServer
}
```

#### 必须实现的方法

```go
// 启动服务器
func (s *SidecarServer) Start(ctx context.Context) error

// 停止服务器
func (s *SidecarServer) Stop() error

// 处理配置消息
func (c *ClientConn) handleConfigMessage(msg *ISPMessage)

// 处理状态查询
func (c *ClientConn) handleStatusMessage(msg *ISPMessage)

// 处理心跳消息
func (c *ClientConn) handleHeartbeatMessage(msg *ISPMessage)

// 数据采集和广播
func (s *SidecarServer) collectData()
func (s *SidecarServer) broadcastData(points []DataPoint)
```

### 2. 标准配置格式

#### 插件配置文件 (.json)

```json
{
  "name": "my-sidecar",
  "version": "1.0.0",
  "type": "adapter",
  "mode": "isp-sidecar",
  "entry": "my-sidecar/my-sidecar.exe",
  "description": "设备适配器（ISP Sidecar模式）",
  "isp_port": 50052
}
```

#### ISP配置消息

```json
{
  "type": "CONFIG",
  "id": "config-001",
  "timestamp": 1751200000000000000,
  "payload": {
    "mode": "tcp",
    "address": "127.0.0.1:502",
    "timeout_ms": 3000,
    "interval_ms": 2000,
    "registers": [
      {
        "key": "temperature",
        "address": 0,
        "quantity": 1,
        "type": "float32",
        "function": 3,
        "scale": 0.1,
        "device_id": 1,
        "tags": {"unit": "°C"}
      }
    ]
  }
}
```

### 3. 数据点格式

```json
{
  "type": "DATA",
  "timestamp": 1751200001000000000,
  "payload": {
    "points": [
      {
        "key": "temperature",
        "source": "my-sidecar",
        "timestamp": 1751200001000000000,
        "value": 25.5,
        "type": "float32",
        "quality": 1,
        "tags": {"unit": "°C", "device": "sensor-01"}
      }
    ]
  }
}
```

## 实现最佳实践

### 1. 错误处理

```go
// 优雅的错误处理
func (c *ClientConn) sendErrorResponse(id string, errMsg string) {
    response := map[string]interface{}{
        "success": false,
        "error":   errMsg,
    }
    
    payload, _ := json.Marshal(response)
    msg := &ISPMessage{
        Type:      "RESPONSE",
        ID:        id,
        Timestamp: time.Now().UnixNano(),
        Payload:   payload,
    }
    
    c.sendMessage(msg)
}
```

### 2. 连接管理

```go
// 支持多客户端连接
func (s *SidecarServer) acceptLoop() {
    for {
        select {
        case <-s.ctx.Done():
            return
        default:
            conn, err := s.listener.Accept()
            if err != nil {
                continue
            }
            
            clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
            client := &ClientConn{
                conn:    conn,
                scanner: bufio.NewScanner(conn),
                writer:  bufio.NewWriter(conn),
                id:      clientID,
                server:  s,
            }
            
            s.clientsMu.Lock()
            s.clients[clientID] = client
            s.clientsMu.Unlock()
            
            go client.handleConnection()
        }
    }
}
```

### 3. 心跳机制

```go
// 定期发送心跳
func (s *SidecarServer) startHeartbeat() {
    ticker := time.NewTicker(15 * time.Second)
    go func() {
        defer ticker.Stop()
        for {
            select {
            case <-s.ctx.Done():
                return
            case <-ticker.C:
                s.sendHeartbeat()
            }
        }
    }()
}
```

### 4. 数据质量

```go
// 质量码定义
const (
    QualityGood    = 1  // 正常
    QualityBad     = 0  // 异常
    QualityUnknown = -1 // 未知
)

// 创建数据点时设置质量码
point := DataPoint{
    Key:       reg.Key,
    Source:    "my-sidecar",
    Timestamp: time.Now().UnixNano(),
    Value:     value,
    Type:      reg.Type,
    Quality:   QualityGood, // 根据实际情况设置
    Tags:      reg.Tags,
}
```

## 部署和配置

### 1. 目录结构

```
plugins/
├── my-sidecar/
│   ├── my-sidecar.exe      # 可执行文件
│   ├── config.json         # 本地配置（可选）
│   └── README.md           # 说明文档
└── my-sidecar-isp.json     # 插件元配置
```

### 2. Gateway配置

```yaml
# config.yaml
gateway:
  plugins_dir: "./plugins"
  
southbound:
  adapters:
    - name: "my_device_adapter"
      type: "my-sidecar"
      enabled: true
      config:
        mode: "tcp"
        address: "192.168.1.100:502"
        timeout_ms: 3000
        interval_ms: 1000
        registers:
          - key: "temperature"
            address: 0
            quantity: 1
            type: "float32"
            function: 3
            scale: 0.1
            device_id: 1
```

### 3. 环境变量

```bash
# 设置ISP端口
export ISP_PORT=50052

# 设置日志级别
export LOG_LEVEL=info

# 启动Sidecar
./my-sidecar.exe
```

## 开发工具

### 1. ISP消息测试工具

```bash
# 使用telnet测试ISP连接
telnet localhost 50052

# 发送配置消息
{"type":"CONFIG","id":"test-001","timestamp":1751200000000000000,"payload":{"mode":"tcp","address":"127.0.0.1:502","timeout_ms":3000,"interval_ms":2000,"registers":[]}}

# 发送状态查询
{"type":"STATUS","id":"status-001","timestamp":1751200000000000000}
```

### 2. 日志监控

```bash
# 查看Sidecar日志
tail -f sidecar.log

# 查看Gateway日志
tail -f logs/gateway.log | grep "isp"
```

### 3. 性能监控

```go
// 添加性能指标
type Metrics struct {
    MessagesReceived  int64
    MessagesSent      int64
    Errors           int64
    ConnectionCount  int64
    LastDataTime     time.Time
}

// 在适当位置更新指标
atomic.AddInt64(&metrics.MessagesReceived, 1)
```

## 故障排除

### 常见问题

1. **连接失败**
   - 检查端口是否被占用：`netstat -an | grep 50052`
   - 检查防火墙设置
   - 验证ISP_PORT环境变量

2. **消息格式错误**
   - 确保JSON格式正确
   - 检查时间戳格式（纳秒）
   - 验证消息类型拼写

3. **数据不更新**
   - 检查设备连接状态
   - 验证寄存器配置
   - 查看错误日志

### 调试模式

```go
// 启用详细日志
log.SetLevel(log.DebugLevel)

// 添加调试输出
log.Debug().
    Str("message_type", msg.Type).
    Str("payload", string(msg.Payload)).
    Msg("收到ISP消息")
```

## 进阶功能

### 1. 配置热重载

```go
// 监听配置文件变化
func (s *SidecarServer) watchConfig() {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return
    }
    defer watcher.Close()
    
    // 添加配置文件监听
    watcher.Add("config.json")
    
    for {
        select {
        case event := <-watcher.Events:
            if event.Op&fsnotify.Write == fsnotify.Write {
                s.reloadConfig()
            }
        case <-s.ctx.Done():
            return
        }
    }
}
```

### 2. 数据缓存

```go
// 实现数据缓存机制
type DataCache struct {
    data map[string]DataPoint
    mu   sync.RWMutex
    ttl  time.Duration
}

func (c *DataCache) Set(key string, point DataPoint) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.data[key] = point
}

func (c *DataCache) Get(key string) (DataPoint, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    point, exists := c.data[key]
    return point, exists
}
```

### 3. 批量数据处理

```go
// 批量收集和发送数据
func (s *SidecarServer) collectBatchData() {
    var points []DataPoint
    batchSize := 100
    
    for _, reg := range s.config.Registers {
        if len(points) >= batchSize {
            s.broadcastData(points)
            points = points[:0] // 清空切片
        }
        
        if value, err := s.readRegister(&reg); err == nil {
            point := DataPoint{
                Key:       reg.Key,
                Source:    "sidecar",
                Timestamp: time.Now().UnixNano(),
                Value:     value,
                Type:      reg.Type,
                Quality:   1,
                Tags:      reg.Tags,
            }
            points = append(points, point)
        }
    }
    
    if len(points) > 0 {
        s.broadcastData(points)
    }
}
```

## 参考实现

完整的参考实现可在以下位置找到：

- **Modbus Sidecar**: `/plugins/modbus-sidecar/`
- **ISP协议定义**: `/internal/plugin/isp_protocol.go`
- **ISP客户端**: `/internal/plugin/isp_client.go`
- **ISP代理适配器**: `/internal/plugin/isp_adapter_proxy.go`

## 总结

Sidecar模式为IoT Gateway提供了灵活、可扩展的插件架构。通过遵循ISP协议规范和最佳实践，可以快速开发出稳定可靠的设备适配器。关键要点：

1. **严格遵循ISP协议规范**
2. **实现完整的错误处理机制**
3. **支持优雅启动和关闭**
4. **提供详细的日志和监控**
5. **考虑性能和资源使用**

这种架构模式特别适合需要特殊协议支持、复杂数据处理或高可用性要求的场景。