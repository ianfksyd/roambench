package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/config"
	"github.com/ianf339/roambench/internal/terminal"
)

func cleanupProjectControlTerminalManager(t *testing.T, manager *terminal.Manager, username string) {
	t.Helper()
	t.Cleanup(func() {
		for _, session := range manager.ListSessions(username) {
			_ = manager.KillSessionForUser(username, session.ID)
		}
		manager.Stop()
	})
}

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

func setProjectControlRunToolAsyncForTest(t *testing.T, fn func(func())) {
	t.Helper()
	previous := projectControlRunToolAsync
	projectControlRunToolAsync = fn
	t.Cleanup(func() {
		projectControlRunToolAsync = previous
	})
}

func setProjectControlProcessStartedAtForTest(t *testing.T, startedAt time.Time) {
	t.Helper()
	previous := projectControlProcessStartedAt
	projectControlProcessStartedAt = startedAt
	t.Cleanup(func() {
		projectControlProcessStartedAt = previous
	})
}

func projectControlTaskFromSnapshotByID(t *testing.T, snapshot projectControlSnapshot, taskID string) projectControlTask {
	t.Helper()
	for _, task := range snapshot.Tasks {
		if task.ID == taskID {
			return task
		}
	}
	t.Fatalf("task %q not found in snapshot", taskID)
	return projectControlTask{}
}

func projectControlTaskFromSnapshotByTitle(t *testing.T, snapshot projectControlSnapshot, title string) projectControlTask {
	t.Helper()
	for _, task := range snapshot.Tasks {
		if task.Title == title {
			return task
		}
	}
	t.Fatalf("task %q not found in snapshot", title)
	return projectControlTask{}
}

func createProjectControlDocsUpdateTask(t *testing.T, srv *Server, token, title string) projectControlTask {
	t.Helper()
	taskBody := fmt.Sprintf(`{"projectId":%q,"workstreamId":%q,"title":%q,"goal":"verify docs lifecycle","priority":"medium","riskLevel":"low","selectedSkill":"docs_update","runbookId":"docs_update_default"}`,
		projectControlProjectID, projectControlWorkstreamUXID, title)
	taskReq := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks", strings.NewReader(taskBody))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/project-control/tasks status = %d, want %d: %s", taskRec.Code, http.StatusCreated, taskRec.Body.String())
	}
	return projectControlTaskFromSnapshotByTitle(t, decodeProjectControlSnapshot(t, taskRec), title)
}

func patchProjectControlTaskAndFind(t *testing.T, srv *Server, token string, task projectControlTask, body string) projectControlTask {
	t.Helper()
	snapshot := patchProjectControlTask(t, srv, token, task.ID, body)
	return projectControlTaskFromSnapshotByID(t, snapshot, task.ID)
}

func startProjectControlTaskPhaseViaAPI(t *testing.T, srv *Server, token string, task projectControlTask, phaseID string) projectControlTask {
	t.Helper()
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"start_phase","phaseId":%q}`, task.RowVersion, phaseID)
	return patchProjectControlTaskAndFind(t, srv, token, task, body)
}

func completeProjectControlTaskPhaseViaAPI(t *testing.T, srv *Server, token string, task projectControlTask, phaseID, artifactKind, outcome, value string) projectControlTask {
	t.Helper()
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":%q,"artifactKind":%q,"artifactOutcome":%q,"artifactLabel":%q,"artifactValue":%q}`,
		task.RowVersion, phaseID, artifactKind, outcome, artifactKind, value)
	return patchProjectControlTaskAndFind(t, srv, token, task, body)
}

func prepareProjectControlCodeChangeTaskAtTestPhase(t *testing.T, srv *Server, token string) projectControlTask {
	t.Helper()
	task := projectControlTask{ID: projectControlTaskPanelID, RowVersion: 1}
	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "plan")
	task = completeProjectControlTaskPhaseViaAPI(t, srv, token, task, "plan", "plan", "recorded", "Implementation plan recorded")
	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "implement")
	task = completeProjectControlTaskPhaseViaAPI(t, srv, token, task, "implement", "diff_summary", "recorded", "Project control implementation changed")
	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "test")
	return task
}

func prepareProjectControlCodeChangeTaskAtReviewPhase(t *testing.T, srv *Server, token string) projectControlTask {
	t.Helper()
	task := prepareProjectControlCodeChangeTaskAtTestPhase(t, srv, token)
	task = completeProjectControlTaskPhaseViaAPI(t, srv, token, task, "test", "test_result", "pass", "go test ./... passed")
	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "review")
	return task
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

func TestProjectControlNormalizeStateCompactsOlderTaskHistoryIntoMemory(t *testing.T) {
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	state := projectControlState{
		ActiveProjectID: projectControlProjectID,
		Projects: []projectControlProject{{
			ID:         projectControlProjectID,
			Name:       "Project",
			Status:     "active",
			RowVersion: 1,
		}},
		Workstreams: []projectControlWorkstream{{
			ID:         projectControlWorkstreamUXID,
			ProjectID:  projectControlProjectID,
			Title:      "Workstream",
			Status:     "running",
			Priority:   "high",
			RowVersion: 1,
		}},
		Tasks: []projectControlTask{{
			ID:               "task-compact",
			ProjectID:        projectControlProjectID,
			WorkstreamID:     projectControlWorkstreamUXID,
			Title:            "Compact history",
			State:            "running",
			AcceptanceStatus: "not_ready",
			Priority:         "high",
			RiskLevel:        "medium",
			SelectedSkill:    projectControlDefaultSkillID,
			RunbookID:        projectControlDefaultRunbookID,
			CurrentPhase:     "implement",
			RunbookState:     "in_progress",
			Timeline:         []projectControlEvent{{Timestamp: base.Format(time.RFC3339), Actor: "system", Action: "legacy", Detail: "derived history should be stripped"}},
			Evidence:         []projectControlEvidence{{Label: "legacy", Value: "derived evidence should be stripped"}},
			Audit:            []projectControlAuditItem{{Timestamp: base.Format(time.RFC3339), Actor: "system", Action: "legacy", Detail: "derived audit should be stripped"}},
			RowVersion:       1,
		}},
	}
	for i := 0; i < 70; i++ {
		state.Events = append(state.Events, projectControlRecordedEvent{
			ID:           fmt.Sprintf("event-%02d", i),
			Timestamp:    base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			Actor:        "system",
			Action:       "phase_progressed",
			Detail:       fmt.Sprintf("event %d", i),
			ProjectID:    projectControlProjectID,
			WorkstreamID: projectControlWorkstreamUXID,
			TaskID:       "task-compact",
		})
	}
	for i := 0; i < 30; i++ {
		state.Artifacts = append(state.Artifacts, projectControlArtifact{
			ID:        fmt.Sprintf("artifact-%02d", i),
			TaskID:    "task-compact",
			Kind:      "evidence_note",
			Outcome:   "recorded",
			Label:     fmt.Sprintf("Artifact %d", i),
			Value:     fmt.Sprintf("Value %d", i),
			CreatedAt: base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
	}
	for i := 0; i < 15; i++ {
		state.PhaseAttempts = append(state.PhaseAttempts, projectControlPhaseAttempt{
			ID:          fmt.Sprintf("attempt-%02d", i),
			TaskID:      "task-compact",
			RunbookID:   projectControlDefaultRunbookID,
			PhaseID:     "implement",
			StartedAt:   base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			CompletedAt: base.Add(time.Duration(i+1) * time.Minute).Format(time.RFC3339),
			Status:      "completed",
		})
		state.ToolRuns = append(state.ToolRuns, projectControlToolRun{
			ID:          fmt.Sprintf("tool-%02d", i),
			TaskID:      "task-compact",
			PhaseID:     "implement",
			ToolID:      "repo_status",
			StartedAt:   base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			CompletedAt: base.Add(time.Duration(i+1) * time.Minute).Format(time.RFC3339),
			Status:      "completed",
		})
	}

	projectControlNormalizeState(&state)

	if len(state.Events) != projectControlRetainedTaskEvents {
		t.Fatalf("len(Events) = %d, want %d", len(state.Events), projectControlRetainedTaskEvents)
	}
	if len(state.Artifacts) != projectControlRetainedTaskArtifacts {
		t.Fatalf("len(Artifacts) = %d, want %d", len(state.Artifacts), projectControlRetainedTaskArtifacts)
	}
	if len(state.PhaseAttempts) != projectControlRetainedTaskPhaseAttempts {
		t.Fatalf("len(PhaseAttempts) = %d, want %d", len(state.PhaseAttempts), projectControlRetainedTaskPhaseAttempts)
	}
	if len(state.ToolRuns) != projectControlRetainedTaskToolRuns {
		t.Fatalf("len(ToolRuns) = %d, want %d", len(state.ToolRuns), projectControlRetainedTaskToolRuns)
	}
	if len(state.Memories) != 1 {
		t.Fatalf("len(Memories) = %d, want 1", len(state.Memories))
	}
	memory := state.Memories[0]
	if memory.TaskID != "task-compact" {
		t.Fatalf("Memory TaskID = %q, want task-compact", memory.TaskID)
	}
	if memory.EventCount != 6 || memory.ArtifactCount != 7 || memory.PhaseAttemptCount != 3 || memory.ToolRunCount != 3 {
		t.Fatalf("Memory counts = %#v, want events=6 artifacts=7 phaseAttempts=3 toolRuns=3", memory)
	}
	if strings.TrimSpace(memory.Summary) == "" {
		t.Fatal("expected memory summary after compaction")
	}
	task := state.Tasks[0]
	if len(task.Timeline) != 0 || len(task.Evidence) != 0 || len(task.Audit) != 0 {
		t.Fatalf("derived task fields were not stripped: timeline=%d evidence=%d audit=%d", len(task.Timeline), len(task.Evidence), len(task.Audit))
	}
}

func TestProjectControlSnapshotRebuildsEvidenceAndTimelineFromMemory(t *testing.T) {
	base := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	state := projectControlState{
		ActiveProjectID: projectControlProjectID,
		Projects: []projectControlProject{{
			ID:         projectControlProjectID,
			Name:       "Project",
			Status:     "active",
			RowVersion: 1,
		}},
		Workstreams: []projectControlWorkstream{{
			ID:         projectControlWorkstreamUXID,
			ProjectID:  projectControlProjectID,
			Title:      "Workstream",
			Status:     "running",
			Priority:   "high",
			RowVersion: 1,
		}},
		Tasks: []projectControlTask{{
			ID:               "task-memory",
			ProjectID:        projectControlProjectID,
			WorkstreamID:     projectControlWorkstreamUXID,
			Title:            "Memory task",
			State:            "running",
			AcceptanceStatus: "not_ready",
			Priority:         "high",
			RiskLevel:        "medium",
			SelectedSkill:    projectControlDefaultSkillID,
			RunbookID:        projectControlDefaultRunbookID,
			CurrentPhase:     "implement",
			RunbookState:     "in_progress",
			Evidence:         []projectControlEvidence{{Label: "Legacy note", Value: "Preserve me"}},
			RowVersion:       1,
		}},
	}
	for i := 0; i < 70; i++ {
		state.Events = append(state.Events, projectControlRecordedEvent{
			ID:           fmt.Sprintf("memory-event-%02d", i),
			Timestamp:    base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			Actor:        "system",
			Action:       "phase_progressed",
			Detail:       fmt.Sprintf("event %d", i),
			ProjectID:    projectControlProjectID,
			WorkstreamID: projectControlWorkstreamUXID,
			TaskID:       "task-memory",
		})
	}

	projectControlNormalizeState(&state)
	snapshot := buildProjectControlSnapshot(state, "ian", nil)
	task := projectControlTaskFromSnapshotByID(t, snapshot, "task-memory")

	foundMemoryEvent := false
	for _, item := range task.Timeline {
		if item.Action == "history_compacted" {
			foundMemoryEvent = true
			break
		}
	}
	if !foundMemoryEvent {
		t.Fatal("expected compacted history marker in task timeline")
	}

	foundLegacyEvidence := false
	foundHistoryMemory := false
	for _, item := range task.Evidence {
		if item.Label == "Legacy note" && item.Value == "Preserve me" {
			foundLegacyEvidence = true
		}
		if item.Label == "History memory" && strings.TrimSpace(item.Value) != "" {
			foundHistoryMemory = true
		}
	}
	if !foundLegacyEvidence {
		t.Fatal("expected migrated legacy evidence in task evidence list")
	}
	if !foundHistoryMemory {
		t.Fatal("expected history memory evidence in task evidence list")
	}
}

func TestProjectControlDefaultRunbookDefinesCodeChangePhases(t *testing.T) {
	skills := defaultProjectControlSkills()
	if len(skills) != 2 {
		t.Fatalf("len(skills) = %d, want 2", len(skills))
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
	docsSkill := skills[1]
	if docsSkill.ID != projectControlDocsUpdateSkillID {
		t.Fatalf("docs skill ID = %q, want %q", docsSkill.ID, projectControlDocsUpdateSkillID)
	}
	if docsSkill.DefaultRunbookID != projectControlDocsUpdateRunbookID {
		t.Fatalf("docs DefaultRunbookID = %q, want %q", docsSkill.DefaultRunbookID, projectControlDocsUpdateRunbookID)
	}
	if docsSkill.PermissionsByPhase["write"] != "scoped_write" {
		t.Fatalf("write permission = %q, want scoped_write", docsSkill.PermissionsByPhase["write"])
	}
	if docsSkill.PermissionsByPhase["fix_or_replan"] != "scoped_write" {
		t.Fatalf("docs fix_or_replan permission = %q, want scoped_write", docsSkill.PermissionsByPhase["fix_or_replan"])
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

	docsRunbook, ok := findProjectControlRunbook(defaultProjectControlRunbooks(), projectControlDocsUpdateRunbookID)
	if !ok {
		t.Fatalf("docs runbook %q not found", projectControlDocsUpdateRunbookID)
	}
	wantDocsPhases := []string{"plan", "write", "review", "fix_or_replan", "final_validation"}
	if len(docsRunbook.Phases) != len(wantDocsPhases) {
		t.Fatalf("len(docsRunbook.Phases) = %d, want %d", len(docsRunbook.Phases), len(wantDocsPhases))
	}
	for i, want := range wantDocsPhases {
		if docsRunbook.Phases[i].ID != want {
			t.Fatalf("docs phase[%d] = %q, want %q", i, docsRunbook.Phases[i].ID, want)
		}
	}
	if next := nextProjectControlRunbookPhaseID(docsRunbook, "plan"); next != "write" {
		t.Fatalf("docs next phase after plan = %q, want write", next)
	}
	if next := nextProjectControlRunbookPhaseID(docsRunbook, "review"); next != "final_validation" {
		t.Fatalf("docs next phase after review = %q, want final_validation", next)
	}
	if next := nextProjectControlRunbookPhaseID(docsRunbook, "fix_or_replan"); next != "review" {
		t.Fatalf("docs next phase after fix_or_replan = %q, want review", next)
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
	if next := nextProjectControlRunbookPhaseID(runbook, "fix_or_replan"); next != "review" {
		t.Fatalf("next phase after fix_or_replan = %q, want review fallback", next)
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
	skill, runbook, err = validateProjectControlSkillRunbookSelection("docs_update", "docs_update_default")
	if err != nil {
		t.Fatalf("validate docs_update error: %v", err)
	}
	if skill.ID != projectControlDocsUpdateSkillID || runbook.ID != projectControlDocsUpdateRunbookID {
		t.Fatalf("docs selection = %q/%q, want %q/%q", skill.ID, runbook.ID, projectControlDocsUpdateSkillID, projectControlDocsUpdateRunbookID)
	}

	_, _, err = validateProjectControlSkillRunbookSelection("unknown", "")
	if err == nil || !strings.Contains(err.Error(), "unknown skill") {
		t.Fatalf("unknown skill error = %v, want unknown skill", err)
	}

	_, _, err = validateProjectControlSkillRunbookSelection("code_change", "missing_runbook")
	if err == nil || !strings.Contains(err.Error(), "unknown runbook") {
		t.Fatalf("unknown runbook error = %v, want unknown runbook", err)
	}
	_, _, err = validateProjectControlSkillRunbookSelection("docs_update", "code_change_default")
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("mismatched runbook error = %v, want not allowed", err)
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
		if err := startProjectControlTaskPhase(&state, &task, step.phase, now, "", nil, nil); err != nil {
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
	if err := startProjectControlTaskPhase(&state, &task, "final_validation", now, "", nil, nil); err != nil {
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

func TestProjectControlDocsUpdateRunbookPhaseLifecycleRecordsArtifacts(t *testing.T) {
	state := projectControlState{
		PhaseAttempts: []projectControlPhaseAttempt{},
		Artifacts:     []projectControlArtifact{},
	}
	task := projectControlTask{
		ID:               "task-docs-runbook-lifecycle",
		ProjectID:        "project-runbook",
		WorkstreamID:     "workstream-runbook",
		State:            "planned",
		AcceptanceStatus: "not_ready",
		RuntimeID:        projectControlRuntimeID,
		SelectedSkill:    projectControlDocsUpdateSkillID,
		RunbookID:        projectControlDocsUpdateRunbookID,
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
		{phase: "plan", kind: "plan", outcome: "recorded", value: "Docs plan artifact", next: "write"},
		{phase: "write", kind: "doc_summary", outcome: "recorded", value: "Updated docs for runbook selection", next: "review"},
		{phase: "review", kind: "review_result", outcome: "pass", value: "Docs review passed", next: "final_validation"},
	}
	for _, step := range steps {
		if err := startProjectControlTaskPhase(&state, &task, step.phase, now, "", nil, nil); err != nil {
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

	if err := startProjectControlTaskPhase(&state, &task, "final_validation", now, "", nil, nil); err != nil {
		t.Fatalf("start final_validation error: %v", err)
	}
	req := projectControlTaskUpdateRequest{
		PhaseID:         "final_validation",
		ArtifactKind:    "completion_check",
		ArtifactOutcome: "pass",
		ArtifactLabel:   "completion_check",
		ArtifactValue:   "Docs completion rules satisfied",
	}
	if err := completeProjectControlTaskPhase(&state, &task, req, now); err != nil {
		t.Fatalf("complete final_validation error: %v", err)
	}
	if task.State != "execution_complete" || task.AcceptanceStatus != "ready_for_acceptance" {
		t.Fatalf("task state/acceptance = %q/%q, want execution_complete/ready_for_acceptance", task.State, task.AcceptanceStatus)
	}
	if len(task.MissingEvidence) != 0 {
		t.Fatalf("MissingEvidence = %#v, want empty", task.MissingEvidence)
	}
}

func TestProjectControlDocsUpdateFailureRecoveryReturnsToReview(t *testing.T) {
	state := projectControlState{
		PhaseAttempts: []projectControlPhaseAttempt{},
		Artifacts:     []projectControlArtifact{},
	}
	task := projectControlTask{
		ID:               "task-docs-runbook-recovery",
		ProjectID:        "project-runbook",
		WorkstreamID:     "workstream-runbook",
		State:            "planned",
		AcceptanceStatus: "not_ready",
		RuntimeID:        projectControlRuntimeID,
		SelectedSkill:    projectControlDocsUpdateSkillID,
		RunbookID:        projectControlDocsUpdateRunbookID,
	}
	projectControlNormalizeState(&state)
	now := "2026-04-15T00:00:00Z"

	if err := startProjectControlTaskPhase(&state, &task, "plan", now, "", nil, nil); err != nil {
		t.Fatalf("start plan error: %v", err)
	}
	if err := completeProjectControlTaskPhase(&state, &task, projectControlTaskUpdateRequest{
		PhaseID:         "plan",
		ArtifactKind:    "plan",
		ArtifactOutcome: "recorded",
		ArtifactLabel:   "plan",
		ArtifactValue:   "Docs recovery plan",
	}, now); err != nil {
		t.Fatalf("complete plan error: %v", err)
	}
	if err := startProjectControlTaskPhase(&state, &task, "write", now, "", nil, nil); err != nil {
		t.Fatalf("start write error: %v", err)
	}
	if err := failProjectControlTaskPhase(&state, &task, "write", "Docs change needs rework", now); err != nil {
		t.Fatalf("fail write error: %v", err)
	}
	if task.CurrentPhase != "fix_or_replan" {
		t.Fatalf("CurrentPhase after fail = %q, want fix_or_replan", task.CurrentPhase)
	}
	if task.NextStep != "Start fix_or_replan, then rerun review evidence." {
		t.Fatalf("NextStep after fail = %q, want docs recovery review guidance", task.NextStep)
	}
	if err := startProjectControlTaskPhase(&state, &task, "fix_or_replan", now, "", nil, nil); err != nil {
		t.Fatalf("start fix_or_replan error: %v", err)
	}
	if err := completeProjectControlTaskPhase(&state, &task, projectControlTaskUpdateRequest{
		PhaseID:         "fix_or_replan",
		ArtifactKind:    "doc_summary",
		ArtifactOutcome: "recorded",
		ArtifactLabel:   "doc_summary",
		ArtifactValue:   "Docs recovery summary",
	}, now); err != nil {
		t.Fatalf("complete fix_or_replan error: %v", err)
	}
	if task.CurrentPhase != "review" {
		t.Fatalf("CurrentPhase after recovery = %q, want review", task.CurrentPhase)
	}
	if got := strings.Join(task.MissingEvidence, ","); got != "review_result:pass,completion_check:pass" {
		t.Fatalf("MissingEvidence after recovery = %q, want review and completion", got)
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
	if err := startProjectControlTaskPhase(&state, &task, "final_validation", now, "", nil, nil); err != nil {
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
	if err := startProjectControlTaskPhase(&state, &task, "final_validation", now, "", nil, nil); err != nil {
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

func TestProjectControlDocsUpdateFinalValidationRequiresDocSummary(t *testing.T) {
	state := projectControlState{
		PhaseAttempts: []projectControlPhaseAttempt{},
		Artifacts: []projectControlArtifact{
			{ID: "artifact-plan", TaskID: "task-docs-missing-summary", Kind: "plan", Outcome: "recorded"},
			{ID: "artifact-review", TaskID: "task-docs-missing-summary", Kind: "review_result", Outcome: "pass"},
		},
	}
	task := projectControlTask{
		ID:               "task-docs-missing-summary",
		ProjectID:        "project-runbook",
		WorkstreamID:     "workstream-runbook",
		State:            "running",
		AcceptanceStatus: "not_ready",
		RuntimeID:        projectControlRuntimeID,
		SelectedSkill:    projectControlDocsUpdateSkillID,
		RunbookID:        projectControlDocsUpdateRunbookID,
		CurrentPhase:     "final_validation",
		RunbookState:     "in_progress",
	}
	now := "2026-04-15T00:00:00Z"
	if err := startProjectControlTaskPhase(&state, &task, "final_validation", now, "", nil, nil); err != nil {
		t.Fatalf("start final_validation error: %v", err)
	}
	req := projectControlTaskUpdateRequest{
		PhaseID:         "final_validation",
		ArtifactKind:    "completion_check",
		ArtifactOutcome: "pass",
		ArtifactLabel:   "completion_check",
		ArtifactValue:   "Docs completion claimed without summary",
	}
	err := completeProjectControlTaskPhase(&state, &task, req, now)
	if err == nil {
		t.Fatal("complete docs final_validation error = nil, want missing doc_summary error")
	}
	if !strings.Contains(err.Error(), "doc_summary") {
		t.Fatalf("error = %q, want doc_summary evidence", err.Error())
	}
	if len(state.Artifacts) != 2 {
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
	if len(snapshot.Runbooks) != 2 || snapshot.Runbooks[0].ID != projectControlDefaultRunbookID || snapshot.Runbooks[1].ID != projectControlDocsUpdateRunbookID {
		t.Fatalf("Runbooks = %#v, want code_change and docs_update runbooks", snapshot.Runbooks)
	}
	if len(snapshot.Skills) != 2 || snapshot.Skills[0].ID != projectControlDefaultSkillID || snapshot.Skills[1].ID != projectControlDocsUpdateSkillID {
		t.Fatalf("Skills = %#v, want code_change and docs_update skills", snapshot.Skills)
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
	phaseSessionID := startSnapshot.PhaseAttempts[0].SessionID
	if phaseSessionID == "" {
		t.Fatalf("PhaseAttempt SessionID = empty, want bound session")
	}
	foundPhaseSession := false
	for _, session := range startSnapshot.Sessions {
		if session.ID == phaseSessionID && session.TaskID == projectControlTaskPanelID && session.PhaseAttemptID == startSnapshot.PhaseAttempts[0].ID {
			foundPhaseSession = true
			if session.State != "active" {
				t.Fatalf("phase session State = %q, want active", session.State)
			}
			break
		}
	}
	if !foundPhaseSession {
		t.Fatalf("Sessions after start = %#v, want phase-bound session %q", startSnapshot.Sessions, phaseSessionID)
	}
	startTask := projectControlTaskFromSnapshotByID(t, startSnapshot, projectControlTaskPanelID)
	if len(startTask.SessionIDs) != 1 || startTask.SessionIDs[0] != phaseSessionID {
		t.Fatalf("Task SessionIDs after start = %#v, want %q", startTask.SessionIDs, phaseSessionID)
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

func TestProjectControlStartReadOnlyPhaseCreatesLogicalSessionWithoutAttach(t *testing.T) {
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
	cleanupProjectControlTerminalManager(t, termMgr, "ian")
	srv := NewServer(cfg, nil, sessions, termMgr, nil)

	startReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"start_phase","phaseId":"plan"}`))
	startReq.Header.Set("Content-Type", "application/json")
	startReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	startRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("PATCH start_phase status = %d, want %d: %s", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	snapshot := decodeProjectControlSnapshot(t, startRec)
	if len(snapshot.PhaseAttempts) != 1 {
		t.Fatalf("len(PhaseAttempts) = %d, want 1", len(snapshot.PhaseAttempts))
	}
	terminalSessions := termMgr.ListSessions("ian")
	if len(terminalSessions) != 0 {
		t.Fatalf("len(terminal sessions) = %d, want 0 for read-only phase", len(terminalSessions))
	}
	attempt := snapshot.PhaseAttempts[0]
	if projectControlTerminalIDFromSessionID(attempt.SessionID) != "" {
		t.Fatalf("PhaseAttempt SessionID = %q, want logical non-terminal session", attempt.SessionID)
	}
	workspaceKind, workspaceDir := projectControlParseWorkspaceRef(attempt.WorkspaceRef)
	if workspaceKind != projectControlWorkspaceReadOnlySnapshot {
		t.Fatalf("PhaseAttempt WorkspaceRef = %q, want %q workspace kind", attempt.WorkspaceRef, projectControlWorkspaceReadOnlySnapshot)
	}
	if workspaceDir == "" {
		t.Fatalf("PhaseAttempt WorkspaceRef = %q, want concrete snapshot path", attempt.WorkspaceRef)
	}
	if workspaceDir == srv.projectControl.runtimeRootDir {
		t.Fatalf("PhaseAttempt workspaceDir = %q, want isolated snapshot path", workspaceDir)
	}
	if !strings.HasPrefix(workspaceDir, srv.projectControl.workspaceRootDir) {
		t.Fatalf("PhaseAttempt workspaceDir = %q, want prefix %q", workspaceDir, srv.projectControl.workspaceRootDir)
	}
	if info, err := os.Stat(workspaceDir); err != nil || !info.IsDir() {
		t.Fatalf("Stat(snapshot workspace) error = %v, want existing directory", err)
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = workspaceDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse in snapshot workspace error: %v", err)
	}
	if got := strings.TrimSpace(string(output)); got != workspaceDir {
		t.Fatalf("git rev-parse workspace = %q, want %q", got, workspaceDir)
	}
	taskSessionCount := 0
	for _, session := range snapshot.Sessions {
		if session.TaskID != projectControlTaskPanelID {
			continue
		}
		taskSessionCount += 1
		if session.ID != attempt.SessionID {
			t.Fatalf("phase session ID = %q, want %q", session.ID, attempt.SessionID)
		}
		if session.TerminalID != "" || session.SupportsAttach {
			t.Fatalf("phase session terminal binding = %#v, want logical non-attachable session", session)
		}
		if session.PhaseAttemptID != attempt.ID {
			t.Fatalf("phase session PhaseAttemptID = %q, want %q", session.PhaseAttemptID, attempt.ID)
		}
		if session.WorkspaceRef != attempt.WorkspaceRef {
			t.Fatalf("phase session WorkspaceRef = %q, want %q", session.WorkspaceRef, attempt.WorkspaceRef)
		}
	}
	if taskSessionCount != 1 {
		t.Fatalf("task session count = %d, want exactly one phase-linked session", taskSessionCount)
	}
}

func TestProjectControlStartWritePhaseCreatesAttachableTerminalSession(t *testing.T) {
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
	cleanupProjectControlTerminalManager(t, termMgr, "ian")
	srv := NewServer(cfg, nil, sessions, termMgr, nil)

	task := projectControlTask{ID: projectControlTaskPanelID, RowVersion: 1}
	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "plan")
	if len(termMgr.ListSessions("ian")) != 0 {
		t.Fatalf("read-only plan phase created live terminal sessions")
	}
	task = completeProjectControlTaskPhaseViaAPI(t, srv, token, task, "plan", "plan", "recorded", "Implementation plan recorded")

	startImplementBody := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"implement"}`, task.RowVersion)
	snapshot := patchProjectControlTask(t, srv, token, task.ID, startImplementBody)
	terminalSessions := termMgr.ListSessions("ian")
	if len(terminalSessions) != 1 {
		t.Fatalf("len(terminal sessions) = %d, want 1 for scoped-write phase", len(terminalSessions))
	}
	wantSessionID := projectControlSessionIDForTerminal(terminalSessions[0].ID)
	wantWorkspaceRef := ""
	taskSessionCount := 0
	for _, attempt := range snapshot.PhaseAttempts {
		if attempt.TaskID != projectControlTaskPanelID || attempt.PhaseID != "implement" {
			continue
		}
		if attempt.SessionID != wantSessionID {
			t.Fatalf("PhaseAttempt SessionID = %q, want %q", attempt.SessionID, wantSessionID)
		}
		workspaceKind, workspaceDir := projectControlParseWorkspaceRef(attempt.WorkspaceRef)
		if workspaceKind != projectControlWorkspaceSharedRepo {
			t.Fatalf("PhaseAttempt WorkspaceRef = %q, want %q workspace kind", attempt.WorkspaceRef, projectControlWorkspaceSharedRepo)
		}
		if workspaceDir != srv.projectControl.runtimeRootDir {
			t.Fatalf("PhaseAttempt workspaceDir = %q, want %q", workspaceDir, srv.projectControl.runtimeRootDir)
		}
		wantWorkspaceRef = attempt.WorkspaceRef
	}
	if wantWorkspaceRef == "" {
		t.Fatal("implement phase attempt not found in snapshot")
	}
	for _, session := range snapshot.Sessions {
		if session.TaskID != projectControlTaskPanelID || session.PhaseAttemptID == "" || session.ExecutionRole != "implement" {
			continue
		}
		taskSessionCount += 1
		if session.ID != wantSessionID {
			t.Fatalf("phase session ID = %q, want %q", session.ID, wantSessionID)
		}
		if session.TerminalID != terminalSessions[0].ID || !session.SupportsAttach {
			t.Fatalf("phase session terminal binding = %#v, want attachable terminal %q", session, terminalSessions[0].ID)
		}
		if session.WorkspaceRef != wantWorkspaceRef {
			t.Fatalf("phase session WorkspaceRef = %q, want %q", session.WorkspaceRef, wantWorkspaceRef)
		}
	}
	if taskSessionCount != 1 {
		t.Fatalf("task session count = %d, want exactly one attachable phase-linked session", taskSessionCount)
	}
}

func TestProjectControlAllowsTerminalAttachOnlyForScopedWritePhaseSessions(t *testing.T) {
	store := newProjectControlStore(t.TempDir())
	_, err := store.withStateLocked("ian", func(state *projectControlState) error {
		state.Tasks = []projectControlTask{{
			ID:               "task-attach-policy",
			ProjectID:        "project-attach",
			WorkstreamID:     "workstream-attach",
			State:            "running",
			AcceptanceStatus: "not_ready",
			RuntimeID:        projectControlRuntimeID,
			SelectedSkill:    projectControlDefaultSkillID,
			RunbookID:        projectControlDefaultRunbookID,
			CurrentPhase:     "implement",
			RunbookState:     "in_progress",
		}}
		state.PhaseAttempts = []projectControlPhaseAttempt{
			{ID: "attempt-review", TaskID: "task-attach-policy", RunbookID: projectControlDefaultRunbookID, PhaseID: "review", SessionID: projectControlSessionIDForTerminal("term-review"), Status: "running"},
			{ID: "attempt-implement", TaskID: "task-attach-policy", RunbookID: projectControlDefaultRunbookID, PhaseID: "implement", SessionID: projectControlSessionIDForTerminal("term-implement"), Status: "running"},
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withStateLocked error: %v", err)
	}
	if store.allowsTerminalAttach("ian", "term-review") {
		t.Fatal("review phase terminal attach should be denied")
	}
	if !store.allowsTerminalAttach("ian", "term-implement") {
		t.Fatal("implement phase terminal attach should be allowed")
	}
	if !store.allowsTerminalAttach("ian", "term-unlinked") {
		t.Fatal("unlinked terminal sessions should remain attachable")
	}
}

func testProjectControlPersistenceState(updatedAt, taskTitle string) projectControlState {
	state := projectControlState{
		ActiveProjectID: "project-wal",
		Projects: []projectControlProject{{
			ID:         "project-wal",
			Name:       "Project WAL",
			Status:     "active",
			RowVersion: 1,
		}},
		Workstreams: []projectControlWorkstream{{
			ID:         "workstream-wal",
			ProjectID:  "project-wal",
			Title:      "Durability",
			Status:     "running",
			Priority:   "high",
			RowVersion: 1,
		}},
		Tasks: []projectControlTask{{
			ID:               "task-wal",
			ProjectID:        "project-wal",
			WorkstreamID:     "workstream-wal",
			Title:            taskTitle,
			State:            "running",
			AcceptanceStatus: "not_ready",
			Priority:         "high",
			RiskLevel:        "medium",
			SelectedSkill:    projectControlDefaultSkillID,
			RunbookID:        projectControlDefaultRunbookID,
			CurrentPhase:     "implement",
			RunbookState:     "in_progress",
			RowVersion:       1,
		}},
		UpdatedAt: updatedAt,
	}
	projectControlNormalizeState(&state)
	state.UpdatedAt = updatedAt
	return state
}

func TestProjectControlLoadLockedRecoversFromNewerWAL(t *testing.T) {
	store := newProjectControlStore(t.TempDir())
	base := testProjectControlPersistenceState("2026-01-01T00:00:00Z", "Base task")
	if err := store.saveLocked("ian", base); err != nil {
		t.Fatalf("saveLocked(base) error: %v", err)
	}

	recovered := testProjectControlPersistenceState("2026-01-01T00:01:00Z", "Recovered task")
	payload, err := json.Marshal(recovered)
	if err != nil {
		t.Fatalf("Marshal(recovered) error: %v", err)
	}
	if err := projectControlWriteFileAtomically(store.walPathFor("ian"), payload, 0600); err != nil {
		t.Fatalf("Write WAL error: %v", err)
	}

	loaded, exists, err := store.loadLocked("ian")
	if err != nil {
		t.Fatalf("loadLocked error: %v", err)
	}
	if !exists {
		t.Fatal("expected state to exist after WAL recovery")
	}
	if got := loaded.Tasks[0].Title; got != "Recovered task" {
		t.Fatalf("loaded task title = %q, want %q", got, "Recovered task")
	}
	if _, err := os.Stat(store.walPathFor("ian")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("walPath still exists after recovery: %v", err)
	}

	persisted, exists, err := projectControlReadStateFile(store.pathFor("ian"))
	if err != nil {
		t.Fatalf("Read persisted state error: %v", err)
	}
	if !exists {
		t.Fatal("expected persisted state after WAL recovery")
	}
	if got := persisted.Tasks[0].Title; got != "Recovered task" {
		t.Fatalf("persisted task title = %q, want %q", got, "Recovered task")
	}
}

func TestProjectControlLoadLockedUsesWALWhenMainStateIsMissing(t *testing.T) {
	store := newProjectControlStore(t.TempDir())
	recovered := testProjectControlPersistenceState("2026-01-01T00:02:00Z", "Recovered from WAL only")
	payload, err := json.Marshal(recovered)
	if err != nil {
		t.Fatalf("Marshal(recovered) error: %v", err)
	}
	if err := projectControlWriteFileAtomically(store.walPathFor("ian"), payload, 0600); err != nil {
		t.Fatalf("Write WAL error: %v", err)
	}

	loaded, exists, err := store.loadLocked("ian")
	if err != nil {
		t.Fatalf("loadLocked error: %v", err)
	}
	if !exists {
		t.Fatal("expected loadLocked to recover from WAL without main state")
	}
	if got := loaded.Tasks[0].Title; got != "Recovered from WAL only" {
		t.Fatalf("loaded task title = %q, want %q", got, "Recovered from WAL only")
	}

	if _, err := os.Stat(store.pathFor("ian")); err != nil {
		t.Fatalf("main state path missing after WAL recovery: %v", err)
	}
	if _, err := os.Stat(store.walPathFor("ian")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("walPath still exists after recovery: %v", err)
	}
}

func TestProjectControlRunToolCompletesTestPhaseOnPassingResult(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	previous := projectControlExecuteTool
	usedWorkspaceDir := ""
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		if toolID != "go_test" {
			t.Fatalf("toolID = %q, want go_test", toolID)
		}
		if strings.TrimSpace(workspaceDir) == "" {
			t.Fatal("workspaceDir is empty, want runtime workspace")
		}
		usedWorkspaceDir = workspaceDir
		return projectControlToolResult{
			ToolID:          "go_test",
			ArtifactKind:    "test_result",
			ArtifactOutcome: "pass",
			ArtifactLabel:   "Go test",
			ArtifactValue:   "Command: go test ./...\nStatus: passed\nOutput:\nok ./...",
		}, nil
	}
	defer func() { projectControlExecuteTool = previous }()

	task := prepareProjectControlCodeChangeTaskAtTestPhase(t, srv, token)
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"test","toolId":"go_test"}`, task.RowVersion)
	snapshot := patchProjectControlTask(t, srv, token, task.ID, body)
	updated := projectControlTaskFromSnapshotByID(t, snapshot, task.ID)
	if updated.CurrentPhase != "review" {
		t.Fatalf("CurrentPhase = %q, want review", updated.CurrentPhase)
	}
	foundArtifact := false
	for _, artifact := range snapshot.Artifacts {
		if artifact.TaskID == task.ID && artifact.Kind == "test_result" && artifact.Outcome == "pass" && strings.Contains(artifact.Value, "go test") {
			foundArtifact = true
			break
		}
	}
	if !foundArtifact {
		t.Fatalf("Artifacts = %#v, want passing test_result from tool", snapshot.Artifacts)
	}
	foundCompletedAttempt := false
	attemptWorkspaceRef := ""
	for _, attempt := range snapshot.PhaseAttempts {
		if attempt.TaskID == task.ID && attempt.PhaseID == "test" && attempt.Status == "completed" {
			foundCompletedAttempt = true
			attemptWorkspaceRef = attempt.WorkspaceRef
			workspaceKind, workspaceDir := projectControlParseWorkspaceRef(attempt.WorkspaceRef)
			if workspaceKind != projectControlWorkspaceReadOnlySnapshot {
				t.Fatalf("test phase WorkspaceRef = %q, want %q workspace kind", attempt.WorkspaceRef, projectControlWorkspaceReadOnlySnapshot)
			}
			if workspaceDir != usedWorkspaceDir {
				t.Fatalf("tool workspaceDir = %q, want %q from completed attempt", usedWorkspaceDir, workspaceDir)
			}
			if workspaceDir == srv.projectControl.runtimeRootDir {
				t.Fatalf("tool workspaceDir = %q, want isolated snapshot instead of shared repo", workspaceDir)
			}
			break
		}
	}
	if !foundCompletedAttempt {
		t.Fatalf("PhaseAttempts = %#v, want completed test attempt", snapshot.PhaseAttempts)
	}
	if attemptWorkspaceRef == "" {
		t.Fatal("completed test attempt did not record workspace ref")
	}
	foundCompletedToolRun := false
	for _, run := range snapshot.ToolRuns {
		if run.TaskID == task.ID && run.PhaseID == "test" && run.ToolID == "go_test" && run.Status == "completed" && run.Outcome == "pass" && run.ArtifactID != "" {
			if run.WorkspaceRef != attemptWorkspaceRef {
				t.Fatalf("ToolRun WorkspaceRef = %q, want %q", run.WorkspaceRef, attemptWorkspaceRef)
			}
			foundCompletedToolRun = true
			break
		}
	}
	if !foundCompletedToolRun {
		t.Fatalf("ToolRuns = %#v, want completed go_test run with artifact", snapshot.ToolRuns)
	}
}

func TestProjectControlRunToolRoutesFailingTestToRecovery(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	previous := projectControlExecuteTool
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		return projectControlToolResult{
			ToolID:          "go_test",
			ArtifactKind:    "test_result",
			ArtifactOutcome: "fail",
			ArtifactLabel:   "Go test",
			ArtifactValue:   "Command: go test ./...\nStatus: failed\nOutput:\nFAIL ./internal/server",
		}, nil
	}
	defer func() { projectControlExecuteTool = previous }()

	task := prepareProjectControlCodeChangeTaskAtTestPhase(t, srv, token)
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"test","toolId":"go_test"}`, task.RowVersion)
	snapshot := patchProjectControlTask(t, srv, token, task.ID, body)
	updated := projectControlTaskFromSnapshotByID(t, snapshot, task.ID)
	if updated.CurrentPhase != "fix_or_replan" {
		t.Fatalf("CurrentPhase = %q, want fix_or_replan", updated.CurrentPhase)
	}
	if updated.RunbookState != "needs_fix" {
		t.Fatalf("RunbookState = %q, want needs_fix", updated.RunbookState)
	}
	foundFailedArtifact := false
	for _, artifact := range snapshot.Artifacts {
		if artifact.TaskID == task.ID && artifact.Kind == "test_result" && artifact.Outcome == "fail" {
			foundFailedArtifact = true
			break
		}
	}
	if !foundFailedArtifact {
		t.Fatalf("Artifacts = %#v, want failed test_result from tool", snapshot.Artifacts)
	}
	foundFailedAttempt := false
	for _, attempt := range snapshot.PhaseAttempts {
		if attempt.TaskID == task.ID && attempt.PhaseID == "test" && attempt.Status == "failed" && strings.Contains(attempt.FailureReason, "go_test") {
			foundFailedAttempt = true
			break
		}
	}
	if !foundFailedAttempt {
		t.Fatalf("PhaseAttempts = %#v, want failed test attempt", snapshot.PhaseAttempts)
	}
	foundFailedToolRun := false
	for _, run := range snapshot.ToolRuns {
		if run.TaskID == task.ID && run.PhaseID == "test" && run.ToolID == "go_test" && run.Status == "failed" && run.Outcome == "fail" && run.ArtifactID != "" {
			foundFailedToolRun = true
			break
		}
	}
	if !foundFailedToolRun {
		t.Fatalf("ToolRuns = %#v, want failed go_test run with artifact", snapshot.ToolRuns)
	}
}

func TestProjectControlRunToolRecordsRepoStatusWithoutAdvancingReviewPhase(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	previous := projectControlExecuteTool
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		if toolID != "repo_status" {
			t.Fatalf("toolID = %q, want repo_status", toolID)
		}
		return projectControlToolResult{
			ToolID:          "repo_status",
			ArtifactKind:    "repo_status",
			ArtifactOutcome: "pass",
			ArtifactLabel:   "Repo status",
			ArtifactValue:   "Command: git status --short\nStatus: clean\nOutput:\nWorking tree clean.",
		}, nil
	}
	defer func() { projectControlExecuteTool = previous }()

	task := prepareProjectControlCodeChangeTaskAtReviewPhase(t, srv, token)
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"review","toolId":"repo_status"}`, task.RowVersion)
	snapshot := patchProjectControlTask(t, srv, token, task.ID, body)
	updated := projectControlTaskFromSnapshotByID(t, snapshot, task.ID)
	if updated.CurrentPhase != "review" {
		t.Fatalf("CurrentPhase = %q, want review", updated.CurrentPhase)
	}
	foundArtifact := false
	for _, artifact := range snapshot.Artifacts {
		if artifact.TaskID == task.ID && artifact.Kind == "repo_status" && artifact.Outcome == "pass" && strings.Contains(artifact.Value, "Working tree clean") {
			foundArtifact = true
			break
		}
	}
	if !foundArtifact {
		t.Fatalf("Artifacts = %#v, want repo_status artifact", snapshot.Artifacts)
	}
	foundRunningAttempt := false
	for _, attempt := range snapshot.PhaseAttempts {
		if attempt.TaskID == task.ID && attempt.PhaseID == "review" && attempt.Status == "running" {
			foundRunningAttempt = true
			break
		}
	}
	if !foundRunningAttempt {
		t.Fatalf("PhaseAttempts = %#v, want running review attempt", snapshot.PhaseAttempts)
	}
	foundCompletedToolRun := false
	for _, run := range snapshot.ToolRuns {
		if run.TaskID == task.ID && run.PhaseID == "review" && run.ToolID == "repo_status" && run.Status == "completed" && run.Outcome == "pass" && run.ArtifactID != "" {
			foundCompletedToolRun = true
			break
		}
	}
	if !foundCompletedToolRun {
		t.Fatalf("ToolRuns = %#v, want completed repo_status run with artifact", snapshot.ToolRuns)
	}
}

func TestProjectControlRunToolReturnsRunningToolRunBeforeAsyncCompletion(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	var queued func()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) {
		queued = fn
	})
	previous := projectControlExecuteTool
	executed := false
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		executed = true
		return projectControlToolResult{}, nil
	}
	defer func() { projectControlExecuteTool = previous }()

	task := prepareProjectControlCodeChangeTaskAtTestPhase(t, srv, token)
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"test","toolId":"go_test"}`, task.RowVersion)
	snapshot := patchProjectControlTask(t, srv, token, task.ID, body)
	if queued == nil {
		t.Fatal("run_tool did not enqueue async completion")
	}
	if executed {
		t.Fatal("tool executed before async completion hook was released")
	}
	updated := projectControlTaskFromSnapshotByID(t, snapshot, task.ID)
	if updated.CurrentPhase != "test" {
		t.Fatalf("CurrentPhase = %q, want test while tool is running", updated.CurrentPhase)
	}
	foundRunningToolRun := false
	for _, run := range snapshot.ToolRuns {
		if run.TaskID == task.ID && run.PhaseID == "test" && run.ToolID == "go_test" && run.Status == "running" && run.ArtifactID == "" {
			foundRunningToolRun = true
			break
		}
	}
	if !foundRunningToolRun {
		t.Fatalf("ToolRuns = %#v, want running go_test run without artifact", snapshot.ToolRuns)
	}
	secondBody := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"test","toolId":"repo_status"}`, updated.RowVersion)
	secondReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+task.ID, strings.NewReader(secondBody))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	secondRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusBadRequest {
		t.Fatalf("second run_tool status = %d, want %d: %s", secondRec.Code, http.StatusBadRequest, secondRec.Body.String())
	}
	if !strings.Contains(secondRec.Body.String(), "already running") {
		t.Fatalf("second run_tool error = %q, want already running", secondRec.Body.String())
	}
}

func TestProjectControlRecoversInterruptedRunningToolRun(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	setProjectControlRunToolAsyncForTest(t, func(fn func()) {})
	task := prepareProjectControlCodeChangeTaskAtTestPhase(t, srv, token)
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"test","toolId":"go_test"}`, task.RowVersion)
	runningSnapshot := patchProjectControlTask(t, srv, token, task.ID, body)
	runningTask := projectControlTaskFromSnapshotByID(t, runningSnapshot, task.ID)
	foundRunningToolRun := false
	for _, run := range runningSnapshot.ToolRuns {
		if run.TaskID == task.ID && run.ToolID == "go_test" && run.Status == "running" {
			foundRunningToolRun = true
			break
		}
	}
	if !foundRunningToolRun {
		t.Fatalf("ToolRuns = %#v, want running go_test before recovery", runningSnapshot.ToolRuns)
	}

	setProjectControlProcessStartedAtForTest(t, time.Now().UTC().Add(time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/project-control status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	recoveredSnapshot := decodeProjectControlSnapshot(t, rec)
	recoveredTask := projectControlTaskFromSnapshotByID(t, recoveredSnapshot, task.ID)
	if recoveredTask.RowVersion <= runningTask.RowVersion {
		t.Fatalf("RowVersion = %d, want > %d after interrupted tool recovery", recoveredTask.RowVersion, runningTask.RowVersion)
	}
	if recoveredTask.CurrentPhase != "test" {
		t.Fatalf("CurrentPhase = %q, want test after interrupted tool recovery", recoveredTask.CurrentPhase)
	}
	foundRecoveredToolRun := false
	for _, run := range recoveredSnapshot.ToolRuns {
		if run.TaskID == task.ID && run.ToolID == "go_test" && run.Status == "failed" && strings.Contains(run.Error, "server restarted") {
			foundRecoveredToolRun = true
			break
		}
	}
	if !foundRecoveredToolRun {
		t.Fatalf("ToolRuns = %#v, want recovered failed go_test run", recoveredSnapshot.ToolRuns)
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

func TestProjectControlCreateTaskAcceptsDocsUpdateSkillRunbook(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	taskBody := fmt.Sprintf(`{"projectId":%q,"workstreamId":%q,"title":"Docs Update Task","goal":"verify docs skill","priority":"medium","riskLevel":"low","selectedSkill":"docs_update","runbookId":"docs_update_default"}`,
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
		if task.Title != "Docs Update Task" {
			continue
		}
		if task.SelectedSkill != projectControlDocsUpdateSkillID || task.RunbookID != projectControlDocsUpdateRunbookID {
			t.Fatalf("created docs task skill/runbook = %q/%q, want %q/%q", task.SelectedSkill, task.RunbookID, projectControlDocsUpdateSkillID, projectControlDocsUpdateRunbookID)
		}
		if got := strings.Join(task.MissingEvidence, ","); got != "plan,doc_summary,review_result:pass,completion_check:pass" {
			t.Fatalf("docs task MissingEvidence = %q, want docs requirements", got)
		}
		return
	}
	t.Fatal("created docs task not found")
}

func TestProjectControlDocsUpdateHTTPRunbookLifecycle(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	task := createProjectControlDocsUpdateTask(t, srv, token, "Docs Update HTTP Lifecycle")
	if task.SelectedSkill != projectControlDocsUpdateSkillID || task.RunbookID != projectControlDocsUpdateRunbookID {
		t.Fatalf("created docs task skill/runbook = %q/%q, want %q/%q", task.SelectedSkill, task.RunbookID, projectControlDocsUpdateSkillID, projectControlDocsUpdateRunbookID)
	}
	if task.CurrentPhase != "plan" || task.RunbookState != "not_started" {
		t.Fatalf("created docs task phase/runbook state = %q/%q, want plan/not_started", task.CurrentPhase, task.RunbookState)
	}
	if got := strings.Join(task.MissingEvidence, ","); got != "plan,doc_summary,review_result:pass,completion_check:pass" {
		t.Fatalf("created docs MissingEvidence = %q, want full docs requirements", got)
	}

	steps := []struct {
		phase       string
		kind        string
		outcome     string
		value       string
		wantPhase   string
		wantState   string
		wantMissing string
	}{
		{phase: "plan", kind: "plan", outcome: "recorded", value: "Docs lifecycle plan", wantPhase: "write", wantState: "running", wantMissing: "doc_summary,review_result:pass,completion_check:pass"},
		{phase: "write", kind: "doc_summary", outcome: "recorded", value: "Docs lifecycle summary", wantPhase: "review", wantState: "waiting_review", wantMissing: "review_result:pass,completion_check:pass"},
		{phase: "review", kind: "review_result", outcome: "pass", value: "Docs lifecycle review passed", wantPhase: "final_validation", wantState: "running", wantMissing: "completion_check:pass"},
		{phase: "final_validation", kind: "completion_check", outcome: "pass", value: "Docs lifecycle completion passed", wantPhase: "ready_for_acceptance", wantState: "execution_complete", wantMissing: ""},
	}
	for _, step := range steps {
		task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, step.phase)
		if task.CurrentPhase != step.phase {
			t.Fatalf("after start %s CurrentPhase = %q, want %q", step.phase, task.CurrentPhase, step.phase)
		}
		task = completeProjectControlTaskPhaseViaAPI(t, srv, token, task, step.phase, step.kind, step.outcome, step.value)
		if task.CurrentPhase != step.wantPhase {
			t.Fatalf("after complete %s CurrentPhase = %q, want %q", step.phase, task.CurrentPhase, step.wantPhase)
		}
		if task.State != step.wantState {
			t.Fatalf("after complete %s State = %q, want %q", step.phase, task.State, step.wantState)
		}
		if got := strings.Join(task.MissingEvidence, ","); got != step.wantMissing {
			t.Fatalf("after complete %s MissingEvidence = %q, want %q", step.phase, got, step.wantMissing)
		}
	}
	if task.AcceptanceStatus != "ready_for_acceptance" {
		t.Fatalf("final AcceptanceStatus = %q, want ready_for_acceptance", task.AcceptanceStatus)
	}
}

func TestProjectControlDocsUpdateHTTPFailureRecoveryReturnsToReview(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	task := createProjectControlDocsUpdateTask(t, srv, token, "Docs Update HTTP Recovery")
	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "plan")
	task = completeProjectControlTaskPhaseViaAPI(t, srv, token, task, "plan", "plan", "recorded", "Docs recovery plan")
	if task.CurrentPhase != "write" {
		t.Fatalf("after complete plan CurrentPhase = %q, want write", task.CurrentPhase)
	}

	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "write")
	failBody := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"fail_phase","phaseId":"write","failureReason":"Docs change needs rework"}`, task.RowVersion)
	task = patchProjectControlTaskAndFind(t, srv, token, task, failBody)
	if task.CurrentPhase != "fix_or_replan" {
		t.Fatalf("after fail CurrentPhase = %q, want fix_or_replan", task.CurrentPhase)
	}
	if task.State != "failed" || task.RunbookState != "needs_fix" {
		t.Fatalf("after fail state/runbook = %q/%q, want failed/needs_fix", task.State, task.RunbookState)
	}
	if task.NextStep != "Start fix_or_replan, then rerun review evidence." {
		t.Fatalf("after fail NextStep = %q, want docs recovery review guidance", task.NextStep)
	}
	if got := strings.Join(task.MissingEvidence, ","); got != "doc_summary,review_result:pass,completion_check:pass" {
		t.Fatalf("after fail MissingEvidence = %q, want docs recovery requirements", got)
	}

	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "fix_or_replan")
	if task.State != "running" {
		t.Fatalf("after start fix_or_replan State = %q, want running", task.State)
	}
	task = completeProjectControlTaskPhaseViaAPI(t, srv, token, task, "fix_or_replan", "doc_summary", "recorded", "Docs recovery summary")
	if task.CurrentPhase != "review" {
		t.Fatalf("after complete fix_or_replan CurrentPhase = %q, want review", task.CurrentPhase)
	}
	if task.State != "waiting_review" {
		t.Fatalf("after complete fix_or_replan State = %q, want waiting_review", task.State)
	}
	if task.NextStep != "Start review and record its required evidence." {
		t.Fatalf("after complete fix_or_replan NextStep = %q, want review evidence guidance", task.NextStep)
	}
	if got := strings.Join(task.MissingEvidence, ","); got != "review_result:pass,completion_check:pass" {
		t.Fatalf("after complete fix_or_replan MissingEvidence = %q, want review and completion", got)
	}
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

	// start_execution now starts the first runbook phase, so get the actual rowVersion after.
	startReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"start_execution"}`))
	startReq.Header.Set("Content-Type", "application/json")
	startReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	startRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("PATCH task start action status = %d, want %d: %s", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	startSnapshot := decodeProjectControlSnapshot(t, startRec)
	startTask := projectControlTaskFromSnapshotByID(t, startSnapshot, projectControlTaskPanelID)
	if startTask.State != "running" {
		t.Fatalf("task state after start_execution = %q, want running", startTask.State)
	}
	rv := startTask.RowVersion

	markCompleteBody := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"mark_execution_complete"}`, rv)
	taskReq := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(markCompleteBody))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusOK {
		t.Fatalf("PATCH task complete action status = %d, want %d: %s", taskRec.Code, http.StatusOK, taskRec.Body.String())
	}
	rv++

	readyBody := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"mark_ready_for_acceptance"}`, rv)
	taskReq = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(readyBody))
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	taskRec = httptest.NewRecorder()
	srv.mux.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH task ready action status = %d, want %d", taskRec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(taskRec.Body.String(), "missing") {
		t.Fatalf("PATCH task ready error = %q, want missing evidence", taskRec.Body.String())
	}

	// Plan phase was already started by start_execution, so complete it then continue.
	completeSteps := []string{
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"plan","artifactKind":"plan","artifactOutcome":"recorded","artifactLabel":"Plan","artifactValue":"ok"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"implement"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"implement","artifactKind":"diff_summary","artifactOutcome":"recorded","artifactLabel":"Diff","artifactValue":"ok"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"test"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"test","artifactKind":"test_result","artifactOutcome":"pass","artifactLabel":"Test","artifactValue":"ok"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"review"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"review","artifactKind":"review_result","artifactOutcome":"pass","artifactLabel":"Review","artifactValue":"ok"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"final_validation"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"final_validation","artifactKind":"completion_check","artifactOutcome":"pass","artifactLabel":"Check","artifactValue":"ok"}`,
	}
	var taskSnapshot projectControlSnapshot
	for _, step := range completeSteps {
		taskSnapshot = patchProjectControlTask(t, srv, token, projectControlTaskPanelID, fmt.Sprintf(step, rv))
		rv++
	}
	for _, task := range taskSnapshot.Tasks {
		if task.ID == projectControlTaskPanelID {
			if task.State != "execution_complete" || task.AcceptanceStatus != "ready_for_acceptance" {
				t.Fatalf("task after actions = state=%q acceptance=%q, want execution_complete + ready_for_acceptance", task.State, task.AcceptanceStatus)
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

	// start_execution now starts the first phase (plan), so we set up
	// the archive override path using direct state transitions instead.
	for _, body := range []string{
		`{"expectedRowVersion":1,"action":"queue_task"}`,
		`{"expectedRowVersion":2,"action":"start_execution"}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH setup status = %d, want %d for %s: %s", rec.Code, http.StatusOK, body, rec.Body.String())
		}
	}
	// After start_execution, plan phase is running. Get current rowVersion.
	snapshot := decodeProjectControlSnapshot(t, func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, r)
		return w
	}())
	task := projectControlTaskFromSnapshotByID(t, snapshot, projectControlTaskPanelID)
	rv := task.RowVersion

	// Mark execution complete and request archive override
	for _, body := range []string{
		fmt.Sprintf(`{"expectedRowVersion":%d,"action":"mark_execution_complete"}`, rv),
		fmt.Sprintf(`{"expectedRowVersion":%d,"action":"request_archive_override"}`, rv+1),
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH status = %d, want %d for %s: %s", rec.Code, http.StatusOK, body, rec.Body.String())
		}
	}
	rv += 2

	// Complete the runbook from plan phase (already started by start_execution).
	// Plan is running, so complete it first, then continue.
	completeSteps := []string{
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"plan","artifactKind":"plan","artifactOutcome":"recorded","artifactLabel":"Plan","artifactValue":"Plan recorded"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"implement"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"implement","artifactKind":"diff_summary","artifactOutcome":"recorded","artifactLabel":"Diff","artifactValue":"Diff recorded"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"test"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"test","artifactKind":"test_result","artifactOutcome":"pass","artifactLabel":"Test","artifactValue":"ok"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"review"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"review","artifactKind":"review_result","artifactOutcome":"pass","artifactLabel":"Review","artifactValue":"ok"}`,
		`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"final_validation"}`,
		`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"final_validation","artifactKind":"completion_check","artifactOutcome":"pass","artifactLabel":"Check","artifactValue":"ok"}`,
	}
	for _, step := range completeSteps {
		snapshot = patchProjectControlTask(t, srv, token, projectControlTaskPanelID, fmt.Sprintf(step, rv))
		rv++
	}

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
	cleanupProjectControlTerminalManager(t, termMgr, "ian")
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
	cleanupProjectControlTerminalManager(t, termMgr, "ian")
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

func TestProjectControlDefaultToolsReturnThreeBuiltinTools(t *testing.T) {
	tools := defaultProjectControlTools()
	if len(tools) != 3 {
		t.Fatalf("len(defaultProjectControlTools()) = %d, want 3", len(tools))
	}
	ids := map[string]bool{}
	for _, def := range tools {
		ids[def.ID] = true
		if len(def.Command) == 0 {
			t.Fatalf("tool %q has empty Command", def.ID)
		}
		if def.ArtifactKind == "" {
			t.Fatalf("tool %q has empty ArtifactKind", def.ID)
		}
		if len(def.AllowedPhases) == 0 {
			t.Fatalf("tool %q has empty AllowedPhases", def.ID)
		}
	}
	for _, expected := range []string{"repo_status", "diff_capture", "go_test"} {
		if !ids[expected] {
			t.Fatalf("missing default tool %q", expected)
		}
	}
}

func TestProjectControlFindToolDefReturnsMatchAndMiss(t *testing.T) {
	tools := defaultProjectControlTools()
	def, ok := findProjectControlToolDef(tools, "go_test")
	if !ok {
		t.Fatal("findProjectControlToolDef(go_test) returned false")
	}
	if def.ArtifactKind != "test_result" {
		t.Fatalf("ArtifactKind = %q, want test_result", def.ArtifactKind)
	}
	_, ok = findProjectControlToolDef(tools, "nonexistent_tool")
	if ok {
		t.Fatal("findProjectControlToolDef(nonexistent_tool) returned true, want false")
	}
}

func TestProjectControlToolDefTimeoutDefaults(t *testing.T) {
	def := projectControlToolDef{}
	if def.timeout() != projectControlToolRunTimeout {
		t.Fatalf("zero timeout() = %v, want %v", def.timeout(), projectControlToolRunTimeout)
	}
	def.TimeoutSeconds = 600
	if def.timeout() != 600*time.Second {
		t.Fatalf("custom timeout() = %v, want 600s", def.timeout())
	}
}

func TestProjectControlToolAllowedInPhaseUsesRegistry(t *testing.T) {
	tools := defaultProjectControlTools()
	if !projectControlToolAllowedInPhase("go_test", "test", tools) {
		t.Fatal("go_test should be allowed in test phase")
	}
	if projectControlToolAllowedInPhase("go_test", "plan", tools) {
		t.Fatal("go_test should not be allowed in plan phase")
	}
	if !projectControlToolAllowedInPhase("repo_status", "implement", tools) {
		t.Fatal("repo_status should be allowed in implement phase")
	}
	if projectControlToolAllowedInPhase("repo_status", "ready_for_acceptance", tools) {
		t.Fatal("repo_status should not be allowed in ready_for_acceptance phase")
	}
	if projectControlToolAllowedInPhase("unknown_tool", "test", tools) {
		t.Fatal("unknown tool should not be allowed in any phase")
	}
}

func TestProjectControlSnapshotIncludesTools(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	req := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/project-control status = %d, want 200", rec.Code)
	}
	snapshot := decodeProjectControlSnapshot(t, rec)
	if len(snapshot.Tools) == 0 {
		t.Fatal("snapshot.Tools is empty, want default tools")
	}
	foundGoTest := false
	for _, def := range snapshot.Tools {
		if def.ID == "go_test" {
			foundGoTest = true
		}
	}
	if !foundGoTest {
		t.Fatal("snapshot.Tools missing go_test")
	}
}

func TestProjectControlAutoProgressAdvancesFromTestToReviewAfterToolPass(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	callCount := 0
	previous := projectControlExecuteTool
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		callCount++
		switch normalizeProjectControlToolID(toolID) {
		case "go_test":
			return projectControlToolResult{
				ToolID: "go_test", ArtifactKind: "test_result", ArtifactOutcome: "pass",
				ArtifactLabel: "Go test", ArtifactValue: "ok",
			}, nil
		case "repo_status":
			return projectControlToolResult{
				ToolID: "repo_status", ArtifactKind: "repo_status", ArtifactOutcome: "pass",
				ArtifactLabel: "Repo status", ArtifactValue: "clean",
			}, nil
		default:
			return projectControlToolResult{
				ToolID: toolID, ArtifactKind: "review_result", ArtifactOutcome: "pass",
				ArtifactLabel: "Review", ArtifactValue: "ok",
			}, nil
		}
	}
	defer func() { projectControlExecuteTool = previous }()

	task := prepareProjectControlCodeChangeTaskAtTestPhase(t, srv, token)
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"test","toolId":"go_test"}`, task.RowVersion)
	snapshot := patchProjectControlTask(t, srv, token, task.ID, body)
	updated := projectControlTaskFromSnapshotByID(t, snapshot, task.ID)

	// After test phase passes, completeProjectControlTaskPhase advances to review.
	// The review phase requires review_result — no default tool produces that,
	// so auto-progression stops at review (no auto_progress event).
	if updated.CurrentPhase != "review" {
		t.Fatalf("CurrentPhase = %q, want review (advanced from test)", updated.CurrentPhase)
	}
	if callCount < 1 {
		t.Fatalf("tool callCount = %d, want >= 1", callCount)
	}

	// Verify phase_completed event was recorded for test phase
	foundPhaseCompleted := false
	req := httptest.NewRequest(http.MethodGet, "/api/project-control/events?limit=20", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var eventsResp projectControlEventsResponse
	if err := json.NewDecoder(rec.Body).Decode(&eventsResp); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	for _, ev := range eventsResp.Events {
		if ev.Action == "phase_completed" && strings.Contains(ev.Detail, "test") {
			foundPhaseCompleted = true
			break
		}
	}
	if !foundPhaseCompleted {
		t.Fatal("expected phase_completed event for test phase")
	}
}

func TestProjectControlAutoProgressDisabledStopsAtCurrentPhase(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	previous := projectControlExecuteTool
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		return projectControlToolResult{
			ToolID: "go_test", ArtifactKind: "test_result", ArtifactOutcome: "pass",
			ArtifactLabel: "Go test", ArtifactValue: "ok",
		}, nil
	}
	defer func() { projectControlExecuteTool = previous }()

	task := prepareProjectControlCodeChangeTaskAtTestPhase(t, srv, token)

	// Disable auto-progress on this task
	disableBody := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"test","toolId":"go_test"}`, task.RowVersion)
	// First, we need to set AutoProgress=false. We do this by directly manipulating state.
	srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID == task.ID {
				f := false
				state.Tasks[i].AutoProgress = &f
				task.RowVersion = state.Tasks[i].RowVersion
				break
			}
		}
		return nil
	})

	disableBody = fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"test","toolId":"go_test"}`, task.RowVersion)
	snapshot := patchProjectControlTask(t, srv, token, task.ID, disableBody)
	updated := projectControlTaskFromSnapshotByID(t, snapshot, task.ID)

	// With auto-progress disabled, should stay at review (the next phase after test)
	// but NOT auto-start the review phase tool
	if updated.CurrentPhase != "review" {
		t.Fatalf("CurrentPhase = %q, want review", updated.CurrentPhase)
	}
	// No auto_progress event should exist
	req := httptest.NewRequest(http.MethodGet, "/api/project-control/events?limit=20", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var eventsResp projectControlEventsResponse
	if err := json.NewDecoder(rec.Body).Decode(&eventsResp); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	for _, ev := range eventsResp.Events {
		if ev.Action == "auto_progress" {
			t.Fatal("auto_progress event found but AutoProgress was disabled")
		}
	}
}

func TestProjectControlToolForPhaseMatchesArtifactKind(t *testing.T) {
	tools := defaultProjectControlTools()
	testPhase := projectControlRunbookPhase{ID: "test", RequiredArtifacts: []string{"test_result"}}
	def, ok := projectControlToolForPhase(tools, testPhase)
	if !ok {
		t.Fatal("expected tool match for test phase")
	}
	if def.ID != "go_test" {
		t.Fatalf("matched tool = %q, want go_test", def.ID)
	}

	planPhase := projectControlRunbookPhase{ID: "plan", RequiredArtifacts: []string{"plan"}}
	_, ok = projectControlToolForPhase(tools, planPhase)
	if ok {
		t.Fatal("expected no tool match for plan phase (no tool produces plan artifact)")
	}
}

func TestProjectControlAutoProgressChainsFromImplementThroughTestToReview(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	toolCalls := []string{}
	previous := projectControlExecuteTool
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		toolCalls = append(toolCalls, normalizeProjectControlToolID(toolID))
		switch normalizeProjectControlToolID(toolID) {
		case "diff_capture":
			return projectControlToolResult{
				ToolID: "diff_capture", ArtifactKind: "diff_summary", ArtifactOutcome: "pass",
				ArtifactLabel: "Diff summary", ArtifactValue: "1 file changed",
			}, nil
		case "go_test":
			return projectControlToolResult{
				ToolID: "go_test", ArtifactKind: "test_result", ArtifactOutcome: "pass",
				ArtifactLabel: "Go test", ArtifactValue: "ok",
			}, nil
		default:
			return projectControlToolResult{}, fmt.Errorf("unexpected tool: %s", toolID)
		}
	}
	defer func() { projectControlExecuteTool = previous }()

	// Prepare task at implement phase (plan already done)
	task := projectControlTask{ID: projectControlTaskPanelID, RowVersion: 1}
	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "plan")
	task = completeProjectControlTaskPhaseViaAPI(t, srv, token, task, "plan", "plan", "recorded", "Plan done")
	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "implement")

	// Run diff_capture tool on implement phase — should auto-chain: implement→test(go_test)→review(stop)
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"implement","toolId":"diff_capture"}`, task.RowVersion)
	snapshot := patchProjectControlTask(t, srv, token, task.ID, body)
	updated := projectControlTaskFromSnapshotByID(t, snapshot, task.ID)

	if updated.CurrentPhase != "review" {
		t.Fatalf("CurrentPhase = %q, want review (auto-chained from implement through test)", updated.CurrentPhase)
	}
	if len(toolCalls) < 2 {
		t.Fatalf("toolCalls = %v, want at least [diff_capture, go_test]", toolCalls)
	}
	if toolCalls[0] != "diff_capture" || toolCalls[1] != "go_test" {
		t.Fatalf("toolCalls = %v, want [diff_capture, go_test, ...]", toolCalls)
	}
}

func TestProjectControlStartExecutionLaunchesFirstPhaseAndTool(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	toolCalls := []string{}
	previous := projectControlExecuteTool
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		toolCalls = append(toolCalls, normalizeProjectControlToolID(toolID))
		return projectControlToolResult{}, fmt.Errorf("not implemented for %s", toolID)
	}
	defer func() { projectControlExecuteTool = previous }()

	// start_execution should start the plan phase. Plan requires "plan" artifact
	// but no default tool produces "plan", so no tool is auto-started.
	snapshot := patchProjectControlTask(t, srv, token, projectControlTaskPanelID,
		`{"expectedRowVersion":1,"action":"start_execution"}`)
	task := projectControlTaskFromSnapshotByID(t, snapshot, projectControlTaskPanelID)

	if task.State != "running" {
		t.Fatalf("State = %q, want running", task.State)
	}
	if task.CurrentPhase != "plan" {
		t.Fatalf("CurrentPhase = %q, want plan", task.CurrentPhase)
	}
	if task.RunbookState != "in_progress" {
		t.Fatalf("RunbookState = %q, want in_progress", task.RunbookState)
	}
	// No tool should have been called (plan has no matching tool)
	if len(toolCalls) != 0 {
		t.Fatalf("toolCalls = %v, want empty (no tool for plan phase)", toolCalls)
	}
	// Verify execution_started event
	req := httptest.NewRequest(http.MethodGet, "/api/project-control/events?limit=20", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var eventsResp projectControlEventsResponse
	if err := json.NewDecoder(rec.Body).Decode(&eventsResp); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	foundExecStarted := false
	for _, ev := range eventsResp.Events {
		if ev.Action == "execution_started" {
			foundExecStarted = true
			break
		}
	}
	if !foundExecStarted {
		t.Fatal("expected execution_started event")
	}
}

func TestProjectControlStartExecutionChainsAutoProgressWithTools(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	toolCalls := []string{}
	previous := projectControlExecuteTool
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		toolCalls = append(toolCalls, normalizeProjectControlToolID(toolID))
		switch normalizeProjectControlToolID(toolID) {
		case "diff_capture":
			return projectControlToolResult{
				ToolID: "diff_capture", ArtifactKind: "diff_summary", ArtifactOutcome: "pass",
				ArtifactLabel: "Diff", ArtifactValue: "1 file changed",
			}, nil
		case "go_test":
			return projectControlToolResult{
				ToolID: "go_test", ArtifactKind: "test_result", ArtifactOutcome: "pass",
				ArtifactLabel: "Test", ArtifactValue: "ok",
			}, nil
		default:
			return projectControlToolResult{}, fmt.Errorf("unexpected tool: %s", toolID)
		}
	}
	defer func() { projectControlExecuteTool = previous }()

	// Prepare: start_execution starts plan (no tool). Manually complete plan,
	// then start implement phase with diff_capture. Auto-progress should chain
	// implement(diff_capture) → test(go_test) → review(stop).
	snapshot := patchProjectControlTask(t, srv, token, projectControlTaskPanelID,
		`{"expectedRowVersion":1,"action":"start_execution"}`)
	task := projectControlTaskFromSnapshotByID(t, snapshot, projectControlTaskPanelID)

	// Complete plan manually
	snapshot = patchProjectControlTask(t, srv, token, task.ID,
		fmt.Sprintf(`{"expectedRowVersion":%d,"action":"complete_phase","phaseId":"plan","artifactKind":"plan","artifactOutcome":"recorded","artifactLabel":"Plan","artifactValue":"ok"}`, task.RowVersion))
	task = projectControlTaskFromSnapshotByID(t, snapshot, task.ID)

	// Start implement and run diff_capture — auto-progress should chain through test
	snapshot = patchProjectControlTask(t, srv, token, task.ID,
		fmt.Sprintf(`{"expectedRowVersion":%d,"action":"start_phase","phaseId":"implement"}`, task.RowVersion))
	task = projectControlTaskFromSnapshotByID(t, snapshot, task.ID)

	snapshot = patchProjectControlTask(t, srv, token, task.ID,
		fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"implement","toolId":"diff_capture"}`, task.RowVersion))
	task = projectControlTaskFromSnapshotByID(t, snapshot, task.ID)

	// Should have auto-chained: implement(diff_capture) → test(go_test) → review(stop)
	if task.CurrentPhase != "review" {
		t.Fatalf("CurrentPhase = %q, want review (auto-chained from implement)", task.CurrentPhase)
	}
	if len(toolCalls) < 2 {
		t.Fatalf("toolCalls = %v, want [diff_capture, go_test]", toolCalls)
	}
	if toolCalls[0] != "diff_capture" || toolCalls[1] != "go_test" {
		t.Fatalf("toolCalls = %v, want [diff_capture, go_test]", toolCalls)
	}
}

func TestProjectControlAgentTokenGenerationAndValidation(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	// Generate agent token
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/agent-token", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST agent-token status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var tokenResp map[string]string
	json.NewDecoder(rec.Body).Decode(&tokenResp)
	agentToken := tokenResp["token"]
	if agentToken == "" {
		t.Fatal("agent token is empty")
	}

	// Second call returns same token
	req2 := httptest.NewRequest(http.MethodPost, "/api/project-control/agent-token", nil)
	req2.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec2 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec2, req2)
	var tokenResp2 map[string]string
	json.NewDecoder(rec2.Body).Decode(&tokenResp2)
	if tokenResp2["token"] != agentToken {
		t.Fatalf("second token = %q, want same as first %q", tokenResp2["token"], agentToken)
	}
}

func TestProjectControlAgentGetTaskReturnsActiveTask(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	// Generate agent token
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/project-control/agent-token", nil)
	tokenReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	tokenRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(tokenRec, tokenReq)
	var tokenResp map[string]string
	json.NewDecoder(tokenRec.Body).Decode(&tokenResp)
	agentToken := tokenResp["token"]

	// Start execution to make a task active
	patchProjectControlTask(t, srv, token, projectControlTaskPanelID,
		`{"expectedRowVersion":1,"action":"start_execution"}`)

	// Agent gets task
	req := httptest.NewRequest(http.MethodGet, "/api/agent/v1/task", nil)
	req.Header.Set("Authorization", "Bearer "+agentToken)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET task status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var taskResp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&taskResp)
	if taskResp["taskId"] == "" {
		t.Fatal("agent task response has empty taskId")
	}
	if taskResp["currentPhase"] != "plan" {
		t.Fatalf("currentPhase = %v, want plan", taskResp["currentPhase"])
	}
}

func TestProjectControlAgentSubmitArtifactAdvancesPhase(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	// Generate agent token
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/project-control/agent-token", nil)
	tokenReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	tokenRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(tokenRec, tokenReq)
	var tokenResp map[string]string
	json.NewDecoder(tokenRec.Body).Decode(&tokenResp)
	agentToken := tokenResp["token"]

	// Start execution (starts plan phase)
	patchProjectControlTask(t, srv, token, projectControlTaskPanelID,
		`{"expectedRowVersion":1,"action":"start_execution"}`)

	// Agent submits plan artifact to complete plan phase
	body := `{"taskId":"` + projectControlTaskPanelID + `","phaseId":"plan","artifactKind":"plan","outcome":"recorded","label":"Plan","value":"Agent plan"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/artifact", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST artifact status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Verify task advanced to implement phase
	snapshot := func() projectControlSnapshot {
		r := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, r)
		return decodeProjectControlSnapshot(t, w)
	}()
	task := projectControlTaskFromSnapshotByID(t, snapshot, projectControlTaskPanelID)
	if task.CurrentPhase != "implement" {
		t.Fatalf("CurrentPhase = %q, want implement (advanced from plan)", task.CurrentPhase)
	}
}

func TestProjectControlAgentRequestCheckpointCreatesPendingCheckpoint(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	// Generate agent token
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/project-control/agent-token", nil)
	tokenReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	tokenRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(tokenRec, tokenReq)
	var tokenResp map[string]string
	json.NewDecoder(tokenRec.Body).Decode(&tokenResp)
	agentToken := tokenResp["token"]

	// Agent requests checkpoint
	body := `{"taskId":"` + projectControlTaskPanelID + `","reason":"Need human review of API design"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/checkpoint", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Verify checkpoint exists
	snapshot := func() projectControlSnapshot {
		r := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, r)
		return decodeProjectControlSnapshot(t, w)
	}()
	found := false
	for _, cp := range snapshot.Checkpoints {
		if cp.TaskID == projectControlTaskPanelID && cp.Kind == "agent_request" && cp.Status == "pending" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected pending agent_request checkpoint")
	}
}

func TestProjectControlAgentInvalidTokenReturns401(t *testing.T) {
	srv, _, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/agent/v1/task", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET task with invalid token status = %d, want 401", rec.Code)
	}
}

func TestProjectControlToolCRUDAPI(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	// Create a custom tool
	body := `{"id":"npm_test","name":"NPM Test","command":["npm","test"],"artifactKind":"test_result","artifactLabel":"NPM test","allowedPhases":["test","final_validation"],"timeoutSeconds":300}`
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tools", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tool status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	// Verify tool appears in snapshot
	snapReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapRec, snapReq)
	snapshot := decodeProjectControlSnapshot(t, snapRec)
	found := false
	for _, def := range snapshot.Tools {
		if def.ID == "npm_test" && def.TimeoutSeconds == 300 {
			found = true
		}
	}
	if !found {
		t.Fatal("npm_test tool not found in snapshot after creation")
	}

	// Update the tool
	updateBody := `{"name":"NPM Test Updated","timeoutSeconds":600}`
	req = httptest.NewRequest(http.MethodPut, "/api/project-control/tools/npm_test", strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT tool status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Delete the tool
	req = httptest.NewRequest(http.MethodDelete, "/api/project-control/tools/npm_test", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE tool status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Verify tool is gone
	snapReq = httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapRec = httptest.NewRecorder()
	srv.mux.ServeHTTP(snapRec, snapReq)
	snapshot = decodeProjectControlSnapshot(t, snapRec)
	for _, def := range snapshot.Tools {
		if def.ID == "npm_test" {
			t.Fatal("npm_test tool still present after deletion")
		}
	}
}

func TestProjectControlToolRunRetriesOnExecutionError(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	callCount := 0
	previous := projectControlExecuteTool
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		callCount++
		if callCount == 1 {
			return projectControlToolResult{}, fmt.Errorf("transient error")
		}
		return projectControlToolResult{
			ToolID: "go_test", ArtifactKind: "test_result", ArtifactOutcome: "pass",
			ArtifactLabel: "Go test", ArtifactValue: "ok",
		}, nil
	}
	defer func() { projectControlExecuteTool = previous }()

	task := prepareProjectControlCodeChangeTaskAtTestPhase(t, srv, token)
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"test","toolId":"go_test"}`, task.RowVersion)
	snapshot := patchProjectControlTask(t, srv, token, task.ID, body)
	updated := projectControlTaskFromSnapshotByID(t, snapshot, task.ID)

	// go_test has MaxRetries=1, so first fail should retry and pass
	if callCount != 2 {
		t.Fatalf("callCount = %d, want 2 (1 fail + 1 retry pass)", callCount)
	}
	// After retry passes, should advance past test phase
	if updated.CurrentPhase != "review" {
		t.Fatalf("CurrentPhase = %q, want review (retry succeeded)", updated.CurrentPhase)
	}
	// Should have a retry event
	req := httptest.NewRequest(http.MethodGet, "/api/project-control/events?limit=30", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var eventsResp projectControlEventsResponse
	json.NewDecoder(rec.Body).Decode(&eventsResp)
	foundRetry := false
	for _, ev := range eventsResp.Events {
		if ev.Action == "tool_retry" {
			foundRetry = true
		}
	}
	if !foundRetry {
		t.Fatal("expected tool_retry event")
	}
}

func TestProjectControlToolRunCreatesCheckpointAfterRetriesExhausted(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	previous := projectControlExecuteTool
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		return projectControlToolResult{}, fmt.Errorf("persistent error")
	}
	defer func() { projectControlExecuteTool = previous }()

	task := prepareProjectControlCodeChangeTaskAtTestPhase(t, srv, token)
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"test","toolId":"go_test"}`, task.RowVersion)
	snapshot := patchProjectControlTask(t, srv, token, task.ID, body)

	// go_test MaxRetries=1: first call fails, retry fails → checkpoint
	foundCheckpoint := false
	for _, cp := range snapshot.Checkpoints {
		if cp.TaskID == task.ID && cp.Kind == "tool_failure" && cp.Status == "pending" {
			foundCheckpoint = true
		}
	}
	if !foundCheckpoint {
		t.Fatal("expected tool_failure checkpoint after retries exhausted")
	}
}

func TestProjectControlHealthMonitorDetectsPhaseTimeout(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	// Start execution to get a running phase
	patchProjectControlTask(t, srv, token, projectControlTaskPanelID,
		`{"expectedRowVersion":1,"action":"start_execution"}`)

	// Backdate the phase attempt to simulate timeout
	srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		old := time.Now().UTC().Add(-45 * time.Minute).Format(time.RFC3339)
		for i := range state.PhaseAttempts {
			if state.PhaseAttempts[i].Status == "running" {
				state.PhaseAttempts[i].StartedAt = old
			}
		}
		return nil
	})

	// Run health check
	srv.projectControl.checkHealth("ian", srv.notifHub)

	// Verify health_alert event was recorded
	req := httptest.NewRequest(http.MethodGet, "/api/project-control/events?limit=30", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var eventsResp projectControlEventsResponse
	json.NewDecoder(rec.Body).Decode(&eventsResp)
	found := false
	for _, ev := range eventsResp.Events {
		if ev.Action == "health_alert" && strings.Contains(ev.Detail, "running for") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected health_alert event for phase timeout")
	}
}

func TestProjectControlHealthMonitorDetectsStalledTask(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	// Start execution
	patchProjectControlTask(t, srv, token, projectControlTaskPanelID,
		`{"expectedRowVersion":1,"action":"start_execution"}`)

	// Backdate all events to simulate stall
	srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		old := time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339)
		for i := range state.Events {
			if state.Events[i].TaskID == projectControlTaskPanelID {
				state.Events[i].Timestamp = old
			}
		}
		// Also backdate phase attempt so it doesn't trigger phase timeout
		for i := range state.PhaseAttempts {
			state.PhaseAttempts[i].StartedAt = time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
		}
		return nil
	})

	srv.projectControl.checkHealth("ian", srv.notifHub)

	req := httptest.NewRequest(http.MethodGet, "/api/project-control/events?limit=30", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var eventsResp projectControlEventsResponse
	json.NewDecoder(rec.Body).Decode(&eventsResp)
	found := false
	for _, ev := range eventsResp.Events {
		if ev.Action == "health_alert" && strings.Contains(ev.Detail, "no activity") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected health_alert event for stalled task")
	}
}

func TestProjectControlAgentAsToolWaitsForCallback(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	// Register an agent-kind tool
	toolBody := `{"id":"agent_implement","name":"Agent Implement","kind":"agent","artifactKind":"diff_summary","artifactLabel":"Agent diff","allowedPhases":["implement"],"timeoutSeconds":600}`
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tools", strings.NewReader(toolBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tool status = %d: %s", rec.Code, rec.Body.String())
	}

	// Verify tool is persisted
	verifyReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	verifyReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	verifyRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(verifyRec, verifyReq)
	verifySnap := decodeProjectControlSnapshot(t, verifyRec)
	foundAgent := false
	for _, def := range verifySnap.Tools {
		if def.ID == "agent_implement" && def.isAgent() {
			foundAgent = true
		}
	}
	if !foundAgent {
		t.Fatalf("agent_implement tool not found or not agent kind in snapshot, tools: %+v", verifySnap.Tools)
	}

	// Prepare task at implement phase
	task := projectControlTask{ID: projectControlTaskPanelID, RowVersion: 1}
	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "plan")
	task = completeProjectControlTaskPhaseViaAPI(t, srv, token, task, "plan", "plan", "recorded", "Plan done")
	task = startProjectControlTaskPhaseViaAPI(t, srv, token, task, "implement")

	// Run the agent tool
	body := fmt.Sprintf(`{"expectedRowVersion":%d,"action":"run_tool","phaseId":"implement","toolId":"agent_implement"}`, task.RowVersion)
	snapshot := patchProjectControlTask(t, srv, token, task.ID, body)

	// Tool run should be in "waiting" status
	foundWaiting := false
	for _, run := range snapshot.ToolRuns {
		if run.TaskID == task.ID && run.ToolID == "agent_implement" {
			if run.Status == "waiting" {
				foundWaiting = true
			} else {
				t.Fatalf("agent tool run status = %q, want waiting (all runs: %+v)", run.Status, snapshot.ToolRuns)
			}
		}
	}
	if !foundWaiting {
		t.Fatalf("expected agent tool run in waiting status, tool runs: %+v", snapshot.ToolRuns)
	}

	// Generate agent token and submit artifact via agent API
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/project-control/agent-token", nil)
	tokenReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	tokenRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(tokenRec, tokenReq)
	var tokenResp map[string]string
	json.NewDecoder(tokenRec.Body).Decode(&tokenResp)
	agentToken := tokenResp["token"]

	artifactBody := `{"taskId":"` + task.ID + `","phaseId":"implement","artifactKind":"diff_summary","outcome":"pass","label":"Agent diff","value":"1 file changed"}`
	artReq := httptest.NewRequest(http.MethodPost, "/api/agent/v1/artifact", strings.NewReader(artifactBody))
	artReq.Header.Set("Authorization", "Bearer "+agentToken)
	artReq.Header.Set("Content-Type", "application/json")
	artRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(artRec, artReq)
	if artRec.Code != http.StatusOK {
		t.Fatalf("POST artifact status = %d: %s", artRec.Code, artRec.Body.String())
	}

	// Verify tool run completed and phase advanced
	snapReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	snapRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapRec, snapReq)
	finalSnapshot := decodeProjectControlSnapshot(t, snapRec)
	finalTask := projectControlTaskFromSnapshotByID(t, finalSnapshot, task.ID)
	if finalTask.CurrentPhase != "test" {
		t.Fatalf("CurrentPhase = %q, want test (advanced after agent callback)", finalTask.CurrentPhase)
	}
	foundCompleted := false
	for _, run := range finalSnapshot.ToolRuns {
		if run.TaskID == task.ID && run.ToolID == "agent_implement" && run.Status == "completed" {
			foundCompleted = true
		}
	}
	if !foundCompleted {
		t.Fatal("expected agent tool run to be completed after artifact submission")
	}
}

func TestProjectControlScheduledToolRunsMatchingToolForRunningPhase(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	setProjectControlRunToolAsyncForTest(t, func(fn func()) { fn() })

	previous := projectControlExecuteTool
	toolCalls := []string{}
	projectControlExecuteTool = func(toolID, workspaceDir string) (projectControlToolResult, error) {
		toolCalls = append(toolCalls, normalizeProjectControlToolID(toolID))
		return projectControlToolResult{
			ToolID: toolID, ArtifactKind: "test_result", ArtifactOutcome: "pass",
			ArtifactLabel: "Test", ArtifactValue: "ok",
		}, nil
	}
	defer func() { projectControlExecuteTool = previous }()

	// Prepare task at test phase with a running attempt
	task := prepareProjectControlCodeChangeTaskAtTestPhase(t, srv, token)
	_ = task

	// Run scheduled tools — should find go_test matches test phase
	srv.projectControl.runScheduledTools("ian")

	if len(toolCalls) == 0 {
		t.Fatal("expected scheduled tool run, got none")
	}
	if toolCalls[0] != "go_test" {
		t.Fatalf("toolCalls[0] = %q, want go_test", toolCalls[0])
	}
}
