package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/southbound"
)

type fakeAdapter struct{ *southbound.BaseAdapter }

func (a *fakeAdapter) Init(json.RawMessage) error                      { return nil }
func (a *fakeAdapter) Start(context.Context, chan<- model.Point) error { return nil }
func (a *fakeAdapter) Stop() error                                     { return nil }

type fakeSink struct{ *northbound.BaseSink }

func (s *fakeSink) Init(json.RawMessage) error  { return nil }
func (s *fakeSink) Start(context.Context) error { return nil }
func (s *fakeSink) Stop() error                 { return nil }
func (s *fakeSink) Publish([]model.Point) error { return nil }

type fakeProvider struct {
	adapters []southbound.Adapter
	sinks    []northbound.Sink
}

func (p fakeProvider) GetAdapters() []southbound.Adapter { return p.adapters }
func (p fakeProvider) GetSinks() []northbound.Sink       { return p.sinks }

func TestAggregateAdapterAndSinkMetrics(t *testing.T) {
	a := &fakeAdapter{southbound.NewBaseAdapter("a1", "mock")}
	for i := 0; i < 10; i++ {
		a.IncrementDataPoints()
	}
	a.IncrementErrors()
	a.AddResponseTime(4)
	s := &fakeSink{northbound.NewBaseSink("console")}
	_ = s.SafePublishBatch([]model.Point{model.NewPoint("k", "d", 1, model.TypeInt)}, func([]model.Point) error { return nil }, time.Now())

	m := NewLightweightMetrics()
	defer m.StopAutoUpdate()
	// 没有提供者时不改变指标
	SetMetricsProvider(nil)
	m.AggregateAdapterMetrics()
	m.AggregateSinkMetrics()

	SetMetricsProvider(fakeProvider{adapters: []southbound.Adapter{a}, sinks: []northbound.Sink{s}})
	defer SetMetricsProvider(nil)
	m.AggregateAdapterMetrics()
	m.AggregateSinkMetrics()
	snap := m.GetMetrics()
	if snap.DataMetrics.TotalDataPoints != 10 || snap.ErrorMetrics.TotalErrors < 1 || snap.DataMetrics.TotalMessagesPublished != 1 {
		t.Fatalf("汇总错误: data=%+v errors=%+v", snap.DataMetrics, snap.ErrorMetrics)
	}
}

func TestUpdateMethodsAndOutputs(t *testing.T) {
	m := NewLightweightMetrics()
	defer m.StopAutoUpdate()
	m.UpdateGatewayMetrics("running", "config.yaml", "plugins", 8081, 8080)
	m.UpdateDataMetrics(1000, 50000, 2.5)
	m.UpdateConnectionMetrics(3, 10, 2)
	m.UpdateErrorMetrics(5, "network", "warning", "timeout")
	m.UpdateErrorMetrics(6, "network", "error", "refused")
	m.UpdatePerformanceMetrics(100, 1, 5, 9, 2, 7)
	m.UpdateSystemMetrics()

	m.cacheMu.Lock()
	m.cache = nil // 跳过快照缓存
	m.cacheMu.Unlock()
	snap := m.GetMetrics()
	if snap.ConnectionMetrics.ActiveConnections != 3 || snap.ConnectionMetrics.FailedConnections != 2 {
		t.Fatalf("连接指标错误: %+v", snap.ConnectionMetrics)
	}
	if snap.ErrorMetrics.TotalErrors != 6 || snap.ErrorMetrics.ErrorsByType["network"] != 2 || snap.ErrorMetrics.ErrorsByLevel["error"] != 1 ||
		snap.ErrorMetrics.LastError != "refused" || snap.ErrorMetrics.ErrorRate != 0.006 {
		t.Fatalf("错误指标错误: %+v", snap.ErrorMetrics)
	}
	if snap.PerformanceMetrics.P95LatencyMS != 5 || snap.PerformanceMetrics.QueueLength != 7 {
		t.Fatalf("性能指标错误: %+v", snap.PerformanceMetrics)
	}
	if snap.SystemMetrics.GoroutineCount == 0 || snap.SystemMetrics.HeapSizeBytes == 0 {
		t.Fatalf("系统指标未采集: %+v", snap.SystemMetrics)
	}
	text := m.ToPlainText()
	for _, want := range []string{"iot_gateway_memory_usage_bytes", "iot_gateway_system_memory_percent", `iot_gateway_errors_by_level{level="error"} 1`} {
		if !strings.Contains(text, want) {
			t.Errorf("文本指标缺少 %q", want)
		}
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/json", m.HTTPHandler("json"))
	r.GET("/text", m.HTTPHandler("text"))
	r.GET("/bad", m.HTTPHandler("xml"))
	for path, code := range map[string]int{"/json": 200, "/text": 200, "/bad": 400} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != code {
			t.Errorf("GET %s = %d, want %d", path, w.Code, code)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/json", nil))
	var decoded map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil || decoded["system"] == nil {
		t.Fatalf("JSON 输出错误: %v %s", err, w.Body.String()[:100])
	}
}

func TestAutoUpdateCallbackAndGlobal(t *testing.T) {
	m := NewLightweightMetrics()
	var calls atomic.Int32
	m.SetUpdateCallback(func() { calls.Add(1) })
	m.StartAutoUpdate(20 * time.Millisecond)
	deadline := time.Now().Add(3 * time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	m.StopAutoUpdate()
	if calls.Load() < 2 {
		t.Fatalf("自动更新应周期性调用回调, got %d", calls.Load())
	}
	m.StopAutoUpdate() // 重复停止无副作用

	InitLightweightMetrics()
	if GetLightweightMetrics() == nil {
		t.Fatal("全局指标未初始化")
	}
	ShutdownLightweightMetrics()
}
