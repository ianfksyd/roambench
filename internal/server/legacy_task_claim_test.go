package server

import (
	"net/http"
	"testing"
)

// 存量回归: 升级前创建的任务没有 origin 字段、停在 awaiting_approval、
// 项目还是 manual/delegated。新策略下人发起任务自动派发,
// 这样的任务必须能被 agent 直接领走,不再卡在审批。
func TestLegacyHumanTaskIsClaimableWithoutGate(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	agentToken := agentTokenFor(t, srv, token)

	// 复刻线上形状: 项目 manual + delegated(未接管),任务无 origin。
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Projects {
			if state.Projects[i].ID == "project-roambench-control" {
				state.Projects[i].Takeover = projectTakeover{Mode: "manual"}
				state.Projects[i].Lifecycle = projectLifecycle{State: "delegated"}
			}
		}
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: "task-legacy-human", ProjectID: "project-roambench-control",
			Title: "存量人类任务", State: "planned",
			DispatchState:     "awaiting_approval", // 无 origin 字段(空)
			AcceptanceCommand: []string{"/bin/true"},
			Timeline:          []projectControlEvent{}, Audit: []projectControlAuditItem{},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-1","agentLabel":"claude"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim = %d, want 200 (存量人发起任务应可领): %s", rec.Code, rec.Body.String())
	}
}

// 对照组: agent 发起的任务仍然被人法门控。
func TestLegacyAgentTaskStillGated(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	agentToken := agentTokenFor(t, srv, token)

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Projects {
			if state.Projects[i].ID == "project-roambench-control" {
				state.Projects[i].Takeover = projectTakeover{Mode: "manual"}
				state.Projects[i].Lifecycle = projectLifecycle{State: "delegated"}
			}
		}
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: "task-legacy-agent", ProjectID: "project-roambench-control",
			Title: "存量agent任务", State: "planned", Origin: "agent",
			DispatchState:     "awaiting_approval",
			AcceptanceCommand: []string{"/bin/true"},
			Timeline:          []projectControlEvent{}, Audit: []projectControlAuditItem{},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-1","agentLabel":"claude"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("claim = %d, want 204 (agent 发起仍需人批)", rec.Code)
	}
}
