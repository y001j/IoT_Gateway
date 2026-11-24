package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

// ruleOutputHeader 规则引擎输出消息的标记（与 rules.RuleOutputHeader 一致）。
// 缓冲 Sink 只接收原始采集数据，跳过规则引擎转发回 iot.data 的消息，与直连模式语义一致。
const ruleOutputHeader = "Iot-Origin"

// BufferConfig 单个 Sink 的缓冲配置（northbound.sinks[].buffer）
type BufferConfig struct {
	Enabled bool
	// Subject 该 Sink 消费的主题过滤，默认 iot.data.>
	Subject string
	// StartFrom 首次创建游标时从哪里开始：new（默认，只处理之后的数据）| all（补写流中已有数据）
	// | time（从 StartTime 开始，用于重放）
	StartFrom string
	// StartTime StartFrom=time 时的起点（按入库时间）
	StartTime time.Time
	// BatchSize 每批拉取/写入的最大条数，默认 500
	BatchSize int
	// MaxWait 拉取等待时间，默认 1s
	MaxWait time.Duration
	// WriteTimeout 单批写入超时，默认 10s
	WriteTimeout time.Duration
	// BackoffMin / BackoffMax 写入失败后的退避区间，默认 1s / 60s
	BackoffMin time.Duration
	BackoffMax time.Duration
	// ReplayRate 写入目标的最大速率（点/秒），0 表示不限。主要用于断网恢复后的补发：
	// 窄带链路或有写入配额的云端服务，全速补发会挤占实时数据
	ReplayRate int
}

// ParseBufferConfig 从 Sink 配置项中的 buffer 字段解析
func ParseBufferConfig(raw interface{}) BufferConfig {
	cfg := BufferConfig{}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return cfg
	}
	cfg.Enabled, _ = m["enabled"].(bool)
	cfg.Subject, _ = m["subject"].(string)
	cfg.StartFrom, _ = m["start_from"].(string)
	switch n := m["batch_size"].(type) {
	case int:
		cfg.BatchSize = n
	case float64:
		cfg.BatchSize = int(n)
	}
	switch n := m["replay_rate"].(type) {
	case int:
		cfg.ReplayRate = n
	case float64:
		cfg.ReplayRate = int(n)
	}
	parseDur := func(key string) time.Duration {
		if s, ok := m[key].(string); ok {
			if d, err := time.ParseDuration(s); err == nil {
				return d
			}
		}
		return 0
	}
	cfg.MaxWait = parseDur("max_wait")
	cfg.WriteTimeout = parseDur("write_timeout")
	cfg.BackoffMin = parseDur("backoff_min")
	cfg.BackoffMax = parseDur("backoff_max")
	return cfg
}

func (c *BufferConfig) applyDefaults() {
	if c.Subject == "" {
		c.Subject = "iot.data.>"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 500
	}
	if c.MaxWait <= 0 {
		c.MaxWait = time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 10 * time.Second
	}
	if c.BackoffMin <= 0 {
		c.BackoffMin = time.Second
	}
	if c.BackoffMax <= 0 {
		c.BackoffMax = 60 * time.Second
	}
	if c.BackoffMax < c.BackoffMin {
		c.BackoffMax = c.BackoffMin
	}
}

// Runner 状态
const (
	StateRealtime  = "realtime"  // 无积压，实时写入
	StateReplaying = "replaying" // 正在补发积压数据
	StateBuffering = "buffering" // 目标不可用，数据在本地积压
	StatePaused    = "paused"    // 被运维人员暂停，数据在本地积压
	StateStopped   = "stopped"
)

// SinkRunner 驱动一个开启缓冲的 Sink：从持久消费者拉取数据，写入成功后 ACK。
type SinkRunner struct {
	name    string
	sink    northbound.Sink
	cfg     BufferConfig
	store   *Store
	durable string
	sub     *nats.Subscription

	delivered      atomic.Uint64 // 成功写入的数据点数
	failed         atomic.Uint64 // 写入失败（可重试）的批次数
	discarded      atomic.Uint64 // 因永久错误或无法解析而丢弃的消息数
	state          atomic.Value  // string
	lastStateCheck atomic.Int64
	lastErr        atomic.Value // string

	paused atomic.Bool
	nextAt time.Time // 限速：下一批最早的写入时间（只在 loop 协程中使用）
	parent context.Context
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

// RunnerStats 运行器统计
type RunnerStats struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Durable   string `json:"durable"`
	Delivered uint64 `json:"delivered_points"`
	Failed    uint64 `json:"failed_batches"`
	Discarded uint64 `json:"discarded_messages"`
	// Pending 尚未写入目标的消息数（积压量）
	Pending uint64 `json:"pending"`
	// Lost 因超出保留上限被淘汰、该 Sink 永远收不到的消息数
	Lost uint64 `json:"lost"`
	// LagSeconds 最早一条未确认消息距今的秒数
	LagSeconds float64 `json:"lag_seconds"`
	LastError  string  `json:"last_error,omitempty"`
}

// DurableName 根据 Sink 名生成合法的消费者名
func DurableName(sinkName string) string {
	var b strings.Builder
	b.WriteString("sink_")
	for _, r := range sinkName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// StartSinkRunner 为 Sink 创建（或复用）持久消费者并启动运行器
func (s *Store) StartSinkRunner(ctx context.Context, name string, sink northbound.Sink, cfg BufferConfig) (*SinkRunner, error) {
	cfg.applyDefaults()
	durable := DurableName(name)
	opts := []nats.SubOpt{
		nats.BindStream(s.cfg.Stream),
		nats.ManualAck(),
		// AckAll：每批只同步确认最后一条消息。逐条异步确认时，下一次拉取可能早于服务端处理确认，
		// 在途消息数仍为上限而拿不到任何消息，只能等到拉取超时（实测吞吐仅约 3000 点/秒）。
		nats.AckAll(),
		// 同一时刻只有一批在途，保证严格按顺序写入
		nats.MaxAckPending(cfg.BatchSize),
		nats.AckWait(cfg.WriteTimeout + 10*time.Second),
		nats.MaxDeliver(-1),
	}
	switch {
	case strings.EqualFold(cfg.StartFrom, "all"):
		opts = append(opts, nats.DeliverAll())
	case strings.EqualFold(cfg.StartFrom, "time") && !cfg.StartTime.IsZero():
		opts = append(opts, nats.StartTime(cfg.StartTime))
	default:
		opts = append(opts, nats.DeliverNew())
	}
	sub, err := s.js.PullSubscribe(cfg.Subject, durable, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建缓冲消费者 %s 失败: %w", durable, err)
	}
	rctx, cancel := context.WithCancel(ctx)
	r := &SinkRunner{name: name, sink: sink, cfg: cfg, store: s, durable: durable, sub: sub, parent: ctx, cancel: cancel, done: make(chan struct{})}
	r.state.Store(StateRealtime)
	r.lastErr.Store("")
	go r.loop(rctx)
	log.Info().Str("sink", name).Str("durable", durable).Str("subject", cfg.Subject).Msg("连接器已开启本地缓冲（断网续传）")
	return r, nil
}

func (r *SinkRunner) loop(ctx context.Context) {
	defer close(r.done)
	backoff := r.cfg.BackoffMin
	wait := func(d time.Duration) bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
			return true
		}
	}
	nextBackoff := func() time.Duration {
		d := backoff
		backoff *= 2
		if backoff > r.cfg.BackoffMax {
			backoff = r.cfg.BackoffMax
		}
		return d
	}

	for ctx.Err() == nil {
		if r.paused.Load() {
			r.state.Store(StatePaused)
			if !wait(200 * time.Millisecond) {
				return
			}
			continue
		}
		// 目标不健康时不拉取，避免消息被反复投递
		if hc, ok := r.sink.(northbound.HealthChecker); ok {
			if err := hc.Healthy(); err != nil {
				r.setState(StateBuffering, err)
				if !wait(nextBackoff()) {
					return
				}
				continue
			}
		}

		msgs, err := r.sub.Fetch(r.cfg.BatchSize, nats.MaxWait(r.cfg.MaxWait))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
				r.markCaughtUp()
				continue
			}
			if ctx.Err() != nil || errors.Is(err, nats.ErrConnectionClosed) || errors.Is(err, nats.ErrBadSubscription) {
				return
			}
			r.lastErr.Store(err.Error())
			if !wait(time.Second) {
				return
			}
			continue
		}
		if len(msgs) == 0 {
			r.markCaughtUp()
			continue
		}

		points := make([]model.Point, 0, len(msgs))
		valid := make([]*nats.Msg, 0, len(msgs))
		badCount := 0
		for _, m := range msgs {
			if m.Header != nil && m.Header.Get(ruleOutputHeader) != "" {
				continue // 规则输出，不属于原始采集数据（随整批一起确认）
			}
			var p model.Point
			if err := json.Unmarshal(m.Data, &p); err != nil {
				badCount++
				continue
			}
			points = append(points, p)
			valid = append(valid, m)
		}
		last := msgs[len(msgs)-1]
		// ackBatch 同步确认整批（AckAll：确认最后一条即确认之前所有消息）
		ackBatch := func() {
			if err := last.AckSync(nats.Context(ctx)); err != nil && ctx.Err() == nil {
				log.Warn().Err(err).Str("sink", r.name).Msg("确认缓冲消息失败（这批数据可能被重复投递）")
			}
		}
		if badCount > 0 {
			r.discarded.Add(uint64(badCount))
		}
		if len(points) == 0 {
			ackBatch()
			continue
		}

		// 写入；可重试错误时原地重试同一批数据，直到成功、永久错误或停止。
		// 不使用 NAK 重投：JetStream 不保证延迟 NAK 的消息按原顺序重新投递。
		for {
			err = r.write(ctx, points)
			if err == nil {
				ackBatch()
				r.delivered.Add(uint64(len(points)))
				backoff = r.cfg.BackoffMin
				r.updateProgressState(false)
				if d := r.paceDelay(len(points), time.Now()); d > 0 && !wait(d) {
					return
				}
				break
			}
			if northbound.IsPermanent(err) {
				// 永久错误：丢弃该批，避免阻塞后续数据
				ackBatch()
				r.discarded.Add(uint64(len(valid)))
				r.lastErr.Store(err.Error())
				log.Error().Err(err).Str("sink", r.name).Int("count", len(valid)).Msg("目标拒绝数据（永久错误），已丢弃该批")
				break
			}
			r.failed.Add(1)
			r.setState(StateBuffering, err)
			d := nextBackoff()
			log.Warn().Err(err).Str("sink", r.name).Dur("retry_in", d).Msg("写入失败，数据保留在本地，稍后重试")
			if !wait(d) {
				// 停止：立即 NAK，重启后这批数据会被重新投递（保持在最前面）
				for _, m := range msgs {
					_ = m.Nak()
				}
				return
			}
			// 延长确认超时，防止重试期间被服务端重投
			for _, m := range msgs {
				_ = m.InProgress()
			}
		}
	}
}

// paceDelay 按 ReplayRate 计算写入 n 个点之后需要等待的时间。
// 空闲后不累积额度：nextAt 早于当前时间时从当前时间开始计算。
func (r *SinkRunner) paceDelay(n int, now time.Time) time.Duration {
	if r.cfg.ReplayRate <= 0 || n <= 0 {
		return 0
	}
	if r.nextAt.Before(now) {
		r.nextAt = now
	}
	r.nextAt = r.nextAt.Add(time.Duration(float64(n) / float64(r.cfg.ReplayRate) * float64(time.Second)))
	return r.nextAt.Sub(now)
}

func (r *SinkRunner) write(ctx context.Context, points []model.Point) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("连接器写入时 panic: %v", rec)
		}
	}()
	wctx, cancel := context.WithTimeout(ctx, r.cfg.WriteTimeout)
	defer cancel()
	if bw, ok := r.sink.(northbound.BatchWriter); ok {
		return bw.WriteBatch(wctx, points)
	}
	return r.sink.Publish(points)
}

func (r *SinkRunner) setState(state string, err error) {
	r.state.Store(state)
	if err != nil {
		r.lastErr.Store(err.Error())
	}
}

func (r *SinkRunner) markCaughtUp() {
	if s, _ := r.state.Load().(string); s != StateBuffering || r.paused.Load() {
		r.state.Store(StateRealtime)
	}
}

// updateProgressState 根据积压情况更新状态（最多每秒查询一次消费者信息，避免每批一次 API 往返）
func (r *SinkRunner) updateProgressState(force bool) {
	now := time.Now().UnixNano()
	if !force && now-r.lastStateCheck.Load() < int64(time.Second) {
		if s, _ := r.state.Load().(string); s == StateBuffering {
			r.state.Store(StateReplaying)
		}
		return
	}
	r.lastStateCheck.Store(now)
	if ci, err := r.sub.ConsumerInfo(); err == nil && ci.NumPending > 0 {
		r.state.Store(StateReplaying)
		return
	}
	r.state.Store(StateRealtime)
}

// Stats 返回运行器统计
func (r *SinkRunner) Stats() RunnerStats {
	st := RunnerStats{
		Name:      r.name,
		Durable:   r.durable,
		Delivered: r.delivered.Load(),
		Failed:    r.failed.Load(),
		Discarded: r.discarded.Load(),
	}
	st.State, _ = r.state.Load().(string)
	st.LastError, _ = r.lastErr.Load().(string)
	ci, err := r.store.js.ConsumerInfo(r.store.cfg.Stream, r.durable)
	if err != nil {
		return st
	}
	st.Pending = ci.NumPending + uint64(ci.NumAckPending)
	si, err := r.store.js.StreamInfo(r.store.cfg.Stream)
	if err == nil {
		// 流中最早的消息序号已经超过消费者确认到的位置：中间的消息在被写入目标之前就被淘汰了
		if si.State.FirstSeq > ci.AckFloor.Stream+1 && ci.Delivered.Stream > 0 {
			st.Lost = si.State.FirstSeq - ci.AckFloor.Stream - 1
		}
		if st.Pending > 0 {
			if m, err := r.store.js.GetMsg(r.store.cfg.Stream, maxU64(ci.AckFloor.Stream+1, si.State.FirstSeq)); err == nil {
				st.LagSeconds = time.Since(m.Time).Seconds()
			}
		}
	}
	return st
}

func maxU64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// Name 返回连接器名称
func (r *SinkRunner) Name() string { return r.name }

// Pause 暂停补发/写入：数据继续写入本地流并积压，恢复后从断点继续
func (r *SinkRunner) Pause() { r.paused.Store(true) }

// Resume 恢复写入
func (r *SinkRunner) Resume() {
	if r.paused.CompareAndSwap(true, false) {
		r.state.Store(StateRealtime)
		r.updateProgressState(true)
	}
}

// Paused 是否处于暂停状态
func (r *SinkRunner) Paused() bool { return r.paused.Load() }

// ResetSinkRunner 停止运行器并重建其游标：startFrom 为 "new" 时丢弃全部积压（跳过），
// 为 "time" 时从 from（入库时间）开始重新投递（重放，会产生重复数据）。返回新的运行器。
func (s *Store) ResetSinkRunner(r *SinkRunner, startFrom string, from time.Time) (*SinkRunner, error) {
	switch startFrom {
	case "new":
	case "time":
		if from.IsZero() {
			return nil, fmt.Errorf("重放需要指定起始时间")
		}
	default:
		return nil, fmt.Errorf("不支持的游标位置 %q", startFrom)
	}
	paused := r.Paused()
	r.Stop()
	if err := s.js.DeleteConsumer(s.cfg.Stream, r.durable); err != nil && !errors.Is(err, nats.ErrConsumerNotFound) {
		return nil, fmt.Errorf("删除游标 %s 失败: %w", r.durable, err)
	}
	cfg := r.cfg
	cfg.StartFrom = startFrom
	cfg.StartTime = from
	nr, err := s.StartSinkRunner(r.parent, r.name, r.sink, cfg)
	if err != nil {
		return nil, err
	}
	if paused {
		nr.Pause()
	}
	return nr, nil
}

// Stop 停止运行器（游标保留在 JetStream 中，重启后从断点继续）
func (r *SinkRunner) Stop() {
	r.once.Do(func() {
		r.cancel()
		<-r.done
		r.state.Store(StateStopped)
	})
}
