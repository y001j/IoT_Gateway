package utils

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTConfig JWT配置
type JWTConfig struct {
	SecretKey     string
	TokenDuration time.Duration
}

// GenerateJWT 生成JWT令牌
func GenerateJWT(userID int, username, role string, config *JWTConfig) (string, error) {
	// jti 保证每个令牌唯一：以前同一用户在同一秒内登录会得到完全相同的令牌，
	// 按令牌查会话时会混淆（登出一个会话可能删掉另一个会话）
	jti, err := RandomString(16)
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"role":     role,
		"iat":      now.Unix(),
		"exp":      now.Add(config.TokenDuration).Unix(),
		"jti":      jti,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.SecretKey))
}

// ValidateJWT 验证JWT令牌
func ValidateJWT(tokenString string, secretKey string) (jwt.MapClaims, error) {
	// 只接受 HS256，防止算法混淆攻击
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(secretKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrSignatureInvalid
}
