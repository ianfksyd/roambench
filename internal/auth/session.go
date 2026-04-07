package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/ianf339/roambench/internal/config"
)

const CookieName = "roambench_session"
const LegacyCookieName = "liteterm_session"

var ErrSessionExpired = errors.New("session expired")

type sessionEntry struct {
	Username  string
	ExpiresAt time.Time
}

// SessionManager handles revocable in-memory sessions.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]sessionEntry
	timeout  time.Duration
	cancel   context.CancelFunc
}

func NewSessionManager(cfg *config.AuthConfig) (*SessionManager, error) {
	ctx, cancel := context.WithCancel(context.Background())
	sm := &SessionManager{
		sessions: make(map[string]sessionEntry),
		timeout:  cfg.GetSessionTimeout(),
		cancel:   cancel,
	}
	go sm.cleanupLoop(ctx)
	return sm, nil
}

func (sm *SessionManager) Stop() {
	sm.cancel()
}

func (sm *SessionManager) CreateSession(username string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, tokenBytes); err != nil {
		return "", err
	}

	token := base64.RawURLEncoding.EncodeToString(tokenBytes)

	sm.mu.Lock()
	sm.sessions[token] = sessionEntry{
		Username:  username,
		ExpiresAt: time.Now().Add(sm.timeout),
	}
	sm.mu.Unlock()

	return token, nil
}

func (sm *SessionManager) ValidateSession(cookieValue string) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[cookieValue]
	if !ok {
		return "", ErrSessionExpired
	}
	if time.Now().After(session.ExpiresAt) {
		delete(sm.sessions, cookieValue)
		return "", ErrSessionExpired
	}

	// Sliding window: extend session on activity
	session.ExpiresAt = time.Now().Add(sm.timeout)
	sm.sessions[cookieValue] = session

	return session.Username, nil
}

func (sm *SessionManager) InvalidateSession(cookieValue string) {
	sm.mu.Lock()
	delete(sm.sessions, cookieValue)
	sm.mu.Unlock()
}

func (sm *SessionManager) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			sm.mu.Lock()
			for token, session := range sm.sessions {
				if now.After(session.ExpiresAt) {
					delete(sm.sessions, token)
				}
			}
			sm.mu.Unlock()
		}
	}
}
