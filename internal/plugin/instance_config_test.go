package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
	_ "github.com/y001j/iot-gateway/internal/southbound/mock"
)

const instanceCfgYAML = `# 网关配置
gateway:
  name: test
southbound:
  adapters:
    # 温度模拟器
    - name: temp
      type: mock
      config:
        interval: 50ms
        data_points:
          - device_id: d1
            key: t
            min_value: 1
            max_value: 2
northbound:
  sinks:
    - name: rec
      type: test_recording
      params:
        password: s-orig
        host: a
    - name: flaky
      type: test_failstart   # 启动时按配置决定是否失败
      params:
        fail: false
`

type failStartSink struct {
	recordingSink
	fail bool
}

func (s *failStartSink) Init(cfg json.RawMessage) error {
	var c struct {
		Params struct {
			Fail bool `json:"fail"`
		} `json:"params"`
	}
	_ = json.Unmarshal(cfg, &c)
	s.fail = c.Params.Fail
	return nil
}
func (s *failStartSink) Start(context.Context) error {
	if s.fail {
		return errors.New("目标不可用")
	}
	return nil
}

func init() {
	northbound.Register("test_failstart", func() northbound.Sink { return &failStartSink{} })
}

func newConfigFileManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(instanceCfgYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	m := NewManager(filepath.Join(dir, "plugins"), nil, v)
	if err := m.Init(nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(context.Background()) })
	return m, path
}

// 页面上修改实例配置：只替换该实例这一项（注释保留），运行中的实例用新配置重启
func TestUpdateInstanceConfig(t *testing.T) {
	m, path := newConfigFileManager(t)

	newCfg := map[string]interface{}{"name": "temp", "type": "mock", "config": map[string]interface{}{
		"interval": "1s", "data_points": []interface{}{map[string]interface{}{"device_id": "d9", "key": "t", "min_value": 1, "max_value": 2}}}}
	restarted, err := m.UpdateInstanceConfig("temp", newCfg)
	if err != nil || !restarted {
		t.Fatalf("保存失败: %v %v", restarted, err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	for _, want := range []string{"# 网关配置", "# 温度模拟器", "d9", "interval: 1s", "# 启动时按配置决定是否失败", "s-orig"} {
		if !strings.Contains(text, want) {
			t.Errorf("配置文件中应有 %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "d1") {
		t.Error("旧配置应被替换")
	}
	if cfg, _ := m.InstanceConfig("temp"); cfg["config"].(map[string]interface{})["interval"] != "1s" {
		t.Fatalf("内存中的配置应更新: %v", cfg)
	}
	if _, ok := m.GetAdapter("temp"); !ok {
		t.Fatal("实例应在重启后运行")
	}

	// 页面读到的配置隐藏了密码，保存时保留原值；停止的实例保存后不重启
	if err := m.StopPlugin("rec"); err != nil {
		t.Fatal(err)
	}
	masked, _ := m.InstanceConfig("rec")
	MaskSecrets(masked)
	masked["params"].(map[string]interface{})["host"] = "b"
	restarted, err = m.UpdateInstanceConfig("rec", masked)
	if err != nil || restarted {
		t.Fatalf("保存失败或不应重启: %v %v", restarted, err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "password: s-orig") || strings.Contains(string(data), SecretMask) || !strings.Contains(string(data), "host: b") {
		t.Fatalf("密码应保留原值:\n%s", data)
	}

	// 不允许的修改与非法配置：文件不变
	before, _ := os.ReadFile(path)
	for name, c := range map[string]struct {
		inst string
		cfg  map[string]interface{}
		want error
	}{
		"改名":      {"temp", map[string]interface{}{"name": "other"}, nil},
		"改类型":     {"temp", map[string]interface{}{"type": "http"}, nil},
		"非法适配器配置": {"temp", map[string]interface{}{"config": map[string]interface{}{"interval": "-1s"}}, nil},
		"非法订阅":    {"rec", map[string]interface{}{"subscriptions": []interface{}{"iot.data.x.>"}}, nil},
		"外部插件":    {"nope", map[string]interface{}{}, ErrInstanceNotConfigured},
	} {
		_, err := m.UpdateInstanceConfig(c.inst, c.cfg)
		if err == nil || (c.want != nil && !errors.Is(err, c.want)) {
			t.Errorf("%s 应失败: %v", name, err)
		}
		if err := m.ValidateInstanceConfig(c.inst, c.cfg); err == nil {
			t.Errorf("%s 校验应失败", name)
		}
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("失败的修改不应改动配置文件")
	}
	if err := m.ValidateInstanceConfig("temp", newCfg); err != nil {
		t.Fatalf("合法配置应通过校验: %v", err)
	}
}

// 新配置无法启动时回滚：配置文件和内存恢复旧配置，实例用旧配置继续运行
func TestUpdateInstanceConfigRollsBack(t *testing.T) {
	m, path := newConfigFileManager(t)
	_, err := m.UpdateInstanceConfig("flaky", map[string]interface{}{"params": map[string]interface{}{"fail": true}})
	if err == nil || !strings.Contains(err.Error(), "已恢复原配置") {
		t.Fatalf("应回滚: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "fail: false") {
		t.Fatalf("配置文件应恢复旧配置:\n%s", data)
	}
	if cfg, _ := m.InstanceConfig("flaky"); cfg["params"].(map[string]interface{})["fail"] != false {
		t.Fatalf("内存中的配置应恢复: %v", cfg)
	}
	if _, ok := m.GetSink("flaky"); !ok {
		t.Fatal("实例应以旧配置重新运行")
	}
}

func TestWriteInstanceToFileErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	cfg := map[string]interface{}{"name": "x"}
	for name, content := range map[string]string{
		"空文件":   "",
		"没有该段":  "gateway: {}\n",
		"不是列表":  "northbound:\n  sinks: {}\n",
		"没有该实例": "northbound:\n  sinks:\n    - name: y\n",
		"格式错误":  "northbound: [\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := writeInstanceToFile(path, "northbound.sinks", "x", cfg); err == nil {
			t.Errorf("%s 应报错", name)
		}
	}
	if err := writeInstanceToFile(filepath.Join(dir, "missing.yaml"), "a", "x", cfg); err == nil {
		t.Error("文件不存在应报错")
	}
	// 没有配置文件的管理器无法保存
	v := viper.New()
	v.Set("southbound.adapters", []interface{}{map[string]interface{}{"name": "m", "type": "mock",
		"config": map[string]interface{}{"interval": "1s", "data_points": []interface{}{map[string]interface{}{"device_id": "d", "key": "k"}}}}})
	m := NewManager(dir, nil, v)
	if _, err := m.UpdateInstanceConfig("m", map[string]interface{}{}); err == nil {
		t.Error("没有配置文件时应报错")
	}
}

func TestRestoreMaskedSecrets(t *testing.T) {
	old := map[string]interface{}{"token": "t1", "list": []interface{}{map[string]interface{}{"password": "p1"}}}
	n := map[string]interface{}{"token": SecretMask, "api_key": SecretMask, "list": []interface{}{map[string]interface{}{"password": SecretMask}, map[string]interface{}{"password": SecretMask}}}
	restoreMaskedSecrets(n, old)
	if n["token"] != "t1" || n["list"].([]interface{})[0].(map[string]interface{})["password"] != "p1" {
		t.Fatalf("应恢复原值: %v", n)
	}
	if _, ok := n["api_key"]; ok {
		t.Fatal("旧配置中没有的敏感字段占位值应删除")
	}
	if _, ok := n["list"].([]interface{})[1].(map[string]interface{})["password"]; ok {
		t.Fatal("新增列表项中的占位值应删除")
	}
	_ = model.Point{}
}
