package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
	_ "github.com/y001j/iot-gateway/internal/southbound/mock"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func TestParseSinkRouting(t *testing.T) {
	r, err := parseSinkRouting(map[string]interface{}{})
	if err != nil || !r.Raw || len(r.Subs) != 0 || r.needsWrapper() {
		t.Fatalf("默认只接收原始数据: %+v %v", r, err)
	}
	r, err = parseSinkRouting(map[string]interface{}{
		"subscriptions": []interface{}{"iot.rules.>", "iot.rules.>", map[string]interface{}{"subject": "iot.alerts.>"},
			map[string]interface{}{"subject": "custom.events", "data_type": "system"}, "other.raw"},
		"filter": map[string]interface{}{"devices": []interface{}{"temp_*"}, "keys": []interface{}{"temperature"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Raw || len(r.Subs) != 4 || r.Subs[0].DataType != "rule" || r.Subs[1].DataType != "alert" ||
		r.Subs[2].DataType != "system" || r.Subs[3].DataType != "raw" || !r.needsWrapper() {
		t.Fatalf("解析结果错误: %+v", r)
	}
	if r, _ := parseSinkRouting(map[string]interface{}{"subscriptions": []interface{}{"iot.data.>"}}); !r.Raw || len(r.Subs) != 0 {
		t.Fatalf("iot.data.> 表示原始数据: %+v", r)
	}
	for name, cfg := range map[string]map[string]interface{}{
		"不是列表":         {"subscriptions": "iot.rules.>"},
		"空列表":          {"subscriptions": []interface{}{}},
		"项类型错误":        {"subscriptions": []interface{}{1}},
		"空主题":          {"subscriptions": []interface{}{" "}},
		"原始数据子主题":      {"subscriptions": []interface{}{"iot.data.d1.>"}},
		"无效 data_type": {"subscriptions": []interface{}{map[string]interface{}{"subject": "a.b", "data_type": "x"}}},
		"filter 不是对象":  {"filter": "x"},
		"设备不是列表":       {"filter": map[string]interface{}{"devices": "d1"}},
		"空字符串":         {"filter": map[string]interface{}{"keys": []interface{}{""}}},
		"无效通配符":        {"filter": map[string]interface{}{"devices": []interface{}{"["}}},
	} {
		if _, err := parseSinkRouting(cfg); err == nil {
			t.Errorf("%s 应报错", name)
		}
	}
	f := pointFilter{Devices: []string{"temp_*", "gw"}, Keys: []string{"t*"}}
	pts := []model.Point{{DeviceID: "temp_1", Key: "temp"}, {DeviceID: "temp_1", Key: "hum"}, {DeviceID: "x", Key: "temp"}, {DeviceID: "gw", Key: "t"}}
	if got := f.apply(pts); len(got) != 2 || got[0].Key != "temp" || got[1].DeviceID != "gw" {
		t.Fatalf("过滤结果错误: %+v", got)
	}
	if got := (pointFilter{}).apply(pts); len(got) != 4 {
		t.Fatal("空过滤器不过滤")
	}
}

// recordingSink 记录收到的数据，并检测是否被并发调用
type recordingSink struct {
	name    string
	mu      sync.Mutex
	points  []model.Point
	inside  atomic.Int32
	overlap atomic.Bool
	health  error
}

func (s *recordingSink) Name() string                   { return s.name }
func (s *recordingSink) Init(cfg json.RawMessage) error { return nil }
func (s *recordingSink) Start(ctx context.Context) error {
	return nil
}
func (s *recordingSink) Stop() error { return nil }
func (s *recordingSink) Publish(batch []model.Point) error {
	if s.inside.Add(1) > 1 {
		s.overlap.Store(true)
	}
	time.Sleep(time.Millisecond)
	s.mu.Lock()
	s.points = append(s.points, batch...)
	s.mu.Unlock()
	s.inside.Add(-1)
	return nil
}
func (s *recordingSink) snapshot() []model.Point {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]model.Point(nil), s.points...)
}

type batchRecordingSink struct{ recordingSink }

func (s *batchRecordingSink) WriteBatch(ctx context.Context, batch []model.Point) error {
	return s.Publish(batch)
}
func (s *batchRecordingSink) Healthy() error { return s.health }

func TestRoutedSinkFiltersAndSerializes(t *testing.T) {
	inner := &recordingSink{name: "r"}
	rs := newRoutedSink(inner, pointFilter{Keys: []string{"keep"}})
	if _, ok := rs.(northbound.BatchWriter); ok {
		t.Fatal("没有实现 BatchWriter 的连接器包装后也不应实现")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rs.Publish([]model.Point{{Key: "keep"}, {Key: "drop"}})
		}()
	}
	wg.Wait()
	if err := rs.Publish([]model.Point{{Key: "drop"}}); err != nil {
		t.Fatal(err)
	}
	if got := inner.snapshot(); len(got) != 8 || inner.overlap.Load() {
		t.Fatalf("应过滤并串行写入: %d 个点, 并发=%v", len(got), inner.overlap.Load())
	}
	if rs.(northbound.HealthChecker).Healthy() != nil {
		t.Fatal("没有健康检查的连接器视为健康")
	}

	bInner := &batchRecordingSink{recordingSink: recordingSink{name: "b", health: errors.New("down")}}
	brs := newRoutedSink(bInner, pointFilter{Devices: []string{"d1"}})
	bw, ok := brs.(northbound.BatchWriter)
	if !ok {
		t.Fatal("实现了 BatchWriter 的连接器包装后应保留")
	}
	if err := bw.WriteBatch(context.Background(), []model.Point{{DeviceID: "d1"}, {DeviceID: "d2"}}); err != nil {
		t.Fatal(err)
	}
	if err := bw.WriteBatch(context.Background(), []model.Point{{DeviceID: "d2"}}); err != nil {
		t.Fatal(err)
	}
	if got := bInner.snapshot(); len(got) != 1 || got[0].DeviceID != "d1" {
		t.Fatalf("WriteBatch 应过滤: %+v", got)
	}
	if brs.(northbound.HealthChecker).Healthy() == nil {
		t.Fatal("应转发健康检查")
	}
}

func init() {
	northbound.Register("test_recording", func() northbound.Sink { return &recordingSink{} })
}

// recordingByName 返回插件管理器创建的测试连接器实例
func recordingByName(t *testing.T, m *Manager, name string) *recordingSink {
	t.Helper()
	s, ok := m.GetSink(name)
	if !ok {
		t.Fatalf("连接器 %s 未创建", name)
	}
	return s.(*recordingSink)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("等待超时: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// 任何连接器都可以直接订阅规则结果、告警，并按设备、数据键过滤原始数据（以前需要 nats_subscriber + target_sinks）
func TestSinkSubscriptionsEndToEnd(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	v := viper.New()
	v.Set("southbound.adapters", []interface{}{map[string]interface{}{"name": "m", "type": "mock",
		"config": map[string]interface{}{"name": "m", "type": "mock", "interval": "20ms", "data_points": []interface{}{
			map[string]interface{}{"device_id": "d1", "key": "temp", "min_value": 1.0, "max_value": 2.0},
			map[string]interface{}{"device_id": "d2", "key": "hum", "min_value": 1.0, "max_value": 2.0}}}}})
	v.Set("northbound.sinks", []interface{}{
		map[string]interface{}{"name": "raw_d1", "type": "test_recording", "filter": map[string]interface{}{"devices": []interface{}{"d1"}}},
		map[string]interface{}{"name": "rules_only", "type": "test_recording", "subscriptions": []interface{}{"iot.rules.>"},
			"buffer": map[string]interface{}{"enabled": true}},
		map[string]interface{}{"name": "both", "type": "test_recording",
			"subscriptions": []interface{}{"iot.data.>", map[string]interface{}{"subject": "iot.alerts.>"}}},
		map[string]interface{}{"name": "bad", "type": "test_recording", "subscriptions": []interface{}{"iot.data.d1.>"}},
	})
	m := NewManager(t.TempDir(), nc, v)
	if err := m.Init(nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(context.Background())

	if _, ok := m.GetSink("bad"); ok {
		t.Fatal("subscriptions 配置无效的连接器应被跳过")
	}
	rawD1, rulesOnly, both := recordingByName(t, m, "raw_d1"), recordingByName(t, m, "rules_only"), recordingByName(t, m, "both")

	// 规则输出与告警
	ruleOut := `{"device_id":"d1","key":"temp_f","value":35.6,"type":"float","timestamp":"2026-11-27T00:00:00Z"}`
	alert := `{"id":"a1","rule_id":"r","device_id":"d1","key":"temp","value":40,"level":"warning","message":"高温","timestamp":"2026-11-27T00:00:00Z"}`
	waitFor(t, "订阅生效", func() bool {
		_ = nc.Publish("iot.rules.transform.d1.temp_f", []byte(ruleOut))
		_ = nc.Publish("iot.alerts.triggered", []byte(alert))
		_ = nc.Publish("iot.rules.bad", []byte("not json"))
		return len(rulesOnly.snapshot()) > 0 && len(both.snapshot()) > 0
	})
	waitFor(t, "原始数据", func() bool {
		hasRaw, hasAlert := false, false
		for _, p := range both.snapshot() {
			hasRaw = hasRaw || p.DeviceID == "d2"
			hasAlert = hasAlert || p.Key != "temp" && p.Key != "hum"
		}
		return len(rawD1.snapshot()) > 0 && hasRaw && hasAlert
	})
	for _, p := range rawD1.snapshot() {
		if p.DeviceID != "d1" {
			t.Fatalf("raw_d1 只应收到 d1 的数据: %+v", p)
		}
	}
	for _, p := range rulesOnly.snapshot() {
		if p.Key != "temp_f" {
			t.Fatalf("rules_only 不应收到原始数据: %+v", p)
		}
	}
	found := false
	for _, st := range m.GetSinkDispatchStats() {
		found = found || strings.HasSuffix(st.Name, "(订阅)")
	}
	if !found {
		t.Fatal("分发统计应包含订阅的分发器")
	}

	// 停止连接器后取消订阅；重新启动后恢复
	if err := m.StopPlugin("rules_only"); err != nil {
		t.Fatal(err)
	}
	m.dispMu.RLock()
	_, still := m.feeds["rules_only"]
	m.dispMu.RUnlock()
	if still {
		t.Fatal("停止连接器后应取消订阅")
	}
	if err := m.StartPlugin("rules_only"); err != nil {
		t.Fatal(err)
	}
	restarted := recordingByName(t, m, "rules_only")
	waitFor(t, "重启后订阅恢复", func() bool {
		_ = nc.Publish("iot.rules.transform.d1.temp_f", []byte(ruleOut))
		return len(restarted.snapshot()) > 0
	})
}

// 没有 NATS 连接时无法订阅
func TestSubscriptionFeedWithoutBus(t *testing.T) {
	m := NewManager(t.TempDir(), nil, viper.New())
	if err := m.startSubscriptionFeed("x", &recordingSink{}, []sinkSubscription{{Subject: "a.b", DataType: "raw"}}); err == nil {
		t.Fatal("没有 NATS 连接应报错")
	}
	m.removeFeed("nope") // 不存在时不报错
}
