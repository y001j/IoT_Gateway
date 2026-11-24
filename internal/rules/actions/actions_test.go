package actions

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/testutil"
)

var testRule = &rules.Rule{ID: "rule-1", Name: "test"}

func point(device, key string, v interface{}, typ model.DataType) model.Point {
	return model.NewPoint(key, device, v, typ)
}

func output(t *testing.T, r *rules.ActionResult) map[string]interface{} {
	t.Helper()
	m, ok := r.Output.(map[string]interface{})
	if !ok {
		t.Fatalf("输出类型错误: %T", r.Output)
	}
	return m
}

// ---------- filter ----------

func runFilter(t *testing.T, h *FilterHandler, p model.Point, cfg map[string]interface{}) bool {
	t.Helper()
	r, err := h.Execute(context.Background(), p, testRule, cfg)
	if err != nil || !r.Success {
		t.Fatalf("filter 执行失败: %v %+v", err, r)
	}
	return output(t, r)["filtered"].(bool)
}

func TestFilterRange(t *testing.T) {
	h := NewFilterHandler()
	cfg := map[string]interface{}{"type": "range", "parameters": map[string]interface{}{"min": 0.0, "max": 100.0}}
	if runFilter(t, h, point("d", "k", 50.0, model.TypeFloat), cfg) {
		t.Errorf("范围内的值不应被过滤")
	}
	if !runFilter(t, h, point("d", "k", 150.0, model.TypeFloat), cfg) {
		t.Errorf("超过最大值应被过滤")
	}
	if !runFilter(t, h, point("d", "k", -1, model.TypeInt), cfg) {
		t.Errorf("小于最小值应被过滤")
	}
	r, _ := h.Execute(context.Background(), point("d", "k", 1.0, model.TypeFloat), testRule,
		map[string]interface{}{"type": "range", "parameters": map[string]interface{}{}})
	if r.Success {
		t.Errorf("缺少 min/max 应返回失败结果")
	}
}

func TestFilterDuplicate(t *testing.T) {
	h := NewFilterHandler()
	cfg := map[string]interface{}{"type": "duplicate", "parameters": map[string]interface{}{"window": "1m"}}
	base := time.Now()
	p1 := point("d", "k", 1.0, model.TypeFloat)
	p1.Timestamp = base
	p2 := p1
	p2.Timestamp = base.Add(10 * time.Second)
	p3 := point("d", "k", 2.0, model.TypeFloat)
	p3.Timestamp = base.Add(11 * time.Second)
	p4 := p1
	p4.Timestamp = base.Add(2 * time.Minute)
	if runFilter(t, h, p1, cfg) {
		t.Errorf("首次出现不应被过滤")
	}
	if !runFilter(t, h, p2, cfg) {
		t.Errorf("窗口内的重复值应被过滤")
	}
	if runFilter(t, h, p3, cfg) {
		t.Errorf("不同的值不应被过滤")
	}
	if runFilter(t, h, p4, cfg) {
		t.Errorf("超出窗口的重复值不应被过滤")
	}
}

func TestFilterNullAndUnknownType(t *testing.T) {
	h := NewFilterHandler()
	if !runFilter(t, h, model.Point{DeviceID: "d", Key: "k", Value: nil}, map[string]interface{}{"type": "null"}) {
		t.Errorf("nil 值应被 null 过滤器过滤")
	}
	if runFilter(t, h, point("d", "k", 1, model.TypeInt), map[string]interface{}{"type": "null"}) {
		t.Errorf("非空值不应被过滤")
	}
	r, _ := h.Execute(context.Background(), point("d", "k", 1, model.TypeInt), testRule, map[string]interface{}{"type": "magic"})
	if r.Success {
		t.Errorf("未知过滤类型应返回失败结果")
	}
}

func TestFilterRateLimit(t *testing.T) {
	h := NewFilterHandler()
	cfg := map[string]interface{}{"type": "rate_limit", "parameters": map[string]interface{}{"max_rate": 1.0, "window": "1m"}}
	if runFilter(t, h, point("d", "k", 1, model.TypeInt), cfg) {
		t.Errorf("第一条不应被限速")
	}
	if !runFilter(t, h, point("d", "k", 2, model.TypeInt), cfg) {
		t.Errorf("紧接着的第二条应被限速")
	}
	if runFilter(t, h, point("other", "k", 1, model.TypeInt), cfg) {
		t.Errorf("限速按设备和 key 独立计算")
	}
}

// ---------- transform ----------

func runTransform(t *testing.T, p model.Point, cfg map[string]interface{}) model.Point {
	t.Helper()
	h := NewTransformHandler(nil)
	r, err := h.Execute(context.Background(), p, testRule, cfg)
	if err != nil || !r.Success {
		t.Fatalf("transform 失败: %v %+v", err, r)
	}
	return output(t, r)["transformed_point"].(model.Point)
}

func TestTransformTypes(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		cfg  map[string]interface{}
		want float64
	}{
		{"scale", 2.0, map[string]interface{}{"type": "scale", "parameters": map[string]interface{}{"factor": 10.0}}, 20},
		{"offset", 2.0, map[string]interface{}{"type": "offset", "parameters": map[string]interface{}{"offset": -5.0}}, -3},
		{"摄氏转华氏", 100.0, map[string]interface{}{"type": "unit_convert", "parameters": map[string]interface{}{"from": "C", "to": "F"}}, 212},
		{"round", 3.14159, map[string]interface{}{"type": "round", "parameters": map[string]interface{}{"decimals": 2.0}}, 3.14},
		{"clamp 上限", 150.0, map[string]interface{}{"type": "clamp", "parameters": map[string]interface{}{"min": 0.0, "max": 100.0}}, 100},
		{"clamp 下限", -5.0, map[string]interface{}{"type": "clamp", "parameters": map[string]interface{}{"min": 0.0, "max": 100.0}}, 0},
		{"expression", 4.0, map[string]interface{}{"type": "expression", "parameters": map[string]interface{}{"expression": "x * 2 + 1"}}, 9},
		{"precision", 1.23456, map[string]interface{}{"type": "scale", "precision": 1.0, "parameters": map[string]interface{}{"factor": 1.0}}, 1.2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runTransform(t, point("d", "k", c.in, model.TypeFloat), c.cfg)
			f, ok := got.Value.(float64)
			if !ok || math.Abs(f-c.want) > 1e-6 {
				t.Fatalf("got %v (%T), want %v", got.Value, got.Value, c.want)
			}
		})
	}
}

func TestTransformLookupOutputKeyTagsAndTimestamp(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in := point("d", "status", "1", model.TypeString)
	in.Timestamp = ts
	got := runTransform(t, in, map[string]interface{}{
		"type":       "lookup",
		"parameters": map[string]interface{}{"table": map[string]interface{}{"1": "running", "0": "stopped"}},
		"output_key": "status_text",
		"add_tags":   map[string]interface{}{"unit": "state"},
	})
	if got.Value != "running" || got.Key != "status_text" {
		t.Fatalf("lookup 结果错误: %+v", got)
	}
	if v, _ := got.GetTag("unit"); v != "state" {
		t.Fatalf("add_tags 未生效")
	}
	if !got.Timestamp.Equal(ts) {
		t.Fatalf("转换不应改变采集时间戳: %v", got.Timestamp)
	}
	if _, ok := in.GetTag("unit"); ok {
		t.Fatalf("转换时添加的标签泄漏到了原数据点（同一数据点会被多个规则共享）")
	}
}

func TestTransformErrorHandling(t *testing.T) {
	h := NewTransformHandler(nil)
	r, _ := h.Execute(context.Background(), point("d", "k", "abc", model.TypeString), testRule,
		map[string]interface{}{"type": "scale", "parameters": map[string]interface{}{"factor": 2.0}})
	if r.Success {
		t.Errorf("无法转换的值应返回失败")
	}
	r, _ = h.Execute(context.Background(), point("d", "k", "abc", model.TypeString), testRule,
		map[string]interface{}{"type": "scale", "error_action": "default", "default_value": 0.0, "parameters": map[string]interface{}{"factor": 2.0}})
	if !r.Success || output(t, r)["transformed_point"].(model.Point).Value != 0.0 {
		t.Errorf("error_action=default 应使用默认值")
	}
	r, _ = h.Execute(context.Background(), point("d", "k", 1.0, model.TypeFloat), testRule, map[string]interface{}{"type": "nope"})
	if r.Success {
		t.Errorf("未知转换类型应返回失败")
	}
}

func TestTransformPublishesWithLoopGuard(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	sub, _ := nc.SubscribeSync("custom.out")
	nc.Flush()
	h := NewTransformHandler(nc)
	r, err := h.Execute(context.Background(), point("d", "k", 2.0, model.TypeFloat), testRule, map[string]interface{}{
		"type": "scale", "publish_subject": "custom.out", "parameters": map[string]interface{}{"factor": 3.0},
	})
	if err != nil || !r.Success {
		t.Fatal(err)
	}
	msg, err := sub.NextMsg(time.Second)
	if err != nil {
		t.Fatalf("应发布转换结果: %v", err)
	}
	if msg.Header.Get(rules.RuleOutputHeader) != rules.RuleOutputHeaderValue {
		t.Fatalf("转换结果应带环路保护消息头")
	}
	var data map[string]interface{}
	json.Unmarshal(msg.Data, &data)
	if data["value"].(float64) != 6 {
		t.Fatalf("发布的值 = %v", data["value"])
	}
}

// ---------- forward ----------

func TestForwardDefaultSubject(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	sub, _ := nc.SubscribeSync(">")
	nc.Flush()
	h := NewForwardHandler(nc)
	r, err := h.Execute(context.Background(), point("plc.1", "temp", 1.0, model.TypeFloat), testRule, map[string]interface{}{})
	if err != nil || !r.Success {
		t.Fatalf("forward 失败: %v %+v", err, r)
	}
	msg, err := sub.NextMsg(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Subject != model.RuleOutputSubject("forward", "plc.1", "temp") {
		t.Fatalf("默认主题 = %q（不能再落在 iot.data 下）", msg.Subject)
	}
	if msg.Header.Get(rules.RuleOutputHeader) != rules.RuleOutputHeaderValue {
		t.Fatalf("应带环路保护消息头")
	}

	r, _ = NewForwardHandler(nil).Execute(context.Background(), point("d", "k", 1, model.TypeInt), testRule, nil)
	if r.Success {
		t.Fatalf("没有 NATS 连接时应返回失败")
	}
}

// ---------- aggregate ----------

func TestAggregateCountWindow(t *testing.T) {
	h := NewAggregateHandler()
	cfg := map[string]interface{}{"window_size": 3.0, "functions": []interface{}{"avg", "max", "min", "count"}}
	var last *rules.ActionResult
	for i, v := range []float64{10, 20, 30} {
		r, err := h.Execute(context.Background(), point("d", "k", v, model.TypeFloat), testRule, cfg)
		if err != nil || !r.Success {
			t.Fatalf("aggregate 失败: %v", err)
		}
		out := output(t, r)
		if i < 2 {
			if _, ok := out["aggregated"]; ok {
				t.Fatalf("窗口未满时不应输出结果")
			}
		}
		last = r
	}
	out := output(t, last)
	res, ok := out["aggregate_result"].(*rules.AggregateResult)
	if !ok || out["aggregated"] != true {
		t.Fatalf("窗口满时应输出聚合结果: %+v", out)
	}
	if f, _ := res.Functions["avg"].(float64); f != 20 {
		t.Fatalf("avg = %v", res.Functions["avg"])
	}
	if f, _ := res.Functions["max"].(float64); f != 30 {
		t.Fatalf("max = %v", res.Functions["max"])
	}
	if f, _ := res.Functions["min"].(float64); f != 10 {
		t.Fatalf("min = %v", res.Functions["min"])
	}
}

func TestAggregateValidation(t *testing.T) {
	h := NewAggregateHandler()
	bad := []map[string]interface{}{
		{"window_type": "time"},                  // 缺少 window
		{"window_type": "time", "window": "abc"}, // 非法时间
		{"window_type": "sliding-ish"},           // 未知窗口类型
		{"window_size": -1.0},                    // 负数
		{"window_size": 1e9},                     // 过大
	}
	for i, cfg := range bad {
		if err := h.Validate(cfg); err == nil {
			t.Errorf("用例 %d 应校验失败: %v", i, cfg)
		}
	}
	if err := h.Validate(map[string]interface{}{"window_type": "time", "window": "30s"}); err != nil {
		t.Errorf("合法配置校验失败: %v", err)
	}
	r, _ := h.Execute(context.Background(), point("d", "k", "text", model.TypeString), testRule, map[string]interface{}{"window_size": 2.0})
	if r == nil || r.Success {
		t.Errorf("非数值数据应返回失败")
	}
}

// ---------- 注册 ----------

func TestRegisterBuiltinHandlers(t *testing.T) {
	svc := rules.NewRuleEngineService()
	RegisterBuiltinHandlers(svc, nil)
	for _, typ := range []string{"filter", "transform", "forward", "aggregate"} {
		if _, ok := svc.GetActionHandler(typ); !ok {
			t.Errorf("动作处理器 %s 未注册", typ)
		}
	}
}
