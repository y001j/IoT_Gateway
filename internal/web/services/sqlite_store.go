package services

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/y001j/iot-gateway/internal/web/models"

	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
)

// PreparedStatements 预处理语句集合
type PreparedStatements struct {
	createUser    *sql.Stmt
	getUserByID   *sql.Stmt
	getUserByUsername *sql.Stmt
	updateUser    *sql.Stmt
	deleteUser    *sql.Stmt
	listUsers     *sql.Stmt
	countUsers    *sql.Stmt
}

// SQLiteStore SQLite存储实现
type SQLiteStore struct {
	db     *sql.DB
	stmts  *PreparedStatements
	mu     sync.RWMutex
}

// NewSQLiteStore 创建新的SQLite存储
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	return NewSQLiteStoreWithConfig(&models.SQLiteConfig{
		Path:              dbPath,
		MaxOpenConns:      25,
		MaxIdleConns:      5,
		ConnMaxLifetime:   "5m",
		ConnMaxIdleTime:   "1m",
	})
}

// NewSQLiteStoreWithConfig 使用配置创建SQLite存储
func NewSQLiteStoreWithConfig(config *models.SQLiteConfig) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", config.Path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %v", err)
	}

	// 配置连接池参数
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	
	// 解析时间字符串
	if config.ConnMaxLifetime != "" {
		if duration, err := time.ParseDuration(config.ConnMaxLifetime); err == nil {
			db.SetConnMaxLifetime(duration)
		} else {
			db.SetConnMaxLifetime(5 * time.Minute) // 默认值
		}
	}
	
	if config.ConnMaxIdleTime != "" {
		if duration, err := time.ParseDuration(config.ConnMaxIdleTime); err == nil {
			db.SetConnMaxIdleTime(duration)
		} else {
			db.SetConnMaxIdleTime(1 * time.Minute) // 默认值
		}
	}

	store := &SQLiteStore{db: db}
	
	// 初始化数据库表
	if err := store.initDatabase(); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化数据库失败: %v", err)
	}

	// 初始化预处理语句
	if err := store.initPreparedStatements(); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化预处理语句失败: %v", err)
	}

	return store, nil
}

// initDatabase 初始化数据库表
func (s *SQLiteStore) initDatabase() error {
	// 创建用户表
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL,
			role TEXT NOT NULL,
			status TEXT NOT NULL,
			display_name TEXT,
			avatar TEXT,
			last_login DATETIME,
			login_count INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	// 创建会话表
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			token TEXT NOT NULL,
			refresh_token TEXT,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)
	`)
	if err != nil {
		return err
	}

	// 创建登录历史表
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS login_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			login_time DATETIME NOT NULL,
			ip TEXT,
			user_agent TEXT,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)
	`)
	if err != nil {
		return err
	}

	// 检查是否需要创建默认管理员用户
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// 创建默认管理员用户
		adminPassword, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		_, err = s.db.Exec(`
			INSERT INTO users (
				username, password, email, role, status, display_name, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`,
			"admin",
			string(adminPassword),
			"admin@example.com",
			"admin",
			"active",
			"系统管理员",
			time.Now(),
			time.Now(),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// initPreparedStatements 初始化预处理语句
func (s *SQLiteStore) initPreparedStatements() error {
	var err error
	stmts := &PreparedStatements{}

	// 创建用户语句
	stmts.createUser, err = s.db.Prepare(`
		INSERT INTO users (username, password, email, role, status, display_name, avatar, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("准备创建用户语句失败: %v", err)
	}

	// 根据ID获取用户语句
	stmts.getUserByID, err = s.db.Prepare(`
		SELECT id, username, password, email, role, status, display_name, avatar, last_login, login_count, created_at, updated_at 
		FROM users WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("准备根据ID获取用户语句失败: %v", err)
	}

	// 根据用户名获取用户语句
	stmts.getUserByUsername, err = s.db.Prepare(`
		SELECT id, username, password, email, role, status, display_name, avatar, last_login, login_count, created_at, updated_at 
		FROM users WHERE username = ?`)
	if err != nil {
		return fmt.Errorf("准备根据用户名获取用户语句失败: %v", err)
	}

	// 更新用户语句
	stmts.updateUser, err = s.db.Prepare(`
		UPDATE users 
		SET username = ?, email = ?, role = ?, status = ?, display_name = ?, avatar = ?, updated_at = ? 
		WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("准备更新用户语句失败: %v", err)
	}

	// 删除用户语句
	stmts.deleteUser, err = s.db.Prepare(`DELETE FROM users WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("准备删除用户语句失败: %v", err)
	}

	// 列出用户语句
	stmts.listUsers, err = s.db.Prepare(`
		SELECT id, username, email, role, status, display_name, avatar, last_login, login_count, created_at, updated_at 
		FROM users ORDER BY created_at DESC LIMIT ? OFFSET ?`)
	if err != nil {
		return fmt.Errorf("准备列出用户语句失败: %v", err)
	}

	// 统计用户数量语句
	stmts.countUsers, err = s.db.Prepare(`SELECT COUNT(*) FROM users`)
	if err != nil {
		return fmt.Errorf("准备统计用户数量语句失败: %v", err)
	}

	s.stmts = stmts
	return nil
}

// CreateUser 创建用户 - 使用预处理语句优化
func (s *SQLiteStore) CreateUser(user *models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 检查用户名是否存在
	var exists int
	err = tx.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", user.Username).Scan(&exists)
	if err != nil {
		return err
	}
	if exists > 0 {
		return errors.New("用户名已存在")
	}

	// 检查邮箱是否存在
	err = tx.QueryRow("SELECT COUNT(*) FROM users WHERE email = ?", user.Email).Scan(&exists)
	if err != nil {
		return err
	}
	if exists > 0 {
		return errors.New("邮箱已存在")
	}

	// 使用预处理语句插入用户
	now := time.Now()
	stmt := tx.Stmt(s.stmts.createUser)
	defer stmt.Close()
	
	result, err := stmt.Exec(
		user.Username,
		user.Password,
		user.Email,
		user.Role,
		user.Status,
		user.DisplayName,
		user.Avatar,
		now,
		now,
	)
	if err != nil {
		return err
	}

	// 获取插入的ID
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = int(id)
	user.CreatedAt = now
	user.UpdatedAt = now

	// 提交事务
	return tx.Commit()
}

// Close 关闭数据库连接和预处理语句
func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 关闭预处理语句
	if s.stmts != nil {
		if s.stmts.createUser != nil {
			s.stmts.createUser.Close()
		}
		if s.stmts.getUserByID != nil {
			s.stmts.getUserByID.Close()
		}
		if s.stmts.getUserByUsername != nil {
			s.stmts.getUserByUsername.Close()
		}
		if s.stmts.updateUser != nil {
			s.stmts.updateUser.Close()
		}
		if s.stmts.deleteUser != nil {
			s.stmts.deleteUser.Close()
		}
		if s.stmts.listUsers != nil {
			s.stmts.listUsers.Close()
		}
		if s.stmts.countUsers != nil {
			s.stmts.countUsers.Close()
		}
	}

	// 关闭数据库连接
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// GetUserByID 通过ID获取用户
func (s *SQLiteStore) GetUserByID(id int) (*models.User, error) {
	user := &models.User{}
	var lastLogin sql.NullTime
	var displayName sql.NullString
	var avatar sql.NullString

	err := s.db.QueryRow(`
		SELECT id, username, password, email, role, status, display_name, avatar,
			   last_login, login_count, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(
		&user.ID,
		&user.Username,
		&user.Password,
		&user.Email,
		&user.Role,
		&user.Status,
		&displayName,
		&avatar,
		&lastLogin,
		&user.LoginCount,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.New("用户不存在")
	}
	if err != nil {
		return nil, err
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}
	if displayName.Valid {
		user.DisplayName = displayName.String
	}
	if avatar.Valid {
		user.Avatar = avatar.String
	}

	return user, nil
}

// GetUserByUsername 通过用户名获取用户
func (s *SQLiteStore) GetUserByUsername(username string) (*models.User, error) {
	user := &models.User{}
	var lastLogin sql.NullTime
	var displayName sql.NullString
	var avatar sql.NullString

	err := s.db.QueryRow(`
		SELECT id, username, password, email, role, status, display_name, avatar,
			   last_login, login_count, created_at, updated_at
		FROM users WHERE username = ?
	`, username).Scan(
		&user.ID,
		&user.Username,
		&user.Password,
		&user.Email,
		&user.Role,
		&user.Status,
		&displayName,
		&avatar,
		&lastLogin,
		&user.LoginCount,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.New("用户不存在")
	}
	if err != nil {
		return nil, err
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}
	if displayName.Valid {
		user.DisplayName = displayName.String
	}
	if avatar.Valid {
		user.Avatar = avatar.String
	}

	return user, nil
}

// GetUserByEmail 通过邮箱获取用户
func (s *SQLiteStore) GetUserByEmail(email string) (*models.User, error) {
	user := &models.User{}
	var lastLogin sql.NullTime
	var displayName sql.NullString
	var avatar sql.NullString

	err := s.db.QueryRow(`
		SELECT id, username, password, email, role, status, display_name, avatar,
			   last_login, login_count, created_at, updated_at
		FROM users WHERE email = ?
	`, email).Scan(
		&user.ID,
		&user.Username,
		&user.Password,
		&user.Email,
		&user.Role,
		&user.Status,
		&displayName,
		&avatar,
		&lastLogin,
		&user.LoginCount,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.New("用户不存在")
	}
	if err != nil {
		return nil, err
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}
	if displayName.Valid {
		user.DisplayName = displayName.String
	}
	if avatar.Valid {
		user.Avatar = avatar.String
	}

	return user, nil
}

// UpdateUser 更新用户
func (s *SQLiteStore) UpdateUser(user *models.User) error {
	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 检查用户名是否被其他用户使用
	var exists int
	err = tx.QueryRow("SELECT COUNT(*) FROM users WHERE username = ? AND id != ?",
		user.Username, user.ID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists > 0 {
		return errors.New("用户名已存在")
	}

	// 检查邮箱是否被其他用户使用
	err = tx.QueryRow("SELECT COUNT(*) FROM users WHERE email = ? AND id != ?",
		user.Email, user.ID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists > 0 {
		return errors.New("邮箱已存在")
	}

	// 更新用户
	var lastLogin interface{} = nil
	if user.LastLogin != nil {
		lastLogin = user.LastLogin
	}

	_, err = tx.Exec(`
		UPDATE users SET
			username = ?, password = ?, email = ?, role = ?, status = ?,
			display_name = ?, avatar = ?, last_login = ?, login_count = ?,
			updated_at = ?
		WHERE id = ?
	`,
		user.Username,
		user.Password,
		user.Email,
		user.Role,
		user.Status,
		user.DisplayName,
		user.Avatar,
		lastLogin,
		user.LoginCount,
		time.Now(),
		user.ID,
	)
	if err != nil {
		return err
	}

	// 提交事务
	return tx.Commit()
}

// DeleteUser 删除用户
func (s *SQLiteStore) DeleteUser(id int) error {
	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 删除用户相关的会话
	_, err = tx.Exec("DELETE FROM sessions WHERE user_id = ?", id)
	if err != nil {
		return err
	}

	// 删除用户的登录历史
	_, err = tx.Exec("DELETE FROM login_history WHERE user_id = ?", id)
	if err != nil {
		return err
	}

	// 删除用户
	result, err := tx.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("用户不存在")
	}

	// 提交事务
	return tx.Commit()
}

// ListUsers 获取用户列表
func (s *SQLiteStore) ListUsers(offset, limit int) ([]*models.User, int, error) {
	// 获取总数
	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 查询用户列表
	rows, err := s.db.Query(`
		SELECT id, username, password, email, role, status, display_name, avatar,
			   last_login, login_count, created_at, updated_at
		FROM users
		ORDER BY id
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := make([]*models.User, 0)
	for rows.Next() {
		user := &models.User{}
		var lastLogin sql.NullTime
		var displayName sql.NullString
		var avatar sql.NullString

		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Password,
			&user.Email,
			&user.Role,
			&user.Status,
			&displayName,
			&avatar,
			&lastLogin,
			&user.LoginCount,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}

		if lastLogin.Valid {
			user.LastLogin = &lastLogin.Time
		}
		if displayName.Valid {
			user.DisplayName = displayName.String
		}
		if avatar.Valid {
			user.Avatar = avatar.String
		}

		users = append(users, user)
	}

	return users, total, nil
}

// CreateSession 创建会话
func (s *SQLiteStore) CreateSession(session *models.Session) error {
	_, err := s.db.Exec(`
		INSERT INTO sessions (
			id, user_id, token, refresh_token, expires_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`,
		session.ID,
		session.UserID,
		session.Token,
		session.RefreshToken,
		session.ExpiresAt,
		time.Now(),
	)
	return err
}

// GetSession 获取会话
func (s *SQLiteStore) GetSession(id string) (*models.Session, error) {
	session := &models.Session{}
	err := s.db.QueryRow(`
		SELECT id, user_id, token, refresh_token, expires_at, created_at
		FROM sessions WHERE id = ?
	`, id).Scan(
		&session.ID,
		&session.UserID,
		&session.Token,
		&session.RefreshToken,
		&session.ExpiresAt,
		&session.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.New("会话不存在")
	}
	if err != nil {
		return nil, err
	}

	return session, nil
}

// DeleteSession 删除会话
func (s *SQLiteStore) DeleteSession(id string) error {
	result, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("会话不存在")
	}

	return nil
}

// CleanExpiredSessions 清理过期会话
func (s *SQLiteStore) CleanExpiredSessions() error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now())
	return err
}

// AddLoginHistory 添加登录历史
func (s *SQLiteStore) AddLoginHistory(history *models.LoginHistory) error {
	_, err := s.db.Exec(`
		INSERT INTO login_history (
			user_id, login_time, ip, user_agent, created_at
		) VALUES (?, ?, ?, ?, ?)
	`,
		history.UserID,
		history.LoginTime,
		history.IP,
		history.UserAgent,
		time.Now(),
	)
	return err
}

// GetUserLoginHistory 获取用户登录历史
func (s *SQLiteStore) GetUserLoginHistory(userID int, limit int) ([]*models.LoginHistory, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, login_time, ip, user_agent, created_at
		FROM login_history
		WHERE user_id = ?
		ORDER BY login_time DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := make([]*models.LoginHistory, 0)
	for rows.Next() {
		h := &models.LoginHistory{}
		err := rows.Scan(
			&h.ID,
			&h.UserID,
			&h.LoginTime,
			&h.IP,
			&h.UserAgent,
			&h.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		history = append(history, h)
	}

	return history, nil
}

// GetSessionByToken 通过令牌获取会话
func (s *SQLiteStore) GetSessionByToken(token string) (*models.Session, error) {
	session := &models.Session{}
	err := s.db.QueryRow(`
		SELECT id, user_id, token, refresh_token, expires_at, created_at
		FROM sessions WHERE token = ?
	`, token).Scan(
		&session.ID,
		&session.UserID,
		&session.Token,
		&session.RefreshToken,
		&session.ExpiresAt,
		&session.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.New("会话不存在")
	}
	if err != nil {
		return nil, err
	}

	return session, nil
}

// GetSessionByRefreshToken 通过刷新令牌获取会话
func (s *SQLiteStore) GetSessionByRefreshToken(refreshToken string) (*models.Session, error) {
	session := &models.Session{}
	err := s.db.QueryRow(`
		SELECT id, user_id, token, refresh_token, expires_at, created_at
		FROM sessions WHERE refresh_token = ?
	`, refreshToken).Scan(
		&session.ID,
		&session.UserID,
		&session.Token,
		&session.RefreshToken,
		&session.ExpiresAt,
		&session.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.New("会话不存在")
	}
	if err != nil {
		return nil, err
	}

	return session, nil
}


// GetUser 获取用户
func (s *SQLiteStore) GetUser(id int) (*models.User, error) {
	return s.GetUserByID(id)
}

// UpdateSession 更新会话
func (s *SQLiteStore) UpdateSession(session *models.Session) error {
	_, err := s.db.Exec(`
		UPDATE sessions SET 
			token = ?, refresh_token = ?, expires_at = ?
		WHERE id = ?
	`,
		session.Token,
		session.RefreshToken,
		session.ExpiresAt,
		session.ID,
	)
	return err
}
