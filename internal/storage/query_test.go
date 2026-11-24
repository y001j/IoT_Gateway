package storage

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

// publishSeries 发布一组指定采集时间的点，并等待写入流
func publishSeries(t *testing.T, js nats.JetStreamContext, device, key string, base time.Time, n int, step time.Duration, val func(i int) interface{}) {
	t.Helper()
	for i := 0; i < n; i++ {
		p := model.NewPoint(key, device, val(i), model.TypeFloat)
		p.Timestamp = base.Add(time.Duration(i) * step)
		data, _ := json.Marshal(p)
		if _, err := js.PublishAsync(model.DataSubject(device, key), data); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-js.PublishAsyncComplete():
	case <-time.After(10 * time.Second):
		t.Fatal("发布超时")
	}
}

func TestQueryRawRangeOrderAndFilter(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := newStore(t, nc, nil)
	js := s.JS()
	// 采集时间贴近入库时间（与生产一致）；范围末尾在入库时间的容忍窗口内，查询会读到序列末尾
	base := time.Now().Add(-2 * time.Minute).Truncate(time.Second)
	// 设备 ID 含 "."，验证主题编码；另一个序列不应混入结果
	publishSeries(t, js, "plc.1", "temp", base, 100, time.Second, func(i int) interface{} { return float64(i) })
	publishSeries(t, js, "plc.1", "pressure", base, 10, time.Second, func(i int) interface{} { return 999.0 })
	// 乱序到达：一个采集时间较早的点最后入库
	late := model.NewPoint("temp", "plc.1", -1.0, model.TypeFloat)
	late.Timestamp = base.Add(95500 * time.Millisecond)
	data, _ := json.Marshal(late)
	if _, err := js.Publish(model.DataSubject("plc.1", "temp"), data); err != nil {
		t.Fatal(err)
	}
	// 规则输出（带 Iot-Origin 头）不属于原始数据
	msg := nats.NewMsg(model.DataSubject("plc.1", "temp"))
	msg.Header.Set(ruleOutputHeader, "rule-engine")
	ro := model.NewPoint("temp", "plc.1", 12345.0, model.TypeFloat)
	ro.Timestamp = base.Add(20 * time.Second)
	msg.Data, _ = json.Marshal(ro)
	if _, err := js.PublishMsg(msg); err != nil {
		t.Fatal(err)
	}

	res, err := s.QueryRaw(context.Background(), RawQuery{DeviceID: "plc.1", Key: "temp",
		Start: base.Add(10 * time.Second), End: base.Add(29 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched != 20 || len(res.Points) != 20 || res.Downsampled || res.Truncated {
		t.Fatalf("应返回 20 个点（10..29 秒）: matched=%d points=%d %+v", res.Matched, len(res.Points), res)
	}
	// 查询过去的时间段时，读到入库时间和采集时间都超过结束时间的数据后提前结束，不扫描到序列末尾
	if res.Scanned >= 100 {
		t.Fatalf("应提前结束扫描, scanned=%d", res.Scanned)
	}
	for i, p := range res.Points {
		if p.Value.(float64) != float64(10+i) {
			t.Fatalf("第 %d 个点值错误: %v", i, p.Value)
		}
	}

	// 乱序点按采集时间排在正确位置
	res, err = s.QueryRaw(context.Background(), RawQuery{DeviceID: "plc.1", Key: "temp", Start: base.Add(95 * time.Second), End: base.Add(96 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Points) != 3 || res.Points[1].Value.(float64) != -1 {
		t.Fatalf("乱序点应按采集时间排序: %+v", res.Points)
	}

	// 没有数据的序列 / 时间范围
	res, err = s.QueryRaw(context.Background(), RawQuery{DeviceID: "nope", Key: "temp", Start: base, End: base.Add(time.Hour)})
	if err != nil || len(res.Points) != 0 {
		t.Fatalf("不存在的序列应返回空: %v %+v", err, res)
	}
	res, err = s.QueryRaw(context.Background(), RawQuery{DeviceID: "plc.1", Key: "temp", Start: base.Add(-time.Hour), End: base.Add(-50 * time.Minute)})
	if err != nil || len(res.Points) != 0 {
		t.Fatalf("范围外应返回空: %v %+v", err, res)
	}
}

func TestQueryRawDownsampleAndLimits(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := newStore(t, nc, func(c *Config) { c.Query.MaxScan = 300; c.Query.MaxLimit = 50; c.Query.DefaultLimit = 20 })
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	publishSeries(t, s.JS(), "d", "v", base, 1000, time.Second, func(i int) interface{} { return float64(i % 10) })

	res, err := s.QueryRaw(context.Background(), RawQuery{DeviceID: "d", Key: "v", Start: base, End: base.Add(1000 * time.Second), Agg: "max"})
	if err != nil {
		t.Fatal(err)
	}
	// 扫描上限 300：标记截断，降采样到默认的 20 个点
	if !res.Truncated || res.Scanned != 300 || res.Matched != 300 {
		t.Fatalf("应在扫描 300 条后截断: %+v", res)
	}
	if !res.Downsampled || len(res.Points) > 20 || res.Agg != "max" {
		t.Fatalf("应降采样到不超过 20 点: n=%d %+v", len(res.Points), res)
	}
	for _, p := range res.Points {
		if p.Value.(float64) != 9 {
			t.Fatalf("max 聚合每个桶都应为 9: %v", p.Value)
		}
	}
	// limit 超过 MaxLimit 时被限制
	res, err = s.QueryRaw(context.Background(), RawQuery{DeviceID: "d", Key: "v", Start: base, End: base.Add(200 * time.Second), Limit: 10000, Agg: "lttb"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Points) != 50 {
		t.Fatalf("limit 应被限制为 50, got %d", len(res.Points))
	}

	// 参数校验
	for _, q := range []RawQuery{
		{Key: "v"},
		{DeviceID: "d"},
		{DeviceID: "d", Key: "v", Start: base.Add(time.Hour), End: base},
		{DeviceID: "d", Key: "v", Agg: "median"},
	} {
		if _, err := s.QueryRaw(context.Background(), q); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v 应返回参数错误, got %v", q, err)
		}
	}
}

func TestQueryConcurrencyLimit(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := newStore(t, nc, func(c *Config) { c.Query.MaxConcurrent = 1; c.Query.Timeout = 100 * time.Millisecond })
	s.querySem <- struct{}{} // 占满许可
	_, err := s.QueryRaw(context.Background(), RawQuery{DeviceID: "d", Key: "v"})
	if !errors.Is(err, ErrQueryBusy) {
		t.Fatalf("许可用尽时应返回繁忙, got %v", err)
	}
	<-s.querySem
	if _, err := s.QueryRaw(context.Background(), RawQuery{DeviceID: "d", Key: "v"}); err != nil {
		t.Fatalf("释放后应可查询: %v", err)
	}
}

func TestListSeriesAndLatest(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := newStore(t, nc, nil)
	base := time.Now().Add(-time.Minute)
	publishSeries(t, s.JS(), "dev.a", "temp", base, 5, time.Second, func(i int) interface{} { return float64(i) })
	publishSeries(t, s.JS(), "dev.a", "hum", base, 3, time.Second, func(i int) interface{} { return float64(100 + i) })
	publishSeries(t, s.JS(), "dev-b", "temp", base, 2, time.Second, func(i int) interface{} { return 7.0 })

	all, total, err := s.ListSeries(context.Background(), "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("应有 3 个序列: %+v", all)
	}
	// 按设备、点位排序，设备 ID 解码还原
	if all[0].DeviceID != "dev-b" || all[1].DeviceID != "dev.a" || all[1].Key != "hum" || all[1].Messages != 3 || all[2].Messages != 5 {
		t.Fatalf("序列列表错误: %+v", all)
	}
	page, total, _ := s.ListSeries(context.Background(), "", 1, 1)
	if total != 3 || len(page) != 1 || page[0].Key != "hum" {
		t.Fatalf("分页错误: %+v", page)
	}
	if page, _, _ := s.ListSeries(context.Background(), "", 10, 5); len(page) != 0 {
		t.Fatalf("越界分页应为空: %+v", page)
	}
	devA, total, _ := s.ListSeries(context.Background(), "dev.a", -1, 0)
	if total != 2 || len(devA) != 2 {
		t.Fatalf("按设备过滤错误: %+v", devA)
	}

	latest, err := s.Latest(context.Background(), "dev.a")
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 2 || latest[0].Key != "hum" || latest[0].Value.(float64) != 102 || latest[1].Value.(float64) != 4 {
		t.Fatalf("最新值错误: %+v", latest)
	}
	if _, err := s.Latest(context.Background(), " "); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("空设备应报错: %v", err)
	}
}

func TestDownsampleFunctions(t *testing.T) {
	base := time.Unix(1700000000, 0)
	pts := make([]SeriesPoint, 100)
	for i := range pts {
		pts[i] = SeriesPoint{Time: base.Add(time.Duration(i) * time.Second), Value: float64(i)}
	}
	end := base.Add(100 * time.Second)
	if got := Downsample(pts, base, end, 200, AggAvg); len(got) != 100 {
		t.Fatal("点数不超过 limit 时应原样返回")
	}
	avg := Downsample(pts, base, end, 10, AggAvg)
	if len(avg) != 10 || avg[0].Value.(float64) != 4.5 || avg[9].Value.(float64) != 94.5 {
		t.Fatalf("avg 错误: %+v", avg)
	}
	mn := Downsample(pts, base, end, 10, AggMin)
	mx := Downsample(pts, base, end, 10, AggMax)
	last := Downsample(pts, base, end, 10, AggLast)
	if mn[1].Value.(float64) != 10 || mx[1].Value.(float64) != 19 || last[1].Value.(float64) != 19 || !last[1].Time.Equal(base.Add(19*time.Second)) {
		t.Fatalf("min/max/last 错误: %v %v %v", mn[1], mx[1], last[1])
	}
	// start/end 无效时用点的时间范围
	if got := Downsample(pts, end, end, 10, AggAvg); len(got) != 10 {
		t.Fatalf("应使用点的时间范围分桶, got %d", len(got))
	}

	// LTTB 保留首尾点和尖峰
	spiky := make([]SeriesPoint, 1000)
	for i := range spiky {
		v := 0.0
		if i == 500 {
			v = 100
		}
		spiky[i] = SeriesPoint{Time: base.Add(time.Duration(i) * time.Millisecond), Value: v}
	}
	lt := Downsample(spiky, base, base.Add(time.Second), 20, AggLTTB)
	if len(lt) != 20 || !lt[0].Time.Equal(spiky[0].Time) || !lt[19].Time.Equal(spiky[999].Time) {
		t.Fatalf("lttb 应保留首尾点: n=%d", len(lt))
	}
	hasSpike := false
	for _, p := range lt {
		if p.Value.(float64) == 100 {
			hasSpike = true
		}
	}
	if !hasSpike {
		t.Fatal("lttb 应保留尖峰")
	}
	if got := lttb(spiky, make([]float64, 1000), 2); len(got) != 2 {
		t.Fatalf("threshold=2 应返回首尾点: %d", len(got))
	}
	if got := lttb(spiky[:5], make([]float64, 5), 10); len(got) != 5 {
		t.Fatal("threshold 不小于点数时原样返回")
	}

	// 非数值点退回 last，返回原始值；bool 按 0/1 处理
	strs := []SeriesPoint{{Time: base, Value: "a"}, {Time: base.Add(time.Second), Value: "b"}, {Time: base.Add(2 * time.Second), Value: "c"}}
	got := Downsample(strs, base, base.Add(3*time.Second), 1, AggAvg)
	if len(got) != 1 || got[0].Value != "c" {
		t.Fatalf("字符串应取最后一个值: %+v", got)
	}
	bools := []SeriesPoint{{Time: base, Value: true}, {Time: base.Add(time.Second), Value: false}}
	if got := Downsample(bools, base, base.Add(2*time.Second), 1, AggAvg); got[0].Value.(float64) != 0.5 {
		t.Fatalf("bool 平均值应为 0.5: %+v", got)
	}
	if _, ok := numeric(math.NaN()); ok {
		t.Fatal("NaN 不应视为数值")
	}
}

func TestQueryConfigFromViper(t *testing.T) {
	v := viper.New()
	v.Set("storage.query.max_concurrent", 4)
	v.Set("storage.query.max_scan", 1000)
	v.Set("storage.query.default_limit", 500)
	v.Set("storage.query.max_limit", 2000)
	v.Set("storage.query.timeout", "3s")
	v.Set("storage.query.tolerance", "0s")
	cfg, err := ConfigFromViper(v)
	if err != nil {
		t.Fatal(err)
	}
	q := cfg.Query
	if q.MaxConcurrent != 4 || q.MaxScan != 1000 || q.DefaultLimit != 500 || q.MaxLimit != 2000 || q.Timeout != 3*time.Second || q.Tolerance != 0 {
		t.Fatalf("查询配置解析错误: %+v", q)
	}
	bad := QueryConfig{DefaultLimit: 5000, MaxLimit: 100, Tolerance: -1}
	bad.applyDefaults()
	if bad.DefaultLimit != 100 || bad.Tolerance != 0 || bad.MaxConcurrent != 2 {
		t.Fatalf("默认值修正错误: %+v", bad)
	}
}

func TestRunnerPauseResumeSkipReplay(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := newStore(t, nc, nil)
	sink := &flakySink{}
	r, err := s.StartSinkRunner(context.Background(), "ops", sink, fastCfg())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { r.Stop() }()
	publishPoints(t, nc, 0, 10)
	testutil.WaitFor(t, 5*time.Second, func() bool { return len(sink.values()) == 10 }, "初始数据未写入")

	// 暂停：数据在本地积压
	r.Pause()
	testutil.WaitFor(t, 2*time.Second, func() bool { return r.Stats().State == StatePaused }, "未进入暂停状态")
	publishPoints(t, nc, 10, 20)
	time.Sleep(300 * time.Millisecond)
	if n := len(sink.values()); n != 10 {
		t.Fatalf("暂停期间不应写入, got %d", n)
	}
	if st := r.Stats(); st.Pending != 10 {
		t.Fatalf("暂停期间积压应为 10: %+v", st)
	}
	// 恢复：按顺序补发
	r.Resume()
	testutil.WaitFor(t, 5*time.Second, func() bool { return len(sink.values()) == 20 }, "恢复后未补发")
	assertSequence(t, sink.values(), 20)

	// 跳过：暂停后积压的数据被丢弃，只写入之后的新数据
	r.Pause()
	testutil.WaitFor(t, 2*time.Second, func() bool { return r.Stats().State == StatePaused }, "未进入暂停状态")
	publishPoints(t, nc, 20, 30)
	r, err = s.ResetSinkRunner(r, "new", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Paused() {
		t.Fatal("重建后应保持暂停状态")
	}
	r.Resume()
	time.Sleep(200 * time.Millisecond)
	publishPoints(t, nc, 30, 35)
	testutil.WaitFor(t, 5*time.Second, func() bool { return len(sink.values()) == 25 }, "跳过后新数据未写入")
	got := sink.values()
	if got[20] != 30 {
		t.Fatalf("跳过后应从 30 开始: %v", got[20:])
	}

	// 重放：从最开始重新投递全部 35 条
	r, err = s.ResetSinkRunner(r, "time", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	testutil.WaitFor(t, 5*time.Second, func() bool { return len(sink.values()) == 60 }, "重放未完成")
	replayed := sink.values()[25:]
	for i, v := range replayed {
		if v != i {
			t.Fatalf("重放顺序错误: %v", replayed)
		}
	}

	// 参数错误
	if _, err := s.ResetSinkRunner(r, "time", time.Time{}); err == nil {
		t.Fatal("重放缺少起始时间应报错")
	}
	if _, err := s.ResetSinkRunner(r, "bogus", time.Time{}); err == nil {
		t.Fatal("未知位置应报错")
	}
	if r.Name() != "ops" {
		t.Fatal(r.Name())
	}
}

func TestBufferedSinkReplayRate(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := newStore(t, nc, nil)
	sink := &flakySink{}
	cfg := fastCfg()
	cfg.StartFrom = "all"
	cfg.ReplayRate = 200 // 点/秒，每批 10 点 → 每批间隔 50ms
	publishPoints(t, nc, 0, 100)
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	r, err := s.StartSinkRunner(context.Background(), "paced", sink, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	testutil.WaitFor(t, 5*time.Second, func() bool { return len(sink.values()) == 100 }, "限速补发未完成")
	if el := time.Since(start); el < 400*time.Millisecond {
		t.Fatalf("100 点按 200 点/秒补发至少需要约 0.45 秒, 实际 %v", el)
	}
	assertSequence(t, sink.values(), 100)

	// paceDelay：空闲后不累积额度
	pr := &SinkRunner{cfg: BufferConfig{ReplayRate: 100}}
	now := time.Now()
	if d := pr.paceDelay(50, now); d != 500*time.Millisecond {
		t.Fatalf("50 点按 100 点/秒应等待 500ms, got %v", d)
	}
	if d := pr.paceDelay(50, now.Add(10*time.Second)); d != 500*time.Millisecond {
		t.Fatalf("空闲后不应累积额度, got %v", d)
	}
	if d := (&SinkRunner{}).paceDelay(1000, now); d != 0 {
		t.Fatal("未配置限速时不应等待")
	}
	if bc := ParseBufferConfig(map[string]interface{}{"enabled": true, "replay_rate": 5000.0}); bc.ReplayRate != 5000 {
		t.Fatalf("replay_rate 解析错误: %+v", bc)
	}
}
