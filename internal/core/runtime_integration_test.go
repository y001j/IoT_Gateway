package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/y001j/iot-gateway/internal/testutil"

	_ "github.com/y001j/iot-gateway/internal/northbound/console"
	_ "github.com/y001j/iot-gateway/internal/southbound/mock"
)

func freeTCPPort(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestRuntimeEndToEnd 用真实配置文件启动整个网关：mock 适配器 → 管道 → NATS → 规则引擎 → 告警，
// 并验证 /health、动作处理器注册和优雅停止。
func TestRuntimeEndToEnd(t *testing.T) {
	srv, nc := testutil.StartNATS(t)
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	os.MkdirAll(rulesDir, 0o755)
	rule := map[string]interface{}{
		"id": "always", "name": "总是告警", "enabled": true,
		"conditions": map[string]interface{}{"field": "key", "operator": "eq", "value": "temperature"},
		"actions": []interface{}{
			map[string]interface{}{"type": "filter", "config": map[string]interface{}{"type": "range", "parameters": map[string]interface{}{"min": -1000.0, "max": 1000.0}}},
			map[string]interface{}{"type": "alert", "config": map[string]interface{}{"level": "warning", "message": "温度 {{.Value}}"}},
		},
	}
	data, _ := json.Marshal(rule)
	os.WriteFile(filepath.Join(rulesDir, "always.json"), data, 0o644)

	port := freeTCPPort(t)
	cfg := fmt.Sprintf(`
gateway:
  name: test
  log_level: error
  nats_url: %q
  http_port: %d
  plugins_dir: %q
  pipeline:
    batch_size: 10
    flush_interval: 20ms
rule_engine:
  enabled: true
  rules_dir: %q
  subject: "iot.data.>"
  hot_reload:
    enabled: false
southbound:
  adapters:
    - name: mock1
      type: mock
      config:
        name: mock1
        type: mock
        interval: 50ms
        data_points:
          - device_id: sensor-1
            key: temperature
            min_value: 20
            max_value: 30
northbound:
  sinks:
    - name: console1
      type: console
      enabled: true
      params: {}
web_ui:
  enabled: false
`, srv.ClientURL(), port, filepath.Join(dir, "plugins"), rulesDir)
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(cfg), 0o644)

	alerts, _ := nc.SubscribeSync("iot.alerts.triggered")
	dataSub, _ := nc.SubscribeSync("iot.data.>")
	nc.Flush()

	rt, err := NewRuntime(cfgPath)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := rt.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, err := dataSub.NextMsg(5 * time.Second); err != nil {
		t.Fatalf("mock 适配器的数据未发布到 iot.data: %v", err)
	}
	msg, err := alerts.NextMsg(5 * time.Second)
	if err != nil {
		t.Fatalf("规则引擎未产生告警（filter 未拦截范围内数据，alert 应执行）: %v", err)
	}
	var alert map[string]interface{}
	json.Unmarshal(msg.Data, &alert)
	if alert["rule_id"] != "always" {
		t.Fatalf("告警内容错误: %v", alert)
	}

	// /health 返回真实状态
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("/health 不可访问: %v", err)
	}
	var report HealthReport
	json.NewDecoder(resp.Body).Decode(&report)
	resp.Body.Close()
	if resp.StatusCode != 200 || report.Components["nats"].Status != "healthy" || report.Components["runtime"].Status != "healthy" {
		t.Fatalf("健康状态错误: %d %+v", resp.StatusCode, report)
	}

	rt.Stop(ctx)
	if rt.Bus.Status() != nats.CLOSED {
		t.Fatalf("停止后 NATS 连接应关闭")
	}
}
