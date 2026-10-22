package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
)

type fsRootsResponse struct {
	Roots []struct {
		Path   string `json:"path"`
		Label  string `json:"label"`
		Source string `json:"source"`
	} `json:"roots"`
	AllowUserRoots bool `json:"allow_user_roots"`
}

func rootsRequest(t *testing.T, srv *Server, token, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/api/fs/roots"+path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	return rec
}

func TestFSRootsListsInstanceRoots(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	rec := rootsRequest(t, srv, token, http.MethodGet, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body fsRootsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.AllowUserRoots {
		t.Error("allow_user_roots = false, want true by default")
	}
	if len(body.Roots) == 0 || body.Roots[0].Path != "/" {
		t.Fatalf("roots = %+v, want the filesystem root first", body.Roots)
	}
}

// storePersonalRoot adds a root through the API and returns an error when
// the request is rejected.
func storePersonalRoot(t *testing.T, srv *Server, token, path string) error {
	t.Helper()

	rec := rootsRequest(t, srv, token, http.MethodPost, "", `{"path":"`+path+`"}`)
	if rec.Code != http.StatusOK {
		return errors.New(rec.Body.String())
	}
	return nil
}

func TestFSRootsAddsAndRemovesPersonalRoot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	target := t.TempDir()
	rec := rootsRequest(t, srv, token, http.MethodPost, "", `{"path":"`+target+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("add status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body fsRootsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range body.Roots {
		if entry.Path == target && entry.Source == "user" {
			found = true
		}
	}
	if !found {
		t.Fatalf("roots = %+v, want the personal root", body.Roots)
	}

	// The personal root is persisted in the per-user file.
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	if err := storePersonalRoot(t, srv, token, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(configHome, "roambench", "roots.toml")); err != nil {
		t.Fatalf("personal roots file missing: %v", err)
	}

	rec = rootsRequest(t, srv, token, http.MethodDelete, "?path="+target, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remove status=%d body=%s", rec.Code, rec.Body.String())
	}
	body = fsRootsResponse{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, entry := range body.Roots {
		if entry.Path == target {
			t.Fatalf("roots = %+v, want the personal root removed", body.Roots)
		}
	}
}

func TestFSRootsRejectsUnusablePath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	rec := rootsRequest(t, srv, token, http.MethodPost, "", `{"path":"/definitely-not-here-xyz"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", rec.Code, rec.Body.String())
	}
	rec = rootsRequest(t, srv, token, http.MethodPost, "", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", rec.Code, rec.Body.String())
	}
}

func TestFSRootsRequiresAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, _, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/fs/roots", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}
}
