package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/y001j/iot-gateway/internal/web/utils"
)

const secret = "middleware-test-secret-123456"

func init() { gin.SetMode(gin.TestMode) }

func token(t *testing.T, role string) string {
	t.Helper()
	s, err := utils.GenerateJWT(1, "user", role, &utils.JWTConfig{SecretKey: secret, TokenDuration: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newRouter() *gin.Engine {
	r := gin.New()
	g := r.Group("/api", JWTMiddleware(secret))
	g.GET("/read", func(c *gin.Context) { c.String(200, c.GetString("username")) })
	g.POST("/write", RequireRole("operator"), func(c *gin.Context) { c.String(200, "ok") })
	g.POST("/admin", RequireRole("admin"), func(c *gin.Context) { c.String(200, "ok") })
	return r
}

func do(r http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestJWTMiddleware(t *testing.T) {
	r := newRouter()
	if w := do(r, "GET", "/api/read", nil); w.Code != 401 {
		t.Fatalf("无令牌应返回 401, got %d", w.Code)
	}
	if w := do(r, "GET", "/api/read", map[string]string{"Authorization": "Bearer bad"}); w.Code != 401 {
		t.Fatalf("非法令牌应返回 401, got %d", w.Code)
	}
	w := do(r, "GET", "/api/read", map[string]string{"Authorization": "Bearer " + token(t, "viewer")})
	if w.Code != 200 || w.Body.String() != "user" {
		t.Fatalf("合法令牌应通过: %d %s", w.Code, w.Body.String())
	}
	if w := do(r, "GET", "/api/read", map[string]string{"Authorization": "bearer " + token(t, "viewer")}); w.Code != 200 {
		t.Fatalf("Bearer 前缀应不区分大小写")
	}
}

func TestQueryTokenOnlyForWebSocket(t *testing.T) {
	r := newRouter()
	path := "/api/read?token=" + token(t, "viewer")
	// 普通请求不允许通过 URL 传递令牌（会被记录进访问日志、浏览器历史）
	if w := do(r, "GET", path, nil); w.Code != 401 {
		t.Fatalf("普通请求不应接受查询参数中的令牌, got %d", w.Code)
	}
	if w := do(r, "GET", path, map[string]string{"Upgrade": "websocket"}); w.Code != 200 {
		t.Fatalf("WebSocket 升级请求应接受查询参数中的令牌, got %d", w.Code)
	}
}

func TestRequireRole(t *testing.T) {
	r := newRouter()
	cases := []struct {
		role, path string
		want       int
	}{
		{"viewer", "/api/write", 403},
		{"user", "/api/write", 403},
		{"operator", "/api/write", 200},
		{"admin", "/api/write", 200},
		{"administrator", "/api/write", 200},
		{"operator", "/api/admin", 403},
		{"admin", "/api/admin", 200},
	}
	for _, c := range cases {
		w := do(r, "POST", c.path, map[string]string{"Authorization": "Bearer " + token(t, c.role)})
		if w.Code != c.want {
			t.Errorf("角色 %s 访问 %s = %d, want %d", c.role, c.path, w.Code, c.want)
		}
	}
	// 未经过 JWT 中间件（上下文中没有角色）
	bare := gin.New()
	bare.GET("/x", RequireRole("admin"), func(c *gin.Context) {})
	if w := do(bare, "GET", "/x", nil); w.Code != 401 {
		t.Fatalf("没有角色信息应返回 401")
	}
}

func TestGinCORS(t *testing.T) {
	newR := func(allowed []string) *gin.Engine {
		r := gin.New()
		r.Use(GinCORS(allowed))
		r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })
		return r
	}

	// 默认不允许跨域
	w := do(newR(nil), "GET", "/x", map[string]string{"Origin": "http://evil.com"})
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("未配置白名单时不应返回 CORS 头")
	}
	if w.Code != 200 {
		t.Fatalf("请求本身应正常处理")
	}

	// 白名单内：回显 origin 并允许凭证
	w = do(newR([]string{"http://ui.local"}), "GET", "/x", map[string]string{"Origin": "http://ui.local"})
	if w.Header().Get("Access-Control-Allow-Origin") != "http://ui.local" || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("白名单来源应被允许: %v", w.Header())
	}

	// 通配：不能同时允许凭证
	w = do(newR([]string{"*"}), "GET", "/x", map[string]string{"Origin": "http://any.com"})
	if w.Header().Get("Access-Control-Allow-Origin") != "*" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("通配时不应允许凭证: %v", w.Header())
	}

	// 预检请求
	w = do(newR([]string{"http://ui.local"}), "OPTIONS", "/x", map[string]string{"Origin": "http://ui.local"})
	if w.Code != http.StatusNoContent {
		t.Fatalf("预检请求应返回 204, got %d", w.Code)
	}
}

func TestCORSWithConfigMaxAge(t *testing.T) {
	h := CORSWithConfig(DefaultCORSConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	// 以前 string(rune(86400)) 会生成一个乱码字符
	if got := w.Header().Get("Access-Control-Max-Age"); got != "86400" {
		t.Fatalf("Max-Age = %q", got)
	}
}
