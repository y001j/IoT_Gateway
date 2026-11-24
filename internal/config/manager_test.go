package config

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConfigManagerLoadGetValidate(t *testing.T) {
	path := writeConfig(t, "gateway:\n  id: gw-9\n  http_port: 9090\n  log_level: debug\nmodbus:\n  name: m1\n  type: modbus\n  host: 10.0.0.1\n  registers:\n    - type: coil\n      device_id: d\n      key: k\n")
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("合法配置校验失败: %v", err)
	}
	if m.Get("gateway.id") != "gw-9" || m.Get("web_ui.port") != 8081 {
		t.Fatalf("取值或默认值错误: %v %v", m.Get("gateway.id"), m.Get("web_ui.port"))
	}
	var gw struct {
		ID       string `json:"id"`
		HTTPPort int    `json:"http_port"`
	}
	if err := m.GetAs("gateway", &gw); err != nil || gw.HTTPPort != 9090 {
		t.Fatalf("GetAs 失败: %v %+v", err, gw)
	}
	// ParseFromManager：从配置管理器解析并校验结构体
	cfg, err := NewParserWithDefaults(GetDefaultModbusConfig()).ParseFromManager(m, "modbus")
	if err != nil || cfg.Host != "10.0.0.1" || len(cfg.Registers) != 1 || cfg.Port != 502 {
		t.Fatalf("ParseFromManager 失败: %v %+v", err, cfg)
	}
	if m.(*Manager).GetViper() == nil {
		t.Fatalf("GetViper 不应为 nil")
	}
}

func TestConfigManagerValidationErrors(t *testing.T) {
	for name, content := range map[string]string{
		"端口越界":  "gateway:\n  id: g\n  http_port: 70000\n",
		"日志级别":  "gateway:\n  id: g\n  log_level: verbose\n",
		"缺少ID":  "gateway:\n  id: \"\"\n",
		"web端口": "gateway:\n  id: g\nweb_ui:\n  port: 0\n",
	} {
		t.Run(name, func(t *testing.T) {
			m, _ := NewManager(writeConfig(t, content))
			if err := m.Load(); err == nil {
				if err := m.Validate(); err == nil {
					t.Fatalf("应校验失败")
				}
			}
		})
	}
	m, _ := NewManager(filepath.Join(t.TempDir(), "missing.yaml"))
	if err := m.Load(); err == nil {
		t.Fatalf("不存在的配置文件应加载失败")
	}
}

func TestConfigManagerHotReload(t *testing.T) {
	path := writeConfig(t, "gateway:\n  id: gw-1\n")
	m, _ := NewManager(path)
	m.Load()
	var called int32
	var last atomic.Value
	if err := m.Hot().AddWatcher("gateway.id", func(v interface{}) {
		atomic.AddInt32(&called, 1)
		last.Store(v)
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.Hot().Enable(); err != nil {
		t.Fatal(err)
	}
	if !m.Hot().IsEnabled() {
		t.Fatalf("应已启用热加载")
	}
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(path, []byte("gateway:\n  id: gw-2\n"), 0o644)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&called) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if atomic.LoadInt32(&called) == 0 || last.Load() != "gw-2" {
		t.Fatalf("配置变更后应通知 watcher, called=%d last=%v", called, last.Load())
	}
	m.Hot().Disable()
	if m.Hot().IsEnabled() {
		t.Fatalf("应已禁用热加载")
	}
}
