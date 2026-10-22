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

// Browsing is not confined to the home directory: any absolute path may be
// listed. The roots returned alongside are quick-access entries only.
func TestFSListListsAnyAbsolutePath(t *testing.T) {
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
		{"outside home", outside, "GET", true, 200},
		{"prefix sibling", sibling, "GET", true, 200},
		{"traversal", home + "/../home-other", "GET", true, 200},
		{"symlink escape", filepath.Join(home, "escape"), "GET", true, 200},
		{"relative", "alpha", "GET", true, 200},
		{"missing", filepath.Join(home, "missing"), "GET", true, 404},
		{"file", filepath.Join(home, "file.txt"), "GET", true, 400},
		{"broken", filepath.Join(home, "broken"), "GET", true, 404},
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
				switch tc.path {
				case home, "":
					// "escape" is a symlink to a directory outside the home
					// tree; linked directories stay browsable.
					want = []string{"alpha", "escape", "safe-link", "zeta"}
				case "alpha":
					want = []string{}
				}
				if !reflect.DeepEqual(body.Directories, want) {
					t.Fatalf("directories=%v want=%v", body.Directories, want)
				}
			}
		})
	}
	// Resolve link/.. before listing children: cleaning it first would list
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
}

// The response carries the browse roots so the picker can offer them as
// shortcuts.
func TestFSListReturnsRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	defer srv.Shutdown(context.Background())

	req := httptest.NewRequest(http.MethodGet, "/api/fs/list?path="+url.QueryEscape(home), nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Roots []struct {
			Path   string `json:"path"`
			Label  string `json:"label"`
			Source string `json:"source"`
		} `json:"roots"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Roots) == 0 {
		t.Fatal("expected at least one browse root")
	}
	found := false
	for _, entry := range body.Roots {
		if entry.Path == "/" && entry.Source == "instance" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the default instance root /, got %+v", body.Roots)
	}
}
