package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/controlplane"
)

// P4 handoff: 复制→粘贴→自动建卡关联 的完整链路。
func TestHandoffCopyPasteCreatesReviewTask(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)

	// 造源项目/工作流/源任务 A（运行中）并把它绑到终端 T1。
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Projects = append(state.Projects, projectControlProject{
			ID: "project-handoff", Name: "HO", Takeover: projectTakeover{Mode: "manual"},
		})
		state.Workstreams = append(state.Workstreams, projectControlWorkstream{
			ID: "ws-handoff", ProjectID: "project-handoff", Title: "WS",
		})
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: "task-src-a", ProjectID: "project-handoff",
			WorkstreamID: "ws-handoff",
			Title:        "原任务 A", State: "running", DispatchState: "claimed",
			SessionIDs: []string{"session-live-t1"},
			Timeline:   []projectControlEvent{}, Audit: []projectControlAuditItem{},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		return rec
	}

	// 1. 复制：记录来源（T1 属于源任务 A）。
	rec := post("/api/terminal-handoff/copy",
		`{"terminalId":"t1","text":"请审查这段专利披露文本"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("copy = %d: %s", rec.Code, rec.Body.String())
	}
	var copyBody struct {
		SourceTaskID string `json:"sourceTaskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &copyBody); err != nil {
		t.Fatal(err)
	}
	if copyBody.SourceTaskID != "task-src-a" {
		t.Fatalf("sourceTaskId = %q, want task-src-a", copyBody.SourceTaskID)
	}

	// 2. 粘贴到新终端 T2（无绑定任务）→ 自动创建审查任务卡。
	rec = post("/api/terminal-handoff/paste",
		`{"terminalId":"t2-new","text":"请审查这段专利披露文本"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("paste = %d: %s", rec.Code, rec.Body.String())
	}
	var pasteBody struct {
		Matched      bool   `json:"matched"`
		TaskID       string `json:"taskId"`
		SourceTaskID string `json:"sourceTaskId"`
		Relation     string `json:"relation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pasteBody); err != nil {
		t.Fatal(err)
	}
	if !pasteBody.Matched || pasteBody.TaskID == "" || pasteBody.SourceTaskID != "task-src-a" {
		t.Fatalf("paste = %+v", pasteBody)
	}

	// 3. 审查任务卡存在、planned（不自动执行）、来源关系为 reviews。
	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	reviewTask, ok := controlTask(state, pasteBody.TaskID)
	if !ok {
		t.Fatal("review task missing")
	}
	if reviewTask.State != "planned" {
		t.Errorf("review task state = %q, want planned (not auto-running)", reviewTask.State)
	}
	refs, err := srv.controlPlane.ListSourceRefs(context.Background(), "ian", pasteBody.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	foundRef := false
	for _, ref := range refs {
		if ref.SourceTaskID == "task-src-a" && ref.Relation == "reviews" {
			foundRef = true
		}
	}
	if !foundRef {
		t.Fatalf("source ref missing: %+v", refs)
	}

	// 4. 再粘贴同内容到同终端 → 不重复建卡。
	rec = post("/api/terminal-handoff/paste",
		`{"terminalId":"t2-new","text":"请审查这段专利披露文本"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("paste 2 = %d", rec.Code)
	}
	var paste2 struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &paste2); err != nil {
		t.Fatal(err)
	}
	if paste2.TaskID != pasteBody.TaskID {
		t.Fatalf("duplicate review task: %s vs %s", paste2.TaskID, pasteBody.TaskID)
	}
}

// 只粘贴未匹配（外部文本）→ 不建卡。
func TestHandoffUnmatchedPasteCreatesNothing(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/api/terminal-handoff/paste",
		strings.NewReader(`{"terminalId":"t9","text":"与任何复制无关的外部文本"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("paste = %d", rec.Code)
	}
	var body struct {
		Matched bool   `json:"matched"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Matched {
		t.Fatal("unmatched paste must not create a task")
	}
	if body.Error != "" {
		t.Fatalf("paste error: %s", body.Error)
	}
}

// 撤销关联。
func TestP3SourceRefDelete(t *testing.T) {
	srv, token, sessions := progressServer(t)
	defer sessions.Stop()
	taskID := createProgressTask(t, srv, token, "撤销关联任务")

	if _, err := srv.controlPlane.AddSourceRef(context.Background(), "ian", controlplane.SourceRef{
		TargetTaskID: taskID,
		SourceTaskID: "task-src",
		Relation:     "reviews",
	}); err != nil {
		t.Fatal(err)
	}

	refs, err := srv.controlPlane.ListSourceRefs(context.Background(), "ian", taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 {
		t.Fatal("ref not recorded")
	}
	del := httptest.NewRequest(http.MethodPost, "/api/source-links/"+refs[0].ID, nil)
	del.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, del)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d", rec.Code)
	}
	refs, err = srv.controlPlane.ListSourceRefs(context.Background(), "ian", taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatal("ref not deleted")
	}
}
