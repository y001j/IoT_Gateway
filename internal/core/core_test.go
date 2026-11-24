package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/testutil"
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
