package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/model"
	_ "github.com/y001j/iot-gateway/internal/northbound/console"
)

func TestLoaderBuiltinGoAndErrorPaths(t *testing.T) {
	dir := t.TempDir()
	l := NewLoader(dir)

	// 内置插件按注册表创建（以前适配器只支持写死的 mock）
	if err := l.LoadPlugin(Meta{Name: "m1", Type: "adapter", Entry: "builtin://mock"}); err != nil {
		t.Fatal(err)
	}
	if err := l.LoadPlugin(Meta{Name: "c1", Type: "sink", Entry: "builtin://console"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := l.GetAdapter("m1"); !ok {
		t.Fatal("应能按配置名获取适配器")
	}
	if _, ok := l.GetAdapter("mock"); !ok {
		t.Fatal("应能按内置名获取适配器")
	}
	if _, ok := l.GetSink("c1"); !ok {
		t.Fatal("应能获取连接器")
	}
	if names := l.ListAdapters(); len(names) != 2 {
		t.Fatalf("ListAdapters = %v", names)
	}
	if err := l.Unload("m1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := l.GetAdapter("m1"); ok {
		t.Fatal("卸载后不应再能获取")
	}
	if _, ok := l.GetAdapter("nope"); ok {
		t.Fatal("不存在的适配器")
	}

	// .so 文件无法打开（不是有效插件，或当前构建不支持 Go 插件）
	so := filepath.Join(dir, "x.so")
	if err := os.WriteFile(so, []byte("not a plugin"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, meta := range map[string]Meta{
		"未知类型":     {Name: "a", Type: "weird", Entry: "builtin://mock"},
		"内置路径无名称":  {Name: "a", Type: "adapter", Entry: "builtin://"},
		"未知内置适配器":  {Name: "a", Type: "adapter", Entry: "builtin://nope"},
		"未知内置连接器":  {Name: "a", Type: "sink", Entry: "builtin://nope"},
		"文件不存在":    {Name: "a", Type: "adapter", Entry: "missing.so"},
		"无效 Go 插件": {Name: "a", Type: "adapter", Entry: "x.so"},
		"绝对路径插件":   {Name: "a", Type: "sink", Entry: so},
		"不支持的扩展名":  {Name: "a", Type: "adapter", Entry: "x.txt"},
	} {
		if err := l.LoadPlugin(meta); err == nil {
			t.Errorf("%s 应返回错误", name)
		}
	}
}

// 外部插件（plugin.json）以 builtin:// 包装的形式启动、停止
func TestManagerStartsExternalPlugin(t *testing.T) {
	dir := t.TempDir()
	meta := map[string]interface{}{"name": "ext-mock", "version": "1.0.0", "type": "adapter", "mode": "isp-sidecar", "entry": "builtin://mock"}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	v := viper.New()
	v.Set("southbound.adapters", []interface{}{map[string]interface{}{"name": "ext-mock", "type": "mock",
		"config": map[string]interface{}{"name": "ext-mock", "type": "mock", "interval": "20ms",
			"data_points": []interface{}{map[string]interface{}{"device_id": "x", "key": "k", "min_value": 1.0, "max_value": 2.0}}}}})
	m := NewManager(dir, nil, v)
	m.ctx = context.Background()
	m.dataChan = make(chan model.Point, 100)
	if err := m.loader.Scan(); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.loader.Get("ext-mock"); !ok {
		t.Fatal("应扫描到 plugin.json")
	}
	if err := m.StartPlugin("ext-mock"); err != nil {
		t.Fatalf("启动外部插件失败: %v", err)
	}
	select {
	case <-m.dataChan:
	case <-time.After(3 * time.Second):
		t.Fatal("外部插件应产生数据")
	}
	if meta, ok := m.GetPlugin("ext-mock"); !ok || meta.Status != "running" {
		t.Fatalf("GetPlugin = %+v %v", meta, ok)
	}
	if err := m.StopPlugin("ext-mock"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.loader.GetAdapter("ext-mock"); ok {
		t.Fatal("停止后应卸载")
	}
}

func TestISPAdapterProxyReconnect(t *testing.T) {
	sc := newFakeSidecar(t)
	p, err := NewISPAdapterProxy("modbus-sidecar", sc.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	cfg := `{"plugin_config":{"protocol":"tcp","host":"10.0.0.1","port":502,"interval_ms":1000,"registers":[]}}`
	if err := p.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	ch := make(chan model.Point, 10)
	if err := p.Start(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	// sidecar 断开后重连：重新发送配置，新连接上的数据能继续送达
	sc.closeAll()
	if err := p.reconnect(context.Background()); err != nil {
		t.Fatalf("重连失败: %v", err)
	}
	dm, _ := NewDataMessage([]DataPoint{{Key: "temp", Source: "plc", Value: 1.5, Type: "float32"}})
	time.Sleep(50 * time.Millisecond)
	sc.broadcast(dm)
	select {
	case pt := <-ch:
		if pt.Key != "temp" {
			t.Fatalf("重连后收到的数据错误: %+v", pt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("重连后应继续收到数据")
	}

	// sidecar 不可用时重连失败
	sc.ln.Close()
	sc.closeAll()
	if err := p.reconnect(context.Background()); err == nil {
		t.Fatal("sidecar 不可用时重连应失败")
	}
}
