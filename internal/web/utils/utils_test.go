package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateAndValidateJWT(t *testing.T) {
	cfg := &JWTConfig{SecretKey: "0123456789abcdef-secret", TokenDuration: time.Hour}
	tok, err := GenerateJWT(7, "alice", "operator", cfg)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidateJWT(tok, cfg.SecretKey)
	if err != nil {
		t.Fatalf("合法令牌校验失败: %v", err)
	}
	if claims["username"] != "alice" || claims["role"] != "operator" || claims["user_id"].(float64) != 7 {
		t.Fatalf("claims 错误: %v", claims)
	}
	tok2, _ := GenerateJWT(7, "alice", "operator", cfg)
	if tok2 == tok {
		t.Fatalf("同一用户连续签发的令牌应不同（jti）")
	}
	if claims["jti"] == nil || claims["iat"] == nil {
		t.Fatalf("令牌应包含 jti 和 iat: %v", claims)
	}
	if _, err := ValidateJWT(tok, "another-secret-xxxxxxxx"); err == nil {
		t.Fatalf("错误密钥应校验失败")
	}
	if _, err := ValidateJWT("garbage", cfg.SecretKey); err == nil {
		t.Fatalf("非法令牌应校验失败")
	}
}

func TestValidateJWTRejectsExpiredAndOtherAlgorithms(t *testing.T) {
	secret := "0123456789abcdef-secret"
	expired, _ := GenerateJWT(1, "u", "admin", &JWTConfig{SecretKey: secret, TokenDuration: -time.Minute})
	if _, err := ValidateJWT(expired, secret); err == nil {
		t.Fatalf("过期令牌应校验失败")
	}

	// 不带过期时间的令牌
	noExp := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user_id": 1, "role": "admin"})
	s, _ := noExp.SignedString([]byte(secret))
	if _, err := ValidateJWT(s, secret); err == nil {
		t.Fatalf("缺少 exp 的令牌应校验失败")
	}

	// HS512 使用同一个密钥签名，也必须拒绝（只接受 HS256）
	other := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{"user_id": 1, "exp": time.Now().Add(time.Hour).Unix()})
	s, _ = other.SignedString([]byte(secret))
	if _, err := ValidateJWT(s, secret); err == nil {
		t.Fatalf("非 HS256 算法的令牌应被拒绝")
	}

	// alg=none
	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"user_id": 1, "exp": time.Now().Add(time.Hour).Unix()})
	s, _ = none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := ValidateJWT(s, secret); err == nil {
		t.Fatalf("alg=none 的令牌必须被拒绝")
	}
}

func TestIsWeakJWTSecret(t *testing.T) {
	for _, s := range []string{"", "   ", InsecureDefaultJWTSecret, "short"} {
		if !IsWeakJWTSecret(s) {
			t.Errorf("%q 应视为弱密钥", s)
		}
	}
	if IsWeakJWTSecret("a-sufficiently-long-secret") {
		t.Errorf("足够长的密钥不应视为弱密钥")
	}
}

func TestEnsureJWTSecret(t *testing.T) {
	dir := t.TempDir()
	strong := "configured-strong-secret-123"
	got, generated, err := EnsureJWTSecret(strong, dir)
	if err != nil || got != strong || generated {
		t.Fatalf("配置了强密钥时应直接使用")
	}

	s1, generated, err := EnsureJWTSecret(InsecureDefaultJWTSecret, dir)
	if err != nil || !generated || IsWeakJWTSecret(s1) {
		t.Fatalf("占位密钥应被替换为随机密钥: %q %v %v", s1, generated, err)
	}
	info, err := os.Stat(filepath.Join(dir, ".jwt_secret"))
	if err != nil {
		t.Fatalf("随机密钥应持久化: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("密钥文件权限 = %v，应为 0600", info.Mode().Perm())
	}
	s2, _, _ := EnsureJWTSecret("", dir)
	if s1 != s2 {
		t.Fatalf("重启后应复用已保存的密钥，否则已签发的令牌全部失效")
	}
}

func TestInitialAdminPassword(t *testing.T) {
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "from-env-pass")
	p, file, err := InitialAdminPassword(t.TempDir())
	if err != nil || p != "from-env-pass" || file != "" {
		t.Fatalf("应使用环境变量中的密码")
	}

	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "")
	dir := t.TempDir()
	p, file, err = InitialAdminPassword(dir)
	if err != nil || len(p) < 12 || p == "admin123" {
		t.Fatalf("应随机生成足够长的密码: %q", p)
	}
	data, err := os.ReadFile(file)
	if err != nil || strings.TrimSpace(string(data)) != p {
		t.Fatalf("初始密码应写入文件 %s", file)
	}
	p2, _, _ := InitialAdminPassword(t.TempDir())
	if p2 == p {
		t.Fatalf("每次生成的密码应不同")
	}
}

func TestRandomString(t *testing.T) {
	a, _ := RandomString(16)
	b, _ := RandomString(16)
	if a == b || len(a) < 20 {
		t.Fatalf("随机字符串异常: %q %q", a, b)
	}
}
