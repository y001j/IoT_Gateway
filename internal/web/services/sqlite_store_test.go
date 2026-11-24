package services

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/web/models"
)

func TestSQLiteStoreAuthFlow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("IOT_GATEWAY_DATA_DIR", dir)
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "sqlite-admin-pass")
	store, err := NewSQLiteStore(filepath.Join(dir, "auth.db"))
	if err != nil {
		t.Fatalf("创建 SQLite 存储失败: %v", err)
	}
	svc := NewAuthService(store, testAuthConfig("sqlite-test-secret-1234567"))

	resp, err := svc.Login(&models.LoginRequest{Username: "admin", Password: "sqlite-admin-pass", Remember: true})
	if err != nil {
		t.Fatalf("默认管理员登录失败: %v", err)
	}
	// 回归：SQLite 存储读出的会话 Active 恒为 false，ValidateToken 必然失败
	if _, err := svc.ValidateToken(resp.Token); err != nil {
		t.Fatalf("SQLite 会话校验失败: %v", err)
	}
	if resp.RefreshToken != "" {
		if _, err := svc.RefreshToken(&models.RefreshTokenRequest{RefreshToken: resp.RefreshToken}); err != nil {
			t.Fatalf("刷新令牌失败: %v", err)
		}
	}

	// 登出后会话失效
	resp2, _ := svc.Login(&models.LoginRequest{Username: "admin", Password: "sqlite-admin-pass"})
	if err := svc.Logout(resp2.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateToken(resp2.Token); err == nil {
		t.Fatalf("登出后会话应失效（以前 SQLite 存储不保存 active 状态）")
	}
	if err := svc.Logout(resp2.Token); err != nil {
		t.Fatalf("重复登出应幂等: %v", err)
	}

	// 修改密码后所有会话失效
	resp3, _ := svc.Login(&models.LoginRequest{Username: "admin", Password: "sqlite-admin-pass"})
	if err := svc.ChangePassword(resp3.User.ID, &models.ChangePasswordRequest{OldPassword: "sqlite-admin-pass", NewPassword: "changed-pass-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateToken(resp3.Token); err == nil {
		t.Fatalf("修改密码后旧会话应失效")
	}

	// 用户管理与持久化
	if err := svc.CreateUser(&models.User{Username: "op", Password: "operator-pass", Role: "operator", Status: "active", Email: "op@example.com"}); err != nil {
		t.Fatal(err)
	}
	users, total, err := svc.ListUsers(0, 10)
	if err != nil || total != 2 || len(users) != 2 {
		t.Fatalf("用户列表错误: %d %v", total, err)
	}
	if err := svc.CreateUser(&models.User{Username: "short", Password: "123"}); err == nil {
		t.Fatalf("过短的密码应被拒绝")
	}

	// 重新打开数据库：数据持久化，且不会重复创建管理员
	store2, err := NewSQLiteStore(filepath.Join(dir, "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc2 := NewAuthService(store2, testAuthConfig("sqlite-test-secret-1234567"))
	if _, err := svc2.Login(&models.LoginRequest{Username: "admin", Password: "changed-pass-1"}); err != nil {
		t.Fatalf("修改后的密码应持久化: %v", err)
	}
	if _, total, _ := svc2.ListUsers(0, 10); total != 2 {
		t.Fatalf("重新打开后用户数 = %d", total)
	}
	if err := store2.CleanExpiredSessions(); err != nil {
		t.Fatal(err)
	}
	_ = time.Now
}
