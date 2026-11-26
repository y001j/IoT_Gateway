package actions

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

// 窗口 > 100 时走批处理路径；以前批处理和直接模式只接受 float64，int 值从不参与统计，
// 且两种模式按不同的键分片，同一序列的统计状态会分裂
func TestShardedAggregateBatchPathAndIntValues(t *testing.T) {
	sam := NewShardedAggregateManager()
	defer sam.Close()
	rule := &rules.Rule{ID: "r"}
	cfg := &AggregateConfig{WindowSize: 200, Functions: []string{"count", "sum", "avg"}, GroupBy: []string{"device_id"}}
	var last *rules.ActionResult
	for i := 1; i <= 10; i++ {
		res, err := sam.ProcessPoint(rule, model.NewPoint("k", "d", i, model.TypeInt), cfg)
		if err != nil {
			t.Fatalf("第 %d 个点: %v", i, err)
		}
		last = res
	}
	out := last.Output.(map[string]interface{})
	fns := out["aggregate_result"].(map[string]interface{})["functions"].(map[string]float64)
	if c, _ := num(out["count"]); c != 10 {
		t.Fatalf("批处理应统计 10 个 int 值: %#v", out)
	}
	if v, _ := num(fns["sum"]); v != 55 {
		t.Fatalf("sum = %v (%#v)", fns["sum"], fns)
	}

	// 直接模式（小窗口）同样接受 int；同一设备（按 device_id 分组）与上面的批处理共享统计状态：
	// 以前两种模式按不同的键分片，状态各自独立
	small := &AggregateConfig{WindowSize: 10, Functions: []string{"sum"}, GroupBy: []string{"device_id"}}
	var res *rules.ActionResult
	for _, v := range []interface{}{1, int64(2), float32(3), "4"} {
		res, _ = sam.ProcessPoint(rule, model.NewPoint("k2", "d", v, model.TypeInt), small)
	}
	dfns := res.Output.(map[string]interface{})["aggregate_result"].(map[string]interface{})["functions"].(map[string]float64)
	if v, _ := num(dfns["sum"]); v != 65 { // 55 + 1+2+3+4
		t.Fatalf("直接模式 sum = %v (%#v)", v, dfns)
	}

	// 空统计状态在清理时被删除
	for _, sh := range sam.shards {
		sh.lastCleanup = 0
		sh.states.Store("empty", NewHighPerformanceStats(10))
	}
	sam.cleanupExpiredStates()
	deadline := time.Now().Add(2 * time.Second)
	for {
		left := 0
		for _, sh := range sam.shards {
			if _, ok := sh.states.Load("empty"); ok {
				left++
			}
		}
		if left == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("空状态未被清理: %d", left)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestShardedAggregateCloseIsSafe(t *testing.T) {
	sam := NewShardedAggregateManager()
	sam.Close()
	sam.Close() // 以前重复关闭会 panic
	// 关闭后批处理路径回退到直接处理，不会向已停止的处理器发送
	cfg := &AggregateConfig{WindowSize: 500, Functions: []string{"count"}}
	res, err := sam.ProcessPoint(&rules.Rule{ID: "r"}, model.NewPoint("k", "d", 1.0, model.TypeFloat), cfg)
	if err != nil || !res.Success {
		t.Fatalf("关闭后应直接处理: %v %+v", err, res)
	}
}

func TestAggregateManagerCleanup(t *testing.T) {
	m := NewAggregateManager()
	defer m.Close()
	m.maxStates = 2
	m.defaultTTL = time.Hour
	rule := &rules.Rule{ID: "r"}
	cfg := &AggregateConfig{WindowSize: 5, Functions: []string{"count"}, GroupBy: []string{"device_id"}}
	for _, dev := range []string{"a", "b"} {
		if _, err := m.ProcessPoint(rule, model.NewPoint("k", dev, 1.0, model.TypeFloat), cfg); err != nil {
			t.Fatal(err)
		}
	}
	// 达到上限：强制清理也清不掉（未过期），新状态不保存
	if _, err := m.ProcessPoint(rule, model.NewPoint("k", "c", 1.0, model.TypeFloat), cfg); err != nil {
		t.Fatal(err)
	}
	if len(m.states) != 2 {
		t.Fatalf("超过上限的状态不应保存: %d", len(m.states))
	}
	// 状态过期后定期清理将其删除
	for _, st := range m.states {
		st.LastAccess = time.Now().Add(-2 * time.Hour)
	}
	m.cleanupExpiredStates()
	if len(m.states) != 0 || m.currentMem != 0 {
		t.Fatalf("过期状态应被清理: %d %d", len(m.states), m.currentMem)
	}
	// 内存上限触发强制清理
	m.maxStates = 100
	m.maxMemory = 1
	if _, err := m.ProcessPoint(rule, model.NewPoint("k", "d", 1.0, model.TypeFloat), cfg); err != nil {
		t.Fatal(err)
	}
	if len(m.states) != 0 {
		t.Fatalf("超过内存上限的状态不应保存: %d", len(m.states))
	}
}

func TestExtractNumericValueComplexTypes(t *testing.T) {
	cases := []struct {
		in   interface{}
		want float64
		ok   bool
	}{
		{map[string]interface{}{"elements": []interface{}{3.0}}, 3, true},
		{map[string]interface{}{"magnitude": 5}, 5, true},
		{map[string]interface{}{"speed": "6"}, 6, true},
		{map[string]interface{}{"brightness": float32(7)}, 7, true},
		{map[string]interface{}{"values": []interface{}{"x", 8}}, 8, true},
		{map[string]interface{}{"value": uint(9)}, 9, true},
		{map[string]interface{}{"x": 1}, 0, false},
		{int32(4), 4, true}, {uint32(4), 4, true}, {uint64(4), 4, true}, {"bad", 0, false}, {[]int{1}, 0, false},
	}
	for _, c := range cases {
		got, err := extractNumericValue(c.in)
		if (err == nil) != c.ok || (c.ok && got != c.want) {
			t.Errorf("%#v -> %v %v", c.in, got, err)
		}
	}
	if _, ok := convertToFloat64(struct{}{}); ok {
		t.Fatal("结构体不应转换成功")
	}
}

func TestAggregateHandlerStatsAndClose(t *testing.T) {
	h := NewAggregateHandler()
	if _, err := h.Execute(context.Background(), model.NewPoint("k", "d", 1.0, model.TypeFloat), &rules.Rule{ID: "r"},
		map[string]interface{}{"window_size": 5.0, "functions": []interface{}{"avg"}}); err != nil {
		t.Fatal(err)
	}
	if st := h.GetStats(); len(st) == 0 {
		t.Fatal("GetStats 应返回统计")
	}
	h.Close()
	if st := (&AggregateHandler{}).GetStats(); st == nil {
		t.Fatal("未初始化时也应返回非 nil")
	}
	(&AggregateHandler{}).Close()
}

func TestIncrementalTimeWindowPaths(t *testing.T) {
	// 缓冲区满后替换最旧数据，需要重新计算 min/max
	s := NewIncrementalStatsWithWindow(3, "time", time.Hour, "none")
	for _, v := range []float64{10, 1, 5, 6, 7} {
		s.AddValue(v)
	}
	if s.GetMin() != 5 || s.GetMax() != 7 {
		t.Fatalf("替换旧值后 min/max = %v/%v, want 5/7", s.GetMin(), s.GetMax())
	}
	// 日历对齐的截止时间
	now := time.Date(2026, 3, 10, 14, 37, 25, 0, time.UTC)
	for dur, want := range map[time.Duration]time.Time{
		48 * time.Hour:   time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC),
		2 * time.Hour:    time.Date(2026, 3, 10, 13, 0, 0, 0, time.UTC),
		5 * time.Minute:  time.Date(2026, 3, 10, 14, 33, 0, 0, time.UTC),
		10 * time.Second: time.Date(2026, 3, 10, 14, 37, 16, 0, time.UTC),
	} {
		c := NewIncrementalStatsWithWindow(10, "time", dur, "calendar")
		if got := c.calculateCalendarAlignedCutoff(now); !got.Equal(want) {
			t.Errorf("%v: cutoff = %v, want %v", dur, got, want)
		}
	}
	cal := NewIncrementalStatsWithWindow(10, "time", time.Hour, "calendar")
	cal.AddValue(1)
	cal.AddValue(2)
	if cal.GetCount() == 0 {
		t.Fatal("日历对齐窗口应保留当前小时内的数据")
	}
}

func TestIncrementalVarianceFallback(t *testing.T) {
	// 平方和溢出为 Inf 时使用备用算法，结果仍为有限值
	s := NewIncrementalStats(5)
	for i := 0; i < 3; i++ {
		s.AddValue(1e200)
	}
	if v := s.GetVariance(); math.IsNaN(v) || math.IsInf(v, 0) {
		t.Fatalf("方差应为有限值: %v", v)
	}
	if sd := s.GetStdDev(); math.IsNaN(sd) || math.IsInf(sd, 0) {
		t.Fatalf("标准差应为有限值: %v", sd)
	}
	c := NewIncrementalStats(0) // 累积模式
	c.AddValue(1e200)
	c.AddValue(1e200)
	if v := c.GetVariance(); math.IsNaN(v) || math.IsInf(v, 0) {
		t.Fatalf("累积模式方差应为有限值: %v", v)
	}
	if (&IncrementalStats{count: 1}).calculateVarianceFallback() != 0 {
		t.Fatal("单个值的方差为 0")
	}
}

func TestTransformMiscBranches(t *testing.T) {
	arr := model.NewCompositePoint("a", "d", &model.ArrayData{Values: []interface{}{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0}, Size: 10})
	for p, want := range map[string]float64{"p25": 3.25, "p50": 5.5, "p75": 7.75, "p90": 9.1, "p95": 9.55, "p99": 9.91} {
		got, ok, msg := tryTransform(t, arr, tp("array_aggregate", map[string]interface{}{"operation": p}))
		if !ok {
			t.Errorf("%s: %s", p, msg)
			continue
		}
		if v, _ := num(got.Value); math.Abs(v-want) > 1e-9 {
			t.Errorf("%s = %v, want %v", p, got.Value, want)
		}
	}
	// 输出类型转换为 bool
	for in, want := range map[interface{}]bool{1.0: true, 0.0: false, "true": true, "no": false, true: true} {
		got, ok, msg := tryTransform(t, model.NewPoint("k", "d", in, model.TypeFloat),
			map[string]interface{}{"type": "pass_through", "output_type": "bool"})
		if !ok {
			t.Errorf("%v: %s", in, msg)
			continue
		}
		if got.Value != want {
			t.Errorf("toBool(%v) = %v", in, got.Value)
		}
	}
	h := NewTransformHandler(nil)
	// int64/int32 的 0 以前被转换成 true
	if h.toBool(int64(0)) || h.toBool(int32(0)) || h.toBool(uint8(0)) || !h.toBool(int64(2)) || h.toBool(0) || h.toBool(float32(0)) {
		t.Fatal("toBool 数值")
	}
	if !h.toBool([]int{}) || h.toBool(nil) {
		t.Fatal("其他类型按是否为 nil")
	}
}

func TestLegacyAlertFieldReflection(t *testing.T) {
	h := NewAlertHandler()
	type s struct {
		Speed float64
		inner int
	}
	if h.extractFieldUsingReflection(&s{Speed: 3}, "speed") != 3.0 {
		t.Fatal("指针结构体字段")
	}
	if h.extractFieldUsingReflection((*s)(nil), "speed") != nil || h.extractFieldUsingReflection(1, "x") != nil ||
		h.extractFieldUsingReflection(nil, "x") != nil || h.extractFieldUsingReflection(s{inner: 1}, "inner") != nil ||
		h.extractFieldUsingReflection(s{}, "nope") != nil {
		t.Fatal("非法输入应返回 nil")
	}
}
