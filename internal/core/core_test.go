package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/testutil"
	"os"
	"path/filepath"
)

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	srv, nc := testutil.StartNATS(t)
	rt := &Runtime{V: viper.New(), Bus: nc, NatsServer: srv}
	return rt
}

func TestHealthStatus(t *testing.T) {
	rt := newTestRuntime(t)
	if got := rt.HealthStatus(); got.Status != "unhealthy" || got.Components["runtime"].Status != "unhealthy" {
		t.Fatalf("启动完成前应为 unhealthy: %+v", got)
	}
	rt.started.Store(true)
	if got := rt.HealthStatus(); got.Status != "healthy" {
		t.Fatalf("应为 healthy: %+v", got)
	}

	// slow consumer 丢消息后降级
	rt.handleNATSAsyncError(nil, nil, nats.ErrSlowConsumer)
	if got := rt.HealthStatus(); got.Status != "degraded" || got.Components["nats"].Status != "degraded" {
		t.Fatalf("发生 slow consumer 后应为 degraded: %+v", got)
	}
	if rt.slowConsumerDrops.Load() != 1 {
		t.Fatalf("slow consumer 计数错误")
	}
	rt.handleNATSAsyncError(nil, nil, nats.ErrBadSubject)
	if rt.natsAsyncErrors.Load() != 1 {
		t.Fatalf("其他异步错误计数错误")
	}

	// NATS 断开 → unhealthy
	rt.Bus.Close()
	if got := rt.HealthStatus(); got.Status != "unhealthy" || got.Components["nats"].Status != "unhealthy" {
		t.Fatalf("NATS 断开时应为 unhealthy: %+v", got)
	}
}

func TestGatewayHTTPHandler(t *testing.T) {
	rt := newTestRuntime(t)
	rt.V.Set("web_ui.cors.allowed_origins", []string{"http://ui.local"})
	h := rt.GatewayHTTPHandler("8080")

	get := func(path string, headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	// 未启动完成：503
	if w := get("/health", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("/health 未就绪时应返回 503, got %d", w.Code)
	}
	rt.started.Store(true)
	w := get("/health", nil)
	var report HealthReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil || w.Code != 200 || report.Status != "healthy" {
		t.Fatalf("/health = %d %s", w.Code, w.Body.String())
	}

	w = get("/info", map[string]string{"Origin": "http://ui.local"})
	var info map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &info)
	if info["gateway_port"] != "8080" || info["nats_port"] == "N/A" {
		t.Fatalf("/info = %v", info)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "http://ui.local" {
		t.Fatalf("白名单来源应允许跨域")
	}
	w = get("/info", map[string]string{"Origin": "http://evil.com"})
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("非白名单来源不应允许跨域（以前固定为 *）")
	}

	if w := get("/metrics", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("没有指标收集器时 /metrics 应返回 503, got %d", w.Code)
	}
	metrics.InitLightweightMetrics()
	rt.metrics = metrics.GetLightweightMetrics()
	if w := get("/metrics", nil); w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("/metrics json = %d", w.Code)
	}
	if w := get("/metrics?format=text", nil); w.Code != 200 {
		t.Fatalf("/metrics text = %d", w.Code)
	}
	if w := get("/metrics?format=xml", nil); w.Code != 400 {
		t.Fatalf("不支持的格式应返回 400, got %d", w.Code)
	}

	req := httptest.NewRequest("OPTIONS", "/health", nil)
	req.Header.Set("Origin", "http://ui.local")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("预检请求应返回 204, got %d", rec.Code)
	}
}

func TestNATSOptions(t *testing.T) {
	rt := &Runtime{}
	opts := rt.natsOptions(nats.Timeout(1))
	var o nats.Options
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			t.Fatal(err)
		}
	}
	if o.MaxReconnect != -1 || o.AsyncErrorCB == nil || o.Name != "iot-gateway" {
		t.Fatalf("NATS 连接参数错误: max_reconnect=%d", o.MaxReconnect)
	}
}

func TestUpdateRuleEngineMetrics(t *testing.T) {
	dir := t.TempDir()
	rule := `{"id":"r1","name":"r1","enabled":true,"conditions":{"field":"key","value":"temp"},"actions":[{"type":"alert","config":{"message":"m"}}]}`
	if err := os.WriteFile(filepath.Join(dir, "r1.json"), []byte(rule), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]interface{}{"enabled": true, "rules_dir": dir, "hot_reload": map[string]interface{}{"enabled": false}}
	rs := rules.NewRuleEngineServiceWithConfig(cfg)
	if err := rs.Init(cfg); err != nil {
		t.Fatal(err)
	}
	if err := rs.GetRuleManager().LoadRules(); err != nil {
		t.Fatal(err)
	}
	r := &Runtime{metrics: metrics.NewLightweightMetrics(), Svcs: []Service{rs}}
	r.updateRuleEngineMetrics()
	if got := r.metrics.RuleMetrics; got.TotalRules != 1 || got.EnabledRules != 1 {
		t.Fatalf("规则指标未同步: %+v", got)
	}
	(&Runtime{}).updateRuleEngineMetrics() // 没有指标收集器时直接返回
}

// /health 与 /metrics 按配置的实例统计插件：以前按"所有可用插件类型"统计，
// 没有被配置使用的类型（redis、influxdb 等）让网关一直显示 degraded
func TestHealthAndMetricsCountConfiguredInstances(t *testing.T) {
	rt := newTestRuntime(t)
	rt.V.Set("southbound.adapters", []interface{}{map[string]interface{}{"name": "m1", "type": "mock",
		"config": map[string]interface{}{"name": "m1", "type": "mock", "interval": "50ms",
			"data_points": []interface{}{map[string]interface{}{"device_id": "d", "key": "k", "min_value": 1.0, "max_value": 2.0}}}}})
	rt.V.Set("northbound.sinks", []interface{}{
		map[string]interface{}{"name": "c1", "type": "console", "params": map[string]interface{}{}},
		map[string]interface{}{"name": "mq", "type": "mqtt", "enabled": false},
	})
	pm := plugin.NewManager(t.TempDir(), rt.Bus, rt.V)
	if err := pm.Init(nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pm.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer pm.Stop(context.Background())
	rt.PluginMgr = pm
	rt.metrics = metrics.NewLightweightMetrics()
	rt.started.Store(true)

	if got := rt.HealthStatus(); got.Status != "healthy" || got.Components["plugins"].Status != "healthy" {
		t.Fatalf("配置的实例都在运行时应为 healthy: %+v", got)
	}
	rt.UpdateMetrics()
	gm := rt.metrics.GatewayMetrics
	if gm.TotalAdapters != 1 || gm.RunningAdapters != 1 || gm.TotalSinks != 1 || gm.RunningSinks != 1 {
		t.Fatalf("指标应只统计配置的实例: %+v", gm)
	}

	if err := pm.StopPlugin("m1"); err != nil {
		t.Fatal(err)
	}
	got := rt.HealthStatus()
	if got.Status != "degraded" || got.Components["plugins"].Status != "degraded" ||
		!strings.Contains(got.Components["plugins"].Message, "m1") {
		t.Fatalf("配置的实例停止后应为 degraded: %+v", got)
	}
	rt.UpdateMetrics()
	if rt.metrics.GatewayMetrics.RunningAdapters != 0 || rt.metrics.GatewayMetrics.TotalAdapters != 1 {
		t.Fatalf("停止后指标错误: %+v", rt.metrics.GatewayMetrics)
	}
}
