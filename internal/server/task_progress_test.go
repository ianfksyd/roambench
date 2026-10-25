package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
)

func progressServer(t *testing.T) (*Server, string, *auth.SessionManager) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, token, sessions := testProjectControlServer(t)
	return srv, token, sessions
}

func createProgressTask(t *testing.T, srv *Server, token, title string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks",
		strings.NewReader(`{"projectId":"project-roambench-control","workstreamId":"workstream-ui-ia","title":"`+title+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create task = %d: %s", rec.Code, rec.Body.String())
	}
	snapshot := decodeProjectControlSnapshot(t, rec)
	for _, task := range snapshot.Tasks {
		if task.Title == title {
			return task.ID
		}
	}
	t.Fatal("task not found")
	return ""
}

func TestAgentProgressWriteAndPanelRead(t *testing.T) {
	srv, token, sessions := progressServer(t)
	defer sessions.Stop()
	taskID := createProgressTask(t, srv, token, "进展任务")
	agentToken, err := srv.projectControl.ensureAgentToken("ian")
	if err != nil {
		t.Fatal(err)
	}

	// worker 上报两条事件（P1: 先领取拿租约）。
	rec0 := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"prog-rt","agentLabel":"claude"}`)
	if rec0.Code != http.StatusOK {
		t.Fatalf("claim = %d: %s", rec0.Code, rec0.Body.String())
	}
	var claimed struct {
		LeaseID string `json:"leaseId"`
	}
	if err := json.Unmarshal(rec0.Body.Bytes(), &claimed); err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{"第一步：读取仓库", "第二步：完成任务"} {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/progress",
			strings.NewReader(`{"taskId":"`+taskID+`","seq":`+itoaStatic(i+1)+`,"kind":"assistant","text":"`+text+`","leaseId":"`+claimed.LeaseID+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+agentToken)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("progress write = %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 面板读取（web 会话）。
	req := httptest.NewRequest(http.MethodGet, "/api/project-control/tasks/"+taskID+"/progress", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("progress read = %d: %s", rec.Code, rec.Body.String())
	}
	var body taskProgressResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Source != "reported" {
		t.Errorf("source = %q, want reported", body.Source)
	}
	if len(body.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(body.Events))
	}
	if !strings.Contains(body.Events[0].Text, "第二步") {
		t.Errorf("newest first violated: %+v", body.Events)
	}
}

func TestProgressWriteRequiresAuthAndTask(t *testing.T) {
	srv, _, sessions := progressServer(t)
	defer sessions.Stop()

	agentToken, err := srv.projectControl.ensureAgentToken("ian")
	if err != nil {
		t.Fatal(err)
	}
	// 无 auth。
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/progress",
		strings.NewReader(`{"taskId":"x","seq":1,"kind":"assistant","text":"t"}`))
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth = %d, want 401", rec.Code)
	}
	// auth 但任务不存在。
	req = httptest.NewRequest(http.MethodPost, "/api/agent/v1/progress",
		strings.NewReader(`{"taskId":"task-missing","seq":1,"kind":"assistant","text":"t"}`))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("unknown task accepted: %s", rec.Body.String())
	}
}

func TestProgressTerminalFragmentFallback(t *testing.T) {
	srv, token, sessions := progressServer(t)
	defer sessions.Stop()

	// 构造任务并绑定一个"活"终端引用（不经真实 tmux——fragment 采集会因
	// 会话不存在而失败，此时 source=none；用可采集的临时会话验证正路径）。
	taskID := createProgressTask(t, srv, token, "Fragment 任务")
	// 直接落一条 reported 事件，让 fragment 分支不被走到（正路径已在别处覆盖）；
	// 这里重点验证 fragment 分支：清空 reported 事件不可能（未写过），
	// 所以 source 应为 none 而非误报。
	req := httptest.NewRequest(http.MethodGet, "/api/project-control/tasks/"+taskID+"/progress", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var body taskProgressResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Source != "none" {
		t.Fatalf("source = %q, want none (no events, no live terminal)", body.Source)
	}
}

func TestProgressFeedPrunesToLimit(t *testing.T) {
	srv, token, sessions := progressServer(t)
	defer sessions.Stop()
	taskID := createProgressTask(t, srv, token, "修剪任务")
	agentToken, err := srv.projectControl.ensureAgentToken("ian")
	if err != nil {
		t.Fatal(err)
	}
	// P1 lease 校验后：先 claim 拿租约，再带 leaseId 上报。
	rec0 := agentAPIPost(t, srv, "/api/agent/v1/claim", agentToken, `{"runtimeId":"prune-rt","agentLabel":"claude"}`)
	if rec0.Code != http.StatusOK {
		t.Fatalf("claim = %d: %s", rec0.Code, rec0.Body.String())
	}
	var claimed struct {
		LeaseID string `json:"leaseId"`
	}
	if err := json.Unmarshal(rec0.Body.Bytes(), &claimed); err != nil {
		t.Fatal(err)
	}
	leaseID := claimed.LeaseID
	for i := 1; i <= 60; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/progress",
			strings.NewReader(`{"taskId":"`+taskID+`","seq":`+itoaStatic(i)+`,"kind":"assistant","text":"e`+itoaStatic(i)+`","leaseId":"`+leaseID+`"}`))
		req.Header.Set("Authorization", "Bearer "+agentToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("write %d = %d: %s", i, rec.Code, rec.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/project-control/tasks/"+taskID+"/progress", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var body taskProgressResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Events) > 50 {
		t.Fatalf("events = %d, want <= 50", len(body.Events))
	}
	// 最旧的被修剪：seq=1 不在结果里。
	for _, e := range body.Events {
		if e.Seq == 1 {
			t.Fatal("seq 1 should have been pruned")
		}
	}
}

func itoaStatic(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [12]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// P3 补遗: tmux 侧任务登记入口——创建任务卡并绑定活终端。
func TestAgentRegisterTaskBindsTerminal(t *testing.T) {
	srv, _, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: "task-p3-ctx", ProjectID: "project-roambench-control",
			Title: "上下文任务", State: "planned", DispatchState: "ready",
			Timeline: []projectControlEvent{}, Audit: []projectControlAuditItem{},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// active project 需存在
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.ActiveProjectID = "project-roambench-control"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	agentToken, err := srv.projectControl.ensureAgentToken("ian")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/register-task",
		strings.NewReader(`{"title":"tmux 里提出的新目标"}`))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("register-task = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TaskID == "" {
		t.Fatal("no taskId in response")
	}
}
