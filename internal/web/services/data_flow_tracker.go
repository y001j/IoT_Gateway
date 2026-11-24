package services

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/web/models"
)

// dataFlowTracker 统计每个序列（iot.data.<device>.<key>）的数据流量。
//
// 以前适配器监控常驻订阅 iot.data.>，每条消息都做一次 JSON 解析并获取全局锁，
// 在每秒几十万点的采集速率下是一笔隐藏的开销（只要开启 Web 界面就存在）。现在：
//   - 按需订阅：首次查询数据流指标时才订阅，idleTimeout 内无人查询则退订；
//   - 每条消息只做原子计数（无锁、无解析）；
//   - 每个序列每秒最多解析一条消息，用于最新值和采集延迟；
//   - 速率由周期性计数快照计算，不再受记录条数上限影响。
type dataFlowTracker struct {
	nc          *nats.Conn
	idleTimeout time.Duration
	parseEvery  time.Duration
	maxSnaps    int

	mu         sync.Mutex
	sub        *nats.Subscription
	lastQuery  time.Time
	subscribed time.Time

	flows sync.Map // subject -> *flowCounter
}

type flowSnapshot struct {
	at    time.Time
	count int64
	bytes int64
}

type flowCounter struct {
	count     atomic.Int64
	bytes     atomic.Int64
	lastSeen  atomic.Int64 // UnixNano
	lastParse atomic.Int64 // UnixNano

	mu            sync.Mutex
	deviceID, key string
	lastValue     interface{}
	lastTimestamp time.Time
	latencySum    time.Duration
	latencyN      int64
	snaps         []flowSnapshot
}

func newDataFlowTracker(nc *nats.Conn) *dataFlowTracker {
	return &dataFlowTracker{
		nc:          nc,
		idleTimeout: 5 * time.Minute,
		parseEvery:  time.Second,
		maxSnaps:    360, // 每 10 秒一次，保留 1 小时
	}
}

// ensureSubscribed 记录一次查询，未订阅时开始订阅
func (t *dataFlowTracker) ensureSubscribed() {
	if t == nil || t.nc == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastQuery = time.Now()
	if t.sub != nil {
		return
	}
	sub, err := t.nc.Subscribe("iot.data.>", t.handle)
	if err != nil {
		log.Warn().Err(err).Msg("订阅数据流失败")
		return
	}
	// 订阅只用于统计，允许在高负载时丢弃消息而不是拖慢连接
	_ = sub.SetPendingLimits(100000, 64<<20)
	t.sub = sub
	t.subscribed = time.Now()
	log.Info().Msg("开始统计数据流（无人查询 5 分钟后自动停止）")
}

// stopIfIdle 超过 idleTimeout 无人查询时退订
func (t *dataFlowTracker) stopIfIdle(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sub != nil && now.Sub(t.lastQuery) > t.idleTimeout {
		_ = t.sub.Unsubscribe()
		t.sub = nil
		log.Info().Msg("数据流统计已停止（无人查询）")
	}
}

func (t *dataFlowTracker) active() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sub != nil
}

func (t *dataFlowTracker) stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sub != nil {
		_ = t.sub.Unsubscribe()
		t.sub = nil
	}
}

func (t *dataFlowTracker) counter(subject string) *flowCounter {
	if v, ok := t.flows.Load(subject); ok {
		return v.(*flowCounter)
	}
	fc := &flowCounter{}
	// 主题 iot.data.<device>.<key>：在解析到消息之前先用主题还原设备和点位
	if parts := strings.Split(subject, "."); len(parts) == 4 {
		fc.deviceID, fc.key = model.DecodeSubjectToken(parts[2]), model.DecodeSubjectToken(parts[3])
	}
	v, _ := t.flows.LoadOrStore(subject, fc)
	return v.(*flowCounter)
}

// handle 每条消息的处理：只做原子计数，按序列限频解析
func (t *dataFlowTracker) handle(msg *nats.Msg) {
	fc := t.counter(msg.Subject)
	now := time.Now().UnixNano()
	fc.count.Add(1)
	fc.bytes.Add(int64(len(msg.Data)))
	fc.lastSeen.Store(now)

	last := fc.lastParse.Load()
	if now-last < int64(t.parseEvery) || !fc.lastParse.CompareAndSwap(last, now) {
		return
	}
	var p model.Point
	if err := json.Unmarshal(msg.Data, &p); err != nil {
		return
	}
	fc.mu.Lock()
	if p.DeviceID != "" {
		fc.deviceID, fc.key = p.DeviceID, p.Key
	}
	fc.lastValue = p.Value
	fc.lastTimestamp = p.Timestamp
	if !p.Timestamp.IsZero() {
		fc.latencySum += time.Since(p.Timestamp)
		fc.latencyN++
	}
	fc.mu.Unlock()
}

// snapshot 记录每个序列的计数快照（由监控循环周期性调用）
func (t *dataFlowTracker) snapshot(now time.Time) {
	t.flows.Range(func(_, v interface{}) bool {
		fc := v.(*flowCounter)
		fc.mu.Lock()
		fc.snaps = append(fc.snaps, flowSnapshot{at: now, count: fc.count.Load(), bytes: fc.bytes.Load()})
		if len(fc.snaps) > t.maxSnaps {
			fc.snaps = fc.snaps[len(fc.snaps)-t.maxSnaps:]
		}
		fc.mu.Unlock()
		return true
	})
}

// cleanup 删除超过 retention 没有数据的序列
func (t *dataFlowTracker) cleanup(now time.Time, retention time.Duration) {
	t.flows.Range(func(k, v interface{}) bool {
		if now.Sub(time.Unix(0, v.(*flowCounter).lastSeen.Load())) > retention {
			t.flows.Delete(k)
		}
		return true
	})
}

// metrics 计算 window 时间窗口内的速率（窗口早于统计开始时，从统计开始计算）
func (t *dataFlowTracker) metrics(window time.Duration, now time.Time) []models.DataFlowMetrics {
	var out []models.DataFlowMetrics
	cutoff := now.Add(-window)
	started := t.subscribedAt()
	t.flows.Range(func(_, v interface{}) bool {
		fc := v.(*flowCounter)
		lastSeen := time.Unix(0, fc.lastSeen.Load())
		if lastSeen.Before(cutoff) {
			return true
		}
		count, bytes := fc.count.Load(), fc.bytes.Load()
		fc.mu.Lock()
		// 基准：窗口起点之前最近的一个快照；没有则从 0 开始（统计开始时间）
		baseAt, baseCount, baseBytes := started, int64(0), int64(0)
		for _, s := range fc.snaps {
			if s.at.After(cutoff) {
				break
			}
			baseAt, baseCount, baseBytes = s.at, s.count, s.bytes
		}
		m := models.DataFlowMetrics{
			AdapterName:   fc.deviceID,
			DeviceID:      fc.deviceID,
			Key:           fc.key,
			LastValue:     fc.lastValue,
			LastTimestamp: fc.lastTimestamp,
		}
		if fc.latencyN > 0 {
			m.LatencyMS = float64((fc.latencySum / time.Duration(fc.latencyN)).Nanoseconds()) / 1e6
		}
		fc.mu.Unlock()
		if elapsed := now.Sub(baseAt).Seconds(); elapsed > 0 {
			m.DataPointsPerSec = float64(count-baseCount) / elapsed
			m.BytesPerSec = float64(bytes-baseBytes) / elapsed
		}
		out = append(out, m)
		return true
	})
	sort.Slice(out, func(i, j int) bool { return out[i].DataPointsPerSec > out[j].DataPointsPerSec })
	return out
}

func (t *dataFlowTracker) subscribedAt() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.subscribed
}
