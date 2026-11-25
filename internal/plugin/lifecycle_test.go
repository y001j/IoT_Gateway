package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/testutil"

	_ "github.com/y001j/iot-gateway/internal/northbound/console"
)

func lifecycleViper() *viper.Viper {
	v := viper.New()
	v.Set("gateway.pipeline.batch_size", 5)
	v.Set("gateway.pipeline.flush_interval", "10ms")
	v.Set("southbound.adapters", []interface{}{map[string]interface{}{
		"name": "mock1", "type": "mock",
		"config": map[string]interface{}{"name": "mock1", "type": "mock", "interval": "20ms",
			"data_points": []interface{}{map[string]interface{}{"device_id": "sensor-1", "key": "temp", "min_value": 1.0, "max_value": 2.0}}},
	}})
	v.Set("northbound.sinks", []interface{}{
		map[string]interface{}{"name": "console1", "type": "console", "params": map[string]interface{}{}},
		map[string]interface{}{"name": "off", "type": "console", "enabled": false},
	})
	return v
}

func TestManagerLifecycleAndPluginControl(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	m := NewManager(t.TempDir(), nc, lifecycleViper())
	if m.Name() == "" {
		t.Fatal("应有名称")
	}
	if err := m.Init(nil); err != nil {
		t.Fatal(err)
	}
	dataSub, _ := nc.SubscribeSync("iot.data.>")
	nc.Flush()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(context.Background())

	if _, err := dataSub.NextMsg(5 * time.Second); err != nil {
		t.Fatalf("适配器数据应发布到 NATS: %v", err)
	}
	if _, ok := m.GetAdapter("mock1"); !ok {
		t.Fatal("应按实例名找到适配器")
	}
	if _, ok := m.GetAdapterInterface("mock1"); !ok {
		t.Fatal("GetAdapterInterface 失败")
	}
	if _, ok := m.GetSink("console1"); !ok {
		t.Fatal("应按实例名找到连接器")
	}
	if _, ok := m.GetSinkInterface("console1"); !ok {
		t.Fatal("GetSinkInterface 失败")
	}
	if _, ok := m.GetSink("off"); ok {
		t.Fatal("未启用的连接器不应初始化")
	}
	if len(m.GetPlugins()) == 0 {
		t.Fatal("应列出插件")
	}

	// 回归：按实例名停止后再启动（以前把实例名当作类型查找，必然失败）
	if err := m.StopPlugin("mock1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.GetAdapter("mock1"); ok {
		t.Fatal("停止后不应再存在")
	}
	if err := m.StopPlugin("mock1"); err == nil {
		t.Fatal("停止未运行的插件应报错")
	}
	if err := m.StartPlugin("mock1"); err != nil {
		t.Fatalf("按实例名启动失败: %v", err)
	}
	if err := m.StartPlugin("mock1"); err == nil {
		t.Fatal("重复启动应报错")
	}
	// 重启后实例仍在运行并继续产生数据
	if err := m.RestartPlugin("mock1"); err != nil {
		t.Fatalf("重启失败: %v", err)
	}
	if _, ok := m.GetAdapter("mock1"); !ok {
		t.Fatal("重启后适配器应在运行")
	}
	for {
		if _, err := dataSub.NextMsg(10 * time.Millisecond); err != nil {
			break // 清空积压
		}
	}
	if _, err := dataSub.NextMsg(5 * time.Second); err != nil {
		t.Fatal("重启后应继续产生数据")
	}

	// 连接器：停止、启动（重新接入数据管道）
	if err := m.StopPlugin("console1"); err != nil {
		t.Fatal(err)
	}
	if err := m.StartPlugin("console1"); err != nil {
		t.Fatalf("按实例名启动连接器失败: %v", err)
	}
	if err := m.StartPlugin("console1"); err == nil {
		t.Fatal("重复启动连接器应报错")
	}
	testutil.WaitFor(t, 3*time.Second, func() bool {
		for _, st := range m.GetSinkDispatchStats() {
			if st.Name == "console1" && st.SentPoints > 0 {
				return true
			}
		}
		return false
	}, "重新启动的连接器应接入数据管道")

	// 没有配置时按类型名启动内置插件
	if err := m.StartPlugin("console"); err != nil {
		t.Fatalf("按类型名启动内置连接器失败: %v", err)
	}
	if err := m.StartPlugin("no-such-plugin"); err == nil {
		t.Fatal("不存在的插件应报错")
	}
	if err := m.RestartPlugin("no-such-plugin"); err == nil {
		t.Fatal("重启不存在的插件应报错")
	}
}

func TestSinkConfigData(t *testing.T) {
	nested, _ := sinkConfigData(map[string]interface{}{"config": map[string]interface{}{"a": 1}}, "n", "t")
	if string(nested) != `{"a":1}` {
		t.Fatalf("旧格式应使用嵌套的 config: %s", nested)
	}
	flat, _ := sinkConfigData(map[string]interface{}{"enabled": true, "params": map[string]interface{}{"x": 1}}, "n", "t")
	if string(flat) != `{"name":"n","params":{"x":1},"type":"t"}` {
		t.Fatalf("新格式应去掉 enabled 并补齐 name/type: %s", flat)
	}
}
