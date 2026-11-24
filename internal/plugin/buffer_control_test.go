package plugin

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/storage"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func TestControlSinkBufferAndFallback(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	m := newTestManager(t, nc)

	// 未注入 store 时配置了 buffer 的连接器退回分发器
	m.bufferConfigs["fallback"] = storage.BufferConfig{Enabled: true}
	fb := &fakeSink{name: "fallback"}
	m.attachSink("fallback", fb)
	if len(m.GetBufferStats()) != 0 {
		t.Fatal("store 为空时不应创建缓冲运行器")
	}
	if err := m.ControlSinkBuffer("fallback", "pause", time.Time{}); !errors.Is(err, ErrSinkNotBuffered) {
		t.Fatalf("未缓冲的连接器应返回 ErrSinkNotBuffered, got %v", err)
	}
	m.detachSink("fallback")

	cfg := storage.DefaultConfig()
	cfg.MaxBytes, cfg.MaxDisk = 64<<20, 512<<20
	store, err := storage.New(nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.SetStore(store)
	if m.GetStore() != store {
		t.Fatal("GetStore 应返回注入的 store")
	}
	m.bufferConfigs["buf"] = storage.ParseBufferConfig(map[string]interface{}{
		"enabled": true, "batch_size": 10.0, "max_wait": "50ms", "backoff_min": "10ms", "backoff_max": "50ms",
	})
	sink := &fakeSink{name: "buf"}
	m.attachSink("buf", sink)
	defer m.detachSink("buf")
	if st := m.GetBufferStats(); len(st) != 1 || st[0].Name != "buf" {
		t.Fatalf("应有一个缓冲运行器: %+v", st)
	}

	publish := func(from, to int) {
		for i := from; i < to; i++ {
			data, _ := json.Marshal(model.NewPoint("k", "dev", i, model.TypeInt))
			_ = nc.Publish(model.DataSubject("dev", "k"), data)
		}
		_ = nc.Flush()
	}
	publish(0, 5)
	testutil.WaitFor(t, 5*time.Second, func() bool { return sink.received() == 5 }, "缓冲连接器未收到数据")

	if err := m.ControlSinkBuffer("buf", "pause", time.Time{}); err != nil {
		t.Fatal(err)
	}
	testutil.WaitFor(t, 2*time.Second, func() bool { return m.GetBufferStats()[0].State == storage.StatePaused }, "未暂停")
	publish(5, 10)
	time.Sleep(200 * time.Millisecond)
	if sink.received() != 5 {
		t.Fatalf("暂停期间不应写入: %d", sink.received())
	}
	// 跳过积压：保持暂停，恢复后只收到新数据
	if err := m.ControlSinkBuffer("buf", "skip", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := m.ControlSinkBuffer("buf", "resume", time.Time{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	publish(10, 12)
	testutil.WaitFor(t, 5*time.Second, func() bool { return sink.received() == 7 }, "跳过后新数据未写入")
	// 重放全部 12 条
	if err := m.ControlSinkBuffer("buf", "replay", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	testutil.WaitFor(t, 5*time.Second, func() bool { return sink.received() == 19 }, "重放未完成")

	if err := m.ControlSinkBuffer("buf", "bogus", time.Time{}); err == nil {
		t.Fatal("未知操作应报错")
	}
	if err := m.ControlSinkBuffer("buf", "replay", time.Time{}); err == nil {
		t.Fatal("重放缺少时间应报错")
	}
}
