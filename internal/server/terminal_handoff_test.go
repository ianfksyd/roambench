package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/controlplane"
	"github.com/ianf339/roambench/internal/terminal"
)

// handoffFixture is a server with a real terminal manager: handoff
// endpoints must validate live sessions, so fixtures with fake terminal
// ids would hide missing checks.
type handoffFixture struct {
	srv   *Server
	token string
	mgr   *terminal.Manager
	t1    terminal.SessionInfo // bound to task A (the copy source)
	t2    terminal.SessionInfo // unregistered paste target
	taskA string
}

func newHandoffFixture(t *testing.T) *handoffFixture {
	t.Helper()
	srv, token, sessions := progressServer(t)
	t.Cleanup(func() { sessions.Stop() })
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	mgr := terminal.NewManager(&srv.cfg.Terminal)
	t.Cleanup(func() { mgr.Stop() })
	srv.terminals = mgr

	dir1, dir2 := t.TempDir(), t.TempDir()
	t1, err := mgr.CreateSessionWithOptions("ian", terminal.SessionCreateOptions{WorkDir: dir1, Name: "handoff-src"})
	if err != nil {
		t.Fatal(err)
	}
	t2, err := mgr.CreateSessionWithOptions("ian", terminal.SessionCreateOptions{WorkDir: dir2, Name: "handoff-dst"})
	if err != nil {
		t.Fatal(err)
	}

	// Source project with task A bound to t1.
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
			SessionIDs: []string{"session-live-" + t1.ID},
			Timeline:   []projectControlEvent{}, Audit: []projectControlAuditItem{},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	return &handoffFixture{srv: srv, token: token, mgr: mgr, t1: t1, t2: t2, taskA: "task-src-a"}
}

func (f *handoffFixture) post(t *testing.T, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: f.token})
	rec := httptest.NewRecorder()
	f.srv.mux.ServeHTTP(rec, req)
	return rec
}

func (f *handoffFixture) copy(t *testing.T, terminalID, text string) *httptest.ResponseRecorder {
	t.Helper()
	return f.post(t, "/api/terminal-handoff/copy",
		fmt.Sprintf(`{"terminalId":%q,"text":%q}`, terminalID, text))
}

func (f *handoffFixture) paste(t *testing.T, terminalID, text string) *httptest.ResponseRecorder {
	t.Helper()
	return f.post(t, "/api/terminal-handoff/paste",
		fmt.Sprintf(`{"terminalId":%q,"text":%q}`, terminalID, text))
}

func (f *handoffFixture) taskCountForTerminal(t *testing.T, terminalID string) int {
	t.Helper()
	state, err := f.srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	return len(tasksForTerminalInState(state, terminalID))
}

func tasksForTerminalInState(state projectControlState, terminalID string) []projectControlTask {
	sessionID := "session-live-" + terminalID
	var out []projectControlTask
	for _, task := range state.Tasks {
		for _, sid := range task.SessionIDs {
			if sid == sessionID {
				out = append(out, task)
				break
			}
		}
	}
	return out
}

// 完整链路：复制 → 粘贴到未登记终端 → 此时不建卡；register-task 提交后
// 才创建目标卡并提升 reviews 来源关系。
func TestHandoffPasteWaitsForSubmissionBeforeCard(t *testing.T) {
	f := newHandoffFixture(t)

	if rec := f.copy(t, f.t1.ID, "请审查这段专利披露文本"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d: %s", rec.Code, rec.Body.String())
	}
	rec := f.paste(t, f.t2.ID, "请审查这段专利披露文本")
	if rec.Code != http.StatusOK {
		t.Fatalf("paste = %d: %s", rec.Code, rec.Body.String())
	}
	var pasteBody struct {
		Matched bool   `json:"matched"`
		Pending bool   `json:"pending"`
		TaskID  string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pasteBody); err != nil {
		t.Fatal(err)
	}
	if !pasteBody.Matched || !pasteBody.Pending || pasteBody.TaskID != "" {
		t.Fatalf("paste = %+v, want matched+pending without a card", pasteBody)
	}
	// 粘贴本身不产生任何卡（普通 shell 只粘贴未提交 → 无空卡片）。
	if n := f.taskCountForTerminal(t, f.t2.ID); n != 0 {
		t.Fatalf("paste created %d tasks before submission", n)
	}

	// 验证过的提交事件：agent 在目标终端登记目标 → 建卡 + 提升来源关系。
	agentToken := agentTokenFor(t, f.srv, f.token)
	regBody, _ := json.Marshal(registerTaskRequest{
		Title: "审查专利披露文本", TerminalID: f.t2.ID,
	})
	rec = agentAPIPost(t, f.srv, "/api/agent/v1/register-task", agentToken, string(regBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("register-task = %d: %s", rec.Code, rec.Body.String())
	}
	var reg struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reg); err != nil {
		t.Fatal(err)
	}

	state, err := f.srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := controlTask(state, reg.TaskID); !ok {
		t.Fatal("submitted task not persisted")
	}
	if n := f.taskCountForTerminal(t, f.t2.ID); n != 1 {
		t.Fatalf("terminal bound to %d tasks after one submission", n)
	}
	refs, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", reg.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ref := range refs {
		if ref.SourceTaskID == f.taskA && ref.Relation == "reviews" && ref.Snapshot == "请审查这段专利披露文本" {
			found = true
		}
	}
	if !found {
		t.Fatalf("promoted source ref missing: %+v", refs)
	}
}

// 未登记来源：关系写入前必须把来源登记持久化，不能留下悬空引用。
func TestHandoffRegistersUnregisteredSource(t *testing.T) {
	f := newHandoffFixture(t)
	// 从未绑定任务的 t2 复制（来源终端没有任务），粘贴到另一个未登记终端 t3。
	t3, err := f.mgr.CreateSessionWithOptions("ian", terminal.SessionCreateOptions{WorkDir: t.TempDir(), Name: "handoff-t3"})
	if err != nil {
		t.Fatal(err)
	}
	if rec := f.copy(t, f.t2.ID, "未登记来源的内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d: %s", rec.Code, rec.Body.String())
	}
	rec := f.paste(t, t3.ID, "未登记来源的内容")
	if rec.Code != http.StatusOK {
		t.Fatalf("paste = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		SourceTaskID string `json:"sourceTaskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	state, err := f.srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := controlTask(state, body.SourceTaskID); !ok {
		t.Fatalf("source task %q not persisted", body.SourceTaskID)
	}
}

// 并发重复粘贴：同一目标、同一内容只留下一条 pending，提交后只有一张卡一条关系。
func TestHandoffConcurrentPastesDeduplicate(t *testing.T) {
	f := newHandoffFixture(t)
	if rec := f.copy(t, f.t1.ID, "并发粘贴内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d", rec.Code)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rec := f.paste(t, f.t2.ID, "并发粘贴内容"); rec.Code != http.StatusOK {
				errs <- fmt.Errorf("paste = %d: %s", rec.Code, rec.Body.String())
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	pendings, err := f.srv.controlPlane.ListPendingHandoffs(context.Background(), "ian", f.t2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendings) != 1 {
		t.Fatalf("pending handoffs = %d, want 1", len(pendings))
	}

	agentToken := agentTokenFor(t, f.srv, f.token)
	regBody, _ := json.Marshal(registerTaskRequest{Title: "并发目标", TerminalID: f.t2.ID})
	if rec := agentAPIPost(t, f.srv, "/api/agent/v1/register-task", agentToken, string(regBody)); rec.Code != http.StatusOK {
		t.Fatalf("register-task = %d: %s", rec.Code, rec.Body.String())
	}
	if n := f.taskCountForTerminal(t, f.t2.ID); n != 1 {
		t.Fatalf("terminal bound to %d tasks after concurrent pastes", n)
	}
	state, err := f.srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	bound := tasksForTerminalInState(state, f.t2.ID)
	refs, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", bound[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].SourceTaskID != f.taskA {
		t.Fatalf("refs = %+v, want exactly one reviews ref to %s", refs, f.taskA)
	}
}

// 终端校验：不存在的来源/目标终端一律拒绝，不制造虚构绑定。
func TestHandoffRejectsUnknownTerminals(t *testing.T) {
	f := newHandoffFixture(t)
	if rec := f.copy(t, "ghost-terminal", "内容"); rec.Code != http.StatusNotFound {
		t.Fatalf("copy ghost = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := f.paste(t, "ghost-terminal", "内容"); rec.Code != http.StatusNotFound {
		t.Fatalf("paste ghost = %d: %s", rec.Code, rec.Body.String())
	}
	// 真实复制后粘贴到幽灵终端：同样拒绝，且不落任何 pending。
	if rec := f.copy(t, f.t1.ID, "真实来源内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d", rec.Code)
	}
	if rec := f.paste(t, "ghost-terminal", "真实来源内容"); rec.Code != http.StatusNotFound {
		t.Fatalf("paste ghost after real copy = %d", rec.Code)
	}
	pendings, err := f.srv.controlPlane.ListPendingHandoffs(context.Background(), "ian", "ghost-terminal")
	if err != nil {
		t.Fatal(err)
	}
	if len(pendings) != 0 {
		t.Fatalf("ghost terminal accumulated %d pendings", len(pendings))
	}
}

// 同终端复制再粘贴：不构成跨会话交接，且重复粘贴不会伪造来源。
func TestHandoffSameTerminalPasteNeverMatches(t *testing.T) {
	f := newHandoffFixture(t)
	if rec := f.copy(t, f.t1.ID, "同终端内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d", rec.Code)
	}
	for i := 0; i < 2; i++ {
		rec := f.paste(t, f.t1.ID, "同终端内容")
		if rec.Code != http.StatusOK {
			t.Fatalf("paste = %d", rec.Code)
		}
		var body struct {
			Matched bool `json:"matched"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Matched {
			t.Fatalf("same-terminal paste matched itself (round %d)", i)
		}
	}
	if n := f.taskCountForTerminal(t, f.t1.ID); n != 1 {
		t.Fatalf("same-terminal pastes produced %d extra bindings", n-1)
	}
}

// 外部文本粘贴：不匹配、不记录，连续粘贴也不会把第一次粘贴当成复制来源。
func TestHandoffUnmatchedPasteCreatesNothing(t *testing.T) {
	f := newHandoffFixture(t)
	for i := 0; i < 2; i++ {
		rec := f.paste(t, f.t2.ID, "与任何复制无关的外部文本")
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
			t.Fatalf("external paste matched (round %d)", i)
		}
		if body.Error != "" {
			t.Fatalf("paste error: %s", body.Error)
		}
	}
	pendings, err := f.srv.controlPlane.ListPendingHandoffs(context.Background(), "ian", f.t2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendings) != 0 {
		t.Fatalf("external paste left %d pendings", len(pendings))
	}
}

// 目标已有任务：补充来源引用；同来源的新选区覆盖旧快照，不丢内容。
func TestHandoffSupplementUpdatesSnapshot(t *testing.T) {
	f := newHandoffFixture(t)
	// 目标 t2 绑定到已有任务 B。
	if _, err := f.srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: "task-dst-b", ProjectID: "project-handoff",
			WorkstreamID: "ws-handoff",
			Title:        "目标任务 B", State: "running",
			SessionIDs: []string{"session-live-" + f.t2.ID},
			Timeline:   []projectControlEvent{}, Audit: []projectControlAuditItem{},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if rec := f.copy(t, f.t1.ID, "第一段选区内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy 1 = %d", rec.Code)
	}
	rec := f.paste(t, f.t2.ID, "第一段选区内容")
	if rec.Code != http.StatusOK {
		t.Fatalf("paste 1 = %d: %s", rec.Code, rec.Body.String())
	}
	var first struct {
		RefID string `json:"refId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}

	// 同来源第二段内容 → 同一条关系，快照替换为本次实际引用的内容。
	if rec := f.copy(t, f.t1.ID, "第二段不同的选区内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy 2 = %d", rec.Code)
	}
	rec = f.paste(t, f.t2.ID, "第二段不同的选区内容")
	if rec.Code != http.StatusOK {
		t.Fatalf("paste 2 = %d: %s", rec.Code, rec.Body.String())
	}
	var second struct {
		RefID string `json:"refId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.RefID != first.RefID {
		t.Fatalf("supplement created a second ref %s (first %s)", second.RefID, first.RefID)
	}
	refs, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", "task-dst-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %d, want 1", len(refs))
	}
	if refs[0].Snapshot != "第二段不同的选区内容" {
		t.Fatalf("snapshot = %q, want the newest selection", refs[0].Snapshot)
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
