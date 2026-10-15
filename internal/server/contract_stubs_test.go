package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
)

// C1 contract stub tests (WS-20261015-001).

func TestContractStubRoutesReturn501(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	// agent token for claim family
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/project-control/agent-token", nil)
	tokenReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	tokenRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(tokenRec, tokenReq)
	var tokenResp map[string]string
	json.NewDecoder(tokenRec.Body).Decode(&tokenResp)
	agentToken := tokenResp["token"]

	cases := []struct {
		method, path, auth string
		body               string
	}{
		{http.MethodGet, "/api/project-control/board", "session", ""},
		{http.MethodGet, "/api/project-control/agents?refresh=1", "session", ""},
		{http.MethodPost, "/api/agent/v1/claim", "agent", `{"runtimeId":"r1","agentLabel":"hermes"}`},
		{http.MethodPost, "/api/agent/v1/heartbeat", "agent", `{"leaseId":"l1"}`},
		{http.MethodPost, "/api/agent/v1/release", "agent", `{"leaseId":"l1","outcome":"done"}`},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		switch tc.auth {
		case "session":
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		case "agent":
			req.Header.Set("Authorization", "Bearer "+agentToken)
		}
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s %s = %d, want 501", tc.method, tc.path, rec.Code)
		}
	}
}

func TestContractStubRoutesRequireAuth(t *testing.T) {
	srv, _, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	for _, path := range []string{"/api/project-control/board", "/api/project-control/agents"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without session = %d, want 401", path, rec.Code)
		}
	}
	for _, path := range []string{"/api/agent/v1/claim", "/api/agent/v1/heartbeat", "/api/agent/v1/release"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without token = %d, want 401", path, rec.Code)
		}
	}
}

func TestValidateProjectControlRepoPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	// empty -> ok (falls back to runtimeRootDir)
	if v, err := validateProjectControlRepoPath(""); err != nil || v != "" {
		t.Errorf("empty repoPath = %q, err %v; want empty ok", v, err)
	}
	// relative -> rejected
	if _, err := validateProjectControlRepoPath("relative/path"); err == nil {
		t.Error("relative repoPath should be rejected")
	}
	// outside home -> rejected
	if _, err := validateProjectControlRepoPath("/etc"); err == nil {
		t.Error("/etc should be rejected (outside home)")
	}
	if _, err := validateProjectControlRepoPath(home + "/../elsewhere"); err == nil {
		t.Error("home/../elsewhere should be rejected")
	}
	// inside home but nonexistent -> rejected
	if _, err := validateProjectControlRepoPath(home + "/definitely-not-here-xyz"); err == nil {
		t.Error("nonexistent dir should be rejected")
	}
	// inside home and exists -> ok
	if _, err := validateProjectControlRepoPath(home); err != nil {
		t.Errorf("home dir should be accepted: %v", err)
	}
}
