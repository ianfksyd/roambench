package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/config"
)

func TestSessionManagerCreateValidateInvalidate(t *testing.T) {
	sm, err := NewSessionManager(&config.AuthConfig{SessionTimeout: "1h"})
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

	token, err := sm.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	username, err := sm.ValidateSession(token)
	if err != nil {
		t.Fatalf("ValidateSession error: %v", err)
	}
	if username != "ian" {
		t.Fatalf("ValidateSession username = %q, want %q", username, "ian")
	}

	sm.InvalidateSession(token)

	_, err = sm.ValidateSession(token)
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("ValidateSession after invalidate error = %v, want %v", err, ErrSessionExpired)
	}
}

func TestSessionManagerExpiresSessions(t *testing.T) {
	sm, err := NewSessionManager(&config.AuthConfig{SessionTimeout: "1ms"})
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

	token, err := sm.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	_, err = sm.ValidateSession(token)
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("ValidateSession expired error = %v, want %v", err, ErrSessionExpired)
	}
}
