package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
)

// projectMemoryServer starts a server with one project and returns the
// project id together with the session cookie and the agent token.
func projectMemoryServer(t *testing.T) (*Server, string, string, *auth.SessionManager) {
	t.Helper()

	srv, token, sessions := testProjectControlServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/projects",
		strings.NewReader(`{"name":"MemoryProj","currentGoal":"统一记忆","repoPath":"/tmp",`+
			`"brief":{"understanding":"记住只读一份","deliverables":"文档"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("create project = %d: %s", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		ActiveProjectID string `json:"activeProjectId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ActiveProjectID == "" {
		t.Fatal("no active project id")
	}
	return srv, snapshot.ActiveProjectID, token, sessions
}

func TestProjectMemorySeededFromBrief(t *testing.T) {
	srv, projectID, token, sessions := projectMemoryServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/project-control/memory?project="+projectID, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var memory projectControlProjectMemory
	if err := json.Unmarshal(rec.Body.Bytes(), &memory); err != nil {
		t.Fatal(err)
	}
	if memory.Revision != 1 {
		t.Errorf("revision = %d, want 1", memory.Revision)
	}
	for _, want := range []string{"统一记忆", "记住只读一份", "文档"} {
		if !strings.Contains(memory.Content, want) {
			t.Errorf("seed memory missing %q:\n%s", want, memory.Content)
		}
	}
}

func TestProjectMemoryAppendAndReplace(t *testing.T) {
	srv, projectID, token, sessions := projectMemoryServer(t)
	defer sessions.Stop()

	post := func(mode, content string) projectControlProjectMemory {
		req := httptest.NewRequest(http.MethodPost, "/api/project-control/memory",
			strings.NewReader(`{"projectId":"`+projectID+`","content":`+mustJSON(content)+`,"mode":"`+mode+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var memory projectControlProjectMemory
		if err := json.Unmarshal(rec.Body.Bytes(), &memory); err != nil {
			t.Fatal(err)
		}
		return memory
	}

	appended := post("append", "网关统一到 3425")
	if appended.Revision != 2 {
		t.Errorf("revision = %d, want 2", appended.Revision)
	}
	if !strings.Contains(appended.Content, "网关统一到 3425") {
		t.Errorf("appended content missing:\n%s", appended.Content)
	}
	if !strings.Contains(appended.Content, "统一记忆") {
		t.Errorf("append dropped the previous content:\n%s", appended.Content)
	}

	replaced := post("replace", "只保留这一条")
	if replaced.Revision != 3 {
		t.Errorf("revision = %d, want 3", replaced.Revision)
	}
	if strings.Contains(replaced.Content, "网关统一到 3425") {
		t.Errorf("replace kept the old content:\n%s", replaced.Content)
	}
}

func TestProjectMemoryRejectsUnknownProject(t *testing.T) {
	srv, _, token, sessions := projectMemoryServer(t)
	_ = token
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/memory",
		strings.NewReader(`{"projectId":"project-nope","content":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestProjectMemoryRequiresAuth(t *testing.T) {
	srv, projectID, _, sessions := projectMemoryServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/project-control/memory?project="+projectID, nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}

	// The agent route needs the agent token.
	req = httptest.NewRequest(http.MethodGet, "/api/agent/v1/memory?project="+projectID, nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("agent status=%d, want 401", rec.Code)
	}
}

func TestAgentMemoryReadsAndWritesProjectMemory(t *testing.T) {
	srv, projectID, _, sessions := projectMemoryServer(t)
	defer sessions.Stop()

	agentToken, err := srv.projectControl.ensureAgentToken("ian")
	if err != nil {
		t.Fatalf("ensureAgentToken error: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/memory",
		strings.NewReader(`{"projectId":"`+projectID+`","content":"agent 追加的事实","mode":"append"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+agentToken)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var memory projectControlProjectMemory
	if err := json.Unmarshal(rec.Body.Bytes(), &memory); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(memory.Content, "agent 追加的事实") {
		t.Fatalf("content missing:\n%s", memory.Content)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/agent/v1/memory?project="+projectID, nil)
	req.Header.Set("Authorization", "Bearer "+agentToken)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &memory); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(memory.Content, "agent 追加的事实") {
		t.Fatalf("agent read lost the content:\n%s", memory.Content)
	}
}

func TestProjectMemorySurvivesReload(t *testing.T) {
	srv, projectID, token, sessions := projectMemoryServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/memory",
		strings.NewReader(`{"projectId":"`+projectID+`","content":"持久化的事实","mode":"append"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// The state file must carry the memory.
	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatalf("loadOrSeed error: %v", err)
	}
	found := false
	for _, memory := range state.ProjectMemories {
		if memory.ProjectID == projectID && strings.Contains(memory.Content, "持久化的事实") {
			found = true
		}
	}
	if !found {
		t.Fatal("project memory was not persisted")
	}
}

func TestProjectMemoryIsProjectScoped(t *testing.T) {
	srv, projectID, token, sessions := projectMemoryServer(t)
	defer sessions.Stop()

	// A second project gets its own memory.
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/projects",
		strings.NewReader(`{"name":"Other","currentGoal":"别的项目","repoPath":"/tmp"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		ActiveProjectID string `json:"activeProjectId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/project-control/memory?project="+projectID, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var memory projectControlProjectMemory
	if err := json.Unmarshal(rec.Body.Bytes(), &memory); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(memory.Content, "别的项目") {
		t.Fatalf("memory leaked across projects:\n%s", memory.Content)
	}
}

// mustJSON encodes a value for embedding in a request body.
func mustJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(data)
}

func TestClaimPayloadCarriesProjectMemory(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	agentToken := agentTokenFor(t, srv, token)

	// A project with a canonical memory.
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/projects",
		strings.NewReader(`{"name":"ClaimProj","currentGoal":"带上记忆","brief":{"understanding":"共享事实"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("create project = %d: %s", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		ActiveProjectID string `json:"activeProjectId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}

	// Attach a task to that project and make it claimable.
	_, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Projects {
			if state.Projects[i].ID == snapshot.ActiveProjectID {
				state.Projects[i].Takeover = projectTakeover{Mode: "active"}
				state.Projects[i].Lifecycle = projectLifecycle{State: "executing"}
			}
		}
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: "task-memory-1", ProjectID: snapshot.ActiveProjectID, Title: "记忆随任务",
			State: "queued", DispatchState: "ready", AcceptanceCommand: []string{"/bin/true"},
			Timeline: []projectControlEvent{}, Audit: []projectControlAuditItem{},
		})
		return nil
	})
	if err != nil {
		t.Fatalf("prepare task: %v", err)
	}

	rec = agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"runtime-1","agentLabel":"claude"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	memory, ok := payload["projectMemory"].(map[string]interface{})
	if !ok {
		t.Fatalf("claim payload missing projectMemory: %s", rec.Body.String())
	}
	content, _ := memory["content"].(string)
	if !strings.Contains(content, "共享事实") {
		t.Fatalf("project memory content missing in claim payload:\n%s", content)
	}
	if memory["projectId"] != snapshot.ActiveProjectID {
		t.Errorf("projectMemory.projectId = %v, want %v", memory["projectId"], snapshot.ActiveProjectID)
	}
}
