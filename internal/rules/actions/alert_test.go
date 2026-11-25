package actions

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func alertRule() *rules.Rule {
	return &rules.Rule{ID: "r1", Name: "高温规则", Tags: map[string]string{"site": "a"}}
}

func TestAlertHandlerChannelsTemplateAndThrottle(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	var hookCalls atomic.Int32
	var gotAuth, gotBody string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hookCalls.Add(1)
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
	}))
	defer hook.Close()

	h := NewAlertHandlerWithNATS(nc)
	if h.Name() != "alert" {
		t.Fatal(h.Name())
	}
	custom, _ := nc.SubscribeSync("alerts.custom")
	triggered, _ := nc.SubscribeSync("iot.alerts.triggered")
	nc.Flush()

	p := model.NewPoint("temp", "dev-1", 42.5, model.TypeFloat)
	p.AddTag("zone", "north")
	cfg := map[string]interface{}{
		"level":    "critical",
		"message":  "{{.RuleName}}: {{.DeviceID}} {{.Key}}={{.Value}} 区域={{.Tags.zone}}",
		"throttle": "1h",
		"tags":     map[string]interface{}{"team": "ops"},
		"channels": []interface{}{
			map[string]interface{}{"type": "console"},
			map[string]interface{}{"type": "webhook", "config": map[string]interface{}{"url": hook.URL, "token": "tk"}},
			map[string]interface{}{"type": "nats", "config": map[string]interface{}{"subject": "alerts.custom"}},
		},
	}
	res, err := h.Execute(context.Background(), p, alertRule(), cfg)
	if err != nil || !res.Success {
		t.Fatalf("执行失败: %v %+v", err, res)
	}
	out := res.Output.(map[string]interface{})
	if out["message"] != "高温规则: dev-1 temp=42.5 区域=north" || out["channels_sent"] != 3 || out["level"] != "critical" {
		t.Fatalf("输出错误: %+v", out)
	}
	if hookCalls.Load() != 1 || gotAuth != "Bearer tk" || !strings.Contains(gotBody, "dev-1") {
		t.Fatalf("webhook 未正确发送: calls=%d auth=%q body=%s", hookCalls.Load(), gotAuth, gotBody)
	}
	if _, err := custom.NextMsg(2 * time.Second); err != nil {
		t.Fatal("nats 渠道应发布到配置的主题")
	}
	msg, err := triggered.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatal("告警应发布到 iot.alerts.triggered")
	}
	var alert map[string]interface{}
	json.Unmarshal(msg.Data, &alert)
	if alert["rule_id"] != "r1" || alert["level"] != "critical" {
		t.Fatalf("发布的告警内容错误: %v", alert)
	}

	// 节流：同一规则/设备/点位在 1 小时内只告警一次
	res, _ = h.Execute(context.Background(), p, alertRule(), cfg)
	if res.Output.(map[string]interface{})["throttled"] != true || hookCalls.Load() != 1 {
		t.Fatalf("应被节流: %+v", res)
	}
	// 其他设备不受影响
	p2 := model.NewPoint("temp", "dev-2", 1.0, model.TypeFloat)
	if res, _ := h.Execute(context.Background(), p2, alertRule(), cfg); res.Output.(map[string]interface{})["throttled"] == true {
		t.Fatal("不同设备不应被节流")
	}
}

func TestAlertHandlerFailuresAndRetry(t *testing.T) {
	var calls atomic.Int32
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer flaky.Close()
	h := NewAlertHandler()
	p := model.NewPoint("temp", "dev-1", 1.0, model.TypeFloat)

	// 重试：前两次 500，第三次成功
	res, _ := h.Execute(context.Background(), p, alertRule(), map[string]interface{}{
		"retry_count": 3.0, "retry_delay": "1ms",
		"channels": []interface{}{map[string]interface{}{"type": "webhook", "config": map[string]interface{}{"url": flaky.URL}}},
	})
	if !res.Success || calls.Load() != 3 {
		t.Fatalf("重试后应成功: calls=%d %+v", calls.Load(), res)
	}

	// 失败的渠道：webhook 未配置 URL、邮件/短信未实现（以前假装发送成功）、NATS 未连接、未知渠道
	// （默认节流 5 分钟，每次使用不同的规则 ID）
	res, _ = h.Execute(context.Background(), p, &rules.Rule{ID: "r-fail", Name: "x"}, map[string]interface{}{
		"channels": []interface{}{
			map[string]interface{}{"type": "webhook", "config": map[string]interface{}{}},
			map[string]interface{}{"type": "email", "config": map[string]interface{}{"to": "a@example.com"}},
			map[string]interface{}{"type": "sms", "config": map[string]interface{}{}},
			map[string]interface{}{"type": "nats", "config": map[string]interface{}{"subject": "x"}},
			map[string]interface{}{"type": "pager"},
		},
	})
	if res.Success || res.Output.(map[string]interface{})["channels_sent"] != 0 {
		t.Fatalf("所有渠道都失败时应返回失败: %+v", res)
	}
	for _, want := range []string{"webhook", "尚未实现", "NATS"} {
		if !strings.Contains(res.Error, want) {
			t.Errorf("错误信息应包含 %q: %s", want, res.Error)
		}
	}
	if err := h.sendEmailAlert(&rules.Alert{}, nil); !errors.Is(err, ErrChannelNotImplemented) {
		t.Fatal("邮件渠道应返回未实现")
	}
	// 带重试时失败的渠道也返回失败
	res, _ = h.Execute(context.Background(), p, &rules.Rule{ID: "r2", Name: "x"}, map[string]interface{}{
		"retry_count": 1, "retry_delay": "1ms",
		"channels": []interface{}{map[string]interface{}{"type": "sms"}, map[string]interface{}{"type": "email"}},
	})
	if res.Success {
		t.Fatalf("重试后仍失败应返回失败: %+v", res)
	}
	// 没有配置渠道时默认输出到控制台
	if res, _ := h.Execute(context.Background(), p, &rules.Rule{ID: "r3", Name: "x"}, map[string]interface{}{"level": "info"}); !res.Success {
		t.Fatalf("默认渠道应成功: %+v", res)
	}
}

func TestAlertHandlerTemplateAndHelpers(t *testing.T) {
	h := NewAlertHandler()
	cfg, err := h.parseConfig(map[string]interface{}{
		"level": "error", "throttle_duration": "5m", "retry_count": 2, "retry_delay": "2s",
		"template": map[string]interface{}{"unit": "°C"}, "tags": map[string]interface{}{"k": "v", "n": 1},
	})
	if err != nil || cfg.Level != "error" || cfg.Throttle != 5*time.Minute || cfg.RetryCount != 2 || cfg.RetryDelay != 2*time.Second || cfg.Tags["k"] != "v" {
		t.Fatalf("配置解析错误: %v %+v", err, cfg)
	}
	// 无效的节流时长被忽略，使用默认 5 分钟（配置写错时告警仍然有效）
	if c, err := h.parseConfig(map[string]interface{}{"throttle": "soon"}); err != nil || c.Throttle != 5*time.Minute {
		t.Fatalf("无效的节流时长应使用默认值: %v %+v", err, c)
	}

	rule := alertRule()
	loc := model.NewPoint("gps", "truck", &model.LocationData{Latitude: 31.2, Longitude: 121.5}, model.TypeLocation)
	vec := model.NewPoint("acc", "imu", &model.Vector3D{X: 1, Y: 2, Z: 3}, model.TypeVector3D)
	num := model.NewPoint("temp", "d", 25.0, model.TypeFloat)
	cases := []struct {
		tpl   string
		point model.Point
		want  string
	}{
		{"位置 {{.value.latitude}},{{.value.longitude}}", loc, "位置 31.2,121.5"},
		{"Z={{.value.z}} X={{.value.x}}", vec, "Z=3 X=1"},
		{"{{.Level}} {{.RuleID}} {{.Type}}", num, "warning r1 float"},
		{"{{.unit}}", num, "°C"},
		{"无效模板 {{.Value", num, "无效模板"}, // 模板语法错误时退回简单替换，不 panic
	}
	ac := &AlertConfig{Level: "warning", Template: map[string]interface{}{"unit": "°C"}}
	for _, c := range cases {
		if got := h.parseMessageTemplate(c.tpl, c.point, rule, ac); !strings.Contains(got, c.want) {
			t.Errorf("模板 %q = %q, want 包含 %q", c.tpl, got, c.want)
		}
	}

	// 节流辅助方法
	a := &rules.Alert{RuleID: "r", DeviceID: "d", Key: "k", Throttle: time.Hour}
	if h.shouldThrottle(a) {
		t.Fatal("首次不应节流")
	}
	h.recordThrottle(a)
	if !h.shouldThrottle(a) || !h.checkAndRecordThrottle(a) {
		t.Fatal("记录后应节流")
	}
	if h.checkAndRecordThrottle(&rules.Alert{}) || h.shouldThrottle(&rules.Alert{}) {
		t.Fatal("未配置节流时不应节流")
	}
	h.recordThrottle(&rules.Alert{})
	if id1, id2 := h.generateAlertID(), h.generateAlertID(); id1 == id2 || id1 == "" {
		t.Fatal("告警 ID 应唯一")
	}
	if chs := h.extractNotificationChannels(&AlertConfig{Channels: []ChannelConfig{{Type: "webhook"}, {Type: "nats"}}}); len(chs) != 2 {
		t.Fatalf("通知渠道提取错误: %v", chs)
	}
	for in, want := range map[interface{}]float64{1.5: 1.5, 2: 2, int64(3): 3, float32(4): 4} {
		if got, ok := toFloat64(in); !ok || got != want {
			t.Errorf("toFloat64(%v) = %v", in, got)
		}
	}
	if _, ok := toFloat64("x"); ok {
		t.Error("字符串不应转换成功")
	}
}
