package main

import (
	"os/user"
	"testing"

	"github.com/user/liteterm-web/internal/config"
)

func TestValidateSecurityConfig(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current error: %v", err)
	}

	t.Run("allows loopback without TLS", func(t *testing.T) {
		cfg := config.Defaults()
		cfg.Server.Host = "127.0.0.1"
		cfg.Auth.SingleUser = currentUser.Username

		if err := validateSecurityConfig(cfg); err != nil {
			t.Fatalf("validateSecurityConfig error: %v", err)
		}
	})

	t.Run("rejects missing single user", func(t *testing.T) {
		cfg := config.Defaults()
		cfg.Server.Host = "127.0.0.1"
		cfg.Auth.SingleUser = ""

		if err := validateSecurityConfig(cfg); err == nil {
			t.Fatal("validateSecurityConfig error = nil, want failure for missing single_user")
		}
	})

	t.Run("rejects insecure remote http by default", func(t *testing.T) {
		cfg := config.Defaults()
		cfg.Server.Host = "0.0.0.0"
		cfg.Auth.SingleUser = currentUser.Username

		if err := validateSecurityConfig(cfg); err == nil {
			t.Fatal("validateSecurityConfig error = nil, want failure for insecure remote HTTP")
		}
	})

	t.Run("allows insecure remote http when explicitly enabled", func(t *testing.T) {
		cfg := config.Defaults()
		cfg.Server.Host = "0.0.0.0"
		cfg.Server.AllowInsecureHTTP = true
		cfg.Auth.SingleUser = currentUser.Username

		if err := validateSecurityConfig(cfg); err != nil {
			t.Fatalf("validateSecurityConfig error: %v", err)
		}
	})
}
