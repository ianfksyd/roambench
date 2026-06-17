package config

import (
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Auth     AuthConfig     `toml:"auth"`
	Terminal TerminalConfig `toml:"terminal"`
	UI       UIConfig       `toml:"ui"`
}

type ServerConfig struct {
	Host              string   `toml:"host"`
	Port              int      `toml:"port"`
	BasePath          string   `toml:"base_path"`
	TLSCert           string   `toml:"tls_cert"`
	TLSKey            string   `toml:"tls_key"`
	AllowInsecureHTTP bool     `toml:"allow_insecure_http"`
	AllowedIPs        []string `toml:"allowed_ips"`
	AllowAllIPs       bool     `toml:"allow_all_ips"`
	TrustProxy        bool     `toml:"trust_proxy"`
}

type AuthConfig struct {
	Method           string `toml:"method"`
	SessionTimeout   string `toml:"session_timeout"`
	MaxLoginAttempts int    `toml:"max_login_attempts"`
	LockoutDuration  string `toml:"lockout_duration"`
	PasswordHash     string `toml:"password_hash"`
	SingleUser       string `toml:"single_user"`
}

type TerminalConfig struct {
	Shell            string `toml:"shell"`
	MaxSessions      int    `toml:"max_sessions"`
	Scrollback       int    `toml:"scrollback"`
	IdleTimeout      string `toml:"idle_timeout"`
	ClientIdleDetach string `toml:"client_idle_detach"`
	PersistDir       string `toml:"persist_dir"`
	PersistMaxBytes  int64  `toml:"persist_max_bytes"`
}

type UIConfig struct {
	Title string `toml:"title"`
	MOTD  string `toml:"motd"`
}

func Defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Host:       "0.0.0.0",
			Port:       3000,
			BasePath:   "/",
			AllowedIPs: []string{"127.0.0.1"},
		},
		Auth: AuthConfig{
			Method:           "password",
			SessionTimeout:   "24h",
			MaxLoginAttempts: 5,
			LockoutDuration:  "15m",
		},
		Terminal: TerminalConfig{
			Shell:            "/bin/bash",
			MaxSessions:      0,
			Scrollback:       10000,
			IdleTimeout:      "72h",
			ClientIdleDetach: "0",
			PersistMaxBytes:  64 << 20,
		},
		UI: UIConfig{
			Title: "RoamBench",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Defaults()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func FindAndLoad() *Config {
	paths := []string{
		"roambench.toml",
		os.ExpandEnv("$HOME/.config/roambench/roambench.toml"),
		"/etc/roambench/roambench.toml",
	}
	for _, p := range paths {
		if cfg, err := Load(p); err == nil {
			return cfg
		}
	}
	return Defaults()
}

func (c *AuthConfig) GetSessionTimeout() time.Duration {
	d, err := time.ParseDuration(c.SessionTimeout)
	if err != nil {
		return 24 * time.Hour
	}
	return d
}

func (c *AuthConfig) GetLockoutDuration() time.Duration {
	d, err := time.ParseDuration(c.LockoutDuration)
	if err != nil {
		return 15 * time.Minute
	}
	return d
}

func (c *TerminalConfig) GetIdleTimeout() time.Duration {
	d, err := time.ParseDuration(c.IdleTimeout)
	if err != nil {
		return 72 * time.Hour
	}
	return d
}

func (c *TerminalConfig) GetClientIdleDetach() time.Duration {
	raw := strings.TrimSpace(c.ClientIdleDetach)
	if raw == "" || raw == "0" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return 0
	}
	return d
}

func (c *TerminalConfig) GetPersistDir() string {
	if strings.TrimSpace(c.PersistDir) != "" {
		return c.PersistDir
	}

	homeDir, err := os.UserHomeDir()
	if err == nil && homeDir != "" {
		return filepath.Join(homeDir, ".local", "state", "roambench", "terminals")
	}

	return filepath.Join(os.TempDir(), "roambench", "terminals")
}

func (c *TerminalConfig) GetPersistMaxBytes() int64 {
	if c.PersistMaxBytes > 0 {
		return c.PersistMaxBytes
	}
	return 64 << 20
}

func (c *ServerConfig) GetBasePath() string {
	raw := strings.TrimSpace(c.BasePath)
	if raw == "" || raw == "/" {
		return "/"
	}

	normalized := raw
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	normalized = path.Clean(normalized)
	if normalized == "." || normalized == "" {
		return "/"
	}
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	if normalized != "/" {
		normalized = strings.TrimRight(normalized, "/")
	}
	if normalized == "" {
		return "/"
	}
	return normalized
}

func Validate(cfg *Config) error {
	if !cfg.Server.AllowAllIPs {
		if len(cfg.Server.AllowedIPs) == 0 {
			return fmt.Errorf("allowed_ips must contain at least one IP when allow_all_ips is false")
		}
		for _, ip := range cfg.Server.AllowedIPs {
			if net.ParseIP(ip) == nil {
				return fmt.Errorf("invalid IP address in allowed_ips: %q", ip)
			}
		}
	}
	if cfg.Terminal.Scrollback <= 0 {
		return fmt.Errorf("terminal.scrollback must be greater than 0")
	}
	if cfg.Terminal.MaxSessions < 0 {
		return fmt.Errorf("terminal.max_sessions must be 0 or greater")
	}
	if cfg.Terminal.PersistMaxBytes < 0 {
		return fmt.Errorf("terminal.persist_max_bytes must be 0 or greater")
	}
	clientIdleDetach := strings.TrimSpace(cfg.Terminal.ClientIdleDetach)
	if clientIdleDetach != "" && clientIdleDetach != "0" {
		duration, err := time.ParseDuration(clientIdleDetach)
		if err != nil {
			return fmt.Errorf("terminal.client_idle_detach must be a valid duration or 0")
		}
		if duration < 0 {
			return fmt.Errorf("terminal.client_idle_detach must be 0 or greater")
		}
	}
	return nil
}
