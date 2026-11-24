package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v3/disk"
)

// AlertConfig 本地暂存告警配置（storage.alerts 段）
type AlertConfig struct {
	Enabled bool
	// Interval 检查间隔，默认 30s
	Interval time.Duration
	// LagWarningRatio / LagCriticalRatio 积压比例阈值，默认 0.5 / 0.8。
	// 积压比例 = max(最早未写出数据的等待时长 / max_age, 积压数据量 / max_bytes)，
	// 达到 1 时最早的积压数据开始被淘汰。
	LagWarningRatio  float64
	LagCriticalRatio float64
	// DiskWarningRatio / DiskCriticalRatio 数据目录所在文件系统的使用率阈值，默认 0.8 / 0.95
	DiskWarningRatio  float64
	DiskCriticalRatio float64
	// Throttle 同一告警（同一类型、连接器和级别）的重复发送间隔，默认 30m
	Throttle time.Duration
}

// DefaultAlertConfig 默认告警配置
func DefaultAlertConfig() AlertConfig {
	return AlertConfig{
		Enabled:           true,
		Interval:          30 * time.Second,
		LagWarningRatio:   0.5,
		LagCriticalRatio:  0.8,
		DiskWarningRatio:  0.8,
		DiskCriticalRatio: 0.95,
		Throttle:          30 * time.Minute,
	}
}

func (c *AlertConfig) applyDefaults() {
	d := DefaultAlertConfig()
	if c.Interval <= 0 {
		c.Interval = d.Interval
	}
	if c.LagWarningRatio <= 0 {
		c.LagWarningRatio = d.LagWarningRatio
	}
	if c.LagCriticalRatio <= 0 {
		c.LagCriticalRatio = d.LagCriticalRatio
	}
	if c.DiskWarningRatio <= 0 {
		c.DiskWarningRatio = d.DiskWarningRatio
	}
	if c.DiskCriticalRatio <= 0 {
		c.DiskCriticalRatio = d.DiskCriticalRatio
	}
	if c.Throttle <= 0 {
		c.Throttle = d.Throttle
	}
}

// 告警类型
const (
	AlertBacklog  = "backlog"   // 积压逼近保留上限
	AlertDataLost = "data_lost" // 有数据在写入目标前被淘汰
	AlertDisk     = "disk"      // 数据目录所在磁盘快满
)

// StorageAlert 一条本地暂存告警
type StorageAlert struct {
	Kind    string  `json:"kind"`
	Sink    string  `json:"sink,omitempty"`
	Level   string  `json:"level"` // warning | critical
	Message string  `json:"message"`
	Value   float64 `json:"value"`
}

// AlertInput 告警评估的输入
type AlertInput struct {
	MaxAge time.Duration
	Stream *StreamStatus
	Sinks  []RunnerStats
	// PrevLost 上次检查时每个连接器的累计丢失量；为 nil 时只要有丢失就告警（用于健康检查）
	PrevLost map[string]uint64
	// DiskRatio 数据目录所在文件系统的使用率，小于 0 表示未知
	DiskRatio float64
}

// BacklogRatio 积压比例：max(等待时长 / max_age, 积压数据量 / max_bytes)。达到 1 时开始丢数据。
func BacklogRatio(st RunnerStats, stream *StreamStatus, maxAge time.Duration) float64 {
	ratio := 0.0
	if maxAge > 0 && st.LagSeconds > 0 {
		ratio = st.LagSeconds / maxAge.Seconds()
	}
	// 积压条数占流中消息的比例 × 流占用率 ≈ 积压数据量 / max_bytes
	if stream != nil && stream.Messages > 0 && st.Pending > 0 {
		r := math.Min(1, float64(st.Pending)/float64(stream.Messages)) * stream.Usage
		if r > ratio {
			ratio = r
		}
	}
	return ratio
}

// EvaluateAlerts 根据当前状态计算告警（纯函数）
func EvaluateAlerts(cfg AlertConfig, in AlertInput) []StorageAlert {
	cfg.applyDefaults()
	var out []StorageAlert
	for _, st := range in.Sinks {
		prev, known := in.PrevLost[st.Name]
		if in.PrevLost == nil {
			prev, known = 0, true
		}
		if known && st.Lost > prev {
			out = append(out, StorageAlert{Kind: AlertDataLost, Sink: st.Name, Level: "critical", Value: float64(st.Lost),
				Message: fmt.Sprintf("连接器 %s 有 %d 条积压数据在写入目标前因超出保留上限被淘汰（累计 %d 条）", st.Name, st.Lost-prev, st.Lost)})
		}
		ratio := BacklogRatio(st, in.Stream, in.MaxAge)
		level := ""
		switch {
		case ratio >= cfg.LagCriticalRatio:
			level = "critical"
		case ratio >= cfg.LagWarningRatio:
			level = "warning"
		}
		if level != "" {
			out = append(out, StorageAlert{Kind: AlertBacklog, Sink: st.Name, Level: level, Value: ratio,
				Message: fmt.Sprintf("连接器 %s 积压 %d 条，最早一条已等待 %s，已达保留上限的 %.0f%%；目标继续不可用将开始丢失数据",
					st.Name, st.Pending, (time.Duration(st.LagSeconds) * time.Second).String(), ratio*100)})
		}
	}
	if in.DiskRatio >= 0 {
		level := ""
		switch {
		case in.DiskRatio >= cfg.DiskCriticalRatio:
			level = "critical"
		case in.DiskRatio >= cfg.DiskWarningRatio:
			level = "warning"
		}
		if level != "" {
			out = append(out, StorageAlert{Kind: AlertDisk, Level: level, Value: in.DiskRatio,
				Message: fmt.Sprintf("本地暂存所在磁盘已使用 %.0f%%，磁盘写满后采集数据将无法暂存", in.DiskRatio*100)})
		}
	}
	return out
}

// diskUsageRatio 返回目录所在文件系统的使用率（可在测试中替换）
var diskUsageRatio = func(dir string) float64 {
	if dir == "" {
		return -1
	}
	u, err := disk.Usage(dir)
	if err != nil || u.Total == 0 {
		return -1
	}
	return u.UsedPercent / 100
}

// AlertMonitor 周期性评估本地暂存状态，把告警发布到 iot.alerts.triggered
// （与规则引擎告警格式一致，Web 告警集成服务会创建告警并发送通知）
type AlertMonitor struct {
	store *Store
	nc    *nats.Conn
	sinks func() []RunnerStats
	cfg   AlertConfig

	mu       sync.Mutex
	prevLost map[string]uint64
	lastSent map[string]time.Time
	current  []StorageAlert

	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

// StartAlertMonitor 启动告警监控；sinks 返回各缓冲连接器的状态
func (s *Store) StartAlertMonitor(ctx context.Context, nc *nats.Conn, sinks func() []RunnerStats) *AlertMonitor {
	cfg := s.cfg.Alerts
	cfg.applyDefaults()
	mctx, cancel := context.WithCancel(ctx)
	m := &AlertMonitor{store: s, nc: nc, sinks: sinks, cfg: cfg, prevLost: map[string]uint64{},
		lastSent: map[string]time.Time{}, cancel: cancel, done: make(chan struct{})}
	s.monitor.Store(m)
	go func() {
		defer close(m.done)
		t := time.NewTicker(cfg.Interval)
		defer t.Stop()
		for {
			select {
			case <-mctx.Done():
				return
			case <-t.C:
				m.Check(time.Now())
			}
		}
	}()
	return m
}

// Check 执行一次检查并发布新的告警，返回当前告警
func (m *AlertMonitor) Check(now time.Time) []StorageAlert {
	status, err := m.store.Status()
	if err != nil {
		log.Warn().Err(err).Msg("读取本地暂存状态失败")
		return nil
	}
	var sinks []RunnerStats
	if m.sinks != nil {
		sinks = m.sinks()
	}
	m.mu.Lock()
	prev := make(map[string]uint64, len(m.prevLost))
	for k, v := range m.prevLost {
		prev[k] = v
	}
	m.mu.Unlock()
	alerts := EvaluateAlerts(m.cfg, AlertInput{
		MaxAge: m.store.cfg.MaxAge, Stream: status, Sinks: sinks, PrevLost: prev,
		DiskRatio: diskUsageRatio(m.store.cfg.StoreDir),
	})

	m.mu.Lock()
	for _, st := range sinks {
		m.prevLost[st.Name] = st.Lost
	}
	m.current = alerts
	var toSend []StorageAlert
	for _, a := range alerts {
		key := a.Kind + "|" + a.Sink + "|" + a.Level
		// 数据丢失每次新增都发送；其他告警按 Throttle 去重（级别升高时立即发送）
		if a.Kind != AlertDataLost && now.Sub(m.lastSent[key]) < m.cfg.Throttle {
			continue
		}
		m.lastSent[key] = now
		toSend = append(toSend, a)
	}
	m.mu.Unlock()

	for _, a := range toSend {
		m.publish(a, now)
	}
	return alerts
}

func (m *AlertMonitor) publish(a StorageAlert, now time.Time) {
	log.Warn().Str("kind", a.Kind).Str("sink", a.Sink).Str("level", a.Level).Msg(a.Message)
	if m.nc == nil {
		return
	}
	key := a.Kind
	if a.Sink != "" {
		key += ":" + a.Sink
	}
	payload := map[string]interface{}{
		"id":        fmt.Sprintf("storage-%s-%d", strings.ReplaceAll(key, ":", "-"), now.UnixNano()),
		"rule_id":   "storage." + a.Kind,
		"rule_name": "本地暂存",
		"level":     a.Level,
		"message":   a.Message,
		"device_id": "gateway",
		"key":       key,
		"value":     a.Value,
		"tags":      map[string]string{"source": "storage", "sink": a.Sink},
		"timestamp": now,
		// 与规则引擎相同：Web 告警集成服务按 rule_id/device_id/key 节流
		"throttle": int64(m.cfg.Throttle),
	}
	data, _ := json.Marshal(payload)
	_ = m.nc.Publish("iot.alerts.triggered", data)
	_ = m.nc.Publish("iot.alerts.triggered."+a.Level, data)
}

// Current 返回最近一次检查的告警
func (m *AlertMonitor) Current() []StorageAlert {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]StorageAlert(nil), m.current...)
}

// Stop 停止告警监控
func (m *AlertMonitor) Stop() {
	m.once.Do(func() {
		m.cancel()
		<-m.done
	})
}

// Assess 用于健康检查：返回当前所有告警（有任何累计丢失都计入）
func (s *Store) Assess(sinks []RunnerStats) ([]StorageAlert, error) {
	status, err := s.Status()
	if err != nil {
		return nil, err
	}
	return EvaluateAlerts(s.cfg.Alerts, AlertInput{MaxAge: s.cfg.MaxAge, Stream: status, Sinks: sinks,
		DiskRatio: diskUsageRatio(s.cfg.StoreDir)}), nil
}

// PrometheusText 输出本地暂存的 Prometheus 文本格式指标
func (s *Store) PrometheusText(sinks []RunnerStats) string {
	var b strings.Builder
	w := func(name, labels string, v float64) {
		if labels != "" {
			fmt.Fprintf(&b, "%s{%s} %g\n", name, labels, v)
		} else {
			fmt.Fprintf(&b, "%s %g\n", name, v)
		}
	}
	b.WriteString("# 本地暂存\n")
	w("iot_gateway_storage_enabled", "", 1)
	status, err := s.Status()
	if err != nil {
		w("iot_gateway_storage_status_error", "", 1)
		return b.String()
	}
	w("iot_gateway_storage_bytes", "", float64(status.Bytes))
	w("iot_gateway_storage_messages", "", float64(status.Messages))
	w("iot_gateway_storage_usage_ratio", "", status.Usage)
	if ratio := diskUsageRatio(s.cfg.StoreDir); ratio >= 0 {
		w("iot_gateway_storage_disk_usage_ratio", "", ratio)
	}
	sorted := append([]RunnerStats(nil), sinks...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, st := range sorted {
		l := fmt.Sprintf(`sink=%q`, st.Name)
		w("iot_gateway_storage_sink_pending", l, float64(st.Pending))
		w("iot_gateway_storage_sink_lag_seconds", l, st.LagSeconds)
		w("iot_gateway_storage_sink_backlog_ratio", l, BacklogRatio(st, status, s.cfg.MaxAge))
		w("iot_gateway_storage_sink_delivered_total", l, float64(st.Delivered))
		w("iot_gateway_storage_sink_failed_batches_total", l, float64(st.Failed))
		w("iot_gateway_storage_sink_discarded_total", l, float64(st.Discarded))
		w("iot_gateway_storage_sink_lost_total", l, float64(st.Lost))
		for _, state := range []string{StateRealtime, StateReplaying, StateBuffering, StatePaused, StateStopped} {
			v := 0.0
			if st.State == state {
				v = 1
			}
			w("iot_gateway_storage_sink_state", fmt.Sprintf(`sink=%q,state=%q`, st.Name, state), v)
		}
	}
	if r := s.Rollup(); r != nil {
		rs := r.Stats()
		w("iot_gateway_storage_rollup_records", "", float64(rs.Records))
		w("iot_gateway_storage_rollup_bytes", "", float64(rs.Bytes))
		w("iot_gateway_storage_rollup_open_windows", "", float64(rs.OpenWindows))
		w("iot_gateway_storage_rollup_late_points_total", "", float64(rs.Late))
	}
	return b.String()
}
