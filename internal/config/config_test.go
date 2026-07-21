package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateRejectsNonPositiveScrollback(t *testing.T) {
	cfg := Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Terminal.Scrollback = 0

	if err := Validate(cfg); err == nil {
		t.Fatal("Validate error = nil, want failure for non-positive terminal.scrollback")
	}
}

func TestServerConfigGetBasePathNormalizesValues(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: "/"},
		{name: "root", raw: "/", want: "/"},
		{name: "missing leading slash", raw: "home/exampleuser", want: "/home/exampleuser"},
		{name: "trailing slash", raw: "/home/exampleuser/", want: "/home/exampleuser"},
		{name: "double slashes", raw: "//home//exampleuser//", want: "/home/exampleuser"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ServerConfig{BasePath: tt.raw}
			if got := cfg.GetBasePath(); got != tt.want {
				t.Fatalf("GetBasePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadTerminalResourceControls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roambench.toml")
	content := `[server]
allow_all_ips = true

[terminal.resources]
min_system_available_bytes = 2147483648
max_system_swap_used_percent = 75
max_memory_pressure_avg10 = 10
session_memory_high_bytes = 3221225472
session_memory_max_bytes = 5368709120
session_swap_max_bytes = 536870912
session_pids_max = 256
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	resources := cfg.Terminal.Resources
	if resources.MinSystemAvailableBytes != 2<<30 || resources.MaxSystemSwapUsedPercent != 75 || resources.MaxMemoryPressureAvg10 != 10 {
		t.Fatalf("admission resources = %#v", resources)
	}
	if resources.SessionMemoryHighBytes != 3<<30 || resources.SessionMemoryMaxBytes != 5<<30 || resources.SessionSwapMaxBytes != 512<<20 || resources.SessionPIDsMax != 256 {
		t.Fatalf("session resources = %#v", resources)
	}
}

func TestValidateRejectsInvalidTerminalResourceControls(t *testing.T) {
	tests := []struct {
		name      string
		resources TerminalResourceConfig
	}{
		{name: "negative available memory", resources: TerminalResourceConfig{MinSystemAvailableBytes: -1}},
		{name: "swap percent above 100", resources: TerminalResourceConfig{MaxSystemSwapUsedPercent: 101}},
		{name: "negative pressure", resources: TerminalResourceConfig{MaxMemoryPressureAvg10: -1}},
		{name: "high above max", resources: TerminalResourceConfig{SessionMemoryHighBytes: 6 << 30, SessionMemoryMaxBytes: 5 << 30}},
		{name: "negative swap limit", resources: TerminalResourceConfig{SessionSwapMaxBytes: -1}},
		{name: "negative PID limit", resources: TerminalResourceConfig{SessionPIDsMax: -1}},
		{name: "CPU weight above maximum", resources: TerminalResourceConfig{SessionCPUWeight: 10001}},
		{name: "negative IO weight", resources: TerminalResourceConfig{SessionIOWeight: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Server.AllowAllIPs = true
			cfg.Terminal.Resources = tt.resources
			if err := Validate(cfg); err == nil {
				t.Fatal("Validate error = nil, want invalid resource-control error")
			}
		})
	}
}

func TestTelegramConfigValidate(t *testing.T) {
	t.Run("disabled is always valid", func(t *testing.T) {
		cfg := Defaults()
		cfg.Telegram.Enabled = false
		cfg.Telegram.BotToken = ""
		cfg.Telegram.ChatID = ""
		if err := cfg.Telegram.Validate(); err != nil {
			t.Fatalf("Validate error = %v, want nil", err)
		}
	})

	t.Run("enabled requires token and chat", func(t *testing.T) {
		cfg := Defaults()
		cfg.Telegram.Enabled = true
		if err := cfg.Telegram.Validate(); err == nil {
			t.Fatal("Validate error = nil, want failure for missing token and chat_id")
		}
		cfg.Telegram.BotToken = "token"
		if err := cfg.Telegram.Validate(); err == nil {
			t.Fatal("Validate error = nil, want failure for missing chat_id")
		}
		cfg.Telegram.ChatID = "123"
		if err := cfg.Telegram.Validate(); err != nil {
			t.Fatalf("Validate error = %v, want nil", err)
		}
	})

	t.Run("webhook requires HTTPS URL", func(t *testing.T) {
		cfg := Defaults()
		cfg.Telegram.Enabled = true
		cfg.Telegram.BotToken = "token"
		cfg.Telegram.ChatID = "123"
		cfg.Telegram.Mode = "webhook"
		if err := cfg.Telegram.Validate(); err == nil {
			t.Fatal("Validate error = nil, want failure for missing webhook_url")
		}
		cfg.Telegram.WebhookURL = "http://example.com/webhook"
		if err := cfg.Telegram.Validate(); err == nil || !strings.Contains(err.Error(), "HTTPS") {
			t.Fatalf("Validate error = %v, want HTTPS failure", err)
		}
		cfg.Telegram.WebhookURL = "https://example.com/webhook"
		if err := cfg.Telegram.Validate(); err != nil {
			t.Fatalf("Validate error = %v, want nil", err)
		}
	})

	t.Run("polling interval clamps to minimum", func(t *testing.T) {
		cfg := Defaults()
		cfg.Telegram.PollingInterval = "100ms"
		d, err := cfg.Telegram.GetPollingInterval()
		if err != nil {
			t.Fatalf("GetPollingInterval error = %v", err)
		}
		if d != 1*time.Second {
			t.Fatalf("GetPollingInterval() = %v, want 1s", d)
		}
	})

	t.Run("default mode is polling", func(t *testing.T) {
		cfg := TelegramConfig{}
		if got := cfg.GetMode(); got != "polling" {
			t.Fatalf("GetMode() = %q, want polling", got)
		}
	})
}
