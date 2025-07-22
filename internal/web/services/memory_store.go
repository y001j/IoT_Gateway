package services

import (
	"errors"
	"sync"
	"time"

	"github.com/y001j/iot-gateway/internal/web/models"

	"golang.org/x/crypto/bcrypt"
)

// MemoryStore 内存存储实现
type MemoryStore struct {
	mu            sync.RWMutex
	users         map[int]*models.User
	sessions      map[string]*models.Session
	loginHistory  map[int][]*models.LoginHistory
	usersByName   map[string]int
	usersByEmail  map[string]int
	lastID        int
	refreshTokens map[string]string // refreshToken -> token
}

// NewMemoryStore 创建新的内存存储
func NewMemoryStore() *MemoryStore {
	store := &MemoryStore{
		users:         make(map[int]*models.User),
		sessions:      make(map[string]*models.Session),
		loginHistory:  make(map[int][]*models.LoginHistory),
		usersByName:   make(map[string]int),
		usersByEmail:  make(map[string]int),
		refreshTokens: make(map[string]string),
	}

	// 创建默认管理员用户
	adminPassword, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	adminUser := &models.User{
		BaseModel: models.BaseModel{
			ID:        1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Username:    "admin",
		Password:    string(adminPassword),
		Email:       "admin@example.com",
		Role:        "admin",
		Status:      "active",
		DisplayName: "系统管理员",
	}
	store.users[adminUser.ID] = adminUser
	store.usersByName[adminUser.Username] = adminUser.ID
	store.usersByEmail[adminUser.Email] = adminUser.ID
	store.lastID = 1

	return store
}

// CreateUser 创建用户
func (s *MemoryStore) CreateUser(user *models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查用户名是否存在
	if _, exists := s.usersByName[user.Username]; exists {
		return errors.New("用户名已存在")
	}

	// 检查邮箱是否存在
	if _, exists := s.usersByEmail[user.Email]; exists {
		return errors.New("邮箱已存在")
	}

	// 生成ID
	s.lastID++
	user.ID = s.lastID
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	// 存储用户
	s.users[user.ID] = user
	s.usersByName[user.Username] = user.ID
	s.usersByEmail[user.Email] = user.ID

	return nil
}

// GetUser 获取用户
func (s *MemoryStore) GetUser(id int) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[id]
	if !exists {
		return nil, errors.New("用户不存在")
	}
	return user, nil
}

// GetUserByUsername 通过用户名获取用户
func (s *MemoryStore) GetUserByUsername(username string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, exists := s.usersByName[username]
	if !exists {
		return nil, errors.New("用户不存在")
	}
	return s.users[id], nil
}

// GetUserByEmail 通过邮箱获取用户
func (s *MemoryStore) GetUserByEmail(email string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, exists := s.usersByEmail[email]
	if !exists {
		return nil, errors.New("用户不存在")
	}
	return s.users[id], nil
}

// GetUserByID 通过ID获取用户
func (s *MemoryStore) GetUserByID(id int) (*models.User, error) {
	return s.GetUser(id)
}

// UpdateUser 更新用户
func (s *MemoryStore) UpdateUser(user *models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.ID]; !exists {
		return errors.New("用户不存在")
	}

	// 检查用户名是否被其他用户使用
	if id, exists := s.usersByName[user.Username]; exists && id != user.ID {
		return errors.New("用户名已存在")
	}

	// 检查邮箱是否被其他用户使用
	if id, exists := s.usersByEmail[user.Email]; exists && id != user.ID {
		return errors.New("邮箱已存在")
	}

	// 更新索引
	oldUser := s.users[user.ID]
	delete(s.usersByName, oldUser.Username)
	delete(s.usersByEmail, oldUser.Email)
	s.usersByName[user.Username] = user.ID
	s.usersByEmail[user.Email] = user.ID

	// 更新用户
	user.UpdatedAt = time.Now()
	s.users[user.ID] = user

	return nil
}

// DeleteUser 删除用户
func (s *MemoryStore) DeleteUser(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[id]
	if !exists {
		return errors.New("用户不存在")
	}

	delete(s.users, id)
	delete(s.usersByName, user.Username)
	delete(s.usersByEmail, user.Email)
	delete(s.loginHistory, id)

	return nil
}

// ListUsers 获取用户列表
func (s *MemoryStore) ListUsers(offset, limit int) ([]*models.User, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.users)
	if offset >= total {
		return []*models.User{}, total, nil
	}

	users := make([]*models.User, 0, limit)
	count := 0
	for _, user := range s.users {
		if count < offset {
			count++
			continue
		}
		if len(users) >= limit {
			break
		}
		users = append(users, user)
	}

	return users, total, nil
}

// CreateSession 创建会话
func (s *MemoryStore) CreateSession(session *models.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.ID] = session
	return nil
}

// GetSession 获取会话
func (s *MemoryStore) GetSession(id string) (*models.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[id]
	if !exists {
		return nil, errors.New("会话不存在")
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, errors.New("会话已过期")
	}

	return session, nil
}

// GetSessionByToken 通过令牌获取会话
func (s *MemoryStore) GetSessionByToken(token string) (*models.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, session := range s.sessions {
		if session.Token == token {
			return session, nil
		}
	}
	return nil, errors.New("会话不存在")
}

// GetSessionByRefreshToken 通过刷新令牌获取会话
func (s *MemoryStore) GetSessionByRefreshToken(refreshToken string) (*models.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, session := range s.sessions {
		if session.RefreshToken == refreshToken {
			return session, nil
		}
	}
	return nil, errors.New("会话不存在")
}

// UpdateSession 更新会话
func (s *MemoryStore) UpdateSession(session *models.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[session.ID]; !exists {
		return errors.New("会话不存在")
	}

	s.sessions[session.ID] = session
	return nil
}

// DeleteSession 删除会话
func (s *MemoryStore) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, id)
	return nil
}

// CleanExpiredSessions 清理过期会话
func (s *MemoryStore) CleanExpiredSessions() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, id)
		}
	}

	return nil
}

// AddLoginHistory 添加登录历史
func (s *MemoryStore) AddLoginHistory(history *models.LoginHistory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[history.UserID]; !exists {
		return errors.New("用户不存在")
	}

	if s.loginHistory[history.UserID] == nil {
		s.loginHistory[history.UserID] = make([]*models.LoginHistory, 0)
	}

	history.ID = len(s.loginHistory[history.UserID]) + 1
	history.CreatedAt = time.Now()
	history.UpdatedAt = time.Now()

	s.loginHistory[history.UserID] = append(s.loginHistory[history.UserID], history)
	return nil
}

// GetUserLoginHistory 获取用户登录历史
func (s *MemoryStore) GetUserLoginHistory(userID int, limit int) ([]*models.LoginHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.users[userID]; !exists {
		return nil, errors.New("用户不存在")
	}

	history := s.loginHistory[userID]
	if history == nil {
		return []*models.LoginHistory{}, nil
	}

	if limit <= 0 || limit > len(history) {
		limit = len(history)
	}

	result := make([]*models.LoginHistory, limit)
	for i := 0; i < limit; i++ {
		result[i] = history[len(history)-1-i]
	}

	return result, nil
}
