package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/testutil"
)

// flakySink 可控制失败的 BatchWriter
type flakySink struct {
	mu        sync.Mutex
	failNext  int   // 接下来多少次写入返回可重试错误
	permanent bool  // 返回永久错误
	down      bool  // Healthy 返回错误
	got       []int // 收到的数据点 value（按顺序）
	writes    int
}

func (f *flakySink) Name() string                  { return "flaky" }
func (f *flakySink) Init(json.RawMessage) error    { return nil }
func (f *flakySink) Start(context.Context) error   { return nil }
func (f *flakySink) Stop() error                   { return nil }
func (f *flakySink) Publish(b []model.Point) error { return f.WriteBatch(context.Background(), b) }
func (f *flakySink) Healthy() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return errors.New("target down")
	}
	return nil
}
func (f *flakySink) WriteBatch(_ context.Context, b []model.Point) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if f.permanent {
		return fmt.Errorf("%w: bad data", northbound.ErrPermanent)
	}
	if f.failNext > 0 {
		f.failNext--
		return errors.New("connection refused")
	}
	for _, p := range b {
		v, _ := northbound.ToInt64(p.Value)
		f.got = append(f.got, int(v))
	}
	return nil
}
func (f *flakySink) values() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.got...)
}
func (f *flakySink) set(fn func(*flakySink)) {
	f.mu.Lock()
	fn(f)
	f.mu.Unlock()
}

func publishPoints(t *testing.T, nc *nats.Conn, from, to int) {
	t.Helper()
	for i := from; i < to; i++ {
		p := model.NewPoint("k", "dev", i, model.TypeInt)
		data, _ := json.Marshal(p)
		if err := nc.Publish(model.DataSubject("dev", "k"), data); err != nil {
			t.Fatal(err)
		}
	}
	nc.Flush()
}

func fastCfg() BufferConfig {
	return BufferConfig{Enabled: true, BatchSize: 10, MaxWait: 50 * time.Millisecond, WriteTimeout: time.Second,
		BackoffMin: 20 * time.Millisecond, BackoffMax: 100 * time.Millisecond}
}

func newStore(t *testing.T, nc *nats.Conn, mutate func(*Config)) *Store {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.MaxBytes = 64 << 20
	cfg.MaxDisk = 512 << 20
	if mutate != nil {
		mutate(&cfg)
	}
	s, err := New(nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func assertSequence(t *testing.T, got []int, n int) {
	t.Helper()
	if len(got) != n {
		t.Fatalf("应收到 %d 条, got %d: %v", n, len(got), got)
	}
	for i, v := range got {
		if v != i {
			t.Fatalf("顺序错误或有丢失/重复: 第 %d 条为 %d (%v)", i, v, got)
		}
	}
}

func TestBufferedSinkSurvivesOutageInOrder(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	store := newStore(t, nc, nil)
	sink := &flakySink{failNext: 5}
	r, err := store.StartSinkRunner(context.Background(), "influx", sink, fastCfg())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	publishPoints(t, nc, 0, 50)
	testutil.WaitFor(t, 10*time.Second, func() bool { return len(sink.values()) == 50 }, "目标恢复后应补发全部数据")
	assertSequence(t, sink.values(), 50)
	st := r.Stats()
	if st.Delivered != 50 || st.Failed < 5 || st.Lost != 0 {
		t.Fatalf("统计错误: %+v", st)
	}
	testutil.WaitFor(t, 3*time.Second, func() bool { return r.Stats().Pending == 0 && r.Stats().State == StateRealtime }, "补发完成后应回到实时状态")
}

func TestBufferedSinkResumesAfterRestart(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	store := newStore(t, nc, nil)
	sink := &flakySink{}
	r, err := store.StartSinkRunner(context.Background(), "cloud", sink, fastCfg())
	if err != nil {
		t.Fatal(err)
	}
	publishPoints(t, nc, 0, 20)
	testutil.WaitFor(t, 5*time.Second, func() bool { return len(sink.values()) == 20 }, "应写入前 20 条")
	r.Stop()

	// 网关停止期间到达的数据保留在流中
	publishPoints(t, nc, 20, 35)
	r2, err := store.StartSinkRunner(context.Background(), "cloud", sink, fastCfg())
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Stop()
	testutil.WaitFor(t, 5*time.Second, func() bool { return len(sink.values()) == 35 }, "重启后应从断点继续")
	assertSequence(t, sink.values(), 35) // 已确认的数据不重复
}

func TestBufferedSinkHealthGatingAndLossAccounting(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	store := newStore(t, nc, func(c *Config) { c.MaxMsgs = 10 })
	sink := &flakySink{down: true}
	r, err := store.StartSinkRunner(context.Background(), "edge", sink, fastCfg())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	publishPoints(t, nc, 0, 30) // 只保留最近 10 条
	time.Sleep(200 * time.Millisecond)
	if st := r.Stats(); st.State != StateBuffering {
		t.Fatalf("目标不健康时应处于缓冲状态: %+v", st)
	}
	sink.set(func(f *flakySink) {
		if f.writes != 0 {
			t.Errorf("目标不健康时不应尝试写入")
		}
		f.down = false
	})
	testutil.WaitFor(t, 5*time.Second, func() bool { return len(sink.values()) == 10 }, "恢复后应写入保留的 10 条")
	got := sink.values()
	if got[0] != 20 || got[9] != 29 {
		t.Fatalf("应收到最近的 10 条（20..29）: %v", got)
	}
}

func TestBufferedSinkLostCountWhenEvicted(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	store := newStore(t, nc, func(c *Config) { c.MaxMsgs = 5 })
	sink := &flakySink{}
	r, err := store.StartSinkRunner(context.Background(), "s", sink, fastCfg())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	publishPoints(t, nc, 0, 3)
	testutil.WaitFor(t, 3*time.Second, func() bool { return len(sink.values()) == 3 }, "先正常写入 3 条")
	sink.set(func(f *flakySink) { f.down = true })
	time.Sleep(100 * time.Millisecond)
	publishPoints(t, nc, 3, 20) // 17 条，只保留 5 条 → 12 条在写入前被淘汰
	time.Sleep(200 * time.Millisecond)
	if st := r.Stats(); st.Lost != 12 {
		t.Fatalf("被淘汰的消息数应为 12, got %+v", st)
	}
}

func TestBufferedSinkPermanentErrorDiscards(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	store := newStore(t, nc, nil)
	sink := &flakySink{permanent: true}
	r, err := store.StartSinkRunner(context.Background(), "p", sink, fastCfg())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	publishPoints(t, nc, 0, 5)
	testutil.WaitFor(t, 3*time.Second, func() bool { return r.Stats().Discarded == 5 }, "永久错误应丢弃数据而不是无限重试")
	sink.set(func(f *flakySink) { f.permanent = false })
	publishPoints(t, nc, 5, 8)
	testutil.WaitFor(t, 3*time.Second, func() bool { return len(sink.values()) == 3 }, "后续数据不应被阻塞")
}

func TestBufferedSinkSkipsRuleOutputsAndGarbage(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	store := newStore(t, nc, nil)
	sink := &flakySink{}
	r, _ := store.StartSinkRunner(context.Background(), "x", sink, fastCfg())
	defer r.Stop()
	msg := nats.NewMsg("iot.data.copy.k")
	msg.Header.Set("Iot-Origin", "rule-engine")
	msg.Data, _ = json.Marshal(model.NewPoint("k", "copy", 99, model.TypeInt))
	nc.PublishMsg(msg)
	nc.Publish("iot.data.bad.k", []byte("not json"))
	publishPoints(t, nc, 0, 1)
	testutil.WaitFor(t, 3*time.Second, func() bool { return len(sink.values()) == 1 }, "应只收到原始数据")
	time.Sleep(100 * time.Millisecond)
	if got := sink.values(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("规则输出不应写入缓冲 Sink: %v", got)
	}
	if r.Stats().Discarded != 1 {
		t.Fatalf("无法解析的消息应计入丢弃: %+v", r.Stats())
	}
}

func TestConfigAndHelpers(t *testing.T) {
	for in, want := range map[string]int64{"512MB": 512 << 20, "2GB": 2 << 30, "1.5K": 1536, "100": 100, "1tb": 1 << 40} {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "abc", "-1GB"} {
		if _, err := ParseSize(bad); err == nil {
			t.Errorf("ParseSize(%q) 应报错", bad)
		}
	}
	v := viper.New()
	v.Set("storage.enabled", true)
	v.Set("storage.max_disk", "1GB")
	v.Set("storage.raw.max_bytes", "256MB")
	v.Set("storage.raw.max_age", "6h")
	v.Set("storage.raw.compression", "none")
	cfg, err := ConfigFromViper(v)
	if err != nil || !cfg.Enabled || cfg.MaxBytes != 256<<20 || cfg.MaxAge != 6*time.Hour || cfg.Compression {
		t.Fatalf("配置解析错误: %+v %v", cfg, err)
	}
	v.Set("storage.raw.max_bytes", "2GB")
	if _, err := ConfigFromViper(v); err == nil {
		t.Fatalf("max_bytes 大于 max_disk 应报错")
	}
	bc := ParseBufferConfig(map[string]interface{}{"enabled": true, "batch_size": 50.0, "max_wait": "2s", "start_from": "all"})
	if !bc.Enabled || bc.BatchSize != 50 || bc.MaxWait != 2*time.Second || bc.StartFrom != "all" {
		t.Fatalf("缓冲配置解析错误: %+v", bc)
	}
	if DurableName("my sink.1") != "sink_my_sink_1" {
		t.Fatalf("DurableName = %s", DurableName("my sink.1"))
	}
	if _, err := New(nil, DefaultConfig()); err == nil {
		t.Fatalf("nil 连接应报错")
	}
}

func TestStoreStatusAndStartFromAll(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	store := newStore(t, nc, nil)
	publishPoints(t, nc, 0, 7)
	// 核心 NATS 发布写入流是异步的，等待全部落盘
	testutil.WaitFor(t, 3*time.Second, func() bool {
		st, err := store.Status()
		return err == nil && st.Messages == 7
	}, "7 条数据应写入流")
	st, _ := store.Status()
	if st.Stream != "IOT_RAW" || st.Usage <= 0 {
		t.Fatalf("流状态错误: %+v", st)
	}
	// 再次 New 会更新而不是重建流
	if _, err := New(nc, store.Config()); err != nil {
		t.Fatal(err)
	}
	sink := &flakySink{}
	cfg := fastCfg()
	cfg.StartFrom = "all"
	r, err := store.StartSinkRunner(context.Background(), "late", sink, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	testutil.WaitFor(t, 3*time.Second, func() bool { return len(sink.values()) == 7 }, "start_from=all 应补写流中已有数据")
}
