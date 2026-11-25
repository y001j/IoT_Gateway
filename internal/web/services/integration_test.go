package services

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/testutil"
	"github.com/y001j/iot-gateway/internal/web/models"

	_ "github.com/y001j/iot-gateway/internal/southbound/mock"
)

// ---------- 用户存储：内存实现和 SQLite 实现行为一致 ----------

func TestUserStoreContract(t *testing.T) {
	sqlite, err := NewSQLiteStore(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlite.Close()
	for name, store := range map[string]models.UserStore{"memory": NewMemoryStore(), "sqlite": sqlite} {
		t.Run(name, func(t *testing.T) { runUserStoreContract(t, store) })
	}
}

func runUserStoreContract(t *testing.T, store models.UserStore) {
	u := &models.User{Username: "alice", Password: "hash", Email: "alice@example.com", Role: "operator", Status: "active"}
	if err := store.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 {
		t.Fatal("创建后应分配 ID")
	}
	if err := store.CreateUser(&models.User{Username: "alice", Password: "x", Email: "other@example.com", Role: "viewer"}); err == nil {
		t.Fatal("重复用户名应报错")
	}
	for _, get := range []func() (*models.User, error){
		func() (*models.User, error) { return store.GetUser(u.ID) },
		func() (*models.User, error) { return store.GetUserByID(u.ID) },
		func() (*models.User, error) { return store.GetUserByUsername("alice") },
		func() (*models.User, error) { return store.GetUserByEmail("alice@example.com") },
	} {
		got, err := get()
		if err != nil || got.Username != "alice" || got.Role != "operator" {
			t.Fatalf("查询用户失败: %v %+v", err, got)
		}
	}
	if _, err := store.GetUserByEmail("nobody@example.com"); err == nil {
		t.Fatal("不存在的邮箱应报错")
	}
	u.Role = "admin"
	if err := store.UpdateUser(u); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetUser(u.ID); got.Role != "admin" {
		t.Fatalf("更新未生效: %+v", got)
	}
	bob := &models.User{Username: "bob", Password: "hash", Email: "bob@example.com", Role: "viewer", Status: "active"}
	if err := store.CreateUser(bob); err != nil {
		t.Fatal(err)
	}
	// 存储初始化时会创建默认管理员，因此至少有 3 个用户
	users, total, err := store.ListUsers(0, 10)
	if err != nil || total < 2 || len(users) != total {
		t.Fatalf("用户列表错误: %v total=%d %d", err, total, len(users))
	}
	names := map[string]bool{}
	for _, u := range users {
		names[u.Username] = true
	}
	if !names["alice"] || !names["bob"] {
		t.Fatalf("用户列表应包含 alice 和 bob: %v", names)
	}
	if page, _, _ := store.ListUsers(1, 1); len(page) != 1 {
		t.Fatalf("分页错误: %d", len(page))
	}

	// 会话
	now := time.Now()
	s1 := &models.Session{ID: "s1", UserID: u.ID, Token: "t1", RefreshToken: "r1", ExpiresAt: now.Add(time.Hour), CreatedAt: now, Active: true, LastAccess: now}
	s2 := &models.Session{ID: "s2", UserID: u.ID, Token: "t2", RefreshToken: "r2", ExpiresAt: now.Add(-time.Minute), CreatedAt: now, Active: true, LastAccess: now}
	s3 := &models.Session{ID: "s3", UserID: bob.ID, Token: "t3", RefreshToken: "r3", ExpiresAt: now.Add(time.Hour), CreatedAt: now, Active: true, LastAccess: now}
	for _, s := range []*models.Session{s1, s2, s3} {
		if err := store.CreateSession(s); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := store.GetSession("s1"); err != nil || got.Token != "t1" {
		t.Fatalf("按 ID 查询会话失败: %v", err)
	}
	if got, err := store.GetSessionByToken("t3"); err != nil || got.UserID != bob.ID {
		t.Fatalf("按令牌查询会话失败: %v", err)
	}
	if got, err := store.GetSessionByRefreshToken("r1"); err != nil || got.ID != "s1" {
		t.Fatalf("按刷新令牌查询会话失败: %v", err)
	}
	s1.Token = "t1-new"
	if err := store.UpdateSession(s1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSessionByToken("t1-new"); err != nil {
		t.Fatalf("更新会话未生效: %v", err)
	}
	if err := store.CleanExpiredSessions(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSession("s2"); err == nil {
		t.Fatal("过期会话应被清理")
	}
	if err := store.DeleteSession("s3"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSessionByToken("t3"); err == nil {
		t.Fatal("删除的会话不应再能查到")
	}
	if err := store.DeleteUserSessions(u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSession("s1"); err == nil {
		t.Fatal("删除用户全部会话后不应再能查到")
	}

	// 登录历史：最新的在前，按 limit 截断
	for i := 0; i < 3; i++ {
		if err := store.AddLoginHistory(&models.LoginHistory{UserID: u.ID, IP: "127.0.0.1", UserAgent: "test", Success: i != 1, LoginTime: now.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	hist, err := store.GetUserLoginHistory(u.ID, 2)
	if err != nil || len(hist) != 2 {
		t.Fatalf("登录历史错误: %v %d", err, len(hist))
	}

	if err := store.DeleteUser(bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetUser(bob.ID); err == nil {
		t.Fatal("删除的用户不应再能查到")
	}
}

// ---------- 告警集成：规则引擎告警 → Web 告警 → 通知 → 自动解除 ----------

func TestAlertIntegrationFlow(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	alerts := NewAlertServiceWithNATS(nc)
	notif := NewNotificationService(alerts)

	var hookCalls atomic.Int32
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hookCalls.Add(1)
		w.WriteHeader(200)
	}))
	defer hook.Close()
	ch, err := alerts.CreateNotificationChannel(&models.NotificationChannelCreateRequest{Name: "hook", Type: "webhook", Enabled: true, Config: map[string]interface{}{"url": hook.URL}})
	if err != nil {
		t.Fatal(err)
	}

	integ := NewAlertIntegrationService(alerts, notif, nc, nil)
	if err := integ.Start(); err != nil {
		t.Fatal(err)
	}
	defer integ.Stop()

	var mu sync.Mutex
	var frontend []string
	sub, _ := nc.Subscribe("iot.frontend.alerts", func(m *nats.Msg) {
		var ev map[string]interface{}
		json.Unmarshal(m.Data, &ev)
		mu.Lock()
		frontend = append(frontend, ev["type"].(string))
		mu.Unlock()
	})
	defer sub.Unsubscribe()

	// 与规则引擎实际发布的格式一致（rules.Alert：Throttle 为纳秒整数）
	ruleAlert := map[string]interface{}{
		"id": "a1", "rule_id": "r1", "rule_name": "高温", "level": "warning", "message": "温度 42 超过阈值",
		"device_id": "d1", "key": "temp", "value": 42, "tags": map[string]string{"zone": "a"},
		"timestamp": time.Now(), "throttle": int64(time.Hour),
		"notification_channels": []string{ch.ID}, "auto_resolve": true, "resolve_timeout": int64(300 * time.Millisecond),
	}
	data, _ := json.Marshal(ruleAlert)
	nc.Publish("iot.alerts.triggered", data)
	nc.Publish("iot.alerts.triggered", data)               // 节流：同一规则/设备/点位一小时内只告警一次
	nc.Publish("iot.alerts.triggered", []byte("not json")) // 无效消息被忽略
	nc.Flush()

	testutil.WaitFor(t, 3*time.Second, func() bool {
		list, _, _ := alerts.GetAlerts(&models.AlertListRequest{Page: 1, PageSize: 10, Source: "rule-engine"})
		return len(list) == 1
	}, "规则引擎告警应转换为 Web 告警")
	list, _, _ := alerts.GetAlerts(&models.AlertListRequest{Page: 1, PageSize: 10, Source: "rule-engine"})
	a := list[0]
	if a.Title != "高温 - d1:temp" || a.Level != "warning" || a.Data["tag_zone"] != "a" || a.Description != "温度 42 超过阈值" {
		t.Fatalf("告警内容错误: %+v", a)
	}
	testutil.WaitFor(t, 3*time.Second, func() bool { return hookCalls.Load() == 1 }, "应通过 webhook 渠道发送通知")
	if st := integ.GetStats(); st["throttle_entries"] != 1 || st["nats_subscriptions"] != 1 {
		t.Fatalf("集成服务状态错误: %v", st)
	}

	// 超时自动解除
	testutil.WaitFor(t, 3*time.Second, func() bool {
		got, _ := alerts.GetAlert(a.ID)
		return got != nil && got.Status == "resolved"
	}, "告警应自动解除")
	testutil.WaitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(frontend, ",") == "alert_created,alert_resolved"
	}, "前端应收到告警创建和解除事件")
	if st := integ.GetStats(); st["active_alerts"] != 0 || st["auto_resolve_timers"] != 0 {
		t.Fatalf("自动解除后应清理状态: %v", st)
	}

	// 标题生成与过期节流记录清理
	if got := integ.generateAlertTitle(&RuleEngineAlert{RuleName: "R", DeviceID: "d"}); got != "R - d" {
		t.Fatal(got)
	}
	if got := integ.generateAlertTitle(&RuleEngineAlert{RuleName: "R"}); got != "R" {
		t.Fatal(got)
	}
	integ.mu.Lock()
	integ.alertThrottleMap["old"] = time.Now().Add(-2 * time.Hour)
	integ.mu.Unlock()
	integ.cleanup()
	if st := integ.GetStats(); st["throttle_entries"] != 1 {
		t.Fatalf("过期的节流记录应被清理: %v", st)
	}
	// 没有通知服务或渠道时跳过；自动解除不存在的告警返回错误
	(&AlertIntegrationService{}).sendNotifications(&a, []string{"x"})
	integ.sendNotifications(&a, nil)
	if err := integ.autoResolveAlert("nope"); err == nil {
		t.Fatal("解除不存在的告警应报错")
	}
}

func TestAlertSubscriberAndStore(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := NewAlertSubscriber(nc)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err == nil {
		t.Fatal("重复启动应报错")
	}
	if !s.IsRunning() || s.GetSubscriptionCount() != 0 {
		t.Fatal("订阅器状态错误（告警创建由告警集成服务负责，这里不订阅）")
	}
	alert := s.convertToWebAlert(map[string]interface{}{
		"id": "x1", "rule_id": "r", "rule_name": "规则", "level": "error", "message": "消息", "device_id": "d", "key": "k",
		"value": 1.5, "timestamp": "2026-11-24T10:00:00Z", "tags": map[string]interface{}{"a": "b", "n": 1.0},
		"notification_channels": []interface{}{"c1", 2.0}, "priority": 3.0, "auto_resolve": true,
	})
	if alert.ID != "x1" || alert.Title != "消息" || alert.Tags["a"] != "b" || len(alert.Tags) != 1 || len(alert.NotificationChannels) != 1 ||
		alert.Priority != 3 || !alert.AutoResolve || alert.CreatedAt.Year() != 2026 || alert.Value != 1.5 {
		t.Fatalf("告警转换错误: %+v", alert)
	}
	if a := s.convertToWebAlert(map[string]interface{}{"rule_name": "仅名称"}); a.Title != "规则告警: 仅名称" {
		t.Fatalf("没有消息时的标题错误: %q", a.Title)
	}
	s.handleAlertMessage(&nats.Msg{Subject: "iot.alerts.triggered", Data: []byte(`{"id":"x2","rule_name":"r","level":"info"}`)})
	s.handleAlertMessage(&nats.Msg{Subject: "iot.alerts.triggered", Data: []byte(`bad`)})

	store := s.GetAlertStore()
	store.AddAlert(alert)
	if store.GetAlertCount() != 2 || len(store.GetAlerts()) != 2 {
		t.Fatalf("告警存储数量错误: %d", store.GetAlertCount())
	}
	alert.Status = "resolved"
	store.UpdateAlert(alert)
	if got, ok := store.GetAlert("x1"); !ok || got.Status != "resolved" {
		t.Fatal("更新告警失败")
	}
	store.DeleteAlert("x1")
	if _, ok := store.GetAlert("x1"); ok {
		t.Fatal("删除告警失败")
	}
	if err := s.Stop(); err != nil || s.IsRunning() {
		t.Fatal("停止失败")
	}
	if err := s.Stop(); err != nil {
		t.Fatal("重复停止应无副作用")
	}
}

// ---------- 系统告警：超过阈值告警，恢复后自动解除 ----------

func TestSystemAlertService(t *testing.T) {
	alerts := NewAlertService()
	cfg := &SystemAlertConfig{Enabled: true, CheckInterval: time.Hour, CPUThreshold: -1, MemoryThreshold: -1, GoroutineThreshold: 0,
		AlertCooldown: time.Hour, AutoResolve: true}
	s := NewSystemAlertService(cfg, alerts, metrics.NewLightweightMetrics())
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err == nil {
		t.Fatal("重复启动应报错")
	}
	// 只统计指标告警（告警服务启动时会有一条"系统启动完成"提示告警）
	metricAlerts := func() []models.Alert {
		list, _, _ := alerts.GetAlerts(&models.AlertListRequest{Page: 1, PageSize: 50, Source: "system"})
		var out []models.Alert
		for _, a := range list {
			if a.Data["metric_type"] != nil {
				out = append(out, a)
			}
		}
		return out
	}
	s.checkSystemMetrics()
	if got := metricAlerts(); len(got) != 3 {
		t.Fatalf("CPU、内存、协程三个指标都应告警, got %d: %+v", len(got), got)
	}
	// 内存告警基于系统内存使用率（以前是"已分配堆/申请的堆"，正常运行时也常超过 85%）
	for _, a := range metricAlerts() {
		if a.Data["metric_type"] == "memory_usage" {
			total, _ := a.Data["total_bytes"].(int64)
			if a.Title != "系统内存使用率过高" || total <= 0 || a.Data["process_heap_bytes"] == nil {
				t.Fatalf("内存告警应基于系统内存: %+v", a)
			}
		}
	}
	s.checkSystemMetrics() // 冷却期内不重复告警
	if got := metricAlerts(); len(got) != 3 {
		t.Fatalf("冷却期内不应重复告警, got %d", len(got))
	}
	if st := s.GetStatus(); st["active_alerts"] != 3 || st["running"] != true {
		t.Fatalf("状态错误: %v", st)
	}

	// 指标恢复：自动解除并从缓存中移除（以前会在每个周期重复解除）
	s.config.CPUThreshold, s.config.MemoryThreshold, s.config.GoroutineThreshold = 1e9, 1e9, 1<<30
	s.checkSystemMetrics()
	for _, a := range metricAlerts() {
		if a.Status != "resolved" {
			t.Fatalf("恢复后告警应解除: %+v", a)
		}
	}
	if st := s.GetStatus(); st["active_alerts"] != 0 {
		t.Fatalf("解除后应清理缓存: %v", st)
	}
	if err := s.Stop(); err != nil || s.IsRunning() {
		t.Fatal("停止失败")
	}
	if err := s.Stop(); err == nil {
		t.Fatal("未运行时停止应报错")
	}

	// 默认配置；未启用时 Start 不运行；没有指标服务时检查直接返回
	d := NewSystemAlertService(nil, alerts, nil)
	if d.config.CPUThreshold != 80 || d.config.CheckInterval != 30*time.Second {
		t.Fatalf("默认配置错误: %+v", d.config)
	}
	d.checkSystemMetrics()
	off := NewSystemAlertService(&SystemAlertConfig{Enabled: false}, alerts, nil)
	if err := off.Start(); err != nil || off.IsRunning() {
		t.Fatal("未启用时不应运行")
	}
}

// ---------- 适配器监控 ----------

func newPluginManagerWithMock(t *testing.T, nc *nats.Conn) *plugin.Manager {
	t.Helper()
	v := viper.New()
	v.Set("southbound.adapters", []interface{}{map[string]interface{}{
		"name": "mock1", "type": "mock",
		"config": map[string]interface{}{"name": "mock1", "type": "mock", "interval": "1s",
			"data_points": []interface{}{map[string]interface{}{"device_id": "dev1", "key": "temp", "min_value": 1.0, "max_value": 2.0}},
			"password":    "secret"},
	}})
	mgr := plugin.NewManager(t.TempDir(), nc, v)
	if err := mgr.Init(nil); err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestAdapterMonitoringService(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	mgr := newPluginManagerWithMock(t, nc)
	s := NewAdapterMonitoringService(mgr, nc)
	defer s.Stop()
	s.TrackAdapterStart("mock")
	s.TrackSinkStart("sink1")

	// 按需订阅：查询数据流之前不订阅（以前常驻订阅 iot.data.> 并逐条解析）
	if s.flows.active() {
		t.Fatal("没有查询时不应订阅数据流")
	}
	if flow, _ := s.GetDataFlowMetrics("1h"); len(flow) != 0 || !s.flows.active() {
		t.Fatal("首次查询后应开始订阅")
	}
	nc.Flush()
	for i := 0; i < 5; i++ {
		p := model.NewPoint("temp", "dev1", 1.5+float64(i), model.TypeFloat)
		p.Timestamp = time.Now().Add(-10 * time.Millisecond)
		data, _ := json.Marshal(p)
		nc.Publish("iot.data.dev1.temp", data)
	}
	nc.Publish(model.DataSubject("plc.1", "p"), []byte("bad")) // 无法解析：仍然计数，设备和点位从主题还原
	nc.Flush()
	var flow []models.DataFlowMetrics
	testutil.WaitFor(t, 3*time.Second, func() bool {
		flow, _ = s.GetDataFlowMetrics("1h")
		return len(flow) == 2
	}, "应统计两个序列")
	byDev := map[string]models.DataFlowMetrics{}
	for _, f := range flow {
		byDev[f.DeviceID] = f
	}
	d1 := byDev["dev1"]
	// 每个序列每秒最多解析一条：最新值来自第一条消息
	if d1.Key != "temp" || d1.LastValue != 1.5 || d1.DataPointsPerSec <= 0 || d1.BytesPerSec <= 0 || d1.LatencyMS <= 0 {
		t.Fatalf("dev1 数据流指标错误: %+v", d1)
	}
	if p1, ok := byDev["plc.1"]; !ok || p1.Key != "p" || p1.LastValue != nil {
		t.Fatalf("无法解析的消息应按主题还原设备: %+v", byDev)
	}
	if v, _ := s.flows.flows.Load("iot.data.dev1.temp"); v.(*flowCounter).count.Load() != 5 {
		t.Fatal("每条消息都应计数")
	}

	// 空闲后退订；长时间无数据的序列被清理
	s.flows.mu.Lock()
	s.flows.lastQuery = time.Now().Add(-time.Hour)
	s.flows.mu.Unlock()
	s.flows.stopIfIdle(time.Now())
	if s.flows.active() {
		t.Fatal("空闲超时后应退订")
	}
	s.retentionDuration = time.Nanosecond
	time.Sleep(time.Millisecond)
	s.cleanupOldMetrics()
	if flow := s.flows.metrics(time.Hour, time.Now()); len(flow) != 0 {
		t.Fatalf("长时间无数据的序列应被清理: %+v", flow)
	}

	if cfg := s.sanitizeConfig(map[string]interface{}{"password": "x", "api_token": "y", "host": "h"}); cfg["password"] == "x" || cfg["api_token"] == "y" || cfg["host"] != "h" {
		t.Fatalf("敏感配置未脱敏: %v", cfg)
	}
	if err := s.RestartAdapter("nope"); err == nil {
		t.Fatal("重启不存在的适配器应报错")
	}
	if err := (&AdapterMonitoringService{}).RestartAdapter("x"); err == nil {
		t.Fatal("没有插件管理器时应报错")
	}
}

// TestDataFlowTrackerRates 速率按计数快照计算，不受记录条数上限影响
func TestDataFlowTrackerRates(t *testing.T) {
	tr := newDataFlowTracker(nil)
	tr.ensureSubscribed() // 没有 NATS 连接时无副作用
	now := time.Now()
	tr.subscribed = now.Add(-60 * time.Second)
	msg := &nats.Msg{Subject: "iot.data.d.k", Data: []byte(`{"device_id":"d","key":"k","value":1}`)}
	for i := 0; i < 50; i++ {
		tr.handle(msg)
	}
	tr.snapshot(now.Add(-10 * time.Second))
	for i := 0; i < 10; i++ {
		tr.handle(msg)
	}
	// 最近 10 秒：基准为 10 秒前的快照（50 条），新增 10 条 → 1 条/秒
	m := tr.metrics(10*time.Second, now)
	if len(m) != 1 || !approxEq(m[0].DataPointsPerSec, 1) || m[0].BytesPerSec <= 0 {
		t.Fatalf("10 秒窗口速率错误: %+v", m)
	}
	// 窗口早于统计开始：从统计开始计算，60 条 / 60 秒
	m = tr.metrics(time.Hour, now)
	if !approxEq(m[0].DataPointsPerSec, 1) {
		t.Fatalf("1 小时窗口速率错误: %+v", m)
	}
	// 快照数量有上限
	tr.maxSnaps = 3
	for i := 0; i < 10; i++ {
		tr.snapshot(now)
	}
	v, _ := tr.flows.Load("iot.data.d.k")
	if n := len(v.(*flowCounter).snaps); n != 3 {
		t.Fatalf("快照应保留最近 3 个, got %d", n)
	}
	tr.stop()
}

func approxEq(a, b float64) bool { return a-b < 0.01 && b-a < 0.01 }

// ---------- 插件服务 ----------

func TestPluginServiceWithAdapter(t *testing.T) {
	mgr := newPluginManagerWithMock(t, nil)
	svc, err := NewPluginService(mgr)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := svc.GetPluginConfig("mock")
	if err != nil || cfg == nil {
		t.Fatalf("获取插件配置失败: %v", err)
	}
	if _, err := svc.GetPluginConfig("nope"); err == nil {
		t.Fatal("不存在的插件应报错")
	}
	if res, err := svc.ValidatePluginConfig("mock", map[string]interface{}{"interval": "1s"}); err != nil || res == nil {
		t.Fatalf("校验插件配置失败: %v", err)
	}
	stats, err := svc.GetPluginStats("mock")
	if err != nil || stats == nil {
		t.Fatalf("获取插件统计失败: %v", err)
	}
	if _, err := svc.GetPluginStats("nope"); err == nil {
		t.Fatal("不存在的插件应报错")
	}

	// 日志：没有日志文件时返回空列表，不再编造示例日志
	dir := t.TempDir()
	wd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(wd)
	logs, total, err := svc.GetPluginLogs("mock", &models.PluginLogRequest{Page: 1, PageSize: 10})
	if err != nil || total != 0 || len(logs) != 0 {
		t.Fatalf("没有日志文件时应返回空列表: %v %d %+v", err, total, logs)
	}
	os.MkdirAll("logs", 0o755)
	lines := []string{
		`{"level":"info","time":"2026-11-24T10:00:00+08:00","message":"mock1 启动","plugin":"mock1"}`,
		`{"level":"error","time":"2026-11-24T10:01:00+08:00","message":"mock1 连接失败"}`,
		`2026-11-24 10:02:00 [WARN] mock1 plain text line`,
		``,
	}
	os.WriteFile("logs/mock.log", []byte(strings.Join(lines, "\n")), 0o644)
	logs, total, err = svc.GetPluginLogs("mock", &models.PluginLogRequest{Page: 1, PageSize: 10})
	if err != nil || total == 0 {
		t.Fatalf("应读取日志文件: %v %d", err, total)
	}
	errorsOnly, _, _ := svc.GetPluginLogs("mock", &models.PluginLogRequest{Page: 1, PageSize: 10, Level: "error"})
	for _, l := range errorsOnly {
		if l.Level != "error" {
			t.Fatalf("按级别过滤失败: %+v", l)
		}
	}
	if page, total2, _ := svc.GetPluginLogs("mock", &models.PluginLogRequest{Page: 100, PageSize: 10}); len(page) != 0 || total2 != total {
		t.Fatal("越界分页应返回空")
	}

	if err := svc.RestartPlugin("nope"); err == nil {
		t.Fatal("重启不存在的插件应报错")
	}
	if err := svc.UpdatePluginConfig("nope", map[string]interface{}{}); err == nil {
		t.Fatal("更新不存在的插件配置应报错")
	}
	if err := svc.DeletePlugin("nope"); err == nil {
		t.Fatal("删除不存在的插件应报错")
	}
}

// ---------- 规则服务：校验和测试与规则引擎语义一致 ----------

func TestRuleServiceValidateAndTestUseEngineSemantics(t *testing.T) {
	mgr := rules.NewManager(t.TempDir())
	if err := mgr.LoadRules(); err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	svc, _ := NewRuleService(mgr)

	// 不带 type 的条件（与通过本服务创建的规则一致）以前必然校验失败
	rule := models.Rule{Name: "高温", Priority: 95,
		Conditions: &models.RuleCondition{And: []models.RuleCondition{
			{Field: "key", Operator: "eq", Value: "temp"},
			{Field: "value", Operator: "gte", Value: 30},
			{Field: "tags.zone", Value: "a"}, // 操作符为空即 eq
		}},
		Actions: []models.RuleAction{{Type: "alert", Config: map[string]interface{}{}}},
	}
	v, err := svc.ValidateRule(&rule)
	if err != nil || !v.Valid {
		t.Fatalf("合法规则应通过校验: %v %+v", err, v)
	}
	if len(v.Warnings) != 2 {
		t.Fatalf("应有优先级和告警消息两条警告: %v", v.Warnings)
	}
	res, err := svc.TestRule(&models.RuleTestRequestExtended{Rule: rule, TestData: []map[string]interface{}{
		{"key": "temp", "value": 35, "zone": "a"},
		{"key": "temp", "value": 25.0, "zone": "a"},
		{"key": "temp", "value": 35, "tags": map[string]interface{}{"zone": "b"}},
	}})
	if err != nil || !res.Success || len(res.Results) != 3 {
		t.Fatalf("测试规则失败: %v %+v", err, res)
	}
	if !res.Results[0].Matched || res.Results[1].Matched || res.Results[2].Matched || len(res.Results[0].Actions) != 1 {
		t.Fatalf("求值结果错误（gte 以前不支持，整数值以前报错）: %+v", res.Results)
	}

	// 表达式条件
	expr := models.Rule{Name: "表达式", Conditions: &models.RuleCondition{Type: "expression", Expression: "value > 10 && device_id == 'd1'"},
		Actions: []models.RuleAction{{Type: "filter", Config: map[string]interface{}{"type": "range"}}}}
	res, _ = svc.TestRule(&models.RuleTestRequestExtended{Rule: expr, TestData: []map[string]interface{}{{"device_id": "d1", "value": 11.0, "quality": 0.0, "type": "float"}}})
	if !res.Success || !res.Results[0].Matched {
		t.Fatalf("表达式求值错误: %+v", res)
	}

	// 非法规则
	for name, bad := range map[string]models.Rule{
		"缺少名称":   {Conditions: &models.RuleCondition{Field: "value", Value: 1}, Actions: []models.RuleAction{{Type: "filter"}}},
		"优先级越界":  {Name: "x", Priority: 101, Conditions: &models.RuleCondition{Field: "value", Value: 1}, Actions: []models.RuleAction{{Type: "filter"}}},
		"缺少条件":   {Name: "x", Actions: []models.RuleAction{{Type: "filter"}}},
		"缺少动作":   {Name: "x", Conditions: &models.RuleCondition{Field: "value", Value: 1}},
		"未知操作符":  {Name: "x", Conditions: &models.RuleCondition{Field: "value", Operator: "between", Value: 1}, Actions: []models.RuleAction{{Type: "filter"}}},
		"未知动作类型": {Name: "x", Conditions: &models.RuleCondition{Field: "value", Value: 1}, Actions: []models.RuleAction{{Type: "email"}}},
		"未知条件类型": {Name: "x", Conditions: &models.RuleCondition{Type: "sql"}, Actions: []models.RuleAction{{Type: "filter"}}},
	} {
		v, _ := svc.ValidateRule(&bad)
		if v.Valid {
			t.Errorf("%s: 应校验失败", name)
		}
		if res, _ := svc.TestRule(&models.RuleTestRequestExtended{Rule: bad, TestData: []map[string]interface{}{{}}}); res.Success {
			t.Errorf("%s: 校验失败的规则测试应返回失败", name)
		}
	}

	// 统计和历史：以前返回随机数冒充真实数据
	created, err := svc.CreateRule(&models.RuleCreateRequest{Name: "r", Enabled: true,
		Conditions: &models.RuleCondition{Field: "value", Operator: "gt", Value: 1.0},
		Actions:    []models.RuleAction{{Type: "filter", Config: map[string]interface{}{"type": "range"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetRuleStats(created.ID); !errors.Is(err, ErrRuleStatsUnavailable) {
		t.Fatalf("规则统计应如实返回不支持: %v", err)
	}
	if _, _, err := svc.GetRuleExecutionHistory(created.ID, &models.RuleHistoryRequest{Page: 1, PageSize: 10}); !errors.Is(err, ErrRuleStatsUnavailable) {
		t.Fatalf("执行历史应如实返回不支持: %v", err)
	}
	if _, err := svc.GetRuleStats("nope"); err == nil || errors.Is(err, ErrRuleStatsUnavailable) {
		t.Fatal("不存在的规则应返回未找到")
	}
	if _, _, err := svc.GetRuleExecutionHistory("nope", &models.RuleHistoryRequest{Page: 1, PageSize: 10}); err == nil || errors.Is(err, ErrRuleStatsUnavailable) {
		t.Fatal("不存在的规则应返回未找到")
	}

	// 从模板创建
	templates, _ := svc.GetRuleTemplates()
	if len(templates) == 0 {
		t.Fatal("应有规则模板")
	}
	fromTpl, err := svc.CreateRuleFromTemplate(templates[0].ID, &models.RuleFromTemplateRequest{Name: "模板规则", Enabled: true,
		Parameters: map[string]interface{}{"threshold": 50, "device_id": "d1", "key": "temp"}})
	if err != nil || fromTpl.ID == "" || fromTpl.Name != "模板规则" {
		t.Fatalf("从模板创建规则失败: %v %+v", err, fromTpl)
	}
	if _, err := svc.CreateRuleFromTemplate("nope", &models.RuleFromTemplateRequest{Name: "x"}); err == nil {
		t.Fatal("不存在的模板应报错")
	}
}

// ---------- 服务容器 ----------

func TestServicesLifecycleAndHelpers(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	t.Setenv("IOT_GATEWAY_DATA_DIR", t.TempDir())
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "svc-test-pass")
	svc, err := NewServices(&ServiceConfig{
		Auth:          &models.AuthConfig{JWTSecret: "svc-test-secret-0123456789", TokenDuration: time.Hour, PasswordMinLength: 6, BcryptCost: 4},
		DBPath:        filepath.Join(t.TempDir(), "auth.db"),
		PluginManager: plugin.NewManager(t.TempDir(), nc, viper.New()),
		NATSConn:      nc,
		Metrics:       metrics.NewLightweightMetrics(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if svc.AlertIntegration == nil || svc.AdapterMonitoring == nil || svc.SystemAlert == nil {
		t.Fatal("有 NATS 和指标服务时应创建告警集成、适配器监控和系统告警服务")
	}
	if err := svc.Start(); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}

	// 没有规则管理器时使用空规则服务
	empty := &emptyRuleService{}
	if list, total, err := empty.GetRules(nil); err != nil || total != 0 || len(list) != 0 {
		t.Fatal("空规则服务应返回空列表")
	}
	if tpl, err := empty.GetRuleTemplates(); err != nil || len(tpl) != 0 {
		t.Fatal("空规则服务应返回空模板")
	}
	errCalls := []error{}
	_, e := empty.GetRule("x")
	errCalls = append(errCalls, e)
	_, e = empty.CreateRule(nil)
	errCalls = append(errCalls, e)
	_, e = empty.UpdateRule("x", nil)
	errCalls = append(errCalls, e)
	errCalls = append(errCalls, empty.DeleteRule("x"), empty.EnableRule("x"), empty.DisableRule("x"))
	_, e = empty.ValidateRule(nil)
	errCalls = append(errCalls, e)
	_, e = empty.TestRule(nil)
	errCalls = append(errCalls, e)
	_, e = empty.GetRuleStats("x")
	errCalls = append(errCalls, e)
	_, _, e = empty.GetRuleExecutionHistory("x", nil)
	errCalls = append(errCalls, e)
	_, e = empty.CreateRuleFromTemplate("x", nil)
	errCalls = append(errCalls, e)
	for i, e := range errCalls {
		if e == nil {
			t.Errorf("空规则服务第 %d 个方法应返回错误", i)
		}
	}

	if r := SuccessResponse("d", "ok"); !r.Success || r.Code != 200 || r.Data != "d" {
		t.Fatal("成功响应错误")
	}
	if r := ErrorResponseWithCode(400, "bad", errors.New("boom")); r.Success || r.Code != 400 || r.Error != "boom" {
		t.Fatal("错误响应错误")
	}
	if r := ErrorResponseWithCode(404, "x", nil); r.Error != "" {
		t.Fatal("没有错误时不应有 error 字段")
	}
	if r := NewPagedResponse([]int{1}, 10, 2, 5); r.Total != 10 || r.Page != 2 || r.Size != 5 || !r.Success {
		t.Fatal("分页响应错误")
	}
}
