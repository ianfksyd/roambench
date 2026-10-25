package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Exercise the worker's real CLI subprocess, so a missing CLI payload field
// or a progress write racing release cannot be hidden by a mock reporter.
func TestWorkerCLIProgressAndExecutionSettlement(t *testing.T) {
	binDir := t.TempDir()
	agent := filepath.Join(binDir, "roambench-agent")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", agent, "./cmd/roambench-agent")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, out)
	}
	writeScript := func(path, script string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	writeScript(filepath.Join(binDir, "tmux"), "exit 0")
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	t.Setenv("ROAMBENCH_AGENT_BIN", agent)
	t.Setenv("ROAMBENCH_TOKEN", "test-token")

	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "timeout"}[timeout], func(t *testing.T) {
			dir := t.TempDir()
			var mu sync.Mutex
			progressCount, released, artifactOutcome := 0, "", ""
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				var body map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				switch r.URL.Path {
				case "/api/agent/v1/terminal":
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"id":"worker-test-terminal"}`))
					return
				case "/api/agent/v1/progress":
					if body["leaseId"] != "lease-test" || body["taskId"] != "task-test" || released != "" {
						t.Errorf("progress lost its lease or arrived after release: %+v release=%s", body, released)
						w.WriteHeader(http.StatusForbidden)
						return
					}
					progressCount++
				case "/api/agent/v1/artifact":
					artifactOutcome, _ = body["outcome"].(string)
				case "/api/agent/v1/release":
					released, _ = body["outcome"].(string)
				default:
					t.Errorf("unexpected worker request: %s", r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer srv.Close()
			t.Setenv("ROAMBENCH_URL", srv.URL)
			executor := filepath.Join(dir, "executor")
			script := `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"正在执行"}]}}'`
			pidfile := filepath.Join(dir, "pid")
			if timeout {
				script = "echo $$ > " + shellQuote(pidfile) + "\n" + script + "\n(sleep 0.3; touch " + shellQuote(filepath.Join(dir, "escaped")) + ") &\nwait"
			}
			writeScript(executor, script)
			done := make(chan struct{})
			go func() {
				runTask(options{server: srv.URL, token: "test-token", executor: executor, maxRuntime: map[bool]time.Duration{false: time.Second, true: 50 * time.Millisecond}[timeout]}, &claimedTask{TaskID: "task-test", LeaseID: "lease-test", Title: "test", Workspace: dir})
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				b, _ := os.ReadFile(pidfile)
				pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
				if pid > 0 {
					_ = syscall.Kill(-pid, syscall.SIGKILL)
				}
				<-done
				t.Fatal("worker did not settle execution within its bound")
			}
			mu.Lock()
			if progressCount == 0 || released != map[bool]string{false: "done", true: "failed"}[timeout] || artifactOutcome != map[bool]string{false: "pass", true: "fail"}[timeout] {
				t.Errorf("progress=%d release=%s artifact=%s", progressCount, released, artifactOutcome)
			}
			mu.Unlock()
			if timeout {
				time.Sleep(350 * time.Millisecond)
				if _, err := os.Stat(filepath.Join(dir, "escaped")); err == nil {
					t.Fatal("executor child survived the timeout")
				}
			}
		})
	}
}
