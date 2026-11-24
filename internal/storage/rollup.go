package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
)

// RollupConfig 汇总层配置（storage.rollup 段）。
// 原始数据受磁盘预算限制只能保留较短时间；汇总层按"序列 × 时间窗口"保存 count/min/max/avg/last，
// 用于查看更长时间的趋势。
type RollupConfig struct {
	Enabled bool
	// Stream 汇总流名称，默认 IOT_ROLLUP
	Stream string
	// Interval 汇总窗口，默认 1m
	Interval time.Duration
	// Lateness 窗口结束后等待迟到数据的时间，默认 10s；更晚到达的点不计入汇总
	Lateness time.Duration
	// MaxAge 汇总数据保留时间，默认 720h（30 天）
	MaxAge time.Duration
	// MaxBytes 汇总流磁盘上限，默认 1GB
	MaxBytes int64
	// BatchSize 每次从原始流拉取的消息数，默认 1000
	BatchSize int
}

// DefaultRollupConfig 默认汇总配置（默认关闭）
func DefaultRollupConfig() RollupConfig {
	return RollupConfig{
		Stream:    "IOT_ROLLUP",
		Interval:  time.Minute,
		Lateness:  10 * time.Second,
		MaxAge:    30 * 24 * time.Hour,
		MaxBytes:  1 << 30,
		BatchSize: 1000,
	}
}

func (c *RollupConfig) applyDefaults() {
	d := DefaultRollupConfig()
	if c.Stream == "" {
		c.Stream = d.Stream
	}
	if c.Interval <= 0 {
		c.Interval = d.Interval
	}
	if c.Lateness < 0 {
		c.Lateness = 0
	}
	if c.MaxAge <= 0 {
		c.MaxAge = d.MaxAge
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = d.MaxBytes
	}
	if c.BatchSize <= 0 {
		c.BatchSize = d.BatchSize
	}
}

// rollupSubjectPrefix 汇总数据主题前缀：iot.rollup.<device>.<key>
const rollupSubjectPrefix = "iot.rollup."

// RollupSubject 汇总数据的主题
func RollupSubject(deviceID, key string) string {
	return rollupSubjectPrefix + model.EncodeSubjectToken(deviceID) + "." + model.EncodeSubjectToken(key)
}

// RollupRecord 一个序列在一个时间窗口内的汇总
type RollupRecord struct {
	DeviceID string    `json:"device_id"`
	Key      string    `json:"key"`
	Start    time.Time `json:"start"` // 窗口起始时间
	Count    int64     `json:"count"`
	Sum      float64   `json:"sum"`
	Min      float64   `json:"min"`
	Max      float64   `json:"max"`
	Last     float64   `json:"last"`
	LastTime time.Time `json:"last_time"`
}

func (r *RollupRecord) add(v float64, t time.Time) {
	if r.Count == 0 {
		r.Min, r.Max = v, v
	}
	r.Count++
	r.Sum += v
	if v < r.Min {
		r.Min = v
	}
	if v > r.Max {
		r.Max = v
	}
	if !t.Before(r.LastTime) {
		r.Last, r.LastTime = v, t
	}
}

// value 按聚合方式取值
func (r *RollupRecord) value(agg string) float64 {
	switch agg {
	case AggMin:
		return r.Min
	case AggMax:
		return r.Max
	case AggLast:
		return r.Last
	default:
		if r.Count == 0 {
			return 0
		}
		return r.Sum / float64(r.Count)
	}
}

type rollupKey struct {
	subject string
	start   int64
}

// Rollup 汇总器：用持久消费者读取原始流，按窗口汇总后写入汇总流。
//
// 确认策略（AckAll）：只确认到"所有仍在内存中的窗口的最小序号"之前，
// 因此网关重启时未写出的窗口会从原始流中重新计算，不会丢失。
// 重启时少量已写出的窗口可能被部分重算并再次写出，查询时同一窗口取 count 最大的记录。
type Rollup struct {
	store *Store
	cfg   RollupConfig
	sub   *nats.Subscription

	mu   sync.Mutex
	open map[rollupKey]*rollupWindow
	// watermark 已见到的最大采集时间（不超过当前时间）。窗口按事件时间关闭，
	// 这样重启后追赶积压时，旧数据的窗口不会因为墙上时钟早已越过而被当成迟到数据。
	watermark time.Time
	cutoff    time.Time   // 窗口起点早于它的点视为迟到（其窗口已写出）
	pending   []*nats.Msg // 每批最后一条消息（用于 AckAll）

	processed atomic.Uint64
	late      atomic.Uint64
	written   atomic.Uint64
	lastErr   atomic.Value

	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

type rollupWindow struct {
	rec    RollupRecord
	minSeq uint64
}

// RollupStats 汇总器状态
type RollupStats struct {
	Stream      string `json:"stream"`
	Interval    string `json:"interval"`
	Processed   uint64 `json:"processed_points"`
	Late        uint64 `json:"late_points"`
	Written     uint64 `json:"written_records"`
	OpenWindows int    `json:"open_windows"`
	Records     uint64 `json:"records"`
	Bytes       uint64 `json:"bytes"`
	LastError   string `json:"last_error,omitempty"`
}

func (s *Store) rollupStreamConfig() *nats.StreamConfig {
	rc := s.cfg.Rollup
	return &nats.StreamConfig{
		Name:        rc.Stream,
		Subjects:    []string{rollupSubjectPrefix + ">"},
		Storage:     nats.FileStorage,
		Retention:   nats.LimitsPolicy,
		Discard:     nats.DiscardOld,
		MaxAge:      rc.MaxAge,
		MaxBytes:    rc.MaxBytes,
		Replicas:    1,
		Compression: nats.S2Compression,
	}
}

// StartRollup 创建汇总流并启动汇总器
func (s *Store) StartRollup(ctx context.Context) (*Rollup, error) {
	rc := s.cfg.Rollup
	rc.applyDefaults()
	s.cfg.Rollup = rc
	sc := s.rollupStreamConfig()
	if _, err := s.js.StreamInfo(sc.Name); err == nil {
		if _, err := s.js.UpdateStream(sc); err != nil {
			return nil, fmt.Errorf("更新汇总流 %s 失败: %w", sc.Name, err)
		}
	} else if _, err := s.js.AddStream(sc); err != nil {
		return nil, fmt.Errorf("创建汇总流 %s 失败: %w", sc.Name, err)
	}

	hold := rc.Interval + rc.Lateness
	durable := "rollup_" + strings.ToLower(rc.Stream)
	opts := []nats.SubOpt{
		nats.BindStream(s.cfg.Stream),
		nats.ManualAck(),
		nats.AckAll(),
		nats.MaxAckPending(-1),
		// 窗口在内存中最多停留 Interval+Lateness，确认超时必须大于它
		nats.AckWait(2*hold + 30*time.Second),
		nats.MaxDeliver(-1),
	}
	var sub *nats.Subscription
	var err error
	ci, ciErr := s.js.ConsumerInfo(s.cfg.Stream, durable)
	switch {
	case ciErr == nil && ci.NumAckPending == 0:
		// 游标已存在且没有未确认的消息：直接绑定
		sub, err = s.js.PullSubscribe("iot.data.>", durable, nats.Bind(s.cfg.Stream, durable), nats.ManualAck())
	case ciErr == nil:
		// 上次停止（或崩溃）时有未写出的窗口：这些消息要等确认超时才会重投，
		// 因此从确认位置之后重建游标，立即重新汇总
		if err := s.js.DeleteConsumer(s.cfg.Stream, durable); err != nil {
			return nil, fmt.Errorf("重建汇总游标失败: %w", err)
		}
		sub, err = s.js.PullSubscribe("iot.data.>", durable, append(opts, nats.StartSequence(ci.AckFloor.Stream+1))...)
	default:
		sub, err = s.js.PullSubscribe("iot.data.>", durable, append(opts, nats.DeliverAll())...)
	}
	if err != nil {
		return nil, fmt.Errorf("创建汇总消费者失败: %w", err)
	}
	rctx, cancel := context.WithCancel(ctx)
	r := &Rollup{store: s, cfg: rc, sub: sub, open: make(map[rollupKey]*rollupWindow), cancel: cancel, done: make(chan struct{})}
	r.lastErr.Store("")
	s.rollup.Store(r)
	go r.loop(rctx)
	log.Info().Str("stream", rc.Stream).Dur("interval", rc.Interval).Dur("max_age", rc.MaxAge).Msg("汇总层已启动")
	return r, nil
}

func (r *Rollup) loop(ctx context.Context) {
	defer close(r.done)
	for ctx.Err() == nil {
		msgs, err := r.sub.Fetch(r.cfg.BatchSize, nats.MaxWait(time.Second))
		if err != nil && !errors.Is(err, nats.ErrTimeout) && !errors.Is(err, context.DeadlineExceeded) {
			if ctx.Err() != nil || errors.Is(err, nats.ErrConnectionClosed) || errors.Is(err, nats.ErrBadSubscription) {
				return
			}
			r.lastErr.Store(err.Error())
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		if len(msgs) > 0 {
			r.ingest(msgs, time.Now())
		}
		// 没有拉到新消息说明已追平积压，此时可以用当前时间推进水位
		r.flush(ctx, time.Now(), len(msgs) == 0)
	}
	// 停止时不写出未完成的窗口：未确认的原始数据会在重启后重新汇总
}

// ingest 把一批原始消息计入内存中的窗口
func (r *Rollup) ingest(msgs []*nats.Msg, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	interval := r.cfg.Interval
	for _, m := range msgs {
		if m.Header != nil && m.Header.Get(ruleOutputHeader) != "" {
			continue
		}
		var p model.Point
		if json.Unmarshal(m.Data, &p) != nil {
			continue
		}
		v, ok := numeric(p.Value)
		if !ok {
			continue // 非数值点不参与汇总
		}
		r.processed.Add(1)
		start := p.Timestamp.Truncate(interval)
		if start.Before(r.cutoff) {
			r.late.Add(1) // 窗口已写出
			continue
		}
		var seq uint64
		if meta, err := m.Metadata(); err == nil {
			seq = meta.Sequence.Stream
		}
		k := rollupKey{subject: RollupSubject(p.DeviceID, p.Key), start: start.UnixNano()}
		w := r.open[k]
		if w == nil {
			w = &rollupWindow{rec: RollupRecord{DeviceID: p.DeviceID, Key: p.Key, Start: start}, minSeq: seq}
			r.open[k] = w
		}
		w.rec.add(v, p.Timestamp)
		if p.Timestamp.After(r.watermark) && !p.Timestamp.After(now) {
			r.watermark = p.Timestamp
		}
	}
	r.pending = append(r.pending, msgs[len(msgs)-1])
}

// flush 写出已经关闭的窗口（窗口起点 + Interval + Lateness 不晚于水位），然后确认可以确认的原始消息。
// caughtUp 为 true 表示已追平积压，水位可推进到 now。
func (r *Rollup) flush(ctx context.Context, now time.Time, caughtUp bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	wm := r.watermark
	if caughtUp && now.After(wm) {
		wm = now
	}
	hold := r.cfg.Interval + r.cfg.Lateness
	var ready []rollupKey
	for k := range r.open {
		if !time.Unix(0, k.start).Add(hold).After(wm) {
			ready = append(ready, k)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i].start < ready[j].start })
	for _, k := range ready {
		w := r.open[k]
		data, _ := json.Marshal(&w.rec)
		if _, err := r.store.js.Publish(k.subject, data, nats.Context(ctx)); err != nil {
			r.lastErr.Store(err.Error())
			log.Warn().Err(err).Str("subject", k.subject).Msg("写入汇总数据失败，稍后重试")
			return // 保留窗口，下次重试
		}
		r.written.Add(1)
		delete(r.open, k)
	}
	// 第一个尚未关闭的窗口起点：更早窗口的点都已迟到
	if !wm.IsZero() {
		if c := wm.Add(-hold).Truncate(r.cfg.Interval).Add(r.cfg.Interval); c.After(r.cutoff) {
			r.cutoff = c
		}
	}

	// 可确认到：所有未写出窗口中最小序号之前
	ackable := uint64(math.MaxUint64)
	for _, w := range r.open {
		if w.minSeq > 0 && w.minSeq-1 < ackable {
			ackable = w.minSeq - 1
		}
	}
	idx := -1
	for i, m := range r.pending {
		meta, err := m.Metadata()
		if err != nil || meta.Sequence.Stream > ackable {
			break
		}
		idx = i
	}
	if idx >= 0 {
		if err := r.pending[idx].AckSync(nats.Context(ctx)); err != nil && ctx.Err() == nil {
			r.lastErr.Store(err.Error())
			return
		}
		r.pending = r.pending[idx+1:]
	}
}

// Stats 返回汇总器状态
func (r *Rollup) Stats() RollupStats {
	r.mu.Lock()
	open := len(r.open)
	r.mu.Unlock()
	st := RollupStats{
		Stream:      r.cfg.Stream,
		Interval:    r.cfg.Interval.String(),
		Processed:   r.processed.Load(),
		Late:        r.late.Load(),
		Written:     r.written.Load(),
		OpenWindows: open,
	}
	st.LastError, _ = r.lastErr.Load().(string)
	if si, err := r.store.js.StreamInfo(r.cfg.Stream); err == nil {
		st.Records, st.Bytes = si.State.Msgs, si.State.Bytes
	}
	return st
}

// Stop 停止汇总器（未写出的窗口在重启后从原始流重新计算）
func (r *Rollup) Stop() {
	r.once.Do(func() {
		r.cancel()
		<-r.done
	})
}

// Rollup 返回正在运行的汇总器（未启用时为 nil）
func (s *Store) Rollup() *Rollup {
	r, _ := s.rollup.Load().(*Rollup)
	return r
}

// Close 停止存储服务的后台任务（汇总器、告警监控）
func (s *Store) Close() {
	if r := s.Rollup(); r != nil {
		r.Stop()
	}
	if m := s.AlertMonitor(); m != nil {
		m.Stop()
	}
}

// AlertMonitor 返回正在运行的告警监控（未启用时为 nil）
func (s *Store) AlertMonitor() *AlertMonitor {
	m, _ := s.monitor.Load().(*AlertMonitor)
	return m
}

// QueryRollup 查询汇总层：每个窗口返回一个点（值按 Agg 取 avg/min/max/last），超过 Limit 时再降采样
func (s *Store) QueryRollup(ctx context.Context, q RawQuery) (*SeriesResult, error) {
	if err := s.normalize(&q); err != nil {
		return nil, err
	}
	r := s.Rollup()
	if r == nil {
		return nil, fmt.Errorf("%w: 汇总层未开启（storage.rollup.enabled）", ErrInvalidQuery)
	}
	qc := s.cfg.Query
	ctx, cancel := context.WithTimeout(ctx, qc.Timeout)
	defer cancel()
	release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	res := &SeriesResult{DeviceID: q.DeviceID, Key: q.Key, Start: q.Start, End: q.End, Tier: "rollup", Points: []SeriesPoint{}}
	subject := RollupSubject(q.DeviceID, q.Key)
	if _, err := s.js.GetLastMsg(r.cfg.Stream, subject, nats.Context(ctx)); err != nil {
		if errors.Is(err, nats.ErrMsgNotFound) {
			return res, nil
		}
		return nil, fmt.Errorf("读取汇总数据失败: %w", err)
	}
	// 汇总流每个序列每个窗口只有一条记录（30 天约 4.3 万条），从头扫描该序列。
	// 不按入库时间定位：补算的旧窗口和带时钟偏差的窗口，写入时间与窗口时间不对应。
	sub, err := s.js.SubscribeSync(subject, nats.BindStream(r.cfg.Stream), nats.OrderedConsumer(), nats.DeliverAll())
	if err != nil {
		return nil, fmt.Errorf("创建查询消费者失败: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	windows := make(map[int64]RollupRecord)
	from := q.Start.Truncate(r.cfg.Interval)
	stopAt := q.End.Add(r.cfg.Interval + r.cfg.Lateness + qc.Tolerance)
	for {
		m, err := sub.NextMsgWithContext(ctx)
		if err != nil {
			if ctx.Err() != nil {
				res.Truncated = true
				break
			}
			return nil, fmt.Errorf("读取汇总数据失败: %w", err)
		}
		res.Scanned++
		var rec RollupRecord
		pastEnd := false
		if json.Unmarshal(m.Data, &rec) == nil {
			if !rec.Start.Before(from) && !rec.Start.After(q.End) {
				// 同一窗口可能因重启被部分重算后再次写出：取 count 最大的记录
				if old, ok := windows[rec.Start.UnixNano()]; !ok || rec.Count > old.Count {
					windows[rec.Start.UnixNano()] = rec
				}
			}
			pastEnd = rec.Start.After(q.End)
		}
		meta, _ := m.Metadata()
		if meta == nil || meta.NumPending == 0 {
			break
		}
		// 与原始查询相同：写入时间和窗口时间都超过结束时间才提前结束（补算的旧窗口是最近才写入的）
		if pastEnd && meta.Timestamp.After(stopAt) {
			break
		}
		if res.Scanned >= qc.MaxScan {
			res.Truncated = true
			break
		}
	}
	points := make([]SeriesPoint, 0, len(windows))
	for _, rec := range windows {
		res.Matched += int(rec.Count)
		points = append(points, SeriesPoint{Time: rec.Start, Value: rec.value(q.Agg)})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Time.Before(points[j].Time) })
	res.Agg = q.Agg
	if len(points) > q.Limit && !q.NoDownsample {
		agg := q.Agg
		if agg == AggLTTB {
			agg = AggAvg
		}
		points = Downsample(points, q.Start, q.End, q.Limit, agg)
		res.Downsampled = true
	}
	res.Points = points
	return res, nil
}

// 查询数据层
const (
	TierAuto   = "auto"
	TierRaw    = "raw"
	TierRollup = "rollup"
)

// Query 按数据层查询：tier 为 auto（默认）时，起始时间早于原始数据中最早一条的入库时间、
// 且汇总层已开启时查询汇总层，否则查询原始层。
func (s *Store) Query(ctx context.Context, q RawQuery, tier string) (*SeriesResult, error) {
	switch strings.ToLower(tier) {
	case TierRaw:
		return s.QueryRaw(ctx, q)
	case TierRollup:
		return s.QueryRollup(ctx, q)
	case "", TierAuto:
	default:
		return nil, fmt.Errorf("%w: 不支持的 tier %q（可选 auto/raw/rollup）", ErrInvalidQuery, tier)
	}
	if s.Rollup() != nil && !q.Start.IsZero() {
		if si, err := s.js.StreamInfo(s.cfg.Stream, nats.Context(ctx)); err == nil {
			if si.State.Msgs == 0 || q.Start.Before(si.State.FirstTime) {
				return s.QueryRollup(ctx, q)
			}
		}
	}
	return s.QueryRaw(ctx, q)
}
