package services

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/testutil"
	"github.com/y001j/iot-gateway/internal/web/models"
)

// 插件页面按实例列出，列表中的名称可以直接用于启停（以前列出的是插件类型，启停全部失败）
func TestPluginServiceListsControllableInstances(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	v := viper.New()
	v.Set("southbound.adapters", []interface{}{map[string]interface{}{
		"name": "temp_sensors", "type": "mock",
		"config": map[string]interface{}{"name": "temp_sensors", "type": "mock", "interval": "50ms",
			"data_points": []interface{}{map[string]interface{}{"device_id": "s1", "key": "temp", "min_value": 1.0, "max_value": 2.0}}},
	}})
	v.Set("northbound.sinks", []interface{}{
		map[string]interface{}{"name": "console1", "type": "console", "params": map[string]interface{}{}},
		map[string]interface{}{"name": "mqtt_out", "type": "mqtt", "enabled": false,
			"params": map[string]interface{}{"broker": "tcp://127.0.0.1:1", "password": "s3cret", "token": ""}},
	})
	pm := plugin.NewManager(t.TempDir(), nc, v)
	if err := pm.Init(nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pm.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer pm.Stop(context.Background())

	svc, err := NewPluginService(pm)
	if err != nil {
		t.Fatal(err)
	}
	list, total, err := svc.GetPlugins(&models.PluginListRequest{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	// 只列出实例，不再列出 redis、influxdb 等没有被配置使用的类型
	if total != 3 || len(list) != 3 {
		t.Fatalf("应列出 3 个实例: %+v", list)
	}
	byName := map[string]models.Plugin{}
	for _, p := range list {
		byName[p.Name] = p
	}
	if p := byName["temp_sensors"]; p.Type != "adapter" || p.PluginType != "mock" || p.Status != "running" || !p.Enabled || p.Mode != "builtin" {
		t.Fatalf("temp_sensors = %+v", p)
	}
	if p := byName["console1"]; p.Type != "sink" || p.Status != "running" {
		t.Fatalf("console1 = %+v", p)
	}
	if p := byName["mqtt_out"]; p.Status != "stopped" || p.Enabled || p.Description != "mqtt 连接器（配置中已禁用）" {
		t.Fatalf("mqtt_out = %+v", p)
	}
	if list[0].Type != "adapter" {
		t.Fatal("适配器应排在连接器前面")
	}
	if sinks, n, _ := svc.GetPlugins(&models.PluginListRequest{Page: 1, PageSize: 50, Type: "sink"}); n != 2 || len(sinks) != 2 {
		t.Fatalf("按类型过滤: %+v", sinks)
	}

	// 用列表中的名称启停
	status := func(name string) string {
		p, err := svc.GetPlugin(name)
		if err != nil {
			t.Fatal(err)
		}
		return p.Status
	}
	if err := svc.StopPlugin("temp_sensors"); err != nil {
		t.Fatalf("停止失败: %v", err)
	}
	if status("temp_sensors") != "stopped" {
		t.Fatal("停止后应为 stopped")
	}
	if err := svc.StartPlugin("temp_sensors"); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if status("temp_sensors") != "running" {
		t.Fatal("启动后应为 running")
	}
	if err := svc.RestartPlugin("console1"); err != nil {
		t.Fatalf("重启失败: %v", err)
	}
	if status("console1") != "running" {
		t.Fatal("重启后应为 running")
	}
	if _, err := svc.GetPlugin("mock"); err == nil {
		t.Fatal("类型名不是实例，应返回不存在")
	}

	// 配置接口返回实例配置，并隐藏敏感字段
	cfg, err := svc.GetPluginConfig("mqtt_out")
	if err != nil {
		t.Fatal(err)
	}
	params := cfg["params"].(map[string]interface{})
	if params["password"] != "******" || params["broker"] != "tcp://127.0.0.1:1" || params["token"] != "" {
		t.Fatalf("敏感字段应被隐藏: %+v", params)
	}
	if orig, _ := pm.InstanceConfig("mqtt_out"); orig["params"].(map[string]interface{})["password"] != "s3cret" {
		t.Fatal("隐藏敏感字段不应修改原配置")
	}
	if _, err := svc.GetPluginConfig("nope"); err == nil {
		t.Fatal("不存在的插件应报错")
	}
	// 统计接口按实例名工作
	if st, err := svc.GetPluginStats("temp_sensors"); err != nil || st == nil {
		t.Fatalf("GetPluginStats: %v", err)
	}
}

func TestMaskSecretsNested(t *testing.T) {
	cfg := map[string]interface{}{
		"api_key": "k", "Private_Key": "p", "list": []interface{}{map[string]interface{}{"db_password": "x", "host": "h"}},
		"auth": map[string]interface{}{"client_secret": "y", "credentials": 3},
	}
	maskSecrets(cfg)
	if cfg["api_key"] != "******" || cfg["Private_Key"] != "******" ||
		cfg["list"].([]interface{})[0].(map[string]interface{})["db_password"] != "******" ||
		cfg["list"].([]interface{})[0].(map[string]interface{})["host"] != "h" ||
		cfg["auth"].(map[string]interface{})["client_secret"] != "******" ||
		cfg["auth"].(map[string]interface{})["credentials"] != "******" {
		t.Fatalf("maskSecrets = %+v", cfg)
	}
}
