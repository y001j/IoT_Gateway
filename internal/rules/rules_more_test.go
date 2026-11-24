package rules

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func TestExpressionPreprocessingRespectsStrings(t *testing.T) {
	p := pt("dev-7", "temp", 1.0, model.TypeFloat)
	p.AddTag("zone", "A")
	cases := map[string]bool{
		"tags['zone'] == 'A'":              true,
		`tags["zone"] == "A"`:              true,
		"tags.zone == 'A'":                 true,
		"regex('^dev-[0-9]+$', device_id)": true, // 字符串中的方括号不能被当作数组字面量
		"'tags.zone' == 'tags.zone'":       true, // 字符串中的 tags.x 不能被改写
		`contains("[1,2]", "[1")`:          true,
	}
	for expr, want := range cases {
		if got := evalExpr(t, expr, p); got != want {
			t.Errorf("%s = %v, want %v", expr, got, want)
		}
	}
	mask := stringLiteralMask(`a "b\"c" d`)
	if mask[0] || !mask[3] || !mask[5] || mask[len(mask)-1] {
		t.Errorf("字符串掩码错误: %v", mask)
	}
}

func TestBuiltinAlertHandlerThrottleTemplateAndWebhook(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	var natsAlerts int32
	nc.Subscribe("iot.alerts.triggered.critical", func(*nats.Msg) { atomic.AddInt32(&natsAlerts, 1) })
	nc.Flush()

	var hookBody atomic.Value
	var hookAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		hookBody.Store(string(b))
		hookAuth.Store(r.Header.Get("Authorization"))
	}))
	defer srv.Close()

	h := &BuiltinAlertHandler{natsConn: nc, throttleMap: map[string]time.Time{}}
	rule := &Rule{ID: "r1", Name: "高温"}
	cfg := map[string]interface{}{
		"level":    "critical",
		"message":  "设备 {{.DeviceID}} 的 {{.Key}} = {{.Value}}",
		"throttle": "1h",
		"channels": []interface{}{map[string]interface{}{"type": "webhook", "config": map[string]interface{}{"url": srv.URL, "token": "tok"}}},
	}
	p := pt("dev-1", "temp", 88.5, model.TypeFloat)

	r, err := h.Execute(context.Background(), p, rule, cfg)
	if err != nil || !r.Success {
		t.Fatalf("告警执行失败: %v %+v", err, r)
	}
	testutil.WaitFor(t, 2*time.Second, func() bool { return hookBody.Load() != nil }, "webhook 应收到告警")
	var payload map[string]interface{}
	json.Unmarshal([]byte(hookBody.Load().(string)), &payload)
	msg, _ := payload["message"].(string)
	if msg == "" {
		if a, ok := payload["alert"].(map[string]interface{}); ok {
			msg, _ = a["message"].(string)
		}
	}
	if msg != "设备 dev-1 的 temp = 88.5" {
		t.Fatalf("消息模板渲染错误: %q (payload=%v)", msg, payload)
	}
	if auth, _ := hookAuth.Load().(string); auth != "Bearer tok" {
		t.Fatalf("webhook 认证头 = %q", auth)
	}
	testutil.WaitFor(t, 2*time.Second, func() bool { return atomic.LoadInt32(&natsAlerts) == 1 }, "应发布到 iot.alerts.triggered.critical")

	// 节流期内同一设备同一规则不再告警
	r2, _ := h.Execute(context.Background(), p, rule, cfg)
	out, _ := r2.Output.(map[string]interface{})
	if out["throttled"] != true {
		t.Fatalf("节流期内应被抑制: %+v", r2)
	}
}

func TestRuleManagerCRUDAndFiles(t *testing.T) {
	dir := t.TempDir()
	// JSON 数组文件 + YAML 文件 + 非法文件
	arr := []map[string]interface{}{
		{"id": "a", "name": "A", "enabled": true, "conditions": map[string]interface{}{"field": "value", "operator": "gt", "value": 1}, "actions": []interface{}{map[string]interface{}{"type": "alert"}}},
		{"id": "b", "name": "B", "enabled": false, "conditions": map[string]interface{}{"field": "value", "operator": "lt", "value": 0}, "actions": []interface{}{map[string]interface{}{"type": "alert"}}},
	}
	data, _ := json.Marshal(arr)
	os.WriteFile(filepath.Join(dir, "many.json"), data, 0o644)
	os.WriteFile(filepath.Join(dir, "c.yaml"), []byte("id: c\nname: C\nenabled: true\nconditions:\n  field: key\n  value: temp\nactions:\n  - type: forward\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o644)
	os.WriteFile(filepath.Join(dir, "invalid.json"), []byte(`{"id":"x","name":"X"}`), 0o644) // 缺少条件和动作

	m := NewManager(dir)
	if err := m.LoadRules(); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if n := len(m.ListRules()); n != 3 {
		t.Fatalf("应加载 3 条合法规则（跳过损坏和不合法的文件）, got %d", n)
	}
	if n := len(m.GetEnabledRules()); n != 2 {
		t.Fatalf("启用的规则 = %d", n)
	}

	if err := m.EnableRule("b"); err != nil {
		t.Fatal(err)
	}
	if r, _ := m.GetRule("b"); !r.Enabled {
		t.Fatalf("启用失败")
	}
	if err := m.DisableRule("a"); err != nil {
		t.Fatal(err)
	}
	newRule := &Rule{ID: "d", Name: "D", Enabled: true, Conditions: &Condition{Field: "value", Operator: "gt", Value: 5.0}, Actions: []Action{{Type: "alert"}}}
	if err := m.SaveRule(newRule); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "d.json")); err != nil {
		t.Fatalf("新规则应保存为文件: %v", err)
	}
	if err := m.SaveRule(&Rule{ID: "bad"}); err == nil {
		t.Fatalf("不合法的规则应保存失败")
	}
	if err := m.DeleteRule("d"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetRule("d"); err == nil {
		t.Fatalf("删除后不应存在")
	}
	if err := m.DeleteRule("nope"); err == nil {
		t.Fatalf("删除不存在的规则应报错")
	}

	// 重新加载：启用/禁用状态已持久化
	m2 := NewManager(dir)
	m2.LoadRules()
	defer m2.Close()
	if r, err := m2.GetRule("a"); err != nil || r.Enabled {
		t.Fatalf("禁用状态应持久化: %v %+v", err, r)
	}
	if r, err := m2.GetRule("b"); err != nil || !r.Enabled {
		t.Fatalf("启用状态应持久化: %v %+v", err, r)
	}
	// 删除多规则文件中定义的规则后，重启不应"复活"
	if err := m2.DeleteRule("c"); err != nil {
		t.Fatal(err)
	}
	if err := m2.SaveRule(&Rule{ID: "e", Name: "E", Enabled: true, Conditions: &Condition{Field: "value", Operator: "gt", Value: 1.0}, Actions: []Action{{Type: "alert"}}}); err != nil {
		t.Fatal(err)
	}
	m3 := NewManager(dir)
	m3.LoadRules()
	defer m3.Close()
	if _, err := m3.GetRule("e"); err != nil {
		t.Fatalf("新保存的规则重启后应存在")
	}
	if _, err := m3.GetRule("c"); err == nil {
		t.Fatalf("已删除的 YAML 规则重启后不应复活")
	}
	matched, errs, err := m3.EvaluateEnabledRules(pt("d", "temp", 10.0, model.TypeFloat))
	if err != nil || len(errs) != 0 || !matched["e"] {
		t.Fatalf("EvaluateEnabledRules = %v %v %v", matched, errs, err)
	}
	if st := m3.GetStats(); st["total_rules"] == nil {
		t.Fatalf("GetStats 缺少字段: %v", st)
	}
}

func TestRuleManagerHotReload(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.SetHotReloadConfig(&HotReloadConfig{Enabled: true, DebounceDelay: "10ms"})
	m.LoadRules()
	defer m.Close()
	ch, err := m.WatchChanges()
	if err != nil {
		t.Fatal(err)
	}
	rule := map[string]interface{}{"id": "hot", "name": "Hot", "enabled": true,
		"conditions": map[string]interface{}{"field": "value", "operator": "gt", "value": 1},
		"actions":    []interface{}{map[string]interface{}{"type": "alert"}}}
	data, _ := json.Marshal(rule)
	os.WriteFile(filepath.Join(dir, "hot.json"), data, 0o644)
	select {
	case ev := <-ch:
		if ev.Rule == nil || ev.Rule.ID != "hot" {
			t.Fatalf("事件内容错误: %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("新增规则文件后应收到变更事件")
	}
	if _, err := m.GetRule("hot"); err != nil {
		t.Fatalf("热加载后应能获取规则: %v", err)
	}
}
