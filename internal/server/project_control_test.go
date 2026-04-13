package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+projectControlCheckpointAcceptanceID+"/decision", strings.NewReader(`{"action":"approve"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint decision status = %d, want %d", rec.Code, http.StatusOK)
	}

	snapshot := decodeProjectControlSnapshot(t, rec)
	if snapshot.ApprovalsCount != 0 {
		t.Fatalf("ApprovalsCount = %d, want 0 after approval", snapshot.ApprovalsCount)
	}
	if got := snapshot.Checkpoints[0].Status; got != "approved" {
		t.Fatalf("Checkpoint status = %q, want approved", got)
	}

	var acceptanceStatus string
	for _, task := range snapshot.Tasks {
		if task.ID == projectControlTaskIAID {
			acceptanceStatus = task.AcceptanceStatus
			break
		}
	}
	if acceptanceStatus != "accepted" {
		t.Fatalf("IA acceptance status = %q, want accepted", acceptanceStatus)
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
