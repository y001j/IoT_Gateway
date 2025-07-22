# IoT Gateway 模块架构设计

## 概述

IoT Gateway 采用模块化架构设计，将整个系统拆分为 8 个独立但松耦合的模块。每个模块职责明确，接口清晰，支持并行开发和独立部署。

## 模块总览

| # | 模块名称 | 关键职责 | 优先级 | 状态 |
|---|---------|---------|--------|------|
| 1 | Core Runtime | 进程生命周期、配置管理、消息总线 | ★★★★★ | ✅ 已完成 |
| 2 | Plugin Manager | 插件发现、加载、热插拔管理 | ★★★★★ | ✅ 已完成 |
| 3 | Southbound Adapters | 设备侧协议驱动集合 | ★★★★☆ | ✅ 已完成 |
| 4 | Northbound Sinks | 上游系统连接器 | ★★★★☆ | ✅ 已完成 |
| 5 | Rule Engine | 数据处理、规则引擎 | ★★★★☆ | ✅ 已完成 |
| 6 | Web UI & REST API | 可视化运维、配置管理 | ★★★☆☆ | 🚧 开发中 |
| 7 | OTA & Versioning | 远程升级、版本管理 | ★★☆☆☆ | 📋 规划中 |
| 8 | Security Layer | 安全认证、权限控制 | ★★☆☆☆ | 📋 规划中 |

---

## 模块详细设计

### 1. Core Runtime 核心运行时

**职责：** 系统核心，负责进程生命周期、配置管理和内部消息总线

#### 主要功能
- 系统启动与关闭流程管理
- 配置文件加载与热更新 (`config.yaml`)
- 内部消息总线（基于 NATS JetStream）
- 日志系统（zerolog）与监控指标（Prometheus）
- 优雅关闭与错误恢复

#### 对外接口
```go
type CoreRuntime interface {
    // 启动系统
    Start() error
    
    // 停止系统
    Stop() error
    
    // 获取消息总线
    GetBus() Bus
    
    // 获取配置
    GetConfig() *Config
    
    // 注册关闭回调
    RegisterShutdownHook(func())
}

type Bus interface {
    Publish(subject string, data []byte) error
    Subscribe(subject string, handler func([]byte)) error
    PublishPoint(point *model.Point) error
    SubscribePoints(handler func(*model.Point)) error
}
```

#### 实现文件
- `internal/core/runtime.go` - 主要实现
- `internal/core/bus/bus.go` - 消息总线实现
- `cmd/gateway/main.go` - 启动入口

#### 依赖关系
- **被依赖：** 所有其他模块
- **依赖：** 无（基础模块）

---

### 2. Plugin Manager 插件管理器

**职责：** 负责插件的发现、加载、生命周期管理和热插拔

#### 主要功能
- 插件目录扫描与文件监控
- 支持 Go Plugin 和 gRPC Sidecar 两种插件模式
- 插件元数据验证与版本管理
- 插件热加载与卸载
- 插件状态监控与健康检查

#### 对外接口
```go
type PluginManager interface {
    // 注册适配器插件
    RegisterAdapter(pluginType string, impl Adapter) error
    
    // 注册北向插件
    RegisterSink(pluginType string, impl Sink) error
    
    // 加载插件
    LoadPlugin(path string) error
    
    // 卸载插件
    UnloadPlugin(name string) error
    
    // 获取插件列表
    ListPlugins() []PluginInfo
    
    // 获取插件状态
    GetPluginStatus(name string) PluginStatus
}
```

#### 插件类型支持
1. **Go Plugin** - 动态库形式，性能最佳
2. **ISP Sidecar** - 独立进程，通过 ISP 协议通信
3. **gRPC Sidecar** - 通过 gRPC 通信（规划中）

#### 实现文件
- `internal/plugin/manager.go` - 插件管理器
- `internal/plugin/loader.go` - 插件加载器
- `internal/plugin/isp_adapter_proxy.go` - ISP 适配器代理
- `internal/plugin/isp_client.go` - ISP 客户端
- `internal/plugin/isp_protocol.go` - ISP 协议实现

#### 依赖关系
- **依赖：** Core Runtime
- **被依赖：** Southbound Adapters, Northbound Sinks

---

### 3. Southbound Adapters 南向适配器

**职责：** 设备侧协议驱动，负责与各种设备和协议的通信

#### 支持的协议
- ✅ **Modbus RTU/TCP** - 工业现场总线协议
- ✅ **HTTP** - RESTful API 接口
- ✅ **MQTT Subscribe** - MQTT 订阅客户端
- ✅ **Mock** - 模拟数据源（测试用）
- 📋 **OPC-UA** - 工业自动化标准（规划中）
- 📋 **BLE** - 蓝牙低功耗（规划中）

#### 统一接口定义
```go
type Adapter interface {
    // 启动适配器，开始数据采集
    Start(output chan<- *model.Point) error
    
    // 停止适配器
    Stop() error
    
    // 获取适配器信息
    GetInfo() AdapterInfo
    
    // 健康检查
    HealthCheck() error
    
    // 配置更新
    UpdateConfig(config map[string]interface{}) error
}

type AdapterInfo struct {
    Name        string            `json:"name"`
    Type        string            `json:"type"`
    Version     string            `json:"version"`
    Description string            `json:"description"`
    Status      string            `json:"status"`
    Config      map[string]interface{} `json:"config"`
}
```

#### 实现文件
- `internal/southbound/adapter.go` - 适配器基础接口
- `internal/southbound/modbus/modbus.go` - Modbus 适配器
- `internal/southbound/http/http.go` - HTTP 适配器
- `internal/southbound/mqtt_sub/mqtt_sub.go` - MQTT 订阅适配器
- `internal/southbound/mock/mock.go` - 模拟适配器

#### 插件形式
- `plugins/modbus/` - Modbus Sidecar 插件
- `plugins/modbus-sidecar/` - Modbus ISP Sidecar 插件

---

### 4. Northbound Sinks 北向连接器

**职责：** 向上游系统发送数据，支持多种目标系统和协议

#### 支持的目标系统
- ✅ **MQTT** - 支持 TLS/SSL 加密
- ✅ **InfluxDB** - 时序数据库
- ✅ **Redis** - 内存数据库
- ✅ **JetStream** - NATS 持久化存储
- ✅ **Console** - 控制台输出（调试用）
- ✅ **WebSocket** - 实时数据推送
- 📋 **Kafka** - 分布式消息队列（规划中）
- 📋 **REST API** - HTTP RESTful 接口（规划中）

#### 统一接口定义
```go
type Sink interface {
    // 发送数据点
    Publish(points []*model.Point) error
    
    // 发送单个数据点
    PublishSingle(point *model.Point) error
    
    // 停止连接器
    Stop() error
    
    // 获取连接器信息
    GetInfo() SinkInfo
    
    // 健康检查
    HealthCheck() error
    
    // 配置更新
    UpdateConfig(config map[string]interface{}) error
}
```

#### 特性支持
- **批量发送** - 支持批量数据发送以提高性能
- **确认机制** - 支持发送确认和重试机制
- **连接池** - 复用连接以提高效率
- **负载均衡** - 支持多目标负载均衡

#### 实现文件
- `internal/northbound/sink.go` - 连接器基础接口
- `internal/northbound/mqtt/mqtt.go` - MQTT 连接器
- `internal/northbound/influxdb/influxdb.go` - InfluxDB 连接器
- `internal/northbound/redis/redis.go` - Redis 连接器
- `internal/northbound/jetstream/jetstream.go` - JetStream 连接器
- `internal/northbound/console/console.go` - 控制台连接器
- `internal/northbound/websocket/websocket.go` - WebSocket 连接器

---

### 5. Rule Engine 规则引擎

**职责：** 数据处理、规则执行、动作触发

#### 核心功能
- **数据过滤** - 基于条件过滤数据点
- **数据转换** - 数据格式转换和计算
- **数据聚合** - 时间窗口内的数据聚合
- **告警处理** - 基于规则的告警生成
- **数据转发** - 条件转发到不同目标

#### 规则类型
1. **Filter Rules** - 数据过滤规则
2. **Transform Rules** - 数据转换规则
3. **Aggregate Rules** - 数据聚合规则
4. **Alert Rules** - 告警规则
5. **Forward Rules** - 转发规则

#### 表达式引擎
- 支持多种表达式语言：`expr`, `JavaScript`, `Lua`
- 内置函数库：数学函数、字符串处理、时间处理
- 自定义函数扩展

#### 对外接口
```go
type RuleEngine interface {
    // 加载规则
    LoadRules(rules []Rule) error
    
    // 添加规则
    AddRule(rule Rule) error
    
    // 删除规则
    RemoveRule(ruleID string) error
    
    // 处理数据点
    ProcessPoint(point *model.Point) error
    
    // 获取规则列表
    ListRules() []Rule
    
    // 获取规则统计
    GetStats() RuleStats
}
```

#### 实现文件
- `internal/rules/manager.go` - 规则管理器
- `internal/rules/evaluator.go` - 规则评估器
- `internal/rules/actions/` - 各种动作实现
  - `filter.go` - 过滤动作
  - `transform.go` - 转换动作
  - `aggregate.go` - 聚合动作
  - `alert.go` - 告警动作
  - `forward.go` - 转发动作

---

### 6. Web UI & REST API 管理界面

**职责：** 提供可视化运维界面和 REST API

#### 功能特性
- **实时监控** - 系统状态、数据流量监控
- **配置管理** - 插件配置、规则配置
- **日志查看** - 系统日志、错误日志
- **用户管理** - RBAC 权限控制
- **API 文档** - Swagger 自动生成

#### REST API 端点
```
GET    /api/status          # 系统状态
GET    /api/plugins         # 插件列表
POST   /api/plugins/reload  # 重载插件
GET    /api/rules           # 规则列表
POST   /api/rules           # 创建规则
PUT    /api/rules/:id       # 更新规则
DELETE /api/rules/:id       # 删除规则
GET    /api/metrics         # 监控指标
WS     /api/ws/data         # 实时数据流
```

#### 技术栈
- **前端：** React + Ant Design + TypeScript
- **后端：** Gin + WebSocket
- **认证：** JWT Token
- **文档：** Swagger/OpenAPI

#### 状态
🚧 **开发中** - 基础 API 已完成，前端界面开发中

---

### 7. OTA & Versioning 远程升级

**职责：** 系统和插件的远程升级管理

#### 主要功能
- **二进制升级** - 系统核心程序升级
- **插件市场** - 插件下载、安装、更新
- **版本管理** - 多版本支持、回滚机制
- **完整性校验** - 文件签名验证
- **多架构支持** - Windows/Linux/ARM 等

#### 升级流程
1. 检查更新 → 2. 下载文件 → 3. 校验签名 → 4. 备份当前版本 → 5. 安装新版本 → 6. 验证功能 → 7. 清理备份

#### API 接口
```
GET  /api/ota/check        # 检查更新
POST /api/ota/upgrade      # 执行升级
POST /api/ota/rollback     # 版本回滚
GET  /api/plugins/market   # 插件市场
POST /api/plugins/install  # 安装插件
```

#### 状态
📋 **规划中** - 设计阶段，优先级较低

---

### 8. Security Layer 安全层

**职责：** 系统安全、认证授权、数据加密

#### 安全功能
- **传输加密** - 双向 TLS 认证
- **证书管理** - 证书轮换、自动续期
- **访问控制** - 细粒度权限模型
- **配置加密** - 敏感配置加密存储
- **审计日志** - 操作审计跟踪

#### 认证方式
- **JWT Token** - API 访问令牌
- **TLS Client Certificate** - 客户端证书认证
- **API Key** - 服务间认证

#### 权限模型
```
用户 → 角色 → 权限 → 资源
Admin → 系统管理员 → 所有权限
Operator → 运维人员 → 监控、配置权限
Viewer → 只读用户 → 查看权限
```

#### 状态
📋 **规划中** - MVP 稳定后加固安全

---

## 开发指南

### 开发顺序建议

1. **第一阶段：核心功能**
   - Core Runtime → Plugin Manager → Southbound + Northbound
   - 目标：保证主流程"采集→总线→上送"闭环可运行

2. **第二阶段：数据处理**
   - Rule Engine
   - 目标：插入总线后即可做数据转换/告警

3. **第三阶段：可视化**
   - Web UI & REST API
   - 目标：有基本功能后再做可视化，降低返工

4. **第四阶段：增强功能**
   - Security Layer, OTA & Versioning
   - 目标：MVP 稳定后加固与运维能力

### 模块交付物模板

每个模块应包含以下标准交付物：

```
/internal/<module>/
├── service.go           # 服务接口定义
├── impl.go             # 具体实现
├── config.go           # 配置结构
├── types.go            # 数据类型定义
├── test/               # 测试文件
│   ├── unit_test.go    # 单元测试
│   └── integration_test.go # 集成测试
└── README.md           # 模块说明

/docs/<module>.md        # 详细设计文档
/configs/examples/<module>_config.yaml # 配置示例
```

### 接口设计原则

1. **统一性** - 同类模块使用统一接口
2. **可扩展** - 接口支持未来功能扩展
3. **可测试** - 接口便于单元测试和 Mock
4. **向后兼容** - 接口变更保持向后兼容

### 错误处理规范

```go
// 统一错误类型
type ModuleError struct {
    Module  string `json:"module"`
    Code    string `json:"code"`
    Message string `json:"message"`
    Cause   error  `json:"cause,omitempty"`
}

// 错误码规范
const (
    ErrCodeConfigInvalid = "CONFIG_INVALID"
    ErrCodePluginNotFound = "PLUGIN_NOT_FOUND"
    ErrCodeConnectionFailed = "CONNECTION_FAILED"
    // ...
)
```

### 日志规范

```go
// 使用结构化日志
log.Info().
    Str("module", "plugin_manager").
    Str("plugin", pluginName).
    Str("action", "load").
    Msg("Plugin loaded successfully")

log.Error().
    Str("module", "modbus_adapter").
    Err(err).
    Str("device", deviceAddr).
    Msg("Failed to connect to device")
```

---

## 总结

通过模块化架构设计，IoT Gateway 实现了：

- ✅ **高内聚低耦合** - 每个模块职责明确，接口清晰
- ✅ **可并行开发** - 团队可同时开发不同模块
- ✅ **易于测试** - 模块独立，便于单元测试和集成测试
- ✅ **支持热插拔** - 插件可动态加载和卸载
- ✅ **易于扩展** - 新功能可作为插件或新模块添加
- ✅ **运维友好** - 提供完整的监控、日志和管理界面

当前系统已完成核心功能模块（模块 1-5），正在开发管理界面（模块 6），为生产环境部署做好了准备。