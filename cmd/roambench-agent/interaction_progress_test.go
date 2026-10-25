package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProgressCarriesLeaseAndOverrides(t *testing.T) {
	swapExitForTest(t)
	var received atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			TaskID  string `json:"taskId"`
			LeaseID string `json:"leaseId"`
			Seq     int    `json:"seq"`
			Text    string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/api/agent/v1/progress" || r.Header.Get("Authorization") != "Bearer test-token" || body.TaskID != "task-1" || body.LeaseID != "lease-1" || body.Seq != 1 || body.Text != "已完成第一步" {
			t.Errorf("unexpected progress request: %s, %+v", r.URL.Path, body)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		received.Store(true)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	t.Setenv("ROAMBENCH_URL", "http://invalid.example")
	t.Setenv("ROAMBENCH_TOKEN", "wrong-token")
	_, code := runClaimCmd(t, func() {
		cmdProgress([]string{"--server", srv.URL, "--token", "test-token", "--task", "task-1", "--lease-id", "lease-1", "--seq", "1", "--text", "已完成第一步\n"})
	})
	if code != 0 || !received.Load() {
		t.Fatalf("progress exit=%d, received=%v", code, received.Load())
	}
}

func TestProgressRequiresLease(t *testing.T) {
	swapExitForTest(t)
	_, code := runClaimCmd(t, func() {
		cmdProgress([]string{"--task", "task-1", "--seq", "1", "--text", "progress"})
	})
	if code == 0 {
		t.Fatal("progress without a lease succeeded")
	}
}

func TestAskWaitsAcrossPendingPollsAndPreservesChoice(t *testing.T) {
	swapExitForTest(t)
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"requestId":"question-1"}`))
			return
		}
		if polls.Add(1) < 3 {
			_, _ = w.Write([]byte(`{"status":"pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"resolved","response":{"action":"answer","feedback":"选择审查路线","selectedOptionIds":["review"]}}`))
	}))
	defer srv.Close()
	stdout, code := runClaimCmd(t, func() {
		cmdAsk([]string{"--server", srv.URL, "--token", "test-token", "--task", "task-1", "--question", "选择路线", "--options", "review,implement", "--wait", "2s"})
	})
	if code != 0 || polls.Load() != 3 {
		t.Fatalf("ask exit=%d polls=%d output=%s", code, polls.Load(), stdout)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	var answer struct {
		Action   string   `json:"action"`
		Feedback string   `json:"feedback"`
		Options  []string `json:"selectedOptionIds"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Action != "answer" || answer.Feedback != "选择审查路线" || len(answer.Options) != 1 || answer.Options[0] != "review" {
		t.Fatalf("answer lost response fields: %+v", answer)
	}
}

func TestAskStopsOnTerminalStatusOrOverallDeadline(t *testing.T) {
	for _, status := range []string{"expired", "cancelled", "pending"} {
		t.Run(status, func(t *testing.T) {
			swapExitForTest(t)
			var polls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"requestId":"question-1"}`))
					return
				}
				if polls.Add(1) > 1 {
					<-r.Context().Done()
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"status": status})
			}))
			defer srv.Close()
			_, code := runClaimCmd(t, func() {
				cmdAsk([]string{"--server", srv.URL, "--token", "test-token", "--task", "task-1", "--question", "Proceed?", "--wait", "250ms"})
			})
			if code == 0 {
				t.Fatal("unanswered question reported success")
			}
			if status == "pending" && polls.Load() != 2 {
				t.Fatalf("pending did not keep waiting: polls=%d", polls.Load())
			}
			if status != "pending" && polls.Load() != 1 {
				t.Fatalf("terminal status was polled again: polls=%d", polls.Load())
			}
		})
	}
}
