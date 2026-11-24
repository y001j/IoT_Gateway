# 设计文档：本地暂存（断网续传 + 近期数据查询）

- 状态：**三个阶段均已实现**（断网续传；近期数据查询、降采样、CSV 导出、历史数据页；汇总层、自动选层、暂停/恢复/跳过/重放）。未实现的部分见文末附录
- 实现位置：`internal/storage`（`storage.go` 持久化流，`runner.go` 缓冲运行器，`query.go` 查询与降采样，`rollup.go` 汇总层），`northbound.BatchWriter`，插件管理器 `attachSink` / `ControlSinkBuffer`，Web 接口 `internal/web/api/data_handler.go`，前端 `web/frontend/src/pages/HistoryPage.tsx`
- 日期：2026-11-24
- 关联：`docs/project_review_2026-11.md` 第 2.5 节、第 4.4 节
- 已确认的决策：两种场景都做（断网续传 + 本地近期数据查询）；底层复用内嵌 NATS JetStream；先出设计再实现

---

## 1. 背景与目标

### 1.1 现状
- 采集到的数据由插件管理器的单个协程直接调用各 Sink 的 `Publish()`（`internal/plugin/manager.go:607-614`）。Sink 写失败只记日志，**数据就丢了**。
- InfluxDB 和 MQTT Sink 是异步写的，`Publish()` 返回 nil 时并不代表目标系统已经收到。所以就算想重试，也拿不到可靠的失败信号。
- 已有的 JetStream Sink（`internal/northbound/jetstream/`）能把数据写进持久化流，但没有任何组件会自动读出来补发。重放只能手动运行 `cmd/tools/jetstream_recovery`。
- 网关本地不保存历史数据。Web 界面只能看实时推送，前端最多在内存里保留 50 条系统指标（`web/frontend/src/hooks/useRealTimeData.ts:167`）。

### 1.2 目标
1. **断网续传**：任何开启了缓冲的输出目标（InfluxDB、云端 MQTT、Redis……）不可用时，数据先留在本地磁盘；目标恢复后**按原顺序自动补发**；网关重启后也能从断点继续。
2. **近期数据查询**：不依赖外部数据库，在网关本地查询任意设备/点位最近一段时间的历史曲线和最新值。
3. **有界可控**：磁盘占用有硬上限，满了按策略淘汰；积压量、积压时长、因淘汰丢失的数据量都可观测，并且能触发告警。
4. **顺带解决性能问题**：开启缓冲的 Sink 不再挂在主管道协程上，慢 Sink 不会拖住采集（评审报告 3.2 第 1 条）。

### 1.3 非目标（本期不做）
- 替代 InfluxDB 这类长期时序库：本地只保留"近期"，长期归档仍然交给北向系统。
- 多网关集群和数据复制。
- 规则引擎从持久化流消费（作为第四阶段的可选项，见第 9 节）。

---

## 2. 核心思路：一次写入，多个游标

```
                          ┌───────────────────────────────────────────┐
 适配器 ──▶ 插件管理器 ──▶ │  JetStream 流 IOT_RAW  (subjects: iot.data.>) │  ← 落盘一次
            (PublishAsync) │  文件存储 + S2 压缩，按容量/时长淘汰最旧数据   │
                          └──────┬──────────┬──────────┬──────────┬──────┘
                                 │          │          │          │
                 核心订阅(实时)  │  持久消费者 │  持久消费者 │  临时有序消费者
                                 ▼          ▼          ▼          ▼
                            规则引擎   sink_influx  sink_mqtt   历史查询 API
                            (不变)     游标+ACK     游标+ACK    (按主题+时间)
                                          │          │
                                     InfluxDB     云端 MQTT

 无缓冲的 Sink（websocket、console）：继续由插件管理器直接 Publish（实时，最多一次）
```

- **IOT_RAW 流同时起两个作用**：一是预写日志（WAL），供断网续传使用；二是近期原始数据仓库，供查询使用。数据只落盘一次。
- **每个开启缓冲的 Sink 对应一个 JetStream 持久消费者（durable consumer）**，消费者就是该 Sink 在流里的"游标"。Sink 确认写成功后才 ACK，游标前移；失败就不 ACK，游标停在原地，数据留在流里等重试。
- 断网续传和正常运行**走同一条代码路径**，没有"故障时切换到缓存模式"的状态机，所以不会有两套逻辑行为不一致的问题。
- 数据只有在**超出流的保留上限**时才会真正丢失，而且丢了多少可以精确算出来（见 6.3）。

---

## 3. 组件设计

### 3.1 新增 `internal/storage` 包（存储服务）

职责：
- 启动时检查 JetStream 是否可用；按配置**创建或更新**流（`AddStream` / `UpdateStream`）。
- 对外提供以下接口：
  ```go
  type Store interface {
      Enabled() bool
      JS() nats.JetStreamContext
      // 历史查询
      QueryRaw(ctx context.Context, q RawQuery) (*SeriesResult, error)
      Latest(ctx context.Context, deviceID string) ([]model.Point, error)
      ListSeries(ctx context.Context, filter string, offset, limit int) ([]SeriesInfo, error)
      // 状态
      Status(ctx context.Context) (*StorageStatus, error)
  }
  ```
- 实现 `core.Service` 接口，由 `Runtime` 管理生命周期。**它必须在插件管理器之前创建**（`internal/core/runtime.go` 中紧接 NATS 初始化之后），插件管理器和 Web 服务通过依赖注入拿到它。

### 3.2 写入路径改造（插件管理器）
- 开启存储后，`sendBatchOptimized` 发布 `iot.data.<dev>.<key>` 时改用 `js.PublishAsync`，并设置 `PublishAsyncMaxPending`（例如 4096）：
  - 能拿到存储层的确认和失败信号，失败会计入指标（`storage_publish_errors_total`）；
  - pending 满了会形成**背压**，而不是静默丢弃；
  - JetStream 发布本身也是一次普通的主题发布，所以规则引擎的核心订阅**不受影响**。
- 可选：设置 `Nats-Msg-Id = hash(device_id, key, timestamp)`，再配合流的 `Duplicates` 窗口（默认 2 分钟），可以过滤掉适配器重发的重复点。
- 去掉无人订阅的 `data.<sinkName>` 发布（评审报告 3.2 第 3 条）。
- 对开启了缓冲的 Sink，主管道**不再调用它的 `Publish()`**，改由第 3.4 节的消费者驱动。

### 3.3 Sink 接口扩展（可选接口，向后兼容）

```go
// BatchWriter：同步写入。返回 nil 表示目标系统已经确认接收（落库或 broker 确认），
// 这是 ACK 游标的唯一依据。
type BatchWriter interface {
    WriteBatch(ctx context.Context, batch []model.Point) error
}

// HealthChecker：目标不健康时暂停拉取，避免空转重试。
type HealthChecker interface {
    Healthy() error
}

// 错误分类：可重试 vs 永久错误（例如 400 格式错误）。永久错误的数据会进死信流，不阻塞后续数据。
var ErrPermanent = errors.New("permanent sink error")
// 用法：return fmt.Errorf("%w: %v", northbound.ErrPermanent, err)
```

各 Sink 的实现要点：

| Sink | 支持缓冲 | `WriteBatch` 实现 |
|---|---|---|
| influxdb | ✅ | 用 `WriteAPIBlocking.WritePoint(ctx, points...)` 整批写入。**前提是先修复时间戳问题，改用 `point.Timestamp`**（评审 2.4），否则重放的数据时间全错，也失去了幂等性。 |
| mqtt | ✅ | QoS ≥ 1 时逐条 `Publish` 并等待所有 token（带超时）。QoS 0 无法确认，配置校验时给出警告。payload 增加 `seq` 字段（流序号），方便下游去重。 |
| redis | ✅ | 用 Pipeline 执行 `SET`/`HSET` 后 `Exec`。整批一次往返，同时解决逐点往返的性能问题。 |
| jetstream | ⚠️ 合并 | 功能已经被 IOT_RAW 覆盖，标记为废弃；如需转发到远端 NATS，另做 `nats_remote` Sink。 |
| websocket / console | ❌ | 只用于实时展示，不提供缓冲（重放历史到实时面板没有意义）。 |
| nats_subscriber | ❌（本期） | 它本身是路由器，目标 Sink 由它内部管理，后续再评估。 |

某个 Sink 配置了缓冲但没有实现 `BatchWriter` 时，**记一条警告，然后对它关闭缓冲、退回直连模式**，不让整个网关启动失败（吸取评审报告中"可选输出配置错误导致网关退出"的教训）。

### 3.4 缓冲 Sink 运行器（`internal/storage/sinkrunner.go`）

每个开启缓冲的 Sink 启动一个运行器协程：

```
loop:
  若 Sink 实现了 HealthChecker 且不健康 → 状态=buffering，退避等待，continue
  msgs := Fetch(batch_size, MaxWait=max_wait)
  points := decode(msgs)                      // 解码失败 → 写入死信流并 Term() 该消息
  令牌桶限速(replay_rate)                     // 只在积压时生效，防止恢复后瞬间冲垮上游
  err := sink.WriteBatch(ctx(timeout), points)
  成功        → 对最后一条消息 AckAll；退避复位；状态=realtime/replaying
  永久错误    → 二分定位坏数据（或逐条重试）→ 坏数据进死信流、Term()，其余正常写入
  可重试错误  → 整批 NakWithDelay(退避)；状态=buffering；退避 1s→2s→…→60s（带抖动）
```

消费者参数：

| 参数 | 取值 | 原因 |
|---|---|---|
| 类型 | 持久拉取消费者，名称 `sink_<sinkName>` | 重启后从断点继续 |
| `FilterSubjects` | Sink 配置的 `subjects`，默认 `["iot.data.>"]` | nats-server 2.10 支持多个过滤主题，可以按设备或点位只转发一部分 |
| `AckPolicy` | `AckAll` | 每批只需要一次 ACK |
| `MaxAckPending` | `= batch_size` | 同一时刻只有一批在途，**保证严格有序** |
| `AckWait` | `write_timeout + 10s` | 避免写入还没完成就被重投递 |
| `MaxDeliver` | `-1`（不限） | 目标恢复之前一直保留；真正的上限是流的保留策略 |
| `DeliverPolicy`（首次创建时） | `new`（默认）或 `all` | `all` 表示新加的 Sink 会把流里已有的数据全部补写一遍 |

**投递语义：至少一次（at-least-once）**。如果 Sink 写成功后、ACK 之前网关崩溃，重启后这一批会重发。InfluxDB（相同 measurement、tag、时间戳会覆盖）和 Redis（SET 本身幂等）天然幂等；MQTT 下游需要用 `seq` 去重。

### 3.5 本地近期数据查询

#### 原始数据查询 `QueryRaw`
- 创建临时有序消费者（`nats.OrderedConsumer()`），过滤主题为 `iot.data.<dev>.<key>`，起始位置为 `StartTime(start - 容忍窗口)`；读到点位时间戳超过 `end`，或者没有待读消息时停止。
- 注意：JetStream 的 StartTime 用的是**入库时间**，而不是点位的采集时间戳。所以起点要往前放宽一个容忍窗口（默认 1 分钟），返回前再按 payload 里的 `timestamp` 精确过滤和排序。
- **服务端降采样**：返回的点数超过 `limit`（默认 1000）时，按时间桶聚合（`avg`/`min`/`max`/`last`），或者用 LTTB 算法保留曲线形状，避免把几十万个点传给浏览器。
- **保护网关资源**：全局最多同时执行 2 个查询（信号量），单次查询最多扫描 20 万条消息，超时 10 秒。超出限制时返回已扫描的部分，并标记 `truncated: true`。

#### 最新值 `Latest`
- 每个序列调用 `js.GetLastMsg("IOT_RAW", subject)`，开销很低。
- 序列列表通过 `StreamInfo(SubjectsFilter: "iot.data.<dev>.>")` 获取（支持分页 offset），结果缓存 30 秒。

#### 汇总层（第三阶段）`IOT_ROLLUP_1M`
原始数据受磁盘预算限制只能保留较短时间（见第 5 节）。为了查看更长时间的趋势，增加一个汇总流：
- 汇总器使用持久消费者读取 IOT_RAW，在内存中按"序列 × 1 分钟"计算 `count/min/max/avg/last`。窗口结束（加上 10 秒迟到容忍）后，发布到 `iot.rollup.1m.<dev>.<key>`，然后 AckAll 到该窗口的最后一条消息，这样重启后不会丢窗口。
- 汇总流单独设置保留期（默认 30 天）。
- 查询 API 根据时间范围**自动选择数据层**：范围不超过原始数据的保留期时查原始层，否则查汇总层。

### 3.6 主题编码（前置要求）
现在的代码直接用 `fmt.Sprintf("iot.data.%s.%s", DeviceID, Key)` 拼主题（`internal/core/bus/bus.go:352`、`internal/plugin/manager.go:561,639`、forward 动作、websocket Sink）。如果设备 ID 或 key 里含有 `.`、`*`、`>` 或空白字符，主题层级就会错乱，**按序列存储和查询会直接出错**。

新增 `model.EncodeSubjectToken` / `DecodeSubjectToken`，采用可逆转义（`~` → `~7E`，`.` → `~2E`，`*` → `~2A`，`>` → `~3E`，空白 → `~20`），所有拼接主题和查询的地方统一使用。payload 里仍然保存原始的 `device_id` 和 `key`。

---

## 4. 配置设计

```yaml
storage:
  enabled: true
  store_dir: "./data/jetstream"   # 与现有内嵌 NATS 的 StoreDir 一致
  max_disk: "4GB"                 # 内嵌 NATS 的 JetStreamMaxStore，所有流的总上限
  sync_interval: "10s"            # 刷盘间隔，见 5.2
  raw:                            # IOT_RAW：WAL + 近期原始数据
    max_age: "24h"
    max_bytes: "2GB"              # 必须小于 max_disk，给汇总流、死信流留出空间
    compression: "s2"
    duplicate_window: "2m"
    discard: "old"                # 满了以后丢最旧的数据（不阻塞采集）
  rollup:                         # 第三阶段
    enabled: false
    interval: "1m"
    max_age: "720h"
    max_bytes: "1GB"
  dlq:
    max_age: "168h"
    max_bytes: "100MB"
  query:
    max_concurrent: 2
    max_scan: 200000
    default_limit: 1000
    timeout: "10s"

northbound:
  sinks:
    - name: "influx_cloud"
      type: "influxdb"
      # ...原有配置...
      buffer:
        enabled: true
        subjects: ["iot.data.>"]   # 可以只缓冲一部分，例如 ["iot.data.plc1.>"]
        start_from: "new"          # new | all：首次创建游标时从哪里开始
        batch_size: 500
        max_wait: "1s"
        write_timeout: "10s"
        replay_rate: 5000          # 积压补发时最多每秒多少点，0 表示不限
        backoff_max: "60s"
```

**外部 NATS 模式**：启动时调用 `js.AccountInfo()` 检查 JetStream 是否可用。不可用时记一条警告，存储功能整体关闭，所有缓冲 Sink 退回直连模式，`/storage/status` 显示 `disabled` 和原因。

同时修复 `docker-compose.yml` 引用但**仓库里并不存在**的 `configs/nats-server.conf`：补上这个文件，并开启 `jetstream { store_dir: /data }`。

---

## 5. 容量规划与持久性

### 5.1 磁盘占用估算
一个带 4 个 tag 的点，JSON 大约 217 字节；加上主题和文件存储的记录头，单条约 270 字节。S2 压缩对重复字段很多的 JSON 效果较好，暂按约 100 字节/条估算（**需要在第一阶段实测校准**）。

| 采集速率 | 每天未压缩 | 每天压缩后（估） | 2GB 原始层能保留多久 |
|---:|---:|---:|---:|
| 100 点/秒 | 2.3 GB | ~0.9 GB | ~2.3 天 |
| 1,000 点/秒 | 23 GB | ~8.6 GB | ~5.6 小时 |
| 10,000 点/秒 | 233 GB | ~86 GB | ~33 分钟 |

**结论：采集速率高的场景，本地原始层只能顶住几个小时的断网。** 需要配合以下措施：
1. **按点位只记录变化（死区）/降频记录**：在适配器或规则层对缓慢变化的量设置死区，这是降低存储量最有效的手段。
2. **按 Sink 过滤主题**：只对真正需要的数据开启缓冲。
3. **紧凑编码**（第四阶段）：设备 ID 和 key 已经在主题里，payload 可以只保留 `{t,v,q}`，预计每条能再减少 60% 以上。
4. 状态 API 根据当前写入速率，**实时估算"还能撑多久"**，在 UI 上展示。

### 5.2 掉电与 Flash 寿命
- JetStream 文件存储默认每 2 分钟 fsync 一次。突然掉电时，**最多丢失最后一个刷盘间隔内的数据**。本设计默认 `sync_interval: 10s`，在丢失窗口和 eMMC/SD 卡的写入寿命之间折中。
- 不建议在 Flash 介质上使用 `sync: always`（每条都刷盘），写放大严重，吞吐也会大幅下降。
- `store_dir` 建议放在独立分区上，避免写满后影响系统日志和配置文件。

---

## 6. Web API 与界面

### 6.1 API（全部需要 JWT；带 🔒 的需要 admin 角色）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/data/series?device_id=&offset=&limit=` | 列出本地存储里的序列（设备 × 点位）和各自的消息数 |
| GET | `/api/v1/data/latest?device_id=` | 设备所有点位的最新值 |
| GET | `/api/v1/data/history?device_id=&key=&start=&end=&limit=&agg=avg\|min\|max\|last\|lttb` | 历史曲线，自动选择原始层或汇总层，返回 `{points, tier, truncated}` |
| GET | `/api/v1/data/export?...` | 导出 CSV（流式输出，同样受扫描上限约束） |
| GET | `/api/v1/storage/status` | 各流的大小、条数、最早和最新时间；每个缓冲 Sink 的状态、积压量、积压时长、因淘汰丢失的数据量、死信数量；还能撑多久 |
| POST 🔒 | `/api/v1/storage/sinks/:name/pause` 、`/resume` | 暂停或恢复某个 Sink 的补发 |
| POST 🔒 | `/api/v1/storage/sinks/:name/replay` `{from_time}` | 把游标重置到某个时间点，重新补发（会产生重复数据，需要二次确认） |
| POST 🔒 | `/api/v1/storage/sinks/:name/skip` | 丢弃积压，游标直接跳到最新 |
| GET / DELETE 🔒 | `/api/v1/storage/dlq/:sink` | 查看或清空死信 |

### 6.2 前端
- **新增"历史数据"页**：设备和点位选择器（数据来自 `/data/series`），时间范围选择，ECharts 折线图（项目已有这个依赖），显示当前数据层（原始/汇总）和是否截断，支持导出 CSV。
- **监控页增加"本地存储"卡片**：磁盘占用、预计可保留时长；每个缓冲 Sink 一行，显示状态灯（实时 / 缓冲中 / 补发中 / 已关闭）、积压量、积压时长和操作按钮。

### 6.3 指标与告警
- `storage_bytes`、`storage_msgs`、`storage_disk_usage_ratio`、`storage_publish_errors_total`
- 每个 Sink：`sink_buffer_pending`（消费者 `NumPending`）、`sink_buffer_lag_seconds`（当前时间减去第一条未确认消息的时间）、`sink_buffer_state`
- **因淘汰丢失的数据量**：当流的 `FirstSeq` 超过消费者的 `AckFloor.Stream + 1` 时，`dropped = FirstSeq - AckFloor.Stream - 1`。说明这个 Sink 的积压已经被淘汰掉了，是必须告警的事件。
- 接入现有的 `system_alert_service`：积压时长超过 `max_age` 的 50% 发 warning，超过 80% 发 critical；磁盘占用超过 80% 发 warning；`dropped` 增长时发 critical。

---

## 7. 对现有组件的影响

| 组件 | 变化 |
|---|---|
| `internal/core/runtime.go` | 内嵌 NATS 的 `server.Options` 增加 `JetStreamMaxStore`、`SyncInterval`；在插件管理器之前创建存储服务并注入 |
| `internal/plugin/manager.go` | 改用 `PublishAsync`；缓冲 Sink 不再直接调用 `Publish()`；去掉 `data.<sink>` 发布 |
| `internal/northbound/*` | 新增可选接口；influxdb、mqtt、redis 实现 `WriteBatch`；sink 初始化失败不再导致整个网关退出 |
| `internal/northbound/jetstream`、`cmd/tools/jetstream_recovery` | 标记为废弃，功能由 IOT_RAW 和 replay API 取代 |
| `internal/web/api` | 新增 data 和 storage 两组路由 |
| `web/frontend` | 新增历史数据页和存储状态卡片 |
| `docker-compose.yml` / `configs/` | 补上 `nats-server.conf` 并开启 JetStream |
| 规则引擎 | 本期不变（继续使用核心订阅）；必须先修复 forward 死循环，否则循环产生的消息会迅速**写满磁盘** |

---

## 8. 前置修复（实现本功能之前或同时完成）

这些问题在没有本地存储时只影响实时数据；有了存储以后，会被**持久化并放大**：

1. forward 动作的默认主题导致死循环（评审 2.1）：死循环会在几分钟内写满流，把真实数据全部淘汰掉。
2. InfluxDB 使用 `time.Now()` 而不是采集时间戳（评审 2.4）：补发的数据时间全错，也无法依赖覆盖写实现幂等。
3. 主题编码（本文 3.6）。
4. Modbus `scale` 默认为 0（评审 2.2）：不修的话，存进去的是一堆错误数据。
5. Sink 初始化失败导致整个网关退出（`internal/plugin/manager.go:232-233`）。

---

## 9. 分阶段实施

| 阶段 | 内容 | 验收标准 |
|---|---|---|
| **一：断网续传** | 第 8 节前置修复；`internal/storage` 和 IOT_RAW；配置；`BatchWriter`（influxdb、mqtt、redis）；SinkRunner；死信流；`/storage/status`；指标和告警；`nats-server.conf` | 断开 InfluxDB 30 分钟后恢复：数据**零丢失、顺序正确**；重启网关后从断点继续；超出保留上限时 `dropped` 计数准确并触发告警 |
| **二：近期查询** | `QueryRaw`、`Latest`、`ListSeries`；降采样；data API；历史数据页；CSV 导出 | 在目标 ARM 设备上，查询 24 小时、单序列历史的 P95 小于 2 秒；并发查询不影响采集速率 |
| **三：汇总层与运维** | `IOT_ROLLUP_1M`；按时间范围自动选层；暂停、重放、跳过、死信管理界面；可保留时长估算 | 30 天趋势查询可用；运维操作都有审计日志 |
| **四（可选）** | 紧凑编码；规则引擎改为从流消费（过载时不丢数据、可以回放重算）；按点位设置死区 | 根据压测结果决定是否做 |

---

## 10. 测试方案

- **单元和集成测试**：使用 `github.com/nats-io/nats-server/v2/test` 在临时目录启动开启 JetStream 的内嵌服务器，配合一个可以注入故障的假 Sink：
  - 先失败 N 次再恢复：校验数据不丢、顺序正确、没有多余重复（崩溃场景除外）；
  - 重启持久化：停掉运行器和服务器，重启后从 AckFloor 继续；
  - 保留溢出：用很小的 `max_bytes` 制造淘汰，校验 `dropped` 的计算；
  - 永久错误：校验坏数据进入死信流，后续数据不受阻塞；
  - 查询：乱序时间戳、跨越 StartTime 容忍窗口、降采样后点数不超过 limit、截断标记；
  - 主题编码往返（含 `.`、`*`、`>`、中文、空格）；
  - 外部 NATS 不支持 JetStream 时正确降级。
- **压测**：在目标 ARM 硬件上对比开启和关闭存储时的最大稳定采集速率、CPU 和磁盘写入速度；实测 S2 压缩比，校准第 5.1 节的估算。

---

## 11. 待确认问题

1. **默认磁盘预算和保留期**：建议 `max_disk: 4GB`、原始层 24 小时（或 2GB，以先到者为准）、汇总层 30 天。需要知道目标设备的存储容量。
2. **默认对哪些 Sink 开启缓冲**：建议 influxdb 和 mqtt 默认开启，redis 默认关闭（通常存的是最新值，补发旧数据意义不大），websocket 和 console 不支持缓冲。
3. **磁盘写满时的策略**：建议"丢弃最旧数据并告警"（`discard: old`），保证采集不中断。另一种选择是"拒绝新数据"，适合审计场景。
4. **掉电丢失窗口**：`sync_interval: 10s` 能否接受？
5. **规则引擎是否要在第四阶段改为从流消费**：好处是过载时不丢数据、可以回放重算；代价是增加一些延迟，复杂度也更高。

---

## 附：实现说明（与设计的差异）

### 第一阶段：断网续传

| 设计 | 实现 |
|---|---|
| 消费者 `AckPolicy = AckAll`，失败时整批 `NakWithDelay` | 确认策略相同（每批只对最后一条 `AckSync`）。失败时**不 NAK**，而是把同一批数据留在内存中原地重试，并用 `InProgress()` 延长确认超时：测试发现 JetStream 不保证延迟 NAK 的消息按原顺序重投。停止时才 NAK，重启后这批数据会重新投递 |
| `FilterSubjects` 支持多个主题 | 每个 Sink 支持一个过滤主题（`buffer.subject`，默认 `iot.data.>`） |
| `replay_rate` 限制补发速率 | 已实现（`buffer.replay_rate`，点/秒）：每批写入成功后按速率推迟下一批，空闲期间不累积额度 |
| 永久错误进入死信流 `IOT_DLQ` | 永久错误（`northbound.ErrPermanent`）的批次被丢弃并计入 `discarded_messages`，未单独建死信流，因此也没有死信查看/清空接口 |
| 状态接口 `/api/v1/storage/status` | 已实现（需登录）；网关 8080 端口的 `GET /storage` 提供同样内容（含当前告警），并汇总到 `/health` 的 `storage` 组件（与告警条件一致） |

已实现 `BatchWriter`（同步确认写入）的 Sink：InfluxDB（阻塞写入 API，4xx 视为永久错误）、Redis（Pipeline）、MQTT（QoS≥1 等待 broker 确认）。其他 Sink 开启缓冲时退回调用 `Publish`。

### 第二阶段：近期数据查询

- `Store.QueryRaw`：临时有序消费者按序列主题读取，起点为 `start - tolerance`（入库时间）。**结束条件**：读到序列末尾（消息元数据的 `NumPending == 0`），或者某条消息的入库时间和采集时间都已超过 `end + tolerance`。注意不能用 `ConsumerInfo().NumPending` 判断：推送式消费者创建后立即推送消息，此时服务端待投递数已经是 0。
- 已知限制：查询过去的时间段时，在序列中入库时间远晚于采集时间、且排在提前结束位置之后的点（严重迟到的数据）不会被读到；查询范围覆盖到当前时间时会读到序列末尾，不受影响。
- 降采样：`avg/min/max/last` 等宽时间桶，`lttb` 保形；存在非数值点（字符串、复合类型）时退回 `last`，返回原始值。
- 资源保护：信号量（`max_concurrent`，超时返回 `ErrQueryBusy` → HTTP 429）、`max_scan`（超出时 `truncated: true`）、`timeout`、`max_limit`。
- `Store.ListSeries`（`StreamInfo` + `SubjectsFilter`，5 秒缓存）、`Store.Latest`（每序列 `GetLastMsg`，最多 1000 个序列）。
- Web 接口（需登录）：`GET /api/v1/data/series`、`/data/latest`、`/data/history`（`tier=auto|raw|rollup`）、`/data/export`（CSV，不降采样，受 `max_scan` 约束；以 `= + - @` 开头的字符串加 `'` 前缀防公式注入）。时间参数接受 RFC3339 或 Unix 秒/毫秒。
- 前端"历史数据"页：设备/点位选择、时间范围、聚合方式、数据层、点数；ECharts 曲线（非数值序列显示表格）、截断/降采样/数据层标记、CSV 导出、设备最新值；"本地存储"卡片显示磁盘占用、汇总层状态、每个缓冲连接器的状态和运维按钮（仅管理员）。

### 第三阶段：汇总层与运维

| 设计 | 实现 |
|---|---|
| 汇总流 `IOT_ROLLUP_1M`，主题 `iot.rollup.1m.<dev>.<key>` | 流名默认 `IOT_ROLLUP`，主题 `iot.rollup.<dev>.<key>`，窗口由 `storage.rollup.interval` 配置（默认 1m） |
| 窗口结束 + 10 秒迟到容忍后写出，AckAll 到窗口最后一条 | 窗口按**事件时间水位**关闭（已见到的最大采集时间；追平积压后才用当前时间），这样重启后追赶积压时旧数据不会被当成迟到数据。只确认到"所有未写出窗口中最小序号"之前，重启后未写出的窗口从原始流重算；启动时若游标有未确认消息，会删除并从确认位置之后重建游标，避免等待确认超时（约 2.5 分钟）|
| — | 重启可能导致少量窗口被部分重算后再次写出，查询时同一窗口取 `count` 最大的记录；窗口写出后才到达的点计入 `late_points`，不再进入汇总 |
| 按时间范围自动选层 | `tier=auto`：起点早于原始流中最早数据时查汇总层，否则查原始层 |
| 暂停、重放、跳过 | `POST /api/v1/storage/sinks/:name/{pause,resume,skip,replay}`（仅管理员，replay 需要 `from_time`）。跳过/重放通过删除并重建游标实现，暂停状态在重建后保留 |
| 死信管理、运维审计日志、可保留时长估算 | 未实现 |

### 第 6.3 节的指标与告警

- **告警**（`internal/storage/alerts.go`，`storage.alerts`，默认开启）：`AlertMonitor` 每 30 秒评估一次，按规则引擎告警的格式发布到 `iot.alerts.triggered`（和 `.warning` / `.critical`），Web 告警集成服务会据此创建告警并发送通知。
  - **积压**：积压比例 = max(最早未写出数据的等待时长 / `max_age`, 积压数据量 / `max_bytes`)，后者按"积压条数 / 流中条数 × 流占用率"估算。≥ 0.5 为 warning，≥ 0.8 为 critical，达到 1 时开始丢数据。同时考虑两种淘汰条件，比设计中只看积压时长更准确。
  - **数据丢失**：连接器的累计丢失量增加时发送 critical（每次增加都发送）。
  - **磁盘**：JetStream 数据目录所在文件系统使用率 ≥ 80% 为 warning，≥ 95% 为 critical。
  - **不按流的占用率告警**：`IOT_RAW` 是环形缓冲区，写满 `max_bytes` 是正常状态（第一阶段 `/health` 曾按占用率 90% 降级，会误报，已改正）。
  - 同一告警（类型、连接器、级别相同）30 分钟内只发送一次，级别升高时立即发送。
- **指标**：`/metrics?format=text` 增加 `iot_gateway_storage_*`（流的字节数/条数/占用率、磁盘使用率；每个连接器的积压、积压时长、积压比例、已写入、失败批次、丢弃、丢失、状态；汇总层记录数、占用、未关闭窗口、迟到点）。

### 性能

| 场景 | Apple M4 Pro（12 核） | Linux/ARM64 虚拟机（4 核，lima） |
|---|---|---|
| 积压补发（`BenchmarkDrainBacklog`） | 约 24.6 万点/秒 | 约 30 万点/秒 |
| 24 小时单序列查询，86400 点降采样到 1000 点（`BenchmarkQueryRaw24h`） | 平均 156ms，最慢 163ms | 平均 162ms，最慢 184ms |
| 开启缓冲的连接器能实时跟上的采集速率（`cmd/tools/benchmark -storage -rate N`） | — | 5 万点/秒积压为 0；10 万点/秒时只能处理约 4.9 万点/秒（`-buffer-batch 5000` 时约 6.3 万点/秒） |

第二阶段的验收目标（24 小时单序列查询 P95 < 2 秒）在 ARM64 上满足。缓冲模式在 4 核上的上限约 5～6 万点/秒，此时内嵌 JetStream 同时承担落盘和投递，成为瓶颈。ARMv7（32 位）无法在 Apple 芯片的虚拟机中运行，需在真实设备上用 `cmd/tools/benchmark` 测量。
