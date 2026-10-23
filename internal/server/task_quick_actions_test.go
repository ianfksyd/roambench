package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// 看板快速操作的后端契约: pause_task / resume_task / cancel_task。
func TestTaskQuickActionsPauseResumeCancel(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	task := createProjectControlDocsUpdateTask(t, srv, token, "Quick Actions Task")

	patch := func(action string) projectControlTask {
		body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":%q}`, task.RowVersion, action)
		return patchProjectControlTaskAndFind(t, srv, token, task, body)
	}

	// 暂停: 状态不变,但带暂停原因 → agent 不能领。
	task = patch("pause_task")
	if task.PauseReason != "paused_by_user" {
		t.Fatalf("PauseReason = %q, want paused_by_user", task.PauseReason)
	}
	if task.State != "planned" {
		t.Fatalf("State = %q, want planned (pause keeps state)", task.State)
	}

	// 继续: 清掉暂停原因,状态仍不变。
	task = patch("resume_task")
	if task.PauseReason != "" {
		t.Fatalf("PauseReason = %q, want empty after resume", task.PauseReason)
	}
	if task.State != "planned" {
		t.Fatalf("State = %q, want planned after resume", task.State)
	}

	// 取消: 进入 cancelled 终态。
	task = patch("cancel_task")
	if task.State != "cancelled" {
		t.Fatalf("State = %q, want cancelled", task.State)
	}

	// 取消后可重新开始(回到 running)。
	task = patch("resume_execution")
	if task.State != "running" {
		t.Fatalf("State = %q, want running after restart from cancelled", task.State)
	}
}

// 暂停中的任务不能被 agent 领取。
func TestPausedTaskIsNotClaimable(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	agentToken := agentTokenFor(t, srv, token)

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID == "task-information-architecture" {
				state.Tasks[i].PauseReason = "paused_by_user"
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-1","agentLabel":"claude"}`)
	if rec.Code != http.StatusOK {
		return // 204 = 没领到任何任务,暂停生效
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if claimed, _ := payload["taskId"].(string); claimed == "task-information-architecture" {
		t.Fatalf("paused task was claimed: %s", rec.Body.String())
	}
}
