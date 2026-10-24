package server

// C5 handler tests (WS-20261015-001): claim/heartbeat/release over HTTP,
// injected-clock expiry through the handler path, and the checkpoint
// approval linkage (approve -> ready -> claimable; reject -> not claimable).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/controlplane"
)

// setTaskDispatchState writes DispatchState directly into the project-control
// task, bypassing the checkpoint approval hook for lease-focused fixtures.
func setTaskDispatchState(t *testing.T, srv *Server, token, taskID, dispatchState string) {
	t.Helper()
	state, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		if _, ok := controlProject(*state, "project-test"); !ok {
			state.Projects = append(state.Projects, projectControlProject{ID: "project-test", Takeover: projectTakeover{Mode: "active"}, Lifecycle: projectLifecycle{State: "executing"}})
		}
		for i := range state.Tasks {
			if state.Tasks[i].ID != taskID {
				continue
			}
			state.Tasks[i].DispatchState = dispatchState
			state.Tasks[i].ClaimedByRuntimeID = ""
			return nil
		}
		if state.Tasks == nil {
			state.Tasks = nil
		}
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: taskID, ProjectID: "project-test", Title: "Test task " + taskID,
			// These fixtures simulate a task waiting for a human dispatch
			// decision, so they are agent-raised: only agent-raised tasks are
			// gated (human-raised ones are dispatched automatically).
			Origin: "agent",
			State:  "queued", DispatchState: dispatchState, AcceptanceCommand: []string{"/bin/true"},
			Timeline: []projectControlEvent{}, Audit: []projectControlAuditItem{},
			FilesChanged: []string{}, SessionIDs: []string{},
		})
		return nil
	})
	_ = state
	if err != nil {
		t.Fatalf("set dispatch state: %v", err)
	}
}

func agentAPIPost(t *testing.T, srv *Server, path, agentToken, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	return rec
}

func agentTokenFor(t *testing.T, srv *Server, token string) string {
	t.Helper()
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/project-control/agent-token", nil)
	tokenReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	tokenRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(tokenRec, tokenReq)
	var tokenResp map[string]string
	json.NewDecoder(tokenRec.Body).Decode(&tokenResp)
	return tokenResp["token"]
}

func TestAgentClaimTaskEndToEnd(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	agentToken := agentTokenFor(t, srv, token)
	muteSeedTasks(t, srv)

	setTaskDispatchState(t, srv, token, "task-e2e-1", "ready")

	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-hermes","agentLabel":"hermes"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode claim response: %v", err)
	}
	if payload["taskId"] != "task-e2e-1" {
		t.Fatalf("claimed taskId = %v, want task-e2e-1", payload["taskId"])
	}
	leaseID, _ := payload["leaseId"].(string)
	if leaseID == "" {
		t.Fatal("claim response missing leaseId")
	}
	expiresAt, _ := payload["leaseExpiresAt"].(string)
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		t.Fatalf("leaseExpiresAt not RFC3339: %v (%q)", err, expiresAt)
	}
	if got := time.Until(expires); got <= 0 || got > controlplane.LeaseTTL+5*time.Second {
		t.Fatalf("leaseExpiresAt %s not ~120s in the future", expiresAt)
	}
	if payload["title"] != "Test task task-e2e-1" {
		t.Fatalf("task fields missing from claim response: %v", payload["title"])
	}

	// The claimed task is gone from the ready pool: next claim is 204.
	rec = agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-b","agentLabel":"codex"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("second claim status = %d, want 204", rec.Code)
	}

	// Heartbeat renews.
	rec = agentAPIPost(t, srv, "/api/agent/v1/heartbeat", agentToken, `{"leaseId":"`+leaseID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Release with done.
	rec = agentAPIPost(t, srv, "/api/agent/v1/release", agentToken, `{"leaseId":"`+leaseID+`","outcome":"done"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("release status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var releasePayload map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&releasePayload)
	if releasePayload["dispatchState"] != controlplane.DispatchStateDone {
		t.Fatalf("release dispatchState = %v, want done", releasePayload["dispatchState"])
	}

	// Task dispatch state landed in the project-control projection.
	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatalf("loadOrSeed: %v", err)
	}
	for _, task := range state.Tasks {
		if task.ID == "task-e2e-1" {
			if task.DispatchState != controlplane.DispatchStateDone {
				t.Fatalf("task dispatchState = %s, want done", task.DispatchState)
			}
		}
	}
}

func TestAgentClaimNoReadyTasksReturns204(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	agentToken := agentTokenFor(t, srv, token)
	muteSeedTasks(t, srv)

	setTaskDispatchState(t, srv, token, "task-awaiting", "awaiting_approval")

	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-1","agentLabel":"hermes"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("claim status = %d, want 204 (awaiting_approval is not claimable): %s", rec.Code, rec.Body.String())
	}
}

func TestCheckpointApprovalGatesClaimability(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	agentToken := agentTokenFor(t, srv, token)
	muteSeedTasks(t, srv)

	// Simulate a freshly created task waiting for a dispatch checkpoint.
	setTaskDispatchState(t, srv, token, "task-gate-1", "awaiting_approval")

	// Before approval: not claimable.
	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-1","agentLabel":"hermes"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("claim before approval = %d, want 204", rec.Code)
	}

	// Approval hook (checkpoint approve) sets ready — the exact write the
	// recordCheckpointDecision hook will perform once wired (K4).
	setTaskDispatchState(t, srv, token, "task-gate-1", "ready")
	rec = agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-1","agentLabel":"hermes"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim after approval = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&payload)
	leaseID, _ := payload["leaseId"].(string)

	// I2: failed release pauses the task; an explicit restart permits re-claim.
	rec = agentAPIPost(t, srv, "/api/agent/v1/release", agentToken, `{"leaseId":"`+leaseID+`","outcome":"failed"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("release failed outcome = %d, want 200", rec.Code)
	}
	if _, err := srv.projectControl.controlTakeover("ian", "project-test", projectTakeoverRequest{Action: "resume", TaskID: "task-gate-1", Restart: true}, false); err != nil {
		t.Fatal(err)
	}
	rec = agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-2","agentLabel":"codex"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-claim after failed release = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Rejection path (checkpoint reject) makes the task non-claimable again.
	setTaskDispatchState(t, srv, token, "task-gate-1", "none")
	rec = agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-3","agentLabel":"kiro-cli"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("claim after rejection = %d, want 204", rec.Code)
	}
}

func TestAgentClaimConcurrentOnlyOneWins(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	agentToken := agentTokenFor(t, srv, token)
	muteSeedTasks(t, srv)

	setTaskDispatchState(t, srv, token, "task-race-1", "ready")

	var mu sync.Mutex
	var winners []string
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(10)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ready.Done()
			<-start
			rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-`+string(rune('a'+n))+`","agentLabel":"hermes"}`)
			mu.Lock()
			defer mu.Unlock()
			if rec.Code == http.StatusOK {
				winners = append(winners, rec.Body.String())
			} else if rec.Code != http.StatusNoContent {
				t.Errorf("concurrent claim returned %d: %s", rec.Code, rec.Body.String())
			}
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()
	if len(winners) != 1 {
		t.Fatalf("concurrent HTTP claims won by %d handlers, want exactly 1", len(winners))
	}
}

func TestAgentLeaseExpiryReturnsTaskToReady(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	agentToken := agentTokenFor(t, srv, token)
	muteSeedTasks(t, srv)

	// Freeze the injected claim clock.
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	var clockValue atomicTimeValue
	clockValue.Store(base)
	previousNow := projectControlClaimNow
	projectControlClaimNow = func() time.Time { return clockValue.Load() }
	defer func() { projectControlClaimNow = previousNow }()

	setTaskDispatchState(t, srv, token, "task-exp-1", "ready")
	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-1","agentLabel":"hermes"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Move past TTL: I2 pauses until an explicit restart; the old lease stays revoked.
	clockValue.Store(base.Add(controlplane.LeaseTTL + time.Second))
	if _, err := srv.projectControl.expireStaleClaimsForUser("ian"); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.projectControl.controlTakeover("ian", "project-test", projectTakeoverRequest{Action: "resume", TaskID: "task-exp-1", Restart: true}, false); err != nil {
		t.Fatal(err)
	}
	rec = agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-2","agentLabel":"codex"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim after lease expiry = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&payload)
	if payload["runtimeId"] != "runtime-2" {
		t.Fatalf("claim after expiry went to %v, want runtime-2", payload["runtimeId"])
	}

	// The expired lease can no longer be heartbeated.
	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatalf("loadOrSeed: %v", err)
	}
	_ = state
	rec = agentAPIPost(t, srv, "/api/agent/v1/heartbeat", agentToken, `{"leaseId":"lease-from-expired"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("heartbeat on expired lease = %d, want 404", rec.Code)
	}
}

// atomicTimeValue is a small atomic wrapper for time.Time in tests.
type atomicTimeValue struct{ v atomic.Value }

func (a *atomicTimeValue) Store(t time.Time) { a.v.Store(t) }
func (a *atomicTimeValue) Load() time.Time   { return a.v.Load().(time.Time) }

// K4 wiring test (FIXER round): creating a task must raise a pending
// kind="dispatch" checkpoint; approving it must set DispatchState=ready and
// make the task claimable end-to-end; rejecting keeps it unclaimable.
func TestDispatchCheckpointCreationAndApprovalGatesClaim(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	agentToken := agentTokenFor(t, srv, token)

	// Create a project + workstream + task via the API.
	projectReq := httptest.NewRequest(http.MethodPost, "/api/project-control/projects", strings.NewReader(`{"key":"k4-demo","name":"K4 Demo"}`))
	projectReq.Header.Set("Content-Type", "application/json")
	projectReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	projectRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(projectRec, projectReq)
	if projectRec.Code != http.StatusCreated {
		t.Fatalf("POST project status = %d: %s", projectRec.Code, projectRec.Body.String())
	}
	projectSnapshot := decodeProjectControlSnapshot(t, projectRec)
	createdProject := projectSnapshot.Projects[len(projectSnapshot.Projects)-1]
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Projects {
			if state.Projects[i].ID == createdProject.ID {
				state.Projects[i].Takeover.Mode = "active"
				state.Projects[i].Lifecycle.State = "executing"
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	workstreamReq := httptest.NewRequest(http.MethodPost, "/api/project-control/workstreams", strings.NewReader(fmt.Sprintf(`{"projectId":%q,"title":"K4 Stream"}`, createdProject.ID)))
	workstreamReq.Header.Set("Content-Type", "application/json")
	workstreamReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	workstreamRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(workstreamRec, workstreamReq)
	if workstreamRec.Code != http.StatusCreated {
		t.Fatalf("POST workstream status = %d: %s", workstreamRec.Code, workstreamRec.Body.String())
	}
	workstreamSnapshot := decodeProjectControlSnapshot(t, workstreamRec)
	createdWorkstream := workstreamSnapshot.Workstreams[len(workstreamSnapshot.Workstreams)-1]

	taskReq := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks", strings.NewReader(fmt.Sprintf(`{"projectId":%q,"workstreamId":%q,"title":"K4 dispatch task","origin":"agent","acceptanceCommand":["/bin/true"]}`, createdProject.ID, createdWorkstream.ID)))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusCreated {
		t.Fatalf("POST task status = %d: %s", taskRec.Code, taskRec.Body.String())
	}
	snapshot := decodeProjectControlSnapshot(t, taskRec)

	// The new task carries a pending dispatch checkpoint and is awaiting_approval.
	var createdTaskID, dispatchCheckpointID string
	for _, task := range snapshot.Tasks {
		if task.Title != "K4 dispatch task" {
			continue
		}
		createdTaskID = task.ID
		if task.DispatchState != "awaiting_approval" {
			t.Fatalf("created task dispatchState = %q, want awaiting_approval", task.DispatchState)
		}
	}
	if createdTaskID == "" {
		t.Fatal("created task not found")
	}
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == createdTaskID && checkpoint.Kind == "dispatch" && checkpoint.Status == "pending" {
			dispatchCheckpointID = checkpoint.ID
		}
	}
	if dispatchCheckpointID == "" {
		t.Fatalf("expected pending dispatch checkpoint, got %#v", snapshot.Checkpoints)
	}

	// Before approval: claim returns 204.
	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-k4","agentLabel":"hermes"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("claim before dispatch approval = %d, want 204", rec.Code)
	}

	// Approve the dispatch checkpoint.
	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+dispatchCheckpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST dispatch decision status = %d: %s", decisionRec.Code, decisionRec.Body.String())
	}
	approved := decodeProjectControlSnapshot(t, decisionRec)
	for _, task := range approved.Tasks {
		if task.ID == createdTaskID && task.DispatchState != "ready" {
			t.Fatalf("task dispatchState after approve = %q, want ready", task.DispatchState)
		}
	}

	// After approval: the claim succeeds and returns the task.
	rec = agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-k4","agentLabel":"hermes"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim after dispatch approval = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var claimPayload map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&claimPayload)
	if claimPayload["taskId"] != createdTaskID {
		t.Fatalf("claimed taskId = %v, want %v", claimPayload["taskId"], createdTaskID)
	}
	leaseID, _ := claimPayload["leaseId"].(string)
	rec = agentAPIPost(t, srv, "/api/agent/v1/release", agentToken, `{"leaseId":"`+leaseID+`","outcome":"done"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("release status = %d, want 200", rec.Code)
	}

	// Reject path: a second task rejected at its checkpoint must be unclaimable.
	taskReq2 := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks", strings.NewReader(fmt.Sprintf(`{"projectId":%q,"workstreamId":%q,"title":"K4 reject task","origin":"agent"}`, createdProject.ID, createdWorkstream.ID)))
	taskReq2.Header.Set("Content-Type", "application/json")
	taskReq2.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec2 := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec2, taskReq2)
	if taskRec2.Code != http.StatusCreated {
		t.Fatalf("POST second task status = %d: %s", taskRec2.Code, taskRec2.Body.String())
	}
	snapshot2 := decodeProjectControlSnapshot(t, taskRec2)
	var rejectTaskID, rejectCheckpointID string
	for _, task := range snapshot2.Tasks {
		if task.Title != "K4 reject task" {
			continue
		}
		rejectTaskID = task.ID
	}
	for _, checkpoint := range snapshot2.Checkpoints {
		if checkpoint.TaskID == rejectTaskID && checkpoint.Kind == "dispatch" && checkpoint.Status == "pending" {
			rejectCheckpointID = checkpoint.ID
		}
	}
	if rejectCheckpointID == "" {
		t.Fatalf("expected pending dispatch checkpoint on second task, got %#v", snapshot2.Checkpoints)
	}
	rejectReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+rejectCheckpointID+"/decision", strings.NewReader(`{"action":"reject"}`))
	rejectReq.Header.Set("Content-Type", "application/json")
	rejectReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rejectRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusOK {
		t.Fatalf("POST reject decision status = %d: %s", rejectRec.Code, rejectRec.Body.String())
	}
	rejected := decodeProjectControlSnapshot(t, rejectRec)
	for _, task := range rejected.Tasks {
		if task.ID == rejectTaskID && task.DispatchState != "none" {
			t.Fatalf("task dispatchState after reject = %q, want none", task.DispatchState)
		}
	}
	rec = agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-k4b","agentLabel":"hermes"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("claim after rejection = %d, want 204", rec.Code)
	}
}

// A task with a very long title must still produce a dispatch checkpoint
// whose ID passes the decision route's length validation (regression: a
// 155-char ID was rejected with 404 before the human could approve).
func TestDispatchCheckpointIDStaysWithinRouteLimit(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	longTitle := strings.Repeat("很长的任务标题", 30) // ~180 chars of seed text
	// Seed the fixture project + workstream through withStateLocked.
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Projects {
			if state.Projects[i].ID == "project-roambench-control" {
				state.Projects[i].Takeover = projectTakeover{Mode: "active"}
				state.Projects[i].Lifecycle = projectLifecycle{State: "executing"}
			}
		}
		if !projectControlWorkstreamExists(*state, "workstream-ui-ia", "project-roambench-control") {
			state.Workstreams = append(state.Workstreams, projectControlWorkstream{ID: "workstream-ui-ia", ProjectID: "project-roambench-control", Title: "WS"})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks",
		strings.NewReader(`{"projectId":"project-roambench-control","workstreamId":"workstream-ui-ia","title":"`+longTitle+`","origin":"agent","acceptanceCommand":["/bin/true"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task = %d: %s", rec.Code, rec.Body.String())
	}
	snapshot := decodeProjectControlSnapshot(t, rec)

	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.Kind == "dispatch" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
		}
	}
	if checkpointID == "" {
		t.Fatal("no pending dispatch checkpoint")
	}
	if len(checkpointID) > 128 {
		t.Fatalf("checkpoint ID is %d chars, must be <= 128: %s", len(checkpointID), checkpointID)
	}

	// The decision endpoint must reach the handler (400/200, not 404).
	agentToken := agentTokenFor(t, srv, token)
	_ = agentToken
	decision := httptest.NewRequest(http.MethodPost,
		"/api/project-control/checkpoints/"+checkpointID+"/decision",
		strings.NewReader(`{"action":"approve"}`))
	decision.Header.Set("Content-Type", "application/json")
	decision.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decision)
	if decisionRec.Code == http.StatusNotFound {
		t.Fatalf("decision route 404 on a %d-char ID: %s", len(checkpointID), decisionRec.Body.String())
	}
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", decisionRec.Code, decisionRec.Body.String())
	}
}

// A task raised by a human is dispatched automatically: no approval gate, no
// pending checkpoint, and an agent may claim it right away.
func TestHumanRaisedTaskIsDispatchedWithoutApproval(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	agentToken := agentTokenFor(t, srv, token)

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

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks",
		strings.NewReader(`{"projectId":"project-roambench-control","workstreamId":"workstream-ui-ia","title":"人类发起的任务","acceptanceCommand":["/bin/true"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task = %d: %s", rec.Code, rec.Body.String())
	}
	snapshot := decodeProjectControlSnapshot(t, rec)

	taskID := ""
	for _, task := range snapshot.Tasks {
		if task.Title != "人类发起的任务" {
			continue
		}
		taskID = task.ID
		if task.DispatchState != "ready" {
			t.Fatalf("dispatchState = %q, want ready (human-raised tasks are dispatched automatically)", task.DispatchState)
		}
	}
	if taskID == "" {
		t.Fatal("created task not found")
	}
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == taskID && checkpoint.Kind == "dispatch" && checkpoint.Status == "pending" {
			t.Fatal("a human-raised task must not leave a pending dispatch gate")
		}
	}

	// An agent can claim it straight away.
	claim := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-1","agentLabel":"claude"}`)
	if claim.Code != http.StatusOK {
		t.Fatalf("claim = %d, want 200: %s", claim.Code, claim.Body.String())
	}
}
