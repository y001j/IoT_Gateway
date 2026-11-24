package services

import (
	"strings"
	"sync"
	"time"
)

// ErrAccountLocked 连续登录失败次数过多，账号被临时锁定
var ErrAccountLocked = errorString("登录失败次数过多，账号已被临时锁定，请稍后再试")

type errorString string

func (e errorString) Error() string { return string(e) }

// loginGuard 记录每个用户名的连续登录失败次数，实现 web_ui.auth.max_login_attempts /
// lockout_duration（以前这两个配置项存在但从未生效，可以无限次暴力尝试密码）。
//
// 按用户名计数，不区分用户是否存在，避免通过锁定行为枚举用户名。
type loginGuard struct {
	mu          sync.Mutex
	maxAttempts int
	lockout     time.Duration
	states      map[string]*loginState
	now         func() time.Time
}

type loginState struct {
	failures    int
	lockedUntil time.Time
	lastFailure time.Time
}

func newLoginGuard(maxAttempts int, lockout time.Duration) *loginGuard {
	if lockout <= 0 {
		lockout = 15 * time.Minute
	}
	return &loginGuard{maxAttempts: maxAttempts, lockout: lockout, states: map[string]*loginState{}, now: time.Now}
}

func normalizeUsername(u string) string { return strings.ToLower(strings.TrimSpace(u)) }

// locked 返回该用户名当前是否处于锁定状态
func (g *loginGuard) locked(username string) bool {
	if g == nil || g.maxAttempts <= 0 {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.states[normalizeUsername(username)]
	return ok && g.now().Before(st.lockedUntil)
}

// recordFailure 记录一次失败，返回是否因此进入锁定
func (g *loginGuard) recordFailure(username string) bool {
	if g == nil || g.maxAttempts <= 0 {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	key := normalizeUsername(username)
	st, ok := g.states[key]
	if !ok {
		if len(g.states) > 10000 {
			g.purgeLocked(now)
		}
		st = &loginState{}
		g.states[key] = st
	}
	// 距离上次失败已超过锁定时长，重新计数
	if !st.lastFailure.IsZero() && now.Sub(st.lastFailure) > g.lockout {
		st.failures = 0
	}
	st.failures++
	st.lastFailure = now
	if st.failures >= g.maxAttempts {
		st.failures = 0
		st.lockedUntil = now.Add(g.lockout)
		return true
	}
	return false
}

// recordSuccess 登录成功后清除失败记录
func (g *loginGuard) recordSuccess(username string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	delete(g.states, normalizeUsername(username))
	g.mu.Unlock()
}

func (g *loginGuard) purgeLocked(now time.Time) {
	for k, st := range g.states {
		if now.After(st.lockedUntil) && now.Sub(st.lastFailure) > g.lockout {
			delete(g.states, k)
		}
	}
}
