package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/model"
	_ "github.com/y001j/iot-gateway/internal/northbound/console"
	"github.com/y001j/iot-gateway/internal/southbound"
	_ "github.com/y001j/iot-gateway/internal/southbound/mock"
)

// startFailAdapter 初始化成功、启动失败
type startFailAdapter struct{}

func (startFailAdapter) Name() string                                    { return "start_fail" }
func (startFailAdapter) Init(json.RawMessage) error                      { return nil }
func (startFailAdapter) Start(context.Context, chan<- model.Point) error { return errors.New("boom") }
func (startFailAdapter) Stop() error                                     { return nil }

func mockAdapterCfg(name string) map[string]interface{} {
	return map[string]interface{}{"name": name, "type": "mock",
		"config": map[string]interface{}{"name": name, "type": "mock", "interval": "50ms",
			"data_points": []interface{}{map[string]interface{}{"device_id": "d", "key": "k", "min_value": 1.0, "max_value": 2.0}}}}
}

// 健康检查按配置的实例统计：没有被配置使用的插件类型不算"未运行"，
// 而初始化失败、启动失败、被停止的实例算作未运行
func TestInstanceStatuses(t *testing.T) {
	southbound.Register("start_fail", func() southbound.Adapter { return startFailAdapter{} })
	t.Cleanup(func() { delete(southbound.Registry, "start_fail") })

	v := viper.New()
	disabled := mockAdapterCfg("mock_off")
	disabled["enabled"] = false
	v.Set("southbound.adapters", []interface{}{
		mockAdapterCfg("mock_a"),
		mockAdapterCfg("mock_b"),
		disabled,
		map[string]interface{}{"name": "unknown_type", "type": "no_such_adapter"},
		map[string]interface{}{"name": "fails_on_start", "type": "start_fail"},
	})
	v.Set("northbound.sinks", []interface{}{
		map[string]interface{}{"name": "console1", "type": "console", "params": map[string]interface{}{}},
		map[string]interface{}{"name": "mqtt_off", "type": "mqtt", "enabled": false},
	})
	m := NewManager(t.TempDir(), nil, v)
	if err := m.Init(nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(context.Background())

	// 格式错误、缺少名称、重名的条目不计入（Init 会拒绝格式错误的配置，这里启动后再加入）
	adapters, _ := v.Get("southbound.adapters").([]interface{})
	v.Set("southbound.adapters", append(adapters, "格式错误的条目",
		map[string]interface{}{"type": "mock"}, mockAdapterCfg("mock_a")))

	got := map[string]InstanceStatus{}
	for _, s := range m.InstanceStatuses() {
		got[s.Name] = s
	}
	want := map[string]InstanceStatus{
		"mock_a":         {Name: "mock_a", Type: "mock", Kind: "adapter", Running: true},
		"mock_b":         {Name: "mock_b", Type: "mock", Kind: "adapter", Running: true},
		"unknown_type":   {Name: "unknown_type", Type: "no_such_adapter", Kind: "adapter", Running: false},
		"fails_on_start": {Name: "fails_on_start", Type: "start_fail", Kind: "adapter", Running: false},
		"console1":       {Name: "console1", Type: "console", Kind: "sink", Running: true},
	}
	if len(got) != len(want) {
		t.Fatalf("实例列表 = %+v", got)
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %+v, want %+v", name, got[name], w)
		}
	}

	// 停止的实例显示为未运行
	if err := m.StopPlugin("mock_b"); err != nil {
		t.Fatal(err)
	}
	for _, s := range m.InstanceStatuses() {
		if s.Name == "mock_b" && s.Running {
			t.Fatal("停止后应为未运行")
		}
	}

	// 不在配置中但正在运行的实例（例如外部插件）也会列出
	m.mu.Lock()
	m.adapters["ext"] = startFailAdapter{}
	m.mu.Unlock()
	found := false
	for _, s := range m.InstanceStatuses() {
		if s.Name == "ext" {
			found = s.Running && s.Kind == "adapter"
		}
	}
	if !found {
		t.Fatal("应列出不在配置中的运行实例")
	}
	m.mu.Lock()
	delete(m.adapters, "ext")
	m.mu.Unlock()
}

// Stop 关闭文件监听器后，后台扫描协程应退出（以前在已关闭的通道上空转）
func TestLoopExitsWhenWatcherClosed(t *testing.T) {
	m := NewManager(t.TempDir(), nil, viper.New())
	if err := m.Init(nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = m.loop(context.Background())
		close(done)
	}()
	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("监听器关闭后 loop 应退出")
	}
}
