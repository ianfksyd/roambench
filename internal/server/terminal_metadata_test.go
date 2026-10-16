package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/terminal"
)

func TestTerminalCreateSemanticMetadata(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	defer srv.Shutdown(context.Background())
	mgr := terminal.NewManager(&srv.cfg.Terminal)
	defer mgr.Stop()
	srv.terminals = mgr
	created := make(map[string]terminal.SessionInfo)
	for _, body := range []string{`{"name":" rb-schema-review ","taskId":"task-1","projectId":"project-1"}`, ""} {
		req := httptest.NewRequest(http.MethodPost, "/api/terminals", strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
		}
		var info terminal.SessionInfo
		if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
			t.Fatal(err)
		}
		created[info.ID] = info
		defer mgr.KillSession(info.ID)
		if body != "" && (info.Name != "rb-schema-review" || info.TaskID != "task-1" || info.ProjectID != "project-1") {
			t.Fatalf("metadata: %+v", info)
		}
		if info.LastActivity.IsZero() || info.Status == "" {
			t.Fatalf("missing activity: %+v", info)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/terminals", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var infos []terminal.SessionInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &infos); err != nil {
		t.Fatal(err)
	}
	// The manager may discover orphan sessions from other tests on this package's
	// private tmux server. Check our exact IDs without assuming an empty inventory.
	for _, info := range infos {
		if want, ok := created[info.ID]; ok {
			if info.Name != want.Name || info.ProjectID != want.ProjectID || info.TaskID != want.TaskID {
				t.Fatalf("listed metadata: %+v, want %+v", info, want)
			}
			delete(created, info.ID)
		}
	}
	if len(created) != 0 {
		t.Fatalf("created sessions missing from list: %+v", created)
	}
}
