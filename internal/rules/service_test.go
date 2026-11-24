package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

// ---------- 工作池 ----------

func TestWorkerPoolPreservesPerSeriesOrder(t *testing.T) {
	var mu sync.Mutex
	seen := map[string][]int{}
	pool := NewOptimizedWorkerPool(WorkerPoolConfig{NumWorkers: 8, QueueSize: 8000}, nil)
	pool.process = func(task RuleTask) error {
		mu.Lock()
		k := task.Point.DeviceID + "/" + task.Point.Key
		seen[k] = append(seen[k], task.Point.Value.(int))
		mu.Unlock()
		return nil
	}
	pool.Start()
	rule := &Rule{ID: "r"}
	for i := 0; i < 500; i++ {
		for d := 0; d < 4; d++ {
			if !pool.SubmitTask(RuleTask{Rule: rule, Point: model.Point{DeviceID: fmt.Sprintf("d%d", d), Key: "k", Value: i}}) {
				t.Fatalf("提交失败")
			}
		}
	}
	pool.Stop()
	for k, vals := range seen {
		if len(vals) != 500 {
			t.Fatalf("%s 处理了 %d 个任务", k, len(vals))
		}
		for i := 1; i < len(vals); i++ {
			if vals[i] < vals[i-1] {
				t.Fatalf("%s 的处理顺序被打乱: ...%d,%d...", k, vals[i-1], vals[i])
			}
		}
	}
	st := pool.GetStats()
	if st.CompletedTasks != 2000 || st.DroppedTasks != 0 || st.SuccessRate != 1 {
		t.Fatalf("统计错误: %+v", st)
	}
}

func TestWorkerPoolDropsAndCountsWhenFull(t *testing.T) {
	block := make(chan struct{})
	pool := NewOptimizedWorkerPool(WorkerPoolConfig{NumWorkers: 1, QueueSize: 16, SubmitTimeout: 5 * time.Millisecond}, nil)
	pool.process = func(RuleTask) error { <-block; return nil }
	pool.Start()

	accepted, dropped := 0, 0
	for i := 0; i < 40; i++ {
		if pool.SubmitTask(RuleTask{Rule: &Rule{ID: "r"}, Point: model.Point{DeviceID: "d", Key: "k"}}) {
			accepted++
		} else {
			dropped++
		}
	}
	if dropped == 0 {
		t.Fatalf("队列满时应丢弃任务")
	}
	if pool.GetStats().DroppedTasks != int64(dropped) {
		t.Fatalf("丢弃计数 = %d, 实际丢弃 %d", pool.GetStats().DroppedTasks, dropped)
	}
	close(block)
	pool.Stop()
	if got := pool.GetStats().CompletedTasks; got != int64(accepted) {
		t.Fatalf("已接收的任务都应在停止前处理完: completed=%d accepted=%d", got, accepted)
	}
	if pool.SubmitTask(RuleTask{}) {
		t.Fatalf("停止后不应接收任务")
	}
}

func TestWorkerPoolRecoversPanicAndCountsFailures(t *testing.T) {
	pool := NewOptimizedWorkerPool(WorkerPoolConfig{NumWorkers: 2}, nil)
	var n int32
	pool.process = func(task RuleTask) error {
		switch atomic.AddInt32(&n, 1) {
		case 1:
			panic("boom")
		case 2:
			return errors.New("fail")
		}
		return nil
	}
	pool.Start()
	for i := 0; i < 3; i++ {
		pool.SubmitTask(RuleTask{Point: model.Point{DeviceID: "d"}})
	}
	pool.Stop()
	st := pool.GetStats()
	if st.FailedTasks != 2 || st.CompletedTasks != 1 {
		t.Fatalf("统计错误: %+v", st)
	}
}

// ---------- 动作管道 ----------

type recordingHandler struct {
	name   string
	mu     sync.Mutex
	inputs []model.Point
	result func(model.Point) (*ActionResult, error)
}

func (h *recordingHandler) Name() string { return h.name }
func (h *recordingHandler) Execute(_ context.Context, p model.Point, _ *Rule, _ map[string]interface{}) (*ActionResult, error) {
	h.mu.Lock()
	h.inputs = append(h.inputs, p)
	h.mu.Unlock()
	if h.result != nil {
		return h.result(p)
	}
	return &ActionResult{Type: h.name, Success: true}, nil
}
func (h *recordingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.inputs)
}

func newTestService() *RuleEngineService {
	s := NewRuleEngineService()
	s.evaluator = NewEvaluator()
	s.config = &RuleEngineConfig{Subject: "iot.data.>"}
	return s
}

func TestFilterActionStopsPipeline(t *testing.T) {
	s := newTestService()
	filter := &recordingHandler{name: "filter", result: func(p model.Point) (*ActionResult, error) {
		return &ActionResult{Success: true, Output: map[string]interface{}{"filtered": p.Value.(float64) > 100}}, nil
	}}
	after := &recordingHandler{name: "after"}
	s.RegisterActionHandler("filter", filter)
	s.RegisterActionHandler("after", after)
	rule := &Rule{ID: "r", Enabled: true, Actions: []Action{{Type: "filter"}, {Type: "after"}}}

	s.processRule(rule, pt("d", "k", 500.0, model.TypeFloat))
	if after.count() != 0 {
		t.Fatalf("被过滤的数据不应继续执行后续动作")
	}
	s.processRule(rule, pt("d", "k", 50.0, model.TypeFloat))
	if after.count() != 1 {
		t.Fatalf("未被过滤的数据应继续执行后续动作")
	}
}

func TestTransformOutputFeedsNextAction(t *testing.T) {
	s := newTestService()
	transform := &recordingHandler{name: "transform", result: func(p model.Point) (*ActionResult, error) {
		out := p
		out.Value = p.Value.(float64) * 10
		return &ActionResult{Success: true, Output: map[string]interface{}{"transformed_point": out}}, nil
	}}
	next := &recordingHandler{name: "next"}
	s.RegisterActionHandler("transform", transform)
	s.RegisterActionHandler("next", next)
	rule := &Rule{ID: "r", Enabled: true, Actions: []Action{{Type: "transform"}, {Type: "next"}}}

	s.processRule(rule, pt("d", "k", 2.5, model.TypeFloat))
	if next.count() != 1 || next.inputs[0].Value.(float64) != 25 {
		t.Fatalf("后续动作应收到转换后的数据, got %+v", next.inputs)
	}
}

func TestFailedActionDoesNotStopLaterActionsAndMetricsCountedOnce(t *testing.T) {
	s := newTestService()
	failing := &recordingHandler{name: "bad", result: func(model.Point) (*ActionResult, error) {
		return &ActionResult{Success: false, Error: "boom"}, nil
	}}
	alert := &recordingHandler{name: "alert2"}
	s.RegisterActionHandler("bad", failing)
	s.RegisterActionHandler("alert2", alert)
	rule := &Rule{ID: "r", Enabled: true, Actions: []Action{{Type: "bad"}, {Type: "alert2"}}}

	if err := s.processRule(rule, pt("d", "k", 1.0, model.TypeFloat)); err != nil {
		t.Fatal(err)
	}
	if alert.count() != 1 {
		t.Fatalf("前一个动作失败后，后续动作仍应执行")
	}
	m := s.monitor.GetMetrics()
	// 以前每个动作会被记录两次
	if m.ActionsExecuted != 2 || m.ActionsFailed != 1 || m.ActionsSucceeded != 1 {
		t.Fatalf("动作统计错误: executed=%d failed=%d succeeded=%d", m.ActionsExecuted, m.ActionsFailed, m.ActionsSucceeded)
	}
}

func TestDisabledRuleAndConditionMismatch(t *testing.T) {
	s := newTestService()
	h := &recordingHandler{name: "h"}
	s.RegisterActionHandler("h", h)
	s.processRule(&Rule{ID: "off", Enabled: false, Actions: []Action{{Type: "h"}}}, pt("d", "k", 1.0, model.TypeFloat))
	s.processRule(&Rule{ID: "nomatch", Enabled: true, Conditions: &Condition{Field: "key", Value: "other"}, Actions: []Action{{Type: "h"}}}, pt("d", "k", 1.0, model.TypeFloat))
	if h.count() != 0 {
		t.Fatalf("禁用或不匹配的规则不应执行动作")
	}
	if err := s.processRule(&Rule{ID: "bad", Enabled: true, Conditions: &Condition{Field: "nope", Value: 1}}, pt("d", "k", 1.0, model.TypeFloat)); err == nil {
		t.Fatalf("条件求值错误应返回")
	}
	if s.processRule(nil, model.Point{}) != nil {
		t.Fatalf("nil 规则应直接返回")
	}
}

func TestLegacyFallbacks(t *testing.T) {
	s := newTestService()
	if _, err := s.executeAction(&Action{Type: "filter"}, pt("d", "k", 1.0, model.TypeFloat), &Rule{ID: "r"}); err == nil {
		t.Fatalf("未注册 filter 处理器时应报错，而不是静默放行")
	}
	if _, err := s.executeAction(&Action{Type: "unknown"}, model.Point{}, &Rule{ID: "r"}); err == nil {
		t.Fatalf("未知动作类型应报错")
	}
	if _, err := s.executeAction(&Action{Type: "forward"}, model.Point{}, &Rule{ID: "r"}); err == nil {
		t.Fatalf("没有 NATS 连接时 forward 应报错")
	}
}

func TestToIntAndConfigParsing(t *testing.T) {
	for in, want := range map[interface{}]int{8: 8, int64(9): 9, 10.0: 10, "11": 11} {
		if got, ok := toInt(in); !ok || got != want {
			t.Errorf("toInt(%v) = %d,%v", in, got, ok)
		}
	}
	if _, ok := toInt([]int{}); ok {
		t.Errorf("非数字应返回 false")
	}
	s := NewRuleEngineServiceWithConfig(map[string]interface{}{
		"worker_pool": map[string]interface{}{"max_workers": 3.0, "queue_size": 64, "submit_timeout": "20ms"},
		"hot_reload":  map[string]interface{}{"enabled": true, "max_retries": 2.0, "debounce_delay": "50ms"},
	})
	if s.maxWorkers != 3 || s.queueSize != 64 || s.submitTimeout != 20*time.Millisecond {
		t.Fatalf("工作池配置解析错误: %d %d %v", s.maxWorkers, s.queueSize, s.submitTimeout)
	}
	if !s.config.HotReload.Enabled || s.config.HotReload.MaxRetries != 2 {
		t.Fatalf("热加载配置解析错误")
	}
	// Init 不应丢掉构造函数里解析的热加载配置
	if err := s.Init(map[string]interface{}{"enabled": true, "rules_dir": t.TempDir(), "publish_rule_events": true}); err != nil {
		t.Fatal(err)
	}
	if s.config.HotReload == nil || !s.publishRuleEvents {
		t.Fatalf("Init 后配置丢失")
	}
}

// ---------- 与 NATS 集成 ----------

type fakeRuntime struct{ nc *nats.Conn }

func (f fakeRuntime) GetBus() *nats.Conn { return f.nc }

func startService(t *testing.T, nc *nats.Conn, rules []*Rule, extra map[string]interface{}) *RuleEngineService {
	t.Helper()
	dir := t.TempDir()
	data, _ := json.Marshal(rules)
	if err := os.WriteFile(filepath.Join(dir, "rules.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]interface{}{
		"enabled":    true,
		"rules_dir":  dir,
		"subject":    "iot.data.>",
		"hot_reload": map[string]interface{}{"enabled": false},
	}
	for k, v := range extra {
		cfg[k] = v
	}
	s := NewRuleEngineServiceWithConfig(cfg)
	s.SetRuntime(fakeRuntime{nc})
	if err := s.Init(cfg); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Stop(context.Background()) })
	return s
}

func publishPoint(t *testing.T, nc *nats.Conn, p model.Point) {
	t.Helper()
	data, _ := json.Marshal(p)
	if err := nc.Publish(model.DataSubject(p.DeviceID, p.Key), data); err != nil {
		t.Fatal(err)
	}
	nc.Flush()
}

func TestServiceEndToEndAlert(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	alerts, _ := nc.SubscribeSync("iot.alerts.triggered")
	nc.Flush()

	rule := &Rule{ID: "high-temp", Name: "高温", Enabled: true,
		Conditions: &Condition{And: []*Condition{
			{Field: "key", Value: "temperature"},
			{Field: "value", Operator: "gt", Value: 40},
		}},
		Actions: []Action{{Type: "alert", Config: map[string]interface{}{"level": "warning", "message": "温度过高"}}},
	}
	startService(t, nc, []*Rule{rule}, nil)

	publishPoint(t, nc, pt("dev-1", "temperature", 30.0, model.TypeFloat))
	publishPoint(t, nc, pt("dev-1", "temperature", 45.0, model.TypeFloat))

	msg, err := alerts.NextMsg(3 * time.Second)
	if err != nil {
		t.Fatalf("应收到告警: %v", err)
	}
	var alert Alert
	if err := json.Unmarshal(msg.Data, &alert); err != nil || alert.RuleID != "high-temp" || alert.Level != "warning" {
		t.Fatalf("告警内容错误: %v %+v", err, alert)
	}
	if _, err := alerts.NextMsg(200 * time.Millisecond); err == nil {
		t.Fatalf("未超过阈值的点不应产生告警")
	}
}

func TestForwardDoesNotLoop(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	var forwarded int32
	nc.Subscribe("iot.rules.forward.>", func(*nats.Msg) { atomic.AddInt32(&forwarded, 1) })
	var dataMsgs int32
	nc.Subscribe("iot.data.>", func(*nats.Msg) { atomic.AddInt32(&dataMsgs, 1) })
	nc.Flush()

	// 规则 1：未配置 subject 的 forward（以前默认发到 iot.data.* 造成死循环）
	r1 := &Rule{ID: "fwd-default", Name: "f1", Enabled: true,
		Conditions: &Condition{Field: "key", Value: "temperature"},
		Actions:    []Action{{Type: "forward", Config: map[string]interface{}{}}}}
	// 规则 2：显式把数据转发回 iot.data（依靠消息头的环路保护）
	r2 := &Rule{ID: "fwd-back", Name: "f2", Enabled: true,
		Conditions: &Condition{Field: "key", Value: "temperature"},
		Actions:    []Action{{Type: "forward", Config: map[string]interface{}{"subject": "iot.data.copy.temperature"}}}}
	s := startService(t, nc, []*Rule{r1, r2}, nil)

	publishPoint(t, nc, pt("dev-1", "temperature", 1.0, model.TypeFloat))
	time.Sleep(500 * time.Millisecond)

	if n := atomic.LoadInt32(&forwarded); n != 1 {
		t.Fatalf("默认 forward 应只产生 1 条消息, got %d", n)
	}
	// iot.data 上：原始点 1 条 + 规则 2 转发回来的 1 条，不能无限增长
	if n := atomic.LoadInt32(&dataMsgs); n != 2 {
		t.Fatalf("iot.data 消息数 = %d，存在转发环路", n)
	}
	if s.GetLoopGuardSkipped() != 1 {
		t.Fatalf("环路保护应忽略 1 条规则输出, got %d", s.GetLoopGuardSkipped())
	}
}

func TestRuleEventsAreOptIn(t *testing.T) {
	run := func(publish bool) (evaluated, matched int32) {
		_, nc := testutil.StartNATS(t)
		nc.Subscribe("iot.rules.evaluated", func(*nats.Msg) { atomic.AddInt32(&evaluated, 1) })
		nc.Subscribe("iot.rules.matched", func(*nats.Msg) { atomic.AddInt32(&matched, 1) })
		nc.Flush()
		h := &recordingHandler{name: "noop"}
		rule := &Rule{ID: "r", Name: "r", Enabled: true,
			Conditions: &Condition{Field: "value", Operator: "gt", Value: 10},
			Actions:    []Action{{Type: "noop"}}}
		s := NewRuleEngineService()
		s.RegisterActionHandler("noop", h)
		dir := t.TempDir()
		data, _ := json.Marshal([]*Rule{rule})
		os.WriteFile(filepath.Join(dir, "r.json"), data, 0o644)
		cfg := map[string]interface{}{"enabled": true, "rules_dir": dir, "publish_rule_events": publish,
			"hot_reload": map[string]interface{}{"enabled": false}}
		s.SetRuntime(fakeRuntime{nc})
		s.Init(cfg)
		s.Start(context.Background())
		defer s.Stop(context.Background())
		for i := 0; i < 5; i++ {
			publishPoint(t, nc, pt("d", "k", float64(i*5), model.TypeFloat)) // 0,5,10,15,20 → 2 个命中
		}
		time.Sleep(400 * time.Millisecond)
		return atomic.LoadInt32(&evaluated), atomic.LoadInt32(&matched)
	}
	ev, m := run(false)
	if ev != 0 || m != 2 {
		t.Fatalf("默认只发布命中事件: evaluated=%d matched=%d", ev, m)
	}
	ev, m = run(true)
	if ev != 5 || m != 2 {
		t.Fatalf("开启后应发布评估事件: evaluated=%d matched=%d", ev, m)
	}
}

func TestPublishRuleOutputHeader(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	sub, _ := nc.SubscribeSync("x.y")
	nc.Flush()
	if err := PublishRuleOutput(nc, "x.y", []byte("1")); err != nil {
		t.Fatal(err)
	}
	msg, err := sub.NextMsg(time.Second)
	if err != nil || msg.Header.Get(RuleOutputHeader) != RuleOutputHeaderValue {
		t.Fatalf("规则输出应带环路保护消息头: %v", err)
	}
	if err := PublishRuleOutput(nil, "x", nil); err == nil {
		t.Fatalf("nil 连接应报错")
	}
}

func TestDispatchPointCountsDrops(t *testing.T) {
	s := newTestService()
	block := make(chan struct{})
	s.optimizedPool = NewOptimizedWorkerPool(WorkerPoolConfig{NumWorkers: 1, QueueSize: 16, SubmitTimeout: time.Millisecond}, nil)
	s.optimizedPool.process = func(RuleTask) error { <-block; return nil }
	s.optimizedPool.Start()
	s.ruleIndex.AddRule(&Rule{ID: "r", Enabled: true})
	for i := 0; i < 50; i++ {
		s.dispatchPoint(pt("d", "k", i, model.TypeInt))
	}
	if s.GetDroppedPoints() == 0 {
		t.Fatalf("过载时应统计丢弃数量")
	}
	if st := s.GetWorkerPoolStats(); st == nil || st.DroppedTasks != s.GetDroppedPoints() {
		t.Fatalf("工作池与服务的丢弃计数不一致")
	}
	close(block)
	s.optimizedPool.Stop()
}

func TestRegisterActionHandlerConcurrent(t *testing.T) {
	s := newTestService()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			s.RegisterActionHandler(fmt.Sprintf("h%d", i), &recordingHandler{name: "h"})
		}(i)
		go func() {
			defer wg.Done()
			s.getActionHandler("h1")
		}()
	}
	wg.Wait()
}

func TestAggregateResultForwardingBothFormats(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	sub, _ := nc.SubscribeSync("iot.rules.aggregate.>")
	nc.Flush()

	for _, format := range []string{"struct", "map"} {
		s := newTestService()
		s.bus = nc
		s.RegisterActionHandler("aggregate", &recordingHandler{name: "aggregate", result: func(p model.Point) (*ActionResult, error) {
			fns := map[string]float64{"max": 30, "avg": 20, "min": 10}
			var res interface{}
			if format == "struct" {
				m := map[string]interface{}{}
				for k, v := range fns {
					m[k] = v
				}
				res = &AggregateResult{DeviceID: p.DeviceID, Key: p.Key, Functions: m, Count: 3}
			} else {
				res = map[string]interface{}{"device_id": p.DeviceID, "key": p.Key, "functions": fns, "count": 3}
			}
			return &ActionResult{Success: true, Output: map[string]interface{}{"aggregated": true, "aggregate_result": res}}, nil
		}})
		rule := &Rule{ID: "agg", Enabled: true, Actions: []Action{{Type: "aggregate", Config: map[string]interface{}{
			"functions": []interface{}{"max", "avg"},
			"output":    map[string]interface{}{"key_template": "{{.Key}}_stat", "forward": true},
		}}}}
		in := pt("dev-1", "temp", 1.0, model.TypeFloat)
		in.Timestamp = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := s.processRule(rule, in); err != nil {
			t.Fatal(err)
		}
		msg, err := sub.NextMsg(2 * time.Second)
		if err != nil {
			t.Fatalf("[%s] 聚合结果未转发: %v", format, err)
		}
		if msg.Subject != "iot.rules.aggregate.dev-1.temp_stat" || msg.Header.Get(RuleOutputHeader) != RuleOutputHeaderValue {
			t.Fatalf("[%s] 主题/消息头错误: %s %v", format, msg.Subject, msg.Header)
		}
		var out model.Point
		json.Unmarshal(msg.Data, &out)
		// 按 functions 配置顺序取第一个（max），而不是随机的 map 遍历顺序
		if out.Value.(float64) != 30 {
			t.Fatalf("[%s] 聚合值 = %v，应为 functions 配置中的第一个函数 max", format, out.Value)
		}
		if fn, _ := out.GetTag("function"); fn != "max" {
			t.Fatalf("[%s] function 标签 = %q", format, fn)
		}
		if !out.Timestamp.Equal(in.Timestamp) {
			t.Fatalf("[%s] 聚合结果应沿用数据点时间: %v", format, out.Timestamp)
		}
	}
}

func TestPrimaryAggregateValue(t *testing.T) {
	fns := map[string]interface{}{"min": 1.0, "zeta": 9.0}
	if n, v := primaryAggregateValue(fns, map[string]interface{}{"functions": "min"}); n != "min" || v != 1.0 {
		t.Fatalf("字符串配置: %s %v", n, v)
	}
	if n, _ := primaryAggregateValue(fns, nil); n != "min" {
		t.Fatalf("无配置时应按名称排序稳定选择: %s", n)
	}
	if n, v := primaryAggregateValue(nil, nil); n != "" || v != 0.0 {
		t.Fatalf("空结果: %s %v", n, v)
	}
}
