package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/utils"

	"github.com/gin-gonic/gin"
)

// TokenValidator 在 JWT 签名校验通过后执行的额外校验（例如检查会话是否已被登出/吊销）
type TokenValidator func(token string) error

// JWTMiddleware JWT认证中间件。
// 以前会把 Authorization 头、令牌前缀和全部 claims 打印到标准输出，已移除。
// validators 用于检查会话状态：JWT 本身是无状态的，不检查会话时登出或修改密码后旧令牌在过期前仍然有效。
func JWTMiddleware(secretKey string, validators ...TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		var token string

		if authHeader := c.GetHeader("Authorization"); authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				token = strings.TrimSpace(parts[1])
			}
		}

		// 浏览器的 WebSocket API 无法设置请求头，只有 WebSocket 升级请求允许通过查询参数传递令牌
		if token == "" && strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
			token = c.Query("token")
		}

		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未提供认证令牌"})
			return
		}

		claims, err := utils.ValidateJWT(token, secretKey)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "无效的认证令牌"})
			return
		}
		for _, v := range validators {
			if v == nil {
				continue
			}
			if err := v(token); err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "会话已失效，请重新登录"})
				return
			}
		}

		c.Set("user_id", claims["user_id"])
		c.Set("username", claims["username"])
		c.Set("role", claims["role"])

		userID := 0
		if uid, ok := claims["user_id"].(float64); ok {
			userID = int(uid)
		}
		user := &models.User{
			BaseModel: models.BaseModel{ID: userID},
			Username:  fmt.Sprintf("%v", claims["username"]),
			Role:      fmt.Sprintf("%v", claims["role"]),
		}
		c.Set("user", user)

		c.Next()
	}
}

// isAdminRole 管理员角色（支持 admin 和 administrator 两种名称）
func isAdminRole(role string) bool {
	return role == "admin" || role == "administrator"
}

// RequireRole 角色验证中间件：管理员总是允许，其他用户必须具有列出的某个角色
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
			return
		}
		role := fmt.Sprintf("%v", userRole)
		if isAdminRole(role) {
			c.Next()
			return
		}
		for _, r := range roles {
			if role == r {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "权限不足"})
	}
}
