package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/utils"
)

func testAuthConfig(secret string) *models.AuthConfig {
	return &models.AuthConfig{
		JWTSecret:         secret,
		TokenDuration:     time.Hour,
		RefreshDuration:   24 * time.Hour,
		PasswordMinLength: 6,
		BcryptCost:        4,
	}
}

func TestMemoryStoreAdminPasswordFromEnv(t *testing.T) {
	t.Setenv("IOT_GATEWAY_DATA_DIR", t.TempDir())
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "env-admin-pass")
	svc := NewAuthService(NewMemoryStore(), testAuthConfig("auth-service-test-secret-1"))

	if _, err := svc.Login(&models.LoginRequest{Username: "admin", Password: "admin123"}); err == nil {
		t.Fatalf("默认密码 admin123 不应再能登录")
	}
	resp, err := svc.Login(&models.LoginRequest{Username: "admin", Password: "env-admin-pass"})
	if err != nil {
		t.Fatalf("使用环境变量设置的密码登录失败: %v", err)
	}
	if resp.Token == "" || resp.User == nil || resp.User.Role != "admin" {
		t.Fatalf("登录响应错误: %+v", resp)
	}
	// 签发的令牌必须能用 JWTSecret() 校验（中间件使用同一个密钥）
	if _, err := utils.ValidateJWT(resp.Token, svc.JWTSecret()); err != nil {
		t.Fatalf("令牌无法用服务密钥校验: %v", err)
	}
	if _, err := svc.ValidateToken(resp.Token); err != nil {
		t.Fatalf("会话校验失败: %v", err)
	}
}

func TestMemoryStoreGeneratesRandomAdminPassword(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("IOT_GATEWAY_DATA_DIR", dir)
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "")
	svc := NewAuthService(NewMemoryStore(), testAuthConfig("auth-service-test-secret-2"))
	data, err := os.ReadFile(filepath.Join(dir, "initial_admin_password"))
	if err != nil {
		t.Fatalf("应生成初始密码文件: %v", err)
	}
	pw := strings.TrimSpace(string(data))
	if _, err := svc.Login(&models.LoginRequest{Username: "admin", Password: pw}); err != nil {
		t.Fatalf("应能使用生成的初始密码登录: %v", err)
	}
}

func TestLoginErrorsDoNotRevealUserExistence(t *testing.T) {
	t.Setenv("IOT_GATEWAY_DATA_DIR", t.TempDir())
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "env-admin-pass")
	svc := NewAuthService(NewMemoryStore(), testAuthConfig("auth-service-test-secret-3"))
	_, errNoUser := svc.Login(&models.LoginRequest{Username: "nobody", Password: "x"})
	_, errBadPw := svc.Login(&models.LoginRequest{Username: "admin", Password: "wrong"})
	if errNoUser == nil || errBadPw == nil {
		t.Fatalf("应返回登录失败")
	}
	if errNoUser.Error() != errBadPw.Error() {
		t.Fatalf("用户不存在和密码错误的提示不能不同（用户名枚举）: %q vs %q", errNoUser, errBadPw)
	}
}

func TestWeakJWTSecretIsReplaced(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("IOT_GATEWAY_DATA_DIR", dir)
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "env-admin-pass")
	svc := NewAuthService(NewMemoryStore(), testAuthConfig(utils.InsecureDefaultJWTSecret))
	if svc.JWTSecret() == utils.InsecureDefaultJWTSecret || utils.IsWeakJWTSecret(svc.JWTSecret()) {
		t.Fatalf("占位密钥应被替换")
	}
	// 用公开的占位密钥伪造的令牌不能通过校验
	forged, _ := utils.GenerateJWT(1, "admin", "admin", &utils.JWTConfig{SecretKey: utils.InsecureDefaultJWTSecret, TokenDuration: time.Hour})
	if _, err := utils.ValidateJWT(forged, svc.JWTSecret()); err == nil {
		t.Fatalf("用默认占位密钥伪造的令牌通过了校验")
	}
	// nil 配置同样使用安全密钥
	if utils.IsWeakJWTSecret(NewAuthService(NewMemoryStore(), nil).JWTSecret()) {
		t.Fatalf("默认配置不应使用弱密钥")
	}
}

func TestDataDir(t *testing.T) {
	t.Setenv("IOT_GATEWAY_DATA_DIR", "")
	if DataDir() != "./data" {
		t.Fatalf("默认数据目录错误")
	}
	t.Setenv("IOT_GATEWAY_DATA_DIR", "/tmp/x")
	if DataDir() != "/tmp/x" {
		t.Fatalf("环境变量未生效")
	}
}
