package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/y001j/iot-gateway/internal/storage"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/testutil"
)

// outageSink 模拟可能断线的云端目标
type outageSink struct {
	*northbound.BaseSink
	mu   sync.Mutex
	down bool
	got  []model.Point
}

var (
	outageOnce     sync.Once
	outageInstance *outageSink
	outageMu       sync.Mutex
)

func registerOutageSink() {
	outageOnce.Do(func() {
		northbound.Register("test_outage", func() northbound.Sink {
			s := &outageSink{BaseSink: northbound.NewBaseSink("test_outage"), down: true}
			outageMu.Lock()
			outageInstance = s
			outageMu.Unlock()
			return s
		})
	})
}

func (s *outageSink) Init(cfg json.RawMessage) error {
	_, err := s.ParseStandardConfig(cfg)
	return err
}
func (s *outageSink) Start(context.Context) error   { return nil }
func (s *outageSink) Stop() error                   { return nil }
func (s *outageSink) Publish(b []model.Point) error { return s.WriteBatch(context.Background(), b) }
func (s *outageSink) WriteBatch(_ context.Context, b []model.Point) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return errors.New("cloud unreachable")
	}
	s.got = append(s.got, b...)
	return nil
}
func (s *outageSink) setDown(d bool) { s.mu.Lock(); s.down = d; s.mu.Unlock() }
func (s *outageSink) count() int     { s.mu.Lock(); defer s.mu.Unlock(); return len(s.got) }

func TestRuntimeStoreAndForward(t *testing.T) {
	registerOutageSink()
	srv, _ := testutil.StartNATS(t)
	dir := t.TempDir()
	port := freeTCPPort(t)
	cfg := fmt.Sprintf(`
gateway:
  log_level: error
  nats_url: %q
  http_port: %d
  plugins_dir: %q
  pipeline:
    batch_size: 5
    flush_interval: 20ms
storage:
  enabled: true
  max_disk: 256MB
  raw:
    max_age: 1h
    max_bytes: 64MB
  rollup:
    enabled: true
    max_bytes: 64MB
rule_engine:
  enabled: false
southbound:
  adapters:
    - name: mock1
      type: mock
      config:
        name: mock1
        type: mock
        interval: 20ms
        data_points:
          - device_id: sensor-1
            key: temperature
            min_value: 20
            max_value: 30
northbound:
  sinks:
    - name: cloud
      type: test_outage
      params: {}
      buffer:
        enabled: true
        batch_size: 20
        max_wait: 50ms
        backoff_min: 20ms
        backoff_max: 100ms
web_ui:
  enabled: false
`, srv.ClientURL(), port, filepath.Join(dir, "plugins"))
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(cfg), 0o644)

	rt, err := NewRuntime(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Store == nil {
		t.Fatalf("storage.enabled 时应创建本地暂存")
	}
	if rt.Store.Rollup() == nil {
		t.Fatalf("storage.rollup.enabled 时应启动汇总层")
	}
	if rt.Store.AlertMonitor() == nil {
		t.Fatalf("默认应启动本地暂存告警监控")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(ctx)

	outageMu.Lock()
	sink := outageInstance
	outageMu.Unlock()

	// 目标不可达期间数据在本地积压
	testutil.WaitFor(t, 5*time.Second, func() bool {
		st := rt.PluginMgr.GetBufferStats()
		return len(st) == 1 && st[0].Pending >= 10
	}, "目标不可达时数据应积压在本地")
	if sink.count() != 0 {
		t.Fatalf("目标不可达时不应写入成功")
	}

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/storage", port))
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Enabled bool `json:"enabled"`
		Sinks   []struct {
			Name    string `json:"name"`
			State   string `json:"state"`
			Pending uint64 `json:"pending"`
		} `json:"sinks"`
		Stream struct {
			Messages uint64 `json:"messages"`
		} `json:"stream"`
		Rollup *storage.RollupStats `json:"rollup"`
	}
	json.NewDecoder(resp.Body).Decode(&status)
	resp.Body.Close()
	if !status.Enabled || len(status.Sinks) != 1 || status.Sinks[0].State != "buffering" || status.Stream.Messages == 0 ||
		status.Rollup == nil || status.Rollup.Stream != "IOT_ROLLUP" {
		t.Fatalf("/storage 状态错误: %+v", status)
	}

	// Prometheus 文本格式包含本地暂存指标
	mresp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics?format=text", port))
	if err != nil {
		t.Fatal(err)
	}
	mbody, _ := io.ReadAll(mresp.Body)
	mresp.Body.Close()
	if !strings.Contains(string(mbody), `iot_gateway_storage_sink_pending{sink="cloud"}`) ||
		!strings.Contains(string(mbody), `iot_gateway_storage_sink_state{sink="cloud",state="buffering"} 1`) {
		t.Fatalf("/metrics 缺少本地暂存指标:\n%s", mbody)
	}

	// 目标恢复：积压的数据全部补发，且按采集顺序
	backlog := rt.PluginMgr.GetBufferStats()[0].Pending
	sink.setDown(false)
	testutil.WaitFor(t, 10*time.Second, func() bool {
		return uint64(sink.count()) >= backlog && rt.PluginMgr.GetBufferStats()[0].Pending == 0
	}, "目标恢复后应补发全部积压数据")
	sink.mu.Lock()
	for i := 1; i < len(sink.got); i++ {
		if sink.got[i].Timestamp.Before(sink.got[i-1].Timestamp) {
			sink.mu.Unlock()
			t.Fatalf("补发顺序错误: 第 %d 条早于前一条", i)
		}
	}
	sink.mu.Unlock()
	if lost := rt.PluginMgr.GetBufferStats()[0].Lost; lost != 0 {
		t.Fatalf("保留上限内不应丢失数据, lost=%d", lost)
	}

	// 近期数据查询：同一份本地数据可按序列查询
	series, total, err := rt.Store.ListSeries(context.Background(), "", 0, 0)
	if err != nil || total == 0 {
		t.Fatalf("应能列出本地序列: %v %+v", err, series)
	}
	res, err := rt.Store.QueryRaw(context.Background(), storage.RawQuery{DeviceID: series[0].DeviceID, Key: series[0].Key,
		Start: time.Now().Add(-time.Minute), End: time.Now().Add(time.Second)})
	// 采集仍在进行，查询时可能比列出序列时多几条
	if err != nil || res.Matched == 0 || uint64(res.Matched) < series[0].Messages {
		t.Fatalf("查询结果应包含该序列的全部数据（至少 %d 条）: %v %+v", series[0].Messages, err, res)
	}
}
