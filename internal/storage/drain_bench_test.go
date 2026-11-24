package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

// BenchmarkDrainBacklog 测量积压数据的补发速度（目标恢复后的追赶能力）
func BenchmarkDrainBacklog(b *testing.B) {
	_, nc := testutil.StartNATS(b)
	cfg := DefaultConfig()
	cfg.MaxBytes = 256 << 20
	cfg.MaxDisk = 1 << 30
	store, err := New(nc, cfg)
	if err != nil {
		b.Fatal(err)
	}
	js, _ := nc.JetStream()
	const n = 200000
	data, _ := json.Marshal(model.NewPoint("temperature", "dev-1", 21.5, model.TypeFloat))
	for i := 0; i < n; i++ {
		if _, err := js.PublishAsync("iot.data.dev-1.temperature", data); err != nil {
			b.Fatal(err)
		}
	}
	<-js.PublishAsyncComplete()
	sink := &flakySink{}
	b.ResetTimer()
	start := time.Now()
	r, err := store.StartSinkRunner(context.Background(), "drain", sink, BufferConfig{StartFrom: "all", BatchSize: 1000, MaxWait: 100 * time.Millisecond})
	if err != nil {
		b.Fatal(err)
	}
	for r.Stats().Delivered < n {
		time.Sleep(20 * time.Millisecond)
	}
	el := time.Since(start)
	r.Stop()
	b.ReportMetric(float64(n)/el.Seconds(), "points/s")
}

// BenchmarkQueryRaw24h 查询单序列 24 小时历史（每秒 1 点，共 86400 点；另有 10 个干扰序列），降采样到 1000 点。
// 设计验收目标：在目标 ARM 设备上 P95 < 2 秒。
func BenchmarkQueryRaw24h(b *testing.B) {
	_, nc := testutil.StartNATS(b)
	cfg := DefaultConfig()
	cfg.MaxBytes = 512 << 20
	cfg.MaxDisk = 1 << 30
	store, err := New(nc, cfg)
	if err != nil {
		b.Fatal(err)
	}
	js, _ := nc.JetStream()
	end := time.Now()
	start := end.Add(-24 * time.Hour)
	const n = 86400
	for i := 0; i < n; i++ {
		p := model.NewPoint("temp", "target", float64(i%100), model.TypeFloat)
		p.Timestamp = start.Add(time.Duration(i) * time.Second)
		data, _ := json.Marshal(p)
		if _, err := js.PublishAsync(model.DataSubject("target", "temp"), data); err != nil {
			b.Fatal(err)
		}
		if i%10 == 0 { // 干扰序列
			o := model.NewPoint("temp", fmt.Sprintf("other-%d", i%7), 1.0, model.TypeFloat)
			od, _ := json.Marshal(o)
			js.PublishAsync(model.DataSubject(o.DeviceID, "temp"), od)
		}
	}
	<-js.PublishAsyncComplete()
	b.ResetTimer()
	var worst time.Duration
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		res, err := store.QueryRaw(context.Background(), RawQuery{DeviceID: "target", Key: "temp", Start: start, End: end, Limit: 1000})
		if err != nil {
			b.Fatal(err)
		}
		if res.Matched != n || len(res.Points) > 1000 || res.Truncated {
			b.Fatalf("结果错误: matched=%d points=%d truncated=%v", res.Matched, len(res.Points), res.Truncated)
		}
		if d := time.Since(t0); d > worst {
			worst = d
		}
	}
	b.ReportMetric(float64(worst.Milliseconds()), "worst_ms")
}
