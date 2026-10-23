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

// pointerTestServer returns a server whose user home is a temporary
// directory, plus the session token.
func pointerTestServer(t *testing.T) (*Server, string, *auth.SessionManager) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, token, sessions := testProjectControlServer(t)
	srv.cfg.Agent.WorkspaceMemoryPointer = true
	return srv, token, sessions
}

func createPointerProject(t *testing.T, srv *Server, token, repoPath string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/projects",
		strings.NewReader(`{"name":"指针项目","currentGoal":"写指针","repoPath":"`+repoPath+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create project = %d: %s", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		ActiveProjectID string `json:"activeProjectId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot.ActiveProjectID
}

// The default is detection only: a memory file that already exists in the
// workspace gets the pointer; the rest of the workspace stays untouched.
func TestProjectCreationInsertsIntoExistingMemoryFile(t *testing.T) {
	srv, token, sessions := pointerTestServer(t)
	defer sessions.Stop()

	workspace := t.TempDir()
	existing := "# 我们的约定\n\n不要提交密钥。\n"
	if err := os.WriteFile(filepath.Join(workspace, "CLAUDE.md"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}
	projectID := createPointerProject(t, srv, token, workspace)

	data, err := os.ReadFile(filepath.Join(workspace, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "不要提交密钥。") {
		t.Fatalf("existing instructions lost:\n%s", content)
	}
	if !strings.Contains(content, "roambench:memory:begin project="+projectID) {
		t.Fatalf("pointer missing:\n%s", content)
	}
	if !strings.Contains(content, "roambench-agent memory get --project "+projectID) {
		t.Fatalf("pointer misses the read command:\n%s", content)
	}

	// AGENTS.md was not there, so it must not have been created.
	if _, err := os.Stat(filepath.Join(workspace, "AGENTS.md")); err == nil {
		t.Fatal("AGENTS.md must not be created when it did not exist")
	}
}

// A workspace without memory files stays untouched unless creation is
// explicitly enabled.
func TestProjectCreationLeavesFreshWorkspaceUntouched(t *testing.T) {
	srv, token, sessions := pointerTestServer(t)
	defer sessions.Stop()

	workspace := t.TempDir()
	createPointerProject(t, srv, token, workspace)

	entries, err := os.ReadDir(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("workspace should stay untouched, got %v", entries)
	}
}

// With create_missing_memory_files, a registered project gets the memory
// files of its installed agents.
func TestProjectCreationCreatesMemoryFilesWhenEnabled(t *testing.T) {
	srv, token, sessions := pointerTestServer(t)
	defer sessions.Stop()
	srv.cfg.Agent.CreateMissingMemoryFiles = true

	workspace := t.TempDir()
	projectID := createPointerProject(t, srv, token, workspace)

	found := false
	for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
		data, err := os.ReadFile(filepath.Join(workspace, name))
		if err != nil {
			continue
		}
		if !strings.Contains(string(data), "project="+projectID) {
			t.Fatalf("%s has no pointer for the project:\n%s", name, data)
		}
		found = true
	}
	if !found {
		t.Fatal("no memory file was written into the workspace")
	}
}

func TestPointerEndpointIsIdempotentAndRemovable(t *testing.T) {
	srv, token, sessions := pointerTestServer(t)
	defer sessions.Stop()
	srv.cfg.Agent.CreateMissingMemoryFiles = true

	workspace := t.TempDir()
	projectID := createPointerProject(t, srv, token, workspace)
	path := filepath.Join(workspace, "CLAUDE.md")

	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Applying again changes nothing.
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/memory/pointers",
		strings.NewReader(`{"projectId":"`+projectID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("pointer rewritten on re-apply:\n%s\n---\n%s", first, second)
	}

	// Removing takes the pointer out again and keeps the rest.
	req = httptest.NewRequest(http.MethodPost, "/api/project-control/memory/pointers",
		strings.NewReader(`{"projectId":"`+projectID+`","remove":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove status=%d body=%s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "roambench:memory:begin") {
		t.Fatalf("pointer still present:\n%s", data)
	}
}

// The global pointer is opt-in and only touches files that already exist.
func TestGlobalPointerReachesExistingAgentMemory(t *testing.T) {
	srv, token, sessions := pointerTestServer(t)
	defer sessions.Stop()
	srv.cfg.Agent.GlobalMemoryPointer = true

	home := os.Getenv("HOME")
	global := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(global), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(global, []byte("@RTK.md\n"), 0644); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	createPointerProject(t, srv, token, workspace)

	data, err := os.ReadFile(global)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "@RTK.md") {
		t.Fatalf("existing global instructions lost:\n%s", content)
	}
	if !strings.Contains(content, "roambench:memory:begin scope=all") {
		t.Fatalf("generic pointer missing:\n%s", content)
	}
}

// A global memory file that does not exist is never created.
func TestGlobalPointerNeverCreatesFiles(t *testing.T) {
	srv, token, sessions := pointerTestServer(t)
	defer sessions.Stop()
	srv.cfg.Agent.GlobalMemoryPointer = true

	workspace := t.TempDir()
	createPointerProject(t, srv, token, workspace)

	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".claude", "CLAUDE.md")); err == nil {
		t.Fatal("global agent memory file must not be created")
	}
}

func TestWorkspacePointerCanBeDisabled(t *testing.T) {
	srv, token, sessions := pointerTestServer(t)
	defer sessions.Stop()
	srv.cfg.Agent.WorkspaceMemoryPointer = false
	srv.cfg.Agent.GlobalMemoryPointer = true

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "CLAUDE.md"), []byte("# keep\n"), 0644); err != nil {
		t.Fatal(err)
	}
	createPointerProject(t, srv, token, workspace)

	data, err := os.ReadFile(filepath.Join(workspace, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# keep\n" {
		t.Fatalf("workspace memory file was modified:\n%s", data)
	}
}

func TestPointerEndpointRequiresAuth(t *testing.T) {
	srv, _, sessions := pointerTestServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/memory/pointers",
		strings.NewReader(`{"projectId":"project-x"}`))
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}
}
