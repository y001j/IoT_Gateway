package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/web/models"
)

func TestConfigServiceLoadSaveUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "system.json")
	cs := NewConfigService(path)

	// 以前配置未加载时 GetConfig 持有读锁调用 LoadConfig，必然死锁
	done := make(chan *models.SystemConfig, 1)
	go func() {
		c, _ := cs.GetConfig()
		done <- c
	}()
	var cfg *models.SystemConfig
	select {
	case cfg = <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("GetConfig 死锁")
	}
	if cfg == nil {
		t.Fatalf("应返回默认配置")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("默认配置应写入文件: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("配置文件包含密钥，权限应为 0600, got %v", info.Mode().Perm())
	}

	updated := *cfg
	updated.Gateway.HTTPPort = 9999
	if err := cs.UpdateConfig(&updated); err != nil {
		t.Fatalf("更新配置失败: %v", err)
	}
	cs2 := NewConfigService(path)
	reloaded, err := cs2.LoadConfig()
	if err != nil || reloaded.Gateway.HTTPPort != 9999 {
		t.Fatalf("配置未持久化: %v", err)
	}

	bad := *cfg
	bad.Gateway.HTTPPort = 0
	if err := cs.UpdateConfig(&bad); err == nil {
		t.Fatalf("非法端口应校验失败")
	}
	bad = *cfg
	bad.Database.SQLite.MaxIdleConns = bad.Database.SQLite.MaxOpenConns + 1
	if err := cs.UpdateConfig(&bad); err == nil {
		t.Fatalf("空闲连接数大于最大连接数应校验失败")
	}
	os.WriteFile(path, []byte("{bad"), 0o600)
	if _, err := NewConfigService(path).LoadConfig(); err == nil {
		t.Fatalf("损坏的配置文件应报错")
	}
}

func TestSystemServiceMasksSecretAndRestart(t *testing.T) {
	cs := NewConfigService(filepath.Join(t.TempDir(), "system.json"))
	cfg, _ := cs.GetConfig()
	cfg.WebUI.Auth.JWTSecret = "real-secret-value-123456"
	if err := cs.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	sys := NewSystemService(&models.AuthConfig{}, cs)

	got, err := sys.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.WebUI.Auth.JWTSecret == "real-secret-value-123456" {
		t.Fatalf("API 返回的配置不应包含明文 JWT 密钥")
	}
	// 提交被遮盖的值时保留原密钥
	got.Gateway.HTTPPort = 8088
	if err := sys.UpdateConfig(got); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	stored, _ := cs.GetConfig()
	if stored.WebUI.Auth.JWTSecret != "real-secret-value-123456" || stored.Gateway.HTTPPort != 8088 {
		t.Fatalf("遮盖值不应覆盖原密钥: %+v", stored.WebUI.Auth)
	}
	if err := sys.RestartService("system"); err != ErrRestartNotSupported {
		t.Fatalf("未实现的重启应如实返回错误, got %v", err)
	}
	if st, err := sys.GetStatus(); err != nil || st == nil {
		t.Fatalf("GetStatus 失败: %v", err)
	}
	if _, err := sys.GetMetrics(); err != nil {
		t.Fatalf("GetMetrics 失败: %v", err)
	}
	if h, err := sys.GetHealth(); err != nil || h == nil {
		t.Fatalf("GetHealth 失败: %v", err)
	}
	if err := NewSystemService(nil, nil).UpdateConfig(got); err == nil {
		t.Fatalf("没有配置服务时更新应报错，而不是假装成功")
	}
}

func TestNotificationServiceChannels(t *testing.T) {
	var mu sync.Mutex
	bodies := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies[r.URL.Path] = string(b)
		mu.Unlock()
		if r.URL.Path == "/fail" {
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()

	alerts := NewAlertService()
	mk := func(name, typ string, enabled bool, cfg map[string]interface{}) string {
		ch, err := alerts.CreateNotificationChannel(&models.NotificationChannelCreateRequest{Name: name, Type: typ, Enabled: enabled, Config: cfg})
		if err != nil {
			t.Fatal(err)
		}
		return ch.ID
	}
	hook := mk("hook", "webhook", true, map[string]interface{}{"url": srv.URL + "/hook", "headers": map[string]interface{}{"X-Token": "abc"}})
	slack := mk("slack", "slack", true, map[string]interface{}{"webhook_url": srv.URL + "/slack"})
	ding := mk("ding", "dingtalk", true, map[string]interface{}{"webhook_url": srv.URL + "/ding"})
	failing := mk("fail", "webhook", true, map[string]interface{}{"url": srv.URL + "/fail"})
	disabled := mk("off", "webhook", false, map[string]interface{}{"url": srv.URL + "/off"})
	noURL := mk("nourl", "webhook", true, map[string]interface{}{})
	unknown := mk("weird", "pigeon", true, nil)

	ns := NewNotificationService(alerts)
	alert := &models.Alert{ID: "a1", Title: "高温", Level: "critical", Source: "rule", Status: "active", CreatedAt: time.Now()}
	for _, id := range []string{hook, slack, ding} {
		if err := ns.SendNotification(id, alert); err != nil {
			t.Fatalf("发送失败 %s: %v", id, err)
		}
	}
	mu.Lock()
	var hookPayload map[string]interface{}
	json.Unmarshal([]byte(bodies["/hook"]), &hookPayload)
	if hookPayload["alert_id"] != "a1" || !strings.Contains(bodies["/slack"], "高温") || !strings.Contains(bodies["/ding"], "高温") {
		t.Fatalf("通知内容错误: %v", bodies)
	}
	mu.Unlock()
	for name, id := range map[string]string{"服务端 500": failing, "已禁用": disabled, "缺少 URL": noURL, "未知类型": unknown, "不存在": "nope"} {
		if err := ns.SendNotification(id, alert); err == nil {
			t.Errorf("%s 应返回错误", name)
		}
	}
	if err := ns.TestChannel(hook); err != nil {
		t.Fatalf("测试渠道失败: %v", err)
	}
	if chs, _ := ns.GetChannels(); len(chs) < 7 {
		t.Fatalf("渠道数量错误")
	}

	// 邮件正文转义（告警内容来自设备数据）
	body := ns.(*notificationService).buildEmailBody(&models.Alert{Title: "<script>x</script>", CreatedAt: time.Now()})
	if strings.Contains(body, "<script>") {
		t.Fatalf("邮件正文未转义 HTML")
	}
}

func TestLogService(t *testing.T) {
	ls := NewLogService()
	if _, err := ls.GetLogs(); err != nil {
		t.Fatal(err)
	}
	if _, err := ls.SearchLogs("x"); err != nil {
		t.Fatal(err)
	}
	if _, err := ls.GetLogStats(); err != nil {
		t.Fatal(err)
	}
}

func TestNewIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 10000; i++ {
		id := newID("x")
		if seen[id] {
			t.Fatalf("ID 重复: %s", id)
		}
		seen[id] = true
	}
	// 快速连续创建的通知渠道 ID 不能相同（以前只用 UnixNano，会互相覆盖）
	alerts := NewAlertService()
	ids := map[string]bool{}
	for i := 0; i < 200; i++ {
		ch, err := alerts.CreateNotificationChannel(&models.NotificationChannelCreateRequest{Name: "c", Type: "webhook", Enabled: true, Config: map[string]interface{}{"url": "http://x"}})
		if err != nil {
			t.Fatal(err)
		}
		if ids[ch.ID] {
			t.Fatalf("渠道 ID 重复: %s", ch.ID)
		}
		ids[ch.ID] = true
	}
}
