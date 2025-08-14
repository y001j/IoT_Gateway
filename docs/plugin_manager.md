# Plugin Manager 设计文档

> 版本：v1.1 &nbsp;&nbsp; 作者：IoT Gateway Team  &nbsp;&nbsp; 日期：2024-08-14

## 1. 目标
负责 IoT Gateway 插件的生命周期管理，主要支持内置（Builtin）和 ISP Sidecar 两种插件类型，为 Southbound `Adapter` 与 Northbound `Sink` 提供统一的加载和管理机制。实现热发现、加载、卸载、升级，并向 Core Runtime 注册可用服务。

## 2. 功能点
1. **插件目录监听**：监控 `./plugins/`，新增/修改/删除 事件触发对应操作。
2. **插件元信息**：`plugin.json` 描述名称、版本、类型(adapter/sink)、入口、依赖、schema 等。
3. **加载机制**：
   - **Builtin**：内置在主程序中的插件，通过 `builtin://` 协议标识，直接注册到插件管理器。
   - **ISP Sidecar**：基于 ISP (IoT Sidecar Protocol) 协议的独立进程插件，支持跨语言实现。
   - **可扩展协议**：架构支持添加新的插件协议类型。
4. **注册表**：维护全局 `Adapters`、`Sinks` map，为 Core Runtime / Rule Engine 查找。
5. **热升级**：新版本放入目录 → 标记 old 为 draining → 新实例 ready → 关闭 old。
6. **错误隔离**：加载失败不影响主进程，错误记录并报警。

## 3. 数据结构与接口
```go
// 插件元信息
package plugin

type Meta struct {
    Name    string `json:"name"`
    Version string `json:"version"`
    Type    string `json:"type"`   // adapter | sink
    Mode    string `json:"mode"`   // builtin | isp-sidecar | 可扩展
    Entry   string `json:"entry"`  // builtin:// 或 可执行文件路径
}

// Manager
package plugin

func NewManager(dir string, bus bus.Bus) *Manager

type Manager struct {
    Adapters map[string]southbound.Adapter
    Sinks    map[string]northbound.Sink
}

// Watch loop
func (m *Manager) Start(ctx context.Context) error
```

## 4. 插件类型

### 4.1 Builtin 插件
内置在主程序中的插件，提供核心功能和高性能实现。

**特征**:
- 编译时集成到主程序
- 无进程间通信开销
- 通过 `builtin://` 协议标识
- 适用于核心功能或高性能场景

**配置示例**:
```json
{
  "name": "mock-adapter",
  "version": "1.0.0",
  "type": "adapter",
  "mode": "builtin",
  "entry": "builtin://mock"
}
```

**支持的内置插件**:
- `mock`: 模拟设备适配器，用于测试和开发
- 其他内置插件可通过代码扩展

### 4.2 ISP Sidecar 插件
基于 ISP (IoT Sidecar Protocol) 协议的独立进程插件。

**特征**:
- 独立进程运行，语言无关
- 基于 TCP + JSON 的简单协议
- 支持热插拔和故障隔离
- 支持 Python, Node.js, Java, C++ 等语言

**协议定义**:
```json
{
  "type": "CONFIG|DATA|STATUS|HEARTBEAT|RESPONSE",
  "id": "message-id",
  "timestamp": 1640995200000,
  "payload": { }
}
```

**配置示例**:
```json
{
  "name": "modbus-sidecar",
  "version": "1.0.0", 
  "type": "adapter",
  "mode": "isp-sidecar",
  "entry": "./modbus-sidecar/main.exe"
}
```

### 4.3 协议扩展性
Plugin Manager 采用可扩展架构，支持添加新的插件协议：

**扩展方式**:
1. 实现新的加载器函数
2. 在 `loader.go` 中注册新协议
3. 添加对应的配置验证逻辑

**示例扩展**:
- WebAssembly (WASM) 插件
- Docker 容器插件 
- HTTP API 插件

## 5. ISP 协议详细说明

### 消息类型
- **CONFIG**: 配置消息，Gateway向Sidecar发送配置信息
- **DATA**: 数据消息，Sidecar向Gateway发送采集的数据  
- **STATUS**: 状态查询消息
- **HEARTBEAT**: 心跳保活消息
- **RESPONSE**: 响应消息，用于请求/响应匹配

### 数据流程
1. **连接建立**: Gateway作为客户端连接Sidecar的TCP服务器
2. **配置下发**: Gateway发送CONFIG消息配置Sidecar
3. **数据采集**: Sidecar周期性发送DATA消息
4. **心跳保活**: 双向HEARTBEAT消息维持连接
5. **状态监控**: Gateway可查询Sidecar状态

## 6. 目录结构
```
internal/plugin/
    ├── manager.go           // 插件管理器主逻辑
    ├── loader.go           // 插件加载器
    ├── isp_client.go       // ISP协议客户端
    ├── isp_protocol.go     // ISP协议定义
    ├── isp_adapter_proxy.go // ISP适配器代理
    └── plugin_init.go      // 插件初始化
plugins/
    ├── modbus-sidecar.json // ISP插件配置示例
    └── README.md           // 插件开发规范
```

## 7. 配置示例

### Gateway 配置
```yaml
plugins:
  dir: ./plugins
  allow_types: ["adapter", "sink"]
  allow_modes: ["builtin", "isp-sidecar"]
```

### Builtin 插件配置
```json
{
  "name": "mock-adapter",
  "version": "1.0.0",
  "type": "adapter",
  "mode": "builtin", 
  "entry": "builtin://mock",
  "description": "Mock adapter for testing"
}
```

### ISP Sidecar 插件配置
```json
{
  "name": "modbus-sidecar",
  "version": "1.0.0",
  "type": "adapter",
  "mode": "isp-sidecar",
  "entry": "./modbus-sidecar/main.exe",
  "config": {
    "address": "localhost:50052",
    "timeout_ms": 5000,
    "retry_count": 3
  }
}
```

## 8. 测试
- 使用 mock 插件验证 Builtin 插件加载机制
- 使用 modbus-sidecar 验证 ISP 协议通信
- 集成测试覆盖插件生命周期管理
- 在不同平台测试插件兼容性

## 9. 平台兼容性
| 插件类型 | Linux | macOS | Windows | 备注 |
|---------|-------|-------|--------|------|
| Builtin | ✅ | ✅ | ✅ | 无兼容性问题 |
| ISP Sidecar | ✅ | ✅ | ✅ | 基于TCP协议，跨平台 |

## 10. 开发指南

### 开发 Builtin 插件
1. 实现 `southbound.Adapter` 或 `northbound.Sink` 接口
2. 在 `loader.go` 中注册新的内置插件
3. 创建对应的工厂函数

### 开发 ISP Sidecar 插件
1. 实现 ISP 协议的 TCP 服务器
2. 处理 CONFIG、STATUS、HEARTBEAT 消息
3. 周期性发送 DATA 消息
4. 创建插件配置 JSON 文件

### 扩展新协议
1. 在 `Meta` 结构中添加新的 `mode` 类型
2. 实现对应的加载函数
3. 在 `loader.go` 中添加加载逻辑
4. 添加配置验证和错误处理

---
