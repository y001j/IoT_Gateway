package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

// 以前规则管理器发出 "update"/"delete" 事件，服务只处理 "created"/"updated"/"deleted"，
// 数据分发使用的规则索引从不更新：热加载（包括 Web 端修改规则文件）后，
// 新规则不触发、修改不生效、删除的规则继续执行，必须重启网关。
func TestHotReloadUpdatesDispatchIndex(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	alerts, _ := nc.SubscribeSync("iot.alerts.triggered")
	nc.Flush()

	s := startService(t, nc, []*Rule{}, map[string]interface{}{
		"hot_reload": map[string]interface{}{"enabled": true, "debounce_delay": "20ms"},
	})
	dir := s.config.RulesDir
	// 文件监控器由 Start 之后的协程异步启动，等它就绪再修改文件
	m := s.manager.(*Manager)
	testutil.WaitFor(t, 3*time.Second, func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.hotReloadEnabled
	}, "规则文件监控器未启动")

	writeRule := func(threshold float64, enabled bool) {
		t.Helper()
		rule := Rule{ID: "hot", Name: "hot", Enabled: enabled,
			Conditions: &Condition{And: []*Condition{{Field: "key", Value: "temp"}, {Field: "value", Operator: "gt", Value: threshold}}},
			Actions:    []Action{{Type: "alert", Config: map[string]interface{}{"level": "warning", "message": "hot", "throttle": "0s"}}},
		}
		data, _ := json.Marshal(rule)
		if err := os.WriteFile(filepath.Join(dir, "hot.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// 等待分发索引反映出变更：threshold < 0 表示规则应不在索引中
	waitIndex := func(threshold float64) {
		t.Helper()
		testutil.WaitFor(t, 10*time.Second, func() bool {
			got := s.ruleIndex.Match(pt("probe", "temp", 0.0, model.TypeFloat))
			if threshold < 0 {
				return len(got) == 0
			}
			if len(got) != 1 || got[0].Conditions == nil || len(got[0].Conditions.And) != 2 {
				return false
			}
			v, _ := got[0].Conditions.And[1].Value.(float64)
			return v == threshold
		}, "规则变更未进入分发索引")
	}
	// 发布数据并检查是否产生告警
	expectAlert := func(device string, value float64, want bool) {
		t.Helper()
		drain(alerts)
		wait := 5 * time.Second
		if !want {
			wait = 500 * time.Millisecond // 索引状态已确认，观察一小段时间即可
		}
		deadline := time.Now().Add(wait)
		for time.Now().Before(deadline) {
			publishPoint(t, nc, pt(device, "temp", value, model.TypeFloat))
			if _, err := alerts.NextMsg(100 * time.Millisecond); err == nil {
				if !want {
					t.Fatalf("%s=%v 不应产生告警", device, value)
				}
				return
			}
		}
		if want {
			t.Fatalf("%s=%v 应产生告警（规则变更未进入分发索引）", device, value)
		}
	}

	writeRule(40, true) // 新增
	waitIndex(40)
	expectAlert("d1", 50, true)

	writeRule(60, true) // 修改阈值：50 不再触发，70 触发
	waitIndex(60)
	expectAlert("d2", 50, false)
	expectAlert("d3", 70, true)

	writeRule(60, false) // 禁用
	waitIndex(-1)
	expectAlert("d4", 70, false)

	writeRule(60, true)
	waitIndex(60)
	expectAlert("d5", 70, true)
	if err := os.Remove(filepath.Join(dir, "hot.json")); err != nil { // 删除
		t.Fatal(err)
	}
	waitIndex(-1)
	expectAlert("d6", 70, false)
}

func drain(sub *nats.Subscription) {
	for {
		if _, err := sub.NextMsg(10 * time.Millisecond); err != nil {
			return
		}
	}
}

func ruleJSON(id string, threshold float64) map[string]interface{} {
	return map[string]interface{}{"id": id, "name": id, "enabled": true,
		"conditions": map[string]interface{}{"field": "value", "operator": "gt", "value": threshold},
		"actions":    []map[string]interface{}{{"type": "alert", "config": map[string]interface{}{"message": id}}}}
}

func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, _ := json.Marshal(v)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func drainEvents(m *Manager) map[string]string {
	got := map[string]string{}
	for {
		select {
		case ev := <-m.changesChan:
			got[ev.Rule.ID] = ev.Type
		default:
			return got
		}
	}
}

func TestManagerTracksRulesPerFile(t *testing.T) {
	dir := t.TempDir()
	multi := filepath.Join(dir, "multi.json")
	other := filepath.Join(dir, "named-differently.json")
	writeJSON(t, multi, []interface{}{ruleJSON("r1", 1), ruleJSON("r2", 2)})
	writeJSON(t, other, ruleJSON("x", 3))
	m := NewManager(dir)
	if err := m.LoadRules(); err != nil {
		t.Fatal(err)
	}
	if len(m.ListRules()) != 3 {
		t.Fatalf("应加载 3 条规则: %d", len(m.ListRules()))
	}

	// 从多规则文件中删掉 r2：r2 应被移除并发出删除事件，r1 更新
	writeJSON(t, multi, []interface{}{ruleJSON("r1", 10)})
	m.handleFileUpdate(multi)
	if _, err := m.GetRule("r2"); err == nil {
		t.Fatal("从文件中删除的规则应被移除")
	}
	if r, _ := m.GetRule("r1"); r.Conditions.Value.(float64) != 10 {
		t.Fatalf("r1 应更新: %+v", r.Conditions)
	}
	if ev := drainEvents(m); ev["r2"] != RuleEventDelete || ev["r1"] != RuleEventUpdate {
		t.Fatalf("事件 = %v", ev)
	}
	if got := m.ruleIndex.Match(pt("d", "k", 100.0, model.TypeFloat)); len(got) != 2 {
		t.Fatalf("索引中应剩 r1、x: %d", len(got))
	}

	// 校验失败的新版本不替换旧版本，也不会删除规则
	bad := ruleJSON("r1", 20)
	bad["actions"] = []interface{}{}
	writeJSON(t, multi, []interface{}{bad})
	m.handleFileUpdate(multi)
	if r, err := m.GetRule("r1"); err != nil || r.Conditions.Value.(float64) != 10 {
		t.Fatalf("校验失败时应保留旧版本: %v %+v", err, r)
	}
	// 解析失败同样保留
	if err := os.WriteFile(multi, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.handleFileUpdate(multi)
	if _, err := m.GetRule("r1"); err != nil {
		t.Fatal("解析失败时应保留原有规则")
	}

	// 空数组合法：移除文件中的全部规则
	writeJSON(t, multi, []interface{}{})
	m.handleFileUpdate(multi)
	if _, err := m.GetRule("r1"); err == nil {
		t.Fatal("空规则文件应移除其中的规则")
	}

	// 删除文件名与规则 ID 不同的文件
	drainEvents(m)
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	m.handleFileDelete(other)
	if _, err := m.GetRule("x"); err == nil {
		t.Fatal("删除文件应移除其中定义的规则")
	}
	if ev := drainEvents(m); ev["x"] != RuleEventDelete {
		t.Fatalf("事件 = %v", ev)
	}
	if len(m.ListRules()) != 0 {
		t.Fatalf("应无剩余规则: %d", len(m.ListRules()))
	}
}

// SaveRule 把规则写到独立文件后，原多规则文件被改写时不应误删该规则
func TestManagerSaveRuleMovesOwnership(t *testing.T) {
	dir := t.TempDir()
	multi := filepath.Join(dir, "multi.json")
	writeJSON(t, multi, []interface{}{ruleJSON("r1", 1), ruleJSON("r2", 2)})
	m := NewManager(dir)
	if err := m.LoadRules(); err != nil {
		t.Fatal(err)
	}
	r1, _ := m.GetRule("r1")
	updated := *r1
	updated.Name = "改名"
	if err := m.SaveRule(&updated); err != nil {
		t.Fatal(err)
	}
	// SaveRule 会把 r1 从 multi.json 中清理掉，随后触发 multi.json 的更新事件
	m.handleFileUpdate(multi)
	if r, err := m.GetRule("r1"); err != nil || r.Name != "改名" {
		t.Fatalf("r1 已归属独立文件，不应被移除: %v %+v", err, r)
	}
	if _, err := m.GetRule("r2"); err != nil {
		t.Fatal("r2 应保留")
	}
	// 删除独立文件后移除 r1；之后 DeleteRule 报不存在
	m.handleFileDelete(filepath.Join(dir, "r1.json"))
	if _, err := m.GetRule("r1"); err == nil {
		t.Fatal("删除独立文件应移除 r1")
	}
	if err := m.DeleteRule("r2"); err != nil {
		t.Fatal(err)
	}
	// 未记录的文件按文件名推断（例如加载失败后被删除的文件）
	m.handleFileDelete(filepath.Join(dir, "unknown.json"))
}
