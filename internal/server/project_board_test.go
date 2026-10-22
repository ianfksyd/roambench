package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/gitstatus"
)

// boardTestServer is a board-specific server harness: it also resets the
// process-wide git cache between tests so entries never leak.
func boardTestServer(t *testing.T) (*Server, string, *auth.SessionManager) {
	t.Helper()
	boardGitCache = gitstatus.NewCache()
	t.Cleanup(func() {
		boardGitCache = gitstatus.NewCache()
	})
	return testProjectControlServer(t)
}

func boardGet(t *testing.T, srv *Server, token string) (*httptest.ResponseRecorder, boardResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/project-control/board", nil)
	req = req.WithContext(context.WithValue(req.Context(), usernameKey, "ian"))
	// Invoke the real handler directly: the 501 stub is still wired into the
	// mux until the integrator swaps it; auth wrapping is covered by the C1
	// route tests.
	rec := httptest.NewRecorder()
	srv.serveProjectControlBoard(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET board = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp boardResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode board response: %v", err)
	}
	return rec, resp
}

func boardGitForProject(t *testing.T, resp boardResponse, projectID string) gitstatus.Status {
	t.Helper()
	for _, p := range resp.Projects {
		if p.ProjectID == projectID {
			return p.Git
		}
	}
	t.Fatalf("project %q not on board", projectID)
	return gitstatus.Status{}
}

func boardCreateProjectWithRepo(t *testing.T, srv *Server, token, name, repoPath string) string {
	t.Helper()
	body := `{"name":"` + name + `","key":"` + strings.ToLower(name) + `"}`
	if repoPath != "" {
		body = `{"name":"` + name + `","key":"` + strings.ToLower(name) + `","repoPath":"` + repoPath + `"}`
	}
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("create project %s = %d: %s", name, rec.Code, rec.Body.String())
	}
	var snapshot projectControlSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode project create: %v", err)
	}
	for _, p := range snapshot.Projects {
		if p.Name == name {
			return p.ID
		}
	}
	t.Fatalf("project %s missing after create", name)
	return ""
}

func boardGitEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	)
}

func boardGitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = boardGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestBoardReturnsGitStatusPerProject(t *testing.T) {
	srv, token, sessions := boardTestServer(t)
	defer sessions.Stop()

	repo := boardTempRepoUnderHome(t)
	boardGitRun(t, repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	boardGitRun(t, repo, "add", "a.txt")
	boardGitRun(t, repo, "commit", "-q", "-m", "init")
	// No upstream configured: upstream stays empty. Just verify the fields
	// that exist without one.
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("u\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_ = boardCreateProjectWithRepo(t, srv, token, "RepoProj", repo)
	_, resp := boardGet(t, srv, token)

	var project boardProject
	for _, p := range resp.Projects {
		if p.Name == "RepoProj" {
			project = p
		}
	}
	git := project.Git
	if !git.Available {
		t.Fatalf("git.available=false, error=%q", git.Error)
	}
	if git.RepoPath != filepath.Clean(repo) {
		t.Errorf("repoPath=%q, want %q", git.RepoPath, repo)
	}
	if git.Branch != "main" {
		t.Errorf("branch=%q, want main", git.Branch)
	}
	if git.HeadSHA == "" {
		t.Error("headSha should be resolved")
	}
	if git.UntrackedCount != 1 {
		t.Errorf("untrackedCount=%d, want 1", git.UntrackedCount)
	}
	if !git.Dirty {
		t.Error("dirty=false, want true with untracked file")
	}
	if resp.GeneratedAt == "" || len(resp.Projects) == 0 {
		t.Errorf("board envelope incomplete: %+v", resp)
	}
}

func TestBoardFallsBackToRuntimeRootWhenRepoPathEmpty(t *testing.T) {
	srv, token, sessions := boardTestServer(t)
	defer sessions.Stop()

	// Seed default state then probe: the default project has empty repoPath,
	// so the board must use runtimeRootDir (the roambench repo itself).
	_, resp := boardGet(t, srv, token)
	for _, p := range resp.Projects {
		if p.Git.RepoPath == "" {
			t.Fatalf("project %q has empty git.repoPath", p.ProjectID)
		}
	}
}

func TestBoardNonRepoReportsAvailableFalse(t *testing.T) {
	if _, err := os.UserHomeDir(); err != nil {
		t.Skip("no home dir")
	}
	srv, token, sessions := boardTestServer(t)
	defer sessions.Stop()

	// A non-repo directory inside the home directory (validation requires
	// confinement under home).
	notRepo, err := os.MkdirTemp(homeDirForTest(t), "board-notrepo-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(notRepo) })

	projectID := boardCreateProjectWithRepo(t, srv, token, "NotRepoProj", notRepo)
	_, resp := boardGet(t, srv, token)
	git := boardGitForProject(t, resp, projectID)
	if git.Available {
		t.Error("git.available=true, want false for non-repo directory")
	}
	if git.Error == "" {
		t.Error("git.error should explain why the repo is unavailable")
	}
}

func homeDirForTest(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	return home
}

// boardTempRepoUnderHome creates a throwaway directory under the user's home
// (repoPath validation requires home confinement, and /tmp is outside it).
func boardTempRepoUnderHome(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(homeDirForTest(t), "roambench-board-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestBoardRepoPathOutsideHomeIsAccepted(t *testing.T) {
	srv, token, sessions := boardTestServer(t)
	defer sessions.Stop()

	// A project may live anywhere the service account can reach: /etc passes
	// absolute+exists, so the board must accept it and report the git probe
	// as unavailable rather than failing the request.
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/projects",
		strings.NewReader(`{"name":"EtcProj","key":"etcproj","repoPath":"/etc"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("creating a project with repoPath=/etc = %d, want 200/201: %s", rec.Code, rec.Body.String())
	}
}

func TestBoardSymlinkToRepoIsResolved(t *testing.T) {
	home := homeDirForTest(t)
	outside := t.TempDir()
	boardGitRun(t, outside, "init", "-q", "-b", "main")

	// A symlink inside home pointing at the repo.
	link := filepath.Join(home, ".roambench-board-escape-test")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(link) })

	srv, _, sessions := boardTestServer(t)
	defer sessions.Stop()

	// The symlink is resolved to the real repository, so the board reports
	// the git status of that repository.
	status := srv.boardGitStatus(link)
	if !status.Available {
		t.Errorf("symlinked repo reported available=false (repoPath=%q error=%q)", status.RepoPath, status.Error)
	}
	if status.RepoPath != outside {
		t.Errorf("repoPath = %q, want the resolved target %q", status.RepoPath, outside)
	}
}

func TestBoardGitErrorDoesNotFailRequest(t *testing.T) {
	srv, token, sessions := boardTestServer(t)
	defer sessions.Stop()

	// Even when git is missing/broken for a project, the response stays 200.
	srv.projectControl.runtimeRootDir = "/nonexistent-board-test-xyz"
	_, resp := boardGet(t, srv, token)
	if len(resp.Projects) == 0 {
		t.Fatal("expected at least one project on the board")
	}
	for _, p := range resp.Projects {
		if p.Git.Available {
			t.Errorf("project %q reported available=true for a nonexistent root", p.ProjectID)
		}
	}
}

func TestBoardCurrentTaskAndLastActivity(t *testing.T) {
	srv, token, sessions := boardTestServer(t)
	defer sessions.Stop()

	// Default state has seeded tasks; the board must pick the first
	// non-terminal task and produce a non-empty activity summary.
	_, resp := boardGet(t, srv, token)
	found := false
	for _, p := range resp.Projects {
		if p.CurrentTask != nil {
			found = true
			if p.CurrentTask.TaskID == "" || p.CurrentTask.Title == "" || p.CurrentTask.State == "" {
				t.Errorf("currentTask incomplete: %+v", p.CurrentTask)
			}
		}
		if p.LastActivityAt == "" || p.LastActivitySummary == "" {
			t.Errorf("project %q missing lastActivity (at=%q summary=%q)", p.ProjectID, p.LastActivityAt, p.LastActivitySummary)
		}
	}
	if !found {
		t.Error("expected at least one project with a current task in seeded state")
	}
}

func TestBoardGitCacheSharedAcrossRequests(t *testing.T) {
	srv, token, sessions := boardTestServer(t)
	defer sessions.Stop()

	repo := boardTempRepoUnderHome(t)
	boardGitRun(t, repo, "init", "-q", "-b", "main")
	boardGitRun(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	projectID := boardCreateProjectWithRepo(t, srv, token, "CacheProj", repo)

	_, resp := boardGet(t, srv, token)
	first := boardGitForProject(t, resp, projectID).HeadSHA

	// Mutate the repo: within the 10s TTL the board must keep serving the
	// cached status.
	boardGitRun(t, repo, "commit", "-q", "--allow-empty", "-m", "second")
	_, resp = boardGet(t, srv, token)
	cached := boardGitForProject(t, resp, projectID)
	if cached.HeadSHA != first {
		t.Errorf("board refetched within TTL: %q -> %q", first, cached.HeadSHA)
	}

	// After the TTL the fresh sha must appear.
	boardGitCache = gitstatus.NewCache()
	_, resp = boardGet(t, srv, token)
	fresh := boardGitForProject(t, resp, projectID)
	if fresh.HeadSHA == first {
		t.Error("board still serves stale headSha after cache reset")
	}
}

func TestBoardGitTimeoutDoesNotBlockResponse(t *testing.T) {
	srv, token, sessions := boardTestServer(t)
	defer sessions.Stop()

	// Swap the process-wide cache for one whose git binary sleeps forever;
	// the board must still return 200 promptly with available=false.
	sleepBin := filepath.Join(t.TempDir(), "git")
	script := "#!/bin/sh\nsleep 30\n"
	if err := os.WriteFile(sleepBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	slow := gitstatus.NewCache()
	slow.SetGitBin(sleepBin)
	boardGitCache = slow
	t.Cleanup(func() { boardGitCache = gitstatus.NewCache() })

	// repoPath must live under the user's home (K1 confinement).
	repo := boardTempRepoUnderHome(t)
	boardGitRun(t, repo, "init", "-q", "-b", "main")
	projectID := boardCreateProjectWithRepo(t, srv, token, "SlowProj", repo)

	start := time.Now()
	done := make(chan boardResponse, 1)
	go func() {
		_, resp := boardGet(t, srv, token)
		done <- resp
	}()
	select {
	case resp := <-done:
		git := boardGitForProject(t, resp, projectID)
		if git.Available {
			t.Error("slow git should yield available=false")
		}
		if git.Error == "" {
			t.Error("error should mention the git timeout")
		}
		if elapsed := time.Since(start); elapsed > 8*time.Second {
			t.Errorf("board took %s with a sleeping git; per-command timeout should cap it near 3s", elapsed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("board request blocked by slow git for >10s")
	}
}
