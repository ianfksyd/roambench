package localruntime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClaude is a stand-in for the claude CLI: it emits valid stream-json
// lines and exits with a controllable code, so tests never need network or
// the real binary.
func fakeClaude(t *testing.T, dir, script string) string {
	t.Helper()
	path := filepath.Join(dir, "fake-claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStartCollectsEventsAndExitCode(t *testing.T) {
	dir := t.TempDir()
	exe := fakeClaude(t, dir, `cat <<'JSON'
{"type":"assistant","message":{"content":[{"type":"text","text":"第一段进展"}]}}
{"type":"result","result":"任务完成"}
JSON
exit 0`)

	var lines []string
	handle, err := Start(context.Background(), StartRequest{
		Executor:  exe,
		Prompt:    "做点事",
		Workspace: dir,
		Observer: ObserverFunc(func(line string) {
			lines = append(lines, line)
		}),
	})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	handle.Wait(context.Background())

	if handle.ExitCode != 0 || handle.Err != nil {
		t.Fatalf("exit=%d err=%v, want clean exit", handle.ExitCode, handle.Err)
	}
	events := handle.Events()
	var sawAssistant, sawResult bool
	for _, e := range events {
		if e.Kind == "assistant" && strings.Contains(e.Text, "第一段进展") {
			sawAssistant = true
		}
		if e.Kind == "result" && strings.Contains(e.Text, "任务完成") {
			sawResult = true
		}
	}
	if !sawAssistant || !sawResult {
		t.Fatalf("events missing: %+v", events)
	}
	// Observer saw the readable text.
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "第一段进展") || !strings.Contains(joined, "[exit 0]") {
		t.Fatalf("observer lines = %v", lines)
	}
}

func TestStartReportsNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	exe := fakeClaude(t, dir, `echo '{"type":"result","result":"boom"}'; exit 3`)

	handle, err := Start(context.Background(), StartRequest{
		Executor:  exe,
		Prompt:    "p",
		Workspace: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.Wait(context.Background())
	if handle.ExitCode != 3 {
		t.Fatalf("exit=%d, want 3", handle.ExitCode)
	}
	if handle.Err == nil {
		t.Error("Err should be non-nil for a non-zero exit")
	}
}

func TestStartRejectsMissingWorkspace(t *testing.T) {
	if _, err := Start(context.Background(), StartRequest{Executor: "claude"}); err == nil {
		t.Fatal("expected an error without a workspace")
	}
}

func TestStopKillsHangingExecutor(t *testing.T) {
	dir := t.TempDir()
	exe := fakeClaude(t, dir, `sleep 60`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handle, err := Start(ctx, StartRequest{Executor: exe, Prompt: "p", Workspace: dir})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	go func() {
		time.Sleep(300 * time.Millisecond)
		handle.Stop()
	}()
	handle.Wait(ctx)
	if time.Now().After(deadline) {
		t.Fatal("Stop did not end the wait within 3s")
	}
	if handle.ExitCode == 0 {
		t.Fatal("killed executor should not report exit 0")
	}
}

func TestParseClaudeLineKinds(t *testing.T) {
	tool := parseClaudeLine(`{"type":"tool_use"}`)
	if tool.Kind != "tool_use" {
		t.Errorf("tool kind = %q", tool.Kind)
	}
	sys := parseClaudeLine(`{"type":"system"}`)
	if sys.Kind != "system" {
		t.Errorf("system kind = %q", sys.Kind)
	}
	garbage := parseClaudeLine("not json")
	if garbage.Kind != "raw" || garbage.Raw != "not json" {
		t.Errorf("garbage = %+v", garbage)
	}
	errLine := parseClaudeLine(`{"type":"result","error":"denied"}`)
	if errLine.Text != "denied" {
		t.Errorf("error text = %q", errLine.Text)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(errLine.Raw), &raw); err != nil {
		t.Errorf("raw is not the original JSON: %v", err)
	}
}

func TestExecutorArgsAndModelArePassed(t *testing.T) {
	dir := t.TempDir()
	script := `for a in "$@"; do echo "ARG:$a"; done; exit 0`
	exe := fakeClaude(t, dir, script)

	handle, err := Start(context.Background(), StartRequest{
		Executor:       exe,
		Args:           []string{"--custom-flag"},
		Model:          "glm-5.3",
		PermissionMode: "acceptEdits",
		Prompt:         "目标文本",
		Workspace:      dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.Wait(context.Background())

	var all strings.Builder
	for _, e := range handle.Events() {
		all.WriteString(e.Raw)
		all.WriteString("\n")
	}
	foundModel := strings.Contains(all.String(), "ARG:--model") && strings.Contains(all.String(), "ARG:glm-5.3")
	foundMode := strings.Contains(all.String(), "ARG:--permission-mode") && strings.Contains(all.String(), "ARG:acceptEdits")
	foundPrompt := strings.Contains(all.String(), "ARG:目标文本")
	_ = foundPrompt
	if !foundModel || !foundMode {
		t.Fatalf("model=%v mode=%v events=%+v", foundModel, foundMode, handle.Events())
	}
	if !strings.Contains(all.String(), "ARG:目标文本") {
		t.Fatalf("prompt not passed: %s", all.String())
	}
	if _, err := exec.LookPath(exe); err != nil {
		t.Fatal(err)
	}
}
