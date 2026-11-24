package services

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/web/models"
)

// ---------- 登录失败锁定 ----------

func TestLoginGuard(t *testing.T) {
	g := newLoginGuard(3, time.Minute)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if g.recordFailure("Alice") {
			t.Fatalf("未达到次数不应锁定")
		}
	}
	if g.locked("alice") {
		t.Fatalf("2 次失败不应锁定")
	}
	if !g.recordFailure(" ALICE ") {
		t.Fatalf("第 3 次失败应锁定（用户名不区分大小写和首尾空格）")
	}
	if !g.locked("alice") || g.locked("bob") {
		t.Fatalf("锁定状态错误")
	}
	now = now.Add(61 * time.Second)
	if g.locked("alice") {
		t.Fatalf("锁定时间过后应解锁")
	}
	g.recordFailure("alice")
	g.recordSuccess("alice")
	g.recordFailure("alice")
	g.recordFailure("alice")
	if g.locked("alice") {
		t.Fatalf("登录成功后应清零失败计数")
	}

	disabled := newLoginGuard(0, time.Minute)
	for i := 0; i < 10; i++ {
		disabled.recordFailure("x")
	}
	if disabled.locked("x") {
		t.Fatalf("max_login_attempts=0 表示不限制")
	}
}

func TestLoginLockoutInAuthService(t *testing.T) {
	t.Setenv("IOT_GATEWAY_DATA_DIR", t.TempDir())
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "correct-pass")
	cfg := testAuthConfig("lockout-test-secret-123456")
	cfg.MaxLoginAttempts = 3
	cfg.LockoutDuration = time.Hour
	svc := NewAuthService(NewMemoryStore(), cfg)

	for i := 0; i < 3; i++ {
		if _, err := svc.Login(&models.LoginRequest{Username: "admin", Password: "wrong"}); err == nil {
			t.Fatalf("错误密码不应登录成功")
		}
	}
	_, err := svc.Login(&models.LoginRequest{Username: "admin", Password: "correct-pass"})
	if err != ErrAccountLocked {
		t.Fatalf("锁定期间即使密码正确也应拒绝, got %v", err)
	}
	// 不存在的用户同样会被锁定（不泄露用户是否存在）
	for i := 0; i < 3; i++ {
		svc.Login(&models.LoginRequest{Username: "ghost", Password: "x"})
	}
	if _, err := svc.Login(&models.LoginRequest{Username: "ghost", Password: "x"}); err != ErrAccountLocked {
		t.Fatalf("不存在的用户名也应计数锁定, got %v", err)
	}
	hist, _ := svc.GetUserLoginHistory(1, 10)
	failed := 0
	for _, h := range hist {
		if !h.Success {
			failed++
		}
	}
	if failed != 3 {
		t.Fatalf("应记录 3 次失败的登录历史, got %d", failed)
	}
}

// ---------- 告警服务 ----------

func TestAlertServiceLifecycle(t *testing.T) {
	svc := NewAlertService()
	_, base, _ := svc.GetAlerts(&models.AlertListRequest{Page: 1, PageSize: 1000})

	a, err := svc.CreateAlert(&models.AlertCreateRequest{Title: "高温告警", Level: "warning", Source: "rule-1", Data: map[string]interface{}{"v": 45}})
	if err != nil || a.ID == "" || a.Status != "active" {
		t.Fatalf("创建告警失败: %v %+v", err, a)
	}
	b, _ := svc.CreateAlert(&models.AlertCreateRequest{Title: "设备离线", Level: "critical", Source: "monitor"})
	if a.ID == b.ID {
		t.Fatalf("告警 ID 应唯一")
	}

	list, total, err := svc.GetAlerts(&models.AlertListRequest{Page: 1, PageSize: 1000})
	if err != nil || total != base+2 || len(list) != total {
		t.Fatalf("列表数量错误: total=%d base=%d", total, base)
	}
	list, total, _ = svc.GetAlerts(&models.AlertListRequest{Page: 1, PageSize: 1000, Level: "critical", Search: "离线"})
	if total < 1 || list[0].Title != "设备离线" {
		t.Fatalf("过滤错误: %+v", list)
	}
	page, total2, _ := svc.GetAlerts(&models.AlertListRequest{Page: 1, PageSize: 1})
	if len(page) != 1 || total2 != base+2 {
		t.Fatalf("分页错误: len=%d total=%d", len(page), total2)
	}

	if err := svc.AcknowledgeAlert(a.ID, "7", "已知悉"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.GetAlert(a.ID)
	if got.Status != "acknowledged" || got.AcknowledgedBy != "7" || got.AcknowledgedAt == nil {
		t.Fatalf("确认告警失败: %+v", got)
	}
	if err := svc.ResolveAlert(a.ID, "7", "已处理"); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.GetAlert(a.ID)
	if got.Status != "resolved" || got.ResolvedAt == nil {
		t.Fatalf("解决告警失败: %+v", got)
	}

	upd, err := svc.UpdateAlert(b.ID, &models.AlertUpdateRequest{Title: "设备离线（已更新）"})
	if err != nil || upd.Title != "设备离线（已更新）" {
		t.Fatalf("更新告警失败: %v", err)
	}
	stats, err := svc.GetAlertStats()
	if err != nil || stats.Total != base+2 || stats.Resolved < 1 || stats.ByLevel["critical"] < 1 {
		t.Fatalf("统计错误: %+v", stats)
	}

	if err := svc.DeleteAlert(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetAlert(b.ID); err == nil {
		t.Fatalf("删除后不应能获取")
	}
	for _, f := range []func() error{
		func() error { return svc.DeleteAlert("nope") },
		func() error { return svc.AcknowledgeAlert("nope", "", "") },
		func() error { return svc.ResolveAlert("nope", "", "") },
	} {
		if f() == nil {
			t.Fatalf("操作不存在的告警应报错")
		}
	}
}

func TestAlertRulesAndChannels(t *testing.T) {
	svc := NewAlertService()
	rule, err := svc.CreateAlertRule(&models.AlertRuleCreateRequest{Name: "温度", Level: "warning", Enabled: true,
		Condition: &models.AlertCondition{Type: "threshold", Field: "temperature", Operator: "gt", Value: 40.0}})
	if err != nil || rule.ID == "" {
		t.Fatalf("创建告警规则失败: %v", err)
	}
	if _, err := svc.UpdateAlertRule(rule.ID, &models.AlertRuleUpdateRequest{Name: "温度2", Level: "error"}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.TestAlertRule(rule.ID, map[string]interface{}{"temperature": 45.0})
	if err != nil || res == nil {
		t.Fatalf("测试告警规则失败: %v", err)
	}
	rulesList, _ := svc.GetAlertRules()
	found := false
	for _, r := range rulesList {
		if r.ID == rule.ID && r.Name == "温度2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("规则列表中未找到更新后的规则")
	}
	if err := svc.DeleteAlertRule(rule.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteAlertRule(rule.ID); err == nil {
		t.Fatalf("重复删除应报错")
	}

	ch, err := svc.CreateNotificationChannel(&models.NotificationChannelCreateRequest{Name: "hook", Type: "webhook", Enabled: true,
		Config: map[string]interface{}{"url": "http://127.0.0.1:1/hook"}})
	if err != nil || ch.ID == "" {
		t.Fatalf("创建通知渠道失败: %v", err)
	}
	if _, err := svc.UpdateNotificationChannel(ch.ID, &models.NotificationChannelUpdateRequest{Name: "hook2", Type: "webhook", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if err := svc.TestNotificationChannel("nope"); err == nil {
		t.Fatalf("测试不存在的渠道应报错")
	}
	chs, _ := svc.GetNotificationChannels()
	if len(chs) == 0 {
		t.Fatalf("渠道列表为空")
	}
	if err := svc.DeleteNotificationChannel(ch.ID); err != nil {
		t.Fatal(err)
	}
}

// ---------- 规则服务 ----------

func TestRuleServiceCRUD(t *testing.T) {
	mgr := rules.NewManager(t.TempDir())
	if err := mgr.LoadRules(); err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if _, err := NewRuleService(nil); err == nil {
		t.Fatalf("缺少规则管理器应报错")
	}
	svc, err := NewRuleService(mgr)
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateRule(&models.RuleCreateRequest{
		Name: "高温", Priority: 5, Enabled: true,
		Conditions: &models.RuleCondition{Field: "value", Operator: "gt", Value: 40.0},
		Actions:    []models.RuleAction{{Type: "alert", Config: map[string]interface{}{"level": "warning"}}},
	})
	if err != nil || created.ID == "" {
		t.Fatalf("创建规则失败: %v", err)
	}
	got, err := svc.GetRule(created.ID)
	if err != nil || got.Name != "高温" || !got.Enabled {
		t.Fatalf("获取规则失败: %v %+v", err, got)
	}
	if err := svc.DisableRule(created.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetRule(created.ID); got.Enabled {
		t.Fatalf("禁用失败")
	}
	if err := svc.EnableRule(created.ID); err != nil {
		t.Fatal(err)
	}
	disabled := false
	upd, err := svc.UpdateRule(created.ID, &models.RuleUpdateRequest{Name: "高温2", Enabled: &disabled})
	if err != nil || upd.Name != "高温2" || upd.Enabled {
		t.Fatalf("更新规则失败: %v %+v", err, upd)
	}
	list, total, err := svc.GetRules(&models.RuleListRequest{Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("规则列表错误: total=%d err=%v", total, err)
	}
	if err := svc.DeleteRule(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetRule(created.ID); err == nil {
		t.Fatalf("删除后不应能获取")
	}
	if _, err := svc.CreateRule(&models.RuleCreateRequest{Name: "无动作", Conditions: &models.RuleCondition{Field: "value", Value: 1}}); err == nil {
		t.Fatalf("没有动作的规则应创建失败")
	}
	if tpls, err := svc.GetRuleTemplates(); err != nil || len(tpls) == 0 {
		t.Fatalf("规则模板为空: %v", err)
	}
}

// ---------- 插件服务 ----------

func TestPluginService(t *testing.T) {
	mgr := plugin.NewManager(t.TempDir(), nil, viper.New())
	if err := mgr.Init(nil); err != nil {
		t.Fatal(err)
	}
	svc, err := NewPluginService(mgr)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.GetPlugins(&models.PluginListRequest{Page: 1, PageSize: 10}); err != nil {
		t.Fatalf("获取插件列表失败: %v", err)
	}
	if _, err := svc.GetPlugin("no-such-plugin"); err == nil {
		t.Fatalf("获取不存在的插件应报错")
	}
	if err := svc.StartPlugin("no-such-plugin"); err == nil {
		t.Fatalf("启动不存在的插件应报错")
	}
	if err := svc.StopPlugin("no-such-plugin"); err == nil {
		t.Fatalf("停止未运行的插件应报错")
	}
}
