package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/testutil"
)

// TestRuntimeWithWebUI 开启 Web 界面和规则引擎启动整个网关：Web 服务使用规则引擎的规则管理器，
// 通过真实登录访问受保护的接口，并检查运行时指标更新。
func TestRuntimeWithWebUI(t *testing.T) {
	srv, _ := testutil.StartNATS(t)
	dir := t.TempDir()
	t.Chdir(dir) // Web 服务在当前目录下创建 ./data
	t.Setenv("IOT_GATEWAY_DATA_DIR", filepath.Join(dir, "data"))
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "runtime-web-pass")
	rulesDir := filepath.Join(dir, "rules")
	os.MkdirAll(rulesDir, 0o755)
	rule := `[{"id":"hot","name":"高温","enabled":true,"conditions":{"field":"value","operator":"gt","value":100},
		"actions":[{"type":"filter","config":{"type":"range","parameters":{"min":0,"max":1000}}}]}]`
	os.WriteFile(filepath.Join(rulesDir, "r.json"), []byte(rule), 0o644)

	gwPort, webPort := freeTCPPort(t), freeTCPPort(t)
	cfg := fmt.Sprintf(`
gateway:
  name: test
  log_level: error
  nats_url: %q
  http_port: %d
  plugins_dir: %q
rule_engine:
  enabled: true
  rules_dir: %q
  hot_reload:
    enabled: false
southbound:
  adapters: []
northbound:
  sinks: []
web_ui:
  enabled: true
  port: %d
  auth:
    jwt_secret: "runtime-web-secret-0123456789"
    token_duration: 1h
    password_min_length: 6
    bcrypt_cost: 4
database:
  sqlite:
    path: %q
`, srv.ClientURL(), gwPort, filepath.Join(dir, "plugins"), rulesDir, webPort, filepath.Join(dir, "auth.db"))
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(cfg), 0o644)

	rt, err := NewRuntime(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())

	base := fmt.Sprintf("http://127.0.0.1:%d", webPort)
	testutil.WaitFor(t, 5*time.Second, func() bool {
		resp, err := http.Get(base + "/health")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == 200
	}, "Web 服务未启动")

	// 运维接口也由 Web 端口提供，/health 返回真实的组件状态（以前固定返回 ok）
	for _, path := range []string{"/health", "/metrics", "/info"} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&m)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("Web 端口的 %s 应可访问: %d", path, resp.StatusCode)
		}
		if path == "/health" && m["components"] == nil {
			t.Fatalf("Web 端口的 /health 应返回组件状态: %v", m)
		}
	}

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "runtime-web-pass"})
	resp, err := http.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var login struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&login)
	resp.Body.Close()
	if login.Data.Token == "" {
		t.Fatalf("登录失败: %d", resp.StatusCode)
	}

	// Web 服务的规则接口应读到规则引擎加载的规则
	req, _ := http.NewRequest("GET", base+"/api/v1/plugins/rules/hot", nil)
	req.Header.Set("Authorization", "Bearer "+login.Data.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Web 规则接口应能读取规则引擎的规则, got %d", resp.StatusCode)
	}

	// 运行时指标
	rt.UpdateMetrics()
	if m := rt.GetMetrics(); m == nil || m.GetMetrics() == nil {
		t.Fatal("运行时应提供指标")
	}
	resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", gwPort))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("/metrics = %d", resp.StatusCode)
	}
}

func TestStorageDirAndDisabledWebService(t *testing.T) {
	v := viper.New()
	if got := storageDir(v); got != "./data/jetstream" {
		t.Fatalf("默认存储目录错误: %s", got)
	}
	v.Set("storage.store_dir", "/var/lib/iot")
	if got := storageDir(v); got != "/var/lib/iot" {
		t.Fatalf("自定义存储目录错误: %s", got)
	}

	// 未启用 Web 界面时 Start/Stop 无副作用
	v.Set("web_ui.enabled", false)
	ws, err := NewWebService(v, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Name() != "WebService" || ws.Init(nil) != nil {
		t.Fatal("名称或 Init 错误")
	}
	ws.SetRuleEngineService(nil)
	if err := ws.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ws.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestOpsPort(t *testing.T) {
	for _, c := range []struct {
		in     string
		webOps bool
		want   string
	}{{"", true, "8080"}, {"9090", true, "9090"}, {"0", true, ""}, {"0", false, "8080"}} {
		if got := opsPort(c.in, c.webOps); got != c.want {
			t.Errorf("opsPort(%q, %v) = %q, want %q", c.in, c.webOps, got, c.want)
		}
	}
}
