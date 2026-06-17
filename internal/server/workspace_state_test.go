package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
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
		"activeWorkspaceId":"view-4",
		"workspaces":[
			{"id":"view-1","layout":"2","terminalIds":["term-1","term-2"],"name":"Main","labelNumber":3},
			{"id":"view-2","layout":"4w","terminalIds":["term-3","","",""],"name":"Ops","labelNumber":7},
			{"id":"view-3","layout":"3","terminalIds":["term-4","term-5","term-6",""],"name":"Stacked","labelNumber":8},
			{"id":"view-4","layout":"3w","terminalIds":["term-7","term-8","term-9",""],"name":"Columns","labelNumber":9}
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
	if resp.ActiveWorkspaceID != "view-4" {
		t.Fatalf("ActiveWorkspaceID = %q, want %q", resp.ActiveWorkspaceID, "view-4")
	}
	if len(resp.Workspaces) != 4 {
		t.Fatalf("len(Workspaces) = %d, want %d", len(resp.Workspaces), 4)
	}
	if resp.Workspaces[0].Layout != "2" {
		t.Fatalf("Workspaces[0].Layout = %q, want %q", resp.Workspaces[0].Layout, "2")
	}
	if resp.Workspaces[2].Layout != "3" {
		t.Fatalf("Workspaces[2].Layout = %q, want %q", resp.Workspaces[2].Layout, "3")
	}
	if resp.Workspaces[3].Layout != "3w" {
		t.Fatalf("Workspaces[3].Layout = %q, want %q", resp.Workspaces[3].Layout, "3w")
	}
	if got := len(resp.Workspaces[0].TerminalIDs); got != 4 {
		t.Fatalf("len(Workspaces[0].TerminalIDs) = %d, want %d", got, 4)
	}
	if resp.Workspaces[1].Name != "Ops" {
		t.Fatalf("Workspaces[1].Name = %q, want %q", resp.Workspaces[1].Name, "Ops")
	}
	if resp.Workspaces[0].LabelNumber != 3 {
		t.Fatalf("Workspaces[0].LabelNumber = %d, want %d", resp.Workspaces[0].LabelNumber, 3)
	}
	if resp.Workspaces[1].LabelNumber != 7 {
		t.Fatalf("Workspaces[1].LabelNumber = %d, want %d", resp.Workspaces[1].LabelNumber, 7)
	}
	if resp.Workspaces[3].LabelNumber != 9 {
		t.Fatalf("Workspaces[3].LabelNumber = %d, want %d", resp.Workspaces[3].LabelNumber, 9)
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

func testWorkspaceStatePayload(updatedAt, activeWorkspaceID, workspaceName string) workspaceStatePayload {
	return workspaceStatePayload{
		ActiveWorkspaceID: activeWorkspaceID,
		Workspaces: []workspaceStateRecord{{
			ID:          activeWorkspaceID,
			Layout:      "2",
			TerminalIDs: []string{"term-1", "term-2"},
			Name:        workspaceName,
			LabelNumber: 1,
		}},
		UpdatedAt: updatedAt,
	}
}

func TestWorkspaceStateLoadRecoversFromNewerWAL(t *testing.T) {
	store := newWorkspaceStateStore(t.TempDir())
	base := testWorkspaceStatePayload("2026-01-01T00:00:00Z", "view-1", "Base")
	if err := store.Save("ian", base); err != nil {
		t.Fatalf("Save(base) error: %v", err)
	}

	recovered := testWorkspaceStatePayload("2026-01-01T00:01:00Z", "view-2", "Recovered")
	payload, err := json.Marshal(recovered)
	if err != nil {
		t.Fatalf("Marshal(recovered) error: %v", err)
	}
	if err := workspaceStateWriteFileAtomically(store.walPathFor("ian"), payload, 0600); err != nil {
		t.Fatalf("Write WAL error: %v", err)
	}

	loaded, exists, err := store.Load("ian")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if !exists {
		t.Fatal("expected workspace state to exist after WAL recovery")
	}
	if loaded.ActiveWorkspaceID != "view-2" {
		t.Fatalf("ActiveWorkspaceID = %q, want %q", loaded.ActiveWorkspaceID, "view-2")
	}
	if loaded.Workspaces[0].Name != "Recovered" {
		t.Fatalf("Workspaces[0].Name = %q, want %q", loaded.Workspaces[0].Name, "Recovered")
	}
	if _, err := os.Stat(store.walPathFor("ian")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("walPath still exists after recovery: %v", err)
	}

	persisted, exists, err := workspaceStateReadFile(store.pathFor("ian"))
	if err != nil {
		t.Fatalf("Read persisted state error: %v", err)
	}
	if !exists {
		t.Fatal("expected persisted workspace state after WAL recovery")
	}
	if persisted.ActiveWorkspaceID != "view-2" {
		t.Fatalf("persisted ActiveWorkspaceID = %q, want %q", persisted.ActiveWorkspaceID, "view-2")
	}
}

func TestWorkspaceStateLoadUsesWALWhenMainStateIsMissing(t *testing.T) {
	store := newWorkspaceStateStore(t.TempDir())
	recovered := testWorkspaceStatePayload("2026-01-01T00:02:00Z", "view-3", "Recovered from WAL only")
	payload, err := json.Marshal(recovered)
	if err != nil {
		t.Fatalf("Marshal(recovered) error: %v", err)
	}
	if err := workspaceStateWriteFileAtomically(store.walPathFor("ian"), payload, 0600); err != nil {
		t.Fatalf("Write WAL error: %v", err)
	}

	loaded, exists, err := store.Load("ian")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if !exists {
		t.Fatal("expected workspace state to recover from WAL without main state")
	}
	if loaded.ActiveWorkspaceID != "view-3" {
		t.Fatalf("ActiveWorkspaceID = %q, want %q", loaded.ActiveWorkspaceID, "view-3")
	}
	if _, err := os.Stat(store.pathFor("ian")); err != nil {
		t.Fatalf("main state path missing after WAL recovery: %v", err)
	}
	if _, err := os.Stat(store.walPathFor("ian")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("walPath still exists after recovery: %v", err)
	}
}
