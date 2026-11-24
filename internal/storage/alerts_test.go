package storage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func TestBacklogRatioAndEvaluateAlerts(t *testing.T) {
	stream := &StreamStatus{Messages: 1000, Usage: 1.0} // 环形缓冲区已写满（正常状态）
	day := 24 * time.Hour

	// 没有积压：流写满本身不告警
	if a := EvaluateAlerts(DefaultAlertConfig(), AlertInput{MaxAge: day, Stream: stream, Sinks: []RunnerStats{{Name: "ok"}}, DiskRatio: 0.1}); len(a) != 0 {
		t.Fatalf("没有积压时不应告警（流写满是正常的）: %+v", a)
	}
	// 积压时长占保留时长的比例
	if r := BacklogRatio(RunnerStats{LagSeconds: 12 * 3600}, nil, day); r != 0.5 {
		t.Fatalf("时长比例错误: %v", r)
	}
	// 积压数据量占 max_bytes 的比例：900/1000 条 × 占用 100%
	if r := BacklogRatio(RunnerStats{Pending: 900, LagSeconds: 60}, stream, day); r != 0.9 {
		t.Fatalf("数据量比例错误: %v", r)
	}
	// 流未写满时，积压占比高也没有淘汰风险
	if r := BacklogRatio(RunnerStats{Pending: 1000}, &StreamStatus{Messages: 1000, Usage: 0.1}, day); r != 0.1 {
		t.Fatalf("流未写满时比例错误: %v", r)
	}

	sinks := []RunnerStats{
		{Name: "warn", Pending: 10, LagSeconds: 13 * 3600},
		{Name: "crit", Pending: 900, LagSeconds: 60},
		{Name: "lost", Lost: 5},
		{Name: "lost-old", Lost: 3},
	}
	alerts := EvaluateAlerts(DefaultAlertConfig(), AlertInput{MaxAge: day, Stream: stream, Sinks: sinks,
		PrevLost: map[string]uint64{"lost": 2, "lost-old": 3}, DiskRatio: 0.96})
	got := map[string]StorageAlert{}
	for _, a := range alerts {
		got[a.Kind+"|"+a.Sink] = a
	}
	if got["backlog|warn"].Level != "warning" || got["backlog|crit"].Level != "critical" {
		t.Fatalf("积压告警级别错误: %+v", alerts)
	}
	if a := got["data_lost|lost"]; a.Level != "critical" || !strings.Contains(a.Message, "3 条") {
		t.Fatalf("新增丢失应告警: %+v", a)
	}
	if _, ok := got["data_lost|lost-old"]; ok {
		t.Fatal("丢失量没有增加时不应重复告警")
	}
	if got["disk|"].Level != "critical" {
		t.Fatalf("磁盘告警错误: %+v", alerts)
	}
	if a := EvaluateAlerts(DefaultAlertConfig(), AlertInput{DiskRatio: 0.85}); len(a) != 1 || a[0].Level != "warning" {
		t.Fatalf("磁盘 85%% 应为 warning: %+v", a)
	}
	if a := EvaluateAlerts(DefaultAlertConfig(), AlertInput{DiskRatio: -1}); len(a) != 0 {
		t.Fatal("磁盘使用率未知时不应告警")
	}
	// PrevLost 为 nil（健康检查）：累计有丢失就告警
	if a := EvaluateAlerts(DefaultAlertConfig(), AlertInput{Sinks: []RunnerStats{{Name: "x", Lost: 1}}, DiskRatio: -1}); len(a) != 1 {
		t.Fatalf("健康检查应报告累计丢失: %+v", a)
	}
	// 新出现的连接器（PrevLost 中没有）首次检查不报告历史丢失
	if a := EvaluateAlerts(DefaultAlertConfig(), AlertInput{Sinks: []RunnerStats{{Name: "new", Lost: 9}}, PrevLost: map[string]uint64{}, DiskRatio: -1}); len(a) != 0 {
		t.Fatalf("首次检查不应把历史丢失当成新增: %+v", a)
	}
}

func TestAlertMonitorPublishesAndThrottles(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := newStore(t, nc, func(c *Config) { c.MaxAge = time.Hour; c.Alerts.Throttle = time.Hour; c.StoreDir = t.TempDir() })
	orig := diskUsageRatio
	defer func() { diskUsageRatio = orig }()
	disk := 0.5
	diskUsageRatio = func(string) float64 { return disk }

	sinks := []RunnerStats{{Name: "cloud", Pending: 100, LagSeconds: 2400}} // 40 分钟 / 1 小时 = 0.67
	sub, _ := nc.SubscribeSync("iot.alerts.triggered")
	levelSub, _ := nc.SubscribeSync("iot.alerts.triggered.warning")
	m := s.StartAlertMonitor(context.Background(), nc, func() []RunnerStats { return sinks })
	defer s.Close()
	if s.AlertMonitor() != m {
		t.Fatal("AlertMonitor 应返回已启动的监控")
	}

	now := time.Now()
	if a := m.Check(now); len(a) != 1 || a[0].Kind != AlertBacklog || a[0].Level != "warning" {
		t.Fatalf("应产生积压 warning: %+v", a)
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := levelSub.NextMsg(2 * time.Second); err != nil {
		t.Fatal("应同时发布到按级别的主题")
	}
	var payload map[string]interface{}
	json.Unmarshal(msg.Data, &payload)
	if payload["rule_id"] != "storage.backlog" || payload["key"] != "backlog:cloud" || payload["level"] != "warning" || payload["throttle"].(float64) != float64(time.Hour) {
		t.Fatalf("告警格式应与规则引擎一致: %v", payload)
	}

	// 节流：同级别一小时内不重复发送
	m.Check(now.Add(time.Minute))
	if _, err := sub.NextMsg(200 * time.Millisecond); err == nil {
		t.Fatal("节流期内不应重复发送")
	}
	// 升级为 critical 立即发送；新增丢失和磁盘告警也发送
	sinks = []RunnerStats{{Name: "cloud", Pending: 100, LagSeconds: 3000, Lost: 4}}
	disk = 0.97
	alerts := m.Check(now.Add(2 * time.Minute))
	if len(alerts) != 3 {
		t.Fatalf("应有积压、丢失、磁盘三条告警: %+v", alerts)
	}
	kinds := map[string]bool{}
	for i := 0; i < 3; i++ {
		msg, err := sub.NextMsg(2 * time.Second)
		if err != nil {
			t.Fatalf("应发送 3 条告警: %v", err)
		}
		json.Unmarshal(msg.Data, &payload)
		kinds[payload["rule_id"].(string)+"/"+payload["level"].(string)] = true
	}
	if !kinds["storage.backlog/critical"] || !kinds["storage.data_lost/critical"] || !kinds["storage.disk/critical"] {
		t.Fatalf("告警内容错误: %v", kinds)
	}
	if len(m.Current()) != 3 {
		t.Fatal("Current 应返回最近一次检查的告警")
	}
	// 丢失量不再增加时不再报告丢失
	if a := m.Check(now.Add(3 * time.Minute)); len(a) != 2 {
		t.Fatalf("丢失量未增加时不应再报告: %+v", a)
	}
}

func TestAssessAndPrometheusText(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := newStore(t, nc, func(c *Config) { c.StoreDir = t.TempDir(); c.Rollup.MaxBytes = 64 << 20 })
	publishPoints(t, nc, 0, 20)
	testutil.WaitFor(t, 3*time.Second, func() bool { st, _ := s.Status(); return st.Messages == 20 }, "数据未写入流")

	orig := diskUsageRatio
	diskUsageRatio = func(string) float64 { return 0.1 }
	if a, err := s.Assess([]RunnerStats{{Name: "ok"}}); err != nil || len(a) != 0 {
		t.Fatalf("正常状态不应有告警: %v %+v", err, a)
	}
	if a, _ := s.Assess([]RunnerStats{{Name: "x", Lost: 2}}); len(a) != 1 || a[0].Kind != AlertDataLost {
		t.Fatalf("健康检查应报告丢失: %+v", a)
	}

	text := s.PrometheusText([]RunnerStats{{Name: "b", State: StateBuffering, Pending: 7, Lost: 1}, {Name: "a", State: StateRealtime}})
	for _, want := range []string{
		"iot_gateway_storage_enabled 1", "iot_gateway_storage_messages 20", "iot_gateway_storage_usage_ratio",
		"iot_gateway_storage_disk_usage_ratio", `iot_gateway_storage_sink_pending{sink="b"} 7`,
		`iot_gateway_storage_sink_lost_total{sink="b"} 1`, `iot_gateway_storage_sink_state{sink="b",state="buffering"} 1`,
		`iot_gateway_storage_sink_state{sink="b",state="realtime"} 0`, `iot_gateway_storage_sink_backlog_ratio{sink="a"} 0`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("指标缺少 %q:\n%s", want, text)
		}
	}
	if strings.Index(text, `sink="a"`) > strings.Index(text, `sink="b"`) {
		t.Error("连接器指标应按名称排序")
	}
	if strings.Contains(text, "rollup") {
		t.Error("未开启汇总层时不应输出汇总指标")
	}
	if _, err := s.StartRollup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !strings.Contains(s.PrometheusText(nil), "iot_gateway_storage_rollup_records") {
		t.Error("开启汇总层后应输出汇总指标")
	}
	diskUsageRatio = orig
	if r := diskUsageRatio(t.TempDir()); r <= 0 || r > 1 {
		t.Errorf("磁盘使用率应在 (0,1]: %v", r)
	}
	if diskUsageRatio("") != -1 || diskUsageRatio("/no/such/dir/xyz") != -1 {
		t.Error("目录为空或不存在时应返回 -1")
	}
}

func TestAlertConfigFromViper(t *testing.T) {
	v := viper.New()
	v.Set("storage.store_dir", "/data/js")
	v.Set("storage.alerts.enabled", false)
	v.Set("storage.alerts.interval", "1m")
	v.Set("storage.alerts.throttle", "10m")
	v.Set("storage.alerts.lag_warning_ratio", 0.3)
	v.Set("storage.alerts.lag_critical_ratio", 0.6)
	v.Set("storage.alerts.disk_warning_ratio", 0.7)
	v.Set("storage.alerts.disk_critical_ratio", 0.9)
	cfg, err := ConfigFromViper(v)
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Alerts
	if cfg.StoreDir != "/data/js" || a.Enabled || a.Interval != time.Minute || a.Throttle != 10*time.Minute ||
		a.LagWarningRatio != 0.3 || a.LagCriticalRatio != 0.6 || a.DiskWarningRatio != 0.7 || a.DiskCriticalRatio != 0.9 {
		t.Fatalf("告警配置解析错误: %+v %s", a, cfg.StoreDir)
	}
	if d := DefaultConfig(); !d.Alerts.Enabled || d.StoreDir != "./data/jetstream" {
		t.Fatal("默认应开启告警")
	}
	var empty AlertConfig
	empty.applyDefaults()
	if empty.Interval != 30*time.Second || empty.LagWarningRatio != 0.5 || empty.DiskCriticalRatio != 0.95 {
		t.Fatalf("默认值修正错误: %+v", empty)
	}
}
