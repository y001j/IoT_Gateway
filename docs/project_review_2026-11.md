# IoT Gateway 项目评审报告（功能与性能）

- 评审日期：2026-11-24
- 评审范围：`master` @ `1c62bd1e`，以 Go 后端为主（`internal/`、`cmd/`、`plugins/modbus-sidecar`），前端只看了构建配置
- 评审方法：通读数据主链路的代码（南向适配器 → 插件管理器 → NATS → 规则引擎 → 北向 Sink → Web），编译并运行 `go vet`，用临时基准测试测量热点操作（测完已删除）

---

## 1. 总体结论

| 维度 | 评价 |
|---|---|
| 架构设计 | **方向正确**。南向/北向插件化、NATS 总线解耦、规则引擎独立、sidecar（ISP）扩展机制，这套分层是合理的。 |
| 功能完整度 | **覆盖面广，但关键路径有缺陷**。有几个缺陷会直接导致数据错误或系统失控（见第 2 节 P0）。 |
| 性能 | **达不到"高性能/百万级设备"的宣传水平**。主链路是单协程串行，Sink 同步阻塞，规则引擎会把消息成倍放大，热路径分配也很重。保守估计，在 ARM 网关硬件上实际吞吐只有每秒几千个点。 |
| 可靠性 | 整条链路是"最多一次"（at-most-once）投递，多处缓冲区满时会**静默丢数据**；没有断网缓存；`/health` 固定返回 healthy。 |
| 安全 | 默认配置不能用于生产：默认账号密码、JWT 密钥写死在配置里、CORS `*` 且允许携带凭证、WebSocket 不校验 Origin、Token 被打印到日志。 |
| 工程质量 | **没有任何 Go 单元测试**；CI 按现状会失败（vet 报错、`scripts/` 编译失败、前端缺少 `npm test` 脚本）。 |

**结论：现在适合做演示或 PoC，不建议直接上生产。** 先修完第 2 节的 P0 问题，再按第 6 节的路线做性能改造，是能达到"中等规模（每秒数万点）边缘网关"水平的。

---

## 2. P0：必须先修（会导致数据错误、系统失控或安全事故）

### 2.1 forward 动作默认会形成无限循环
`internal/rules/actions/forward.go:46-48`

```go
subject, ok := config["subject"].(string)
if !ok || subject == "" {
    subject = fmt.Sprintf("iot.data.%s.%s", point.DeviceID, point.Key)
}
```

规则引擎订阅的就是 `iot.data.>`（`config.yaml` 里的 `rule_engine.subject`）。所以只要 forward 动作没配 `subject`，或者配成了 `iot.data.*` 下的主题，转发出去的消息会再次被规则引擎收到，命中同一条规则后再次转发，形成**正反馈死循环**，几秒内就能把 CPU 和 NATS 打满。代码里也没有任何防环标记（比如 hop 计数或来源标签）。

**建议**：默认主题改为 `iot.rules.forward.<device>.<key>`；禁止 forward 目标落在规则引擎订阅的主题范围内；在消息里加入 `_hops` / `_origin_rule` 字段，收到自己发出的消息时丢弃。

### 2.2 Modbus 不配置 `scale` 时，所有读数都变成 `offset`（通常是 0）
`internal/southbound/modbus/modbus.go:323,334,345,356,368`；`internal/config/types.go:116`

```go
scaledValue := uint64(float64(value)*reg.Scale + reg.Offset)
```

`Scale` 是 `float64`，`omitempty`，没有默认值（`GetDefaultModbusConfig` 只设置了顶层字段）。寄存器没写 `scale` 时，值永远等于 `0*value + offset`。

另外，整数类型乘以小数系数后又被强转成整数：`int16` 读到 `235`、`scale: 0.1`，期望 `23.5`，实际得到 `23`。

**建议**：`Scale == 0` 时按 1 处理；只要配置了 scale 或 offset，就输出 `float64` / `TypeFloat`。

### 2.3 filter 动作实际上不起作用；同一规则内的动作不串联
- `internal/rules/actions/filter.go:110-120`：filter 只在结果里返回 `"filtered": true`。
- `internal/rules/service.go:907-940`：`executeAction` 只处理 `aggregate` 的返回值，**完全忽略 filter 的结果**，后续动作照常执行。
- 同一条规则的多个动作，每个都拿到的是**原始 point**（`service.go:765-767`、`892-893`）。transform 的结果不会传给后面的 forward 或 alert。
- 更根本的问题：原始数据在进入规则引擎**之前**就已经直接写进了所有 Sink（`internal/plugin/manager.go:607-614`）。所以规则引擎没法对入库数据做清洗或过滤，"数据过滤"这项能力在架构上就不成立。

**建议**：明确定义"动作管道"的语义（前一个动作的输出是下一个动作的输入，filter 命中就中断）；提供"先经规则再入库"的数据通路选项（Sink 订阅规则输出主题），而不是只能"原始数据直写 Sink"。

### 2.4 InfluxDB 写入的是写入时刻，不是采集时间戳
`internal/northbound/influxdb/influxdb.go:191-196`：`write.NewPoint(..., time.Now())`。

只要链路上出现排队、重放或批量延迟，时序数据的时间就是错的。这对时序库来说是严重问题。

**建议**：使用 `point.Timestamp`，为零值时再回退到 `time.Now()`。

### 2.5 规则任务被静默丢弃，上游却看不到
- `internal/rules/optimized_worker_pool.go:270-285`：分发器把任务转给 worker 时，如果 worker 队列已满就**直接丢弃**。此时 `SubmitTask` 已经返回了 true，调用方以为任务提交成功。
- `optimized_worker_pool.go:183-215`：入队失败时，在 **NATS 订阅回调里**最多阻塞等待 50ms（而且每次都新建一个 `time.After` 定时器）。等待失败后，`handleDataPoint` 会退回到**另一套**同步处理逻辑 `processRule`（`service.go:651-654`），这套逻辑的行为和 worker 池不一致（见 4.6）。
- 全项目都没有设置 `SetPendingLimits` 或 NATS 的 `ErrorHandler`。订阅回调一旦变慢，NATS 客户端会因为 slow consumer **静默丢弃消息**，没有任何日志或指标。

**建议**：用统一的有界队列加明确的背压策略（阻塞，或者丢弃并计数且可观测）；注册 NATS 错误回调并暴露 dropped 指标；关键数据走 JetStream 消费者。

### 2.6 默认安全配置可以直接被利用

| 问题 | 位置 |
|---|---|
| 默认账号 `admin / admin123`，首次登录不强制修改 | `internal/web/services/sqlite_store.go:154`、`memory_store.go:37` |
| JWT 密钥写死在仓库配置里：`your-secret-key-change-in-production` | `config.yaml` `web_ui.auth.jwt_secret` |
| 全局 CORS 设置 `Allow-Origin: *`，同时 `Allow-Credentials: true` | `internal/web/api/routes.go:14-17` |
| WebSocket `CheckOrigin` 恒返回 true（Web 端和 websocket Sink 都是） | `internal/web/api/websocket_handler.go:197`、`internal/northbound/websocket/websocket.go:101` |
| JWT Token 前 20 个字符用 `fmt.Printf` 打到标准输出；Token 允许放在 URL query 里传 | `internal/web/middleware/auth.go:33-46` |
| 任何已登录用户（非 admin）都能启停插件、增删改规则、管理告警通道 | `internal/web/api/routes.go:119-184` |
| 8080 端口的 `/metrics`、`/info`、`/health` 没有鉴权，也是 `*` CORS | `internal/core/runtime.go` 中的 HTTP 服务 |

**建议**：配置里没有密钥就拒绝启动（或启动时随机生成并持久化）；首次登录强制改密码；CORS 和 Origin 改为白名单；删除打印 Token 的代码；写操作统一加 `RequireRole("admin"/"operator")`；`jwt.Parse` 加上 `jwt.WithValidMethods([]string{"HS256"})`。

---

## 3. 性能分析

### 3.1 基准测试数据（Apple M4 Pro，Go 1.27.4，`-benchtime 1s`）

| 操作 | 耗时 | 内存 | 分配次数 |
|---|---:|---:|---:|
| 简单条件求值（`key == x AND value > 40`） | 257 ns | 112 B | 5 |
| **表达式条件求值**（`value > 40 && key == 'temperature'`） | **16,860 ns** | **17.7 KB** | **159** |
| **创建一个带 4 个 tag 的 Point**（Modbus 适配器每个点都这样做） | **2,684 ns** | **2.8 KB** | **40** |
| Point JSON 序列化 + 反序列化一次 | 6,467 ns | 4.8 KB | 68 |

说明：
- 表达式条件比简单条件**慢约 65 倍**。原因是每次求值都**新建一个 ExpressionEngine**（包括初始化地理区域数据、注册全部内置函数），并且每次都重新调用 `go/parser.ParseExpr`，没有缓存 AST（`internal/rules/evaluator.go:452`）。
- 光是创建一个 Point 就要 2.7µs、40 次分配。原因是每个 Point 的 tag 容器都是 `ShardedTags(16)`：16 个分片，每个分片有自己的锁和 map，外加统计结构（`internal/model/point.go:838-843`、`internal/utils/sharded_tags.go`）。一个 Point 只会被一个协程持有，带几个 tag，用分片锁完全是过度设计。规则引擎每次反序列化 Point 都要重建这套结构。
- 以上数据来自桌面级 M4 Pro。典型的 ARM 网关（Cortex-A53/A7）**慢 5～10 倍**。

### 3.2 吞吐瓶颈（按影响从大到小）

1. **主数据管道是单协程，Sink 写入是同步的**（`internal/plugin/manager.go:294-365`、`607-614`）
   所有适配器的数据都汇入一个 `dataChan`（容量 1000），由**一个 goroutine** 串行做三件事：组批、依次调用每个 Sink 的 `Publish()`、发布到 NATS。只要有一个 Sink 慢，整个网关的采集就会停下来：
   - Redis Sink 每个点执行一次 `SET`，没用 Pipeline，**每个点一次网络往返**（`internal/northbound/redis/redis.go:256-303`）。往返时间 0.5ms 时，整个网关的上限就是约 2000 点/秒。
   - JetStream Sink 每个点**同步**调用 `js.Publish` 等待 ACK，而且是在持锁状态下（`internal/northbound/jetstream/jetstream.go:274-300`）。
   - 当 InfluxDB 的 org/bucket 和默认值不同时，每个点都会用 `WriteAPIBlocking` 同步写一次（`influxdb.go:266-271`）。

   `dataChan` 满了以后，适配器端直接丢点（`internal/southbound/base_adapter.go:211-219`）。
2. **批次大小 10 写死，并且每批都调用 `nc.Flush()`**（`manager.go:345`、`665`）。Flush 是一次同步的 PING/PONG 往返，等于每 10 个点就阻塞一次。
3. **每个点要发布 (Sink 数 + 1) 次 NATS 消息**：每个 Sink 发一次 `data.<sinkName>`，再加一次 `iot.data.<dev>.<key>`（`manager.go:622-642`）。`data.<sink>` 这个主题在代码库里没有找到订阅方，属于无效流量。
4. **规则引擎的消息放大**（`internal/rules/service.go:732`、`2510-2570`）
   每个点、每条候选规则，**不论是否命中**，都会构造一个 map，序列化成 JSON，发布一条 `iot.rules.evaluated` 事件。在同步处理路径上，命中后还会再发 `matched`、每个动作一条 `action_executed`、最后一条 `executed`。
   举例：10 条通配规则、每秒 1 万个点，就是**每秒至少 10 万条事件**，比原始数据量大一个数量级。而 README 推荐的 `nats_subscriber` 正好订阅 `iot.rules.*`，会把这些事件全部转发进 InfluxDB 或 WebSocket。
5. **热路径上有 Info 级日志**：`handleDataPoint` 每收到一条消息打 3 条 Info（`service.go:588`、`628`、`658`），`processRule` 每条规则打 1 条 Info（"临时调试"，`service.go:681`），`enableMetrics=false` 时每次还会打 Warn。默认日志级别是 info，生产环境开着就是灾难。（当前 `config.yaml` 设成了 `error`，所以暂时没暴露出来。）
6. **重复的序列化和反射**：管道里每个点都执行一次 `fmt.Sprintf("%v", point.Value)`，只是为了估算字节数（`manager.go:327`）；filter 和 aggregate 每次执行都重新解析 `map[string]interface{}` 配置；聚合动作每次调用 `os.Getenv`（`service.go:971`）。
7. **Modbus 每个寄存器单独发一次请求**（`modbus.go:199-212`），没有合并连续地址。100 个寄存器、20ms RTT，一次轮询就要 2 秒。这是工业现场最常见的性能问题。

### 3.3 吞吐量粗略估算

以单个点在主链路上的 CPU 开销估算（M4 Pro）：创建 Point 约 2.7µs，NATS 序列化约 3µs × (Sink 数 + 1)，规则引擎反序列化约 3.5µs，每条候选规则约 0.3µs（简单条件）或 17µs（表达式），再加上每条候选规则一次事件序列化约 3～5µs。

- **理想情况**（无 Redis/JetStream Sink，简单规则，日志级别 error）：单核约 5～10 万点/秒（M4 Pro），ARM 网关约 0.5～1 万点/秒。
- **配置了 Redis 或 JetStream Sink**：被网络往返锁死，**约 1000～3000 点/秒**。
- "百万级设备连接"这个说法目前没有任何代码或测试支撑。

### 3.4 数据乱序
worker 池按轮询/最空闲策略分发任务（`optimized_worker_pool.go:298-325`），而且每个 worker 攒够 20 条或等满 10ms 才处理一批。**同一设备、同一 key 的连续点会被不同 worker 并行处理**，带来以下问题：
- 滑动窗口和 `change_rate`、`consecutive`、`duplicate` 这类依赖顺序的过滤结果不确定；
- 告警可能"先恢复后触发"；
- 每个点额外增加最多 10ms 延迟。

**建议**：按 `hash(device_id+key) % N` 固定分到某个 worker，保证同一序列的处理顺序。

---

## 4. 功能缺口与正确性问题

1. **内置 Modbus 不支持 RTU**：`modbus.go:84-86` 直接返回 `RTU mode not yet implemented`，和 README 宣传的"Modbus TCP/RTU"不符。RTU 只能用 `plugins/modbus-sidecar`。
2. **Modbus 不支持字节序/字序配置**：只支持 Big-Endian ABCD；大量 PLC 和仪表用的是 CDAB/BADC。也不支持 float64、字符串、位域，不支持写寄存器（**没有下行控制能力**，整个项目都没有 `Write*` 调用）。
3. **Modbus 重连没有退避**：每个轮询周期都尝试重连；`a.connected` 字段没有同步保护。
4. **没有断网缓存，也没有持久化**：适配器 → 规则引擎走的是 NATS Core（不持久化），Sink 写失败就丢弃，也没有本地队列重试。对边缘网关来说，这是核心能力缺失。
5. **`/health` 固定返回 healthy**（`internal/core/runtime.go:320`），不反映 NATS、适配器、Sink 的真实状态，没法用于 K8s 或监控探针。
6. **两套规则处理路径行为不一致**：`processRule`（同步回退路径）会检查 metrics、发布 4 类事件；`processRuleTaskInternal`（worker 池路径）检查 `rule.Enabled`，只发 1 类事件。同一条规则，负载不同时表现就不同。
7. **动作统计被重复计数**：`processRule` 记一次（`service.go:780`），`executeAction` 内部又记一次（`service.go:918`）。
8. **聚合分组键格式错误**：`service.go:1239` 用 `%d` 格式化字符串类型的 `device_id`，生成的键形如 `%!d(string=dev1)`（这也是 go vet 报出的问题）。
9. **Sink tag 互相污染**：`BaseSink.AddTags` 直接修改共享 batch 里各个 Point 的 tag 指针（`internal/northbound/base.go:201-210`）。前一个 Sink 加的 tag 会出现在后一个 Sink 和随后发布到 NATS 的数据里。
10. **指标快照复制了带锁的结构体**：`internal/metrics/lightweight_metrics.go:511` 的 `*metrics = *m` 复制了 `sync.RWMutex` 和原子字段，存在数据竞争（go vet 也报了）。
11. **规则索引的兜底策略**：索引没命中候选规则时会返回**全部规则**（`internal/rules/index.go`），数据点种类多的时候索引等于失效。

---

## 5. 工程质量

| 项 | 现状 |
|---|---|
| 单元测试 | 仓库里 **0 个** `*_test.go`。规则引擎、表达式、聚合这些逻辑复杂的模块完全没有回归保护。 |
| `go vet` | 3 处报错：`internal/northbound/base.go:259`（非常量格式串）、`internal/rules/service.go:1239`（`%d` 用于 string）、`internal/metrics/lightweight_metrics.go:511`（复制锁）。另外 `go test` 默认会跑 vet，所以测试包直接**构建失败**。 |
| 编译 | `scripts/verify_alignment.go` 有未使用的 import，`go build ./...` / `go vet ./...` 在这个包上失败。 |
| gofmt | 83 个文件不符合 `gofmt -s`，CI 的格式检查会失败。 |
| 前端 CI | `ci.yml` 执行了 `npm test`，但 `web/frontend/package.json` 里没有 `test` 脚本。 |
| 代码体量 | `transform.go` 4500 行，`expression.go` 2900 行，`service.go` 2600 行；同一功能有多套并存实现（聚合有 4 套、worker 池有 2 套、`sendBatch` 和 `sendBatchOptimized`），维护成本高。 |

---

## 6. 改进路线（按优先级）

### 第一阶段：正确性与安全（约 1～2 周）
1. 修复 forward 死循环（2.1）、Modbus scale（2.2）、InfluxDB 时间戳（2.4）、聚合分组键（4.8）。
2. 让 filter 和动作管道的语义生效（2.3）。
3. 加固安全默认值（2.6）。
4. 修好 vet、gofmt、`scripts` 编译和 CI 的 `npm test`，让 CI 变绿。
5. 给规则求值、表达式、聚合函数、Modbus 解析补上表驱动的单元测试。

### 第二阶段：吞吐与可靠性（约 2～4 周）
1. **把 Sink 和主管道解耦**：每个 Sink 有自己的有界队列和写协程，内部批量写（Redis 用 Pipeline，JetStream 用 `PublishAsync` 加窗口，InfluxDB 只用异步 WriteAPI）。慢 Sink 只影响自己。
2. 去掉 `data.<sink>` 冗余发布；去掉每批的 `Flush()`；批次大小和超时可配置（比如 500 条或 50ms）。
3. 规则事件默认关闭或采样，只保留 `matched` 和 `alert`；去掉热路径上的 Info/Warn 日志。
4. ExpressionEngine 做成单例或池化，**按表达式字符串缓存解析后的 AST**；规则加载时预编译条件和动作配置。
5. Point 的 tag 换成普通 `map[string]string`（或小切片），需要并发共享时再复制。
6. worker 按 `device_id+key` 哈希分片，保证顺序；所有丢弃都计数并对外暴露指标；注册 NATS slow consumer 回调。
7. 关键链路改为 JetStream，加本地断网缓存（store-and-forward）。
8. `/health` 反映真实组件状态。

### 第三阶段：功能完善
1. Modbus：内置 RTU、字节序/字序、连续地址合并读、写寄存器（下行控制）、重连指数退避。
2. 压测基线：写一个可重复执行的 benchmark 或负载测试（mock 适配器 + 若干规则 + 各类 Sink），在目标 ARM 硬件上给出 P50/P99 延迟和最大稳定吞吐，**用这些数据替换 README 里"百万级"的说法**。
3. 精简重复实现：聚合、worker 池、sendBatch 各保留一套。

---

## 附录：本次验证方式

- 编译：`go build ./...`（使用 go1.27.4 工具链；本机 Homebrew 的 go1.25.1 和 `GOROOT` 不匹配，需要 `env -u GOROOT` 才能用正确的工具链）。
- 静态检查：`go vet ./internal/...`、`gofmt -s -l .`。
- 基准测试：在 `internal/rules` 下临时放了一个 `_test.go`，用 `go test -vet=off -bench .` 运行，测完已删除，没有改动仓库代码。
- 第 3.3 节的吞吐估算是基于基准数据和代码路径分析的推算，不是实测压测结果；要确认真实水平，需要在目标硬件上跑第三阶段第 2 条的压测。
