package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
)

func agentConfigServer(t *testing.T) (*Server, string, *auth.SessionManager) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, token, sessions := testProjectControlServer(t)
	srv.cfg.Agent.Gateway.Name = "magpie"
	srv.cfg.Agent.Gateway.BaseURL = "http://127.0.0.1:3425"
	srv.cfg.Agent.Gateway.AuthToken = "magpie"
	srv.cfg.Agent.Gateway.CodexPath = "/backend-api/codex"
	return srv, token, sessions
}

func TestAgentConfigOverviewListsAgents(t *testing.T) {
	srv, token, sessions := agentConfigServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/project-control/agent-config", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var overview agentConfigOverview
	if err := json.Unmarshal(rec.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if overview.Profile.BaseURL != "http://127.0.0.1:3425" {
		t.Errorf("profile = %+v", overview.Profile)
	}
	labels := map[string]bool{}
	for _, agent := range overview.Agents {
		labels[agent.Label] = true
	}
	for _, want := range []string{"claude", "codex", "opencode"} {
		if !labels[want] {
			t.Errorf("agent %q missing from the overview: %+v", want, overview.Agents)
		}
	}
}

func TestAgentConfigPlanAndApply(t *testing.T) {
	srv, token, sessions := agentConfigServer(t)
	defer sessions.Stop()
	// The server works on the home of the signed-in user, which the test
	// pins to a temporary directory. Read it after the setup so the test can
	// never touch the real home directory.
	home := os.Getenv("HOME")
	if home == "" || home == "/home/ian" {
		t.Fatalf("refusing to run against the real home: %q", home)
	}

	// An existing claude config that still points at the vendor.
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	original := `{"env":{"ANTHROPIC_BASE_URL":"https://api.anthropic.com"},"theme":"dark"}`
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	// The plan reports the differences without touching the file.
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/agent-config?agent=claude",
		strings.NewReader(`{"profile":{"baseUrl":"http://127.0.0.1:3425","authToken":"magpie","model":"auto/glm-5"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("plan status=%d body=%s", rec.Code, rec.Body.String())
	}
	var plan struct {
		Changes []struct {
			Key    string `json:"key"`
			Before string `json:"before"`
			After  string `json:"after"`
			Action string `json:"action"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) == 0 {
		t.Fatal("expected changes in the plan")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatal("the plan must not modify the file")
	}

	// Applying writes the profile and keeps a backup.
	req = httptest.NewRequest(http.MethodPost, "/api/project-control/agent-config?agent=claude&action=apply",
		strings.NewReader(`{"profile":{"baseUrl":"http://127.0.0.1:3425","authToken":"magpie","model":"auto/glm-5"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status=%d body=%s", rec.Code, rec.Body.String())
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "http://127.0.0.1:3425") {
		t.Fatalf("base url not written:\n%s", content)
	}
	if !strings.Contains(string(content), `"theme": "dark"`) {
		t.Fatalf("unrelated settings lost:\n%s", content)
	}
	backup, err := os.ReadFile(path + ".roambench-bak")
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(backup) != original {
		t.Fatalf("backup = %q, want %q", backup, original)
	}
}

func TestAgentConfigGuideReturnsSteps(t *testing.T) {
	srv, token, sessions := agentConfigServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/project-control/agent-config?agent=claude", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Steps []struct {
			Title   string `json:"title"`
			Command string `json:"command"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Steps) == 0 {
		t.Fatal("expected guide steps")
	}
}

func TestAgentConfigRequiresAuth(t *testing.T) {
	srv, _, sessions := agentConfigServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/project-control/agent-config", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}
}
