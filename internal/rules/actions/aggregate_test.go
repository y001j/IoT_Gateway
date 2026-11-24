package actions

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestIncrementalStatsWindowed(t *testing.T) {
	s := NewIncrementalStatsWithConfig(4, map[string]interface{}{"upper_limit": 10.0, "lower_limit": 0.0})
	for _, v := range []float64{100, 2, 4, 6, 8} { // 窗口 4：100 被挤出
		s.AddValue(v)
	}
	if s.GetCount() != 4 { // 窗口模式下为窗口内的数量
		t.Fatalf("count = %d", s.GetCount())
	}
	if !approx(s.GetMean(), 5) || s.GetMin() != 2 || s.GetMax() != 8 {
		t.Fatalf("窗口统计错误: mean=%v min=%v max=%v", s.GetMean(), s.GetMin(), s.GetMax())
	}
	if !approx(s.GetMedian(), 5) {
		t.Fatalf("median = %v", s.GetMedian())
	}
	if s.GetLastValue() != 8 {
		t.Fatalf("last = %v", s.GetLastValue())
	}
	if !approx(s.GetVariance(), 20.0/3) && !approx(s.GetVariance(), 5) {
		t.Fatalf("variance = %v（应为样本或总体方差）", s.GetVariance())
	}
	if s.GetAboveCount() != 0 || s.GetBelowCount() != 0 || s.GetInRangeCount() != 4 {
		t.Fatalf("阈值计数错误: above=%v below=%v in=%v", s.GetAboveCount(), s.GetBelowCount(), s.GetInRangeCount())
	}
	st := s.GetStats()
	if st["count"] == 0 || st["avg"] == 0 {
		t.Fatalf("GetStats 缺少字段: %v", st)
	}
	s.AddNullValue()
	if s.GetNullRate() <= 0 || s.GetCompleteness() >= 1 {
		t.Fatalf("空值统计错误: null_rate=%v completeness=%v", s.GetNullRate(), s.GetCompleteness())
	}
	s.Reset()
	if !s.IsEmpty() || s.GetCount() != 0 {
		t.Fatalf("Reset 后应为空")
	}
}

func TestHighPerformanceStatsCountsZeroValues(t *testing.T) {
	hps := NewHighPerformanceStatsWithConfig(5, map[string]interface{}{"lower_limit": 0.0, "upper_limit": 10.0})
	for _, v := range []float64{0, 0, 0, 5, 20} {
		hps.AddValue(v)
	}
	st := hps.GetStats()
	// 回归：值为 0.0 的读数以前被当作"空槽位"排除
	if st["p50"] != 0 {
		t.Fatalf("p50 应为 0（3 个 0 值）, got %v", st["p50"])
	}
	if st["in_range_count"] != 4 || st["above_count"] != 1 {
		t.Fatalf("阈值计数错误（0 值和下限 0 都应生效）: %v", st)
	}
	if st["count"] != 5 || st["min"] != 0 || st["max"] != 20 || !approx(st["avg"], 5) {
		t.Fatalf("基础统计错误: %v", st)
	}
	hps.Reset()
	if !hps.IsEmpty() {
		t.Fatalf("Reset 后应为空")
	}
	hps.AddBatch([]float64{1, 2, 3})
	if hps.GetCount() != 3 {
		t.Fatalf("AddBatch 计数错误")
	}
}

func TestHighPerformanceStatsNegativeLimits(t *testing.T) {
	hps := NewHighPerformanceStatsWithConfig(3, map[string]interface{}{"upper_limit": -5.0})
	for _, v := range []float64{-10, -1, -6} {
		hps.AddValue(v)
	}
	if st := hps.GetStats(); st["above_count"] != 1 {
		t.Fatalf("负数上限应生效: %v", st)
	}
}

func TestAggregateGroupBy(t *testing.T) {
	h := NewAggregateHandler()
	cfg := map[string]interface{}{"window_size": 2.0, "functions": []interface{}{"avg"}, "group_by": []interface{}{"device_id"}}
	rule := &rules.Rule{ID: "g"}
	run := func(dev string, v float64) map[string]interface{} {
		r, err := h.Execute(context.Background(), point(dev, "k", v, model.TypeFloat), rule, cfg)
		if err != nil {
			t.Fatal(err)
		}
		return output(t, r)
	}
	run("a", 10)
	run("b", 100)
	out := run("a", 20) // 设备 a 的窗口满：(10+20)/2，不能混入设备 b 的 100
	res, ok := out["aggregate_result"].(*rules.AggregateResult)
	if !ok {
		t.Fatalf("设备 a 窗口满时应输出结果: %v", out)
	}
	if res.Functions["avg"].(float64) != 15 {
		t.Fatalf("分组聚合混入了其他设备的数据: avg=%v", res.Functions["avg"])
	}
}

func TestAggregateTimeWindow(t *testing.T) {
	h := NewAggregateHandler()
	cfg := map[string]interface{}{"window_type": "time", "window": "1h", "functions": []interface{}{"count", "sum"}}
	var out map[string]interface{}
	for _, v := range []float64{1, 2, 3} {
		r, err := h.Execute(context.Background(), point("d", "k", v, model.TypeFloat), &rules.Rule{ID: "t"}, cfg)
		if err != nil {
			t.Fatal(err)
		}
		out = output(t, r)
	}
	res, ok := out["aggregate_result"].(*rules.AggregateResult)
	if !ok {
		t.Fatalf("时间窗口应输出结果: %v", out)
	}
	if res.Functions["sum"].(float64) != 6 {
		t.Fatalf("sum = %v", res.Functions["sum"])
	}
}

func TestOptimizedAggregateHandler(t *testing.T) {
	h := NewOptimizedAggregateHandler()
	defer h.Close()
	cfg := map[string]interface{}{"window_size": 3.0, "functions": []interface{}{"avg", "max"}}
	var last *rules.ActionResult
	for _, v := range []float64{3, 6, 9} {
		r, err := h.Execute(context.Background(), point("d", "k", v, model.TypeFloat), &rules.Rule{ID: "o"}, cfg)
		if err != nil || !r.Success {
			t.Fatalf("执行失败: %v %+v", err, r)
		}
		last = r
	}
	out := output(t, last)
	if out["aggregated"] != true {
		t.Fatalf("窗口满时应输出: %v", out)
	}
	res, ok := out["aggregate_result"].(map[string]interface{})
	if !ok {
		t.Fatalf("aggregate_result 类型 = %T", out["aggregate_result"])
	}
	fns, _ := res["functions"].(map[string]float64)
	if avg, ok := fns["avg"]; !ok || avg != 6 {
		t.Fatalf("avg 错误: %v", res)
	}
	if h.GetMetrics() == nil {
		t.Fatalf("GetMetrics 不应为 nil")
	}
}

func TestAggregateHandlerSelectionByEnv(t *testing.T) {
	t.Setenv("IOT_GATEWAY_ENABLE_OPTIMIZED_AGGREGATE", "")
	if _, ok := newAggregateActionHandler().(*AggregateHandler); !ok {
		t.Fatalf("默认应使用 AggregateHandler")
	}
	t.Setenv("IOT_GATEWAY_ENABLE_OPTIMIZED_AGGREGATE", "true")
	if _, ok := newAggregateActionHandler().(*OptimizedAggregateHandler); !ok {
		t.Fatalf("开启环境变量后应使用 OptimizedAggregateHandler")
	}
}

func TestAggregateStateCleanup(t *testing.T) {
	m := NewAggregateManager()
	defer m.Close()
	cfg := &AggregateConfig{WindowSize: 10, Functions: []string{"avg"}, WindowType: "count", TTL: time.Millisecond}
	if _, err := m.ProcessPoint(&rules.Rule{ID: "c"}, point("d", "k", 1.0, model.TypeFloat), cfg); err != nil {
		t.Fatal(err)
	}
	if m.GetStats()["total_states"] == nil && len(m.GetStats()) == 0 {
		t.Fatalf("应能获取聚合管理器统计")
	}
}
