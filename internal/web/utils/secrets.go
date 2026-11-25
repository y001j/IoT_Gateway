package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InsecureDefaultJWTSecret 历史配置中的占位密钥，视为未配置
const InsecureDefaultJWTSecret = "your-secret-key-change-in-production"

// RandomString 生成 URL 安全的随机字符串（nBytes 字节熵）
func RandomString(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// IsWeakJWTSecret 判断 JWT 密钥是否为空、为占位值或过短
func IsWeakJWTSecret(secret string) bool {
	s := strings.TrimSpace(secret)
	return s == "" || s == InsecureDefaultJWTSecret || len(s) < 16
}

// EnsureJWTSecret 返回可用的 JWT 密钥。
//
// 配置了足够强的密钥时直接使用；否则从 <dataDir>/.jwt_secret 读取，
// 文件不存在时随机生成并以 0600 权限保存，保证重启后已签发的令牌仍然有效。
// generated=true 表示使用的是自动生成的密钥（调用方应提示用户在配置中显式设置）。
func EnsureJWTSecret(configured, dataDir string) (secret string, generated bool, err error) {
	if !IsWeakJWTSecret(configured) {
		return configured, false, nil
	}
	if dataDir == "" {
		dataDir = "./data"
	}
	path := filepath.Join(dataDir, ".jwt_secret")
	if data, rerr := os.ReadFile(filepath.Clean(path)); rerr == nil {
		s := strings.TrimSpace(string(data))
		if !IsWeakJWTSecret(s) {
			return s, true, nil
		}
	}
	s, err := RandomString(32)
	if err != nil {
		return "", false, fmt.Errorf("生成JWT密钥失败: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		// 无法持久化时仍返回随机密钥（重启后需重新登录）
		return s, true, nil
	}
	_ = os.WriteFile(path, []byte(s), 0o600)
	return s, true, nil
}

// InitialAdminPassword 返回首次创建管理员账号时使用的密码。
//
// 优先使用环境变量 IOT_GATEWAY_ADMIN_PASSWORD；未设置时随机生成，
// 并写入 <dataDir>/initial_admin_password（0600）供管理员首次登录使用。
// 以前固定为 admin123，任何人都能用默认账号登录。
func InitialAdminPassword(dataDir string) (password string, file string, err error) {
	if p := os.Getenv("IOT_GATEWAY_ADMIN_PASSWORD"); p != "" {
		return p, "", nil
	}
	p, err := RandomString(12)
	if err != nil {
		return "", "", err
	}
	if dataDir == "" {
		dataDir = "./data"
	}
	file = filepath.Join(dataDir, "initial_admin_password")
	if err := os.MkdirAll(dataDir, 0o700); err == nil {
		if werr := os.WriteFile(file, []byte(p+"\n"), 0o600); werr != nil {
			file = ""
		}
	} else {
		file = ""
	}
	return p, file, nil
}
