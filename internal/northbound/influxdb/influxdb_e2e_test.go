package influxdb

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

type writeLog struct {
	mu     sync.Mutex
	bodies map[string][]string // bucket -> line protocol bodies
}

func newFakeInflux(t *testing.T) (*httptest.Server, *writeLog) {
	wl := &writeLog{bodies: map[string][]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Write([]byte(`{"status":"pass","name":"influxdb","message":"ok"}`))
			return
		}
		if r.URL.Path != "/api/v2/write" {
			w.WriteHeader(404)
			return
		}
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				w.WriteHeader(400)
				return
			}
			body = gz
		}
		b, _ := io.ReadAll(body)
		wl.mu.Lock()
		bucket := r.URL.Query().Get("bucket")
		wl.bodies[bucket] = append(wl.bodies[bucket], string(b))
		wl.mu.Unlock()
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	return srv, wl
}

func (wl *writeLog) all(bucket string) string {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	return strings.Join(wl.bodies[bucket], "\n")
}

func TestInfluxDBSinkWritesWithAcquisitionTime(t *testing.T) {
	srv, wl := newFakeInflux(t)
	s := NewInfluxDBSink()
	cfg := fmt.Sprintf(`{"name":"influx","type":"influxdb","batch_size":100,"params":{"url":%q,"token":"t","org":"o","bucket":"main",
		"flush_interval_ms":50,"points":{"pressure":{"bucket":"other","measurement":"press"}}}}`, srv.URL)
	if err := s.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Healthy(); err != nil {
		t.Fatalf("健康检查失败: %v", err)
	}

	ts := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	p1 := model.NewPoint("temp", "dev1", uint64(25), model.TypeInt)
	p1.Timestamp = ts
	p2 := model.NewPoint("pressure", "dev1", 101.3, model.TypeFloat)
	p2.Timestamp = ts
	bad := model.NewPoint("broken", "dev1", "not-a-number", model.TypeFloat)
	if err := s.Publish([]model.Point{p1, p2, bad}); err != nil {
		t.Fatalf("部分点不合法不应导致整批失败: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	main := wl.all("main")
	wantTS := fmt.Sprintf("%d", ts.UnixNano())
	if !strings.Contains(main, "temp,") || !strings.Contains(main, "value=25i") || !strings.Contains(main, wantTS) {
		t.Fatalf("默认 bucket 写入内容错误（应包含采集时间 %s）:\n%s", wantTS, main)
	}
	if strings.Contains(main, "broken") {
		t.Fatalf("不合法的点应被跳过")
	}
	if other := wl.all("other"); !strings.Contains(other, "press,") || !strings.Contains(other, "value=101.3") {
		t.Fatalf("点位配置的 bucket 应使用非阻塞写入且同样被刷新:\n%s", other)
	}
	if err := s.Publish([]model.Point{p1}); err == nil {
		t.Fatalf("停止后写入应报错")
	}
}

func TestInfluxDBWriteBatchErrorClassification(t *testing.T) {
	status := 204
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/api/v2/write" {
			if status >= 400 {
				w.WriteHeader(status)
				w.Write([]byte(`{"code":"invalid","message":"bad"}`))
				return
			}
			w.WriteHeader(status)
		}
	}))
	defer srv.Close()
	s := NewInfluxDBSink()
	cfg := fmt.Sprintf(`{"name":"i","type":"influxdb","params":{"url":%q,"org":"o","bucket":"b"}}`, srv.URL)
	if err := s.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	p := model.NewPoint("k", "d", 1.0, model.TypeFloat)
	ctx := context.Background()
	if err := s.WriteBatch(ctx, []model.Point{p}); err != nil {
		t.Fatalf("同步写入应成功: %v", err)
	}
	mu.Lock()
	status = 400
	mu.Unlock()
	if err := s.WriteBatch(ctx, []model.Point{p}); !northbound.IsPermanent(err) {
		t.Fatalf("400 应为永久错误, got %v", err)
	}
	mu.Lock()
	status = 503
	mu.Unlock()
	if err := s.WriteBatch(ctx, []model.Point{p}); err == nil || northbound.IsPermanent(err) {
		t.Fatalf("503 应为可重试错误, got %v", err)
	}
	if err := s.WriteBatch(ctx, []model.Point{model.NewPoint("k", "d", "x", model.TypeFloat)}); !northbound.IsPermanent(err) {
		t.Fatalf("全部无法转换的批次应为永久错误, got %v", err)
	}
}
