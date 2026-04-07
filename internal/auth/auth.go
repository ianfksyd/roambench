package auth

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/user/liteterm-web/internal/config"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrRateLimited        = errors.New("too many login attempts, try again later")
	ErrNotConfigured      = errors.New("authentication not configured")
)

// AuthProvider authenticates a username/password pair.
type AuthProvider interface {
	Authenticate(username, password string) error
}

// PasswordAuth authenticates against a bcrypt hash from config.
type PasswordAuth struct {
	hash       []byte
	singleUser string
}

func NewPasswordAuth(cfg *config.AuthConfig) (*PasswordAuth, error) {
	hash := cfg.PasswordHash
	if hash == "" {
		return nil, ErrNotConfigured
	}
	if cfg.SingleUser == "" {
		return nil, errors.New("single_user is required in password mode")
	}
	return &PasswordAuth{
		hash:       []byte(hash),
		singleUser: cfg.SingleUser,
	}, nil
}

func (p *PasswordAuth) Authenticate(username, password string) error {
	if p.singleUser != "" && username != p.singleUser {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword(p.hash, []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// HashPassword generates a bcrypt hash for the given password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// NewAuthProvider creates the appropriate provider based on config.
func NewAuthProvider(cfg *config.AuthConfig) (AuthProvider, error) {
	switch cfg.Method {
	case "pam":
		return NewPAMAuth()
	case "password", "":
		return NewPasswordAuth(cfg)
	default:
		return nil, fmt.Errorf("unknown auth method: %q", cfg.Method)
	}
}

// RateLimiter tracks login attempts per IP.
type RateLimiter struct {
	mu             sync.Mutex
	attempts       map[string][]time.Time
	maxAttempts    int
	lockoutWindow  time.Duration
}

func NewRateLimiter(maxAttempts int, lockoutDuration time.Duration) *RateLimiter {
	rl := &RateLimiter{
		attempts:      make(map[string][]time.Time),
		maxAttempts:   maxAttempts,
		lockoutWindow: lockoutDuration,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.lockoutWindow)
		for ip, attempts := range rl.attempts {
			var recent []time.Time
			for _, t := range attempts {
				if t.After(cutoff) {
					recent = append(recent, t)
				}
			}
			if len(recent) == 0 {
				delete(rl.attempts, ip)
			} else {
				rl.attempts[ip] = recent
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Check(ip string) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.lockoutWindow)

	// Filter to recent attempts only
	recent := make([]time.Time, 0)
	for _, t := range rl.attempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) == 0 {
		delete(rl.attempts, ip)
		return nil
	}
	rl.attempts[ip] = recent

	if len(recent) >= rl.maxAttempts {
		return ErrRateLimited
	}
	return nil
}

func (rl *RateLimiter) Record(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.attempts[ip] = append(rl.attempts[ip], time.Now())
}
