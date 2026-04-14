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
}

func TestProjectControlCheckpointDecisionApproveUpdatesSnapshot(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"start_execution"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH start execution status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":2,"action":"mark_execution_complete"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH execution complete status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":3,"action":"mark_ready_for_acceptance"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH ready for acceptance status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":4,"action":"request_acceptance_review"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH request acceptance review status = %d, want %d", rec.Code, http.StatusOK)
	}

	checkpointID := ""
	readySnapshot := decodeProjectControlSnapshot(t, rec)
	for _, checkpoint := range readySnapshot.Checkpoints {
		if checkpoint.TaskID == projectControlTaskPanelID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			checkpointID = checkpoint.ID
			break
		}
	}
	if checkpointID == "" {
		t.Fatal("expected pending final_acceptance checkpoint for task")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+checkpointID+"/decision", strings.NewReader(`{"action":"approve"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()

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
	if taskRec.Code != http.StatusOK {
		t.Fatalf("PATCH task ready action status = %d, want %d", taskRec.Code, http.StatusOK)
	}
	taskSnapshot := decodeProjectControlSnapshot(t, taskRec)
	for _, task := range taskSnapshot.Tasks {
		if task.ID == projectControlTaskPanelID {
			if task.State != "execution_complete" || task.AcceptanceStatus != "ready_for_acceptance" || task.RowVersion != 4 {
				t.Fatalf("task after actions = %#v, want execution_complete + ready_for_acceptance + rowVersion=4", task)
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

	req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"start_execution"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH start execution status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":2,"action":"mark_execution_complete"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH execution complete status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":3,"action":"mark_ready_for_acceptance"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH ready for acceptance status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":4,"action":"request_acceptance_review"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH request acceptance review status = %d, want %d", rec.Code, http.StatusOK)
	}
	snapshot := decodeProjectControlSnapshot(t, rec)

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

	req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":1,"action":"start_execution"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH start execution status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":2,"action":"mark_execution_complete"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH execution complete status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":3,"action":"mark_ready_for_acceptance"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH ready for acceptance status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":4,"action":"request_acceptance_review"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH request acceptance review status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+projectControlTaskPanelID, strings.NewReader(`{"expectedRowVersion":5,"action":"reopen_task"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
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

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/project-control/events?taskId="+projectControlTaskPanelID+"&limit=20", nil)
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

	steps := []string{
		`{"expectedRowVersion":1,"action":"start_execution"}`,
		`{"expectedRowVersion":2,"action":"mark_execution_complete"}`,
		`{"expectedRowVersion":3,"action":"mark_ready_for_acceptance"}`,
		`{"expectedRowVersion":4,"action":"request_acceptance_review"}`,
	}
	for _, body := range steps {
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

	steps := []string{
		`{"expectedRowVersion":1,"action":"start_execution"}`,
		`{"expectedRowVersion":2,"action":"mark_execution_complete"}`,
		`{"expectedRowVersion":3,"action":"mark_ready_for_acceptance"}`,
		`{"expectedRowVersion":4,"action":"request_acceptance_review"}`,
	}
	for _, body := range steps {
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
