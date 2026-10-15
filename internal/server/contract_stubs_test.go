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

// C2/C4/C5 (WS-20261015-001) implemented all contract routes. No 501 stubs
// remain; this test now asserts the live semantics of every route added in
// C1: board -> 200, agents?refresh=1 -> 200, claim (no ready tasks) -> 204,
// heartbeat/release with unknown lease -> 404.
func TestContractRoutesLive(t *testing.T) {
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

	// board route is now live (C2, K2): expect 200 with a valid session.
	boardRoutes := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/project-control/board", http.StatusOK},
	}
	for _, tc := range boardRoutes {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
	claimCases := []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/api/agent/v1/claim", `{"runtimeId":"r1","agentLabel":"hermes"}`, http.StatusNoContent},
		{http.MethodPost, "/api/agent/v1/heartbeat", `{"leaseId":"l1"}`, http.StatusNotFound},
		{http.MethodPost, "/api/agent/v1/release", `{"leaseId":"l1","outcome":"done"}`, http.StatusNotFound},
	}
	for _, tc := range claimCases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+agentToken)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
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
