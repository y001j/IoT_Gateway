package northbound_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/testutil"
)

// captureSink 记录收到的数据点，注册为 "capture" 类型供 nats_subscriber 作为目标使用
type captureSink struct {
	*northbound.BaseSink
	mu     sync.Mutex
	points []model.Point
}

var (
	capMu     sync.Mutex
	captures  = map[string]*captureSink{}
	registerO sync.Once
)

func registerCapture() {
	registerO.Do(func() {
		northbound.Register("capture", func() northbound.Sink {
			return &captureSink{BaseSink: northbound.NewBaseSink("capture")}
		})
	})
}

func (c *captureSink) Init(cfg json.RawMessage) error {
	if _, err := c.ParseStandardConfig(cfg); err != nil {
		return err
	}
	capMu.Lock()
	captures[c.Name()] = c
	capMu.Unlock()
	return nil
}
func (c *captureSink) Start(context.Context) error { return nil }
func (c *captureSink) Stop() error                 { return nil }
func (c *captureSink) Publish(b []model.Point) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range b {
		b[i].AddTag("touched_by", c.Name()) // 模拟 Sink 修改数据
	}
	c.points = append(c.points, b...)
	return nil
}
func (c *captureSink) got() []model.Point {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]model.Point(nil), c.points...)
}

func getCapture(name string) *captureSink {
	capMu.Lock()
	defer capMu.Unlock()
	return captures[name]
}

func TestNATSSubscriberRoutesRuleEventsAndRuleOutputs(t *testing.T) {
	registerCapture()
	srv, nc := testutil.StartNATS(t)
	_ = srv
	s := northbound.NewNATSSubscriberSink()
	params := `{"flush_interval": 20000000, "subscriptions":[
		{"subject":"iot.rules.>","data_type":"rule","enabled":true,"tags":{"source":"rule_engine"}},
		{"subject":"iot.alerts.>","data_type":"alert","enabled":true},
		{"subject":"iot.system.>","data_type":"system","enabled":true},
		{"subject":"ignored.>","data_type":"raw","enabled":false}],
		"target_sinks":[{"name":"t1","type":"capture","params":{}},{"name":"t2","type":"capture","params":{}}]}`
	if err := s.Init(json.RawMessage(`{"name":"sub","type":"nats_subscriber","params":` + params + `}`)); err != nil {
		t.Fatal(err)
	}
	s.SetNATSConnection(nc)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	pub := func(subject string, v interface{}) {
		data, _ := json.Marshal(v)
		nc.Publish(subject, data)
	}
	// 规则事件格式（data_point 嵌套）
	pub("iot.rules.matched", map[string]interface{}{"event_type": "matched",
		"data_point": map[string]interface{}{"device_id": "d1", "key": "temp", "value": 41.0, "type": "float"}})
	// 规则输出格式（扁平数据点），以前会因为缺少 data_point 而解析失败
	pub("iot.rules.forward.d2.temp", map[string]interface{}{"device_id": "d2", "key": "temp", "value": 1.0, "type": "float",
		"timestamp": "2026-01-01T00:00:00Z"})
	pub("iot.alerts.triggered", map[string]interface{}{"id": "a1", "level": "warning", "message": "高温", "device_id": "d1"})
	pub("iot.system.event", map[string]interface{}{"event_type": "restart", "message": "ok"})
	pub("ignored.x", map[string]interface{}{"key": "x"})
	pub("iot.rules.bad", "not an object")
	nc.Flush()

	t1, t2 := getCapture("t1"), getCapture("t2")
	testutil.WaitFor(t, 3*time.Second, func() bool { return len(t1.got()) == 4 && len(t2.got()) == 4 }, "两个目标都应收到 4 个数据点")

	byKey := map[string]model.Point{}
	for _, p := range t1.got() {
		byKey[p.DeviceID+"/"+p.Key] = p
	}
	if p := byKey["d1/temp"]; p.Value != 41.0 {
		t.Fatalf("规则事件解析错误: %+v", p)
	}
	fwd := byKey["d2/temp"]
	if fwd.Value != 1.0 || !fwd.Timestamp.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("扁平规则输出解析错误: %+v", fwd)
	}
	if v, _ := fwd.GetTag("source"); v != "rule_engine" {
		t.Fatalf("订阅配置的标签未附加")
	}
	if p := byKey["d1/alert"]; p.Value != "高温" {
		t.Fatalf("告警解析错误: %+v", byKey)
	}
	if _, ok := byKey["system/restart"]; !ok {
		t.Fatalf("系统事件解析错误: %+v", byKey)
	}
	// 目标 Sink 之间不能互相污染
	for _, p := range t2.got() {
		if v, _ := p.GetTag("touched_by"); v != "t2" {
			t.Fatalf("t2 收到的数据被 t1 修改过: touched_by=%q", v)
		}
	}
}

func TestNATSSubscriberFilterRules(t *testing.T) {
	registerCapture()
	_, nc := testutil.StartNATS(t)
	s := northbound.NewNATSSubscriberSink()
	params := `{"flush_interval": 20000000,
		"subscriptions":[{"subject":"iot.data.>","data_type":"raw","enabled":true}],
		"filter_rules":[
			{"field":"type","operator":"eq","value":"float","action":"include"},
			{"field":"key","operator":"contains","value":"tem","action":"include"},
			{"field":"value","operator":"gt","value":10,"action":"include"},
			{"field":"device_id","operator":"regex","value":"^test-","action":"exclude"}],
		"target_sinks":[{"name":"f1","type":"capture","params":{}}]}`
	if err := s.Init(json.RawMessage(`{"name":"sub2","type":"nats_subscriber","params":` + params + `}`)); err != nil {
		t.Fatal(err)
	}
	s.SetNATSConnection(nc)
	s.Start(context.Background())
	defer s.Stop()

	send := func(p model.Point) {
		data, _ := json.Marshal(p)
		nc.Publish(model.DataSubject(p.DeviceID, p.Key), data)
	}
	send(model.NewPoint("temperature", "d1", 20.0, model.TypeFloat))     // 通过
	send(model.NewPoint("temperature", "d1", 5.0, model.TypeFloat))      // value 不大于 10
	send(model.NewPoint("humidity", "d1", 50.0, model.TypeFloat))        // key 不含 tem
	send(model.NewPoint("temperature", "d1", 20, model.TypeInt))         // 类型不是 float
	send(model.NewPoint("temperature", "test-1", 20.0, model.TypeFloat)) // 被排除
	nc.Flush()

	f1 := getCapture("f1")
	testutil.WaitFor(t, 2*time.Second, func() bool { return len(f1.got()) >= 1 }, "应有数据通过过滤")
	time.Sleep(150 * time.Millisecond)
	if got := f1.got(); len(got) != 1 || got[0].Value != 20.0 || got[0].DeviceID != "d1" {
		t.Fatalf("过滤结果错误: %+v", got)
	}
}

func TestNATSSubscriberInitErrorsAndDirectPublish(t *testing.T) {
	registerCapture()
	s := northbound.NewNATSSubscriberSink()
	if err := s.Init(json.RawMessage(`{"name":"x","params":{"target_sinks":[{"name":"n","type":"no-such-sink"}]}}`)); err == nil {
		t.Fatalf("未知的目标 Sink 类型应报错")
	}
	s2 := northbound.NewNATSSubscriberSink()
	if err := s2.Init(json.RawMessage(`{"name":"y","params":{"target_sinks":[{"name":"direct","type":"capture","params":{}}]}}`)); err != nil {
		t.Fatal(err)
	}
	if err := s2.Publish([]model.Point{model.NewPoint("k", "d", 1, model.TypeInt)}); err != nil {
		t.Fatal(err)
	}
	if len(getCapture("direct").got()) != 1 {
		t.Fatalf("直接 Publish 应转发给目标 Sink")
	}
	if s2.Healthy() == nil {
		t.Fatalf("未启动时应不健康")
	}
}
