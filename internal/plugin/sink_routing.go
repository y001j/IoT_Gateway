package plugin

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

// rawDataSubject 原始采集数据：不经 NATS 订阅，由管道直接分发给连接器（开启 buffer 时由本地暂存补发）
const rawDataSubject = "iot.data.>"

// sinkSubscription 连接器订阅的一个 NATS 主题
type sinkSubscription struct {
	Subject  string
	DataType string // raw | rule | alert | system
}

// pointFilter 按设备 ID、数据键过滤（path.Match 通配符，例如 temp_*；列表为空表示不过滤）
type pointFilter struct {
	Devices []string
	Keys    []string
}

func (f pointFilter) empty() bool { return len(f.Devices) == 0 && len(f.Keys) == 0 }

func matchAny(patterns []string, s string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if ok, _ := path.Match(p, s); ok {
			return true
		}
	}
	return false
}

func (f pointFilter) match(p model.Point) bool {
	return matchAny(f.Devices, p.DeviceID) && matchAny(f.Keys, p.Key)
}

// apply 返回通过过滤的数据点（不修改原批次）
func (f pointFilter) apply(batch []model.Point) []model.Point {
	if f.empty() {
		return batch
	}
	out := make([]model.Point, 0, len(batch))
	for _, p := range batch {
		if f.match(p) {
			out = append(out, p)
		}
	}
	return out
}

// sinkRouting 连接器接收哪些数据：
//
//	subscriptions: ["iot.data.>", "iot.rules.>", {subject: "iot.alerts.>", data_type: alert}]
//	filter: {devices: ["temp_*"], keys: ["temperature"]}
//
// 不配置 subscriptions 时只接收原始采集数据（以前的行为）。以前要把规则结果写入 InfluxDB 等连接器，
// 必须再配置一个 nats_subscriber 并在 target_sinks 里把连接器重写一遍；现在任何连接器都可以直接订阅。
type sinkRouting struct {
	Raw    bool
	Subs   []sinkSubscription
	Filter pointFilter
}

// needsWrapper 是否需要过滤或串行化写入（多个数据来源）
func (r sinkRouting) needsWrapper() bool { return !r.Filter.empty() || len(r.Subs) > 0 }

func defaultDataType(subject string) string {
	switch {
	case strings.HasPrefix(subject, "iot.alerts."):
		return "alert"
	case strings.HasPrefix(subject, "iot.rules."):
		return "rule"
	default:
		return "raw"
	}
}

func stringList(v interface{}, field string) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s 必须是列表", field)
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("%s 中的每一项必须是非空字符串", field)
		}
		if _, err := path.Match(s, ""); err != nil {
			return nil, fmt.Errorf("%s 中的通配符 %q 无效: %w", field, s, err)
		}
		out = append(out, s)
	}
	return out, nil
}

// parseSinkRouting 从连接器配置中读取 subscriptions 和 filter
func parseSinkRouting(cfg map[string]interface{}) (sinkRouting, error) {
	r := sinkRouting{Raw: true}
	if raw, ok := cfg["subscriptions"]; ok && raw != nil {
		list, ok := raw.([]interface{})
		if !ok || len(list) == 0 {
			return r, fmt.Errorf("subscriptions 必须是非空列表")
		}
		r.Raw = false
		seen := map[string]bool{}
		for _, item := range list {
			var sub sinkSubscription
			switch v := item.(type) {
			case string:
				sub.Subject = v
			case map[string]interface{}:
				sub.Subject, _ = v["subject"].(string)
				sub.DataType, _ = v["data_type"].(string)
			default:
				return r, fmt.Errorf("subscriptions 的每一项必须是主题字符串或 {subject, data_type}")
			}
			sub.Subject = strings.TrimSpace(sub.Subject)
			if sub.Subject == "" {
				return r, fmt.Errorf("subscriptions 中的主题不能为空")
			}
			if seen[sub.Subject] {
				continue
			}
			seen[sub.Subject] = true
			if sub.Subject == rawDataSubject {
				r.Raw = true
				continue
			}
			if strings.HasPrefix(sub.Subject, "iot.data.") {
				// 原始数据走管道直接分发（不重复经过 NATS），按设备、键筛选请用 filter
				return r, fmt.Errorf("原始数据请订阅 %s 并用 filter 按设备、数据键筛选，而不是 %s", rawDataSubject, sub.Subject)
			}
			if sub.DataType == "" {
				sub.DataType = defaultDataType(sub.Subject)
			}
			switch sub.DataType {
			case "raw", "rule", "alert", "system":
			default:
				return r, fmt.Errorf("主题 %s 的 data_type %q 无效（raw / rule / alert / system）", sub.Subject, sub.DataType)
			}
			r.Subs = append(r.Subs, sub)
		}
	}
	if f, ok := cfg["filter"]; ok && f != nil {
		fm, ok := f.(map[string]interface{})
		if !ok {
			return r, fmt.Errorf("filter 必须是对象，例如 {devices: [...], keys: [...]}")
		}
		var err error
		if r.Filter.Devices, err = stringList(fm["devices"], "filter.devices"); err != nil {
			return r, err
		}
		if r.Filter.Keys, err = stringList(fm["keys"], "filter.keys"); err != nil {
			return r, err
		}
	}
	return r, nil
}

// routedSink 包装连接器：先过滤，再把写入串行化（原始数据分发器、缓冲运行器、订阅分发器可能同时写入）
type routedSink struct {
	northbound.Sink
	filter pointFilter
	mu     *sync.Mutex
}

func (s *routedSink) Publish(batch []model.Point) error {
	batch = s.filter.apply(batch)
	if len(batch) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Sink.Publish(batch)
}

// Healthy 转发连接器的健康检查（没有实现时视为健康，与未包装时缓冲运行器的行为一致）
func (s *routedSink) Healthy() error {
	if hc, ok := s.Sink.(northbound.HealthChecker); ok {
		return hc.Healthy()
	}
	return nil
}

// routedBatchSink 连接器实现了 BatchWriter 时使用（缓冲运行器据此选择阻塞写入）
type routedBatchSink struct {
	*routedSink
	bw northbound.BatchWriter
}

func (s *routedBatchSink) WriteBatch(ctx context.Context, batch []model.Point) error {
	batch = s.filter.apply(batch)
	if len(batch) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bw.WriteBatch(ctx, batch)
}

func newRoutedSink(sink northbound.Sink, filter pointFilter) northbound.Sink {
	rs := &routedSink{Sink: sink, filter: filter, mu: &sync.Mutex{}}
	if bw, ok := sink.(northbound.BatchWriter); ok {
		return &routedBatchSink{routedSink: rs, bw: bw}
	}
	return rs
}

// subscriptionFeed 把连接器订阅的 NATS 消息组批后交给它自己的分发器（与原始数据的队列分开）
type subscriptionFeed struct {
	subs    []*nats.Subscription
	disp    *sinkDispatcher
	ch      chan model.Point
	stopCh  chan struct{}
	done    chan struct{}
	once    sync.Once
	started bool
}

func (m *Manager) startSubscriptionFeed(name string, target northbound.Sink, subs []sinkSubscription) error {
	if m.bus == nil {
		return fmt.Errorf("没有 NATS 连接，无法订阅")
	}
	f := &subscriptionFeed{
		disp:   newSinkDispatcher(name+" (订阅)", target, m.sinkQueueSize),
		ch:     make(chan model.Point, 1000),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
	for _, s := range subs {
		dataType := s.DataType
		sub, err := m.bus.Subscribe(s.Subject, func(msg *nats.Msg) {
			p, err := northbound.ParseSubscribedMessage(msg.Data, dataType)
			if err != nil {
				log.Debug().Err(err).Str("sink", name).Str("subject", msg.Subject).Msg("忽略无法解析的订阅消息")
				return
			}
			select {
			case f.ch <- p:
			default:
				f.disp.droppedPoints.Add(1)
			}
		})
		if err != nil {
			f.stop()
			return fmt.Errorf("订阅 %s 失败: %w", s.Subject, err)
		}
		f.subs = append(f.subs, sub)
	}
	f.started = true
	go f.run(m.batchSize, m.flushInterval)

	m.dispMu.Lock()
	old := m.feeds[name]
	m.feeds[name] = f
	m.dispMu.Unlock()
	if old != nil {
		old.stop()
	}
	log.Info().Str("sink", name).Int("subscriptions", len(subs)).Msg("连接器已订阅 NATS 主题")
	return nil
}

// run 按 batchSize / flushInterval 组批
func (f *subscriptionFeed) run(batchSize int, flushInterval time.Duration) {
	defer close(f.done)
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushInterval <= 0 {
		flushInterval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	batch := make([]model.Point, 0, batchSize)
	flush := func() {
		if len(batch) > 0 {
			f.disp.enqueue(batch)
			batch = make([]model.Point, 0, batchSize)
		}
	}
	for {
		select {
		case <-f.stopCh:
			flush()
			return
		case p := <-f.ch:
			batch = append(batch, p)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// stop 取消订阅，写完已收到的数据后停止分发器
func (f *subscriptionFeed) stop() {
	f.once.Do(func() {
		for _, s := range f.subs {
			_ = s.Unsubscribe()
		}
		close(f.stopCh)
		if f.started {
			select {
			case <-f.done:
			case <-time.After(5 * time.Second):
			}
		}
		f.disp.stop(5 * time.Second)
	})
}

// removeFeed 停止连接器的订阅
func (m *Manager) removeFeed(name string) {
	m.dispMu.Lock()
	f := m.feeds[name]
	delete(m.feeds, name)
	m.dispMu.Unlock()
	if f != nil {
		f.stop()
	}
}
