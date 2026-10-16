package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
)

func TestFSListHomeBoundary(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	outside := filepath.Join(root, "outside")
	sibling := filepath.Join(root, "home-other")
	for _, dir := range []string{home, outside, sibling, filepath.Join(home, "alpha"), filepath.Join(home, "zeta")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "file.txt"), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"safe-link": filepath.Join(home, "alpha"), "escape": outside, "broken": filepath.Join(root, "absent")} {
		if err := os.Symlink(target, filepath.Join(home, name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	defer srv.Shutdown(context.Background())
	for _, tc := range []struct {
		name, path, method string
		auth               bool
		status             int
	}{
		{"home", home, "GET", true, 200},
		{"default", "", "GET", true, 200},
		{"empty directory", filepath.Join(home, "alpha"), "GET", true, 200},
		{"internal link", filepath.Join(home, "safe-link"), "GET", true, 200},
		{"outside", outside, "GET", true, 403},
		{"prefix sibling", sibling, "GET", true, 403},
		{"traversal", home + "/../home-other", "GET", true, 403},
		{"symlink escape", filepath.Join(home, "escape"), "GET", true, 403},
		{"symlink descendant", filepath.Join(home, "escape", "missing"), "GET", true, 403},
		{"relative", "alpha", "GET", true, 403},
		{"missing", filepath.Join(home, "missing"), "GET", true, 403},
		{"file", filepath.Join(home, "file.txt"), "GET", true, 403},
		{"broken", filepath.Join(home, "broken"), "GET", true, 403},
		{"nul", home + "/\x00", "GET", true, 403},
		{"method", home, "POST", true, 405},
		{"auth", home, "GET", false, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/fs/list?path="+url.QueryEscape(tc.path), nil)
			if tc.auth {
				req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			}
			rec := httptest.NewRecorder()
			srv.mux.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.status == 200 {
				var body struct {
					Path        string   `json:"path"`
					Directories []string `json:"directories"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				want := []string{}
				if tc.path == home || tc.path == "" {
					want = []string{"alpha", "safe-link", "zeta"}
				}
				if !reflect.DeepEqual(body.Directories, want) {
					t.Fatalf("directories=%v want=%v", body.Directories, want)
				}
			}
		})
	}
	// Resolve link/.. before checking child paths: cleaning it first would list
	// the wrong directory or drop valid children.
	nested := filepath.Join(home, "alpha", "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(nested, filepath.Join(home, "deep-link")); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/fs/list?path="+url.QueryEscape(home+"/deep-link/.."), nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var resolvedBody struct {
		Path        string   `json:"path"`
		Directories []string `json:"directories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resolvedBody); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || resolvedBody.Path != filepath.Join(home, "alpha") || !reflect.DeepEqual(resolvedBody.Directories, []string{"nested"}) {
		t.Fatalf("symlink parent: %d %s", rec.Code, rec.Body.String())
	}
	// HOME itself may legitimately be a symlink.
	alias := filepath.Join(root, "home-alias")
	if err := os.Symlink(home, alias); err != nil {
		t.Fatal(err)
	}
	if _, ok := resolveHomeDirectory(alias, filepath.Join(alias, "alpha")); !ok {
		t.Fatal("symlink home rejected")
	}
}
