// Package testutil provides isolation for tests that use external processes.
package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// RunWithIsolatedTmux runs a test package against a private tmux server. It must
// be called from TestMain before any Manager is constructed. TMUX must be cleared
// as well as TMUX_TMPDIR: an inherited TMUX socket takes precedence over the
// default socket directory when tests are launched inside a real terminal.
func RunWithIsolatedTmux(run func() int) int {
	// Keep Unix socket paths short, including when TMPDIR points into a long path.
	dir, err := os.MkdirTemp("/tmp", "rb-test-tmux-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "isolate tmux:", err)
		return 1
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "tmux-"+strconv.Itoa(os.Getuid()), "default")
	defer func() {
		// Always name our socket explicitly; never clean up the user's default server.
		_ = exec.Command("tmux", "-S", socket, "kill-server").Run()
	}()
	for key, value := range map[string]string{"TMUX": "", "TMUX_TMPDIR": dir} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintln(os.Stderr, "isolate tmux:", err)
			return 1
		}
	}
	return run()
}
