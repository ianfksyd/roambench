package terminal

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/config"
)

func writeCgroupFixture(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestSessionCgroupPrepareAppliesMemorySwapAndPIDLimits(t *testing.T) {
	root := t.TempDir()
	sessionID := "lt-0123456789abcdef"
	sessionDir := filepath.Join(root, "terminals", sessionID)

	writeCgroupFixture(t, root, "cgroup.controllers", "cpu io memory pids\n")
	writeCgroupFixture(t, root, "cgroup.subtree_control", "")
	writeCgroupFixture(t, root, "terminals/cgroup.controllers", "cpu io memory pids\n")
	writeCgroupFixture(t, root, "terminals/cgroup.subtree_control", "")
	for _, name := range []string{"memory.high", "memory.max", "memory.swap.max", "memory.oom.group", "pids.max", "cpu.weight", "io.weight", "cgroup.procs"} {
		writeCgroupFixture(t, root, filepath.Join("terminals", sessionID, name), "")
	}

	manager := &sessionCgroupManager{
		root: root,
		resources: config.TerminalResourceConfig{
			SessionMemoryHighBytes: 3 << 30,
			SessionMemoryMaxBytes:  5 << 30,
			SessionSwapMaxBytes:    512 << 20,
			SessionPIDsMax:         256,
			SessionCPUWeight:       80,
			SessionIOWeight:        80,
		},
	}

	procsPath, err := manager.Prepare(sessionID)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if procsPath != filepath.Join(sessionDir, "cgroup.procs") {
		t.Fatalf("procs path = %q, want %q", procsPath, filepath.Join(sessionDir, "cgroup.procs"))
	}

	wants := map[string]string{
		"memory.high":      strconv.FormatInt(3<<30, 10),
		"memory.max":       strconv.FormatInt(5<<30, 10),
		"memory.swap.max":  strconv.FormatInt(512<<20, 10),
		"memory.oom.group": "1",
		"pids.max":         "256",
		"cpu.weight":       "80",
		"io.weight":        "default 80",
	}
	for name, want := range wants {
		data, readErr := os.ReadFile(filepath.Join(sessionDir, name))
		if readErr != nil {
			t.Fatalf("ReadFile(%s): %v", name, readErr)
		}
		if got := strings.TrimSpace(string(data)); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	for _, path := range []string{filepath.Join(root, "cgroup.subtree_control"), filepath.Join(root, "terminals", "cgroup.subtree_control")} {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s): %v", path, readErr)
		}
		got := string(data)
		for _, controller := range []string{"+cpu", "+io", "+memory", "+pids"} {
			if !strings.Contains(got, controller) {
				t.Fatalf("%s missing %q in %q", path, controller, got)
			}
		}
	}
}

func TestSessionCgroupPrepareRejectsUnsafeSessionID(t *testing.T) {
	manager := &sessionCgroupManager{root: t.TempDir()}
	if _, err := manager.Prepare("../escape"); err == nil {
		t.Fatal("Prepare error = nil, want unsafe session ID rejection")
	}
}

func TestSessionCgroupPrepareReportsMissingDelegatedController(t *testing.T) {
	root := t.TempDir()
	writeCgroupFixture(t, root, "cgroup.controllers", "memory\n")
	writeCgroupFixture(t, root, "cgroup.subtree_control", "")
	manager := &sessionCgroupManager{
		root:      root,
		resources: config.TerminalResourceConfig{SessionPIDsMax: 256},
	}

	_, err := manager.Prepare("lt-0123456789abcdef")
	if !errors.Is(err, ErrResourceControlUnavailable) {
		t.Fatalf("Prepare error = %v, want %v", err, ErrResourceControlUnavailable)
	}
}

func TestTmuxShellCommandMovesShellIntoSessionCgroupBeforeExec(t *testing.T) {
	cgroupProcs := filepath.Join(t.TempDir(), "cgroup.procs")
	if err := os.WriteFile(cgroupProcs, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	command := tmuxShellCommandInCgroup("/bin/sh", t.TempDir(), cgroupProcs)
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdin = strings.NewReader("")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shell command failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}

	data, err := os.ReadFile(cgroupProcs)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		t.Fatalf("cgroup.procs = %q, want a positive PID", strings.TrimSpace(string(data)))
	}
}
