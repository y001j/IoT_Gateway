package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"
)

type apiResp struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func newFullRouter(t *testing.T) (*gin.Engine, *services.Services) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("IOT_GATEWAY_DATA_DIR", t.TempDir())
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "handler-test-pass")
	mgr := rules.NewManager(t.TempDir())
	if err := mgr.LoadRules(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mgr.Close() })
	svc, err := services.NewServices(&services.ServiceConfig{
		Auth: &models.AuthConfig{JWTSecret: "handler-test-secret-0123456789", TokenDuration: time.Hour,
			PasswordMinLength: 6, BcryptCost: 4, MaxLoginAttempts: 5, LockoutDuration: time.Minute},
		PluginManager: plugin.NewManager(t.TempDir(), nil, viper.New()),
		RuleManager:   mgr,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	SetupRoutes(r, svc, nil)
	return r, svc
}

func doJSON(t *testing.T, r http.Handler, method, path, tok string, body interface{}) (int, apiResp) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp apiResp
	json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func login(t *testing.T, r http.Handler, user, pass string) string {
	t.Helper()
	code, resp := doJSON(t, r, "POST", "/api/v1/auth/login", "", map[string]string{"username": user, "password": pass})
	if code != 200 {
		t.Fatalf("登录失败: %d %s", code, resp.Message)
	}
	var data struct {
		Token string `json:"token"`
	}
	json.Unmarshal(resp.Data, &data)
	if data.Token == "" {
		t.Fatalf("登录响应中没有 token: %s", resp.Data)
	}
	return data.Token
}

func TestAuthHandlersProfileAndPassword(t *testing.T) {
	r, _ := newFullRouter(t)
	tok := login(t, r, "admin", "handler-test-pass")

	code, resp := doJSON(t, r, "GET", "/api/v1/auth/profile", tok, nil)
	if code != 200 || !bytes.Contains(resp.Data, []byte(`"username":"admin"`)) {
		t.Fatalf("获取资料失败: %d %s", code, resp.Data)
	}
	if code, _ := doJSON(t, r, "PUT", "/api/v1/auth/profile", tok, map[string]string{"email": "ops@example.com", "display_name": "运维"}); code != 200 {
		t.Fatalf("更新资料失败: %d", code)
	}
	if code, _ := doJSON(t, r, "PUT", "/api/v1/auth/password", tok, map[string]string{"old_password": "wrong", "new_password": "new-pass-123"}); code == 200 {
		t.Fatalf("旧密码错误时不应修改成功")
	}
	if code, resp := doJSON(t, r, "PUT", "/api/v1/auth/password", tok, map[string]string{"old_password": "handler-test-pass", "new_password": "new-pass-123"}); code != 200 {
		t.Fatalf("修改密码失败: %d %s", code, resp.Message)
	}
	// 修改密码后，修改前签发的令牌应失效
	if code, _ := doJSON(t, r, "GET", "/api/v1/auth/profile", tok, nil); code != http.StatusUnauthorized {
		t.Fatalf("修改密码后旧令牌应失效, got %d", code)
	}
	tok = login(t, r, "admin", "new-pass-123")
	if code, _ := doJSON(t, r, "POST", "/api/v1/auth/login", "", map[string]string{"username": "admin", "password": "handler-test-pass"}); code == 200 {
		t.Fatalf("修改后旧密码不应再能登录")
	}
	if code, _ := doJSON(t, r, "POST", "/api/v1/auth/logout", tok, nil); code != 200 {
		t.Fatalf("登出失败: %d", code)
	}
	if code, _ := doJSON(t, r, "POST", "/api/v1/auth/login", "", map[string]string{"username": "admin"}); code != 400 {
		t.Fatalf("缺少密码应返回 400, got %d", code)
	}
}

func TestRuleHandlersCRUD(t *testing.T) {
	r, _ := newFullRouter(t)
	tok := login(t, r, "admin", "handler-test-pass")

	body := map[string]interface{}{
		"name": "HTTP 规则", "enabled": true, "priority": 3,
		"conditions": map[string]interface{}{"field": "value", "operator": "gt", "value": 10},
		"actions":    []interface{}{map[string]interface{}{"type": "alert", "config": map[string]interface{}{"level": "info"}}},
	}
	code, resp := doJSON(t, r, "POST", "/api/v1/plugins/rules", tok, body)
	if code != http.StatusCreated {
		t.Fatalf("创建规则失败: %d %s", code, resp.Message)
	}
	var created models.Rule
	json.Unmarshal(resp.Data, &created)
	if created.ID == "" {
		t.Fatalf("响应中没有规则 ID: %s", resp.Data)
	}
	base := "/api/v1/plugins/rules/" + created.ID

	if code, _ := doJSON(t, r, "GET", base, tok, nil); code != 200 {
		t.Fatalf("获取规则失败: %d", code)
	}
	if code, _ := doJSON(t, r, "POST", base+"/disable", tok, nil); code != 200 {
		t.Fatalf("禁用失败: %d", code)
	}
	if code, _ := doJSON(t, r, "POST", base+"/enable", tok, nil); code != 200 {
		t.Fatalf("启用失败: %d", code)
	}
	if code, _ := doJSON(t, r, "PUT", base, tok, map[string]interface{}{"name": "改名后"}); code != 200 {
		t.Fatalf("更新失败: %d", code)
	}
	code, resp = doJSON(t, r, "GET", "/api/v1/plugins/rules?page=1&page_size=10", tok, nil)
	if code != 200 || !bytes.Contains(resp.Data, []byte("改名后")) {
		t.Fatalf("规则列表错误: %d %s", code, resp.Data)
	}
	if code, _ := doJSON(t, r, "DELETE", base, tok, nil); code != 200 && code != http.StatusNoContent {
		t.Fatalf("删除失败: %d", code)
	}
	if code, _ := doJSON(t, r, "GET", base, tok, nil); code == 200 {
		t.Fatalf("删除后不应能获取")
	}
	if code, _ := doJSON(t, r, "POST", "/api/v1/plugins/rules", tok, map[string]interface{}{"name": "x"}); code == http.StatusCreated {
		t.Fatalf("缺少条件和动作的规则不应创建成功")
	}
}

func TestAlertHandlersLifecycle(t *testing.T) {
	r, _ := newFullRouter(t)
	tok := login(t, r, "admin", "handler-test-pass")
	code, resp := doJSON(t, r, "POST", "/api/v1/alerts", tok, map[string]interface{}{"title": "HTTP 告警", "level": "warning", "source": "test"})
	if code != 200 && code != 201 {
		t.Fatalf("创建告警失败: %d %s", code, resp.Message)
	}
	var created models.Alert
	json.Unmarshal(resp.Data, &created)
	if created.ID == "" {
		t.Fatalf("响应中没有告警 ID: %s", resp.Data)
	}
	// 回归：确认/解决接口以前因 user_id 类型断言必然 panic
	if code, resp := doJSON(t, r, "POST", "/api/v1/alerts/"+created.ID+"/acknowledge", tok, map[string]string{"comment": "收到"}); code != 200 {
		t.Fatalf("确认告警失败: %d %s", code, resp.Message)
	}
	if code, _ := doJSON(t, r, "POST", "/api/v1/alerts/"+created.ID+"/resolve", tok, map[string]string{"comment": "已处理"}); code != 200 {
		t.Fatalf("解决告警失败: %d", code)
	}
	if code, _ := doJSON(t, r, "GET", "/api/v1/alerts/stats", tok, nil); code != 200 {
		t.Fatalf("告警统计失败: %d", code)
	}
	if code, _ := doJSON(t, r, "GET", "/api/v1/alerts?page=1&page_size=10", tok, nil); code != 200 {
		t.Fatalf("告警列表失败: %d", code)
	}
	if code, _ := doJSON(t, r, "DELETE", "/api/v1/alerts/"+created.ID, tok, nil); code != 200 {
		t.Fatalf("删除告警失败: %d", code)
	}
}

func TestSystemAndPluginHandlers(t *testing.T) {
	r, _ := newFullRouter(t)
	tok := login(t, r, "admin", "handler-test-pass")
	for _, path := range []string{"/api/v1/system/status", "/api/v1/system/health", "/api/v1/plugins", "/api/v1/websocket/stats"} {
		if code, resp := doJSON(t, r, "GET", path, tok, nil); code != 200 {
			t.Errorf("GET %s = %d %s", path, code, resp.Message)
		}
	}
	if code, _ := doJSON(t, r, "GET", "/api/v1/plugins/no-such-plugin", tok, nil); code == 200 {
		t.Errorf("不存在的插件应返回错误")
	}
}
