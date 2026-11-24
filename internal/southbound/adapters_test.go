package southbound_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/southbound"
	_ "github.com/y001j/iot-gateway/internal/southbound/composite_mock"
	_ "github.com/y001j/iot-gateway/internal/southbound/http"
	_ "github.com/y001j/iot-gateway/internal/southbound/mock"
)

func TestBaseAdapter(t *testing.T) {
	b := southbound.NewBaseAdapter("a1", "mock")
	if b.Name() != "a1" || b.IsRunning() {
		t.Fatalf("初始状态错误")
	}
	b.SetRunning(true)
	ch := make(chan model.Point, 1)
	b.SafeSendDataPoint(ch, model.Point{Key: "k"}, time.Now())
	// 通道满：丢弃并记录错误，不能阻塞
	done := make(chan struct{})
	go func() {
		b.SafeSendDataPoint(ch, model.Point{Key: "k2"}, time.Now())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("通道满时不应阻塞")
	}
	if b.GetLastError() == nil {
		t.Fatalf("丢弃数据点时应记录错误")
	}
	b.SetLastError(errors.New("x"))
	h, _ := b.Health()
	if h.Status == "" {
		t.Fatalf("健康状态为空")
	}
	if _, err := b.GetMetrics(); err != nil {
		t.Fatal(err)
	}
	b.AddResponseTime(10)
	b.AddResponseTime(20)
	if avg := b.GetAverageResponseTime(); avg <= 0 {
		t.Fatalf("平均响应时间 = %v", avg)
	}
}

func startAdapter(t *testing.T, typ, cfg string) (southbound.Adapter, chan model.Point) {
	t.Helper()
	a, ok := southbound.Create(typ)
	if !ok {
		t.Fatalf("适配器类型 %s 未注册", typ)
	}
	if err := a.Init(json.RawMessage(cfg)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	ch := make(chan model.Point, 1000)
	if err := a.Start(context.Background(), ch); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return a, ch
}

func waitPoint(t *testing.T, ch chan model.Point, d time.Duration) model.Point {
	t.Helper()
	select {
	case p := <-ch:
		return p
	case <-time.After(d):
		t.Fatalf("超时未收到数据点")
	}
	return model.Point{}
}

func TestMockAdapterProducesDataAndRestarts(t *testing.T) {
	cfg := `{"name":"m","type":"mock","interval":"20ms","data_points":[{"device_id":"d1","key":"temp","min_value":10,"max_value":20}]}`
	a, ch := startAdapter(t, "mock", cfg)
	p := waitPoint(t, ch, 2*time.Second)
	if p.DeviceID != "d1" || p.Key != "temp" {
		t.Fatalf("数据点错误: %+v", p)
	}
	if v, ok := p.Value.(float64); !ok || v < 10 || v > 20 {
		t.Fatalf("模拟值超出范围: %v", p.Value)
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := a.Start(context.Background(), ch); err != nil {
		t.Fatalf("停止后应可再次启动: %v", err)
	}
	waitPoint(t, ch, 2*time.Second)
	a.Stop()
}

func TestCompositeMockDoubleStop(t *testing.T) {
	cfg := `{"name":"c","type":"composite_mock","interval":"20ms","composite_data_types":[
		{"device_id":"gps","key":"loc","data_type":"location","location_config":{"start_latitude":39.9,"start_longitude":116.4}}]}`
	a, ch := startAdapter(t, "composite_mock", cfg)
	p := waitPoint(t, ch, 2*time.Second)
	if p.Type != model.TypeLocation {
		t.Fatalf("应产生位置数据: %+v", p)
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	// 以前第二次 Stop 会 close 已关闭的通道而 panic
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPAdapterPollsAndClosesResponses(t *testing.T) {
	var requests int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"sensor":{"temp":%d.5,"ok":true,"name":"s1"}}`, n)
	}))
	var newConns int32
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			atomic.AddInt32(&newConns, 1)
		}
	}
	srv.Start()
	defer srv.Close()

	cfg := fmt.Sprintf(`{"name":"h","type":"http","url":%q,"interval":"20ms","timeout":"1s",
		"data_points":[
			{"key":"temp","path":"sensor.temp","type":"float","device_id":"dev"},
			{"key":"ok","path":"sensor.ok","type":"bool","device_id":"dev"},
			{"key":"name","path":"sensor.name","type":"string","device_id":"dev"}]}`, srv.URL)
	a, ch := startAdapter(t, "http", cfg)
	got := map[string]model.Point{}
	deadline := time.After(3 * time.Second)
	for len(got) < 3 {
		select {
		case p := <-ch:
			got[p.Key] = p
		case <-deadline:
			t.Fatalf("超时，已收到: %v", got)
		}
	}
	if _, ok := got["temp"].Value.(float64); !ok || got["ok"].Value != true || got["name"].Value != "s1" {
		t.Fatalf("解析结果错误: %+v", got)
	}

	// 多次轮询后，连接应被复用（响应体每次都被关闭）
	time.Sleep(300 * time.Millisecond)
	a.Stop()
	if r := atomic.LoadInt32(&requests); r < 5 {
		t.Fatalf("轮询次数过少: %d", r)
	}
	if c := atomic.LoadInt32(&newConns); c > 2 {
		t.Fatalf("建立了 %d 个新连接（响应体未关闭导致连接无法复用）", c)
	}
	a.Stop()
}
