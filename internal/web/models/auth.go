package models

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// User 用户信息
type User struct {
	BaseModel
	Username    string     `json:"username"`
	Password    string     `json:"-"` // 密码不会在JSON中返回
	Salt        string     `json:"-"` // 密码盐值
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	LastLogin   *time.Time `json:"last_login,omitempty"`
	LoginCount  int        `json:"login_count"`
	Avatar      string     `json:"avatar,omitempty"`
	DisplayName string     `json:"display_name,omitempty"`
}

// UserInfo 用户公开信息
type UserInfo struct {
	ID          int        `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	DisplayName string     `json:"display_name,omitempty"`
	Avatar      string     `json:"avatar,omitempty"`
	LastLogin   *time.Time `json:"last_login,omitempty"`
	LoginCount  int        `json:"login_count"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Remember bool   `json:"remember"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         *UserInfo `json:"user"`
}

// RefreshTokenRequest 刷新令牌请求
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// UpdateProfileRequest 更新资料请求
type UpdateProfileRequest struct {
	Email       string `json:"email" binding:"required,email"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// LoginHistory 登录历史
type LoginHistory struct {
	BaseModel
	UserID    int       `json:"user_id"`
	LoginTime time.Time `json:"login_time"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Success   bool      `json:"success"`
}

// Session 会话信息
type Session struct {
	ID           string    `json:"id"`
	UserID       int       `json:"user_id"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	Active       bool      `json:"active"`
	LastAccess   time.Time `json:"last_access"`
}

// Claims JWT声明
type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// AuthConfig 认证配置
type AuthConfig struct {
	JWTSecret         string        `json:"jwt_secret" mapstructure:"jwt_secret"`
	TokenDuration     time.Duration `json:"token_duration" mapstructure:"token_duration"`
	RefreshDuration   time.Duration `json:"refresh_duration" mapstructure:"refresh_duration"`
	MaxLoginAttempts  int           `json:"max_login_attempts" mapstructure:"max_login_attempts"`
	LockoutDuration   time.Duration `json:"lockout_duration" mapstructure:"lockout_duration"`
	EnableTwoFactor   bool          `json:"enable_two_factor" mapstructure:"enable_two_factor"`
	SessionTimeout    time.Duration `json:"session_timeout" mapstructure:"session_timeout"`
	PasswordMinLength int           `json:"password_min_length" mapstructure:"password_min_length"`
	BcryptCost        int           `json:"bcrypt_cost" mapstructure:"bcrypt_cost"`
}

// UserStore 用户存储接口
type UserStore interface {
	// 用户管理
	CreateUser(user *User) error
	GetUser(id int) (*User, error)
	GetUserByID(id int) (*User, error)
	GetUserByUsername(username string) (*User, error)
	GetUserByEmail(email string) (*User, error)
	UpdateUser(user *User) error
	DeleteUser(id int) error
	ListUsers(offset, limit int) ([]*User, int, error)

	// 会话管理
	CreateSession(session *Session) error
	GetSession(id string) (*Session, error)
	GetSessionByToken(token string) (*Session, error)
	GetSessionByRefreshToken(refreshToken string) (*Session, error)
	UpdateSession(session *Session) error
	DeleteSession(id string) error
	CleanExpiredSessions() error

	// 登录历史
	AddLoginHistory(history *LoginHistory) error
	GetUserLoginHistory(userID int, limit int) ([]*LoginHistory, error)
}
