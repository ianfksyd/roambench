package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
)

// 存量回归: 自动派发策略上线前,人发起任务的 pending dispatch gate 已写进
// control plane。加载时该门必须被自动批准并从状态里清掉——否则任务明明
// running,界面却永远显示"awaiting approval"。
func TestStaleDispatchGateResolvedOnLoad(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	// 复刻线上形状: 人发起(无 origin)任务已 running,但仍有 pending 派发门。
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID == "task-information-architecture" {
				state.Tasks[i].State = "running"
			}
		}
		state.Checkpoints = append(state.Checkpoints, projectControlCheckpoint{
			ID: "checkpoint-stale-dispatch", TaskID: "task-information-architecture",
			Kind: "dispatch", Status: "pending", AllowedActions: []string{"approve", "reject"}, RowVersion: 1,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// 触发一次加载(hydrate + resolve + save)。
	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	for _, checkpoint := range state.Checkpoints {
		if checkpoint.ID == "checkpoint-stale-dispatch" && checkpoint.Status == "pending" {
			t.Fatal("stale dispatch gate still pending after load")
		}
	}
	for _, task := range state.Tasks {
		if task.ID == "task-information-architecture" && task.DispatchState == "awaiting_approval" {
			t.Fatal("running human task still awaiting_approval after load")
		}
	}

	// 再加载一次: 幂等,不再产生新事件或改写。
	before := len(state.Events)
	state, err = srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Events) > before {
		t.Fatalf("resolve is not idempotent: events %d -> %d", before, len(state.Events))
	}

	// 快照(HTTP 视角)同样干净。
	req := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var snapshot projectControlSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.ID == "checkpoint-stale-dispatch" && checkpoint.Status == "pending" {
			t.Fatal("snapshot still carries the stale pending gate")
		}
	}
}

// agent 发起任务的 pending 门不受影响。
func TestStaleGateResolutionSparesAgentTasks(t *testing.T) {
	srv, _, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: "task-agent-raised", ProjectID: "project-roambench-control",
			Title: "agent 发起", State: "planned", Origin: "agent",
			DispatchState: "awaiting_approval",
			Timeline:      []projectControlEvent{}, Audit: []projectControlAuditItem{},
		})
		state.Checkpoints = append(state.Checkpoints, projectControlCheckpoint{
			ID: "checkpoint-agent-dispatch", TaskID: "task-agent-raised",
			Kind: "dispatch", Status: "pending", AllowedActions: []string{"approve", "reject"}, RowVersion: 1,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	for _, checkpoint := range state.Checkpoints {
		if checkpoint.ID == "checkpoint-agent-dispatch" && checkpoint.Status != "pending" {
			t.Fatal("agent-raised gate must stay pending for human decision")
		}
	}
}
