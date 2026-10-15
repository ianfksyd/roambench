// Package gitstatus provides read-only git repository introspection for the
// project control board (K2, WS-20261015-001).
//
// Safety rules enforced here:
//   - only read-only git commands are ever executed (rev-parse, symbolic-ref,
//     rev-list, status, worktree list). `git status` is additionally wrapped
//     with --no-optional-locks / GIT_OPTIONAL_LOCKS=0 so even the opportunistic
//     index refresh can never write.
//   - every command runs with a 3s context timeout via exec.CommandContext, so
//     a hung git cannot block a board response.
//   - the environment is sanitized (GIT_CONFIG_GLOBAL/SYSTEM point at /dev/null)
//     so user config cannot inject hooks, fsmonitor daemons or alternate
//     GIT_DIR redirections into a read.
//
// Results are cached per repoPath for 10s (CacheTTL) behind a mutex, which is
// what the board handler uses.
package gitstatus

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// CommandTimeout caps a single git invocation.
	CommandTimeout = 3 * time.Second
	// CacheTTL is how long a Fetch result stays fresh per repoPath.
	CacheTTL = 10 * time.Second
)

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	HeadSHA   string `json:"headSha"`
	IsCurrent bool   `json:"isCurrent"`
}

// Status is the K2 git block payload for one repository.
type Status struct {
	RepoPath       string     `json:"repoPath"`
	Available      bool       `json:"available"`
	Error          string     `json:"error,omitempty"`
	Branch         string     `json:"branch"`
	HeadSHA        string     `json:"headSha"`
	Upstream       string     `json:"upstream"`
	Ahead          int        `json:"ahead"`
	Behind         int        `json:"behind"`
	Dirty          bool       `json:"dirty"`
	ChangedCount   int        `json:"changedCount"`
	UntrackedCount int        `json:"untrackedCount"`
	Worktrees      []Worktree `json:"worktrees"`
}

// Fetch runs the read-only git queries against repoPath using the `git`
// binary from PATH.
func Fetch(repoPath string) Status {
	return FetchWith("git", repoPath)
}

// FetchWith is Fetch with an explicit git binary (tests install fake `git`
// wrappers through it).
func FetchWith(gitBin, repoPath string) Status {
	return fetchWithTimeout(gitBin, repoPath, CommandTimeout)
}

func fetchWithTimeout(gitBin, repoPath string, timeout time.Duration) Status {
	st := Status{
		RepoPath:  repoPath,
		Worktrees: []Worktree{},
	}
	run := func(args ...string) (string, error) {
		return runGit(gitBin, repoPath, timeout, args...)
	}

	toplevel, err := run("rev-parse", "--show-toplevel")
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.Available = true
	toplevel = filepath.Clean(toplevel)

	// Branch: symbolic-ref works even on an unborn branch; when HEAD is
	// detached it fails, and rev-parse --abbrev-ref reports "HEAD".
	if branch, err := run("symbolic-ref", "--quiet", "--short", "HEAD"); err == nil && branch != "" {
		st.Branch = branch
	} else if branch, err := run("rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		st.Branch = branch
	}

	// HEAD sha. Failure here means an unborn branch (no commits yet); the
	// repository itself is still available.
	if sha, err := run("rev-parse", "--verify", "HEAD"); err == nil {
		st.HeadSHA = sha
	}

	// Upstream tracking branch; empty when none is configured (or detached).
	upstream, err := run("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err == nil {
		st.Upstream = upstream
		if counts, err := run("rev-list", "--left-right", "--count", "HEAD...@{upstream}"); err == nil {
			st.Ahead, st.Behind = parseLeftRightCounts(counts)
		}
	}

	// Working tree state. `??` entries are untracked; every other porcelain
	// line is a tracked change (staged and/or unstaged).
	if porcelain, err := run("status", "--porcelain", "--untracked-files=normal"); err == nil {
		for _, line := range strings.Split(porcelain, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) == "" {
				continue
			}
			if strings.HasPrefix(line, "??") {
				st.UntrackedCount++
			} else {
				st.ChangedCount++
			}
		}
	}
	st.Dirty = st.ChangedCount > 0 || st.UntrackedCount > 0

	// Worktrees; the one matching the queried toplevel is marked current.
	if out, err := run("worktree", "list", "--porcelain"); err == nil {
		st.Worktrees = parseWorktrees(out, toplevel)
	}
	return st
}

// runGit executes one read-only git command with a hard timeout and a
// sanitized, non-writing environment.
func runGit(gitBin, dir string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	argv := append([]string{"--no-optional-locks"}, args...)
	cmd := exec.CommandContext(ctx, gitBin, argv...)
	cmd.Dir = dir
	cmd.Env = sanitizedGitEnv(os.Environ())
	// Run the git process in its own group and kill the whole group on
	// timeout: git can spawn helper children (fsmonitor, hooks-free here, but
	// still) and a bare process kill would leave a child holding the stdout
	// pipe, which blocks cmd.Wait forever. WaitDelay caps that too.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 250 * time.Millisecond
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("git %s timed out after %s", args[0], timeout)
	}
	if err != nil {
		if msg := compactGitStderr(stderr.String()); msg != "" {
			return "", fmt.Errorf("git %s: %s", args[0], msg)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

// sanitizedGitEnv strips inherited GIT_* variables (which could redirect the
// repo or inject config) and pins a strictly read-only, deterministic env.
func sanitizedGitEnv(environ []string) []string {
	env := make([]string, 0, len(environ)+4)
	for _, kv := range environ {
		if strings.HasPrefix(kv, "GIT_") || strings.HasPrefix(kv, "LC_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"LC_ALL=C",
	)
}

func compactGitStderr(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 200 {
			line = line[:200]
		}
		return line
	}
	return ""
}

func parseLeftRightCounts(out string) (ahead, behind int) {
	parts := strings.Fields(out)
	if len(parts) != 2 {
		return 0, 0
	}
	fmt.Sscanf(parts[0], "%d", &ahead)
	fmt.Sscanf(parts[1], "%d", &behind)
	return ahead, behind
}

func parseWorktrees(out, toplevel string) []Worktree {
	worktrees := []Worktree{}
	var current *Worktree
	finalize := func() {
		if current != nil {
			current.IsCurrent = sameDir(current.Path, toplevel)
			worktrees = append(worktrees, *current)
			current = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "worktree "):
			finalize()
			current = &Worktree{Path: strings.TrimSpace(strings.TrimPrefix(line, "worktree "))}
		case current == nil:
			continue
		case strings.HasPrefix(line, "HEAD "):
			current.HeadSHA = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}
	finalize()
	return worktrees
}

// sameDir compares two paths after resolving symlinks, tolerating paths that
// no longer exist.
func sameDir(a, b string) bool {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	if a == b {
		return true
	}
	if real, err := filepath.EvalSymlinks(a); err == nil && real == b {
		return true
	}
	if real, err := filepath.EvalSymlinks(b); err == nil && real == a {
		return true
	}
	return false
}

type cacheEntry struct {
	status    Status
	fetchedAt time.Time
}

// Cache memoizes Fetch results per repoPath for CacheTTL. It is safe for
// concurrent use.
type Cache struct {
	mu      sync.Mutex
	ttl     time.Duration
	gitBin  string
	entries map[string]cacheEntry
}

func NewCache() *Cache {
	return &Cache{
		ttl:     CacheTTL,
		gitBin:  "git",
		entries: map[string]cacheEntry{},
	}
}

// GitBin overrides the git binary used by this cache (test hook for fake git
// wrappers, e.g. a sleeping git for timeout tests).
func (c *Cache) SetGitBin(bin string) {
	c.mu.Lock()
	c.gitBin = bin
	c.mu.Unlock()
}

// GitBin returns the currently configured git binary.
func (c *Cache) GitBin() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gitBin
}

// Fetch returns the cached Status for repoPath if it is fresher than the TTL,
// otherwise runs a fresh read-only probe and caches it.
func (c *Cache) Fetch(repoPath string) Status {
	now := time.Now()
	c.mu.Lock()
	if entry, ok := c.entries[repoPath]; ok && now.Sub(entry.fetchedAt) < c.ttl {
		cached := entry.status
		c.mu.Unlock()
		return cached
	}
	c.mu.Unlock()

	status := fetchWithTimeout(c.gitBin, repoPath, CommandTimeout)

	c.mu.Lock()
	c.entries[repoPath] = cacheEntry{status: status, fetchedAt: time.Now()}
	c.mu.Unlock()
	return status
}

// Invalidate drops the cached entry for repoPath (useful when the board knows
// the repository just changed).
func (c *Cache) Invalidate(repoPath string) {
	c.mu.Lock()
	delete(c.entries, repoPath)
	c.mu.Unlock()
}
