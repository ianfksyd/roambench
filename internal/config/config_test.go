package config

import "testing"

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
		{name: "missing leading slash", raw: "home/ianf339", want: "/home/ianf339"},
		{name: "trailing slash", raw: "/home/ianf339/", want: "/home/ianf339"},
		{name: "double slashes", raw: "//home//ianf339//", want: "/home/ianf339"},
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
