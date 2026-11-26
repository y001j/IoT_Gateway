package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/testutil"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"
)

// newPluginRouter 创建带一个运行中的 mock 适配器的路由
func newPluginRouter(t *testing.T) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("IOT_GATEWAY_DATA_DIR", t.TempDir())
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "handler-test-pass")
	_, nc := testutil.StartNATS(t)
	v := viper.New()
	v.Set("southbound.adapters", []interface{}{map[string]interface{}{
		"name": "mock1", "type": "mock",
		"config": map[string]interface{}{"name": "mock1", "type": "mock", "interval": "50ms",
			"data_points": []interface{}{map[string]interface{}{"device_id": "s1", "key": "temp", "min_value": 1.0, "max_value": 2.0}}},
	}})
	pm := plugin.NewManager(t.TempDir(), nc, v)
	if err := pm.Init(nil); err != nil {
		t.Fatal(err)
	}
	if err := pm.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pm.Stop(context.Background()) })
	mgr := rules.NewManager(t.TempDir())
	_ = mgr.LoadRules()
	t.Cleanup(func() { mgr.Close() })
	svc, err := services.NewServices(&services.ServiceConfig{
		Auth: &models.AuthConfig{JWTSecret: "plugin-test-secret-0123456789", TokenDuration: time.Hour,
			PasswordMinLength: 6, BcryptCost: 4, MaxLoginAttempts: 5, LockoutDuration: time.Minute},
		PluginManager: pm,
		RuleManager:   mgr,
		NATSConn:      nc,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	SetupRoutes(r, svc, nc)
	return r
}

func TestPluginDetailEndpoints(t *testing.T) {
	r := newPluginRouter(t)
	tok := login(t, r, "admin", "handler-test-pass")

	for _, path := range []string{"/api/v1/plugins/mock1/config", "/api/v1/plugins/mock1/logs?page=1&page_size=10", "/api/v1/plugins/mock1/stats"} {
		if code, resp := doJSON(t, r, "GET", path, tok, nil); code != 200 {
			t.Errorf("GET %s = %d %s", path, code, resp.Message)
		}
	}
	code, resp := doJSON(t, r, "POST", "/api/v1/plugins/mock1/config/validate", tok, map[string]interface{}{"config": map[string]interface{}{"name": "mock1", "type": "mock"}})
	if code != 200 {
		t.Fatalf("validate = %d %s", code, resp.Message)
	}
	// 尚未支持的操作返回错误而不是假装成功
	if code, _ := doJSON(t, r, "PUT", "/api/v1/plugins/mock1/config", tok, map[string]interface{}{"config": map[string]interface{}{"interval": "1s"}}); code == 200 {
		t.Error("更新插件配置尚未实现，应返回错误")
	}
	if code, _ := doJSON(t, r, "DELETE", "/api/v1/plugins/mock1", tok, nil); code == 200 {
		t.Error("删除插件尚未实现，应返回错误")
	}
	// 不存在的插件、错误的请求体
	for _, path := range []string{"/api/v1/plugins/nope/config", "/api/v1/plugins/nope/stats"} {
		if code, _ := doJSON(t, r, "GET", path, tok, nil); code == 200 {
			t.Errorf("%s 不存在的插件应返回错误", path)
		}
	}
	req := httptest.NewRequest("POST", "/api/v1/plugins/mock1/config/validate", strings.NewReader("{bad"))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == 200 {
		t.Error("非法 JSON 应返回错误")
	}
}

func TestSystemConfigValidateDoesNotSave(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("IOT_GATEWAY_DATA_DIR", t.TempDir())
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "handler-test-pass")
	mgr := rules.NewManager(t.TempDir())
	_ = mgr.LoadRules()
	t.Cleanup(func() { mgr.Close() })
	svc, err := services.NewServices(&services.ServiceConfig{
		Auth: &models.AuthConfig{JWTSecret: "config-test-secret-0123456789", TokenDuration: time.Hour,
			PasswordMinLength: 6, BcryptCost: 4, MaxLoginAttempts: 5, LockoutDuration: time.Minute},
		PluginManager: plugin.NewManager(t.TempDir(), nil, viper.New()),
		RuleManager:   mgr,
		ConfigPath:    filepath.Join(t.TempDir(), "system.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	SetupRoutes(r, svc, nil)
	tok := login(t, r, "admin", "handler-test-pass")
	code, resp := doJSON(t, r, "GET", "/api/v1/system/config", tok, nil)
	if code != 200 {
		t.Fatalf("GET config = %d", code)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(resp.Data, &cfg); err != nil {
		t.Fatal(err)
	}
	before, _ := svc.System.GetConfig()

	// 非法配置：校验返回 valid=false，配置不变（以前"校验"会直接保存并应用）
	gw := cfg["gateway"].(map[string]interface{})
	gw["http_port"] = 70000.0
	gw["plugins_dir"] = "/changed/by/validate"
	code, resp = doJSON(t, r, "POST", "/api/v1/system/config/validate", tok, cfg)
	if code != 200 {
		t.Fatalf("validate = %d %s", code, resp.Message)
	}
	var vr models.ConfigValidationResponse
	json.Unmarshal(resp.Data, &vr)
	if vr.Valid || len(vr.Errors) == 0 {
		t.Fatalf("非法端口应校验失败: %+v", vr)
	}
	// 合法配置：校验通过，但仍不保存
	gw["http_port"] = 8080.0
	code, resp = doJSON(t, r, "POST", "/api/v1/system/config/validate", tok, cfg)
	json.Unmarshal(resp.Data, &vr)
	if code != 200 || !vr.Valid {
		t.Fatalf("合法配置应通过校验: %d %+v %s", code, vr, resp.Message)
	}
	after, _ := svc.System.GetConfig()
	if after.Gateway.PluginsDir != before.Gateway.PluginsDir {
		t.Fatalf("校验不应修改配置: %q -> %q", before.Gateway.PluginsDir, after.Gateway.PluginsDir)
	}

	// 更新：非法配置被拒绝，合法配置保存
	gw["http_port"] = 70000.0
	if code, _ := doJSON(t, r, "PUT", "/api/v1/system/config", tok, cfg); code == 200 {
		t.Fatal("非法配置不应保存")
	}
	gw["http_port"] = 8080.0
	if code, resp := doJSON(t, r, "PUT", "/api/v1/system/config", tok, cfg); code != 200 {
		t.Fatalf("PUT config = %d %s", code, resp.Message)
	}
	if after, _ := svc.System.GetConfig(); after.Gateway.PluginsDir != "/changed/by/validate" {
		t.Fatalf("更新后应保存: %q", after.Gateway.PluginsDir)
	}
	// 非管理员不能修改或校验
	op := tokenFor(t, svc, "operator")
	for _, m := range []string{"PUT /api/v1/system/config", "POST /api/v1/system/config/validate"} {
		parts := strings.SplitN(m, " ", 2)
		if code, _ := doJSON(t, r, parts[0], parts[1], op, cfg); code != http.StatusForbidden {
			t.Errorf("%s 非管理员应返回 403, got %d", m, code)
		}
	}
}

func TestAuthRefreshEndpoint(t *testing.T) {
	r, _ := newFullRouter(t)
	// 勾选"记住我"时才签发刷新令牌
	code, resp := doJSON(t, r, "POST", "/api/v1/auth/login", "", map[string]interface{}{"username": "admin", "password": "handler-test-pass", "remember": true})
	if code != 200 {
		t.Fatal("登录失败")
	}
	var lr struct {
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}
	json.Unmarshal(resp.Data, &lr)
	if lr.RefreshToken == "" {
		t.Fatal("remember=true 时应返回 refresh_token")
	}
	code, resp = doJSON(t, r, "POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": lr.RefreshToken})
	if code != 200 {
		t.Fatalf("refresh = %d %s", code, resp.Message)
	}
	var rr struct {
		Token string `json:"token"`
	}
	json.Unmarshal(resp.Data, &rr)
	if rr.Token == "" {
		t.Fatal("刷新后应返回新令牌")
	}
	if code, _ := doJSON(t, r, "GET", "/api/v1/auth/profile", rr.Token, nil); code != 200 {
		t.Fatalf("新令牌应可用: %d", code)
	}
	// 旧会话已删除：旧访问令牌失效，旧刷新令牌不能再用
	if code, _ := doJSON(t, r, "GET", "/api/v1/auth/profile", lr.Token, nil); code != http.StatusUnauthorized {
		t.Fatalf("刷新后旧令牌应失效: %d", code)
	}
	if code, _ := doJSON(t, r, "POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": lr.RefreshToken}); code == 200 {
		t.Fatal("旧刷新令牌不应能重复使用")
	}
	for _, body := range []interface{}{map[string]string{"refresh_token": "forged"}, map[string]string{}} {
		if code, _ := doJSON(t, r, "POST", "/api/v1/auth/refresh", "", body); code == 200 {
			t.Errorf("%v 应被拒绝", body)
		}
	}
}

func TestWebSocketInactiveCleanup(t *testing.T) {
	_, svc := newFullRouter(t)
	h := NewWebSocketHandler(svc, nil)
	up := websocket.Upgrader{}
	serverConns := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConns <- c
	}))
	defer srv.Close()
	clientConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()
	sc := <-serverConns

	stale := &WebSocketClient{ID: "stale", Conn: sc, UserID: "u1", Send: make(chan []byte, 1), Hub: h, messageCount: 42}
	stale.lastActivity.Store(time.Now().Add(-time.Hour).UnixNano())
	fresh := &WebSocketClient{ID: "fresh", Conn: sc, UserID: "u2", Send: make(chan []byte, 1), Hub: h, messageCount: 7}
	fresh.touch()
	h.mutex.Lock()
	h.clients["stale"], h.clients["fresh"] = stale, fresh
	h.connectionStats["u1"], h.connectionStats["u2"] = 1, 1
	h.mutex.Unlock()

	h.resetMessageCounters()
	if fresh.messageCount != 0 {
		t.Fatal("消息计数应重置")
	}
	h.cleanupInactiveClients()
	h.mutex.RLock()
	_, staleLeft := h.clients["stale"]
	_, freshLeft := h.clients["fresh"]
	stats := h.connectionStats["u1"]
	h.mutex.RUnlock()
	if staleLeft || !freshLeft || stats != 0 {
		t.Fatalf("应只清理不活跃连接: stale=%v fresh=%v stats=%d", staleLeft, freshLeft, stats)
	}
	if _, ok := <-stale.Send; ok {
		t.Fatal("清理后发送通道应关闭")
	}
	// 读协程退出时再注销不会重复关闭通道
	h.unregisterClient(stale)
}
