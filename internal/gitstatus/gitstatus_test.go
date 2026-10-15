package gitstatus

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// All fixtures run real git inside t.TempDir(). Every git invocation uses a
// sanitized env (GIT_CONFIG_GLOBAL=/dev/null) so host config and ownership
// checks cannot interfere with throwaway repos.

func gitEnv() []string {
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

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	return dir
}

func commitFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	gitRun(t, dir, "add", name)
	gitRun(t, dir, "commit", "-q", "-m", "add "+name)
	return gitRun(t, dir, "rev-parse", "HEAD")
}

// countingGitWrapper returns a git binary that forwards to the real git but
// appends one line to a log file per invocation.
func countingGitWrapper(t *testing.T) (bin string, logFile string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "git")
	logFile = filepath.Join(dir, "calls.log")
	script := fmt.Sprintf("#!/bin/sh\necho x >> %s\nexec %q \"$@\"\n", logFile, realGitPath(t))
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return bin, logFile
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(strings.TrimSpace(string(raw)), "\n") + 1
}

func TestFetch(t *testing.T) {
	cases := []struct {
		name   string
		build  func(t *testing.T) string
		verify func(t *testing.T, dir string, st Status)
	}{
		{
			name: "clean repo with upstream ahead and behind",
			build: func(t *testing.T) string {
				// Bare "origin" repo with an initial commit.
				base := initRepo(t)
				commitFile(t, base, "a.txt", "a\n")
				remote := filepath.Join(t.TempDir(), "remote.git")
				gitRun(t, base, "clone", "-q", "--bare", ".", remote)
				clone := filepath.Join(t.TempDir(), "clone")
				gitRun(t, t.TempDir(), "clone", "-q", remote, clone)
				gitRun(t, clone, "branch", "--set-upstream-to=origin/main")

				// Remote moves 1 commit beyond the clone's origin/main.
				drop := filepath.Join(t.TempDir(), "drop")
				gitRun(t, t.TempDir(), "clone", "-q", remote, drop)
				commitFile(t, drop, "remote.txt", "remote\n")
				gitRun(t, drop, "push", "-q", "origin", "HEAD:refs/heads/main")

				// Local clone moves 2 commits ahead of that ref.
				commitFile(t, clone, "b.txt", "b\n")
				commitFile(t, clone, "c.txt", "c\n")
				// Fetching realizes ahead=2/behind=1 against the new remote tip.
				gitRun(t, clone, "fetch", "-q", "origin")
				return clone
			},
			verify: func(t *testing.T, dir string, st Status) {
				if !st.Available {
					t.Fatalf("available=false, error=%q", st.Error)
				}
				if st.Branch != "main" {
					t.Errorf("branch=%q, want main", st.Branch)
				}
				if st.Upstream != "origin/main" {
					t.Errorf("upstream=%q, want origin/main", st.Upstream)
				}
				if st.Ahead != 2 || st.Behind != 1 {
					t.Errorf("ahead/behind=%d/%d, want 2/1", st.Ahead, st.Behind)
				}
				if st.Dirty || st.ChangedCount != 0 || st.UntrackedCount != 0 {
					t.Errorf("dirty=%v changed=%d untracked=%d, want all clean", st.Dirty, st.ChangedCount, st.UntrackedCount)
				}
				if len(st.Worktrees) != 1 || !st.Worktrees[0].IsCurrent {
					t.Fatalf("worktrees=%+v, want 1 current", st.Worktrees)
				}
				if st.Worktrees[0].HeadSHA != st.HeadSHA || st.HeadSHA == "" {
					t.Errorf("headSha=%q worktree head=%q", st.HeadSHA, st.Worktrees[0].HeadSHA)
				}
			},
		},
		{
			name: "detached HEAD",
			build: func(t *testing.T) string {
				dir := initRepo(t)
				first := commitFile(t, dir, "a.txt", "a\n")
				commitFile(t, dir, "b.txt", "b\n")
				gitRun(t, dir, "checkout", "-q", "--detach", first)
				return dir
			},
			verify: func(t *testing.T, dir string, st Status) {
				if !st.Available {
					t.Fatalf("available=false, error=%q", st.Error)
				}
				if st.Branch != "HEAD" {
					t.Errorf("branch=%q, want HEAD for detached", st.Branch)
				}
				if st.Upstream != "" {
					t.Errorf("upstream=%q, want empty when detached", st.Upstream)
				}
				if st.HeadSHA == "" {
					t.Error("headSha should still be resolved when detached")
				}
			},
		},
		{
			name: "dirty working tree counts changed and untracked",
			build: func(t *testing.T) string {
				dir := initRepo(t)
				commitFile(t, dir, "a.txt", "a\n")
				commitFile(t, dir, "b.txt", "b\n")
				// one staged modification
				if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a2\n"), 0644); err != nil {
					t.Fatal(err)
				}
				gitRun(t, dir, "add", "a.txt")
				// one unstaged modification
				if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b2\n"), 0644); err != nil {
					t.Fatal(err)
				}
				// two untracked files
				if err := os.WriteFile(filepath.Join(dir, "new1.txt"), []byte("n\n"), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "new2.txt"), []byte("n\n"), 0644); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			verify: func(t *testing.T, dir string, st Status) {
				if !st.Available {
					t.Fatalf("available=false, error=%q", st.Error)
				}
				if st.ChangedCount != 2 {
					t.Errorf("changedCount=%d, want 2", st.ChangedCount)
				}
				if st.UntrackedCount != 2 {
					t.Errorf("untrackedCount=%d, want 2", st.UntrackedCount)
				}
				if !st.Dirty {
					t.Error("dirty=false, want true")
				}
			},
		},
		{
			name: "two linked worktrees, queried from the linked one",
			build: func(t *testing.T) string {
				dir := initRepo(t)
				commitFile(t, dir, "a.txt", "a\n")
				link := filepath.Join(t.TempDir(), "wt-feature")
				gitRun(t, dir, "worktree", "add", "-b", "feature", link, "HEAD")
				return link
			},
			verify: func(t *testing.T, dir string, st Status) {
				if !st.Available {
					t.Fatalf("available=false, error=%q", st.Error)
				}
				if st.Branch != "feature" {
					t.Errorf("branch=%q, want feature", st.Branch)
				}
				if len(st.Worktrees) != 2 {
					t.Fatalf("worktrees=%d, want 2", len(st.Worktrees))
				}
				currentCount := 0
				for _, wt := range st.Worktrees {
					if wt.IsCurrent {
						currentCount++
					}
				}
				if currentCount != 1 {
					t.Errorf("current worktrees=%d, want exactly 1", currentCount)
				}
			},
		},
		{
			name: "no upstream configured",
			build: func(t *testing.T) string {
				dir := initRepo(t)
				commitFile(t, dir, "a.txt", "a\n")
				return dir
			},
			verify: func(t *testing.T, dir string, st Status) {
				if !st.Available {
					t.Fatalf("available=false, error=%q", st.Error)
				}
				if st.Upstream != "" {
					t.Errorf("upstream=%q, want empty", st.Upstream)
				}
				if st.Ahead != 0 || st.Behind != 0 {
					t.Errorf("ahead/behind=%d/%d, want 0/0 without upstream", st.Ahead, st.Behind)
				}
				if st.Branch != "main" {
					t.Errorf("branch=%q, want main", st.Branch)
				}
			},
		},
		{
			name:  "not a git repository",
			build: func(t *testing.T) string { return t.TempDir() },
			verify: func(t *testing.T, dir string, st Status) {
				if st.Available {
					t.Error("available=true, want false for non-repo")
				}
				if st.Error == "" {
					t.Error("error should explain why the repo is unavailable")
				}
				if st.Branch != "" || st.HeadSHA != "" || st.Upstream != "" || st.Dirty {
					t.Errorf("fields should stay zero-valued: %+v", st)
				}
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.build(t)
			st := Fetch(dir)
			tc.verify(t, dir, st)
			// JSON round-trip must keep the K2 shape.
			raw, err := json.Marshal(st)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var back map[string]interface{}
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			for _, key := range []string{"repoPath", "available", "branch", "headSha", "upstream", "ahead", "behind", "dirty", "changedCount", "untrackedCount", "worktrees"} {
				if _, ok := back[key]; !ok {
					t.Errorf("JSON payload missing key %q: %s", key, raw)
				}
			}
		})
	}
}

func TestFetchUnbornRepoHasNoCommits(t *testing.T) {
	dir := initRepo(t) // init only, zero commits
	st := Fetch(dir)
	if !st.Available {
		t.Fatalf("available=false, error=%q (unborn repo should still be a repo)", st.Error)
	}
	if st.Branch != "main" {
		t.Errorf("branch=%q, want main on unborn branch", st.Branch)
	}
	if st.HeadSHA != "" {
		t.Errorf("headSha=%q, want empty with no commits", st.HeadSHA)
	}
}

func TestCacheHitsWithinTTL(t *testing.T) {
	dir := initRepo(t)
	bin, logFile := countingGitWrapper(t)
	c := NewCache()
	c.gitBin = bin

	first := c.Fetch(dir)
	if !first.Available {
		t.Fatalf("first fetch unavailable: %s", first.Error)
	}
	n1 := countLines(t, logFile)
	if n1 == 0 {
		t.Fatal("expected at least one git invocation on first fetch")
	}
	second := c.Fetch(dir)
	if n2 := countLines(t, logFile); n2 != n1 {
		t.Errorf("cache miss within TTL: git invocations %d -> %d", n1, n2)
	}
	if second.HeadSHA != first.HeadSHA || second.Branch != first.Branch {
		t.Error("cached status should equal the first fetch")
	}
}

func TestCacheTTLExpiryRefetches(t *testing.T) {
	dir := initRepo(t)
	commitFile(t, dir, "a.txt", "a\n")
	c := NewCache()
	c.ttl = 20 * time.Millisecond
	first := c.Fetch(dir)
	// Uncommitted change: a new untracked file.
	if err := os.WriteFile(filepath.Join(dir, "late.txt"), []byte("late\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Still cached: unchanged within TTL.
	if got := c.Fetch(dir); got.ChangedCount != 0 || got.UntrackedCount != 0 {
		t.Errorf("changed=%d untracked=%d within TTL, want cached 0/0", got.ChangedCount, got.UntrackedCount)
	}
	time.Sleep(40 * time.Millisecond)
	got := c.Fetch(dir)
	if got.ChangedCount != 0 || got.UntrackedCount != 1 {
		t.Errorf("changed=%d untracked=%d after TTL, want refetched 0/1", got.ChangedCount, got.UntrackedCount)
	}
	if got.HeadSHA != first.HeadSHA || got.HeadSHA == "" {
		t.Errorf("headSha=%q, want stable resolved HEAD %q", got.HeadSHA, first.HeadSHA)
	}
}

func TestCacheConcurrent(t *testing.T) {
	dir := initRepo(t)
	c := NewCache()
	done := make(chan Status, 8)
	for i := 0; i < 8; i++ {
		go func() { done <- c.Fetch(dir) }()
	}
	for i := 0; i < 8; i++ {
		st := <-done
		if !st.Available {
			t.Errorf("concurrent fetch %d unavailable: %s", i, st.Error)
		}
	}
}

func TestSlowGitDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	// Fake git that sleeps far past the timeout before it would answer.
	bin := filepath.Join(t.TempDir(), "git")
	script := fmt.Sprintf("#!/bin/sh\nsleep 30\nexec %q \"$@\"\n", realGitPath(t))
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	st := fetchWithTimeout(bin, dir, 300*time.Millisecond)
	elapsed := time.Since(start)
	if st.Available {
		t.Error("slow git should yield available=false")
	}
	if st.Error == "" {
		t.Error("error should explain the timeout")
	}
	if elapsed > 2*time.Second {
		t.Errorf("fetch took %s, timeout should cap it well below", elapsed)
	}
}

func realGitPath(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("host git not found: %v", err)
	}
	return path
}

func TestSanitizedEnvKeepsGitStateless(t *testing.T) {
	env := sanitizedGitEnv([]string{
		"GIT_DIR=/tmp/somewhere",
		"GIT_WORK_TREE=/tmp/other",
		"GIT_CONFIG_COUNT=1",
		"PATH=/usr/bin",
		"HOME=/home/test",
	})
	joined := strings.Join(env, "\n")
	for _, banned := range []string{"GIT_DIR=", "GIT_WORK_TREE=", "GIT_CONFIG_COUNT="} {
		if strings.Contains(joined, banned) {
			t.Errorf("sanitized env still contains %s: %s", banned, joined)
		}
	}
	for _, required := range []string{"GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/dev/null"} {
		if !strings.Contains(joined, required) {
			t.Errorf("sanitized env missing %s: %s", required, joined)
		}
	}
}
