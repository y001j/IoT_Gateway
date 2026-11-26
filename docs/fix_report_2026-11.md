# 修复报告：根据评审结果完善程序与补充单元测试

- 日期：2026-11-24
- 依据：`docs/project_review_2026-11.md`
- 验证结果：`go build ./...`、`go vet ./...`、`gofmt -s -l .`（无输出）全部通过；`go test -race ./...` 21 个测试包全部通过；`GOOS=linux GOARCH=arm GOARM=7` 与 `GOARCH=arm64` 交叉编译通过。

---

## 1. 评审问题处理情况

### P0（正确性与安全）

| 评审条目 | 处理 | 主要位置 | 对应测试 |
|---|---|---|---|
| 2.1 forward 默认主题导致死循环 | 默认主题改为 `iot.rules.forward.<device>.<key>`；所有规则输出带 `Iot-Origin: rule-engine` 消息头，规则引擎忽略此类消息（即使显式转发回 `iot.data.*` 也不会成环）；聚合结果改发 `iot.rules.aggregate.*` | `rules/service.go`（`PublishRuleOutput`、`handleDataPoint`）、`rules/actions/forward.go` | `TestForwardDoesNotLoop`、`TestForwardDefaultSubject`、`TestPublishRuleOutputHeader` |
| 2.2 Modbus `scale` 默认为 0、整数换算被截断 | scale 为 0 时按 1 处理；配置了换算时输出浮点数 | `southbound/modbus/modbus.go` | `TestParseRegisterDataScaling` |
| 2.3 filter 无效、动作不串联 | 运行时注册 actions 包中完整的 filter/transform/forward/aggregate 处理器（以前 filter 实际是只打日志的空实现）；动作管道：filter 命中终止后续动作，transform 输出作为后续动作输入 | `rules/actions/register.go`、`rules/service.go`（`processRule`、`applyActionResult`）、`core/runtime.go` | `TestFilterActionStopsPipeline`、`TestTransformOutputFeedsNextAction`、`TestRegisterBuiltinHandlers` |
| 2.4 InfluxDB 使用写入时刻 | 使用数据点采集时间 | `northbound/influxdb/influxdb.go`（`buildInfluxPoint`） | `TestBuildInfluxPointUsesAcquisitionTime` |
| 2.5 任务静默丢弃、slow consumer 无感知 | 工作池不再"提交成功后再丢弃"；队列满时有界等待，超时才丢弃并计数；不再回退到会乱序的同步处理；订阅设置更大的 pending 上限；注册 NATS 异步错误回调，slow consumer 计数并使健康检查降级 | `rules/optimized_worker_pool.go`、`rules/service.go`、`core/runtime.go` | `TestWorkerPoolDropsAndCountsWhenFull`、`TestDispatchPointCountsDrops`、`TestHealthStatus` |
| 2.6 默认安全配置 | 见下表 | | |

安全加固明细：

| 问题 | 处理 | 测试 |
|---|---|---|
| 默认账号 admin/admin123 | 初始密码取自 `IOT_GATEWAY_ADMIN_PASSWORD`，未设置时随机生成并写入 `<数据目录>/initial_admin_password`（0600）；前端登录页不再预填密码 | `TestMemoryStoreAdminPasswordFromEnv`、`TestMemoryStoreGeneratesRandomAdminPassword`、`TestLoginAndCORS` |
| JWT 密钥写死在配置中 | 配置为空/为旧占位值/过短时自动生成随机密钥并持久化到 `.jwt_secret`；签发与校验统一使用 `AuthService.JWTSecret()` | `TestEnsureJWTSecret`、`TestWeakJWTSecretIsReplaced` |
| JWT 算法未限制 | 只接受 HS256，且必须带 `exp` | `TestValidateJWTRejectsExpiredAndOtherAlgorithms`（含 alg=none、HS512） |
| CORS `*` + 凭证 | 改为白名单（`web_ui.cors.allowed_origins`），`*` 时不允许凭证；8080 端口同样处理 | `TestGinCORS`、`TestGatewayHTTPHandler` |
| WebSocket 不校验 Origin | Web 端和 websocket Sink 默认只允许同源和非浏览器客户端 | `TestCheckOrigin`、`TestDefaultOriginPolicy` |
| Token 打印到日志、可放在 URL | 删除所有令牌/claims/密码哈希打印；查询参数令牌仅限 WebSocket 升级请求 | `TestQueryTokenOnlyForWebSocket` |
| 普通用户可执行写操作 | 写操作需要 operator 或 admin；通知渠道、系统配置仅 admin | `TestWriteRoutesRequireOperator`、`TestNotificationChannelsAdminOnly`、`TestRequireRole` |
| 登录错误信息可枚举用户 | 统一为"用户名或密码错误" | `TestLoginErrorsDoNotRevealUserExistence` |

### 性能

| 评审条目 | 处理 | 效果 / 测试 |
|---|---|---|
| 3.2-1 单协程串行 + 同步 Sink | 每个 Sink 独立的有界队列和写入协程（`plugin/dispatcher.go`），慢 Sink 只影响自己并计数丢弃；Redis 改用 Pipeline；JetStream 改用 `PublishAsync`；InfluxDB 非默认 bucket 改为缓存的非阻塞 WriteAPI | `TestSlowSinkDoesNotBlockOthers`、`TestJetStreamSinkPublishesBatch` |
| 3.2-2 批次 10 写死、每批 Flush | 批次大小/刷新间隔/通道大小/队列长度可配置（`gateway.pipeline.*`，默认 100 / 50ms），去掉每批 `Flush()` | `TestPipelineFlushesByBatchSizeAndInterval` |
| 3.2-3 无人订阅的 `data.<sink>` 发布 | 移除 | `TestPipelinePublishesToNATS` |
| 3.2-4 规则事件放大 | `iot.rules.evaluated/action_executed/executed` 默认关闭（`rule_engine.publish_rule_events`） | `TestRuleEventsAreOptIn` |
| 3.2-5 热路径 Info/Warn 日志 | 规则引擎、插件管理器、transform 热路径日志降为 Debug 或删除 | — |
| 3.2-6 热路径无谓分配 | 去掉每点 `fmt.Sprintf` 估算；聚合环境变量启动时读取一次 | — |
| 表达式引擎每次新建、每次解析 | 引擎对象池 + AST 缓存，每次求值前清空变量 | 16.9µs/159 次分配 → **2.7µs/90 次分配**；`TestExpressionEngineConcurrentCorrectness` |
| 每个 Point 的 16 分片标签容器 | 改为单锁 + 惰性 map（API 不变）；原实现每次读写还要争用一把全局统计锁 | 创建带 4 个标签的点 2.7µs/40 次分配 → **0.28µs/3 次分配** |
| 3.4 数据乱序 | 工作池按 `hash(device_id,key)` 分片，同一序列按到达顺序处理；任务到达即处理，不再攒批等待 | `TestWorkerPoolPreservesPerSeriesOrder` |
| 规则索引失效（评审 4.11 的根因） | 原实现对设备/key/类型取并集且缺少条件的规则进入通配桶，几乎所有规则都是候选；改为只用必然成立的 AND 等值约束并取交集，不匹配时返回空 | `TestIndexIntersectsConstraints`、`TestIndexIgnoresNonNecessaryConstraints` |
| 3.2-7 Modbus 逐寄存器请求 | 同类型、地址间隔不超过 `max_read_gap`（默认 10）的寄存器合并为一次读取（不超过协议上限 125） | `TestBuildReadGroupsMergesContiguous`、`TestPollOnceReadsMergedGroup`（4 个点 → 2 次请求） |

### 功能缺口与正确性

| 评审条目 | 处理 | 测试 |
|---|---|---|
| 4.1 内置 Modbus 不支持 RTU | 支持 RTU（`serial_port`、`baud_rate`、`data_bits`、`parity`、`stop_bits`） | `TestInitConfig` |
| 4.2 无字节序配置、数据类型有限 | `byte_order`：ABCD/CDAB/BADC/DCBA（适配器级和寄存器级）；新增 int64/uint64/float64 | `TestByteOrders` |
| 4.3 重连无退避、阻塞启动 | Start 不再阻塞（以前设备不可达时最多阻塞 25 秒并导致适配器启动失败）；后台指数退避重连；连接状态原子化；Stop 后可再次 Start | `TestStartDoesNotBlockAndReconnectsWithBackoff` |
| 4.5 `/health` 固定 healthy | 汇总 NATS、运行时、插件、Sink 丢弃、规则引擎过载状态，unhealthy 返回 503 | `TestHealthStatus`、`TestGatewayHTTPHandler` |
| 4.6 两套规则处理路径不一致 | 合并为同一个 `processRule` | `TestDisabledRuleAndConditionMismatch` |
| 4.7 动作统计重复计数 | 只在 `executeAction` 记录一次 | `TestFailedActionDoesNotStopLaterActionsAndMetricsCountedOnce` |
| 4.8 聚合分组键 `%d` | 修复 | `TestGenerateGroupKey` |
| 4.9 Sink 之间标签互相污染 | 每个 Sink 拿到深拷贝的批次（`Point.Clone()`） | `TestDispatchBatchIsolatesSinks` |
| 4.10 指标快照复制锁 + 数据竞争 | 只复制导出字段，缓存使用独立锁 | `TestGetMetricsSnapshotConcurrent`、`TestSnapshotIsIndependent` |
| 设备 ID 含 `.` 等字符时主题错乱 | 新增可逆主题编码 `model.EncodeSubjectToken` / `DataSubject` / `RuleOutputSubject`，所有拼接主题处统一使用 | `TestEncodeDecodeSubjectToken`、`TestNatsBusPublishSubscribePoints` |
| Sink 初始化失败导致网关退出 | 适配器和 Sink 初始化失败都只记录错误并跳过 | `TestRepoConfigAdaptersInit` |

### 工程质量

| 评审条目 | 处理 |
|---|---|
| 0 个单元测试 | 新增 23 个测试文件、126 个测试函数、4 个基准测试；`internal/testutil` 提供内嵌 NATS 测试服务器 |
| `go vet` 3 处报错 | 全部修复，`go vet ./...` 无输出 |
| `scripts/verify_alignment.go` 编译失败 | 修复 |
| 83 个文件未 gofmt | 全部格式化，`gofmt -s -l .` 无输出 |
| 前端 CI 的 `npm test` 无脚本 | **未处理**，见第 3 节 |

---

## 2. 修复过程中新发现并修复的问题

这些问题评审时没有发现，是写测试或改代码时暴露出来的：

1. **多个 Sink 把 int64/uint64 写成 0 或整批失败**：Redis/MQTT/WebSocket Sink 的 `TypeInt` 只处理 `int`/`float64`，Modbus 产生的 `int64`/`uint64` 被写成 0；InfluxDB Sink 直接返回错误导致整批失败。新增统一的 `northbound.NormalizeValue`。（`TestNormalizeValue`、`TestBuildInfluxPointIntegerTypes`、`TestConvertValue`）
2. **表达式引擎伪造历史数据**：每次求值都用当前值 ±10% 生成假的 `last_values`，统计类表达式会在假数据上算出结果。已删除。（`TestExpressionEngineNoStaleVariables`）
3. **transform 修改共享标签、覆盖采集时间**：转换结果与原数据点共享标签容器，一个规则添加的标签会出现在其他规则对同一数据点的处理中；转换后时间戳被改为当前时间。（`TestTransformLookupOutputKeyTagsAndTimestamp`）
4. **HTTP 适配器资源泄漏**：轮询循环中的 `defer cancel()` / `defer resp.Body.Close()` 要等协程退出才执行，每次轮询泄漏一个上下文和一个未关闭的响应体，连接无法复用。（`TestHTTPAdapterPollsAndClosesResponses`）
5. **mock 适配器停止后可能继续产生数据**：Stop 替换 `stopCh` 时与采集协程存在数据竞争（`-race` 检出）。
6. **composite_mock 重复 Stop 会 panic**（close 已关闭的通道）；mqtt_sub Stop 后无法再次 Start。
7. **确认/解决告警接口必然 panic**：把 JWT 中 float64 类型的 `user_id` 断言为 string。（`TestWriteRoutesRequireOperator` 检出）
8. **`config_no_hotreload.yaml` 中的 mock 适配器配置格式已过时**，以前会导致使用该配置时网关启动失败。（`TestRepoConfigAdaptersInit` 检出）
9. **配置校验器的两个缺陷**：`oneof` 对未填写的可选字段也报错；切片中的结构体（如 Modbus 寄存器）从不校验。
10. **`CORSWithConfig` 的 Max-Age 头是乱码**（`string(rune(86400))`）。
11. **`cmd/tools/modbus_test` 编译失败**（引用已删除的 `Point.Tags`）。
12. **`docker-compose.yml` 挂载的 `configs/nats-server.conf` 不存在**，已补充（开启 JetStream）。
13. 新增的原子计数器使用 `atomic.Int64/Uint64` 类型，保证在 32 位 ARM 上对齐。

---

## 3. 未完成事项

| 事项 | 原因 / 建议 |
|---|---|
| 本地暂存的少数设计项 | 三个阶段均已实现，积压告警、指标和补发限速见第 8 节。未实现：死信流及其管理接口、每个 Sink 多个过滤主题、运维审计日志。详见 `docs/design_local_storage.md` 附录。 |
| ARMv7（32 位）实机性能 | 已用 QEMU 用户态模拟在 ARMv7 上运行全部测试和端到端压测（第 9.1 节），功能正确性已验证；模拟运行的吞吐不代表真机性能，实机数据需在设备上运行 `cmd/tools/benchmark`。 |
| 部分包的行覆盖率 | 所有包的主要功能都有测试，覆盖率见第 7.4 节。仍在 50% 左右的是 `rules/actions`（`transform.go` 4500 行复合数据转换分支）、`plugin`（sidecar 进程管理）、`metrics`。 |

---

## 4. 行为变化与升级注意

1. **规则输出主题变化**：订阅规则输出时请使用 `iot.rules.>`（不再是单级的 `iot.rules.*`），forward 默认主题为 `iot.rules.forward.<device>.<key>`。
2. **主题编码**：设备 ID / key 中含 `.`、`*`、`>`、空白、`~` 的，`iot.data.*` 主题中会被转义（例如 `plc.1` → `plc~2E1`）。普通 ID 不受影响。
3. **评估事件默认关闭**：依赖 `iot.rules.evaluated` 的外部系统需设置 `rule_engine.publish_rule_events: true`。
4. **首次启动的管理员密码**：新建数据库时不再是 admin123，请设置 `IOT_GATEWAY_ADMIN_PASSWORD` 或查看 `data/initial_admin_password`。已有数据库中的账号不受影响。
5. **JWT 密钥**：配置为空或为旧占位值时使用自动生成的密钥，已登录的用户需要重新登录一次。
6. **权限**：viewer/user 角色不能再启停插件、修改规则和告警；通知渠道仅 admin 可见。
7. **跨域**：默认不再允许跨域访问 API 和 WebSocket；前后端分离部署时请配置 `web_ui.cors.allowed_origins`（websocket Sink 使用其 `allow_origins`）。
8. **Modbus**：未配置 `scale` 的寄存器读数从"恒等于 offset"变为正确值；配置了小数系数的整数寄存器输出浮点数。
9. **filter 动作开始真正生效**，以前配置了 filter 但实际未过滤的规则，行为会发生变化。

---

## 5. 第二轮：补齐测试与新发现问题的修复

### 5.1 测试补齐

新增测试覆盖了第一轮未覆盖的全部模块，所有测试都不依赖外部服务：

| 模块 | 测试方式 |
|---|---|
| MQTT Sink、mqtt_sub 适配器 | 内嵌 NATS 开启 MQTT 协议作为测试 broker（`testutil.StartNATSWithMQTT`） |
| Redis Sink | 测试内实现的最小 RESP 服务器，验证 Pipeline 批量、键格式、过期时间、采集时间戳 |
| InfluxDB Sink | 模拟 `/api/v2/write` 的 HTTP 服务器，验证多 bucket 写入、采集时间戳、坏点跳过 |
| WebSocket Sink | 真实 WebSocket 客户端，验证广播、点位配置、跨域拒绝 |
| nats_subscriber、console Sink | 内嵌 NATS + 捕获型目标 Sink |
| ISP sidecar 协议 | 模拟 sidecar 的 TCP 服务器：请求/响应、数据顺序、>64KB 消息、心跳、断线检测 |
| monitoring | 系统指标采集、阈值、启停、配置更新 |
| 规则引擎 | 表达式内置函数、内置告警（节流/模板/webhook/NATS）、规则管理器（JSON/YAML、增删改、持久化、热加载）、全部过滤类型、其余转换类型、两种聚合实现与统计 |
| Web | 告警/规则/插件服务、SQLite 存储完整认证流程、HTTP 接口端到端（认证、规则 CRUD、告警生命周期、系统/插件接口） |
| 整体 | `TestRuntimeEndToEnd`：用真实配置文件启动网关，验证 mock 适配器 → 管道 → NATS → 规则引擎（filter+alert）→ 告警，以及 /health 和优雅停止 |
| 仓库数据 | `TestRepoConfigAdaptersInit`、`TestRepoRuleFilesAreValid`：仓库中的配置文件和示例规则都能通过校验 |
| 前端 | `npm test`（`scripts/run-unit-tests.mjs` + `node:test`，无需额外依赖），覆盖 `src/utils/format.ts` |

统计：Go 测试包 26 个全部通过（`go test -race ./...`），前端 4 个测试通过；`go vet` 无报错，`gofmt -s -l .` 无输出，ARMv7/ARM64 交叉编译通过。

### 5.2 新发现并修复的问题

**安全**
1. 每次登录把**明文密码**打印到日志；MQTT Sink 初始化打印包含 broker 密码的完整配置。
2. 登录失败锁定（`max_login_attempts`/`lockout_duration`）从未生效 → 已实现（按用户名计数，不区分用户是否存在）。
3. **令牌无法吊销**：JWT 中间件不检查会话，登出、修改密码后旧令牌在过期前仍然有效 → 中间件对每个请求校验会话；登出删除会话；修改密码删除该用户所有会话。
4. 登出接口必然返回 500（把整个 `Authorization` 头当作令牌查找会话）。
5. SQLite 存储读出的会话 `Active` 恒为 false（表中没有该列），导致 `ValidateToken` 和刷新令牌在生产默认存储下必然失败；`UpdateSession` 也无法持久化失效状态。
6. 同一用户同一秒内签发的 JWT 完全相同，按令牌查找会话会混淆 → 增加 `jti`、`iat`。
7. 告警通道（邮件/短信）日志会输出包含密钥的完整配置 → 敏感字段打码（`rules.RedactConfig`）。

**可靠性**
8. ISP 客户端：sidecar 10 秒无数据或断开后接收循环永久停止并 100% CPU 空转，连接状态仍显示已连接；>64KB 消息同样导致停止；断开时有等待中的请求会 panic；数据消息乱序。
9. monitoring：`UpdateConfig` 与采集协程数据竞争、网络计数器并发竞争、`UpdateConfig` 不填默认值、`UpdateConfig` 后 `Stop` panic、初始采集失败后无法再次启动。
10. MQTT/Redis 在网关启动时不可达会导致启动无限阻塞或连接器被永久跳过；MQTT 健康检查在重连期间误报健康。
11. WebSocket Sink 广播无写超时，一个卡住的客户端会阻塞所有客户端；停止后新连接的协程永久阻塞。
12. `TimeSeriesData`/`MatrixData` 空数据计算衍生值时 panic（外部输入可触发）。

**正确性**
13. 规则管理器：校验失败的规则仍被加载；单规则 YAML 文件被静默忽略；启用/禁用状态重启后可能回退；删除定义在多规则文件或 YAML 文件中的规则后重启会"复活"；校验器比求值器更严格（要求显式操作符、AND/OR 至少 2 个子条件），启用校验后会误拒合法规则 → 已与求值器对齐，并拒绝未知操作符。
14. 表达式预处理会改写字符串字面量中的方括号和 `tags.x`（例如 `regex('^dev-[0-9]+$', id)` 被破坏）。
15. 高性能统计把值为 0 的读数排除在百分位/阈值/异常值计算之外；下限为 0 或负数的阈值被忽略。
16. 聚合结果在配置多个函数时随机取值（遍历 map）；优化聚合的结果因类型断言失败而丢失 → 按 `functions` 配置顺序确定性取值，两种结果格式统一处理。
17. pattern 过滤的 `?` 和中间 `*` 不生效；consecutive 过滤缺少配置时 panic。
18. nats_subscriber 无法解析规则输出（扁平数据点格式）；`contains` 实际做相等比较；`eq` 对 `type` 字段永远不成立；多个目标 Sink 共享同一批数据。
19. console Sink 忽略 `batch_size` 和标签配置。
20. `ConfigParser.ParseFromManager` 不应用默认值。
21. 前端 `formatBytes` 对小于 1 或超过 TB 的值输出 `undefined`。

### 5.3 精简
- 删除 `service.go` 中重复的聚合实现（约 400 行）及 `sharded_aggregate_states.go`；聚合统一由动作处理器提供，`IOT_GATEWAY_ENABLE_OPTIMIZED_AGGREGATE=true` 时使用 `OptimizedAggregateHandler`，否则使用 `AggregateHandler`。
- 删除未被引用的 `utils/thread_safe_tags.go`、`utils/tags_migration.go`。

### 5.4 新增行为变化
- 修改密码、登出后，对应令牌立即失效；网关升级后所有用户需要重新登录一次（新令牌带 `jti`）。
- 连续登录失败达到 `max_login_attempts` 次后，该用户名锁定 `lockout_duration`。
- 规则文件中不合法的规则（以及使用未知操作符的条件）不再被加载，会在日志中给出原因。

---

## 6. 第三轮：剩余模块测试、断网续传、压测工具

### 6.1 断网续传（评审 4.4 节）
按 `docs/design_local_storage.md` 第一阶段实现：
- `storage.enabled: true` 时，所有采集数据写入内嵌 JetStream 持久化流 `IOT_RAW`（文件存储、S2 压缩、按 `max_age`/`max_bytes` 淘汰最旧数据）；内嵌 NATS 的磁盘上限和刷盘间隔由 `storage.max_disk`、`storage.sync_interval` 控制。
- 连接器配置 `buffer.enabled: true` 后，由 `SinkRunner` 通过 JetStream 持久消费者驱动：写入成功（`BatchWriter` 同步确认）才确认消息；目标不可用时数据留在本地、按退避重试，恢复后**按原顺序**补发；网关重启后从断点继续；永久错误丢弃并计数；因超出保留上限而丢失的数据量可精确统计。
- 状态：`GET :8080/storage`；`/health` 新增 `storage` 组件。
- 测试：`internal/storage`（断线恢复顺序、重启续传、健康门控、淘汰计数、永久错误、跳过规则输出、`start_from=all`）、`TestRuntimeStoreAndForward`（整机：目标断线 → 本地积压 → 恢复后按序补发）、各 Sink 的 `WriteBatch` 错误分类测试。
- 压测中发现并修复：逐条异步确认导致每次拉取都等到超时，补发速度只有约 3000 点/秒 → 改为每批一次同步确认，补发速度约 24.6 万点/秒。

### 6.2 补齐测试与新发现的问题
- 新增测试：transform 全部复合数据转换类型、ISP 适配器代理（模拟 sidecar）、Web 系统/配置/通知/日志服务、SQLite 认证流程、令牌吊销。
- **令牌吊销**：中间件对每个请求校验会话；登出删除会话，修改密码删除该用户全部会话；JWT 增加 `jti`（同一秒内签发的令牌以前完全相同）；修复 SQLite 会话 `Active` 恒为 false 导致令牌校验/刷新必然失败的问题。
- 配置服务 `GetConfig` 在未加载配置时**必然死锁**（持有读锁调用需要写锁的 `LoadConfig`）；配置文件（含 JWT 密钥）权限改为 0600。
- `GET /system/config` 返回明文 JWT 密钥 → 返回遮盖值；提交遮盖值时保留原密钥。
- `RestartService` 什么也不做却返回成功 → 如实返回"不支持在线重启"（HTTP 501）。
- 通知邮件正文未转义告警内容（HTML 注入）。
- ISP 适配器代理：重连时无锁替换客户端（数据竞争）；重连等待不响应停止；`int64/uint64/double` 等类型的数据点被丢弃；sidecar 未提供时间戳时数据点时间为 1970 年。
- WebSocket Sink：广播无写超时，一个卡住的客户端阻塞所有客户端。
- 数据模型：空的时间序列/矩阵数据计算衍生值时 panic。
- `ConfigParser.ParseFromManager` 不应用默认值；规则管理器删除 YAML 规则后重启会复活。
- 删除未被引用的 `utils/thread_safe_tags.go`、`utils/tags_migration.go`。

### 6.3 压测工具与本机结果
`cmd/tools/benchmark`：使用生产运行时启动网关（高速生成数据的适配器 → 管道 → NATS → 规则引擎 → 计数连接器），输出采集/连接器/规则任务吞吐、丢弃、积压和内存。

```bash
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o bench ./cmd/tools/benchmark   # 或 GOARCH=arm64
./bench -duration 30s -devices 100 -rules 10 -expr-rules 2          # 直连模式
./bench -duration 30s -rules 10 -storage                            # 开启本地暂存
```

本机（Apple M4 Pro，12 核）：直连模式 10 条简单规则 + 2 条表达式规则，端到端约 **36.8 万点/秒**，规则任务约 66 万次/秒，平均延迟约 28µs，无丢弃。ARM 网关的数据需在设备上运行上述命令获得。

### 6.4 验证
`go test -race ./...` 26 个测试包全部通过（含 `internal/storage`），`go vet` 无报错，`gofmt -s -l .` 无输出，ARMv7/ARM64 交叉编译通过；前端 `npm test` 通过。

---

## 7. 第四轮：ARM64 实测、近期数据查询与汇总层、补齐低覆盖率模块

### 7.1 Linux/ARM64 实测
在本机的 Linux/ARM64 虚拟机（lima，4 核，aarch64 内核 6.17）上：
- 26 个测试包全部通过（交叉编译测试二进制后在虚拟机中运行）。
- 端到端压测（`cmd/tools/benchmark`）：直连模式 10 条简单规则 + 2 条表达式规则，约 **33.7 万点/秒**，无丢弃，堆内存 206MB；开启本地暂存后，缓冲连接器在 5 万点/秒时实时跟上（积压为 0），10 万点/秒时只能处理约一半。
- 修复压测工具：`-rate` 以前用微秒级 Ticker，实际速率远低于设定值（设 2 万只有约 4100）；改为每 10ms 成批生成。新增 `-buffer-batch` 参数。

### 7.2 本地暂存第二、三阶段
- **近期数据查询**（`internal/storage/query.go`）：`QueryRaw`（按序列读取、按采集时间过滤排序、`avg/min/max/last/lttb` 降采样、并发/扫描量/超时限制）、`ListSeries`、`Latest`。24 小时单序列查询在 ARM64 上约 0.18 秒（验收目标 2 秒）。
- **汇总层**（`internal/storage/rollup.go`）：`storage.rollup.enabled` 后按窗口汇总 count/sum/min/max/last 写入 `IOT_ROLLUP` 流；按事件时间关闭窗口，重启不丢未写出的窗口；`Store.Query` 按时间范围自动选择原始层或汇总层。
- **运维**：暂停、恢复、跳过积压、从指定时间重放（`Manager.ControlSinkBuffer`、`ResetSinkRunner`）。
- **Web 接口**：`/api/v1/data/{series,latest,history,export}`、`/api/v1/storage/status`、`/api/v1/storage/sinks/:name/:action`（仅管理员）。
- **前端**：新增"历史数据"页（侧边栏），含本地存储状态卡片和运维按钮；纯函数在 `src/utils/history.ts`，已加入前端单元测试。
- 实现过程中发现：推送式消费者的 `ConsumerInfo().NumPending` 在创建后立即变为 0，不能用来判断查询何时结束；只按入库时间提前结束会漏掉延迟上报的数据。两处都已处理并有测试。

### 7.3 补齐测试过程中发现的问题
| 模块 | 问题 | 修复 |
|---|---|---|
| mqtt_sub | 消息不是 JSON 对象时（纯文本、`23.5` 这类单个数值），解析错误没有清除，**消息全部被丢弃** | 按配置类型转换整条消息；复合类型要求 JSON 对象；字符串类型保留原文 |
| Web 系统接口 | `GET /api/v1/system/alert-status` 必然空指针 panic（处理器没有服务容器） | `NewSystemHandlerWithServices` |
| 适配器监控 | "重启适配器"只记日志就返回成功，实际没有重启 | 调用插件管理器重启，不存在时返回 404 |
| WebSocket | 客户端最后活动时间被读、写、广播三个协程无锁并发读写（数据竞争） | 改为原子变量 |
| 告警服务 | `GetAlert/CreateAlert/UpdateAlert` 返回内部指针，调用方在锁外读取时与解除/更新存在数据竞争 | 返回副本 |
| 系统告警 | 告警解除后不从缓存删除，每个检查周期重复解除；堆大小为 0 时内存使用率为 +Inf 导致误报 | 解除后删除；分母为 0 时视为 0 |
| 规则服务（Web） | 校验要求条件必须写 `type`，通过本服务创建的规则测试时必然失败；测试规则用另写的简化求值（只支持少数操作符和 float64） | 复用 `rules.ValidateRule`（新导出）和规则引擎的 `Evaluator` |
| 规则服务（Web） | 规则统计、执行历史返回 `rand` 生成的**假数据** | 如实返回 `ErrRuleStatsUnavailable` |
| 插件服务 | 没有日志文件时返回编造的"插件启动成功"等示例日志；名为 mock 的插件统计返回写死的调试数字；每次查询以 Info 级别输出调试日志 | 返回空列表；删除调试数据；日志降为 Debug |
| 表达式引擎 | `vectorNormalize` 注册了两个同名函数，三维用法 `vectorNormalize(x,y,z)` / `vectorNormalize(<Vector3D>)` 被通用向量版本覆盖，必然报错 | 通用版本遇到三维参数时转交 |
| 正则缓存 | 淘汰时选择得分**最低**的条目，淘汰的恰恰是最近、最常用的正则；缩小上限不立即生效 | 改为淘汰最高分；`SetMaxSize` 立即淘汰 |
| 规则错误分类 | `IsRetryableError/GetErrorType/GetErrorLevel` 用类型断言，被 `%w` 包装后无法识别 | `errors.As` |
| geo | `ValidateGeoData` 只在两个字段都缺失时才查别名，只缺经度时不报错；五份重复的 `toFloat64` | 逐字段查找；合并为一个函数 |

### 7.4 覆盖率（`go test -cover ./...`，本轮变化）
| 包 | 之前 | 现在 |
|---|---|---|
| rules/geo | 27.7% | 99.2% |
| southbound/mqtt_sub | 39.3% | 94.7% |
| storage | 83.3% | 88.4% |
| web/api | 28.3% | 77.5% |
| web/services | 33.5% | 72.7% |
| core | 48.0% | 68.6% |
| rules | 46.6% | 61.4% |

其余包不变（model 89.7%、influxdb 91.8%、modbus 91.2%、utils 98.8% 等）。

### 7.5 验证
`go test -race ./...` 26 个包全部通过；Linux/ARM64 上 26 个包全部通过；`go vet` 无报错；`gofmt -s -l .` 无输出；ARMv7/ARM64 交叉编译通过，`scripts/verify_alignment.go` 通过；前端 `npm test` 9 项通过，`npm run build` 成功，新增文件 ESLint 和 `tsc` 无报错（前端其余文件原有约 95 个类型错误，不在本次范围）。

---

## 8. 第五轮：积压告警与指标、补发限速、数据流统计、内存告警

### 8.1 本地暂存积压告警与 Prometheus 指标
- 新增 `storage.AlertMonitor`（默认开启，`storage.alerts`）：积压逼近保留上限（时长或容量，warning 50% / critical 80%）、有数据在写入前被淘汰、数据目录所在磁盘快满时，以规则引擎告警的格式发布到 `iot.alerts.triggered`，进入 Web 告警和通知渠道；同一告警 30 分钟内只发一次，级别升高立即发送。
- `/health` 的 `storage` 组件与告警条件一致。**修正**：上一轮按流占用率超过 90% 降级，而流是环形缓冲区，正常写满就会误报。
- `/metrics?format=text` 增加 `iot_gateway_storage_*` 指标；`/storage`、`/api/v1/storage/status` 返回当前告警，历史数据页显示告警。

### 8.2 补发限速
`buffer.replay_rate`（点/秒）：每批写入成功后按速率推迟下一批，空闲期间不累积额度，避免断网恢复后的补发挤占窄带链路或超过云端写入配额。

### 8.3 适配器监控的数据流统计
以前只要开启 Web 界面就常驻订阅 `iot.data.>`，每条消息都做 JSON 解析并获取全局锁；速率计算也有错误（记录最多 1000 条，"每秒点数"有上限；"每秒字节数"用累计字节除以时间范围）。改为：
- 按需订阅：首次查询数据流时才订阅，5 分钟无人查询自动退订；
- 每条消息只做原子计数（无锁、无解析），每个序列每秒最多解析一条用于最新值和延迟；
- 速率由每 10 秒的计数快照计算；订阅设置了待处理上限，高负载时丢弃统计消息而不是拖慢连接。

### 8.4 系统内存告警
以前按"已分配堆 / 向系统申请的堆"计算内存使用率，Go 程序正常运行时这个比值经常超过 85%，会误报。改为系统内存使用率（系统采集器，gopsutil），告警中附带网关进程堆内存；采集不到系统内存时不判断。`LightweightMetrics` 增加 `system_memory_percent` 等字段和 `iot_gateway_system_memory_percent` 指标。

### 8.5 验证
`go test -race ./...` 26 个包全部通过（storage 89.7%、core 72.4%、web/services 70.4%）；Linux/ARM64 上 26 个包全部通过；`go vet` 无报错；`gofmt -s -l .` 无输出；ARMv7/ARM64 交叉编译和 32 位对齐检查通过；前端 9 项测试通过，构建成功。

---

## 9. 第六轮：ARMv7 实测、CI、剩余低覆盖率模块、前端类型与 lint

### 9.1 ARMv7（32 位 ARM）
- 方法：在 ARM64 虚拟机中解压 `qemu-user` 软件包（不安装），用 `qemu-arm` 运行 `GOARCH=arm GOARM=7` 编译的测试二进制。
- **发现严重问题**：`northbound.BaseSink` 中被原子操作访问的 int64 计数器位于偏移 20，ARMv7 上**所有北向连接器写数据时都会 `panic: unaligned 64-bit atomic operation`**。以前的"32 位对齐修复"只调整了 `SinkStats` 内部字段顺序，`scripts/verify_alignment.go` 只检查结构体顶层字段，未能发现。修复：`stats` 移到 `BaseSink` 第一个字段。
- **第二个只在 32 位平台出现的问题**：分片聚合用 `int(hash.Sum32()) % numShards` 计算分片，32 位平台上 `int` 只有 32 位，哈希值大于 2^31 时变为负数，得到负下标——开启优化聚合（`IOT_GATEWAY_ENABLE_OPTIMIZED_AGGREGATE=true`）时 ARMv7 上约一半的序列会 panic。改为先在 `uint32` 上取模。
- 修复后 26 个测试包在 ARMv7 上全部通过；端到端压测在模拟下完整运行（采集 → 规则引擎 → 连接器、本地暂存），无 panic、无丢弃。
- CI 新增 **ARMv7 Tests (QEMU)** 任务（`docker/setup-qemu-action` + `GOARCH=arm go test ./...`），防止再次出现此类问题。

### 9.2 CI 配置
- `actions/upload-artifact`、`download-artifact` v3 → v4（v3 已被 GitHub 停用，任务启动即失败）。
- `securecodewarrior/github-action-gosec`（仓库已不存在）→ `securego/gosec@v2.22.4`；`codeql-action/upload-sarif` v2 → v3。
- 修复后仍会失败的两项属于项目原有问题，（已在第 10 节清零）：前端 lint 要求零警告（约 230 个 `any`/hooks 依赖警告）；gosec 报告 220 项（90 个 G404 `math/rand`、65 个 G104 未处理错误、46 个 G115 整数转换等）。其中真实问题已修：WebSocket 连接器 HTTP 服务缺少 `ReadHeaderTimeout`（Slowloris）、数据目录权限 0755 → 0750。

### 9.3 补测试中发现并修复的问题
| 模块 | 问题 | 修复 |
|---|---|---|
| 增量统计 | `GetStdDev` 在缓存失效时持有写锁调用 `GetVariance`（再加读锁），**调用方永久卡死** | 内部 `varianceLocked` |
| 增量统计 | `GetNullRate/GetChange/GetVolatility/GetAboveCount` 等 10 个导出方法不加锁，与 `AddValue` 数据竞争 | 导出方法加读锁，内部实现供 `GetStats` 使用 |
| 分片聚合 | `UpdatePerformanceConfig` 与批处理协程并发读写 `batchSize/flushInterval` | 原子类型 |
| 插件管理器 | `StartPlugin` 把实例名当作类型查找：配置中 `name: mock1, type: mock` 的实例停止后无法再启动，`RestartPlugin`（含 Web 的"重启适配器"）会把运行中的实例停掉；单独启动的连接器配置格式与启动时不一致 | 按配置中的实例启动；连接器配置生成提取为 `sinkConfigData` 共用 |
| 告警动作（actions） | 邮件、短信渠道什么也不做却返回发送成功 | 返回 `ErrChannelNotImplemented` |

覆盖率：metrics 41.5% → 88.3%，northbound/jetstream 57.7% → 77.5%，plugin 49.6% → 65.4%，rules/actions 48.5% → 58.2%。

### 9.4 前端
- 能访问到的页面：TypeScript 错误 0、ESLint 错误 0（以前 tsc 95 个、lint 141 个错误）。修复中包括：缺少 `vite-env.d.ts`；`SystemStatus/SystemMetrics/HealthCheck` 类型与后端不一致（系统页显示 undefined 的字段）；连接监控页把网关总数据点数**平均分给每个适配器/连接器**冒充各自的数量，改为插件统计接口的真实值；规则编辑器保存时把不完整的规则直接提交；`websocketService` 的 async Promise 执行器（异常不会使 Promise 失败）；`<style jsx global>`（Vite 不支持）；列表缺少 key；以及约 90 个未使用的变量/导入。
- **设置模块未接入**：`SettingsPage` 及 11 个设置组件没有挂到路由上，调用的约 30 个接口（`/settings/*`、`/roles`、`/users`、`/audit-logs`）后端均未实现；剩余 15 个 tsc 错误和 29 个 lint 错误都在这里。修复了其中会导致渲染崩溃的 `Text` 未定义、不存在的图标 `TestOutlined`。（已在第 10 节删除。）

### 9.5 验证
本机 `go test -race ./...` 26 个包全部通过；ARMv7（QEMU）26 个包全部通过；`go vet`、`gofmt -s -l .` 无输出；前端 `npm test` 9 项通过、`npm run build` 成功。

## 10. 第七轮：删除设置模块、前端 lint 清零、gosec 清零

### 10.1 前端
- **删除未接入的设置模块**：`SettingsPage`、`components/settings/*`（11 个文件）、`settingsService`、`types/settings`。它们没有挂到路由上，调用的接口后端均未实现。
- **ESLint 警告 229 → 0**（CI 的 `--max-warnings 0` 规则不变）：
  - `any` 全部换成具体类型。新增 `types/realtime.ts`、`utils/errors.ts`（`asHttpError`，用于 catch 块）、`utils/ruleData.ts`（读取自由格式的动作配置）、`utils/compositeData.ts`，均有单元测试。
  - hooks 依赖逐个审查：数据加载函数改为 `useCallback` 并列出真实依赖；只需在挂载时或认证完成后执行一次的加载，用 ref 调用最新的函数；列表页只依赖 `current/pageSize`，不依赖 `total`，避免"更新 total → 重新请求"的循环。
- **类型收紧过程中发现的问题**：
  - 5 个规则编辑器的 `writeRuleJson` 读到的是过期的条件和动作（闭包陈旧），改为通过 ref 读取。
  - 插件详情弹窗的 8 个请求混用了显示名和插件名，统一使用 `plugin.name`。
  - 切换插件时日志不会重新加载。
  - 图表 CSV 导出的 subject 字段取错。
  - 复合数据选择器只读 `values`，读不到 `elements`。
  - 系统监控图表读取不存在的 `network_*_per_sec` 字段。
  - 规则列表"清空筛选"的语义改为传 undefined（不再传 false）。
- 前端单元测试 9 → 14 项。

### 10.2 gosec（220 → 0，标注 29 处）
- **G104 未处理错误（65）**：
  - 有实际影响的已改为处理错误：
    - Web WebSocket 的 NATS 订阅失败会被静默忽略，现在记录日志。
    - 设置写超时失败时退出写协程。
    - WebSocket 连接器写入失败时关闭客户端。
    - JetStream 的 Ack/Nak 失败现在记录。
    - 告警 ID 的 `crypto/rand` 失败时会生成全零 ID，改为回退到时间戳。
    - 告警通知渠道配置格式错误现在记录警告。
    - benchmark 工具写文件失败时直接退出。
    - modbus sidecar 发送响应失败时记录日志。
  - 其余的 Close/Disconnect 调用显式写成 `_ =`。
- **G404（90）**：只出现在模拟适配器中，用来生成演示用的传感器数据。随机数集中到各自包的 `simrand.go`，并加注 `#nosec`。
- **G115 整数转换（47）**：
  - 字节数、包数计数器用 `utils.SaturatingInt64` 做饱和转换。
  - `validateMin/validateMax` 遇到负的 min/max 且字段是无符号类型时，以前会得到错误结果，已修正。
  - 端口号为无符号类型时检查是否超过 65535。
  - `ToInt64(uint)` 溢出时返回失败。
  - Modbus 有符号寄存器的补码解释、哈希取模等确认安全的转换，加注 `#nosec` 并写明理由。
- **G301/G306**：规则目录、配置目录、插件目录改为 0750，规则文件改为 0600。
- **G304**：路径来自配置，统一做 `filepath.Clean`。
- **G402**：`skip_verify` 只在用户显式配置时生效，加注说明。
- **modbus sidecar 原来无法编译**：`go.sum` 缺少 gopsutil 依赖，已执行 `go mod tidy`。

### 10.3 验证
- 后端：
  - 本机 `go test -race ./...` 26 个包全部通过。
  - `go vet`、`gofmt -s -l .` 无输出。
  - ARMv7 交叉编译通过。
  - gosec（Go 1.24）`Issues: 0`。
- 前端：
  - `npm run lint` 0 警告。
  - `tsc --noEmit` 0 错误。
  - `npm test` 14 项通过。
  - `npm run build` 成功。

## 11. 第八轮：函数级覆盖、死代码清理、补测试中发现的问题

### 11.1 方法
- 用 `go test -coverpkg=./internal/... ./internal/...` 做跨包统计。默认的按包统计只计入本包测试执行的代码，`testutil` 等被其他包使用的函数会显示为 0%。
- 用 `deadcode -test ./...` 找出从任何程序入口和测试都不可达的函数：
  - 内部死代码直接删除；
  - 公开的小 API（默认配置函数）补测试。
- **结果**：
  - `internal/...` 中 1813 个函数全部被测试执行到（`internal/test` 下的手动工具程序除外），`deadcode -test` 输出为空。
  - 跨包语句覆盖率 82.1%（上一轮按包统计 63.9%）。

### 11.2 删除的死代码
| 位置 | 内容 |
|---|---|
| `rules/utils.go` | `SafeStringMap` 及一串 map 拷贝函数（19 个） |
| `rules/predefined_data_types.go` | 字段推断（10 个） |
| `rules/types.go` | `CircularBuffer` |
| `rules/evaluator.go` | 只写不读的内置函数表（9 个函数类型，表达式函数实际由 `ExpressionEngine` 提供） |
| `rules/service.go` | 5 个从未调用的告警渠道函数 |
| `rules/service.go` | transform/alert 的重复简化实现（运行时由注册的处理器执行，transform 还直接拼接 NATS 主题） |
| `rules/service.go` | 存在数据竞争且不起作用的 `SetHotReloadEnabled` 开关 |
| `rules/expression.go` | 返回"未实现"的 `in_array` 空实现 |
| actions 包 | 3 个无人调用的辅助函数，以及 `HighPerformanceStats.AddNull` |
| transform | 5 个伪重采样函数（被真实实现替换） |
| web/api | 未挂载的 `Logger`/`Recovery`/`CORS` 中间件，未注册路由的 `GetSystemHealth`，5 个无人使用的 `BaseHandler` 方法 |
| 其他 | `plugin.containsPluginWithType`、`ISPClient.GetDataChannel` |

### 11.3 发现并修复的问题
| 模块 | 问题 | 修复 |
|---|---|---|
| 规则热加载 | 管理器发出 `update`/`delete` 事件，服务只处理 `created`/`updated`/`deleted`，数据分发使用的规则索引从不更新。**热加载以及 Web 端修改规则后，新规则不触发、修改不生效、删除的规则继续执行，必须重启网关** | 事件类型常量统一 |
| 规则热加载 | 从多规则文件中删除的规则仍在运行；删除文件时按文件名推断规则 ID，文件名与 ID 不同时不移除任何规则；`[]` 空文件被当作解析错误 | 记录规则与文件的对应关系；校验失败的新版本保留旧版本 |
| 规则监控 | `Close` 关闭通知通道：run 协程读到 nil 后空指针 panic（停止规则引擎时网关崩溃），之后的 `RecordError` 向已关闭通道发送也会 panic，重复 Close 也会 panic | 只取消 context |
| 规则监控 | `ToJSON` 递归加读锁（遇到等待中的写锁会死锁）；健康检查持有写锁执行（每项最长 5 秒），期间规则处理路径被阻塞 | 拆开加锁 |
| 告警（内置处理器） | 邮件、短信渠道什么都不做却返回成功 | 返回 `ErrChannelNotImplemented` |
| 表达式 | `exists()` 对 temperature/humidity 等"常见字段名"一律返回 true | 只检查实际字段 |
| 聚合（优化版） | 只接受 float64，int 等数值从不参与统计 | 统一数值转换 |
| 聚合（优化版） | 批处理按规则 ID 分片、直接模式按状态键分片，同一序列状态分裂 | 统一按状态键分片 |
| 聚合（优化版） | 两个后台协程永不退出；`Close` 关闭通道后并发发送 panic、重复 Close 也会 panic | 后台协程可停止，Close 可重复调用 |
| transform | 表达式中替换所有字母 x，`exp(x)` 变成 `e4p(4)` | 只替换独立的标识符 x |
| transform | 重采样忽略频率，一律按 60 个点分组且结果没有时间戳 | 改为按真实时间分桶 |
| transform | 移动平均和差分的结果没有时间戳（`Validate` 不通过） | 补上对齐的时间戳 |
| transform | `toBool(int64(0))` 为 true | 修正数值判断 |
| transform | 未知的平滑、归一化、变换、离群检测方法和坐标系静默回退（变换回退为原样返回） | 改为报错 |
| transform | GCJ02↔BD09 未接入 | 接入 |
| mock 适配器 | **配置文件中的 location/vector3d/color/vector/array 模拟点位实际发出的都是 float 0** | 复合类型保留生成函数给出的类型 |
| mock 适配器 | `oscillate` 模式只配置全局范围时 panic（网关崩溃）；指数分布超出上限 | 修复取值范围 |
| mock 适配器 | circular/linear 轨迹越过 ±180° 经线；标签数与维度不一致时数据被 `Validate` 拒绝 | 修正坐标和标签 |
| mock 适配器 | 矩阵微调破坏 identity/diagonal 结构，symmetric 从不对称；单位矩阵受稀疏度影响 | 每次生成后保证矩阵结构 |
| composite_mock | 配置的 tags 从未附加；int 数组 `max_value<1` 时 panic；间隔为 0 时 panic | 逐项修复（mock、http 适配器的间隔同样加了保护） |
| Web 配置 | "校验配置"接口实际调用 `UpdateConfig`，校验即保存并应用 | 单独的校验路径 |
| Web 鉴权 | 刷新令牌后旧会话不失效（SQLite 不保存 Active），旧刷新令牌可以反复使用 | 刷新后删除旧会话 |
| Web 插件 | 详情、配置、统计接口按类型名查找，配置实例（如 mock1）返回 404（启停接口已按实例名工作） | 支持按实例名查找 |
| Web 插件 | 资源占用是随机数；前端把字节当 MB 显示 | 返回 0 并在前端说明 |
| Web 监控 | `ExtendedAdapter` 的 `GetMetrics` 签名与所有实现都不一致，监控页的数据点数、错误数从未生效；监控按插件类型而不是实例统计 | 统一签名，按实例统计 |
| Web 通知 | 渠道和规则 ID 只用 UnixNano，同一微秒内创建会重复并互相覆盖 | 加入递增序号 |
| Web 通知 | 短信失败或返回错误状态码时仍报成功；邮件中文主题未编码 | 逐项修复 |
| 插件加载 | `builtin://` 适配器只支持写死的 mock | 改为按注册表创建 |
| 日志 | 规则指标同步每 5 秒打 2 条 Info 日志；loader 每次查找打 Info/Error 日志 | 降为 Debug |

### 11.4 验证
- 本机 `go test -race ./...` 29 个包全部通过。
- `go vet`、`gofmt -s -l .` 无输出。
- ARMv7 交叉编译和 vet 通过。
- gosec（v2.22.4）`Issues: 0`。
- `deadcode -test ./...` 无输出。
- 前端 lint 0 警告、tsc 0 错误、14 项测试通过、构建成功。

## 12. 第九轮：插件健康统计

试用时发现：网关所有配置的实例都在正常运行，`/health` 却显示 degraded（"4/13 个插件在运行"）。

| 位置 | 问题 | 修复 |
|---|---|---|
| `/health`、`/metrics` | 插件统计用的是 `GetPlugins()`，它列出所有可用的插件类型（内置注册表 + plugins 目录中的外部插件）。没有被配置使用的类型（redis、influxdb、http 等）都算"未运行"，网关一直显示 degraded；同类型的多个实例被合并成一个 | 新增 `Manager.InstanceStatuses()`，按配置中启用的实例统计（加上不在配置中但正在运行的外部插件）；初始化失败、启动失败、被停止的实例算作未运行 |
| 插件管理器启动 | 启动失败的适配器/连接器仍留在运行列表中，被统计为运行中 | 启动失败时从列表中移除 |
| 插件管理器 `loop` | `Stop` 关闭文件监听器后，`loop` 在已关闭的通道上空转，反复扫描插件目录并打 Info 日志，直到上下文取消 | 通道关闭时退出 |

### 12.1 插件管理页面无法控制插件

| 位置 | 问题 | 修复 |
|---|---|---|
| Web 插件列表 | 列出的是插件类型（mock、websocket、redis…），启停接口按实例名（mock_temperature_sensors…）工作：对 mock、websocket 点停止报 `plugin 'mock' is not running`，对没配置的类型点启动会用空配置创建实例 | 按实例列出（`InstanceStatuses`：配置中的实例，包括禁用的；插件目录中的外部插件；其他运行中的实例），新增 `plugin_type`、`mode` 字段 |
| Web 插件详情 | `GetPlugin` 只返回名称（TODO） | 返回实例的完整信息 |
| Web 插件配置 | 返回类型的元数据而不是实例配置；配置接口对所有登录用户开放 | 返回实例配置（`Manager.InstanceConfig`，副本），隐藏 password/secret/token 等字段 |
| 前端插件页面 | 删除、保存配置后端不支持（固定返回错误），"添加插件"按钮没有绑定操作 | 去掉这些按钮，配置改为只读查看并说明修改方式；类型列显示插件类型 |

验证：新增 `TestInstanceStatuses`、`TestLoopExitsWhenWatcherClosed`（无修复时失败）、`TestHealthAndMetricsCountConfiguredInstances`、`TestPluginServiceListsControllableInstances`、`TestMaskSecretsNested`；另外用新代码在独立端口启动网关，通过 Web 接口实测列表、停止、启动、重启、统计、配置均正常；`go test -race ./...` 全部通过，vet、gofmt、deadcode 无输出。
