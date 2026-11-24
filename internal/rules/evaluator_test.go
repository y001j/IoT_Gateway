package rules

import (
	"fmt"
	"sync"
	"testing"

	"github.com/y001j/iot-gateway/internal/model"
)

func pt(device, key string, value interface{}, typ model.DataType) model.Point {
	p := model.NewPoint(key, device, value, typ)
	return p
}

func TestEvaluatorSimpleOperators(t *testing.T) {
	e := NewEvaluator()
	p := pt("dev-1", "temperature", 42.5, model.TypeFloat)
	p.AddTag("location", "room-101")

	cases := []struct {
		name string
		cond *Condition
		want bool
	}{
		{"nil 条件恒为真", nil, true},
		{"eq 字符串", &Condition{Field: "key", Operator: "eq", Value: "temperature"}, true},
		{"默认操作符为 eq", &Condition{Field: "device_id", Value: "dev-1"}, true},
		{"ne", &Condition{Field: "device_id", Operator: "ne", Value: "dev-2"}, true},
		{"gt", &Condition{Field: "value", Operator: "gt", Value: 40}, true},
		{"gt 不满足", &Condition{Field: "value", Operator: "gt", Value: 50}, false},
		{"gte 边界", &Condition{Field: "value", Operator: "gte", Value: 42.5}, true},
		{"lt", &Condition{Field: "value", Operator: "lt", Value: 50}, true},
		{"lte 边界", &Condition{Field: "value", Operator: "lte", Value: 42.5}, true},
		{"contains", &Condition{Field: "key", Operator: "contains", Value: "temp"}, true},
		{"startswith", &Condition{Field: "device_id", Operator: "startswith", Value: "dev"}, true},
		{"endswith", &Condition{Field: "device_id", Operator: "endswith", Value: "-1"}, true},
		{"regex", &Condition{Field: "device_id", Operator: "regex", Value: `^dev-\d+$`}, true},
		{"标签字段", &Condition{Field: "location", Operator: "eq", Value: "room-101"}, true},
		{"and", &Condition{Type: "and", And: []*Condition{
			{Field: "key", Value: "temperature"},
			{Field: "value", Operator: "gt", Value: 40},
		}}, true},
		{"and 短路为假", &Condition{And: []*Condition{
			{Field: "key", Value: "humidity"},
			{Field: "value", Operator: "gt", Value: 40},
		}}, false},
		{"or", &Condition{Type: "or", Or: []*Condition{
			{Field: "key", Value: "humidity"},
			{Field: "value", Operator: "gt", Value: 40},
		}}, true},
		{"not", &Condition{Not: &Condition{Field: "key", Value: "humidity"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := e.Evaluate(c.cond, p)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestEvaluatorErrors(t *testing.T) {
	e := NewEvaluator()
	p := pt("d", "k", 1.0, model.TypeFloat)
	cases := map[string]*Condition{
		"未知操作符":      {Field: "value", Operator: "between", Value: 1},
		"未知字段":       {Field: "no_such_field", Value: 1},
		"字符串操作符用于数字": {Field: "value", Operator: "contains", Value: "1"},
		"空正则":        {Field: "key", Operator: "regex", Value: "  "},
		"非法正则":       {Field: "key", Operator: "regex", Value: "("},
		"未知条件类型":     {Type: "magic"},
		"and 缺少子条件":  {Type: "and"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := e.Evaluate(c, p); err == nil {
				t.Fatalf("应返回错误")
			}
		})
	}
}

func TestEvaluatorExpressions(t *testing.T) {
	e := NewEvaluator()
	p := pt("dev-1", "temperature", 42.5, model.TypeFloat)
	cases := map[string]bool{
		"value > 40 && key == 'temperature'": true,
		"value > 50":                         false,
		"value * 2 > 80":                     true,
		"device_id == 'dev-1' || value < 0":  true,
		"!(value > 40)":                      false,
	}
	for expr, want := range cases {
		got, err := e.Evaluate(&Condition{Type: "expression", Expression: expr}, p)
		if err != nil {
			t.Errorf("%q: %v", expr, err)
			continue
		}
		if got != want {
			t.Errorf("%q = %v, want %v", expr, got, want)
		}
	}
	if _, err := e.Evaluate(&Condition{Type: "expression", Expression: ""}, p); err == nil {
		t.Errorf("空表达式应报错")
	}
	if _, err := e.Evaluate(&Condition{Type: "expression", Expression: "value > ("}, p); err == nil {
		t.Errorf("语法错误应报错")
	}
}

// TestExpressionEngineConcurrentCorrectness 并发求值时每个数据点必须使用自己的变量。
// 表达式引擎被对象池复用，如果变量没有隔离或没有清理，并发时会读到其他数据点的值。
func TestExpressionEngineConcurrentCorrectness(t *testing.T) {
	e := NewEvaluator()
	cond := &Condition{Type: "expression", Expression: "value > 50"}
	var wg sync.WaitGroup
	errs := make(chan string, 100)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				v := float64((g*31 + i*17) % 100)
				got, err := e.Evaluate(cond, pt(fmt.Sprintf("d%d", g), "k", v, model.TypeFloat))
				if err != nil {
					errs <- err.Error()
					return
				}
				if got != (v > 50) {
					errs <- fmt.Sprintf("value=%v 得到 %v", v, got)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Fatal(msg)
	}
}

func TestExpressionEngineNoStaleVariables(t *testing.T) {
	eng := AcquireExpressionEngine()
	defer ReleaseExpressionEngine(eng)
	withTag := pt("d", "k", 1.0, model.TypeFloat)
	withTag.AddTag("zone", "A")
	if _, err := eng.Evaluate("tag_zone == 'A'", withTag); err != nil {
		t.Fatalf("标签变量求值失败: %v", err)
	}
	// 同一个引擎处理下一个没有该标签的数据点时，不能读到上一个点的标签
	got, _ := eng.Evaluate("tag_zone == 'A'", pt("d", "k", 1.0, model.TypeFloat))
	if got == true {
		t.Fatalf("读到了上一个数据点残留的变量")
	}
	if _, ok := eng.variables["last_values"]; ok {
		t.Fatalf("不应再伪造 last_values 历史数据")
	}
}

func TestParseExprCached(t *testing.T) {
	a, err1 := parseExprCached("1 + 2")
	b, err2 := parseExprCached("1 + 2")
	if err1 != nil || err2 != nil || a != b {
		t.Fatalf("相同表达式应返回缓存的 AST")
	}
	if _, err := parseExprCached("1 +"); err == nil {
		t.Fatalf("语法错误应被返回（并缓存）")
	}
	if _, err := parseExprCached("1 +"); err == nil {
		t.Fatalf("缓存的语法错误应再次返回")
	}
}

func BenchmarkExpressionCondition(b *testing.B) {
	e := NewEvaluator()
	c := &Condition{Type: "expression", Expression: "value > 40 && key == 'temperature'"}
	p := pt("dev-001", "temperature", 42.5, model.TypeFloat)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Evaluate(c, p)
	}
}

func BenchmarkSimpleCondition(b *testing.B) {
	e := NewEvaluator()
	c := &Condition{And: []*Condition{
		{Field: "key", Operator: "eq", Value: "temperature"},
		{Field: "value", Operator: "gt", Value: 40.0}}}
	p := pt("dev-001", "temperature", 42.5, model.TypeFloat)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Evaluate(c, p)
	}
}
