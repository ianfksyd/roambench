package terminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/config"
)

func TestDisabledManagerDoesNotInvokeTmuxDuringStartup(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls")
	// Any tmux invocation, including startup scans and global options, is a bug.
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$RB_TMUX_PROBE_LOG\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RB_TMUX_PROBE_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	mgr := newTestManager(t, &config.TerminalConfig{Shell: "/bin/sh", IdleTimeout: "1h"})
	if mgr.HasTmux() {
		t.Fatal("unit test manager enabled tmux")
	}
	mgr.Stop()
	if calls, err := os.ReadFile(logPath); !os.IsNotExist(err) {
		t.Fatalf("disabled manager invoked tmux: %q (read error: %v)", calls, err)
	}
}

// Simulate running go test from a real tmux terminal, using only a disposable
// sentinel server. Even an expired lt- session on that server must survive.
func TestManagerInheritedTmuxSessionSurvives(t *testing.T) {
	if os.Getenv("RB_TMUX_ISOLATION_CHILD") == "1" {
		if os.Getenv("TMUX") != "" {
			t.Fatal("TestMain retained an inherited tmux connection")
		}
		mgr := NewManager(&config.TerminalConfig{
			Shell: "/bin/sh", IdleTimeout: "1ns", PersistDir: t.TempDir(),
		})
		defer mgr.Stop()
		if sessions := mgr.listTmuxSessions(); len(sessions) != 0 {
			t.Fatalf("startup accessed external tmux sessions: %+v", sessions)
		}
		return
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "rb-sentinel-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "socket")
	runTmux := func(args ...string) ([]byte, error) {
		return exec.Command("tmux", append([]string{"-S", socket, "-f", "/dev/null"}, args...)...).CombinedOutput()
	}
	defer runTmux("kill-server")
	const sentinel = "lt-isolation-sentinel"
	if out, err := runTmux("new-session", "-d", "-s", sentinel, "sleep 120"); err != nil {
		t.Fatalf("start sentinel: %v (%s)", err, out)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestManagerInheritedTmuxSessionSurvives$", "-test.count=1")
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "TMUX=") && !strings.HasPrefix(env, "TMUX_TMPDIR=") && !strings.HasPrefix(env, "RB_TMUX_ISOLATION_CHILD=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "TMUX="+socket+",0,0", "TMUX_TMPDIR="+dir, "RB_TMUX_ISOLATION_CHILD=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("nested test: %v\n%s", err, out)
	}
	if out, err := runTmux("has-session", "-t", sentinel); err != nil {
		t.Fatalf("test startup destroyed external session: %v (%s)", err, out)
	}
}
