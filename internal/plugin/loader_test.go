package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestLoaderScanAndManagerPlugins(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "modbus-sidecar.json"), []byte(`{"name":"modbus-sidecar","version":"1.0","type":"adapter","mode":"isp-sidecar","entry":"./modbus-sidecar"}`), 0o644)
	os.WriteFile(filepath.Join(dir, "noname.json"), []byte(`{"type":"sink","mode":"builtin"}`), 0o644)
	os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{oops`), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte(`ignored`), 0o644)

	l := NewLoader(dir)
	if err := l.Scan(); err != nil {
		t.Fatal(err)
	}
	if n := len(l.List()); n != 2 {
		t.Fatalf("应发现 2 个插件（跳过损坏文件和非 JSON 文件）, got %d", n)
	}
	m, ok := l.Get("modbus-sidecar")
	if !ok || m.Mode != "isp-sidecar" || m.Type != "adapter" {
		t.Fatalf("插件元数据错误: %+v", m)
	}
	if _, ok := l.Get("noname"); !ok {
		t.Fatalf("缺少 name 时应使用文件名")
	}
	if _, ok := l.Get("broken"); ok {
		t.Fatalf("损坏的元数据不应被加载")
	}
	if err := NewLoader(filepath.Join(dir, "missing")).Scan(); err == nil {
		t.Fatalf("不存在的目录应报错")
	}

	// 管理器：插件列表与状态
	mgr := NewManager(dir, nil, viper.New())
	if err := mgr.Init(nil); err != nil {
		t.Fatal(err)
	}
	plugins := mgr.GetPlugins()
	found := false
	for _, p := range plugins {
		if p.Name == "modbus-sidecar" {
			found = true
			if p.Status == "running" {
				t.Fatalf("未启动的插件状态不应为 running")
			}
		}
	}
	if !found {
		t.Fatalf("管理器插件列表中缺少 modbus-sidecar: %+v", plugins)
	}
	if _, ok := mgr.GetPlugin("modbus-sidecar"); !ok {
		t.Fatalf("GetPlugin 失败")
	}
	if err := mgr.StopPlugin("modbus-sidecar"); err == nil {
		t.Fatalf("停止未运行的插件应报错")
	}
	if _, ok := mgr.GetAdapter("nope"); ok {
		t.Fatalf("不存在的适配器")
	}
}
