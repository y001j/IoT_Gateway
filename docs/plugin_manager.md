# Plugin Manager 设计文档

> 版本：v0.2 &nbsp;&nbsp; 作者：Rocky Yang  &nbsp;&nbsp; 日期：2025-06-28

## 1. 目标
负责 IoT Gateway 插件的生命周期管理，包括本地 `.so` 动态库（Go Plugin）、跨语言 gRPC Sidecar、内置（builtin）三种形式的 Southbound `Adapter` 与 Northbound `Sink`。实现热发现、加载、卸载、升级，并向 Core Runtime 注册可用服务。

## 2. 功能点
1. **插件目录监听**：监控 `./plugins/`，新增/修改/删除 事件触发对应操作。
2. **插件元信息**：`plugin.json` 描述名称、版本、类型(adapter/sink)、入口、依赖、schema 等。
3. **加载机制**：
   - **Go Plugin**：使用 `plugin.Open` 加载符号，断言实现了 `Adapter` 或 `Sink` 接口。
   - **gRPC Sidecar**：解析 `exec` 字段，拉起子进程并通过 protobuf 定义接口通信。
   - **Builtin**：内置在主程序中的插件，无需额外加载，直接注册到插件管理器。
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
    Mode    string `json:"mode"`   // go-plugin | grpc-sidecar | builtin
    Entry   string `json:"entry"`  // .so 或 可执行文件，builtin 模式下可为空
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

### 4.1 Go Plugin
```go
//export AdapterInit
type AdapterInit func(cfg []byte) (southbound.Adapter, error)
```
编译指令：
```bash
go build -buildmode=plugin -o plugins/modbus.so ./adapter/modbus
```

### 4.2 gRPC Sidecar
- 协议：`plugin.proto` 定义 `Collect(Request) stream Point` / `Publish(Batch)`。
- Manager 以 `os/exec` 启动子进程，健康探针 & 心跳。
- 支持多语言实现，如 Python, Node.js, Java 等。

### 4.3 Builtin
- 内置在主程序中的插件，无需额外加载。
- 通过代码直接注册到插件管理器。
- 适用于核心功能或高性能场景。
```go
func RegisterBuiltinAdapter(name string, adapter southbound.Adapter) error
func RegisterBuiltinSink(name string, sink northbound.Sink) error
```

## 6. 目录结构
```
internal/plugin/
    ├── manager.go
    ├── loader.go
    ├── watcher.go
    └── proto/
plugins/
    └── README.md  // 规范
```

## 7. 配置示例
```yaml
plugins:
  dir: ./plugins
  allow_types: ["adapter", "sink"]
  allow_modes: ["go-plugin", "grpc-sidecar", "builtin"]
```

## 8. 测试
- 使用 mock 插件生成器自动产出 Built-in、`.so` 和 sidecar 三种类型插件，跑集成测试。
- 在不同平台（Linux、macOS、Windows）测试插件兼容性。

## 9. 平台兼容性
| 插件类型 | Linux | macOS | Windows |
|---------|-------|-------|--------|
| Built-in | ✅ | ✅ | ✅ |
| Go Plugin | ✅ | ✅ | ⚠️ 有限制 |
| gRPC Sidecar | ✅ | ✅ | ✅ |

---
