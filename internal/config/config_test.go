package config

import (
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

func TestTerminalConfigGetClientIdleDetach(t *testing.T) {
	cfg := Defaults()
	cfg.Terminal.ClientIdleDetach = "4m"

	if got := cfg.Terminal.GetClientIdleDetach(); got != 4*time.Minute {
		t.Fatalf("GetClientIdleDetach() = %v, want 4m", got)
	}

	cfg.Terminal.ClientIdleDetach = "0"
	if got := cfg.Terminal.GetClientIdleDetach(); got != 0 {
		t.Fatalf("GetClientIdleDetach() for 0 = %v, want 0", got)
	}

	cfg.Terminal.ClientIdleDetach = "not-a-duration"
	if got := cfg.Terminal.GetClientIdleDetach(); got != 0 {
		t.Fatalf("GetClientIdleDetach() for invalid value = %v, want 0", got)
	}
}

func TestValidateRejectsInvalidClientIdleDetach(t *testing.T) {
	cfg := Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Terminal.ClientIdleDetach = "soon"

	if err := Validate(cfg); err == nil {
		t.Fatal("Validate error = nil, want failure for invalid terminal.client_idle_detach")
	}

	cfg.Terminal.ClientIdleDetach = "-1s"
	if err := Validate(cfg); err == nil {
		t.Fatal("Validate error = nil, want failure for negative terminal.client_idle_detach")
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
