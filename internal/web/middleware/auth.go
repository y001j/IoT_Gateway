package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/utils"

	"github.com/gin-gonic/gin"
)

// JWTMiddleware JWT认证中间件
func JWTMiddleware(secretKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var token string

		// 首先尝试从 Authorization header 获取令牌
		authHeader := c.GetHeader("Authorization")
		fmt.Printf("收到认证请求 URL: %s\n", c.Request.URL.Path)
		fmt.Printf("Authorization header: %s\n", authHeader)

		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}

		// 如果 header 中没有令牌，尝试从查询参数获取（用于 WebSocket）
		if token == "" {
			token = c.Query("token")
			if token != "" {
				fmt.Printf("从查询参数获取到令牌: %s...\n", token[:min(20, len(token))])
			}
		}

		// 如果仍然没有令牌，返回认证失败
		if token == "" {
			fmt.Printf("认证失败：未提供认证令牌\n")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未提供认证令牌"})
			return
		}

		fmt.Printf("开始验证JWT token: %s...\n", token[:min(20, len(token))])
		claims, err := utils.ValidateJWT(token, secretKey)
		if err != nil {
			fmt.Printf("认证失败：JWT验证错误 - %v\n", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "无效的认证令牌"})
			return
		}

		fmt.Printf("JWT验证成功，claims: %+v\n", claims)

		// 设置用户信息到上下文
		c.Set("user_id", claims["user_id"])
		c.Set("username", claims["username"])
		c.Set("role", claims["role"])
		
		// 调试信息：打印设置的role
		fmt.Printf("设置到context的role: %v\n", claims["role"])

		// 创建完整的用户对象并设置到上下文（为 WebSocket 处理器使用）
		userID := 0
		if uid, ok := claims["user_id"].(float64); ok {
			userID = int(uid)
		}

		user := &models.User{
			BaseModel: models.BaseModel{
				ID: userID,
			},
			Username: fmt.Sprintf("%v", claims["username"]),
			Role:     fmt.Sprintf("%v", claims["role"]),
		}
		c.Set("user", user)

		c.Next()
	}
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RequireRole 角色验证中间件
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Printf("RequireRole中间件 - 需要角色: %s\n", role)
		
		userRole, exists := c.Get("role")
		if !exists {
			fmt.Printf("RequireRole中间件 - 未找到role信息\n")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
			return
		}

		userRoleStr := fmt.Sprintf("%v", userRole)
		fmt.Printf("RequireRole中间件 - 用户角色: %s\n", userRoleStr)
		
		// 管理员拥有所有权限 (支持admin和administrator两种角色名)
		if userRoleStr == "admin" || userRoleStr == "administrator" {
			fmt.Printf("RequireRole中间件 - 用户是管理员(%s)，允许访问\n", userRoleStr)
			c.Next()
			return
		}

		// 检查是否匹配指定角色
		if userRoleStr != role {
			fmt.Printf("RequireRole中间件 - 角色不匹配: %s != %s\n", userRoleStr, role)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "权限不足"})
			return
		}

		fmt.Printf("RequireRole中间件 - 角色匹配，允许访问\n")
		c.Next()
	}
}
