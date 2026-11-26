package services

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	_ "github.com/y001j/iot-gateway/internal/northbound/console" // 注册 console 连接器
	"github.com/y001j/iot-gateway/internal/plugin"
	_ "github.com/y001j/iot-gateway/internal/southbound/mock" // 注册 mock 适配器
	"github.com/y001j/iot-gateway/internal/testutil"
	"github.com/y001j/iot-gateway/internal/web/models"
)

// 适配器监控按运行中的实例统计（以前按插件类型统计，实例名与类型不同时拿不到详细指标）
func TestAdapterMonitoringByInstance(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	v := viper.New()
	v.Set("southbound.adapters", []interface{}{map[string]interface{}{
		"name": "mock1", "type": "mock",
		"config": map[string]interface{}{"name": "mock1", "type": "mock", "interval": "20ms",
			"data_points": []interface{}{map[string]interface{}{"device_id": "s1", "key": "temp", "min_value": 1.0, "max_value": 2.0}}},
	}})
	v.Set("northbound.sinks", []interface{}{map[string]interface{}{"name": "console1", "type": "console", "params": map[string]interface{}{}}})
	pm := plugin.NewManager(t.TempDir(), nc, v)
	if err := pm.Init(nil); err != nil {
		t.Fatal(err)
	}
	if err := pm.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer pm.Stop(context.Background())

	s := NewAdapterMonitoringService(pm, nc)
	defer s.Stop()
	time.Sleep(100 * time.Millisecond) // 让适配器产生数据
	s.collectMetrics()
	adapters, sinks, overview, err := s.GetAdapterStatus()
	if err != nil {
		t.Fatal(err)
	}
	if len(adapters) != 1 || adapters[0].Name != "mock1" || adapters[0].Type != "mock" || adapters[0].Status != "running" || adapters[0].Health != "healthy" {
		t.Fatalf("adapters = %+v", adapters)
	}
	if adapters[0].DataPointsCount == 0 {
		t.Fatal("应拿到适配器的数据点计数")
	}
	if len(sinks) != 1 || sinks[0].Name != "console1" || sinks[0].Type != "console" || sinks[0].Health != "healthy" {
		t.Fatalf("sinks = %+v", sinks)
	}
	if overview.TotalAdapters != 1 || overview.RunningAdapters != 1 || overview.HealthySinks != 1 {
		t.Fatalf("overview = %+v", overview)
	}

	// 停止后标记为已停止
	if err := pm.StopPlugin("mock1"); err != nil {
		t.Fatal(err)
	}
	s.collectMetrics()
	adapters, _, overview, _ = s.GetAdapterStatus()
	if adapters[0].Status != "stopped" || adapters[0].Health != "stopped" || overview.RunningAdapters != 0 {
		t.Fatalf("停止后状态 = %+v %+v", adapters[0], overview)
	}

	if got := s.calculateAverageResponseTime([]float64{1, 2, 3}); got != 2 {
		t.Fatalf("平均响应时间 = %v", got)
	}
	if s.calculateAverageResponseTime(nil) != 0 {
		t.Fatal("空列表应为 0")
	}
	// 非扩展接口的适配器/连接器只给出基础健康状态
	m := &AdapterRuntimeMetrics{}
	s.updateAdapterDetailedMetrics(struct{}{}, m)
	if m.Health != "" {
		t.Fatal("未知类型不应修改健康状态")
	}
}

// fakeSMTP 是一个最小的 SMTP 服务器，记录收到的邮件
func fakeSMTP(t *testing.T) (addr string, mails chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	mails = make(chan string, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				w := func(s string) { io.WriteString(c, s+"\r\n") }
				w("220 fake ESMTP")
				var data strings.Builder
				inData := false
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if inData {
						if line == ".\r\n" {
							inData = false
							mails <- data.String()
							w("250 OK")
							continue
						}
						data.WriteString(line)
						continue
					}
					cmd := strings.ToUpper(strings.TrimSpace(line))
					switch {
					case strings.HasPrefix(cmd, "EHLO"):
						w("250-fake")
						w("250 AUTH PLAIN")
					case strings.HasPrefix(cmd, "AUTH"):
						w("235 OK")
					case strings.HasPrefix(cmd, "DATA"):
						inData = true
						w("354 go")
					case strings.HasPrefix(cmd, "QUIT"):
						w("221 bye")
						return
					default:
						w("250 OK")
					}
				}
			}(c)
		}
	}()
	return ln.Addr().String(), mails
}

func TestNotificationEmailSMSAndTestChannel(t *testing.T) {
	addr, mails := fakeSMTP(t)
	host, port, _ := net.SplitHostPort(addr)
	var portNum float64
	json.Unmarshal([]byte(port), &portNum)

	var mu sync.Mutex
	smsCount := 0
	sms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		smsCount++
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/fail") {
			w.WriteHeader(500)
		}
	}))
	defer sms.Close()

	alerts := NewAlertService()
	mk := func(typ string, cfg map[string]interface{}) string {
		ch, err := alerts.CreateNotificationChannel(&models.NotificationChannelCreateRequest{Name: typ, Type: typ, Enabled: true, Config: cfg})
		if err != nil {
			t.Fatal(err)
		}
		return ch.ID
	}
	email := mk("email", map[string]interface{}{"smtp_host": host, "smtp_port": portNum, "username": "u", "password": "p",
		"from": "gw@example.com", "recipients": []interface{}{"ops@example.com", 42}})
	smsOK := mk("sms", map[string]interface{}{"service_url": sms.URL + "/ok", "phone_numbers": []interface{}{"13800000000", "13900000000"}})
	smsFail := mk("sms", map[string]interface{}{"service_url": sms.URL + "/fail", "phone_numbers": []interface{}{"13800000000"}})
	smsBadURL := mk("sms", map[string]interface{}{"service_url": "http://127.0.0.1:1/x", "phone_numbers": []interface{}{"1"}})
	emailNoTo := mk("email", map[string]interface{}{"smtp_host": host})
	emailEmptyTo := mk("email", map[string]interface{}{"smtp_host": host, "recipients": []interface{}{1}})
	smsNoPhones := mk("sms", map[string]interface{}{"service_url": sms.URL})

	ns := NewNotificationService(alerts)
	alert := &models.Alert{ID: "a1", Title: "高温告警", Description: "温度 80℃", Level: "critical", Source: "rule", Status: "active", CreatedAt: time.Now()}
	if err := ns.SendNotification(email, alert); err != nil {
		t.Fatalf("邮件发送失败: %v", err)
	}
	select {
	case m := <-mails:
		// 中文主题按 RFC 2047 编码
		if !strings.Contains(m, "Subject: =?utf-8?q?") || !strings.Contains(m, "To: ops@example.com") || !strings.Contains(m, "温度 80") {
			t.Fatalf("邮件内容错误:\n%s", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("未收到邮件")
	}
	if err := ns.SendNotification(smsOK, alert); err != nil {
		t.Fatalf("短信发送失败: %v", err)
	}
	mu.Lock()
	if smsCount != 2 {
		t.Fatalf("应向 2 个号码发送短信: %d", smsCount)
	}
	mu.Unlock()
	// 以前短信服务返回错误或连接失败时仍返回成功
	for name, id := range map[string]string{"短信服务 500": smsFail, "短信连接失败": smsBadURL, "缺少收件人": emailNoTo,
		"无有效收件人": emailEmptyTo, "缺少号码": smsNoPhones} {
		if err := ns.SendNotification(id, alert); err == nil {
			t.Errorf("%s 应返回错误", name)
		}
	}

	// 测试渠道、渠道列表
	if err := ns.TestChannel(smsOK); err != nil {
		t.Fatalf("TestChannel: %v", err)
	}
	chans, err := ns.GetChannels()
	if err != nil || len(chans) < 7 {
		t.Fatalf("GetChannels: %v %d", err, len(chans))
	}
	body := ns.(*notificationService).buildEmailBody(alert)
	if !strings.Contains(body, "高温告警") {
		t.Fatal("邮件正文应包含标题")
	}
	for _, lvl := range []string{"critical", "error", "warning", "info", "other"} {
		if ns.(*notificationService).getAlertColor(lvl) == "" {
			t.Errorf("%s 缺少颜色", lvl)
		}
	}
}

func TestLoginGuardPurgesStaleEntries(t *testing.T) {
	g := newLoginGuard(3, time.Minute)
	now := time.Now()
	g.now = func() time.Time { return now }
	for i := 0; i <= 10000; i++ {
		g.states[string(rune('a'+i%26))+strings.Repeat("x", i/26)] = &loginState{lastFailure: now.Add(-time.Hour)}
	}
	g.recordFailure("new-user")
	if len(g.states) != 1 {
		t.Fatalf("过期记录应被清理: %d", len(g.states))
	}
	if ErrAccountLocked.Error() == "" {
		t.Fatal("ErrAccountLocked")
	}
}

func TestMainConfigConstructorsAndMonitoring(t *testing.T) {
	cs := NewConfigServiceWithMainConfig(filepath.Join(t.TempDir(), "system.json"), "../../../config.yaml")
	cfg, err := cs.GetConfig()
	if err != nil || cfg == nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if err := cs.ValidateConfig(cfg); err != nil {
		t.Fatalf("仓库主配置导出的系统配置应通过校验: %v", err)
	}
	sys := NewSystemServiceWithMainConfig(&models.AuthConfig{}, cs, "../../../config.yaml").(*systemService)
	if sys.GetMonitoringService() == nil {
		t.Fatal("应创建监控服务")
	}
	if err := sys.StopMonitoring(); err != nil {
		t.Fatal(err)
	}
	if err := sys.StartMonitoring(); err != nil {
		t.Fatal(err)
	}
	_ = sys.StopMonitoring()
	empty := &systemService{}
	if err := empty.StartMonitoring(); err == nil {
		t.Fatal("未初始化监控服务时应报错")
	}
	if err := empty.StopMonitoring(); err != nil {
		t.Fatal(err)
	}
	if err := empty.ValidateConfig(cfg); err == nil {
		t.Fatal("没有配置服务时应报错")
	}
}
