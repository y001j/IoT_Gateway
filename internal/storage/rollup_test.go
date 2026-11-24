package storage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func rollupStore(t *testing.T, nc *nats.Conn) *Store {
	t.Helper()
	return newStore(t, nc, func(c *Config) {
		c.Rollup = DefaultRollupConfig()
		c.Rollup.Enabled = true
		c.Rollup.MaxBytes = 64 << 20
	})
}

func TestRollupAggregatesWindowsAndAutoTier(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := rollupStore(t, nc)
	js := s.JS()
	base := time.Now().Add(-30 * time.Minute).Truncate(time.Minute)
	// 3 个 1 分钟窗口，每个 10 个点：窗口 w 的值为 w*100 + 0..9
	for w := 0; w < 3; w++ {
		publishSeries(t, js, "plc.1", "temp", base.Add(time.Duration(w)*time.Minute), 10, 5*time.Second,
			func(i int) interface{} { return float64(w*100 + i) })
	}
	// 非数值点和规则输出不参与汇总
	publishSeries(t, js, "plc.1", "state", base, 3, time.Second, func(int) interface{} { return "on" })
	msg := nats.NewMsg(model.DataSubject("plc.1", "temp"))
	msg.Header.Set(ruleOutputHeader, "rule-engine")
	ro := model.NewPoint("temp", "plc.1", 99999.0, model.TypeFloat)
	ro.Timestamp = base.Add(time.Second)
	msg.Data, _ = json.Marshal(ro)
	if _, err := js.PublishMsg(msg); err != nil {
		t.Fatal(err)
	}

	if _, err := s.QueryRollup(context.Background(), RawQuery{DeviceID: "plc.1", Key: "temp"}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("汇总层未启动时应返回参数错误, got %v", err)
	}
	r, err := s.StartRollup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	testutil.WaitFor(t, 10*time.Second, func() bool { return r.Stats().Written == 3 }, "应写出 3 个窗口")
	st := r.Stats()
	if st.Processed != 30 || st.OpenWindows != 0 || st.Records != 3 || st.Interval != "1m0s" {
		t.Fatalf("汇总器状态错误: %+v", st)
	}

	q := RawQuery{DeviceID: "plc.1", Key: "temp", Start: base, End: base.Add(3 * time.Minute)}
	res, err := s.QueryRollup(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tier != "rollup" || len(res.Points) != 3 || res.Matched != 30 {
		t.Fatalf("汇总查询错误: %+v", res)
	}
	for w, p := range res.Points {
		if !p.Time.Equal(base.Add(time.Duration(w)*time.Minute)) || p.Value.(float64) != float64(w*100)+4.5 {
			t.Fatalf("窗口 %d 平均值错误: %+v", w, p)
		}
	}
	for agg, want := range map[string]float64{AggMin: 100, AggMax: 109, AggLast: 109} {
		q2 := q
		q2.Agg = agg
		res, err := s.QueryRollup(context.Background(), q2)
		if err != nil || res.Points[1].Value.(float64) != want {
			t.Fatalf("%s 聚合错误: %v %+v", agg, err, res)
		}
	}
	// 降采样：limit 小于窗口数
	q3 := q
	q3.Limit = 1
	q3.Agg = AggLTTB
	if res, err := s.QueryRollup(context.Background(), q3); err != nil || len(res.Points) != 1 || !res.Downsampled {
		t.Fatalf("汇总层降采样错误: %v %+v", err, res)
	}
	// 没有汇总数据的序列
	if res, err := s.QueryRollup(context.Background(), RawQuery{DeviceID: "none", Key: "x", Start: base}); err != nil || len(res.Points) != 0 {
		t.Fatalf("不存在的序列应返回空: %v %+v", err, res)
	}

	// 自动选层：起点早于原始流中最早数据的入库时间（数据都是刚刚写入的）→ 汇总层
	res, err = s.Query(context.Background(), q, "")
	if err != nil || res.Tier != "rollup" {
		t.Fatalf("应自动选择汇总层: %v %+v", err, res)
	}
	res, err = s.Query(context.Background(), q, TierRaw)
	if err != nil || res.Tier != "raw" || res.Matched != 30 {
		t.Fatalf("指定原始层错误: %v %+v", err, res)
	}
	res, err = s.Query(context.Background(), RawQuery{DeviceID: "plc.1", Key: "temp", Start: time.Now().Add(-100 * time.Millisecond)}, TierAuto)
	if err != nil || res.Tier != "raw" {
		t.Fatalf("起点在原始数据范围内应查原始层: %v %+v", err, res)
	}
	if _, err := s.Query(context.Background(), q, "cold"); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("未知数据层应报错: %v", err)
	}

	// 迟到数据：窗口已写出后到达的点不再计入
	publishSeries(t, js, "plc.1", "temp", base.Add(30*time.Second), 1, time.Second, func(int) interface{} { return 1e6 })
	testutil.WaitFor(t, 5*time.Second, func() bool { return r.Stats().Late == 1 }, "迟到点应被计数")
	if r.Stats().Written != 3 {
		t.Fatal("迟到点不应产生新的汇总记录")
	}
}

func TestRollupRestartDoesNotLoseOpenWindows(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := rollupStore(t, nc)
	r, err := s.StartRollup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// 采集时间在未来的点：水位不超过当前时间，窗口一直处于打开状态
	future := time.Now().Add(time.Hour).Truncate(time.Minute)
	publishSeries(t, s.JS(), "d", "v", future, 5, time.Second, func(i int) interface{} { return float64(i) })
	testutil.WaitFor(t, 5*time.Second, func() bool { return r.Stats().Processed == 5 }, "未处理")
	if st := r.Stats(); st.OpenWindows != 1 || st.Written != 0 {
		t.Fatalf("窗口应保持打开: %+v", st)
	}
	// 停止（模拟重启）：未写出的窗口对应的原始消息没有确认
	r.Stop()
	r2, err := s.StartRollup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	testutil.WaitFor(t, 5*time.Second, func() bool { return r2.Stats().Processed == 5 }, "重启后应重新汇总未确认的数据")
	r2.flush(context.Background(), future.Add(2*time.Hour), true)
	if r2.Stats().Written != 1 {
		t.Fatalf("应写出 1 个窗口: %+v", r2.Stats())
	}
	res, err := s.QueryRollup(context.Background(), RawQuery{DeviceID: "d", Key: "v", Start: future, End: future.Add(time.Minute)})
	if err != nil || len(res.Points) != 1 || res.Matched != 5 {
		t.Fatalf("重启后窗口数据应完整（5 个点）: %v %+v", err, res)
	}
}

func TestRollupDuplicateWindowsKeepLargestCount(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := rollupStore(t, nc)
	if _, err := s.StartRollup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	start := time.Now().Add(-10 * time.Minute).Truncate(time.Minute)
	for _, rec := range []RollupRecord{
		{DeviceID: "d", Key: "k", Start: start, Count: 10, Sum: 100, Min: 1, Max: 20, Last: 5},
		{DeviceID: "d", Key: "k", Start: start, Count: 4, Sum: 400, Min: 100, Max: 100, Last: 100}, // 重启后部分重算
	} {
		data, _ := json.Marshal(rec)
		if _, err := s.JS().Publish(RollupSubject("d", "k"), data); err != nil {
			t.Fatal(err)
		}
	}
	res, err := s.QueryRollup(context.Background(), RawQuery{DeviceID: "d", Key: "k", Start: start, End: start.Add(time.Minute)})
	if err != nil || len(res.Points) != 1 || res.Points[0].Value.(float64) != 10 || res.Matched != 10 {
		t.Fatalf("同一窗口应取 count 最大的记录: %v %+v", err, res)
	}
}

func TestRollupRecordAndConfig(t *testing.T) {
	var r RollupRecord
	if r.value(AggAvg) != 0 {
		t.Fatal("空记录平均值应为 0")
	}
	t0 := time.Unix(1000, 0)
	r.add(5, t0.Add(2*time.Second))
	r.add(1, t0) // 采集时间更早：不改变 last
	r.add(9, t0.Add(time.Second))
	if r.Count != 3 || r.Min != 1 || r.Max != 9 || r.Last != 5 || r.value(AggAvg) != 5 {
		t.Fatalf("汇总计算错误: %+v", r)
	}
	if RollupSubject("a.b", "k") != "iot.rollup.a~2Eb.k" {
		t.Fatal(RollupSubject("a.b", "k"))
	}

	v := viper.New()
	v.Set("storage.rollup.enabled", true)
	v.Set("storage.rollup.stream", "R")
	v.Set("storage.rollup.interval", "5m")
	v.Set("storage.rollup.lateness", "0s")
	v.Set("storage.rollup.max_age", "2160h")
	v.Set("storage.rollup.max_bytes", "512MB")
	cfg, err := ConfigFromViper(v)
	if err != nil {
		t.Fatal(err)
	}
	rc := cfg.Rollup
	if !rc.Enabled || rc.Stream != "R" || rc.Interval != 5*time.Minute || rc.Lateness != 0 || rc.MaxAge != 2160*time.Hour || rc.MaxBytes != 512<<20 {
		t.Fatalf("汇总配置解析错误: %+v", rc)
	}
	v.Set("storage.rollup.max_bytes", "3GB") // 2GB + 3GB > 4GB
	if _, err := ConfigFromViper(v); err == nil {
		t.Fatal("原始流与汇总流之和超过 max_disk 应报错")
	}
	v.Set("storage.rollup.max_bytes", "lots")
	if _, err := ConfigFromViper(v); err == nil {
		t.Fatal("无效容量应报错")
	}
	bad := RollupConfig{Lateness: -time.Second}
	bad.applyDefaults()
	if bad.Lateness != 0 || bad.Interval != time.Minute || bad.Stream != "IOT_ROLLUP" || bad.BatchSize != 1000 {
		t.Fatalf("默认值修正错误: %+v", bad)
	}
}
