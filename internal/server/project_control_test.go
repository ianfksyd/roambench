package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/config"
	"github.com/ianf339/roambench/internal/terminal"
)

func testProjectControlServer(t *testing.T) (*Server, string, *auth.SessionManager) {
	t.Helper()
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	return NewServer(cfg, nil, sessions, nil, nil), token, sessions
}

func decodeProjectControlSnapshot(t *testing.T, rec *httptest.ResponseRecorder) projectControlSnapshot {
	t.Helper()
	var snapshot projectControlSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&snapshot); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	return snapshot
}

func patchProjectControlTask(t *testing.T, srv *Server, token, taskID, body string) projectControlSnapshot {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+taskID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH task status = %d, want %d for %s: %s", rec.Code, http.StatusOK, body, rec.Body.String())
	}
	return decodeProjectControlSnapshot(t, rec)
}

func completeProjectControlTaskRunbook(t *testing.T, srv *Server, token, taskID string, rowVersion int) (projectControlSnapshot, int) {
	t.Helper()
	steps := []string{
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"plan"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"plan","artifactKind":"plan","artifactOutcome":"recorded","artifactLabel":"Plan","artifactValue":"Implementation plan recorded"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"implement"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"implement","artifactKind":"diff_summary","artifactOutcome":"recorded","artifactLabel":"Diff summary","artifactValue":"Implementation diff recorded"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"test"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"test","artifactKind":"test_result","artifactOutcome":"pass","artifactLabel":"Test result","artifactValue":"go test ./... passed"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"review"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"review","artifactKind":"review_result","artifactOutcome":"pass","artifactLabel":"Review result","artifactValue":"No blocking review findings"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"final_validation"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"final_validation","artifactKind":"completion_check","artifactOutcome":"pass","artifactLabel":"Completion check","artifactValue":"Completion rules satisfied"}`,
	}
	var snapshot projectControlSnapshot
	for _, step := range steps {
		snapshot = patchProjectControlTask(t, srv, token, taskID, fmt.Sprintf(step, rowVersion))
		rowVersion++
	}
	return snapshot, rowVersion
}

func completeProjectControlTaskToAcceptanceReview(t *testing.T, srv *Server, token, taskID string, rowVersion int) (projectControlSnapshot, int) {
	t.Helper()
	snapshot, rowVersion := completeProjectControlTaskRunbook(t, srv, token, taskID, rowVersion)
	if !projectControlSnapshotHasTaskAcceptance(snapshot, taskID, "ready_for_acceptance") {
		t.Fatalf("task %q did not reach ready_for_acceptance", taskID)
	}
	snapshot = patchProjectControlTask(t, srv, token, taskID, fmt.Sprintf(`{"expectedRowVersion":%d,"action":"request_acceptance_review"}`, rowVersion))
	rowVersion++
	return snapshot, rowVersion
}

func projectControlSnapshotHasTaskAcceptance(snapshot projectControlSnapshot, taskID, acceptanceStatus string) bool {
	for _, task := range snapshot.Tasks {
		if task.ID == taskID && task.AcceptanceStatus == acceptanceStatus {
			return true
		}
	}
	return false
}

func TestProjectControlDashboardCountsRunningWorkstreamsFromWorkstreamStatus(t *testing.T) {
	dashboard := buildProjectControlDashboard(
		[]projectControlWorkstream{
			{ID: "workstream-running", Status: "running"},
			{ID: "workstream-planned", Status: "planned"},
		},
		[]projectControlTask{
			{ID: "task-running", WorkstreamID: "workstream-planned", State: "running"},
		},
		nil,
		nil,
		nil,
	)

	if dashboard.RunningWorkstreams != 1 {
		t.Fatalf("RunningWorkstreams = %d, want 1 from workstream status only", dashboard.RunningWorkstreams)
	}
	if dashboard.RunningTasks != 1 {
		t.Fatalf("RunningTasks = %d, want 1", dashboard.RunningTasks)
	}
}

func TestProjectControlDefaultRunbookDefinesCodeChangePhases(t *testing.T) {
	skills := defaultProjectControlSkills()
	if len(skills) != 1 {
		t.Fatalf("len(skills) = %d, want 1", len(skills))
	}
	skill := skills[0]
	if skill.ID != projectControlDefaultSkillID {
		t.Fatalf("skill ID = %q, want %q", skill.ID, projectControlDefaultSkillID)
	}
	if skill.DefaultRunbookID != projectControlDefaultRunbookID {
		t.Fatalf("DefaultRunbookID = %q, want %q", skill.DefaultRunbookID, projectControlDefaultRunbookID)
	}
	if skill.PermissionsByPhase["implement"] != "scoped_write" {
		t.Fatalf("implement permission = %q, want scoped_write", skill.PermissionsByPhase["implement"])
	}

	runbook := defaultProjectControlRunbook()
	if runbook.ID != projectControlDefaultRunbookID {
		t.Fatalf("runbook ID = %q, want %q", runbook.ID, projectControlDefaultRunbookID)
	}
	wantPhases := []string{"plan", "implement", "test", "review", "fix_or_replan", "final_validation"}
	if len(runbook.Phases) != len(wantPhases) {
		t.Fatalf("len(runbook.Phases) = %d, want %d", len(runbook.Phases), len(wantPhases))
	}
	for i, want := range wantPhases {
		if runbook.Phases[i].ID != want {
			t.Fatalf("phase[%d] = %q, want %q", i, runbook.Phases[i].ID, want)
		}
	}
	if next := nextProjectControlRunbookPhaseID(runbook, "review"); next != "final_validation" {
		t.Fatalf("next phase after review = %q, want final_validation", next)
	}
	if next := nextProjectControlRunbookPhaseID(runbook, "fix_or_replan"); next != "test" {
		t.Fatalf("next phase after fix_or_replan = %q, want test", next)
	}
	if next := nextProjectControlRunbookPhaseID(runbook, "final_validation"); next != "ready_for_acceptance" {
		t.Fatalf("next phase after final_validation = %q, want ready_for_acceptance", next)
	}
	if next := nextProjectControlRunbookPhaseID(runbook, "missing"); next != "" {
		t.Fatalf("next phase after missing = %q, want empty", next)
	}
	if next := nextProjectControlRunbookPhaseID(runbook, "ready_for_acceptance"); next != "" {
		t.Fatalf("next phase after ready_for_acceptance = %q, want empty", next)
	}
}

func TestProjectControlRunbookPhaseProgressionUsesRunbookOrder(t *testing.T) {
	runbook := projectControlRunbook{
		ID:    "docs_update_default",
		Skill: "docs_update",
		Phases: []projectControlRunbookPhase{
			{ID: "plan"},
			{ID: "review"},
			{ID: "final_validation"},
		},
	}

	if next := nextProjectControlRunbookPhaseID(runbook, "plan"); next != "review" {
		t.Fatalf("next phase after plan = %q, want review", next)
	}
	if next := nextProjectControlRunbookPhaseID(runbook, "review"); next != "final_validation" {
		t.Fatalf("next phase after review = %q, want final_validation", next)
	}
	if next := nextProjectControlRunbookPhaseID(runbook, "final_validation"); next != "ready_for_acceptance" {
		t.Fatalf("next phase after final_validation = %q, want ready_for_acceptance", next)
	}
	if next := nextProjectControlRunbookPhaseID(runbook, "fix_or_replan"); next != "test" {
		t.Fatalf("next phase after fix_or_replan = %q, want test override", next)
	}
}

func TestProjectControlRunbookPhaseProgressionSkipsRecoveryPhaseOnSuccess(t *testing.T) {
	runbook := projectControlRunbook{
		ID:    "code_change_custom",
		Skill: "code_change",
		Phases: []projectControlRunbookPhase{
			{ID: "plan"},
			{ID: "implement"},
			{ID: "review"},
			{ID: "fix_or_replan"},
			{ID: "final_validation"},
		},
	}

	if next := nextProjectControlRunbookPhaseID(runbook, "review"); next != "final_validation" {
		t.Fatalf("next phase after successful review = %q, want final_validation", next)
	}
}

func TestProjectControlSkillRunbookSelectionValidatesRegistry(t *testing.T) {
	skill, runbook, err := validateProjectControlSkillRunbookSelection("code_change", "code_change_default")
	if err != nil {
		t.Fatalf("validate explicit default error: %v", err)
	}
	if skill.ID != projectControlDefaultSkillID || runbook.ID != projectControlDefaultRunbookID {
		t.Fatalf("selection = %q/%q, want %q/%q", skill.ID, runbook.ID, projectControlDefaultSkillID, projectControlDefaultRunbookID)
	}

	_, _, err = validateProjectControlSkillRunbookSelection("unknown", "")
	if err == nil || !strings.Contains(err.Error(), "unknown skill") {
		t.Fatalf("unknown skill error = %v, want unknown skill", err)
	}

	_, _, err = validateProjectControlSkillRunbookSelection("code_change", "missing_runbook")
	if err == nil || !strings.Contains(err.Error(), "unknown runbook") {
		t.Fatalf("unknown runbook error = %v, want unknown runbook", err)
	}
}

func TestProjectControlRunbookRegistryFallsBackToDefaultSkill(t *testing.T) {
	task := projectControlTask{
		ID:               "task-registry-fallback",
		State:            "planned",
		AcceptanceStatus: "not_ready",
		SelectedSkill:    "unknown_skill",
		RunbookID:        "unknown_runbook",
	}

	refreshProjectControlTaskRunbookFields(&task, nil)

	if task.SelectedSkill != projectControlDefaultSkillID {
		t.Fatalf("SelectedSkill = %q, want %q", task.SelectedSkill, projectControlDefaultSkillID)
	}
	if task.RunbookID != projectControlDefaultRunbookID {
		t.Fatalf("RunbookID = %q, want %q", task.RunbookID, projectControlDefaultRunbookID)
	}
	if task.CurrentPhase != "plan" {
		t.Fatalf("CurrentPhase = %q, want plan", task.CurrentPhase)
	}
	if got := strings.Join(task.MissingEvidence, ","); got != "plan,diff_summary,test_result:pass,review_result:pass,completion_check:pass" {
		t.Fatalf("MissingEvidence = %q, want default code_change requirements", got)
	}
}

func TestProjectControlRunbookPhaseLifecycleRecordsArtifacts(t *testing.T) {
	state := projectControlState{
		PhaseAttempts: []projectControlPhaseAttempt{},
		Artifacts:     []projectControlArtifact{},
	}
	task := projectControlTask{
		ID:               "task-runbook-lifecycle",
		ProjectID:        "project-runbook",
		WorkstreamID:     "workstream-runbook",
		State:            "planned",
		AcceptanceStatus: "not_ready",
		RuntimeID:        projectControlRuntimeID,
	}
	projectControlNormalizeState(&state)
	now := "2026-04-15T00:00:00Z"

	steps := []struct {
		phase   string
		kind    string
		outcome string
		value   string
		next    string
	}{
		{phase: "plan", kind: "plan", outcome: "recorded", value: "Plan artifact", next: "implement"},
		{phase: "implement", kind: "diff_summary", outcome: "recorded", value: "Changed project control runbook code", next: "test"},
		{phase: "test", kind: "test_result", outcome: "pass", value: "go test ./... passed", next: "review"},
		{phase: "review", kind: "review_result", outcome: "pass", value: "No high severity objections", next: "final_validation"},
	}
	for _, step := range steps {
		if err := startProjectControlTaskPhase(&state, &task, step.phase, now); err != nil {
			t.Fatalf("start %s error: %v", step.phase, err)
		}
		req := projectControlTaskUpdateRequest{
			PhaseID:         step.phase,
			ArtifactKind:    step.kind,
			ArtifactOutcome: step.outcome,
			ArtifactLabel:   step.kind,
			ArtifactValue:   step.value,
		}
		if err := completeProjectControlTaskPhase(&state, &task, req, now); err != nil {
			t.Fatalf("complete %s error: %v", step.phase, err)
		}
		if task.CurrentPhase != step.next {
			t.Fatalf("after %s CurrentPhase = %q, want %q", step.phase, task.CurrentPhase, step.next)
		}
	}

	if len(state.Artifacts) != 4 {
		t.Fatalf("len(Artifacts) = %d, want 4", len(state.Artifacts))
	}
	if task.DiffSummary != "Changed project control runbook code" {
		t.Fatalf("DiffSummary = %q, want implement artifact value", task.DiffSummary)
	}
	if err := startProjectControlTaskPhase(&state, &task, "final_validation", now); err != nil {
		t.Fatalf("start final_validation error: %v", err)
	}
	req := projectControlTaskUpdateRequest{
		PhaseID:         "final_validation",
		ArtifactKind:    "completion_check",
		ArtifactOutcome: "pass",
		ArtifactLabel:   "completion_check",
		ArtifactValue:   "All completion rules satisfied",
	}
	if err := completeProjectControlTaskPhase(&state, &task, req, now); err != nil {
		t.Fatalf("complete final_validation error: %v", err)
	}
	if task.State != "execution_complete" {
		t.Fatalf("State = %q, want execution_complete", task.State)
	}
	if task.AcceptanceStatus != "ready_for_acceptance" {
		t.Fatalf("AcceptanceStatus = %q, want ready_for_acceptance", task.AcceptanceStatus)
	}
	if task.CurrentPhase != "ready_for_acceptance" {
		t.Fatalf("CurrentPhase = %q, want ready_for_acceptance", task.CurrentPhase)
	}
	if len(task.MissingEvidence) != 0 {
		t.Fatalf("MissingEvidence = %#v, want empty", task.MissingEvidence)
	}
}

func TestProjectControlFinalValidationRequiresCompletionEvidence(t *testing.T) {
	state := projectControlState{
		PhaseAttempts: []projectControlPhaseAttempt{},
		Artifacts:     []projectControlArtifact{},
	}
	task := projectControlTask{
		ID:               "task-runbook-missing-evidence",
		ProjectID:        "project-runbook",
		WorkstreamID:     "workstream-runbook",
		State:            "running",
		AcceptanceStatus: "not_ready",
		RuntimeID:        projectControlRuntimeID,
		SelectedSkill:    projectControlDefaultSkillID,
		RunbookID:        projectControlDefaultRunbookID,
		CurrentPhase:     "final_validation",
		RunbookState:     "in_progress",
	}
	now := "2026-04-15T00:00:00Z"
	if err := startProjectControlTaskPhase(&state, &task, "final_validation", now); err != nil {
		t.Fatalf("start final_validation error: %v", err)
	}
	req := projectControlTaskUpdateRequest{
		PhaseID:         "final_validation",
		ArtifactKind:    "completion_check",
		ArtifactOutcome: "pass",
		ArtifactLabel:   "completion_check",
		ArtifactValue:   "Completion claimed without prior evidence",
	}
	err := completeProjectControlTaskPhase(&state, &task, req, now)
	if err == nil {
		t.Fatal("complete final_validation error = nil, want missing evidence error")
	}
	if !strings.Contains(err.Error(), "missing plan") {
		t.Fatalf("error = %q, want missing plan evidence", err.Error())
	}
	if task.AcceptanceStatus == "ready_for_acceptance" {
		t.Fatalf("AcceptanceStatus = %q, should not be ready_for_acceptance", task.AcceptanceStatus)
	}
	if len(state.Artifacts) != 0 {
		t.Fatalf("len(Artifacts) = %d, want 0 after rejected completion", len(state.Artifacts))
	}
}

func TestProjectControlCompletionEvidenceRequiresPassingOutcomes(t *testing.T) {
	state := projectControlState{
		PhaseAttempts: []projectControlPhaseAttempt{},
		Artifacts: []projectControlArtifact{
			{ID: "artifact-plan", TaskID: "task-runbook-failed-test", Kind: "plan", Outcome: "recorded"},
			{ID: "artifact-diff", TaskID: "task-runbook-failed-test", Kind: "diff_summary", Outcome: "recorded"},
			{ID: "artifact-test", TaskID: "task-runbook-failed-test", Kind: "test_result", Outcome: "fail"},
			{ID: "artifact-review", TaskID: "task-runbook-failed-test", Kind: "review_result", Outcome: "pass"},
		},
	}
	task := projectControlTask{
		ID:               "task-runbook-failed-test",
		ProjectID:        "project-runbook",
		WorkstreamID:     "workstream-runbook",
		State:            "running",
		AcceptanceStatus: "not_ready",
		RuntimeID:        projectControlRuntimeID,
		SelectedSkill:    projectControlDefaultSkillID,
		RunbookID:        projectControlDefaultRunbookID,
		CurrentPhase:     "final_validation",
		RunbookState:     "in_progress",
	}
	now := "2026-04-15T00:00:00Z"
	if err := startProjectControlTaskPhase(&state, &task, "final_validation", now); err != nil {
		t.Fatalf("start final_validation error: %v", err)
	}
	req := projectControlTaskUpdateRequest{
		PhaseID:         "final_validation",
		ArtifactKind:    "completion_check",
		ArtifactOutcome: "pass",
		ArtifactLabel:   "completion_check",
		ArtifactValue:   "Completion claimed with failed test evidence",
	}
	err := completeProjectControlTaskPhase(&state, &task, req, now)
	if err == nil {
		t.Fatal("complete final_validation error = nil, want failed test evidence error")
	}
	if !strings.Contains(err.Error(), "test_result:pass") {
		t.Fatalf("error = %q, want missing passing test evidence", err.Error())
	}
	if len(state.Artifacts) != 4 {
		t.Fatalf("len(Artifacts) = %d, want original artifacts only after rejected completion", len(state.Artifacts))
	}
}

func TestProjectControlFinalAcceptanceApprovalRequiresCompletionEvidence(t *testing.T) {
	state := projectControlState{
		Tasks: []projectControlTask{{
			ID:               "task-acceptance-without-evidence",
			State:            "execution_complete",
			AcceptanceStatus: "under_human_review",
		}},
		Artifacts: []projectControlArtifact{},
	}
	checkpoint := projectControlCheckpoint{
		ID:     "checkpoint-final-acceptance",
		TaskID: "task-acceptance-without-evidence",
		Kind:   "final_acceptance",
		Status: "pending",
	}
	err := validateProjectControlFinalAcceptanceApproval(&state, checkpoint)
	if err == nil {
		t.Fatal("final acceptance approval error = nil, want missing evidence error")
	}
	if !strings.Contains(err.Error(), "missing plan") {
		t.Fatalf("error = %q, want missing plan evidence", err.Error())
	}
}

func TestProjectControlFinalAcceptanceApprovalRequiresExecutionComplete(t *testing.T) {
	state := projectControlState{
		Tasks: []projectControlTask{{
			ID:               "task-acceptance-running",
			State:            "running",
			AcceptanceStatus: "under_human_review",
		}},
		Artifacts: []projectControlArtifact{
			{ID: "artifact-plan", TaskID: "task-acceptance-running", Kind: "plan", Outcome: "recorded"},
			{ID: "artifact-diff", TaskID: "task-acceptance-running", Kind: "diff_summary", Outcome: "recorded"},
			{ID: "artifact-test", TaskID: "task-acceptance-running", Kind: "test_result", Outcome: "pass"},
			{ID: "artifact-review", TaskID: "task-acceptance-running", Kind: "review_result", Outcome: "pass"},
			{ID: "artifact-completion", TaskID: "task-acceptance-running", Kind: "completion_check", Outcome: "pass"},
		},
	}
	checkpoint := projectControlCheckpoint{
		ID:     "checkpoint-final-acceptance",
		TaskID: "task-acceptance-running",
		Kind:   "final_acceptance",
		Status: "pending",
	}
	err := validateProjectControlFinalAcceptanceApproval(&state, checkpoint)
	if err == nil {
		t.Fatal("final acceptance approval error = nil, want execution_complete error")
	}
	if !strings.Contains(err.Error(), "requires execution_complete") {
		t.Fatalf("error = %q, want execution_complete guard", err.Error())
	}
}

func TestProjectControlSnapshotRouteReturnsSeededPrototype(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/project-control status = %d, want %d", rec.Code, http.StatusOK)
	}

	snapshot := decodeProjectControlSnapshot(t, rec)
	if snapshot.ActiveProjectID != projectControlProjectID {
		t.Fatalf("ActiveProjectID = %q, want %q", snapshot.ActiveProjectID, projectControlProjectID)
	}
	if len(snapshot.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(snapshot.Projects))
	}
	if len(snapshot.Workstreams) < 2 {
		t.Fatalf("len(Workstreams) = %d, want at least 2", len(snapshot.Workstreams))
	}
	if len(snapshot.Tasks) < 4 {
		t.Fatalf("len(Tasks) = %d, want at least 4", len(snapshot.Tasks))
	}
	if snapshot.ApprovalsCount != 1 {
		t.Fatalf("ApprovalsCount = %d, want 1", snapshot.ApprovalsCount)
	}
	if len(snapshot.Checkpoints) != 1 || snapshot.Checkpoints[0].Status != "pending" {
		t.Fatalf("Checkpoints = %#v, want one pending checkpoint", snapshot.Checkpoints)
	}
	if len(snapshot.Runbooks) != 1 || snapshot.Runbooks[0].ID != projectControlDefaultRunbookID {
		t.Fatalf("Runbooks = %#v, want default runbook", snapshot.Runbooks)
	}
	if len(snapshot.Skills) != 1 || snapshot.Skills[0].ID != projectControlDefaultSkillID || snapshot.Skills[0].DefaultRunbookID != projectControlDefaultRunbookID {
		t.Fatalf("Skills = %#v, want default code_change skill", snapshot.Skills)
	}
	for _, task := range snapshot.Tasks {
		if task.SelectedSkill == "" || task.RunbookID == "" || task.CurrentPhase == "" {
			t.Fatalf("task %q missing skill/runbook fields: %#v", task.ID, task)
		}
		if task.ID == projectControlTaskIAID {
			if task.AcceptanceStatus != "under_human_review" {
				t.Fatalf("seed IA acceptanceStatus = %q, want under_human_review", task.AcceptanceStatus)
			}
			if len(task.MissingEvidence) != 0 {
				t.Fatalf("seed IA MissingEvidence = %#v, want empty", task.MissingEvidence)
			}
		}
	}
}

func TestProjectControlTaskPhaseActionsPersistThroughAPI(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	startReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"start_phase","phaseId":"plan"}`))
	startReq.Header.Set("Content-Type", "application/json")
	startReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	startRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("PATCH start_phase status = %d, want %d: %s", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	startSnapshot := decodeProjectControlSnapshot(t, startRec)
	if len(startSnapshot.PhaseAttempts) != 1 || startSnapshot.PhaseAttempts[0].Status != "running" {
		t.Fatalf("PhaseAttempts after start = %#v, want one running attempt", startSnapshot.PhaseAttempts)
	}

	completeReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":2,"action":"complete_phase","phaseId":"plan","artifactKind":"plan","artifactLabel":"Plan","artifactValue":"Implementation plan recorded"}`))
	completeReq.Header.Set("Content-Type", "application/json")
	completeReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	completeRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("PATCH complete_phase status = %d, want %d: %s", completeRec.Code, http.StatusOK, completeRec.Body.String())
	}
	completeSnapshot := decodeProjectControlSnapshot(t, completeRec)
	foundPlanArtifact := false
	for _, artifact := range completeSnapshot.Artifacts {
		if artifact.TaskID == projectControlTaskPanelID && artifact.Kind == "plan" && artifact.Outcome == "recorded" {
			foundPlanArtifact = true
			break
		}
	}
	if !foundPlanArtifact {
		t.Fatalf("Artifacts after complete = %#v, want recorded plan artifact for task", completeSnapshot.Artifacts)
	}
	foundTask := false
	for _, task := range completeSnapshot.Tasks {
		if task.ID == projectControlTaskPanelID {
			foundTask = true
			if task.CurrentPhase != "implement" {
				t.Fatalf("CurrentPhase = %q, want implement", task.CurrentPhase)
			}
			if task.RunbookState != "in_progress" {
				t.Fatalf("RunbookState = %q, want in_progress", task.RunbookState)
			}
			break
		}
	}
	if !foundTask {
		t.Fatalf("task %q not found", projectControlTaskPanelID)
	}
}

func TestProjectControlPhaseActionsRejectDirectStateOverrides(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"start_phase","phaseId":"plan","state":"execution_complete","acceptanceStatus":"ready_for_acceptance"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH phase action with direct state status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "phase actions cannot include direct state") {
		t.Fatalf("PATCH phase action error = %q, want direct state rejection", rec.Body.String())
	}
}

func TestProjectControlCheckpointDecisionApproveUpdatesSnapshot(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	checkpointID := ""
	readySnapshot, _ := completeProjectControlTaskToAcceptanceReview(t, srv, token, projectControlTaskPanelID, 1)
	for _, checkpoint := range readySnapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending final_acceptance checkpoint for task")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", rec.Code, http.StatusOK)
	}

	snapshot := decodeProjectControlSnapshot(t, rec)
	if snapshot.ApprovalsCount != 1 {
		t.Fatalf("ApprovalsCount = %d, want 1 because the seeded IA checkpoint remains pending", snapshot.ApprovalsCount)
	}
	foundApprovedCheckpoint := false
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.ID == checkpointID && checkpoint.Status == "approved" {
			foundApprovedCheckpoint = true
		}
	}
	if !foundApprovedCheckpoint {
		t.Fatalf("Checkpoints = %#v, want approved checkpoint %q", snapshot.Checkpoints, checkpointID)
	}

	var acceptanceStatus string
	for _, task := range snapshot.Tasks {
		if task.ID == projectControlTaskPanelID {
			acceptanceStatus = task.AcceptanceStatus
			break
		}
	}
	if acceptanceStatus != "accepted" {
		t.Fatalf("task acceptance status = %q, want accepted", acceptanceStatus)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/project-control/events?checkpointId="+checkpointID+"&limit=10", nil)
	eventsReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	eventsRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("GET checkpoint events status = %d, want %d", eventsRec.Code, http.StatusOK)
	}
	var payload struct {
		Events []projectControlRecordedEvent `json:"events"`
	}
	if err := json.NewDecoder(eventsRec.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode events error: %v", err)
	}
	foundFinalAcceptance := false
	for _, event := range payload.Events {
		if event.Action == "final_acceptance_approved" {
			foundFinalAcceptance = true
		}
	}
	if !foundFinalAcceptance {
		t.Fatalf("expected final_acceptance_approved event, got %#v", payload.Events)
	}
}

func TestProjectControlCheckpointDecisionRejectsUnknownAction(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+projectControlCheckpointAcceptanceID+"/decision", strings.NewReader(`{"action":"ship-it"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST checkpoint decision status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestProjectControlDashboardTimelineRecordsCreateEvents(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	projectReq := httptest.NewRequest(http.MethodPost, "/api/project-control/projects", strings.NewReader(`{"key":"demo","name":"Demo Project","description":"persistent demo","currentGoal":"ship prototype"}`))
	projectReq.Header.Set("Content-Type", "application/json")
	projectReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	projectRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(projectRec, projectReq)
	if projectRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/projects status = %d, want %d", projectRec.Code, http.StatusCreated)
	}
	projectSnapshot := decodeProjectControlSnapshot(t, projectRec)
	createdProject := projectSnapshot.Projects[len(projectSnapshot.Projects)-1]

	workstreamBody := fmt.Sprintf(`{"projectId":%q,"title":"Stream A","description":"new stream","priority":"high","scopeSummary":"deliver ui"}`, createdProject.ID)
	workstreamReq := httptest.NewRequest(http.MethodPost, "/api/project-control/workstreams", strings.NewReader(workstreamBody))
	workstreamReq.Header.Set("Content-Type", "application/json")
	workstreamReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	workstreamRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(workstreamRec, workstreamReq)
	if workstreamRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/workstreams status = %d, want %d", workstreamRec.Code, http.StatusCreated)
	}
	workstreamSnapshot := decodeProjectControlSnapshot(t, workstreamRec)
	createdWorkstream := workstreamSnapshot.Workstreams[len(workstreamSnapshot.Workstreams)-1]

	taskBody := fmt.Sprintf(`{"projectId":%q,"workstreamId":%q,"title":"Implement persisted panel","goal":"replace seeded-only state","priority":"high","riskLevel":"medium"}`,
		createdProject.ID, createdWorkstream.ID)
	taskReq := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks", strings.NewReader(taskBody))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/tasks status = %d, want %d", taskRec.Code, http.StatusCreated)
	}
	taskSnapshot := decodeProjectControlSnapshot(t, taskRec)
	foundTask := false
	for _, task := range taskSnapshot.Tasks {
		if task.Title != "Implement persisted panel" {
			continue
		}
		foundTask = true
		if task.SelectedSkill != projectControlDefaultSkillID || task.RunbookID != projectControlDefaultRunbookID {
			t.Fatalf("created task skill/runbook = %q/%q, want %q/%q", task.SelectedSkill, task.RunbookID, projectControlDefaultSkillID, projectControlDefaultRunbookID)
		}
	}
	if !foundTask {
		t.Fatal("created task not found in snapshot")
	}

	joinedTimeline := strings.Join(taskSnapshot.Dashboard.ProjectTimeline, "\n")
	if !strings.Contains(joinedTimeline, "project_created") {
		t.Fatalf("dashboard timeline = %q, want project_created event", joinedTimeline)
	}
	if !strings.Contains(joinedTimeline, "workstream_created") {
		t.Fatalf("dashboard timeline = %q, want workstream_created event", joinedTimeline)
	}
	if !strings.Contains(joinedTimeline, "task_created") {
		t.Fatalf("dashboard timeline = %q, want task_created event", joinedTimeline)
	}
}

func TestProjectControlCreateTaskAcceptsExplicitSkillRunbook(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	taskBody := fmt.Sprintf(`{"projectId":%q,"workstreamId":%q,"title":"Explicit Skill Task","goal":"verify explicit selection","priority":"high","riskLevel":"medium","selectedSkill":"code_change","runbookId":"code_change_default"}`,
		projectControlProjectID, projectControlWorkstreamUXID)
	taskReq := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks", strings.NewReader(taskBody))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/tasks status = %d, want %d: %s", taskRec.Code, http.StatusCreated, taskRec.Body.String())
	}
	snapshot := decodeProjectControlSnapshot(t, taskRec)
	for _, task := range snapshot.Tasks {
		if task.Title != "Explicit Skill Task" {
			continue
		}
		if task.SelectedSkill != projectControlDefaultSkillID || task.RunbookID != projectControlDefaultRunbookID {
			t.Fatalf("created task skill/runbook = %q/%q, want %q/%q", task.SelectedSkill, task.RunbookID, projectControlDefaultSkillID, projectControlDefaultRunbookID)
		}
		return
	}
	t.Fatal("created task not found")
}

func TestProjectControlCreateTaskRejectsUnknownSkillRunbook(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	cases := []struct {
		name     string
		body     string
		wantBody string
	}{
		{
			name: "unknown skill",
			body: fmt.Sprintf(`{"projectId":%q,"workstreamId":%q,"title":"Unknown Skill Task","selectedSkill":"unknown_skill"}`,
				projectControlProjectID, projectControlWorkstreamUXID),
			wantBody: "unknown skill",
		},
		{
			name: "unknown runbook",
			body: fmt.Sprintf(`{"projectId":%q,"workstreamId":%q,"title":"Unknown Runbook Task","selectedSkill":"code_change","runbookId":"missing_runbook"}`,
				projectControlProjectID, projectControlWorkstreamUXID),
			wantBody: "unknown runbook",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			rec := httptest.NewRecorder()
			srv.mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("POST /api/project-control/tasks status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestProjectControlDecisionAppendsTaskTimelineEventAndPersists(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	srv := NewServer(cfg, nil, sessions, nil, nil)
	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+projectControlCheckpointAcceptanceID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, decisionRec)

	var foundDecisionEvent bool
	for _, task := range snapshot.Tasks {
		if task.ID != projectControlTaskIAID {
			continue
		}
		for _, item := range task.Timeline {
			if item.Action == "final_acceptance_approved" {
				foundDecisionEvent = true
				break
			}
		}
	}
	if !foundDecisionEvent {
		t.Fatal("expected final_acceptance_approved event in task timeline after approval")
	}

	restarted := NewServer(cfg, nil, sessions, nil, nil)
	getReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	getReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	getRec := httptest.NewRecorder()
	restarted.mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/project-control status = %d, want %d", getRec.Code, http.StatusOK)
	}
	persisted := decodeProjectControlSnapshot(t, getRec)
	foundDecisionEvent = false
	for _, task := range persisted.Tasks {
		if task.ID != projectControlTaskIAID {
			continue
		}
		for _, item := range task.Timeline {
			if item.Action == "final_acceptance_approved" {
				foundDecisionEvent = true
				break
			}
		}
	}
	if !foundDecisionEvent {
		t.Fatal("expected final_acceptance_approved event in persisted task timeline after restart")
	}
}

func TestProjectControlCanCreateProjectWorkstreamAndTask(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	projectReq := httptest.NewRequest(http.MethodPost, "/api/project-control/projects", strings.NewReader(`{"key":"demo","name":"Demo Project","description":"persistent demo","currentGoal":"ship prototype"}`))
	projectReq.Header.Set("Content-Type", "application/json")
	projectReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	projectRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(projectRec, projectReq)
	if projectRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/projects status = %d, want %d", projectRec.Code, http.StatusCreated)
	}
	projectSnapshot := decodeProjectControlSnapshot(t, projectRec)
	if len(projectSnapshot.Projects) != 2 {
		t.Fatalf("len(Projects) = %d, want 2 after create", len(projectSnapshot.Projects))
	}
	createdProject := projectSnapshot.Projects[1]

	workstreamBody := fmt.Sprintf(`{"projectId":%q,"title":"Stream A","description":"new stream","priority":"high","scopeSummary":"deliver ui"}`, createdProject.ID)
	workstreamReq := httptest.NewRequest(http.MethodPost, "/api/project-control/workstreams", strings.NewReader(workstreamBody))
	workstreamReq.Header.Set("Content-Type", "application/json")
	workstreamReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	workstreamRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(workstreamRec, workstreamReq)
	if workstreamRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/workstreams status = %d, want %d", workstreamRec.Code, http.StatusCreated)
	}
	workstreamSnapshot := decodeProjectControlSnapshot(t, workstreamRec)
	createdWorkstream := workstreamSnapshot.Workstreams[len(workstreamSnapshot.Workstreams)-1]

	taskBody := fmt.Sprintf(`{"projectId":%q,"workstreamId":%q,"title":"Implement persisted panel","goal":"replace seeded-only state","priority":"high","riskLevel":"medium"}`,
		createdProject.ID, createdWorkstream.ID)
	taskReq := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks", strings.NewReader(taskBody))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/tasks status = %d, want %d", taskRec.Code, http.StatusCreated)
	}
	taskSnapshot := decodeProjectControlSnapshot(t, taskRec)

	var foundProject, foundTask bool
	for _, project := range taskSnapshot.Projects {
		if project.ID == createdProject.ID && project.Name == "Demo Project" {
			foundProject = true
		}
	}
	for _, task := range taskSnapshot.Tasks {
		if task.Title == "Implement persisted panel" && task.WorkstreamID == createdWorkstream.ID {
			foundTask = true
			if task.State != "planned" {
				t.Fatalf("created task state = %q, want planned", task.State)
			}
		}
	}
	if !foundProject {
		t.Fatal("created project missing from snapshot")
	}
	if !foundTask {
		t.Fatal("created task missing from snapshot")
	}
}

func TestProjectControlTaskAndWorkstreamUpdatesEmitTransitionEvents(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	workstreamID := projectControlWorkstreamUXID
	taskID := projectControlTaskIAID

	workstreamReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/workstreams/"+workstreamID, strings.NewReader(`{"expectedRowVersion":1,"status":"blocked","priority":"critical"}`))
	workstreamReq.Header.Set("Content-Type", "application/json")
	workstreamReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	workstreamRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(workstreamRec, workstreamReq)
	if workstreamRec.Code != http.StatusOK {
		t.Fatalf("PATCH workstream status = %d, want %d", workstreamRec.Code, http.StatusOK)
	}
	wsSnapshot := decodeProjectControlSnapshot(t, workstreamRec)
	foundBlocked := false
	for _, workstream := range wsSnapshot.Workstreams {
		if workstream.ID == workstreamID && workstream.Status == "blocked" && workstream.Priority == "critical" && workstream.RowVersion == 2 {
			foundBlocked = true
		}
	}
	if !foundBlocked {
		t.Fatal("workstream update not reflected in snapshot")
	}

	taskReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+taskID, strings.NewReader(`{"expectedRowVersion":1,"state":"blocked","acceptanceStatus":"not_ready"}`))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusOK {
		t.Fatalf("PATCH task status = %d, want %d", taskRec.Code, http.StatusOK)
	}
	taskSnapshot := decodeProjectControlSnapshot(t, taskRec)
	foundTaskUpdate := false
	for _, task := range taskSnapshot.Tasks {
		if task.ID == taskID && task.State == "blocked" && task.AcceptanceStatus == "not_ready" && task.RowVersion == 2 {
			foundTaskUpdate = true
		}
	}
	if !foundTaskUpdate {
		t.Fatal("task update not reflected in snapshot")
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/project-control/events?taskId="+taskID+"&limit=10", nil)
	eventsReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	eventsRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("GET events status = %d, want %d", eventsRec.Code, http.StatusOK)
	}
	var payload struct {
		Events []projectControlRecordedEvent `json:"events"`
	}
	if err := json.NewDecoder(eventsRec.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	seenTaskState := false
	seenAcceptanceState := false
	for _, event := range payload.Events {
		if event.Action == "task_state_changed" {
			seenTaskState = true
		}
		if event.Action == "acceptance_state_changed" {
			seenAcceptanceState = true
		}
	}
	if !seenTaskState || !seenAcceptanceState {
		t.Fatalf("expected task transition events, got %#v", payload.Events)
	}
}

func TestProjectControlTaskAndWorkstreamUpdatesRejectStaleRowVersion(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	workstreamReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/workstreams/"+projectControlWorkstreamUXID, strings.NewReader(`{"expectedRowVersion":99,"status":"blocked"}`))
	workstreamReq.Header.Set("Content-Type", "application/json")
	workstreamReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	workstreamRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(workstreamRec, workstreamReq)
	if workstreamRec.Code != http.StatusConflict {
		t.Fatalf("PATCH stale workstream status = %d, want %d", workstreamRec.Code, http.StatusConflict)
	}
	var workstreamErr map[string]string
	if err := json.NewDecoder(workstreamRec.Body).Decode(&workstreamErr); err != nil {
		t.Fatalf("Decode workstream error: %v", err)
	}
	if !strings.Contains(workstreamErr["error"], "row version conflict") {
		t.Fatalf("workstream error = %q, want row version conflict", workstreamErr["error"])
	}

	taskReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":99,"state":"blocked"}`))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusConflict {
		t.Fatalf("PATCH stale task status = %d, want %d", taskRec.Code, http.StatusConflict)
	}
	var taskErr map[string]string
	if err := json.NewDecoder(taskRec.Body).Decode(&taskErr); err != nil {
		t.Fatalf("Decode task error: %v", err)
	}
	if !strings.Contains(taskErr["error"], "row version conflict") {
		t.Fatalf("task error = %q, want row version conflict", taskErr["error"])
	}
}

func TestProjectControlTaskAndWorkstreamActionTransitions(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	workstreamReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/workstreams/"+projectControlWorkstreamUXID, strings.NewReader(`{"expectedRowVersion":1,"action":"mark_blocked"}`))
	workstreamReq.Header.Set("Content-Type", "application/json")
	workstreamReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	workstreamRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(workstreamRec, workstreamReq)
	if workstreamRec.Code != http.StatusOK {
		t.Fatalf("PATCH workstream action status = %d, want %d", workstreamRec.Code, http.StatusOK)
	}
	workstreamSnapshot := decodeProjectControlSnapshot(t, workstreamRec)
	for _, workstream := range workstreamSnapshot.Workstreams {
		if workstream.ID == projectControlWorkstreamUXID {
			if workstream.Status != "blocked" || workstream.RowVersion != 2 {
				t.Fatalf("workstream after action = %#v, want blocked rowVersion=2", workstream)
			}
		}
	}

	taskReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"start_execution"}`))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusOK {
		t.Fatalf("PATCH task start action status = %d, want %d", taskRec.Code, http.StatusOK)
	}

	taskReq = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":2,"action":"mark_execution_complete"}`))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec = httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusOK {
		t.Fatalf("PATCH task complete action status = %d, want %d", taskRec.Code, http.StatusOK)
	}

	taskReq = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":3,"action":"mark_ready_for_acceptance"}`))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec = httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH task ready action status = %d, want %d", taskRec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(taskRec.Body.String(), "missing plan") {
		t.Fatalf("PATCH task ready error = %q, want missing plan evidence", taskRec.Body.String())
	}

	taskSnapshot, rowVersion := completeProjectControlTaskRunbook(t, srv, token, projectControlTaskPanelID, 3)
	if rowVersion != 13 {
		t.Fatalf("rowVersion after runbook = %d, want 13", rowVersion)
	}
	for _, task := range taskSnapshot.Tasks {
		if task.ID == projectControlTaskPanelID {
			if task.State != "execution_complete" || task.AcceptanceStatus != "ready_for_acceptance" || task.RowVersion != 13 {
				t.Fatalf("task after actions = %#v, want execution_complete + ready_for_acceptance + rowVersion=13", task)
			}
		}
	}
}

func TestProjectControlTaskAndWorkstreamRejectUnknownAction(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	workstreamReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/workstreams/"+projectControlWorkstreamUXID, strings.NewReader(`{"expectedRowVersion":1,"action":"teleport"}`))
	workstreamReq.Header.Set("Content-Type", "application/json")
	workstreamReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	workstreamRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(workstreamRec, workstreamReq)
	if workstreamRec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH unknown workstream action status = %d, want %d", workstreamRec.Code, http.StatusBadRequest)
	}

	taskReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"teleport"}`))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH unknown task action status = %d, want %d", taskRec.Code, http.StatusBadRequest)
	}
}

func TestProjectControlRejectsIllegalGuardedTransitions(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	workstreamReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/workstreams/"+projectControlWorkstreamUXID, strings.NewReader(`{"expectedRowVersion":1,"action":"archive"}`))
	workstreamReq.Header.Set("Content-Type", "application/json")
	workstreamReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	workstreamRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(workstreamRec, workstreamReq)
	if workstreamRec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH illegal workstream action status = %d, want %d", workstreamRec.Code, http.StatusBadRequest)
	}

	taskReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"mark_ready_for_acceptance"}`))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH illegal task action status = %d, want %d", taskRec.Code, http.StatusBadRequest)
	}

	taskReq = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"state":"running","acceptanceStatus":"under_human_review"}`))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec = httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH illegal acceptance transition status = %d, want %d", taskRec.Code, http.StatusBadRequest)
	}
}

func TestProjectControlReadyForAcceptanceCreatesCheckpoint(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	snapshot, _ := completeProjectControlTaskToAcceptanceReview(t, srv, token, projectControlTaskPanelID, 1)

	foundTask := false
	for _, task := range snapshot.Tasks {
		if task.ID == projectControlTaskPanelID && task.AcceptanceStatus == "under_human_review" {
			foundTask = true
		}
	}
	if !foundTask {
		t.Fatal("expected task to be under_human_review")
	}

	foundCheckpoint := false
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			foundCheckpoint = true
		}
	}
	if !foundCheckpoint {
		t.Fatalf("expected pending final_acceptance checkpoint for task, got %#v", snapshot.Checkpoints)
	}
}

func TestProjectControlLeavingAcceptanceFlowExpiresCheckpoint(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	_, rowVersion := completeProjectControlTaskToAcceptanceReview(t, srv, token, projectControlTaskPanelID, 1)

	req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(fmt.Sprintf(`{"expectedRowVersion":%d,"action":"reopen_task"}`, rowVersion)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH reopen task status = %d, want %d", rec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, rec)

	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			t.Fatalf("expected no pending final_acceptance checkpoint after leaving acceptance flow, got %#v", checkpoint)
		}
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/project-control/events?taskId="+projectControlTaskPanelID+"&limit=100", nil)
	eventsReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	eventsRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("GET task events status = %d, want %d", eventsRec.Code, http.StatusOK)
	}
	var payload struct {
		Events []projectControlRecordedEvent `json:"events"`
	}
	if err := json.NewDecoder(eventsRec.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode events error: %v", err)
	}
	foundExpired := false
	for _, event := range payload.Events {
		if event.Action == "checkpoint_expired" {
			foundExpired = true
		}
	}
	if !foundExpired {
		t.Fatalf("expected checkpoint_expired event, got %#v", payload.Events)
	}
}

func TestProjectControlEventsRouteSupportsFilteringAndOrdering(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	projectReq := httptest.NewRequest(http.MethodPost, "/api/project-control/projects", strings.NewReader(`{"key":"events-demo","name":"Events Demo","description":"event checks","currentGoal":"query events"}`))
	projectReq.Header.Set("Content-Type", "application/json")
	projectReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	projectRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(projectRec, projectReq)
	if projectRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/projects status = %d, want %d", projectRec.Code, http.StatusCreated)
	}
	projectSnapshot := decodeProjectControlSnapshot(t, projectRec)
	createdProject := projectSnapshot.Projects[len(projectSnapshot.Projects)-1]

	workstreamBody := fmt.Sprintf(`{"projectId":%q,"title":"Events Stream","description":"stream","priority":"high","scopeSummary":"scope"}`, createdProject.ID)
	workstreamReq := httptest.NewRequest(http.MethodPost, "/api/project-control/workstreams", strings.NewReader(workstreamBody))
	workstreamReq.Header.Set("Content-Type", "application/json")
	workstreamReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	workstreamRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(workstreamRec, workstreamReq)
	if workstreamRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/workstreams status = %d, want %d", workstreamRec.Code, http.StatusCreated)
	}
	workstreamSnapshot := decodeProjectControlSnapshot(t, workstreamRec)
	createdWorkstream := workstreamSnapshot.Workstreams[len(workstreamSnapshot.Workstreams)-1]

	taskBody := fmt.Sprintf(`{"projectId":%q,"workstreamId":%q,"title":"Events Task","goal":"observe history","priority":"high","riskLevel":"medium"}`,
		createdProject.ID, createdWorkstream.ID)
	taskReq := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks", strings.NewReader(taskBody))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/tasks status = %d, want %d", taskRec.Code, http.StatusCreated)
	}
	taskSnapshot := decodeProjectControlSnapshot(t, taskRec)
	var createdTask projectControlTask
	for _, task := range taskSnapshot.Tasks {
		if task.Title == "Events Task" {
			createdTask = task
			break
		}
	}
	if createdTask.ID == "" {
		t.Fatal("created task not found in snapshot")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/project-control/events?projectId="+createdProject.ID+"&taskId="+createdTask.ID+"&limit=2", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/project-control/events status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Events     []projectControlRecordedEvent `json:"events"`
		NextCursor string                        `json:"nextCursor"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("len(events) = %d, want 1 for filtered task history", len(payload.Events))
	}
	if payload.Events[0].Action != "task_created" {
		t.Fatalf("events[0].Action = %q, want task_created", payload.Events[0].Action)
	}
	if payload.Events[0].TaskID != createdTask.ID {
		t.Fatalf("events[0].TaskID = %q, want %q", payload.Events[0].TaskID, createdTask.ID)
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+projectControlCheckpointAcceptanceID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}

	checkpointReq := httptest.NewRequest(http.MethodGet, "/api/project-control/events?checkpointId="+projectControlCheckpointAcceptanceID+"&limit=5", nil)
	checkpointReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	checkpointRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(checkpointRec, checkpointReq)
	if checkpointRec.Code != http.StatusOK {
		t.Fatalf("GET checkpoint events status = %d, want %d", checkpointRec.Code, http.StatusOK)
	}
	payload = struct {
		Events     []projectControlRecordedEvent `json:"events"`
		NextCursor string                        `json:"nextCursor"`
	}{}
	if err := json.NewDecoder(checkpointRec.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(payload.Events) == 0 {
		t.Fatal("expected checkpoint filtered events")
	}
	if payload.Events[0].Action != "final_acceptance_approved" {
		t.Fatalf("latest checkpoint event = %q, want final_acceptance_approved", payload.Events[0].Action)
	}
}

func TestProjectControlEventsRoutePaginatesWithCursor(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	projectReq := httptest.NewRequest(http.MethodPost, "/api/project-control/projects", strings.NewReader(`{"key":"cursor-demo","name":"Cursor Demo","description":"pagination","currentGoal":"page events"}`))
	projectReq.Header.Set("Content-Type", "application/json")
	projectReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	projectRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(projectRec, projectReq)
	if projectRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/projects status = %d, want %d", projectRec.Code, http.StatusCreated)
	}
	projectSnapshot := decodeProjectControlSnapshot(t, projectRec)
	projectID := projectSnapshot.Projects[len(projectSnapshot.Projects)-1].ID

	wsReq := httptest.NewRequest(http.MethodPost, "/api/project-control/workstreams", strings.NewReader(fmt.Sprintf(`{"projectId":%q,"title":"Cursor Stream","description":"desc","priority":"high","scopeSummary":"scope"}`, projectID)))
	wsReq.Header.Set("Content-Type", "application/json")
	wsReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	wsRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(wsRec, wsReq)
	if wsRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/workstreams status = %d, want %d", wsRec.Code, http.StatusCreated)
	}

	firstReq := httptest.NewRequest(http.MethodGet, "/api/project-control/events?projectId="+projectID+"&limit=1", nil)
	firstReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	firstRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("GET first page status = %d, want %d", firstRec.Code, http.StatusOK)
	}
	var firstPage struct {
		Events     []projectControlRecordedEvent `json:"events"`
		NextCursor string                        `json:"nextCursor"`
	}
	if err := json.NewDecoder(firstRec.Body).Decode(&firstPage); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(firstPage.Events) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("first page = %#v, want 1 event and nextCursor", firstPage)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/api/project-control/events?projectId="+projectID+"&limit=1&cursor="+url.QueryEscape(firstPage.NextCursor), nil)
	secondReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	secondRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("GET second page status = %d, want %d", secondRec.Code, http.StatusOK)
	}
	var secondPage struct {
		Events     []projectControlRecordedEvent `json:"events"`
		NextCursor string                        `json:"nextCursor"`
	}
	if err := json.NewDecoder(secondRec.Body).Decode(&secondPage); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(secondPage.Events) != 1 {
		t.Fatalf("len(second page events) = %d, want 1", len(secondPage.Events))
	}
	if secondPage.Events[0].ID == firstPage.Events[0].ID {
		t.Fatal("cursor pagination returned duplicate event")
	}
}

func TestProjectControlTaskReplayRouteReturnsOrderedSteps(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+projectControlCheckpointAcceptanceID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}

	replayReq := httptest.NewRequest(http.MethodGet, "/api/project-control/tasks/"+projectControlTaskIAID+"/replay", nil)
	replayReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	replayRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("GET task replay status = %d, want %d", replayRec.Code, http.StatusOK)
	}
	var replay projectControlReplayResponse
	if err := json.NewDecoder(replayRec.Body).Decode(&replay); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if replay.TaskID != projectControlTaskIAID {
		t.Fatalf("replay task id = %q, want %q", replay.TaskID, projectControlTaskIAID)
	}
	if len(replay.Steps) < 3 {
		t.Fatalf("len(replay steps) = %d, want at least 3", len(replay.Steps))
	}
	if replay.Steps[0].Action != "submitted_doc" {
		t.Fatalf("first replay step = %q, want submitted_doc", replay.Steps[0].Action)
	}
	if replay.Steps[len(replay.Steps)-1].Action != "final_acceptance_approved" {
		t.Fatalf("last replay step = %q, want final_acceptance_approved", replay.Steps[len(replay.Steps)-1].Action)
	}
	if len(replay.Sections) < 2 {
		t.Fatalf("len(replay sections) = %d, want at least 2", len(replay.Sections))
	}
	if replay.Sections[0].Kind != "execution" {
		t.Fatalf("first section kind = %q, want execution", replay.Sections[0].Kind)
	}
	if replay.Sections[len(replay.Sections)-1].Kind != "decision" {
		t.Fatalf("last section kind = %q, want decision", replay.Sections[len(replay.Sections)-1].Kind)
	}
	if len(replay.Transitions) == 0 {
		t.Fatal("expected replay transitions")
	}
}

func TestProjectControlReplayExposesGuardedStateTransitions(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	startReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"start_execution"}`))
	startReq.Header.Set("Content-Type", "application/json")
	startReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	startRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("PATCH start execution status = %d, want %d", startRec.Code, http.StatusOK)
	}

	completeReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":2,"action":"mark_execution_complete"}`))
	completeReq.Header.Set("Content-Type", "application/json")
	completeReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	completeRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("PATCH complete execution status = %d, want %d", completeRec.Code, http.StatusOK)
	}

	replayReq := httptest.NewRequest(http.MethodGet, "/api/project-control/tasks/"+projectControlTaskPanelID+"/replay", nil)
	replayReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	replayRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("GET task replay status = %d, want %d", replayRec.Code, http.StatusOK)
	}
	var replay projectControlReplayResponse
	if err := json.NewDecoder(replayRec.Body).Decode(&replay); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	foundGuardedTransition := false
	for _, transition := range replay.Transitions {
		if transition.Type == "task_state" && transition.From == "running" && transition.To == "execution_complete" && strings.Contains(transition.Reason, "mark_execution_complete") {
			foundGuardedTransition = true
		}
	}
	if !foundGuardedTransition {
		t.Fatalf("expected replay transitions to expose guarded task action, got %#v", replay.Transitions)
	}
}

func TestProjectControlReplayExposesFinalAcceptanceDecision(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	completeProjectControlTaskToAcceptanceReview(t, srv, token, projectControlTaskPanelID, 1)

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("GET snapshot status = %d, want %d", snapshotRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending final_acceptance checkpoint")
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}

	replayReq := httptest.NewRequest(http.MethodGet, "/api/project-control/tasks/"+projectControlTaskPanelID+"/replay", nil)
	replayReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	replayRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("GET task replay status = %d, want %d", replayRec.Code, http.StatusOK)
	}
	var replay projectControlReplayResponse
	if err := json.NewDecoder(replayRec.Body).Decode(&replay); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	foundDecisionStep := false
	foundDecisionTransition := false
	for _, step := range replay.Steps {
		if step.Action == "final_acceptance_approved" {
			foundDecisionStep = true
		}
	}
	for _, transition := range replay.Transitions {
		if transition.Type == "decision" && transition.To == "final_acceptance_approved" {
			foundDecisionTransition = true
		}
	}
	if !foundDecisionStep || !foundDecisionTransition {
		t.Fatalf("expected replay to expose final acceptance decision, steps=%#v transitions=%#v", replay.Steps, replay.Transitions)
	}
}

func TestProjectControlDecisionEventsIncludeDecisionMadeAndCheckpointResolved(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	completeProjectControlTaskToAcceptanceReview(t, srv, token, projectControlTaskPanelID, 1)

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("GET snapshot status = %d, want %d", snapshotRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending final_acceptance checkpoint")
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/project-control/events?checkpointId="+checkpointID+"&limit=20", nil)
	eventsReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	eventsRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("GET checkpoint events status = %d, want %d", eventsRec.Code, http.StatusOK)
	}
	var payload struct {
		Events []projectControlRecordedEvent `json:"events"`
	}
	if err := json.NewDecoder(eventsRec.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode events error: %v", err)
	}
	seenDecisionMade := false
	seenCheckpointResolved := false
	for _, event := range payload.Events {
		if event.Action == "decision_made" {
			seenDecisionMade = true
		}
		if event.Action == "checkpoint_resolved" {
			seenCheckpointResolved = true
		}
	}
	if !seenDecisionMade || !seenCheckpointResolved {
		t.Fatalf("expected decision_made and checkpoint_resolved events, got %#v", payload.Events)
	}
}

func TestProjectControlFinalAcceptanceDecisionPersistsAndBackfillsTaskReference(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	srv := NewServer(cfg, nil, sessions, nil, nil)
	completeProjectControlTaskToAcceptanceReview(t, srv, token, projectControlTaskPanelID, 1)

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("GET snapshot status = %d, want %d", snapshotRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending final_acceptance checkpoint")
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}
	approvedSnapshot := decodeProjectControlSnapshot(t, decisionRec)
	decisionID := ""
	for _, task := range approvedSnapshot.Tasks {
		if task.ID == projectControlTaskPanelID {
			decisionID = task.AcceptanceDecisionID
			if decisionID == "" {
				t.Fatal("expected task acceptanceDecisionId to be populated")
			}
		}
	}

	restarted := NewServer(cfg, nil, sessions, nil, nil)
	reloadReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	reloadReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	reloadRec := httptest.NewRecorder()
	restarted.mux.ServeHTTP(reloadRec, reloadReq)
	if reloadRec.Code != http.StatusOK {
		t.Fatalf("GET snapshot after restart status = %d, want %d", reloadRec.Code, http.StatusOK)
	}
	reloaded := decodeProjectControlSnapshot(t, reloadRec)
	foundDecision := false
	for _, decision := range reloaded.Decisions {
		if decision.ID == decisionID && decision.DecisionType == "final_acceptance_approved" && decision.TaskID == projectControlTaskPanelID && decision.CheckpointID == checkpointID {
			foundDecision = true
		}
	}
	if !foundDecision {
		t.Fatalf("expected persisted final acceptance decision %q in snapshot, got %#v", decisionID, reloaded.Decisions)
	}
}

func TestProjectControlReplayIncludesAcceptanceDecisionMetadata(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	completeProjectControlTaskToAcceptanceReview(t, srv, token, projectControlTaskPanelID, 1)

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("GET snapshot status = %d, want %d", snapshotRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending final_acceptance checkpoint")
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}

	replayReq := httptest.NewRequest(http.MethodGet, "/api/project-control/tasks/"+projectControlTaskPanelID+"/replay", nil)
	replayReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	replayRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("GET task replay status = %d, want %d", replayRec.Code, http.StatusOK)
	}
	var replay projectControlReplayResponse
	if err := json.NewDecoder(replayRec.Body).Decode(&replay); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if replay.AcceptanceDecision == nil {
		t.Fatal("expected replay acceptanceDecision metadata")
	}
	if replay.AcceptanceDecision.DecisionType != "final_acceptance_approved" {
		t.Fatalf("replay acceptance decision type = %q, want final_acceptance_approved", replay.AcceptanceDecision.DecisionType)
	}
}

func TestProjectControlUnarchivePreservesAcceptanceDecisionMetadata(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	completeProjectControlTaskToAcceptanceReview(t, srv, token, projectControlTaskPanelID, 1)

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending final_acceptance checkpoint")
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}
	approved := decodeProjectControlSnapshot(t, decisionRec)
	rowVersion := 0
	for _, task := range approved.Tasks {
		if task.ID == projectControlTaskPanelID {
			rowVersion = task.RowVersion
			if task.AcceptanceDecisionID == "" {
				t.Fatal("expected acceptanceDecisionId after approval")
			}
			break
		}
	}
	if rowVersion == 0 {
		t.Fatal("expected approved task row version")
	}

	archiveReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(fmt.Sprintf(`{"expectedRowVersion":%d,"action":"archive"}`, rowVersion)))
	archiveReq.Header.Set("Content-Type", "application/json")
	archiveReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	archiveRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(archiveRec, archiveReq)
	if archiveRec.Code != http.StatusOK {
		t.Fatalf("PATCH archive accepted task status = %d, want %d", archiveRec.Code, http.StatusOK)
	}
	archived := decodeProjectControlSnapshot(t, archiveRec)
	archiveRowVersion := 0
	for _, task := range archived.Tasks {
		if task.ID == projectControlTaskPanelID {
			archiveRowVersion = task.RowVersion
			break
		}
	}
	if archiveRowVersion == 0 {
		t.Fatal("expected archived task row version")
	}

	unarchiveReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(fmt.Sprintf(`{"expectedRowVersion":%d,"action":"unarchive"}`, archiveRowVersion)))
	unarchiveReq.Header.Set("Content-Type", "application/json")
	unarchiveReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	unarchiveRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(unarchiveRec, unarchiveReq)
	if unarchiveRec.Code != http.StatusOK {
		t.Fatalf("PATCH unarchive task status = %d, want %d", unarchiveRec.Code, http.StatusOK)
	}
	unarchived := decodeProjectControlSnapshot(t, unarchiveRec)
	for _, task := range unarchived.Tasks {
		if task.ID == projectControlTaskPanelID {
			if task.AcceptanceDecisionID == "" {
				t.Fatal("expected acceptanceDecisionId to remain after unarchive")
			}
			if task.AcceptanceStatus != "accepted" {
				t.Fatalf("acceptanceStatus after unarchive = %q, want accepted", task.AcceptanceStatus)
			}
			break
		}
	}

	replayReq := httptest.NewRequest(http.MethodGet, "/api/project-control/tasks/"+projectControlTaskPanelID+"/replay", nil)
	replayReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	replayRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("GET task replay status = %d, want %d", replayRec.Code, http.StatusOK)
	}
	var replay projectControlReplayResponse
	if err := json.NewDecoder(replayRec.Body).Decode(&replay); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if replay.AcceptanceDecision == nil {
		t.Fatal("expected replay acceptanceDecision to remain after unarchive")
	}
}

func TestProjectControlAcceptedTaskRejectsGenericAcceptanceReset(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	completeProjectControlTaskToAcceptanceReview(t, srv, token, projectControlTaskPanelID, 1)

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending final_acceptance checkpoint")
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	approved := decodeProjectControlSnapshot(t, decisionRec)
	rowVersion := 0
	for _, task := range approved.Tasks {
		if task.ID == projectControlTaskPanelID {
			rowVersion = task.RowVersion
			break
		}
	}

	resetReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(fmt.Sprintf(`{"expectedRowVersion":%d,"acceptanceStatus":"not_ready"}`, rowVersion)))
	resetReq.Header.Set("Content-Type", "application/json")
	resetReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resetRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(resetRec, resetReq)
	if resetRec.Code != http.StatusBadRequest {
		t.Fatalf("generic acceptance reset status = %d, want %d", resetRec.Code, http.StatusBadRequest)
	}
}

func TestProjectControlArchiveOverrideDecisionArchivesUnacceptedTask(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	for _, body := range []string{
		`{"expectedRowVersion":1,"action":"start_execution"}`,
		`{"expectedRowVersion":2,"action":"mark_execution_complete"}`,
		`{"expectedRowVersion":3,"action":"request_archive_override"}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH setup status = %d, want %d for %s", rec.Code, http.StatusOK, body)
		}
	}

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("GET snapshot status = %d, want %d", snapshotRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "archive_override" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending archive_override checkpoint")
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST archive override checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}
	archived := decodeProjectControlSnapshot(t, decisionRec)
	decisionID := ""
	for _, task := range archived.Tasks {
		if task.ID == projectControlTaskPanelID {
			if task.State != "archived" {
				t.Fatalf("task state after archive override = %q, want archived", task.State)
			}
			decisionID = task.ArchiveDecisionID
			if decisionID == "" {
				t.Fatal("expected archiveDecisionId after archive override approval")
			}
			break
		}
	}
	if decisionID == "" {
		t.Fatal("expected archive override decision id")
	}
	foundDecision := false
	for _, decision := range archived.Decisions {
		if decision.ID == decisionID && decision.DecisionType == "archive_override_approved" && decision.TaskID == projectControlTaskPanelID && decision.CheckpointID == checkpointID {
			foundDecision = true
		}
	}
	if !foundDecision {
		t.Fatalf("expected archive override decision metadata in snapshot, got %#v", archived.Decisions)
	}

	replayReq := httptest.NewRequest(http.MethodGet, "/api/project-control/tasks/"+projectControlTaskPanelID+"/replay", nil)
	replayReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	replayRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("GET task replay status = %d, want %d", replayRec.Code, http.StatusOK)
	}
	var replay projectControlReplayResponse
	if err := json.NewDecoder(replayRec.Body).Decode(&replay); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if replay.ArchiveDecision == nil || replay.ArchiveDecision.DecisionType != "archive_override_approved" {
		t.Fatalf("replay archiveDecision = %#v, want archive_override_approved", replay.ArchiveDecision)
	}
}

func TestProjectControlRequestArchiveOverrideCreatesPendingCheckpoint(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	for _, body := range []string{
		`{"expectedRowVersion":1,"action":"start_execution"}`,
		`{"expectedRowVersion":2,"action":"mark_execution_complete"}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH setup status = %d, want %d for %s", rec.Code, http.StatusOK, body)
		}
	}

	requestReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":3,"action":"request_archive_override"}`))
	requestReq.Header.Set("Content-Type", "application/json")
	requestReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	requestRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(requestRec, requestReq)
	if requestRec.Code != http.StatusOK {
		t.Fatalf("PATCH request archive override status = %d, want %d", requestRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, requestRec)
	if snapshot.ApprovalsCount != 2 {
		t.Fatalf("ApprovalsCount = %d, want 2 with seeded IA checkpoint plus archive override request", snapshot.ApprovalsCount)
	}
	foundPending := false
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "archive_override" && checkpoint.Status == "pending" {
			foundPending = true
			if len(checkpoint.AllowedActions) != 2 || checkpoint.AllowedActions[0] != "approve" || checkpoint.AllowedActions[1] != "reject" {
				t.Fatalf("archive override allowedActions = %#v, want approve/reject", checkpoint.AllowedActions)
			}
		}
	}
	if !foundPending {
		t.Fatalf("expected pending archive_override checkpoint, got %#v", snapshot.Checkpoints)
	}
}

func TestProjectControlRequestArchiveOverrideRequiresExecutionCompleteTask(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"request_archive_override"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH request archive override from planned status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestProjectControlLeavingArchiveOverrideFlowExpiresCheckpoint(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	for _, body := range []string{
		`{"expectedRowVersion":1,"action":"start_execution"}`,
		`{"expectedRowVersion":2,"action":"mark_execution_complete"}`,
		`{"expectedRowVersion":3,"action":"request_archive_override"}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH setup status = %d, want %d for %s", rec.Code, http.StatusOK, body)
		}
	}

	reopenReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":4,"action":"reopen_task"}`))
	reopenReq.Header.Set("Content-Type", "application/json")
	reopenReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	reopenRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(reopenRec, reopenReq)
	if reopenRec.Code != http.StatusOK {
		t.Fatalf("PATCH reopen task status = %d, want %d", reopenRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, reopenRec)
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "archive_override" && checkpoint.Status == "pending" {
			t.Fatalf("expected archive_override checkpoint to stop pending, got %#v", checkpoint)
		}
	}
	if snapshot.ApprovalsCount != 1 {
		t.Fatalf("ApprovalsCount = %d, want 1 because only the seeded IA checkpoint should remain pending", snapshot.ApprovalsCount)
	}
}

func TestProjectControlReadyForAcceptanceExpiresArchiveOverrideCheckpoint(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	for _, body := range []string{
		`{"expectedRowVersion":1,"action":"start_execution"}`,
		`{"expectedRowVersion":2,"action":"mark_execution_complete"}`,
		`{"expectedRowVersion":3,"action":"request_archive_override"}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH setup status = %d, want %d for %s", rec.Code, http.StatusOK, body)
		}
	}

	snapshot, _ := completeProjectControlTaskRunbook(t, srv, token, projectControlTaskPanelID, 4)
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "archive_override" && checkpoint.Status == "pending" {
			t.Fatalf("expected archive_override checkpoint to expire when entering acceptance path, got %#v", checkpoint)
		}
	}
}

func TestProjectControlArchiveOverrideDirectTaskRouteMethodNotAllowed(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks/"+projectControlTaskPanelID+"/archive-override-decision", strings.NewReader(`{"action":"approve"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST direct archive override route status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestProjectControlArchiveOverrideCheckpointApprovalArchivesTask(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	for _, body := range []string{
		`{"expectedRowVersion":1,"action":"start_execution"}`,
		`{"expectedRowVersion":2,"action":"mark_execution_complete"}`,
		`{"expectedRowVersion":3,"action":"request_archive_override"}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH setup status = %d, want %d for %s", rec.Code, http.StatusOK, body)
		}
	}

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("GET snapshot status = %d, want %d", snapshotRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "archive_override" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending archive_override checkpoint")
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST archive override checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}
	approved := decodeProjectControlSnapshot(t, decisionRec)
	if approved.ApprovalsCount != 1 {
		t.Fatalf("ApprovalsCount = %d, want 1 because only the seeded IA checkpoint remains pending", approved.ApprovalsCount)
	}
	foundApprovedCheckpoint := false
	foundArchivedTask := false
	for _, checkpoint := range approved.Checkpoints {
		if checkpoint.ID == checkpointID {
			foundApprovedCheckpoint = checkpoint.Status == "approved" && checkpoint.ResolvedByDecisionID != ""
		}
	}
	for _, task := range approved.Tasks {
		if task.ID == projectControlTaskPanelID {
			foundArchivedTask = task.State == "archived" && task.ArchiveDecisionID != ""
		}
	}
	if !foundApprovedCheckpoint {
		t.Fatalf("expected approved archive override checkpoint, got %#v", approved.Checkpoints)
	}
	if !foundArchivedTask {
		t.Fatalf("expected archived task with archiveDecisionId, got %#v", approved.Tasks)
	}
}

func TestProjectControlResolvedCheckpointCannotBeDecidedTwice(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	for _, body := range []string{
		`{"expectedRowVersion":1,"action":"start_execution"}`,
		`{"expectedRowVersion":2,"action":"mark_execution_complete"}`,
		`{"expectedRowVersion":3,"action":"request_archive_override"}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH setup status = %d, want %d for %s", rec.Code, http.StatusOK, body)
		}
	}

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "archive_override" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending archive_override checkpoint")
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("first checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"reject"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	secondRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusBadRequest {
		t.Fatalf("second checkpoint decision status = %d, want %d", secondRec.Code, http.StatusBadRequest)
	}
}

func TestProjectControlArchiveOverrideRejectsPendingAcceptanceReview(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	_, rowVersion := completeProjectControlTaskToAcceptanceReview(t, srv, token, projectControlTaskPanelID, 1)

	requestReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(fmt.Sprintf(`{"expectedRowVersion":%d,"action":"request_archive_override"}`, rowVersion)))
	requestReq.Header.Set("Content-Type", "application/json")
	requestReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	requestRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(requestRec, requestReq)
	if requestRec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH archive override during pending review status = %d, want %d", requestRec.Code, http.StatusBadRequest)
	}
}

func TestProjectControlUnarchiveClearsArchiveDecisionMetadata(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	for _, body := range []string{
		`{"expectedRowVersion":1,"action":"start_execution"}`,
		`{"expectedRowVersion":2,"action":"mark_execution_complete"}`,
		`{"expectedRowVersion":3,"action":"request_archive_override"}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH setup status = %d, want %d for %s", rec.Code, http.StatusOK, body)
		}
	}

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("GET snapshot status = %d, want %d", snapshotRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "archive_override" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending archive_override checkpoint")
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST archive override checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}
	archived := decodeProjectControlSnapshot(t, decisionRec)
	rowVersion := 0
	for _, task := range archived.Tasks {
		if task.ID == projectControlTaskPanelID {
			rowVersion = task.RowVersion
			if task.ArchiveDecisionID == "" {
				t.Fatal("expected archiveDecisionId after archive override approval")
			}
			break
		}
	}
	if rowVersion == 0 {
		t.Fatal("expected archived task row version")
	}

	unarchiveReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(fmt.Sprintf(`{"expectedRowVersion":%d,"action":"unarchive"}`, rowVersion)))
	unarchiveReq.Header.Set("Content-Type", "application/json")
	unarchiveReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	unarchiveRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(unarchiveRec, unarchiveReq)
	if unarchiveRec.Code != http.StatusOK {
		t.Fatalf("PATCH unarchive task status = %d, want %d", unarchiveRec.Code, http.StatusOK)
	}
	unarchived := decodeProjectControlSnapshot(t, unarchiveRec)
	for _, task := range unarchived.Tasks {
		if task.ID == projectControlTaskPanelID {
			if task.ArchiveDecisionID != "" {
				t.Fatalf("archiveDecisionId after unarchive = %q, want empty", task.ArchiveDecisionID)
			}
			break
		}
	}

	replayReq := httptest.NewRequest(http.MethodGet, "/api/project-control/tasks/"+projectControlTaskPanelID+"/replay", nil)
	replayReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	replayRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("GET task replay status = %d, want %d", replayRec.Code, http.StatusOK)
	}
	var replay projectControlReplayResponse
	if err := json.NewDecoder(replayRec.Body).Decode(&replay); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if replay.ArchiveDecision != nil {
		t.Fatalf("replay archiveDecision = %#v, want nil after unarchive", replay.ArchiveDecision)
	}
}

func TestProjectControlArchiveRequiresAcceptedTask(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	for _, body := range []string{
		`{"expectedRowVersion":1,"action":"start_execution"}`,
		`{"expectedRowVersion":2,"action":"mark_execution_complete"}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH setup status = %d, want %d for %s", rec.Code, http.StatusOK, body)
		}
	}

	archiveReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":3,"action":"archive"}`))
	archiveReq.Header.Set("Content-Type", "application/json")
	archiveReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	archiveRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(archiveRec, archiveReq)
	if archiveRec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH archive without acceptance status = %d, want %d", archiveRec.Code, http.StatusBadRequest)
	}
}

func TestProjectControlAcceptedTaskCanArchive(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	completeProjectControlTaskToAcceptanceReview(t, srv, token, projectControlTaskPanelID, 1)

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("GET snapshot status = %d, want %d", snapshotRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	checkpointID := ""
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending final_acceptance checkpoint")
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}
	approved := decodeProjectControlSnapshot(t, decisionRec)
	rowVersion := 0
	for _, task := range approved.Tasks {
		if task.ID == projectControlTaskPanelID {
			rowVersion = task.RowVersion
			break
		}
	}
	if rowVersion == 0 {
		t.Fatal("expected accepted task in snapshot")
	}

	archiveReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(fmt.Sprintf(`{"expectedRowVersion":%d,"action":"archive"}`, rowVersion)))
	archiveReq.Header.Set("Content-Type", "application/json")
	archiveReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	archiveRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(archiveRec, archiveReq)
	if archiveRec.Code != http.StatusOK {
		t.Fatalf("PATCH archive accepted task status = %d, want %d", archiveRec.Code, http.StatusOK)
	}
	archived := decodeProjectControlSnapshot(t, archiveRec)
	for _, task := range archived.Tasks {
		if task.ID == projectControlTaskPanelID && task.State == "archived" {
			return
		}
	}
	t.Fatal("expected accepted task to archive successfully")
}

func TestProjectControlTaskReplayIncludesLiveSessionAndRuntimeEvents(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	termMgr := terminal.NewManager(&cfg.Terminal)
	defer termMgr.Stop()
	if _, err := termMgr.CreateSession("ian"); err != nil {
		t.Fatalf("Create terminal session error: %v", err)
	}

	srv := NewServer(cfg, nil, sessions, termMgr, nil)
	replayReq := httptest.NewRequest(http.MethodGet, "/api/project-control/tasks/"+projectControlTaskPanelID+"/replay", nil)
	replayReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	replayRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("GET task replay status = %d, want %d", replayRec.Code, http.StatusOK)
	}
	var replay projectControlReplayResponse
	if err := json.NewDecoder(replayRec.Body).Decode(&replay); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	foundSession := false
	for _, step := range replay.Steps {
		if step.Action == "session_started" {
			foundSession = true
			break
		}
	}
	if !foundSession {
		t.Fatal("expected live session_started event in replay steps")
	}
	foundExecutionSection := false
	for _, section := range replay.Sections {
		if section.Kind == "execution" {
			for _, step := range section.Steps {
				if step.Action == "session_started" {
					foundExecutionSection = true
					break
				}
			}
		}
	}
	if !foundExecutionSection {
		t.Fatal("expected execution section to include session_started")
	}
}

func TestProjectControlStoreSurvivesTerminalManagerStartup(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	srv := NewServer(cfg, nil, sessions, nil, nil)
	createReq := httptest.NewRequest(http.MethodPost, "/api/project-control/projects", strings.NewReader(`{"key":"persisted","name":"Persisted Project","description":"survives terminal manager"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	createRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/projects status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	termMgr := terminal.NewManager(&cfg.Terminal)
	defer termMgr.Stop()
	restarted := NewServer(cfg, nil, sessions, termMgr, nil)
	getReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	getReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	getRec := httptest.NewRecorder()
	restarted.mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/project-control status = %d, want %d", getRec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, getRec)
	for _, project := range snapshot.Projects {
		if project.Name == "Persisted Project" {
			return
		}
	}
	t.Fatal("persisted project missing after terminal manager startup")
}

func TestProjectControlStatePersistsAcrossServerRestart(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	srv := NewServer(cfg, nil, sessions, nil, nil)
	createReq := httptest.NewRequest(http.MethodPost, "/api/project-control/projects", strings.NewReader(`{"key":"persisted","name":"Persisted Project","description":"survives restart"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	createRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/projects status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+projectControlCheckpointAcceptanceID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", decisionRec.Code, http.StatusOK)
	}

	restarted := NewServer(cfg, nil, sessions, nil, nil)
	getReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	getReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	getRec := httptest.NewRecorder()
	restarted.mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/project-control status = %d, want %d", getRec.Code, http.StatusOK)
	}
	persistedSnapshot := decodeProjectControlSnapshot(t, getRec)

	var found bool
	for _, project := range persistedSnapshot.Projects {
		if project.Name == "Persisted Project" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("persisted project missing after restart")
	}
	if persistedSnapshot.Checkpoints[0].Status != "approved" {
		t.Fatalf("checkpoint status after restart = %q, want approved", persistedSnapshot.Checkpoints[0].Status)
	}
}
