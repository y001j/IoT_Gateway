package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"
	"github.com/y001j/iot-gateway/internal/web/utils"
)

func newTestRouter(t *testing.T, origins []string) (*gin.Engine, *services.Services) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("IOT_GATEWAY_DATA_DIR", t.TempDir())
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "route-test-pass")
	svc, err := services.NewServices(&services.ServiceConfig{
		Auth: &models.AuthConfig{
			JWTSecret: "routes-test-secret-0123456789", TokenDuration: time.Hour,
			PasswordMinLength: 6, BcryptCost: 4,
		},
		PluginManager: plugin.NewManager(t.TempDir(), nil, viper.New()),
	})
	if err != nil {
		t.Fatalf("创建服务失败: %v", err)
	}
	r := gin.New()
	SetupRoutes(r, svc, nil, RouteOptions{AllowedOrigins: origins})
	return r, svc
}

func tokenFor(t *testing.T, svc *services.Services, role string) string {
	t.Helper()
	// 令牌必须对应一个有效会话（中间件会校验会话），所以创建真实用户并登录
	username := "user_" + role
	if _, err := svc.Auth.Login(&models.LoginRequest{Username: username, Password: "role-pass-123"}); err != nil {
		if err := svc.Auth.CreateUser(&models.User{Username: username, Password: "role-pass-123", Role: role, Status: "active", Email: username + "@example.com"}); err != nil {
			t.Fatalf("创建用户失败: %v", err)
		}
	}
	resp, err := svc.Auth.Login(&models.LoginRequest{Username: username, Password: "role-pass-123"})
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	return resp.Token
}

func TestTokenRevokedAfterLogoutAndForgedTokenRejected(t *testing.T) {
	r, svc := newTestRouter(t, nil)
	tok := tokenFor(t, svc, "viewer")
	if code := call(r, "GET", "/api/v1/plugins", tok, nil); code != http.StatusOK {
		t.Fatalf("有效令牌应可访问, got %d", code)
	}
	if code := call(r, "POST", "/api/v1/auth/logout", tok, nil); code != http.StatusOK {
		t.Fatalf("登出失败: %d", code)
	}
	// 以前 JWT 不检查会话，登出后令牌在过期前仍然有效
	if code := call(r, "GET", "/api/v1/plugins", tok, nil); code != http.StatusUnauthorized {
		t.Fatalf("登出后令牌应失效, got %d", code)
	}
	// 签名正确但没有对应会话的令牌（例如密钥泄漏后伪造）也应被拒绝
	forged, _ := utils.GenerateJWT(1, "admin", "admin", &utils.JWTConfig{SecretKey: svc.Auth.JWTSecret(), TokenDuration: time.Hour})
	if code := call(r, "GET", "/api/v1/plugins", forged, nil); code != http.StatusUnauthorized {
		t.Fatalf("没有会话的令牌应被拒绝, got %d", code)
	}
}

func call(r http.Handler, method, path, tok string, headers map[string]string) int {
	req := httptest.NewRequest(method, path, bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestRoutesRequireAuthentication(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	for _, path := range []string{"/api/v1/plugins", "/api/v1/alerts", "/api/v1/system/status", "/api/v1/plugins/rules"} {
		if code := call(r, "GET", path, "", nil); code != http.StatusUnauthorized {
			t.Errorf("GET %s 未登录应返回 401, got %d", path, code)
		}
	}
	if code := call(r, "GET", "/health", "", nil); code != http.StatusOK {
		t.Errorf("/health 无需认证, got %d", code)
	}
}

func TestWriteRoutesRequireOperator(t *testing.T) {
	r, svc := newTestRouter(t, nil)
	viewer := tokenFor(t, svc, "viewer")
	operator := tokenFor(t, svc, "operator")

	writes := []struct{ method, path string }{
		{"POST", "/api/v1/plugins/x/start"},
		{"POST", "/api/v1/plugins/x/stop"},
		{"POST", "/api/v1/plugins/x/restart"},
		{"POST", "/api/v1/plugins/rules"},
		{"PUT", "/api/v1/plugins/rules/x"},
		{"DELETE", "/api/v1/plugins/rules/x"},
		{"POST", "/api/v1/plugins/rules/x/enable"},
		{"POST", "/api/v1/alerts"},
		{"POST", "/api/v1/alerts/x/acknowledge"},
		{"DELETE", "/api/v1/alerts/x"},
		{"POST", "/api/v1/alerts/rules"},
	}
	for _, w := range writes {
		if code := call(r, w.method, w.path, viewer, nil); code != http.StatusForbidden {
			t.Errorf("viewer %s %s 应返回 403, got %d", w.method, w.path, code)
		}
		if code := call(r, w.method, w.path, operator, nil); code == http.StatusForbidden || code == http.StatusUnauthorized {
			t.Errorf("operator %s %s 不应被拒绝, got %d", w.method, w.path, code)
		}
	}
	// 读操作对普通用户开放
	if code := call(r, "GET", "/api/v1/plugins", viewer, nil); code == http.StatusForbidden || code == http.StatusUnauthorized {
		t.Errorf("viewer 读插件列表不应被拒绝, got %d", code)
	}
}

func TestNotificationChannelsAdminOnly(t *testing.T) {
	r, svc := newTestRouter(t, nil)
	operator := tokenFor(t, svc, "operator")
	admin := tokenFor(t, svc, "admin")
	if code := call(r, "GET", "/api/v1/alerts/channels", operator, nil); code != http.StatusForbidden {
		t.Errorf("通知渠道包含敏感信息，operator 应返回 403, got %d", code)
	}
	if code := call(r, "GET", "/api/v1/alerts/channels", admin, nil); code == http.StatusForbidden {
		t.Errorf("admin 应可访问通知渠道, got %d", code)
	}
	if code := call(r, "PUT", "/api/v1/system/config", operator, nil); code != http.StatusForbidden {
		t.Errorf("修改系统配置仅限 admin, got %d", code)
	}
}

func TestLoginAndCORS(t *testing.T) {
	r, _ := newTestRouter(t, []string{"http://ui.local"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"route-test-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://ui.local")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("登录失败: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "http://ui.local" {
		t.Fatalf("白名单来源应返回 CORS 头")
	}

	req = httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.com")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Fatalf("默认密码 admin123 不应能登录")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("非白名单来源不应返回 CORS 头")
	}
}
