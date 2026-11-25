package actions

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestIncrementalStatsCumulativeAndQuality(t *testing.T) {
	s := NewIncrementalStatsWithConfig(0, map[string]interface{}{"upper_limit": 8.0, "lower_limit": 2.0, "outlier_threshold": 1.5})
	if !s.IsEmpty() {
		t.Fatal("新建的统计应为空")
	}
	for _, v := range []float64{2, 4, 4, 4, 5, 5, 7, 9} {
		s.AddValue(v)
	}
	s.AddNullValue()
	s.AddNullValue()
	// 回归：缓存失效时 GetStdDev 持有写锁调用 GetVariance（再加读锁），以前会永久卡死
	if sd := s.GetStdDev(); !near(sd, math.Sqrt(32.0/7)) {
		t.Fatalf("标准差错误: %v", sd)
	}
	if s.GetCount() != 8 || s.GetSum() != 40 || s.GetMean() != 5 || !near(s.GetStdDev(), math.Sqrt(32.0/7)) {
		t.Fatalf("累积统计错误: n=%d sum=%v mean=%v sd=%v", s.GetCount(), s.GetSum(), s.GetMean(), s.GetStdDev())
	}
	if s.GetFirstValue() != 2 || s.GetLastValue() != 9 || s.GetMin() != 2 || s.GetMax() != 9 {
		t.Fatal("首末值/极值错误")
	}
	if s.GetNullRate() != 0.2 || !near(s.GetCompleteness(), 0.8) || s.GetChange() != 7 || s.GetChangeRate() != 350 {
		t.Fatalf("质量/变化指标错误: null=%v change=%v rate=%v", s.GetNullRate(), s.GetChange(), s.GetChangeRate())
	}
	st := s.GetStats()
	if st["count"] != 8 || st["p50"] == 0 || st["volatility"] <= 0 || st["cv"] != st["volatility"] {
		t.Fatalf("GetStats 错误: %v", st)
	}
	if s.GetVolatility() <= 0 || s.GetCV() != s.GetVolatility() {
		t.Fatal("波动率在 GetStats 之后应可读取")
	}
	if s.GetAboveCount() != 1 || s.GetBelowCount() != 0 || s.GetInRangeCount() != 7 {
		t.Logf("累积模式下的阈值计数: above=%v below=%v in=%v", s.GetAboveCount(), s.GetBelowCount(), s.GetInRangeCount())
	}
	if s.GetOutlierCount() < 0 || s.Age() < 0 || s.GetLastUpdateTime().IsZero() {
		t.Fatal("异常值/时间信息错误")
	}
	s.Reset()
	if !s.IsEmpty() || s.GetCount() != 0 {
		t.Fatal("Reset 后应为空")
	}
	// 没有阈值配置时计数为 0
	if NewIncrementalStats(0).GetAboveCount() != 0 || NewIncrementalStats(0).GetBelowCount() != 0 {
		t.Fatal("未配置阈值时应为 0")
	}
}

func TestIncrementalStatsWindows(t *testing.T) {
	// 计数窗口：只保留最近 3 个值
	w := NewIncrementalStatsWithConfig(3, map[string]interface{}{"upper_limit": 10.0, "lower_limit": 2.0})
	for _, v := range []float64{100, 1, 5, 20} {
		w.AddValue(v)
	}
	if w.GetCount() != 3 || w.GetMax() != 20 || w.GetMin() != 1 || w.GetSum() != 26 {
		t.Fatalf("计数窗口错误: n=%d max=%v min=%v sum=%v", w.GetCount(), w.GetMax(), w.GetMin(), w.GetSum())
	}
	if w.GetAboveCount() != 1 || w.GetBelowCount() != 1 || w.GetInRangeCount() != 1 {
		t.Fatalf("窗口阈值计数错误: %v %v %v", w.GetAboveCount(), w.GetBelowCount(), w.GetInRangeCount())
	}
	if w.GetMedian() != 5 {
		t.Fatalf("窗口中位数错误: %v", w.GetMedian())
	}

	// 时间窗口（含日历对齐）
	tw := NewIncrementalStatsWithWindow(100, "time", time.Minute, "minute")
	for i := 0; i < 5; i++ {
		tw.AddValue(float64(i))
	}
	if tw.GetCount() == 0 || tw.GetMax() != 4 {
		t.Fatalf("时间窗口错误: n=%d max=%v", tw.GetCount(), tw.GetMax())
	}
	if st := tw.GetStats(); st["p95"] < 3 {
		t.Fatalf("时间窗口分位数错误: %v", st)
	}
	for _, align := range []string{"hour", "day", "none"} {
		x := NewIncrementalStatsWithWindow(10, "time", time.Hour, align)
		x.AddValue(1)
		if x.GetCount() != 1 {
			t.Errorf("对齐方式 %s 的时间窗口错误", align)
		}
	}
	// 窗口大小边界
	if NewIncrementalStatsWithWindow(-5, "count", 0, "none").windowSize != 0 || NewIncrementalStatsWithWindow(1<<30, "count", 0, "none").windowSize != 1000000 {
		t.Fatal("窗口大小边界修正错误")
	}
}

// TestIncrementalStatsConcurrentReaders 回归：导出的质量/变化指标以前不加锁，与 AddValue 并发时存在数据竞争
func TestIncrementalStatsConcurrentReaders(t *testing.T) {
	s := NewIncrementalStatsWithConfig(50, map[string]interface{}{"upper_limit": 50.0, "lower_limit": 10.0})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			s.AddValue(float64(i % 100))
			if i%10 == 0 {
				s.AddNullValue()
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			_ = s.GetNullRate() + s.GetCompleteness() + s.GetChange() + s.GetChangeRate() + s.GetVolatility() +
				s.GetCV() + s.GetAboveCount() + s.GetBelowCount() + s.GetInRangeCount() + s.GetOutlierCount()
			if i%100 == 0 {
				_ = s.GetStats()
			}
		}
	}()
	wg.Wait()
}

func TestOptimizedAggregateHandlerConfigAndFallback(t *testing.T) {
	h := NewOptimizedAggregateHandler()
	defer h.Close()
	if h.Name() != "optimized_aggregate" || !h.IsOptimizationEnabled() || len(h.GetSupportedFunctions()) < 10 {
		t.Fatalf("基本信息错误: %s %v", h.Name(), h.GetSupportedFunctions())
	}
	rule := &rules.Rule{ID: "agg", Name: "agg"}
	cfg := map[string]interface{}{"window_size": 3.0, "functions": []interface{}{"avg", "max", "count"},
		"group_by": []interface{}{"device_id"}, "upper_limit": 100.0, "lower_limit": 0.0}
	if err := h.Validate(cfg); err != nil {
		t.Fatalf("合法配置应通过校验: %v", err)
	}
	var last *rules.ActionResult
	for i := 1; i <= 3; i++ {
		res, err := h.Execute(context.Background(), model.NewPoint("temp", "d1", float64(i*10), model.TypeFloat), rule, cfg)
		if err != nil || !res.Success {
			t.Fatalf("执行失败: %v %+v", err, res)
		}
		last = res
	}
	if last.Output == nil {
		t.Fatalf("窗口满后应输出聚合结果: %+v", last)
	}
	for _, bad := range []map[string]interface{}{
		{"functions": []interface{}{"nope"}},
		{"window_size": -1.0, "functions": []interface{}{"avg"}},
		{"functions": []interface{}{"avg"}, "upper_limit": "x"},
	} {
		if err := h.Validate(bad); err == nil {
			t.Errorf("非法配置 %v 应校验失败", bad)
		}
	}
	// 字符串形式的窗口、函数和分组
	if res, err := h.Execute(context.Background(), model.NewPoint("temp", "d2", 1.0, model.TypeFloat), rule,
		map[string]interface{}{"window_size": "2", "functions": "avg", "group_by": "device_id"}); err != nil || !res.Success {
		t.Fatalf("字符串配置执行失败: %v %+v", err, res)
	}

	h.EnableOptimization(false)
	if h.IsOptimizationEnabled() {
		t.Fatal("关闭优化失败")
	}
	// 关闭优化后回退到原始实现
	if res, err := h.Execute(context.Background(), model.NewPoint("temp", "d3", 1.0, model.TypeFloat), rule, cfg); err != nil || !res.Success {
		t.Fatalf("回退实现执行失败: %v %+v", err, res)
	}
	h.EnableOptimization(true)
	pc := h.GetPerformanceConfig()
	pc.BatchSize = 64
	h.UpdatePerformanceConfig(pc)
	if h.GetMetrics() == nil {
		t.Fatal("应返回指标")
	}
}

// TestShardIDAlwaysInRange 回归：哈希值先转 int 再取模，在 32 位平台上会得到负下标（ARMv7 上 panic）
func TestShardIDAlwaysInRange(t *testing.T) {
	sam := NewShardedAggregateManager()
	defer sam.Close()
	for i := 0; i < 5000; i++ {
		id := sam.getShardID("device-" + time.Duration(i).String())
		if id < 0 || id >= int(sam.numShards) {
			t.Fatalf("分片下标越界: %d (共 %d 个分片)", id, sam.numShards)
		}
	}
}
