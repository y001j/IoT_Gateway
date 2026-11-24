package actions

import (
	"context"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

func TestFilterThreshold(t *testing.T) {
	h := NewFilterHandler()
	cases := []struct {
		op   string
		v    float64
		want bool
	}{
		{"gt", 11, true}, {"gt", 10, false}, {"lt", 9, true}, {"gte", 10, true},
		{"lte", 10, true}, {"eq", 10, true}, {"ne", 10, false},
	}
	for _, c := range cases {
		cfg := map[string]interface{}{"type": "threshold", "parameters": map[string]interface{}{"threshold": 10.0, "operator": c.op}}
		if got := runFilter(t, h, point("d", "k", c.v, model.TypeFloat), cfg); got != c.want {
			t.Errorf("%s %v: got %v", c.op, c.v, got)
		}
	}
	r, _ := h.Execute(context.Background(), point("d", "k", 1.0, model.TypeFloat), testRule,
		map[string]interface{}{"type": "threshold", "parameters": map[string]interface{}{"threshold": 1.0, "operator": "between"}})
	if r.Success {
		t.Errorf("未知操作符应返回失败")
	}
}

func TestFilterPatternWildcards(t *testing.T) {
	h := NewFilterHandler()
	for _, c := range []struct {
		pattern, device string
		want            bool
	}{
		{"test-*", "test-01", true},
		{"*-01", "test-01", true},
		{"plc*line?", "plc-A-line3", true}, // 以前中间的 * 和 ? 不生效
		{"plc*line?", "plc-A-line34", false},
		{"dev", "dev", true},
		{"dev", "dev2", false},
		{"*", "anything", true},
	} {
		cfg := map[string]interface{}{"type": "pattern", "parameters": map[string]interface{}{"pattern": c.pattern, "field": "device_id"}}
		if got := runFilter(t, h, point(c.device, "k", 1, model.TypeInt), cfg); got != c.want {
			t.Errorf("pattern %q vs %q: got %v want %v", c.pattern, c.device, got, c.want)
		}
	}
}

func TestFilterQuality(t *testing.T) {
	h := NewFilterHandler()
	bad := point("d", "k", 1, model.TypeInt)
	bad.Quality = 2
	if !runFilter(t, h, bad, map[string]interface{}{"type": "quality"}) {
		t.Errorf("默认只允许质量码 0")
	}
	if runFilter(t, h, bad, map[string]interface{}{"type": "quality", "parameters": map[string]interface{}{"allowed_quality": []interface{}{0.0, 2.0}}}) {
		t.Errorf("允许列表中的质量码不应被过滤")
	}
}

func TestFilterTimeWindow(t *testing.T) {
	h := NewFilterHandler()
	old := point("d", "k", 1, model.TypeInt)
	old.Timestamp = time.Now().Add(-time.Hour)
	cfg := map[string]interface{}{"type": "time_window", "parameters": map[string]interface{}{"window": "5m"}}
	if !runFilter(t, h, old, cfg) {
		t.Errorf("过旧的数据应被过滤")
	}
	if runFilter(t, h, point("d", "k", 1, model.TypeInt), cfg) {
		t.Errorf("新数据不应被过滤")
	}
}

func TestFilterChangeRate(t *testing.T) {
	h := NewFilterHandler()
	cfg := map[string]interface{}{"type": "change_rate", "parameters": map[string]interface{}{"max_change_rate": 1.0, "time_window": "1m"}}
	base := time.Now()
	mk := func(v float64, dt time.Duration) model.Point {
		p := point("d", "k", v, model.TypeFloat)
		p.Timestamp = base.Add(dt)
		return p
	}
	if runFilter(t, h, mk(10, 0), cfg) {
		t.Errorf("第一个点不应被过滤")
	}
	if !runFilter(t, h, mk(30, 2*time.Second), cfg) {
		t.Errorf("10/s 的变化率超过 1/s，应被过滤")
	}
	if runFilter(t, h, mk(11, 3*time.Second), cfg) {
		t.Errorf("相对基准值的变化率 0.33/s 不应被过滤（被过滤的点不更新基准）")
	}
}

func TestFilterStatisticalAnomaly(t *testing.T) {
	h := NewFilterHandler()
	cfg := map[string]interface{}{"type": "statistical_anomaly", "parameters": map[string]interface{}{"window_size": 10.0, "std_threshold": 3.0, "min_samples": 5.0}}
	for _, v := range []float64{10, 11, 9, 10, 11, 9, 10} {
		if runFilter(t, h, point("d", "k", v, model.TypeFloat), cfg) {
			t.Fatalf("正常值 %v 不应被过滤", v)
		}
	}
	if !runFilter(t, h, point("d", "k", 100.0, model.TypeFloat), cfg) {
		t.Errorf("明显异常的值应被过滤")
	}
}

func TestFilterConsecutive(t *testing.T) {
	h := NewFilterHandler()
	cfg := map[string]interface{}{"type": "consecutive", "parameters": map[string]interface{}{
		"consecutive_count": 3.0,
		"inner_filter":      map[string]interface{}{"type": "threshold", "parameters": map[string]interface{}{"threshold": 50.0, "operator": "gt"}},
	}}
	results := []bool{}
	for _, v := range []float64{60, 60, 60, 10, 60} {
		results = append(results, runFilter(t, h, point("d", "k", v, model.TypeFloat), cfg))
	}
	want := []bool{false, false, true, false, false}
	for i := range want {
		if results[i] != want[i] {
			t.Fatalf("连续异常过滤结果 = %v, want %v", results, want)
		}
	}
	// 以前 inner_filter 缺少 type 时会 panic
	r, err := h.Execute(context.Background(), point("d", "k", 1.0, model.TypeFloat), testRule,
		map[string]interface{}{"type": "consecutive", "parameters": map[string]interface{}{"inner_filter": map[string]interface{}{}}})
	if err != nil || r.Success {
		t.Errorf("缺少 inner_filter.type 应返回失败结果而不是 panic")
	}
}
