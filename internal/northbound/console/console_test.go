package console

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *safeBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func newSink(t *testing.T, cfg string) (*ConsoleSink, *safeBuf) {
	t.Helper()
	s := NewConsoleSink()
	buf := &safeBuf{}
	s.out = buf
	if err := s.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	return s, buf
}

func TestConsoleSinkFlushesAtBatchSizeWithTags(t *testing.T) {
	s, buf := newSink(t, `{"name":"c","type":"console","batch_size":2,"tags":{"site":"A"}}`)
	p := model.NewPoint("temp", "dev-1", 21.5, model.TypeFloat)
	p.Timestamp = time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	if err := s.Publish([]model.Point{p}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "" {
		t.Fatalf("未达到 batch_size 不应输出")
	}
	s.Publish([]model.Point{model.NewPoint("hum", "dev-1", 50, model.TypeInt)})
	out := buf.String()
	if !strings.Contains(out, "dev-1.temp = 21.5 (float)") || !strings.Contains(out, "dev-1.hum = 50 (int)") {
		t.Fatalf("输出内容错误:\n%s", out)
	}
	if !strings.Contains(out, "site=A") {
		t.Fatalf("应附加配置的标签:\n%s", out)
	}
	if s.Publish(nil) != nil {
		t.Fatalf("空批次应直接返回")
	}
}

func TestConsoleSinkPeriodicAndFinalFlush(t *testing.T) {
	s, buf := newSink(t, `{"name":"c","type":"console","batch_size":1000}`)
	if s.Healthy() == nil {
		t.Fatalf("未启动时应不健康")
	}
	s.Start(context.Background())
	if s.Healthy() != nil {
		t.Fatalf("启动后应健康")
	}
	s.Publish([]model.Point{model.NewPoint("k", "d", 1, model.TypeInt)})
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "d.k = 1") {
		t.Fatalf("停止时应刷新缓冲区: %q", buf.String())
	}
}
