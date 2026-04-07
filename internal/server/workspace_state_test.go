package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/config"
)

func TestWorkspaceStateRouteRoundTripsPersistedViews(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	srv := NewServer(cfg, nil, sessions, nil, nil)
	putReq := httptest.NewRequest(http.MethodPut, "/api/workspace-state", strings.NewReader(`{
		"activeWorkspaceId":"view-2",
		"workspaces":[
			{"id":"view-1","layout":"2","terminalIds":["term-1","term-2"],"name":"Main"},
			{"id":"view-2","layout":"4w","terminalIds":["term-3","","",""],"name":"Ops"}
		]
	}`))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	putRec := httptest.NewRecorder()

	srv.mux.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT /api/workspace-state status = %d, want %d", putRec.Code, http.StatusOK)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/workspace-state", nil)
	getReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	getRec := httptest.NewRecorder()

	srv.mux.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspace-state status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var resp workspaceStatePayload
	if err := json.NewDecoder(getRec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if resp.ActiveWorkspaceID != "view-2" {
		t.Fatalf("ActiveWorkspaceID = %q, want %q", resp.ActiveWorkspaceID, "view-2")
	}
	if len(resp.Workspaces) != 2 {
		t.Fatalf("len(Workspaces) = %d, want %d", len(resp.Workspaces), 2)
	}
	if resp.Workspaces[0].Layout != "2" {
		t.Fatalf("Workspaces[0].Layout = %q, want %q", resp.Workspaces[0].Layout, "2")
	}
	if got := len(resp.Workspaces[0].TerminalIDs); got != 4 {
		t.Fatalf("len(Workspaces[0].TerminalIDs) = %d, want %d", got, 4)
	}
	if resp.Workspaces[1].Name != "Ops" {
		t.Fatalf("Workspaces[1].Name = %q, want %q", resp.Workspaces[1].Name, "Ops")
	}
	if resp.UpdatedAt == "" {
		t.Fatal("UpdatedAt = empty, want non-empty timestamp")
	}
}

func TestWorkspaceStateRouteReturnsEmptyPayloadWhenMissing(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	srv := NewServer(cfg, nil, sessions, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/workspace-state", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspace-state status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp workspaceStatePayload
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(resp.Workspaces) != 0 {
		t.Fatalf("len(Workspaces) = %d, want 0", len(resp.Workspaces))
	}
	if resp.ActiveWorkspaceID != "" {
		t.Fatalf("ActiveWorkspaceID = %q, want empty", resp.ActiveWorkspaceID)
	}
}
