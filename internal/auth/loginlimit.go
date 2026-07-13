package auth

import (
	"sync"
	"time"

	"github.com/sam33339999/wikibuild/internal/clock"
)

// LimiterConfig tunes brute-force protection for the login endpoint.
type LimiterConfig struct {
	// MaxAttempts is the number of failures within Window that trigger a
	// lockout.
	MaxAttempts int
	// Window is the sliding window over which failures are counted.
	Window time.Duration
	// Lockout is how long a key stays locked once MaxAttempts is reached.
	Lockout time.Duration
}

// DefaultLimiterConfig is sane for a single-admin blog login.
func DefaultLimiterConfig() LimiterConfig {
	return LimiterConfig{
		MaxAttempts: 5,
		Window:      15 * time.Minute,
		Lockout:     15 * time.Minute,
	}
}

// keyState holds the per-identity attempt history and lock state.
type keyState struct {
	failures    []time.Time
	lockedUntil time.Time
}

// LoginLimiter tracks failed login attempts per identity (typically client IP)
// and locks the identity out for a period once attempts exceed the threshold.
// It is safe for concurrent use and deterministic via an injected clock.Clock.
type LoginLimiter struct {
	mu    sync.Mutex
	clock clock.Clock
	cfg   LimiterConfig
	state map[string]*keyState
}

// NewLoginLimiter builds a limiter with the given clock and config.
func NewLoginLimiter(clk clock.Clock, cfg LimiterConfig) *LoginLimiter {
	return &LoginLimiter{clock: clk, cfg: cfg, state: make(map[string]*keyState)}
}

// IsLocked reports whether key is currently locked out.
func (l *LoginLimiter) IsLocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	st, ok := l.state[key]
	if !ok {
		return false
	}
	return l.clock.Now().Before(st.lockedUntil)
}

// RegisterFailure records a failed attempt for key. If the key is already
// locked, the attempt extends the lockout, frustrating continued guessing.
func (l *LoginLimiter) RegisterFailure(key string) {
	now := l.clock.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	st := l.state[key]
	if st == nil {
		st = &keyState{}
		l.state[key] = st
	}

	// Already locked: each attempt pushes the lockout further out.
	if now.Before(st.lockedUntil) {
		st.lockedUntil = now.Add(l.cfg.Lockout)
		return
	}

	st.failures = append(st.failures, now)
	cutoff := now.Add(-l.cfg.Window)
	// Drop failures that fell out of the window.
	kept := st.failures[:0]
	for _, f := range st.failures {
		if f.After(cutoff) {
			kept = append(kept, f)
		}
	}
	st.failures = kept

	if len(st.failures) >= l.cfg.MaxAttempts {
		st.lockedUntil = now.Add(l.cfg.Lockout)
		st.failures = nil
	}
}

// RegisterSuccess clears the attempt history for key.
func (l *LoginLimiter) RegisterSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.state, key)
}
