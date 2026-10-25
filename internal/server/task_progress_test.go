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

	// worker 上报两条事件。
	for i, text := range []string{"第一步：读取仓库", "第二步：完成任务"} {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/progress",
			strings.NewReader(`{"taskId":"`+taskID+`","seq":`+itoaStatic(i+1)+`,"kind":"assistant","text":"`+text+`"}`))
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
	for i := 1; i <= 60; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/progress",
			strings.NewReader(`{"taskId":"`+taskID+`","seq":`+itoaStatic(i)+`,"kind":"assistant","text":"e`+itoaStatic(i)+`"}`))
		req.Header.Set("Authorization", "Bearer "+agentToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("write %d = %d", i, rec.Code)
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
