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
	Shell           string                 `toml:"shell"`
	MaxSessions     int                    `toml:"max_sessions"`
	Scrollback      int                    `toml:"scrollback"`
	IdleTimeout     string                 `toml:"idle_timeout"`
	PersistDir      string                 `toml:"persist_dir"`
	PersistMaxBytes int64                  `toml:"persist_max_bytes"`
	Resources       TerminalResourceConfig `toml:"resources"`
}

type TerminalResourceConfig struct {
	MinSystemAvailableBytes  int64   `toml:"min_system_available_bytes"`
	MaxSystemSwapUsedPercent float64 `toml:"max_system_swap_used_percent"`
	MaxMemoryPressureAvg10   float64 `toml:"max_memory_pressure_avg10"`
	SessionMemoryHighBytes   int64   `toml:"session_memory_high_bytes"`
	SessionMemoryMaxBytes    int64   `toml:"session_memory_max_bytes"`
	SessionSwapMaxBytes      int64   `toml:"session_swap_max_bytes"`
	SessionPIDsMax           int64   `toml:"session_pids_max"`
	SessionCPUWeight         int64   `toml:"session_cpu_weight"`
	SessionIOWeight          int64   `toml:"session_io_weight"`
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
			Shell:           "/bin/bash",
			MaxSessions:     0,
			Scrollback:      10000,
			IdleTimeout:     "72h",
			PersistMaxBytes: 64 << 20,
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
	resources := cfg.Terminal.Resources
	if resources.MinSystemAvailableBytes < 0 {
		return fmt.Errorf("terminal.resources.min_system_available_bytes must be 0 or greater")
	}
	if resources.MaxSystemSwapUsedPercent < 0 || resources.MaxSystemSwapUsedPercent > 100 {
		return fmt.Errorf("terminal.resources.max_system_swap_used_percent must be between 0 and 100")
	}
	if resources.MaxMemoryPressureAvg10 < 0 || resources.MaxMemoryPressureAvg10 > 100 {
		return fmt.Errorf("terminal.resources.max_memory_pressure_avg10 must be between 0 and 100")
	}
	if resources.SessionMemoryHighBytes < 0 || resources.SessionMemoryMaxBytes < 0 || resources.SessionSwapMaxBytes < 0 {
		return fmt.Errorf("terminal session memory resource limits must be 0 or greater")
	}
	if resources.SessionMemoryHighBytes > 0 && resources.SessionMemoryMaxBytes > 0 && resources.SessionMemoryHighBytes > resources.SessionMemoryMaxBytes {
		return fmt.Errorf("terminal.resources.session_memory_high_bytes must not exceed session_memory_max_bytes")
	}
	if resources.SessionPIDsMax < 0 {
		return fmt.Errorf("terminal.resources.session_pids_max must be 0 or greater")
	}
	if resources.SessionCPUWeight < 0 || resources.SessionCPUWeight > 10000 {
		return fmt.Errorf("terminal.resources.session_cpu_weight must be 0 or between 1 and 10000")
	}
	if resources.SessionIOWeight < 0 || resources.SessionIOWeight > 10000 {
		return fmt.Errorf("terminal.resources.session_io_weight must be 0 or between 1 and 10000")
	}
	return nil
}
