package rules

import (
	"testing"

	"github.com/y001j/iot-gateway/internal/model"
)

func ids(rules []*Rule) []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.ID
	}
	return out
}

func hasID(rules []*Rule, id string) bool {
	for _, r := range rules {
		if r.ID == id {
			return true
		}
	}
	return false
}

func TestIndexIntersectsConstraints(t *testing.T) {
	idx := NewIndex()
	tempRule := &Rule{ID: "temp", Enabled: true, Conditions: &Condition{Field: "key", Value: "temperature"}}
	devRule := &Rule{ID: "dev1", Enabled: true, Conditions: &Condition{Field: "device_id", Value: "dev-1"}}
	both := &Rule{ID: "both", Enabled: true, Conditions: &Condition{And: []*Condition{
		{Field: "device_id", Value: "dev-1"},
		{Field: "key", Value: "humidity"},
	}}}
	anyRule := &Rule{ID: "any", Enabled: true, Conditions: &Condition{Field: "value", Operator: "gt", Value: 1}}
	typeRule := &Rule{ID: "bool", Enabled: true, Conditions: &Condition{Field: "type", Value: "bool"}}
	for _, r := range []*Rule{tempRule, devRule, both, anyRule, typeRule} {
		idx.AddRule(r)
	}

	got := idx.Match(model.Point{DeviceID: "dev-2", Key: "temperature", Type: model.TypeFloat})
	// 以前取并集：dev1/both 也会因为设备通配或 key 通配被选中
	if !hasID(got, "temp") || !hasID(got, "any") || hasID(got, "dev1") || hasID(got, "both") || hasID(got, "bool") {
		t.Fatalf("dev-2/temperature 候选规则 = %v", ids(got))
	}

	got = idx.Match(model.Point{DeviceID: "dev-1", Key: "humidity", Type: model.TypeFloat})
	if !hasID(got, "dev1") || !hasID(got, "both") || !hasID(got, "any") || hasID(got, "temp") {
		t.Fatalf("dev-1/humidity 候选规则 = %v", ids(got))
	}

	got = idx.Match(model.Point{DeviceID: "x", Key: "y", Type: model.TypeBool})
	if !hasID(got, "bool") || !hasID(got, "any") || len(got) != 2 {
		t.Fatalf("类型约束候选规则 = %v", ids(got))
	}

	// 没有任何规则匹配时返回空，而不是返回全部规则
	idx2 := NewIndex()
	idx2.AddRule(tempRule)
	if got := idx2.Match(model.Point{DeviceID: "d", Key: "other"}); len(got) != 0 {
		t.Fatalf("不匹配时应返回空, got %v", ids(got))
	}
}

func TestIndexIgnoresNonNecessaryConstraints(t *testing.T) {
	idx := NewIndex()
	// OR 分支中的等值条件不是必然成立的约束：key=humidity 且 value>5 的点也能命中
	orRule := &Rule{ID: "or", Enabled: true, Conditions: &Condition{Or: []*Condition{
		{Field: "key", Value: "temperature"},
		{Field: "value", Operator: "gt", Value: 5},
	}}}
	// NOT 中的等值条件表示"排除"，绝不能当作约束
	notRule := &Rule{ID: "not", Enabled: true, Conditions: &Condition{Not: &Condition{Field: "device_id", Value: "dev-1"}}}
	// 非 eq 操作符不构成约束
	neRule := &Rule{ID: "ne", Enabled: true, Conditions: &Condition{Field: "key", Operator: "ne", Value: "temperature"}}
	// 表达式条件不构成约束
	exprRule := &Rule{ID: "expr", Enabled: true, Conditions: &Condition{Type: "expression", Expression: "key == 'x'"}}
	// 通配值不构成约束
	starRule := &Rule{ID: "star", Enabled: true, Conditions: &Condition{Field: "device_id", Value: "*"}}
	for _, r := range []*Rule{orRule, notRule, neRule, exprRule, starRule} {
		idx.AddRule(r)
	}
	got := idx.Match(model.Point{DeviceID: "dev-2", Key: "humidity"})
	for _, id := range []string{"or", "not", "ne", "expr", "star"} {
		if !hasID(got, id) {
			t.Errorf("规则 %s 不应被索引排除, got %v", id, ids(got))
		}
	}
}

func TestIndexPriorityUpdateRemove(t *testing.T) {
	idx := NewIndex()
	low := &Rule{ID: "low", Priority: 1, Enabled: true}
	high := &Rule{ID: "high", Priority: 10, Enabled: true, Conditions: &Condition{Field: "device_id", Value: "d"}}
	mid := &Rule{ID: "mid", Priority: 5, Enabled: true}
	idx.AddRule(low)
	idx.AddRule(high)
	idx.AddRule(mid)
	got := ids(idx.Match(model.Point{DeviceID: "d", Key: "k"}))
	if len(got) != 3 || got[0] != "high" || got[1] != "mid" || got[2] != "low" {
		t.Fatalf("应按优先级排序: %v", got)
	}

	// 更新规则：禁用后应从索引中移除
	disabled := &Rule{ID: "mid", Priority: 5, Enabled: false}
	idx.AddRule(disabled)
	if hasID(idx.Match(model.Point{DeviceID: "d"}), "mid") {
		t.Fatalf("禁用的规则应被移除")
	}
	idx.RemoveRule(high)
	if hasID(idx.GetAllRules(), "high") {
		t.Fatalf("删除的规则仍在索引中")
	}
	if len(idx.GetRulesByDevice("d")) != 1 || len(idx.GetRulesByKey("k")) != 1 {
		t.Fatalf("按设备/key 查询结果错误")
	}
	stats := idx.GetStats()
	if stats["total_rules"] != 1 {
		t.Fatalf("统计错误: %v", stats)
	}
	idx.Clear()
	if len(idx.GetAllRules()) != 0 {
		t.Fatalf("Clear 后应为空")
	}
	idx.AddRule(nil)
	idx.RemoveRule(nil)
}
