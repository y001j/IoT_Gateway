// benchmark 网关端到端压测工具，用于在目标硬件（例如 ARM 网关）上测量实际吞吐。
//
// 用法（交叉编译后拷贝到设备运行）：
//
//	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o bench ./cmd/tools/benchmark
//	./bench -duration 30s -devices 100 -rules 10 -expr-rules 2 -storage
//
// 数据路径与生产一致：适配器 → 数据管道 → NATS → 规则引擎（工作池）→ 动作；同时写入一个计数连接器。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/rs/zerolog"
	"github.com/y001j/iot-gateway/internal/core"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/southbound"
)

var (
	generated atomic.Uint64
	received  atomic.Uint64
)

// benchAdapter 以最高速度（或指定速率）生成数据点
type benchAdapter struct {
	*southbound.BaseAdapter
	devices int
	rate    int
	stop    chan struct{}
}

func (a *benchAdapter) Init(json.RawMessage) error {
	a.BaseAdapter = southbound.NewBaseAdapter("bench", "bench")
	a.stop = make(chan struct{})
	return nil
}

func (a *benchAdapter) Start(ctx context.Context, ch chan<- model.Point) error {
	go func() {
		// 限速时每 10ms 生成一批（time.Ticker 的精度不足以支持微秒级间隔）
		perTick := 0
		var tick <-chan time.Time
		if a.rate > 0 {
			perTick = a.rate / 100
			if perTick < 1 {
				perTick = 1
			}
			t := time.NewTicker(10 * time.Millisecond)
			defer t.Stop()
			tick = t.C
		}
		i := 0
		for {
			n := 1
			if tick != nil {
				select {
				case <-tick:
				case <-a.stop:
					return
				case <-ctx.Done():
					return
				}
				n = perTick
			}
			for j := 0; j < n; j++ {
				p := model.NewPoint("temperature", fmt.Sprintf("dev-%d", i%a.devices), float64(i%100), model.TypeFloat)
				p.AddTag("source", "bench")
				select {
				case ch <- p:
					generated.Add(1)
				case <-a.stop:
					return
				case <-ctx.Done():
					return
				}
				i++
			}
		}
	}()
	return nil
}

func (a *benchAdapter) Stop() error { close(a.stop); return nil }

// countSink 统计收到的数据点
type countSink struct{ *northbound.BaseSink }

func (s *countSink) Init(cfg json.RawMessage) error { _, err := s.ParseStandardConfig(cfg); return err }
func (s *countSink) Start(context.Context) error    { return nil }
func (s *countSink) Stop() error                    { return nil }
func (s *countSink) Publish(b []model.Point) error  { received.Add(uint64(len(b))); return nil }
func (s *countSink) WriteBatch(_ context.Context, b []model.Point) error {
	received.Add(uint64(len(b)))
	return nil
}

func freePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func main() {
	duration := flag.Duration("duration", 20*time.Second, "压测时长")
	devices := flag.Int("devices", 100, "模拟设备数")
	rate := flag.Int("rate", 0, "生成速率（点/秒），0 表示尽可能快")
	nRules := flag.Int("rules", 10, "简单条件规则数")
	nExpr := flag.Int("expr-rules", 0, "表达式条件规则数")
	useStorage := flag.Bool("storage", false, "开启本地暂存，计数连接器使用缓冲模式")
	bufBatch := flag.Int("buffer-batch", 1000, "缓冲模式每批拉取的消息数")
	flag.Parse()
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	southbound.Register("bench", func() southbound.Adapter { return &benchAdapter{devices: *devices, rate: *rate} })
	northbound.Register("bench_count", func() northbound.Sink { return &countSink{BaseSink: northbound.NewBaseSink("bench_count")} })

	dir, err := os.MkdirTemp("", "iot-bench-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	ns, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true,
		JetStream: true, StoreDir: filepath.Join(dir, "js"), JetStreamMaxMemory: 256 << 20, JetStreamMaxStore: 4 << 30})
	if err != nil {
		panic(err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		panic("NATS 启动超时")
	}
	defer ns.Shutdown()

	rulesDir := filepath.Join(dir, "rules")
	os.MkdirAll(rulesDir, 0o755)
	var ruleList []*rules.Rule
	for i := 0; i < *nRules; i++ {
		ruleList = append(ruleList, &rules.Rule{ID: fmt.Sprintf("simple-%d", i), Name: "simple", Enabled: true,
			Conditions: &rules.Condition{And: []*rules.Condition{
				{Field: "key", Operator: "eq", Value: "temperature"},
				{Field: "value", Operator: "gt", Value: float64(90 + i%10)},
			}},
			Actions: []rules.Action{{Type: "filter", Config: map[string]interface{}{"type": "range", "parameters": map[string]interface{}{"min": 0.0, "max": 1000.0}}}}})
	}
	for i := 0; i < *nExpr; i++ {
		ruleList = append(ruleList, &rules.Rule{ID: fmt.Sprintf("expr-%d", i), Name: "expr", Enabled: true,
			Conditions: &rules.Condition{Type: "expression", Expression: "value > 95 && key == 'temperature'"},
			Actions:    []rules.Action{{Type: "filter", Config: map[string]interface{}{"type": "null"}}}})
	}
	data, _ := json.Marshal(ruleList)
	os.WriteFile(filepath.Join(rulesDir, "bench.json"), data, 0o644)

	buffer := ""
	storageCfg := "storage:\n  enabled: false\n"
	if *useStorage {
		storageCfg = fmt.Sprintf("storage:\n  enabled: true\n  store_dir: %q\n  max_disk: 4GB\n  raw:\n    max_bytes: 2GB\n", filepath.Join(dir, "js"))
		buffer = fmt.Sprintf("      buffer:\n        enabled: true\n        batch_size: %d\n", *bufBatch)
	}
	cfg := fmt.Sprintf(`gateway:
  log_level: error
  nats_url: %q
  http_port: %d
  plugins_dir: %q
  pipeline:
    batch_size: 500
    flush_interval: 20ms
    channel_size: 20000
%s
rule_engine:
  enabled: %v
  rules_dir: %q
  hot_reload:
    enabled: false
southbound:
  adapters:
    - name: bench
      type: bench
      config: {name: bench, type: bench}
northbound:
  sinks:
    - name: count
      type: bench_count
      params: {}
%sweb_ui:
  enabled: false
`, ns.ClientURL(), freePort(), filepath.Join(dir, "plugins"), storageCfg, *nRules+*nExpr > 0, rulesDir, buffer)
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(cfg), 0o644)

	rt, err := core.NewRuntime(cfgPath)
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := rt.Start(ctx); err != nil {
		panic(err)
	}

	fmt.Printf("平台: %s/%s, CPU: %d, 设备: %d, 规则: %d 简单 + %d 表达式, 本地暂存: %v\n",
		runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), *devices, *nRules, *nExpr, *useStorage)
	start := time.Now()
	ticker := time.NewTicker(time.Second)
	var lastGen, lastRecv uint64
	for time.Since(start) < *duration {
		<-ticker.C
		g, r := generated.Load(), received.Load()
		fmt.Printf("  %4.0fs  生成 %8d/s  连接器 %8d/s\n", time.Since(start).Seconds(), g-lastGen, r-lastRecv)
		lastGen, lastRecv = g, r
	}
	ticker.Stop()
	elapsed := time.Since(start).Seconds()

	var ruleStats *rules.WorkerPoolStats
	for _, svc := range rt.Svcs {
		if rs, ok := svc.(*rules.RuleEngineService); ok {
			ruleStats = rs.GetWorkerPoolStats()
		}
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Println("==== 结果 ====")
	fmt.Printf("采集吞吐:     %.0f 点/秒\n", float64(generated.Load())/elapsed)
	fmt.Printf("连接器吞吐:   %.0f 点/秒\n", float64(received.Load())/elapsed)
	if ruleStats != nil {
		fmt.Printf("规则任务:     %.0f 次/秒（完成 %d，丢弃 %d，平均延迟 %v）\n",
			float64(ruleStats.CompletedTasks)/elapsed, ruleStats.CompletedTasks, ruleStats.DroppedTasks, ruleStats.AvgLatency)
	}
	for _, st := range rt.PluginMgr.GetSinkDispatchStats() {
		fmt.Printf("连接器 %s:   丢弃 %d 点\n", st.Name, st.DroppedPoints)
	}
	for _, st := range rt.PluginMgr.GetBufferStats() {
		fmt.Printf("缓冲 %s:     积压 %d，丢失 %d\n", st.Name, st.Pending, st.Lost)
	}
	fmt.Printf("内存:         堆 %.1f MB，系统 %.1f MB，协程 %d\n", float64(ms.HeapAlloc)/(1<<20), float64(ms.Sys)/(1<<20), runtime.NumGoroutine())

	cancel()
	rt.Stop(context.Background())
}
