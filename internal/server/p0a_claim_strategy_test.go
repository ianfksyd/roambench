package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/controlplane"
)

// P0a: 租约策略一致性。manual 项目下人发起任务领取后，
// 终端绑定、状态保存、健康监控、revokeClosedClaims 都不得撤销其租约；
// 显式 pause/stop 才撤销（对照组）。
func TestP0AManualProjectHumanLeaseSurvivesMaintenance(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Projects {
			if state.Projects[i].ID == "project-roambench-control" {
				state.Projects[i].Takeover = projectTakeover{Mode: "manual"}
				state.Projects[i].Lifecycle = projectLifecycle{State: "delegated"}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	taskReq := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks",
		strings.NewReader(`{"projectId":"project-roambench-control","workstreamId":"workstream-ui-ia","title":"P0A human task"}`))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusCreated {
		t.Fatalf("create task = %d: %s", taskRec.Code, taskRec.Body.String())
	}
	snapshot := decodeProjectControlSnapshot(t, taskRec)
	taskID := ""
	for _, task := range snapshot.Tasks {
		if task.Title == "P0A human task" {
			taskID = task.ID
		}
	}
	if taskID == "" {
		t.Fatal("task not found")
	}

	leaseID := claimLeaseForTask(t, srv, token, taskID)

	leaseActive := func() bool {
		claim, err := srv.projectControl.claimStoreFor().GetClaim(context.Background(), "ian", leaseID)
		return err == nil && claim.Status == "active"
	}
	if !leaseActive() {
		t.Fatal("lease not active right after claim")
	}

	// 终端绑定不得撤销租约。
	srv.projectControl.bindTaskTerminal("ian", taskID, "lt-p0a-probe", "project-roambench-control")
	if !leaseActive() {
		t.Fatal("bindTaskTerminal revoked the lease")
	}

	// 状态保存（一次普通变更）不得撤销租约。
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !leaseActive() {
		t.Fatal("a plain state save revoked the lease")
	}

	// 健康监控不得撤销租约。
	srv.projectControl.checkHealth("ian", nil)
	if !leaseActive() {
		t.Fatal("checkHealth revoked the lease")
	}

	// revokeClosedClaims（P0a 修复点）：manual 项目的人发起任务不撤销。
	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.projectControl.revokeClosedClaims("ian", state); err != nil {
		t.Fatal(err)
	}
	if !leaseActive() {
		t.Fatal("revokeClosedClaims revoked a live human-task lease in a manual project")
	}

	// 对照组 1：显式 pause → 撤销。
	if _, err := srv.projectControl.controlTakeover("ian", "project-roambench-control",
		projectTakeoverRequest{Action: "pause", TaskID: taskID, Reason: "p0a_pause"}, false); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if leaseActive() {
		t.Fatal("explicit pause did not revoke the lease")
	}

	// 恢复：manual 模式下人发起任务由策略直接放行，手动清 pause 并置 ready。
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID == taskID {
				state.Tasks[i].PauseReason = ""
				state.Tasks[i].DispatchState = "ready"
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	leaseID2 := claimLeaseForTask(t, srv, token, taskID)
	if _, err := srv.projectControl.controlTakeover("ian", "project-roambench-control",
		projectTakeoverRequest{Action: "stop"}, false); err != nil {
		t.Fatalf("stop: %v", err)
	}
	claim2, err := srv.projectControl.claimStoreFor().GetClaim(context.Background(), "ian", leaseID2)
	if err != nil {
		t.Fatal(err)
	}
	if claim2.Status == "active" {
		t.Fatal("explicit stop did not revoke the lease")
	}
}

// claimLeaseForTask claims until the given task is obtained and returns the lease id.
func claimLeaseForTask(t *testing.T, srv *Server, token, taskID string) string {
	t.Helper()
	agentToken := agentTokenFor(t, srv, token)
	for attempts := 0; attempts < 5; attempts++ {
		rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"p0a-runtime","agentLabel":"claude"}`)
		if rec.Code != http.StatusOK {
			continue
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["taskId"] == taskID {
			lease, _ := payload["leaseId"].(string)
			if lease == "" {
				t.Fatal("claim response missing leaseId")
			}
			return lease
		}
	}
	t.Fatalf("task %s was never claimed", taskID)
	return ""
}

// P0a: 无验收命令任务的完成 = 执行成功，验证未完成。
func TestP0ANoCommandReleaseIsExecutionSuccessNotVerified(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Projects {
			if state.Projects[i].ID == "project-roambench-control" {
				state.Projects[i].Takeover = projectTakeover{Mode: "active"}
				state.Projects[i].Lifecycle = projectLifecycle{State: "executing"}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	taskReq := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks",
		strings.NewReader(`{"projectId":"project-roambench-control","workstreamId":"workstream-ui-ia","title":"P0A no command task"}`))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusCreated {
		t.Fatalf("create task = %d", taskRec.Code)
	}
	snapshot := decodeProjectControlSnapshot(t, taskRec)
	taskID := ""
	for _, task := range snapshot.Tasks {
		if task.Title == "P0A no command task" {
			taskID = task.ID
		}
	}

	agentToken := agentTokenFor(t, srv, token)
	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"p0a-runtime","agentLabel":"claude"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim = %d: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	lease, _ := payload["leaseId"].(string)

	rec = agentAPIPost(t, srv, "/api/agent/v1/release", agentToken,
		`{"leaseId":"`+lease+`","outcome":"done"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("release = %d: %s", rec.Code, rec.Body.String())
	}
	var releaseBody map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&releaseBody); err != nil {
		t.Fatal(err)
	}
	if passed, _ := releaseBody["acceptancePassed"].(bool); passed {
		t.Error("acceptancePassed must be false without an acceptance command")
	}

	task, ok := controlTask(i2State(t, srv), "")
	_ = task
	_ = ok
	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	final, ok := controlTask(state, taskID)
	if !ok {
		t.Fatal("task missing")
	}
	if final.State != "execution_complete" || final.DispatchState != "done" {
		t.Errorf("state=%q dispatch=%q, want execution_complete/done", final.State, final.DispatchState)
	}
	if final.AcceptancePassed {
		t.Error("AcceptancePassed must stay false without an acceptance command")
	}
	if final.AcceptanceStatus != "not_ready" {
		t.Errorf("AcceptanceStatus = %q, want not_ready (verification pending)", final.AcceptanceStatus)
	}

	// 重复 release → 404（同一 lease 已终结）。
	rec = agentAPIPost(t, srv, "/api/agent/v1/release", agentToken,
		`{"leaseId":"`+lease+`","outcome":"done"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("duplicate release = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	// 未知 lease → 404。
	rec = agentAPIPost(t, srv, "/api/agent/v1/release", agentToken,
		`{"leaseId":"claim-unknown","outcome":"done"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown lease release = %d, want 404", rec.Code)
	}
}

// P0a: 过期租约的 release 被拒绝。
func TestP0AExpiredLeaseReleaseRejected(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Projects {
			if state.Projects[i].ID == "project-roambench-control" {
				state.Projects[i].Takeover = projectTakeover{Mode: "active"}
				state.Projects[i].Lifecycle = projectLifecycle{State: "executing"}
			}
		}
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: "task-p0a-expired", ProjectID: "project-roambench-control",
			Title: "expired release", State: "queued", Origin: "agent",
			DispatchState: "ready", AcceptanceCommand: []string{"/bin/true"},
			Timeline: []projectControlEvent{}, Audit: []projectControlAuditItem{},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	agentToken := agentTokenFor(t, srv, token)
	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"p0a-runtime","agentLabel":"claude"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim = %d", rec.Code)
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	lease, _ := payload["leaseId"].(string)

	// 把 worker 时钟拨到租约过期之后（ReleaseClaim 用 ClaimStore 的时钟判断过期）。
	base := time.Now().UTC().Add(10 * time.Minute)
	previousNow := projectControlClaimNow
	projectControlClaimNow = func() time.Time { return base }
	t.Cleanup(func() { projectControlClaimNow = previousNow })
	// claimStoreFor 每次用 projectControlClaimNow 重建，时钟覆盖随之生效。
	rec = agentAPIPost(t, srv, "/api/agent/v1/release", agentToken,
		`{"leaseId":"`+lease+`","outcome":"done"}`)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusConflict {
		t.Fatalf("expired release = %d, want 404/409: %s", rec.Code, rec.Body.String())
	}
}

// P0a: 依赖语义——无验收命令的 dep 完成（done）即放行；
// 有验收命令的 dep 必须真正通过。
func TestP0ADependencySemanticsForUnverifiedDeps(t *testing.T) {
	state := projectControlState{
		Projects: []projectControlProject{{ID: "p", Takeover: projectTakeover{Mode: "manual"}}},
		Tasks: []projectControlTask{
			{ID: "dep-no-command", ProjectID: "p", DispatchState: "done"},
			{ID: "dep-with-command", ProjectID: "p", DispatchState: "done", AcceptanceCommand: []string{"/bin/true"}},
			{ID: "dep-with-command-failed", ProjectID: "p", DispatchState: "done", AcceptanceCommand: []string{"/bin/false"}, AcceptancePassed: false},
			{ID: "dep-with-command-passed", ProjectID: "p", DispatchState: "done", AcceptanceCommand: []string{"/bin/true"}, AcceptancePassed: true},
		},
	}
	dep := func(id string, dependsOn ...string) projectControlTask {
		return projectControlTask{ID: "t-" + id, ProjectID: "p", DispatchState: "ready", DependsOn: dependsOn}
	}
	if !taskDependenciesPassed(state, dep("a", "dep-no-command")) {
		t.Error("done dep without command should satisfy the dependency")
	}
	if taskDependenciesPassed(state, dep("b", "dep-with-command")) {
		t.Error("done dep with unpassed command must not satisfy")
	}
	if !taskDependenciesPassed(state, dep("c", "dep-with-command-passed")) {
		t.Error("passed dep should satisfy")
	}
	if taskDependenciesPassed(state, dep("d", "dep-with-command-failed", "dep-no-command")) {
		t.Error("failed command dep must block")
	}
}

// P0a: JSON 保存失败时领取必须干净失败——租约被回滚、任务投影可恢复、重试成功。
func TestP0AClaimSurvivesJSONSaveFailure(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID == "task-information-architecture" {
				state.Tasks[i].State = "planned"
				state.Tasks[i].DispatchState = "ready"
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// 注入：把 JSON 状态目录变为只读，真实触发 saveLocked 失败。
	agentToken := agentTokenFor(t, srv, token)

	usersDir := srv.projectControl.rootDir
	if err := os.Chmod(usersDir, 0500); err != nil {
		t.Fatalf("chmod users dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(usersDir, 0700) })

	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"p0a-runtime","agentLabel":"claude"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("claim during JSON failure = %d, want 500: %s", rec.Code, rec.Body.String())
	}

	// 恢复写入。
	if err := os.Chmod(usersDir, 0700); err != nil {
		t.Fatal(err)
	}

	// 租约必须已被回滚：失败路径调用 RevokeTaskClaims 后，重新领取应能成功
	// （若租约残留，task_claims 的 active 唯一索引会让重试冲突失败）。
	claims := srv.projectControl.claimStoreFor()
	if claims == nil {
		t.Fatal("claim store unavailable")
	}
	if _, err := claims.ExpireStaleClaims(context.Background()); err != nil {
		t.Fatal(err)
	}

	// 任务投影回到 ready（JSON 未被部分写入）。
	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, task := range state.Tasks {
		if task.ID == "task-information-architecture" && task.DispatchState == "ready" {
			found = true
		}
	}
	if !found {
		t.Fatal("task did not recover to ready after the failed save")
	}

	// 重试领取成功。
	rec = agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"p0a-runtime","agentLabel":"claude"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("retry claim = %d: %s", rec.Code, rec.Body.String())
	}
}

// P0a: 快照投影——claimed 但租约丢失的任务显示 unverified；有租约保持 claimed。
func TestP0ASnapshotUnverifiedProjection(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID == "task-information-architecture" {
				state.Tasks[i].State = "planned"
				state.Tasks[i].DispatchState = "ready"
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	agentToken := agentTokenFor(t, srv, token)
	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"p0a-runtime","agentLabel":"claude"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim = %d: %s", rec.Code, rec.Body.String())
	}

	// 有租约：保持 claimed。
	snapshot, err := srv.projectControl.snapshotForUser("ian", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range snapshot.Tasks {
		if task.ID == "task-information-architecture" && task.DispatchState != "claimed" {
			t.Fatalf("claimed task projected as %q", task.DispatchState)
		}
	}

	// 租约丢失（模拟历史遗留）：通过 claim 失败回滚同款 RevokeTaskClaims 清掉
	// active 租约，再投影为 unverified，且不写回磁盘。
	if err := srv.projectControl.claimStoreFor().RevokeTaskClaims(context.Background(), "ian", "task-information-architecture"); err != nil {
		t.Fatal(err)
	}
	snapshot, err = srv.projectControl.snapshotForUser("ian", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range snapshot.Tasks {
		if task.ID == "task-information-architecture" && task.DispatchState != "unverified" {
			t.Fatalf("claimed-without-lease projected as %q, want unverified", task.DispatchState)
		}
	}
	persisted, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range persisted.Tasks {
		if task.ID == "task-information-architecture" && task.DispatchState == "unverified" {
			t.Fatal("unverified projection must not be persisted")
		}
	}
	_ = controlplane.LeaseTTL
}
