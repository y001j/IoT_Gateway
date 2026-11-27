package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/southbound"
	"github.com/y001j/iot-gateway/internal/testutil"

	// 注册内置适配器，用于校验仓库中的真实配置文件
	_ "github.com/y001j/iot-gateway/internal/southbound/composite_mock"
	_ "github.com/y001j/iot-gateway/internal/southbound/http"
	_ "github.com/y001j/iot-gateway/internal/southbound/mock"
	_ "github.com/y001j/iot-gateway/internal/southbound/modbus"
)

// fakeSink 记录收到的批次；可选地阻塞、返回错误或修改数据点
type fakeSink struct {
	name    string
	mu      sync.Mutex
	batches [][]model.Point
	block   chan struct{}
	fail    bool
	tag     string // 非空时给每个点加标签（模拟 BaseSink.AddTags）
	calls   int32
}

func (f *fakeSink) Name() string                { return f.name }
func (f *fakeSink) Init(json.RawMessage) error  { return nil }
func (f *fakeSink) Start(context.Context) error { return nil }
func (f *fakeSink) Stop() error                 { return nil }
func (f *fakeSink) Publish(batch []model.Point) error {
	atomic.AddInt32(&f.calls, 1)
	if f.block != nil {
		<-f.block
	}
	if f.tag != "" {
		for i := range batch {
			batch[i].AddTag("sink", f.tag)
		}
	}
	f.mu.Lock()
	f.batches = append(f.batches, batch)
	f.mu.Unlock()
	if f.fail {
		return errors.New("sink failure")
	}
	return nil
}
func (f *fakeSink) received() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, b := range f.batches {
		n += len(b)
	}
	return n
}

func newTestManager(t *testing.T, nc *nats.Conn) *Manager {
	t.Helper()
	v := viper.New()
	v.Set("gateway.pipeline.batch_size", 5)
	v.Set("gateway.pipeline.flush_interval", "10ms")
	v.Set("gateway.pipeline.sink_queue_size", 2)
	return NewManager(t.TempDir(), nc, v)
}

func TestNewManagerPipelineConfig(t *testing.T) {
	m := newTestManager(t, nil)
	if m.batchSize != 5 || m.flushInterval != 10*time.Millisecond || m.sinkQueueSize != 2 {
		t.Fatalf("管道配置未生效: %d %v %d", m.batchSize, m.flushInterval, m.sinkQueueSize)
	}
	d := NewManager(t.TempDir(), nil, nil)
	if d.batchSize != defaultPipelineBatchSize || cap(d.dataChan) != defaultPipelineChannelSize {
		t.Fatalf("默认管道配置错误")
	}
}

func TestDispatchBatchIsolatesSinks(t *testing.T) {
	m := newTestManager(t, nil)
	a := &fakeSink{name: "a", tag: "A"}
	b := &fakeSink{name: "b", tag: "B"}
	m.addDispatcher("a", a)
	m.addDispatcher("b", b)
	defer m.Stop(context.Background())

	p := model.NewPoint("temp", "dev", 1.0, model.TypeFloat)
	p.AddTag("origin", "x")
	batch := []model.Point{p}
	m.dispatchBatch(batch)

	testutil.WaitFor(t, time.Second, func() bool { return a.received() == 1 && b.received() == 1 }, "两个连接器都应收到数据")

	// 每个连接器添加的标签不能出现在其他连接器或原始数据中
	if v, _ := a.batches[0][0].GetTag("sink"); v != "A" {
		t.Fatalf("连接器 a 看到的标签 = %q", v)
	}
	if v, _ := b.batches[0][0].GetTag("sink"); v != "B" {
		t.Fatalf("连接器 b 看到的标签 = %q（被其他连接器污染）", v)
	}
	if _, ok := batch[0].GetTag("sink"); ok {
		t.Fatalf("原始数据被连接器修改")
	}
	if v, _ := b.batches[0][0].GetTag("origin"); v != "x" {
		t.Fatalf("原始标签应被复制给连接器")
	}
}

func TestSlowSinkDoesNotBlockOthers(t *testing.T) {
	m := newTestManager(t, nil)
	slow := &fakeSink{name: "slow", block: make(chan struct{})}
	fast := &fakeSink{name: "fast"}
	m.addDispatcher("slow", slow) // 队列长度 2，很快就会满
	m.dispMu.Lock()
	m.dispatchers["fast"] = newSinkDispatcher("fast", fast, 100)
	m.dispMu.Unlock()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			m.dispatchBatch([]model.Point{model.NewPoint("k", "d", i, model.TypeInt)})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("慢连接器阻塞了数据管道")
	}
	testutil.WaitFor(t, time.Second, func() bool { return fast.received() == 20 }, "快连接器应收到全部数据")

	var slowStats SinkDispatchStats
	for _, st := range m.GetSinkDispatchStats() {
		if st.Name == "slow" {
			slowStats = st
		}
	}
	if slowStats.DroppedBatches == 0 || slowStats.DroppedPoints == 0 {
		t.Fatalf("慢连接器队列满时应计数丢弃: %+v", slowStats)
	}
	close(slow.block)
	m.Stop(context.Background())
}

func TestDispatcherCountsFailuresAndRecoversPanic(t *testing.T) {
	failing := &fakeSink{name: "f", fail: true}
	d := newSinkDispatcher("f", failing, 4)
	d.enqueue([]model.Point{{Key: "k"}})
	testutil.WaitFor(t, time.Second, func() bool { return d.stats().FailedBatches == 1 }, "失败批次应计数")

	panicking := newSinkDispatcher("p", panicSink{}, 4)
	panicking.enqueue([]model.Point{{Key: "k"}})
	testutil.WaitFor(t, time.Second, func() bool { return panicking.stats().FailedBatches == 1 }, "panic 应被恢复并计为失败")

	d.stop(time.Second)
	panicking.stop(time.Second)
	if d.enqueue([]model.Point{{}}) {
		t.Fatalf("停止后不应再接收数据")
	}
	d.stop(time.Second) // 重复停止不应 panic
}

type panicSink struct{}

func (panicSink) Name() string                { return "p" }
func (panicSink) Init(json.RawMessage) error  { return nil }
func (panicSink) Start(context.Context) error { return nil }
func (panicSink) Stop() error                 { return nil }
func (panicSink) Publish([]model.Point) error { panic("boom") }

func TestDispatcherDrainsOnStop(t *testing.T) {
	s := &fakeSink{name: "s"}
	d := newSinkDispatcher("s", s, 100)
	for i := 0; i < 50; i++ {
		d.enqueue([]model.Point{{Key: "k"}})
	}
	d.stop(2 * time.Second)
	if s.received() != 50 {
		t.Fatalf("停止时应写完队列中的数据, got %d", s.received())
	}
}

func TestPipelinePublishesToNATS(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	sub, err := nc.SubscribeSync("iot.data.>")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := nc.SubscribeSync("data.>")
	nc.Flush()

	m := newTestManager(t, nc)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.runPipeline(ctx)

	p := model.NewPoint("temp.c", "plc.1", 21.5, model.TypeFloat)
	m.dataChan <- p

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("规则引擎主题未收到数据: %v", err)
	}
	if msg.Subject != model.DataSubject("plc.1", "temp.c") {
		t.Fatalf("主题 = %q（设备 ID 中的点号必须转义）", msg.Subject)
	}
	var back model.Point
	if err := json.Unmarshal(msg.Data, &back); err != nil || back.DeviceID != "plc.1" {
		t.Fatalf("消息内容错误: %v %+v", err, back)
	}
	// 已移除无人订阅的 data.<sink> 冗余发布
	if _, err := other.NextMsg(100 * time.Millisecond); err == nil {
		t.Fatalf("不应再发布 data.<sink> 主题")
	}
	if st := m.GetPipelineStats(); st.PointsReceived != 1 || st.ChannelCap != cap(m.dataChan) {
		t.Fatalf("管道统计错误: %+v", st)
	}
}

func TestPipelineFlushesByBatchSizeAndInterval(t *testing.T) {
	m := newTestManager(t, nil)
	s := &fakeSink{name: "s"}
	m.addDispatcher("s", s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.runPipeline(ctx)

	for i := 0; i < 5; i++ { // 恰好一批
		m.dataChan <- model.Point{Key: "k", DeviceID: "d", Value: i}
	}
	testutil.WaitFor(t, time.Second, func() bool { return s.received() == 5 }, "满批次应立即发送")
	m.dataChan <- model.Point{Key: "k", DeviceID: "d"}
	testutil.WaitFor(t, time.Second, func() bool { return s.received() == 6 }, "未满批次应在刷新间隔后发送")
	m.Stop(context.Background())
}

func TestAdapterConfigData(t *testing.T) {
	nested := map[string]interface{}{"name": "a", "type": "mock", "enabled": true, "config": map[string]interface{}{"x": 1}}
	data, err := adapterConfigData(nested, "a", "mock")
	if err != nil || string(data) != `{"name":"a","type":"mock","x":1}` {
		t.Fatalf("嵌套配置缺少 name/type 时应补上: %s %v", data, err)
	}
	full := map[string]interface{}{"config": map[string]interface{}{"name": "inner", "type": "mock", "x": 1}}
	if data, _ := adapterConfigData(full, "a", "mock"); string(data) != `{"name":"inner","type":"mock","x":1}` {
		t.Fatalf("嵌套配置已有 name/type 时原样使用: %s", data)
	}
	flat := map[string]interface{}{"name": "a", "enabled": true, "interval": "1s"}
	data, _ = adapterConfigData(flat, "a", "mock")
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	if _, ok := m["enabled"]; ok || m["type"] != "mock" || m["interval"] != "1s" {
		t.Fatalf("新格式配置 = %v", m)
	}
}

// TestRepoConfigAdaptersInit 校验仓库里的配置文件中启用的内置适配器都能通过 Init
// （防止配置校验规则收紧后导致现有配置无法启动）
func TestRepoConfigAdaptersInit(t *testing.T) {
	for _, file := range []string{"../../config.yaml", "../../config_no_hotreload.yaml"} {
		if _, err := os.Stat(file); err != nil {
			continue
		}
		v := viper.New()
		v.SetConfigFile(file)
		if err := v.ReadInConfig(); err != nil {
			t.Fatalf("读取 %s 失败: %v", file, err)
		}
		list, _ := v.Get("southbound.adapters").([]interface{})
		for _, item := range list {
			am, _ := item.(map[string]interface{})
			name, _ := am["name"].(string)
			typ, _ := am["type"].(string)
			if enabled, ok := am["enabled"].(bool); ok && !enabled {
				continue
			}
			adapter, ok := southbound.Create(typ)
			if !ok {
				continue // 外部插件类型
			}
			data, err := adapterConfigData(am, name, typ)
			if err != nil {
				t.Fatal(err)
			}
			if err := adapter.Init(data); err != nil {
				t.Errorf("%s: 适配器 %s(%s) 初始化失败: %v", file, name, typ, err)
			}
		}
	}
}

var _ northbound.Sink = (*fakeSink)(nil)
