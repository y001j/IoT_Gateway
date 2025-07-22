# Core Runtime 设计文档

> 版本：v0.1 &nbsp;&nbsp; 作者：Rocky Yang  &nbsp;&nbsp; 日期：2025-06-27

## 1. 目标
提供 **IoT Gateway** 的主进程运行时，负责进程生命周期管理、配置加载与热更新、内部消息总线、日志与指标、依赖注入等公共基础能力；为其它模块（Plugin Manager、Rule Engine、Web API …）提供统一的运行环境与服务发现。

## 2. 职责清单
1. **启动流程**：解析 CLI 参数 → 读取 `config.yaml` → 初始化 Logger/Metric → 启动消息总线 → 依次启动各子模块。
2. **配置中心**：基于 *Viper* + *fsnotify* 实现热更新，支持 YAML/JSON/TOML 与环境变量覆盖。
3. **内部消息总线**：封装 *NATS JetStream*（默认内嵌，亦可连接外部集群）。
4. **日志**：使用 *zerolog*，提供 JSON/Console 两种编码；支持动态调整 log level。
5. **指标**：集成 *Prometheus client_golang*，暴露 `/metrics` HTTP 端点。
6. **健康检查**：HTTP `/healthz`，返回各子模块状态。
7. **进程管理**：优雅停止 (SIGINT/SIGTERM)，按依赖拓扑逆序关闭组件。

## 3. 架构图
```
           ┌───────────────┐
           │   main.go     │
           └──────┬────────┘
                  │ Init
      ┌───────────▼──────────┐
      │     Core Runtime      │
      │───────────────────────│
      │ • Config Ctr          │
      │ • Logger              │
      │ • Metrics             │
      │ • MsgBus (NATS)       │
      │ • Service Registry    │
      └───────┬───────┬──────┘
              │       │
              ▼       ▼
       PluginMgr   RuleEngine …
```

## 4. 数据结构与接口
```go
// core.Service 统一生命周期接口
package core

type Service interface {
    Name() string
    Init(cfg any) error   // 同步初始化
    Start(ctx context.Context) error // 启动（阻塞式）
    Stop(ctx context.Context) error  // 优雅关闭
}

// Bus 抽象
package bus

type Bus interface {
    Publish(subject string, v any) error
    Subscribe(subject string, fn nats.MsgHandler) (*nats.Subscription, error)
}
```

## 5. 配置
```yaml
gateway:
  id: edge-001
  http_port: 8080
  log_level: info
  data_dir: /var/lib/iotgw
nats:
  embedded: true
  jetstream: true
```

## 6. 目录结构
```
internal/core/
    ├── config.go      // 配置加载 & 热更
    ├── logger.go      // 日志包装
    ├── metrics.go     // Prometheus exporter
    ├── bus/           // NATS 封装
    ├── runtime.go     // ServiceRegistry + Hook
    └── main.go        // 仅引用
```

## 7. 错误处理
- 所有服务初始化失败将使进程退出 (`log.Fatal`).
- 运行中错误通过 `Bus` 派发或记录日志，不直接崩溃。

## 8. 可测试性
- 使用 `testing` + `testcontainers-go` 启动内嵌 NATS。
- 配置热更新：修改临时文件后断言事件。

## 9. 性能预算
- 内存：<30 MB (ARMv7)
- 启动时间：<200 ms (x86_64)

## 10. 安全
- 日志脱敏 (注入 email/密钥过滤)。
- 消息总线 TLS/认证 留作 v1.1。

---
