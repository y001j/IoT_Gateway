package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/testutil"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"
)

// newNATSRouter 创建带 NATS、指标服务的完整路由（启用告警集成、适配器监控、WebSocket 实时推送）
func newNATSRouter(t *testing.T) (*gin.Engine, *services.Services, *nats.Conn) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("IOT_GATEWAY_DATA_DIR", t.TempDir())
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "handler-test-pass")
	_, nc := testutil.StartNATS(t)
	mgr := rules.NewManager(t.TempDir())
	if err := mgr.LoadRules(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mgr.Close() })
	svc, err := services.NewServices(&services.ServiceConfig{
		Auth: &models.AuthConfig{JWTSecret: "nats-test-secret-0123456789", TokenDuration: time.Hour,
			PasswordMinLength: 6, BcryptCost: 4, MaxLoginAttempts: 5, LockoutDuration: time.Minute},
		PluginManager: plugin.NewManager(t.TempDir(), nc, viper.New()),
		RuleManager:   mgr,
		NATSConn:      nc,
		Metrics:       metrics.NewLightweightMetrics(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	r := gin.New()
	SetupRoutes(r, svc, nc)
	return r, svc, nc
}

// readWS 读取消息直到出现指定类型
func readWS(t *testing.T, conn *websocket.Conn, typ string) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(deadline)
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("读取 WebSocket 消息失败（等待 %s）: %v", typ, err)
		}
		if msg["type"] == typ {
			return msg
		}
	}
	t.Fatalf("未收到 %s 消息", typ)
	return nil
}

func TestWebSocketRealtimeEndToEnd(t *testing.T) {
	r, svc, nc := newNATSRouter(t)
	srv := httptest.NewServer(r)
	defer srv.Close()
	tok := login(t, r, "admin", "handler-test-pass")

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws/realtime?token=" + tok
	if _, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/api/v1/ws/realtime", nil); err == nil {
		t.Fatal("没有令牌时不应建立连接")
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	readWS(t, conn, "welcome")

	// 客户端消息：ping / subscribe / unsubscribe / 无效消息
	conn.WriteJSON(map[string]interface{}{"type": "ping"})
	readWS(t, conn, "pong")
	conn.WriteMessage(websocket.TextMessage, []byte("not json"))
	conn.WriteJSON(map[string]interface{}{"no_type": 1})
	conn.WriteJSON(map[string]interface{}{"type": "subscribe"})                                   // 缺少 data
	conn.WriteJSON(map[string]interface{}{"type": "subscribe", "data": map[string]interface{}{}}) // 缺少 channel
	conn.WriteJSON(map[string]interface{}{"type": "unknown"})
	conn.WriteJSON(map[string]interface{}{"type": "subscribe", "data": map[string]interface{}{"channel": "all"}})
	confirmed := readWS(t, conn, "subscription_confirmed")
	if confirmed["data"].(map[string]interface{})["channel"] != "all" {
		t.Fatalf("订阅确认错误: %v", confirmed)
	}
	conn.WriteJSON(map[string]interface{}{"type": "unsubscribe", "data": map[string]interface{}{"channel": "x"}})

	// NATS 数据 → WebSocket 推送（带类型和派生值）
	p := model.NewPoint("location", "truck.1", &model.LocationData{Latitude: 31.2, Longitude: 121.5, Speed: 30}, model.TypeLocation)
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	nc.Publish(model.DataSubject("truck.1", "location"), data)
	nc.Flush()
	// IoT 数据和规则事件共用 50ms 的全局限流窗口
	time.Sleep(80 * time.Millisecond)
	nc.Publish("iot.rules.matched", []byte(`{"rule_id":"r1"}`))
	nc.Publish("iot.system.status", []byte(`{"ok":true}`))
	nc.Flush()
	iot := readWS(t, conn, "iot_data")
	inner := iot["data"].(map[string]interface{})["data"].(map[string]interface{})
	if inner["data_type"] != "location" || inner["derived_values"].(map[string]interface{})["moving"] != true {
		t.Fatalf("推送的数据缺少类型或派生值: %v", inner)
	}
	// 规则事件和系统事件来自不同的订阅，到达顺序不固定
	seen := map[string]bool{}
	deadline := time.Now().Add(5 * time.Second)
	for !(seen["rule_event"] && seen["system_event"]) {
		conn.SetReadDeadline(deadline)
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("等待规则/系统事件失败: %v (已收到 %v)", err, seen)
		}
		seen[msg["type"].(string)] = true
	}

	// 统计和管理接口
	var stats struct {
		ClientCount int `json:"client_count"`
		Clients     []struct {
			ClientID string `json:"client_id"`
		} `json:"clients"`
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/websocket/stats", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &stats)
	if stats.ClientCount != 1 || len(stats.Clients) != 1 {
		t.Fatalf("应有 1 个客户端: %s", w.Body.String())
	}
	id := stats.Clients[0].ClientID
	for _, c := range []struct {
		path string
		body interface{}
		want int
	}{
		{"/api/v1/websocket/push/enable", map[string]bool{"enabled": false}, 200},
		{"/api/v1/websocket/push/enable", map[string]bool{"enabled": true}, 200},
		{"/api/v1/websocket/clients/" + id + "/push", map[string]bool{"enabled": false}, 200},
		{"/api/v1/websocket/clients/" + id + "/push", map[string]bool{"enabled": true}, 200},
		{"/api/v1/websocket/clients/" + id + "/subscriptions", map[string][]string{"subscriptions": {"iot_data"}}, 200},
		{"/api/v1/websocket/clients/nope/push", map[string]bool{"enabled": true}, 404},
		{"/api/v1/websocket/clients/nope/subscriptions", map[string][]string{"subscriptions": {}}, 404},
	} {
		if code, _ := doJSON(t, r, "POST", c.path, tok, c.body); code != c.want {
			t.Errorf("POST %s = %d, want %d", c.path, code, c.want)
		}
	}
	for _, path := range []string{"/api/v1/websocket/push/enable", "/api/v1/websocket/clients/" + id + "/push", "/api/v1/websocket/clients/" + id + "/subscriptions"} {
		req := httptest.NewRequest("POST", path, strings.NewReader("{bad"))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Errorf("POST %s 无效 JSON 应返回 400, got %d", path, w.Code)
		}
	}
	viewer := tokenFor(t, svc, "viewer")
	if code, _ := doJSON(t, r, "POST", "/api/v1/websocket/push/enable", viewer, map[string]bool{"enabled": false}); code != 403 {
		t.Errorf("推送管理仅限管理员, got %d", code)
	}

	// 只订阅 iot_data 后，规则事件不再推送；关闭连接后客户端被注销
	time.Sleep(80 * time.Millisecond)
	nc.Publish("iot.rules.matched", []byte(`{"rule_id":"r2"}`))
	nc.Flush()
	time.Sleep(80 * time.Millisecond)
	nc.Publish(model.DataSubject("d2", "temp"), mustJSON(model.NewPoint("temp", "d2", 21.5, model.TypeFloat)))
	nc.Flush()
	msg := readWS(t, conn, "iot_data")
	if msg["data"].(map[string]interface{})["subject"] != "iot.data.d2.temp" {
		t.Fatalf("应只收到 IoT 数据: %v", msg)
	}
	conn.Close()
	testutil.WaitFor(t, 3*time.Second, func() bool {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/websocket/stats", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		r.ServeHTTP(w, req)
		return strings.Contains(w.Body.String(), `"client_count":0`)
	}, "连接关闭后客户端应被注销")
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestEnhanceIoTData(t *testing.T) {
	h := &WebSocketHandler{}
	cases := []struct {
		data     map[string]interface{}
		wantType string
		check    func(d map[string]interface{}) bool
	}{
		{map[string]interface{}{"value": map[string]interface{}{"latitude": 1.0, "longitude": 2.0, "altitude": 10.0}}, "location",
			func(d map[string]interface{}) bool { return d["altitude_m"] == 10.0 && d["speed_kmh"] == nil }},
		{map[string]interface{}{"value": map[string]interface{}{"x": 3.0, "y": 4.0, "z": 0.0}}, "vector3d",
			func(d map[string]interface{}) bool { return d != nil }},
		{map[string]interface{}{"value": map[string]interface{}{"r": 255.0, "g": 0.0, "b": 16.0}}, "color",
			func(d map[string]interface{}) bool {
				return strings.Contains(d["display_value"].(string)+d["summary"].(string), "#FF0010")
			}},
		{map[string]interface{}{"value": map[string]interface{}{"values": []interface{}{3.0, 4.0}, "dimension": 2.0}}, "vector",
			func(d map[string]interface{}) bool { return d != nil }},
		{map[string]interface{}{"value": map[string]interface{}{"values": []interface{}{}, "rows": 2.0, "cols": 3.0}}, "matrix",
			func(d map[string]interface{}) bool { return d["size"] == 6 && d["is_square"] == false }},
		{map[string]interface{}{"value": map[string]interface{}{"timestamps": []interface{}{}, "values": []interface{}{1.0, 5.0}}}, "timeseries",
			func(d map[string]interface{}) bool { return d["trend"] == "increasing" && d["total_change"] == 4.0 }},
		{map[string]interface{}{"type": "timeseries", "value": map[string]interface{}{"values": []interface{}{5.0, 1.0}}}, "timeseries",
			func(d map[string]interface{}) bool { return d["trend"] == "decreasing" }},
		{map[string]interface{}{"type": "timeseries", "value": map[string]interface{}{"values": []interface{}{}}}, "timeseries",
			func(d map[string]interface{}) bool { return d["length"] == 0 }},
		{map[string]interface{}{"type": "array", "value": map[string]interface{}{"values": []interface{}{1.0, nil, "s", 3}}}, "array",
			func(d map[string]interface{}) bool { return d != nil && d["numeric_count"] == 2 }},
		{map[string]interface{}{"value": []interface{}{1.0}}, "array", func(d map[string]interface{}) bool { return d == nil }},
		{map[string]interface{}{"value": map[string]interface{}{"foo": 1}}, "object", func(d map[string]interface{}) bool { return d == nil }},
		{map[string]interface{}{"value": 21.456}, "float", func(d map[string]interface{}) bool { return d["summary"] == "21.46" }},
		{map[string]interface{}{"type": "int", "value": "7"}, "int", func(d map[string]interface{}) bool { return d["summary"] == "7.00" }},
		{map[string]interface{}{"value": true}, "bool", func(d map[string]interface{}) bool { return d == nil }},
		{map[string]interface{}{"value": "on"}, "string", func(d map[string]interface{}) bool { return d == nil }},
		{map[string]interface{}{"value": nil}, "unknown", func(d map[string]interface{}) bool { return d == nil }},
		{map[string]interface{}{}, "unknown", func(d map[string]interface{}) bool { return d == nil }},
		// 类型声明与值结构不符时不 panic，只是没有派生值
		{map[string]interface{}{"type": "location", "value": 1.0}, "location", func(d map[string]interface{}) bool { return d == nil }},
		{map[string]interface{}{"type": "vector", "value": map[string]interface{}{"values": "x"}}, "vector", func(d map[string]interface{}) bool { return d == nil }},
		{map[string]interface{}{"type": "matrix", "value": "x"}, "matrix", func(d map[string]interface{}) bool { return d == nil }},
		{map[string]interface{}{"type": "color", "value": "x"}, "color", func(d map[string]interface{}) bool { return d == nil }},
		{map[string]interface{}{"type": "vector3d", "value": "x"}, "vector3d", func(d map[string]interface{}) bool { return d == nil }},
		{map[string]interface{}{"type": "array", "value": "x"}, "array", func(d map[string]interface{}) bool { return d == nil }},
		{map[string]interface{}{"type": "timeseries", "value": map[string]interface{}{"values": 1}}, "timeseries", func(d map[string]interface{}) bool { return d == nil }},
	}
	for i, c := range cases {
		out := h.enhanceIoTData(c.data)
		if out["data_type"] != c.wantType {
			t.Errorf("case %d: data_type = %v, want %s", i, out["data_type"], c.wantType)
			continue
		}
		d, _ := out["derived_values"].(map[string]interface{})
		if !c.check(d) {
			t.Errorf("case %d (%s): 派生值错误: %v", i, c.wantType, out["derived_values"])
		}
	}
	for in, want := range map[interface{}]float64{float32(1.5): 1.5, int64(2): 2, int32(3): 3, "4.5": 4.5, "x": 0, nil: 0, 5: 5} {
		if got := toFloat64(in); got != want {
			t.Errorf("toFloat64(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestSmartThrottler(t *testing.T) {
	th := NewSmartThrottler()
	if !th.IsGlobalEnabled() {
		t.Fatal("默认应开启推送")
	}
	th.SetSensorRate("s1", time.Hour)
	if !th.ShouldPush("s1", 1) {
		t.Fatal("第一条应推送")
	}
	if th.ShouldPush("s1", 1) {
		t.Fatal("限流间隔内不应推送")
	}
	if !th.ShouldPush("s1", 2) {
		t.Fatal("高优先级消息不受传感器限流影响")
	}
	th.SetGlobalEnabled(false)
	if th.ShouldPush("s2", 1) || th.IsGlobalEnabled() {
		t.Fatal("全局关闭后不应推送")
	}
	if st := th.GetStats(); st["enabled"] != false || st["push_count"] != int64(2) {
		t.Fatalf("统计错误: %v", st)
	}
}

func TestAlertRulesChannelsAndMonitoringRoutes(t *testing.T) {
	r, _, _ := newNATSRouter(t)
	tok := login(t, r, "admin", "handler-test-pass")

	// 告警：创建 → 查询 → 更新 → 过滤 → 删除
	code, resp := doJSON(t, r, "POST", "/api/v1/alerts", tok, map[string]interface{}{"title": "温度过高", "level": "critical", "source": "rule"})
	if code != 201 {
		t.Fatalf("创建告警失败: %d %s", code, resp.Message)
	}
	var alert models.Alert
	json.Unmarshal(resp.Data, &alert)
	if code, _ := doJSON(t, r, "GET", "/api/v1/alerts/"+alert.ID, tok, nil); code != 200 {
		t.Fatalf("查询告警失败: %d", code)
	}
	if code, _ := doJSON(t, r, "GET", "/api/v1/alerts/nope", tok, nil); code != 404 {
		t.Fatalf("不存在的告警应返回 404, got %d", code)
	}
	if code, _ := doJSON(t, r, "PUT", "/api/v1/alerts/"+alert.ID, tok, map[string]interface{}{"description": "更新", "level": "warning"}); code != 200 {
		t.Fatalf("更新告警失败: %d", code)
	}
	if code, _ := doJSON(t, r, "PUT", "/api/v1/alerts/nope", tok, map[string]interface{}{"title": "x"}); code != 500 {
		t.Fatalf("更新不存在的告警应失败, got %d", code)
	}
	start := time.Now().Add(-time.Hour).Format(time.RFC3339)
	if code, _ := doJSON(t, r, "GET", "/api/v1/alerts?level=warning&status=active&source=rule&search=温度&start_time="+start+"&end_time="+time.Now().Add(time.Hour).Format(time.RFC3339), tok, nil); code != 200 {
		t.Fatalf("过滤告警失败: %d", code)
	}
	for _, p := range []string{"/api/v1/alerts", "/api/v1/alerts/rules", "/api/v1/alerts/channels"} {
		if code, _ := doJSON(t, r, "POST", p, tok, map[string]interface{}{}); code != 400 {
			t.Errorf("POST %s 缺少必填字段应返回 400, got %d", p, code)
		}
	}
	for _, p := range []string{"/api/v1/alerts/x/acknowledge", "/api/v1/alerts/x/resolve"} {
		if code, _ := doJSON(t, r, "POST", p, tok, map[string]string{"comment": "c"}); code != 500 {
			t.Errorf("POST %s 不存在的告警应失败, got %d", p, code)
		}
	}

	// 告警规则
	code, resp = doJSON(t, r, "POST", "/api/v1/alerts/rules", tok, map[string]interface{}{
		"name": "高温", "level": "warning", "enabled": true,
		"condition": map[string]interface{}{"type": "threshold", "field": "temperature", "operator": "gt", "value": 30},
	})
	if code != 201 {
		t.Fatalf("创建告警规则失败: %d %s", code, resp.Message)
	}
	var rule models.AlertRule
	json.Unmarshal(resp.Data, &rule)
	code, resp = doJSON(t, r, "POST", "/api/v1/alerts/rules/"+rule.ID+"/test", tok, map[string]interface{}{"data": map[string]interface{}{"temperature": 35}})
	if code != 200 {
		t.Fatalf("测试告警规则失败: %d %s", code, resp.Message)
	}
	if code, _ := doJSON(t, r, "PUT", "/api/v1/alerts/rules/"+rule.ID, tok, map[string]interface{}{"name": "高温2", "level": "critical"}); code != 200 {
		t.Fatalf("更新告警规则失败: %d", code)
	}
	if code, _ := doJSON(t, r, "GET", "/api/v1/alerts/rules", tok, nil); code != 200 {
		t.Fatalf("告警规则列表失败: %d", code)
	}
	if code, _ := doJSON(t, r, "POST", "/api/v1/alerts/rules/nope/test", tok, map[string]interface{}{"data": map[string]interface{}{}}); code != 500 {
		t.Fatalf("测试不存在的规则应失败, got %d", code)
	}
	if code, _ := doJSON(t, r, "DELETE", "/api/v1/alerts/rules/"+rule.ID, tok, nil); code != 200 {
		t.Fatalf("删除告警规则失败: %d", code)
	}

	// 通知渠道（webhook 指向本地测试服务器）
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer hook.Close()
	code, resp = doJSON(t, r, "POST", "/api/v1/alerts/channels", tok, map[string]interface{}{"name": "hook", "type": "webhook", "enabled": true, "config": map[string]interface{}{"url": hook.URL}})
	if code != 201 {
		t.Fatalf("创建通知渠道失败: %d %s", code, resp.Message)
	}
	var ch models.NotificationChannel
	json.Unmarshal(resp.Data, &ch)
	if code, resp := doJSON(t, r, "POST", "/api/v1/alerts/channels/"+ch.ID+"/test", tok, nil); code != 200 {
		t.Fatalf("测试通知渠道失败: %d %s", code, resp.Message)
	}
	if code, _ := doJSON(t, r, "PUT", "/api/v1/alerts/channels/"+ch.ID, tok, map[string]interface{}{"name": "hook2", "type": "webhook", "config": map[string]interface{}{"url": hook.URL}}); code != 200 {
		t.Fatalf("更新通知渠道失败: %d", code)
	}
	if code, _ := doJSON(t, r, "GET", "/api/v1/alerts/channels", tok, nil); code != 200 {
		t.Fatalf("通知渠道列表失败: %d", code)
	}
	if code, _ := doJSON(t, r, "DELETE", "/api/v1/alerts/channels/"+ch.ID, tok, nil); code != 200 {
		t.Fatalf("删除通知渠道失败: %d", code)
	}

	// 适配器监控（NATS 可用时注册）
	for _, p := range []string{
		"/api/v1/monitoring/adapters/status", "/api/v1/monitoring/adapters/data-flow?time_range=5m&limit=10",
		"/api/v1/monitoring/adapters/mock1/diagnostics", "/api/v1/monitoring/adapters/mock1/performance?period=5m",
	} {
		if code, resp := doJSON(t, r, "GET", p, tok, nil); code != 200 {
			t.Errorf("GET %s = %d %s", p, code, resp.Message)
		}
	}
	if code, _ := doJSON(t, r, "POST", "/api/v1/monitoring/adapters/mock1/test-connection", tok, nil); code != 200 {
		t.Errorf("测试连接失败: %d", code)
	}
	// 回归：以前不论适配器是否存在都返回成功，实际并没有重启
	if code, _ := doJSON(t, r, "POST", "/api/v1/monitoring/adapters/nope/restart", tok, nil); code != 404 {
		t.Errorf("重启不存在的适配器应返回 404, got %d", code)
	}

	// 系统接口
	for _, p := range []string{"/api/v1/system/status", "/api/v1/system/metrics", "/api/v1/system/health", "/api/v1/system/alert-status", "/api/v1/system/config"} {
		if code, resp := doJSON(t, r, "GET", p, tok, nil); code != 200 {
			t.Errorf("GET %s = %d %s", p, code, resp.Message)
		}
	}
	// 轻量级指标由网关主服务的 /metrics 提供
	if code, _ := doJSON(t, r, "GET", "/api/v1/system/metrics/lightweight", tok, nil); code != 501 {
		t.Errorf("轻量级指标应返回 501, got %d", code)
	}
	if code, _ := doJSON(t, r, "POST", "/api/v1/system/restart", tok, map[string]int{"delay": 0}); code != 501 {
		t.Errorf("在线重启不支持，应返回 501, got %d", code)
	}
}
