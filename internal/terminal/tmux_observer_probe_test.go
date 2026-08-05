//go:build phase0_probe

package terminal

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolatedTmuxServer(t *testing.T, suffix string) (socketName, target string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is required for the phase 0 observer probe")
	}
	socketName = fmt.Sprintf("roambench-phase0-%s-%d", suffix, time.Now().UnixNano())
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socketName, "kill-server").Run()
	})
	if output, err := exec.Command("tmux", "-L", socketName, "new-session", "-d", "-s", "probe", "-x", "80", "-y", "24").CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	return socketName, "probe:0.0"
}

func sendProbeOutput(t *testing.T, socketName, target, marker string) {
	t.Helper()
	command := fmt.Sprintf("printf '\\033]9;%s\\007'", marker)
	if output, err := exec.Command("tmux", "-L", socketName, "send-keys", "-l", "-t", target, command).CombinedOutput(); err != nil {
		t.Fatalf("tmux send-keys literal: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("tmux", "-L", socketName, "send-keys", "-t", target, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("tmux send-keys Enter: %v (%s)", err, strings.TrimSpace(string(output)))
	}
}

func tmuxPaneSize(t *testing.T, socketName, target string) string {
	t.Helper()
	output, err := exec.Command("tmux", "-L", socketName, "display-message", "-p", "-t", target, "#{pane_width}x#{pane_height}").CombinedOutput()
	if err != nil {
		t.Fatalf("tmux display pane size: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}

func waitForFileContains(t *testing.T, path, marker string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		payload, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(payload), marker) {
			return
		}
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read probe output: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("probe output %s did not contain %q within %s", path, marker, timeout)
}

// This characterizes the tmux behavior RoamBench relies on if pipe-pane is
// selected: -O starts piping without toggling an existing pipe off, and pane
// output is copied without requiring an attached terminal client.
func TestPhase0PipePaneCopiesOutputWithoutAttach(t *testing.T) {
	socketName, target := isolatedTmuxServer(t, "pipe")
	outputPath := filepath.Join(t.TempDir(), "pane-output.log")
	pipeCommand := fmt.Sprintf("cat >> %q", outputPath)
	if output, err := exec.Command("tmux", "-L", socketName, "pipe-pane", "-O", "-t", target, pipeCommand).CombinedOutput(); err != nil {
		t.Fatalf("tmux pipe-pane: %v (%s)", err, strings.TrimSpace(string(output)))
	}

	marker := "phase0-pipe-pane-marker"
	sendProbeOutput(t, socketName, target, marker)
	waitForFileContains(t, outputPath, marker, 2*time.Second)
}

// This characterizes the tmux behavior RoamBench relies on if control mode is
// selected: one read-only observer client receives %output records while the
// pane remains independently attachable by normal clients.
func TestPhase0ControlModeStreamsOutputWithoutInteractivePTY(t *testing.T) {
	socketName, target := isolatedTmuxServer(t, "control")
	if output, err := exec.Command("tmux", "-L", socketName, "resize-window", "-t", "probe", "-x", "123", "-y", "37").CombinedOutput(); err != nil {
		t.Fatalf("tmux resize-window: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	sizeBeforeAttach := tmuxPaneSize(t, socketName, target)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	observer := exec.CommandContext(ctx, "tmux", "-L", socketName, "-C", "attach-session", "-t", "probe")
	stdout, err := observer.StdoutPipe()
	if err != nil {
		t.Fatalf("control mode stdout pipe: %v", err)
	}
	stdin, err := observer.StdinPipe()
	if err != nil {
		t.Fatalf("control mode stdin pipe: %v", err)
	}
	if err := observer.Start(); err != nil {
		t.Fatalf("start control mode observer: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		cancel()
		_ = observer.Wait()
	}()

	lines := make(chan string, 64)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait until the control client has attached before producing output.
	ready := time.NewTimer(time.Second)
	defer ready.Stop()
	for {
		select {
		case line := <-lines:
			if strings.HasPrefix(line, "%session-changed") || strings.HasPrefix(line, "%window-add") {
				goto attached
			}
		case <-ready.C:
			t.Fatal("control mode observer did not report an attached session")
		}
	}

attached:
	sizeAfterAttach := tmuxPaneSize(t, socketName, target)
	t.Logf("control mode pane size before attach=%s after attach=%s", sizeBeforeAttach, sizeAfterAttach)
	marker := "phase0-control-mode-marker"
	sendProbeOutput(t, socketName, target, marker)
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	var observed []string
	for {
		select {
		case line := <-lines:
			observed = append(observed, line)
			if strings.HasPrefix(line, "%output ") && strings.Contains(line, marker) {
				return
			}
		case <-deadline.C:
			t.Fatalf("control mode observer did not receive %%output containing %q; records: %q", marker, observed)
		}
	}
}
