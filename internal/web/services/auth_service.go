package services

import (
	"errors"
	"fmt"
	"time"

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
}

// authService 认证服务实现
type authService struct {
	store     models.UserStore
	jwtConfig *utils.JWTConfig
	config    *models.AuthConfig
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

	return &authService{
		store:  store,
		config: config,
		jwtConfig: &utils.JWTConfig{
			SecretKey:     config.JWTSecret,
			TokenDuration: config.TokenDuration,
		},
	}
}

// Login 用户登录
func (a *authService) Login(req *models.LoginRequest) (*models.LoginResponse, error) {
	// 获取用户
	user, err := a.store.GetUserByUsername(req.Username)
	if err != nil {
		// 调试：打印用户查找失败的错误
		fmt.Printf("查找用户失败: Username=[%s], Error=[%v]\n", req.Username, err)
		return nil, errors.New("用户名或密码错误 (用户未找到)")
	}

	// 调试：打印从数据库获取的密码哈希
	fmt.Printf("数据库中的哈希: [%s]\n", user.Password)

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		// 调试：打印密码比较失败的错误
		fmt.Printf("密码比较失败: %v\n", err)
		return nil, errors.New("用户名或密码错误 (密码不匹配)")
	}

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
		return err
	}

	session.Active = false
	return a.store.UpdateSession(session)
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
	return a.store.UpdateUser(user)
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
