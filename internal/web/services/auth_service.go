package services

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/utils"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// AuthService 认证服务接口
type AuthService interface {
	Login(req *models.LoginRequest) (*models.LoginResponse, error)
	Logout(token string) error
	RefreshToken(req *models.RefreshTokenRequest) (*models.LoginResponse, error)
	GetProfile(userID int) (*models.User, error)
	UpdateProfile(userID int, req *models.UpdateProfileRequest) error
	ChangePassword(userID int, req *models.ChangePasswordRequest) error
	ValidateToken(token string) (*models.User, error)
	CreateUser(user *models.User) error
	ListUsers(offset, limit int) ([]*models.User, int, error)
	GetUserLoginHistory(userID int, limit int) ([]*models.LoginHistory, error)
	// JWTSecret 返回实际用于签发和校验令牌的密钥（可能是自动生成的）
	JWTSecret() string
}

// DataDir 返回网关数据目录（环境变量 IOT_GATEWAY_DATA_DIR，默认 ./data）
func DataDir() string {
	if d := os.Getenv("IOT_GATEWAY_DATA_DIR"); d != "" {
		return d
	}
	return "./data"
}

// authService 认证服务实现
type authService struct {
	store     models.UserStore
	jwtConfig *utils.JWTConfig
	config    *models.AuthConfig
	guard     *loginGuard
}

// NewAuthService 创建认证服务
func NewAuthService(store models.UserStore, config *models.AuthConfig) AuthService {
	if store == nil {
		store = NewMemoryStore()
	}

	if config == nil {
		config = &models.AuthConfig{
			JWTSecret:         "your-secret-key-change-in-production",
			TokenDuration:     24 * time.Hour,
			RefreshDuration:   7 * 24 * time.Hour,
			MaxLoginAttempts:  5,
			LockoutDuration:   15 * time.Minute,
			EnableTwoFactor:   false,
			SessionTimeout:    30 * time.Minute,
			PasswordMinLength: 6,
			BcryptCost:        10,
		}
	}

	// JWT 密钥为空或仍是占位值时，使用自动生成并持久化的随机密钥
	cfgCopy := *config
	secret, generated, err := utils.EnsureJWTSecret(cfgCopy.JWTSecret, DataDir())
	if err == nil {
		if generated {
			log.Warn().Msg("web_ui.auth.jwt_secret 未配置或使用了不安全的默认值，已使用自动生成的随机密钥（保存在数据目录 .jwt_secret）")
		}
		cfgCopy.JWTSecret = secret
	}
	config = &cfgCopy

	return &authService{
		store:  store,
		config: config,
		guard:  newLoginGuard(config.MaxLoginAttempts, config.LockoutDuration),
		jwtConfig: &utils.JWTConfig{
			SecretKey:     config.JWTSecret,
			TokenDuration: config.TokenDuration,
		},
	}
}

// JWTSecret 返回签发令牌使用的密钥
func (a *authService) JWTSecret() string {
	return a.jwtConfig.SecretKey
}

// Login 用户登录
func (a *authService) Login(req *models.LoginRequest) (*models.LoginResponse, error) {
	// 连续失败次数过多时临时锁定（锁定期间即使密码正确也拒绝）
	if a.guard.locked(req.Username) {
		return nil, ErrAccountLocked
	}

	// 获取用户
	user, err := a.store.GetUserByUsername(req.Username)
	if err != nil {
		// 不区分"用户不存在"和"密码错误"，避免用户名枚举
		if a.guard.recordFailure(req.Username) {
			log.Warn().Str("username", req.Username).Msg("登录失败次数过多，已临时锁定")
		}
		return nil, errors.New("用户名或密码错误")
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		if a.guard.recordFailure(req.Username) {
			log.Warn().Str("username", req.Username).Msg("登录失败次数过多，已临时锁定")
		}
		_ = a.store.AddLoginHistory(&models.LoginHistory{UserID: user.ID, Success: false, LoginTime: time.Now()})
		return nil, errors.New("用户名或密码错误")
	}
	a.guard.recordSuccess(req.Username)

	// 生成访问令牌
	token, err := utils.GenerateJWT(user.ID, user.Username, user.Role, a.jwtConfig)
	if err != nil {
		return nil, fmt.Errorf("生成令牌失败: %v", err)
	}

	// 生成刷新令牌
	refreshToken := ""
	if req.Remember {
		refreshToken = uuid.New().String()
	}

	// 创建会话
	now := time.Now()
	session := &models.Session{
		ID:           uuid.New().String(),
		UserID:       user.ID,
		Token:        token,
		RefreshToken: refreshToken,
		ExpiresAt:    now.Add(a.config.TokenDuration),
		Active:       true,
		LastAccess:   now,
	}

	if err := a.store.CreateSession(session); err != nil {
		return nil, fmt.Errorf("创建会话失败: %v", err)
	}

	// 更新用户登录信息
	user.LastLogin = &now
	user.LoginCount++
	if err := a.store.UpdateUser(user); err != nil {
		return nil, fmt.Errorf("更新用户信息失败: %v", err)
	}

	// 记录登录历史
	history := &models.LoginHistory{
		UserID:    user.ID,
		Success:   true,
		LoginTime: now,
	}
	if err := a.store.AddLoginHistory(history); err != nil {
		// 记录历史失败不影响登录
		fmt.Printf("记录登录历史失败: %v\n", err)
	}

	response := &models.LoginResponse{
		Token:        token,
		RefreshToken: refreshToken,
		ExpiresAt:    session.ExpiresAt,
		User: &models.UserInfo{
			ID:          user.ID,
			Username:    user.Username,
			Email:       user.Email,
			Role:        user.Role,
			DisplayName: user.DisplayName,
			Avatar:      user.Avatar,
			LastLogin:   user.LastLogin,
			LoginCount:  user.LoginCount,
		},
	}

	return response, nil
}

// Logout 用户登出
func (a *authService) Logout(token string) error {
	session, err := a.store.GetSessionByToken(token)
	if err != nil {
		// 会话不存在（例如修改密码后已失效）时登出视为成功，保持幂等
		return nil
	}
	// 直接删除会话：SQLite 存储没有 active 列，以前设置 Active=false 不会被持久化
	return a.store.DeleteSession(session.ID)
}

// RefreshToken 刷新令牌
func (a *authService) RefreshToken(req *models.RefreshTokenRequest) (*models.LoginResponse, error) {
	// 获取原会话
	session, err := a.store.GetSessionByRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, err
	}

	if !session.Active {
		return nil, errors.New("会话已失效")
	}

	// 获取用户信息
	user, err := a.store.GetUserByID(session.UserID)
	if err != nil {
		return nil, err
	}

	// 生成新的访问令牌
	newToken, err := utils.GenerateJWT(user.ID, user.Username, user.Role, a.jwtConfig)
	if err != nil {
		return nil, fmt.Errorf("生成令牌失败: %v", err)
	}

	// 创建新会话
	now := time.Now()
	newSession := &models.Session{
		ID:           uuid.New().String(),
		UserID:       user.ID,
		Token:        newToken,
		RefreshToken: uuid.New().String(),
		ExpiresAt:    now.Add(a.config.TokenDuration),
		Active:       true,
		LastAccess:   now,
	}

	if err := a.store.CreateSession(newSession); err != nil {
		return nil, fmt.Errorf("创建会话失败: %v", err)
	}

	// 使原会话失效
	session.Active = false
	if err := a.store.UpdateSession(session); err != nil {
		fmt.Printf("使原会话失效失败: %v\n", err)
	}

	response := &models.LoginResponse{
		Token:        newToken,
		RefreshToken: newSession.RefreshToken,
		ExpiresAt:    newSession.ExpiresAt,
		User: &models.UserInfo{
			ID:          user.ID,
			Username:    user.Username,
			Email:       user.Email,
			Role:        user.Role,
			DisplayName: user.DisplayName,
			Avatar:      user.Avatar,
			LastLogin:   user.LastLogin,
			LoginCount:  user.LoginCount,
		},
	}

	return response, nil
}

// GetProfile 获取用户资料
func (a *authService) GetProfile(userID int) (*models.User, error) {
	return a.store.GetUserByID(userID)
}

// UpdateProfile 更新用户资料
func (a *authService) UpdateProfile(userID int, req *models.UpdateProfileRequest) error {
	user, err := a.store.GetUserByID(userID)
	if err != nil {
		return err
	}

	if req.Email != "" {
		user.Email = req.Email
	}
	if req.DisplayName != "" {
		user.DisplayName = req.DisplayName
	}
	if req.Avatar != "" {
		user.Avatar = req.Avatar
	}

	return a.store.UpdateUser(user)
}

// ChangePassword 修改密码
func (a *authService) ChangePassword(userID int, req *models.ChangePasswordRequest) error {
	user, err := a.store.GetUserByID(userID)
	if err != nil {
		return err
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		return errors.New("旧密码错误")
	}

	// 验证新密码长度
	if len(req.NewPassword) < a.config.PasswordMinLength {
		return fmt.Errorf("新密码长度不能小于%d个字符", a.config.PasswordMinLength)
	}

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), a.config.BcryptCost)
	if err != nil {
		return fmt.Errorf("密码加密失败: %v", err)
	}

	user.Password = string(hashedPassword)
	if err := a.store.UpdateUser(user); err != nil {
		return err
	}
	// 修改密码后使该用户所有已签发的令牌失效
	return a.store.DeleteUserSessions(user.ID)
}

// ValidateToken 验证令牌
func (a *authService) ValidateToken(token string) (*models.User, error) {
	session, err := a.store.GetSessionByToken(token)
	if err != nil {
		return nil, err
	}

	if !session.Active {
		return nil, errors.New("会话已失效")
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, errors.New("会话已过期")
	}

	return a.store.GetUserByID(session.UserID)
}

// CreateUser 创建用户
func (a *authService) CreateUser(user *models.User) error {
	// 验证密码长度
	if len(user.Password) < a.config.PasswordMinLength {
		return fmt.Errorf("密码长度不能小于%d个字符", a.config.PasswordMinLength)
	}

	// 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), a.config.BcryptCost)
	if err != nil {
		return fmt.Errorf("密码加密失败: %v", err)
	}

	user.Password = string(hashedPassword)
	return a.store.CreateUser(user)
}

// ListUsers 列出用户
func (a *authService) ListUsers(offset, limit int) ([]*models.User, int, error) {
	return a.store.ListUsers(offset, limit)
}

// GetUserLoginHistory 获取用户登录历史
func (a *authService) GetUserLoginHistory(userID int, limit int) ([]*models.LoginHistory, error) {
	return a.store.GetUserLoginHistory(userID, limit)
}
