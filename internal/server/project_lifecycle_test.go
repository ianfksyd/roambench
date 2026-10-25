package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/controlplane"
)

func setupTakeover(t *testing.T) (*Server, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, cookie, sessions := testProjectControlServer(t)
	t.Cleanup(sessions.Stop)
	srv.projectControl.runtimeRootDir = home
	agent := agentTokenFor(t, srv, cookie)
	_, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Projects = append(state.Projects, projectControlProject{ID: "project-i2", Name: "I2", RepoPath: home, RowVersion: 1, Takeover: projectTakeover{Mode: "active"}, Lifecycle: projectLifecycle{State: "executing", MilestoneID: "M1"}, Milestones: []projectMilestone{{ID: "M1", Name: "First", Status: "current"}}})
		state.Tasks = append(state.Tasks, projectControlTask{ID: "task-i2", ProjectID: "project-i2", Title: "Build", MilestoneID: "M1", State: "queued", DispatchState: "ready", Origin: "agent", AcceptanceCommand: []string{"/bin/true"}, RowVersion: 1})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, cookie, agent
}

func i2State(t *testing.T, srv *Server) projectControlState {
	t.Helper()
	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	return state
}
func i2Claim(t *testing.T, srv *Server, agent string) string {
	t.Helper()
	r := agentAPIPost(t, srv, "/api/agent/v1/claim", agent, `{"runtimeId":"hermes","agentLabel":"Hermes"}`)
	if r.Code != 200 {
		t.Fatalf("claim: %d %s", r.Code, r.Body.String())
	}
	var p struct {
		LeaseID string `json:"leaseId"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p.LeaseID
}
func i2Human(t *testing.T, srv *Server, cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	return rec
}

func TestI2PauseScopeAndNotification(t *testing.T) {
	for _, reason := range []string{"task_failed", "acceptance_failed_twice", "heartbeat_lost", "path_violation", "tool_violation", "shared_directory_conflict", "high_risk"} {
		t.Run(reason, func(t *testing.T) {
			srv, _, agent := setupTakeover(t)
			_, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
				state.Tasks = append(state.Tasks, projectControlTask{ID: "dependent", ProjectID: "project-i2", Title: "Depends", DispatchState: "ready", DependsOn: []string{"task-i2"}, AcceptanceCommand: []string{"/bin/true"}}, projectControlTask{ID: "independent", ProjectID: "project-i2", Title: "Independent", DispatchState: "ready", AcceptanceCommand: []string{"/bin/true"}})
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			lease := i2Claim(t, srv, agent)
			notifications := srv.notifHub.subscribe()
			defer srv.notifHub.unsubscribe(notifications)
			_, err = srv.projectControl.controlTakeover("ian", "project-i2", projectTakeoverRequest{Action: "pause", TaskID: "task-i2", Reason: reason}, true)
			if err != nil {
				t.Fatal(err)
			}
			state := i2State(t, srv)
			task, _ := controlTask(state, "task-i2")
			dep, _ := controlTask(state, "dependent")
			ind, _ := controlTask(state, "independent")
			p, _ := controlProject(state, "project-i2")
			if task.PauseReason != reason || dep.PauseReason != reason || task.DispatchState == "claimed" {
				t.Fatalf("scope not paused: %+v %+v", task, dep)
			}
			if pauseProjectScope(reason) != (p.Takeover.Mode == "paused") || pauseProjectScope(reason) != (ind.PauseReason != "") {
				t.Fatalf("incorrect project scope: %+v %+v", p, ind)
			}
			c, err := srv.projectControl.claimStoreFor().GetClaim(context.Background(), "ian", lease)
			if err != nil || c.Status != "released" {
				t.Fatalf("lease survived: %+v %v", c, err)
			}
			r := agentAPIPost(t, srv, "/api/agent/v1/heartbeat", agent, `{"leaseId":"`+lease+`"}`)
			if r.Code == 200 {
				t.Fatal("paused lease renewed")
			}
			select {
			case n := <-notifications:
				if n.Body != reason {
					t.Fatal(n)
				}
			default:
				t.Fatal("missing pause notification")
			}
		})
	}
}

func TestI2ResumeCheckpointOrExplicitRestart(t *testing.T) {
	for _, checkpoint := range []bool{false, true} {
		t.Run(map[bool]string{false: "restart", true: "checkpoint"}[checkpoint], func(t *testing.T) {
			srv, cookie, agent := setupTakeover(t)
			lease := i2Claim(t, srv, agent)
			if checkpoint {
				rec := agentAPIPost(t, srv, "/api/agent/v1/artifact", agent, `{"taskId":"task-i2","leaseId":"`+lease+`","artifactKind":"checkpoint","outcome":"pass","value":"saved execution cursor 42"}`)
				if rec.Code != 200 {
					t.Fatalf("checkpoint: %d %s", rec.Code, rec.Body.String())
				}
			}
			_, err := srv.projectControl.controlTakeover("ian", "project-i2", projectTakeoverRequest{Action: "pause", TaskID: "task-i2", Reason: "task_failed"}, true)
			if err != nil {
				t.Fatal(err)
			}
			// Reopen JSON store to verify that the restart decision uses durable content.
			old := srv.projectControl
			srv.projectControl = newProjectControlStore(filepath.Dir(filepath.Dir(old.rootDir)))
			srv.projectControl.rootDir = old.rootDir
			srv.projectControl.controlPlane = old.controlPlane
			srv.projectControl.runtimeRootDir = old.runtimeRootDir
			body := `{"action":"resume","taskId":"task-i2"}`
			rec := i2Human(t, srv, cookie, "POST", "/api/project-control/projects/project-i2/takeover", body)
			if !checkpoint {
				if rec.Code != 400 {
					t.Fatalf("blind restart allowed: %d %s", rec.Code, rec.Body.String())
				}
				rec = i2Human(t, srv, cookie, "POST", "/api/project-control/projects/project-i2/takeover", `{"action":"resume","taskId":"task-i2","restart":true}`)
			}
			if rec.Code != 200 {
				t.Fatalf("resume: %d %s", rec.Code, rec.Body.String())
			}
			task, _ := controlTask(i2State(t, srv), "task-i2")
			if task.DispatchState != "ready" || task.ClaimedByRuntimeID != "" {
				t.Fatalf("resume bypassed claim: %+v", task)
			}
			if checkpoint != (task.ResumeCheckpointID != "") || checkpoint == task.Restart {
				t.Fatalf("wrong resume semantics: %+v", task)
			}
			newLease := i2Claim(t, srv, agent)
			if newLease == lease {
				t.Fatal("old lease reused")
			}
			rec = agentAPIPost(t, srv, "/api/agent/v1/artifact", agent, `{"taskId":"task-i2","leaseId":"`+lease+`","artifactKind":"checkpoint","outcome":"pass","value":"stale"}`)
			if rec.Code == 200 {
				t.Fatal("stale artifact accepted")
			}
		})
	}
}

func TestI2StopRevokesOnlyProjectAndReleasesSessions(t *testing.T) {
	srv, cookie, agent := setupTakeover(t)
	lease := i2Claim(t, srv, agent)
	_, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Projects = append(state.Projects, projectControlProject{ID: "project-other", Takeover: projectTakeover{Mode: "active"}, Lifecycle: projectLifecycle{State: "executing"}})
		state.Tasks = append(state.Tasks, projectControlTask{ID: "task-other", ProjectID: "project-other", DispatchState: "ready", AcceptanceCommand: []string{"/bin/true"}})
		state.PhaseAttempts = append(state.PhaseAttempts, projectControlPhaseAttempt{ID: "attempt-i2", TaskID: "task-i2", SessionID: "session-i2", Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	otherLease := i2Claim(t, srv, agent)
	rec := i2Human(t, srv, cookie, "POST", "/api/project-control/projects/project-i2/takeover", `{"action":"stop"}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	c, _ := srv.projectControl.claimStoreFor().GetClaim(context.Background(), "ian", lease)
	other, _ := srv.projectControl.claimStoreFor().GetClaim(context.Background(), "ian", otherLease)
	if c.Status != "released" || other.Status != "active" {
		t.Fatalf("wrong revocation scope: %+v %+v", c, other)
	}
	state := i2State(t, srv)
	p, _ := controlProject(state, "project-i2")
	if p.Takeover.Mode != "manual" {
		t.Fatal(p)
	}
	for _, a := range state.PhaseAttempts {
		if a.ID == "attempt-i2" && (!a.Released || projectControlPhaseAttemptSessionState(a) != "released") {
			t.Fatalf("session not released: %+v", a)
		}
	}
	rec = agentAPIPost(t, srv, "/api/agent/v1/pause", agent, `{"projectId":"project-i2","action":"start"}`)
	if rec.Code == 200 {
		t.Fatal("agent escalated authority")
	}
	rec = agentAPIPost(t, srv, "/api/agent/v1/artifact", agent, `{"taskId":"task-i2","leaseId":"`+lease+`","artifactKind":"checkpoint","outcome":"pass","value":"stale"}`)
	if rec.Code == 200 {
		t.Fatal("stopped agent can write")
	}
}

func TestI2AcceptanceAndAutomaticContinuation(t *testing.T) {
	srv, _, agent := setupTakeover(t)
	_, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Tasks = append(state.Tasks, projectControlTask{ID: "next-task", ProjectID: "project-i2", MilestoneID: "M1", DispatchState: "ready", DependsOn: []string{"task-i2"}, AcceptanceCommand: []string{"/bin/true"}}, projectControlTask{ID: "no-command", ProjectID: "project-i2", MilestoneID: "M1", DispatchState: "ready", Origin: "agent"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	lease := i2Claim(t, srv, agent)
	// 任务 i2 有依赖未通过,不可领;no-command 是人发起任务(无 origin),
	// 新契约下无验收命令也可领——这里它还没轮到(i2 先被 release 后才轮到),
	// 但依赖检查仍然挡住 i2 之前的手动领取。断言 other runtime 领不到 i2:
	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agent, `{"runtimeId":"other","agentLabel":"Hermes"}`)
	if rec.Code == 200 {
		var body map[string]interface{}
		_ = json.NewDecoder(rec.Body).Decode(&body)
		if body["taskId"] == "task-i2" {
			t.Fatalf("dependency bypassed: %d %s", rec.Code, rec.Body.String())
		}
	}
	rec = agentAPIPost(t, srv, "/api/agent/v1/release", agent, `{"leaseId":"`+lease+`","outcome":"done"}`)
	if rec.Code != 200 {
		t.Fatalf("release: %d %s", rec.Code, rec.Body.String())
	}
	task, _ := controlTask(i2State(t, srv), "task-i2")
	if !task.AcceptancePassed || task.DispatchState != "done" {
		t.Fatal(task)
	}
	next := i2Claim(t, srv, agent)
	c, _ := srv.projectControl.claimStoreFor().GetClaim(context.Background(), "ian", next)
	if c.TaskID != "next-task" {
		t.Fatal(c)
	}
	noCommand, _ := controlTask(i2State(t, srv), "no-command")
	if noCommand.DispatchState != "ready" {
		t.Fatal(noCommand)
	}
}

// 新契约: 人发起(无 origin)且无验收命令的任务可领——release 直接算验收。
func TestI2NoCommandHumanTaskClaimableAndAccepted(t *testing.T) {
	srv, _, agent := setupTakeover(t)
	_, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		// task-i2 已被 setup 标为 agent 发起;让它暂不可领,保证本次领到目标任务。
		for i := range state.Tasks {
			if state.Tasks[i].ID == "task-i2" {
				state.Tasks[i].DispatchState = "none"
			}
		}
		state.Tasks = append(state.Tasks, projectControlTask{ID: "human-no-command", ProjectID: "project-i2", MilestoneID: "M1", DispatchState: "ready"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	lease := ""
	rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agent, `{"runtimeId":"r","agentLabel":"Hermes"}`)
	if rec.Code == 200 {
		var body map[string]interface{}
		_ = json.NewDecoder(rec.Body).Decode(&body)
		lease, _ = body["leaseId"].(string)
		if body["taskId"] != "human-no-command" {
			t.Fatalf("claimed %v, want human-no-command", body["taskId"])
		}
	} else if rec.Code != 204 {
		t.Fatalf("claim = %d", rec.Code)
	}
	if lease == "" {
		t.Fatal("no-command human task was not claimable")
	}
	rec = agentAPIPost(t, srv, "/api/agent/v1/release", agent, `{"leaseId":"`+lease+`","outcome":"done"}`)
	if rec.Code != 200 {
		t.Fatalf("release without acceptance command = %d %s", rec.Code, rec.Body.String())
	}
	// P0a 契约：无验收命令的完成 = 执行成功，验证未完成。
	task, _ := controlTask(i2State(t, srv), "human-no-command")
	if task.State != "execution_complete" || task.DispatchState != "done" {
		t.Fatalf("no-command task execution incomplete: %+v", task)
	}
	if task.AcceptancePassed || task.AcceptanceStatus != "not_ready" {
		t.Fatalf("no-command task must stay unverified: %+v", task)
	}
}

func TestI2AcceptanceFailureTwiceRequiresHumanRestart(t *testing.T) {
	srv, _, agent := setupTakeover(t)
	_, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID == "task-i2" {
				state.Tasks[i].AcceptanceCommand = []string{"/bin/false"}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		lease := i2Claim(t, srv, agent)
		rec := agentAPIPost(t, srv, "/api/agent/v1/release", agent, `{"leaseId":"`+lease+`","outcome":"done"}`)
		if rec.Code != 200 {
			t.Fatalf("release %d: %s", rec.Code, rec.Body.String())
		}
		task, _ := controlTask(i2State(t, srv), "task-i2")
		if task.AcceptancePassed || task.AcceptanceFailures != attempt || task.PauseReason == "" {
			t.Fatalf("failure not recorded: %+v", task)
		}
		if attempt == 2 && task.PauseReason != "acceptance_failed_twice" {
			t.Fatal(task.PauseReason)
		}
		rec = agentAPIPost(t, srv, "/api/agent/v1/claim", agent, `{"runtimeId":"blind-retry","agentLabel":"Hermes"}`)
		if rec.Code != 204 {
			t.Fatal("blind retry allowed")
		}
		if attempt == 1 {
			if _, err := srv.projectControl.controlTakeover("ian", "project-i2", projectTakeoverRequest{Action: "resume", TaskID: "task-i2", Restart: true}, false); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestI2HeartbeatExpiryClosesExecution(t *testing.T) {
	srv, _, agent := setupTakeover(t)
	now := time.Now().UTC()
	old := projectControlClaimNow
	projectControlClaimNow = func() time.Time { return now }
	defer func() { projectControlClaimNow = old }()
	lease := i2Claim(t, srv, agent)
	now = now.Add(121 * time.Second)
	srv.projectControl.checkHealth("ian", nil)
	task, _ := controlTask(i2State(t, srv), "task-i2")
	if task.PauseReason != "heartbeat_lost" || task.DispatchState == "ready" {
		t.Fatalf("expired task not paused: %+v", task)
	}
	rec := agentAPIPost(t, srv, "/api/agent/v1/heartbeat", agent, `{"leaseId":"`+lease+`"}`)
	if rec.Code != 404 {
		t.Fatal(rec.Code)
	}
}

func TestI2NextAction(t *testing.T) {
	p := projectControlProject{ID: "p", Takeover: projectTakeover{Mode: "active"}, Lifecycle: projectLifecycle{State: "executing"}}
	for _, tc := range []struct {
		name, state, dispatch, pause, actor, contains string
		command, claim                                bool
	}{
		{name: "recon", state: "reconnaissance", actor: "Hermes", contains: "brief"},
		{name: "confirm", state: "awaiting_confirmation", actor: "human", contains: "确认"},
		{name: "ready", state: "executing", dispatch: "ready", command: true, actor: "Hermes", contains: "领取"},
		{name: "missing command", state: "executing", dispatch: "ready", actor: "human", contains: "验收命令"},
		{name: "running", state: "executing", dispatch: "claimed", command: true, claim: true, actor: "Hermes", contains: "继续执行"},
		{name: "paused", state: "executing", pause: "task_failed", actor: "human", contains: "重跑"},
		{name: "milestone", state: "awaiting_milestone_acceptance", actor: "human", contains: "里程碑"},
		{name: "done", state: "completed", actor: "human", contains: "已完成"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.Lifecycle.State = tc.state
			task := projectControlTask{ID: "t", Title: "Build", ProjectID: "p", DispatchState: tc.dispatch, PauseReason: tc.pause}
			if tc.command {
				task.AcceptanceCommand = []string{"true"}
			}
			var claims []controlplane.TaskClaim
			if tc.claim {
				claims = append(claims, controlplane.TaskClaim{TaskID: "t", Status: "active", LeaseExpiresAt: time.Now().Add(time.Minute)})
			}
			got := deriveNextAction(p, []projectControlTask{task}, claims)
			if got.Actor != tc.actor || !strings.Contains(got.Text, tc.contains) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestI2ContractLifecycleAndDetailHTTP(t *testing.T) {
	srv, cookie, _ := setupTakeover(t)
	home := srv.projectControl.runtimeRootDir
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = home
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init")
	if err := os.WriteFile(filepath.Join(home, "tracked.txt"), []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "tracked.txt")
	git("-c", "user.name=fixture", "-c", "user.email=fixture@local", "commit", "-m", "base")
	base := git("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(home, "tracked.txt"), []byte("new\nextra\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "untracked.txt"), []byte("new file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rec := i2Human(t, srv, cookie, "POST", "/api/project-control/projects", `{"name":"Lifecycle","brief":{"understanding":"goal","deliverables":"ship","outOfScope":"no cloud","assumptions":"local"},"milestones":[{"id":"M1","name":"Deliver","acceptanceEvidence":"test log","status":"current"}]}`)
	if rec.Code != 201 {
		t.Fatal(rec.Body.String())
	}
	snap := decodeProjectControlSnapshot(t, rec)
	p := snap.Projects[len(snap.Projects)-1]
	for _, next := range []string{"reconnaissance", "planning", "awaiting_confirmation", "executing", "awaiting_milestone_acceptance"} {
		body, _ := json.Marshal(projectContractRequest{ExpectedRowVersion: p.RowVersion, Lifecycle: next})
		rec = i2Human(t, srv, cookie, "PATCH", "/api/project-control/projects/"+p.ID+"/contract", string(body))
		if rec.Code != 200 {
			t.Fatalf("transition %s: %d %s", next, rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
	}
	ms := []projectMilestone{{ID: "M1", Name: "Deliver", AcceptanceEvidence: "test log", Status: "done"}}
	body, _ := json.Marshal(projectContractRequest{ExpectedRowVersion: p.RowVersion, Lifecycle: "completed", Milestones: &ms})
	rec = i2Human(t, srv, cookie, "PATCH", "/api/project-control/projects/"+p.ID+"/contract", string(body))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	mutateI2(t, srv, func(state *projectControlState) {
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{ID: "detail-decision", ProjectID: p.ID, Action: "checkpoint_resolved", SourceID: "detail-checkpoint", Kind: "design", Actor: "human"})
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{ID: "detail-artifact", ProjectID: p.ID, Action: "artifact_recorded", SourceID: "detail-patch", Kind: "coding", Actor: "agent"})
	})
	rec = i2Human(t, srv, cookie, "GET", "/api/project-control/projects/"+p.ID+"/detail", "")
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var d projectDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Brief.Understanding != "goal" || d.Lifecycle.State != "completed" || d.Milestones[0].Status != "done" || d.Issues == nil || !reflect.DeepEqual(d.Stats.CheckpointsByKind, map[string]int{"design": 1, "test": 0, "coding": 0, "unclassified": 0}) {
		t.Fatalf("bad detail: %+v", d)
	}
	if !reflect.DeepEqual(d.Stats.ArtifactsByKind, map[string]int{"design": 0, "test": 0, "coding": 1, "unclassified": 0}) {
		t.Fatalf("wrong HTTP artifacts: %+v", d.Stats.ArtifactsByKind)
	}
	changes := d.Stats.CodeChanges
	if changes.Error != "" || changes.Base != base || changes.Added != 2 || changes.Removed != 1 || changes.Files != 1 || changes.Untracked != 1 || changes.Binary != 0 {
		t.Fatalf("wrong HTTP codeChanges: %+v", changes)
	}
	req := httptest.NewRequest("GET", "/api/project-control/projects/"+p.ID+"/detail", nil)
	unauth := httptest.NewRecorder()
	srv.mux.ServeHTTP(unauth, req)
	if unauth.Code == 200 {
		t.Fatal("unauthenticated detail")
	}
	// Verify the actual JSON state, not just an in-memory snapshot.
	payload, err := os.ReadFile(srv.projectControl.pathFor("ian"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "no cloud") {
		t.Fatal("brief not persisted")
	}
}

func TestI2ActivityCountsDistinctSourcesAndPersists(t *testing.T) {
	srv, _, _ := setupTakeover(t)
	_, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		task, _ := controlTask(*state, "task-i2")
		attempt := projectControlPhaseAttempt{}
		for _, kind := range []string{"plan", "test_result", "diff_summary", "checkpoint"} {
			recordProjectControlPhaseArtifact(state, &task, &attempt, "plan", kind, "pass", "", "saved", "agent", time.Now().UTC().Format(time.RFC3339))
		}
		event := projectControlRecordedEvent{ID: "event-one", ProjectID: "project-i2", TaskID: "task-i2", Action: "checkpoint_resolved", SourceID: "checkpoint-one", Kind: "design"}
		projectControlAppendRecordedEvent(state, event)
		event.ID = "event-replay"
		projectControlAppendRecordedEvent(state, event)
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{ID: "audit-copy", ProjectID: "project-i2", Action: "decision_made", SourceID: "checkpoint-one", Kind: "design"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	state := i2State(t, srv)
	p, _ := controlProject(state, "project-i2")
	d := aggregateProjectDetail(state, p, nil)
	counts := d.Stats.CheckpointsByKind
	if !reflect.DeepEqual(counts, map[string]int{"design": 1, "test": 0, "coding": 0, "unclassified": 0}) {
		t.Fatal(counts)
	}
	artifacts := d.Stats.ArtifactsByKind
	if !reflect.DeepEqual(artifacts, map[string]int{"design": 1, "test": 1, "coding": 1, "unclassified": 1}) {
		t.Fatal(artifacts)
	}
}

func TestI2ClaimDetectsHighRiskAndSharedDirectoryConflict(t *testing.T) {
	for _, reason := range []string{"high_risk", "shared_directory_conflict"} {
		t.Run(reason, func(t *testing.T) {
			srv, _, agent := setupTakeover(t)
			_, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
				for i := range state.Tasks {
					if state.Tasks[i].ID == "task-i2" {
						if reason == "high_risk" {
							state.Tasks[i].RiskLevel = "high"
						} else {
							state.Tasks[i].CurrentPhase = "implement"
						}
					}
				}
				if reason != "high_risk" {
					state.Tasks = append(state.Tasks, projectControlTask{ID: "conflict", ProjectID: "project-i2", MilestoneID: "M1", CurrentPhase: "implement", DispatchState: "ready", AcceptanceCommand: []string{"true"}})
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if reason != "high_risk" {
				i2Claim(t, srv, agent)
			}
			rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agent, `{"runtimeId":"next","agentLabel":"Hermes"}`)
			if rec.Code != 204 {
				t.Fatal(rec.Code, rec.Body.String())
			}
			p, _ := controlProject(i2State(t, srv), "project-i2")
			if p.Takeover.Mode != "paused" || p.Lifecycle.Reason != reason {
				t.Fatal(p)
			}
		})
	}
}
