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
	"time"

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

// startFakeAgent runs a process whose argv[0] is a known agent CLI inside
// the pane, then waits until the server-side pane inspection sees it.
func (f *handoffFixture) startFakeAgent(t *testing.T, terminalID, name string) {
	t.Helper()
	if err := f.mgr.SendKeys("ian", terminalID, "exec -a "+name+" sleep 60", true); err != nil {
		t.Fatalf("send keys: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := f.mgr.AgentProcessInSession(terminalID); ok {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("fake agent not detected in pane")
}

func (f *handoffFixture) submit(t *testing.T, terminalID, text string) *httptest.ResponseRecorder {
	t.Helper()
	return f.post(t, "/api/terminal-handoff/submit",
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

// 同一目标的两次粘贴是独立事件：都入账、提交时都被消费；同来源的选区
// 按 (target,source,relation) 收敛为一条关系、快照为最新一段（既有验收
// 语义；完整输入版本历史是已知边界，本轮不做）。
func TestHandoffDistinctSelectionsAllPreserved(t *testing.T) {
	f := newHandoffFixture(t)
	if rec := f.copy(t, f.t1.ID, "第一段选区内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy 1 = %d", rec.Code)
	}
	if rec := f.paste(t, f.t2.ID, "第一段选区内容"); rec.Code != http.StatusOK {
		t.Fatalf("paste 1 = %d", rec.Code)
	}
	if rec := f.copy(t, f.t1.ID, "第二段不同的选区内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy 2 = %d", rec.Code)
	}
	if rec := f.paste(t, f.t2.ID, "第二段不同的选区内容"); rec.Code != http.StatusOK {
		t.Fatalf("paste 2 = %d", rec.Code)
	}

	agentToken := agentTokenFor(t, f.srv, f.token)
	regBody, _ := json.Marshal(registerTaskRequest{Title: "多选区目标", TerminalID: f.t2.ID})
	rec := agentAPIPost(t, f.srv, "/api/agent/v1/register-task", agentToken, string(regBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("register-task = %d: %s", rec.Code, rec.Body.String())
	}
	var reg struct {
		Promotion struct {
			Promoted int `json:"promoted"`
			Failed   int `json:"failed"`
		} `json:"promotion"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reg); err != nil {
		t.Fatal(err)
	}
	if reg.Promotion.Promoted != 2 || reg.Promotion.Failed != 0 {
		t.Fatalf("promotion = %+v, want both pastes consumed", reg.Promotion)
	}
	state, err := f.srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	bound := tasksForTerminalInState(state, f.t2.ID)
	if len(bound) != 1 {
		t.Fatalf("terminal bound to %d tasks", len(bound))
	}
	refs, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", bound[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Snapshot != "第二段不同的选区内容" {
		t.Fatalf("refs = %+v, want one ref with the newest selection", refs)
	}
}

// 原始需求：裸 agent（无可适配器 CLI）流程。粘贴只是草稿入账（不建卡）；
// 用户 Enter 提交后，卡与 reviews 来源自动出现，无需 register-task、无需确认。
func TestHandoffSubmitCreatesCardForBareAgent(t *testing.T) {
	f := newHandoffFixture(t)
	if rec := f.copy(t, f.t1.ID, "裸 Codex 审查请求"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d", rec.Code)
	}
	f.startFakeAgent(t, f.t2.ID, "codex")
	rec := f.paste(t, f.t2.ID, "裸 Codex 审查请求")
	if rec.Code != http.StatusOK {
		t.Fatalf("paste = %d: %s", rec.Code, rec.Body.String())
	}
	var pasteBody struct {
		Pending bool   `json:"pending"`
		TaskID  string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pasteBody); err != nil {
		t.Fatal(err)
	}
	// 进程存在不等于提交：未提交的草稿不产生任何卡。
	if !pasteBody.Pending || pasteBody.TaskID != "" {
		t.Fatalf("paste = %+v, want pending without a card", pasteBody)
	}
	if n := f.taskCountForTerminal(t, f.t2.ID); n != 0 {
		t.Fatalf("unsubmitted draft created %d tasks", n)
	}

	// 用户 Enter 提交 → 卡与来源自动出现。
	rec = f.submit(t, f.t2.ID, "裸 Codex 审查请求")
	if rec.Code != http.StatusOK {
		t.Fatalf("submit = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Submitted bool   `json:"submitted"`
		TaskID    string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Submitted || body.TaskID == "" {
		t.Fatalf("submit = %+v", body)
	}
	state, err := f.srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	task, ok := controlTask(state, body.TaskID)
	if !ok {
		t.Fatal("submitted card not persisted")
	}
	if task.ProjectID != "project-handoff" {
		t.Fatalf("card project = %q, want source project", task.ProjectID)
	}
	refs, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", body.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ref := range refs {
		if ref.SourceTaskID == f.taskA && ref.Snapshot == "裸 Codex 审查请求" {
			found = true
		}
	}
	if !found {
		t.Fatalf("submitted card refs missing source: %+v", refs)
	}
}

// 普通 shell 中按 Enter 不是提交：没有 agent 接收，不建卡。
func TestHandoffSubmitWithoutAgentCreatesNothing(t *testing.T) {
	f := newHandoffFixture(t)
	if rec := f.copy(t, f.t1.ID, "shell 里的内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d", rec.Code)
	}
	if rec := f.paste(t, f.t2.ID, "shell 里的内容"); rec.Code != http.StatusOK {
		t.Fatalf("paste = %d", rec.Code)
	}
	rec := f.submit(t, f.t2.ID, "shell 里的内容")
	if rec.Code != http.StatusOK {
		t.Fatalf("submit = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Submitted bool `json:"submitted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Submitted {
		t.Fatal("submit succeeded without an agent session")
	}
	if n := f.taskCountForTerminal(t, f.t2.ID); n != 0 {
		t.Fatalf("shell submit created %d tasks", n)
	}
}

// 并发提交同一目标：代际决策串行化，恰好一张卡。
func TestHandoffConcurrentSubmitsSingleCard(t *testing.T) {
	f := newHandoffFixture(t)
	if rec := f.copy(t, f.t1.ID, "并发提交内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d", rec.Code)
	}
	f.startFakeAgent(t, f.t2.ID, "codex")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rec := f.submit(t, f.t2.ID, "并发提交内容"); rec.Code != http.StatusOK && rec.Code != http.StatusMultiStatus {
				errs <- fmt.Errorf("submit = %d: %s", rec.Code, rec.Body.String())
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if n := f.taskCountForTerminal(t, f.t2.ID); n != 1 {
		t.Fatalf("8 concurrent submissions produced %d cards, want 1", n)
	}
}

// 目标终端已有显式项目归属：提交卡落在目标项目，不被来源项目覆盖。
func TestHandoffSubmitKeepsExplicitTargetProject(t *testing.T) {
	f := newHandoffFixture(t)
	if _, err := f.srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Projects = append(state.Projects, projectControlProject{ID: "project-target", Name: "Target", Takeover: projectTakeover{Mode: "manual"}})
		state.Workstreams = append(state.Workstreams, projectControlWorkstream{ID: "ws-target", ProjectID: "project-target", Title: "Target"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	target, err := f.mgr.CreateSessionWithOptions("ian", terminal.SessionCreateOptions{WorkDir: t.TempDir(), Name: "explicit-target", ProjectID: "project-target"})
	if err != nil {
		t.Fatal(err)
	}
	f.startFakeAgent(t, target.ID, "codex")
	if rec := f.copy(t, f.t1.ID, "跨项目审查内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d", rec.Code)
	}
	if rec := f.paste(t, target.ID, "跨项目审查内容"); rec.Code != http.StatusOK {
		t.Fatalf("paste = %d", rec.Code)
	}
	rec := f.submit(t, target.ID, "跨项目审查内容")
	if rec.Code != http.StatusOK {
		t.Fatalf("submit = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	state, err := f.srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	task, ok := controlTask(state, body.TaskID)
	if !ok {
		t.Fatalf("card missing: %s", rec.Body.String())
	}
	if task.ProjectID != "project-target" {
		t.Fatalf("terminal belongs to project-target but card created in %s", task.ProjectID)
	}
}

// 自动关联写入失败：返回 207 且留有钉住原任务的 pending；
// 解除失败后下一次 register-task 恢复到原提交的卡。
func TestHandoffSubmitLinkFailureReportedAndRecoverable(t *testing.T) {
	f := newHandoffFixture(t)
	if rec := f.copy(t, f.t1.ID, "失败注入的自动来源"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d", rec.Code)
	}
	f.startFakeAgent(t, f.t2.ID, "codex")
	if rec := f.paste(t, f.t2.ID, "失败注入的自动来源"); rec.Code != http.StatusOK {
		t.Fatalf("paste = %d", rec.Code)
	}
	if err := f.srv.controlPlane.ExecAdmin(context.Background(),
		`CREATE TRIGGER review_fail_ref BEFORE INSERT ON task_source_refs BEGIN SELECT RAISE(ABORT, 'review injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	rec := f.submit(t, f.t2.ID, "失败注入的自动来源")
	if rec.Code == http.StatusOK {
		t.Fatalf("failed provenance write reported as clean 200: %s", rec.Body.String())
	}
	var body struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	pendings, err := f.srv.controlPlane.ListPendingHandoffs(context.Background(), "ian", f.t2.ID)
	if err != nil {
		t.Fatal(err)
	}
	pinned := false
	for _, pending := range pendings {
		if pending.IntendedTaskID == body.TaskID {
			pinned = true
		}
	}
	if !pinned {
		t.Fatalf("no pending pinned to %s: %+v", body.TaskID, pendings)
	}
	if err := f.srv.controlPlane.ExecAdmin(context.Background(), `DROP TRIGGER review_fail_ref`); err != nil {
		t.Fatal(err)
	}
	agentToken := agentTokenFor(t, f.srv, f.token)
	regBody, _ := json.Marshal(registerTaskRequest{Title: "下一个提交", TerminalID: f.t2.ID})
	rec = agentAPIPost(t, f.srv, "/api/agent/v1/register-task", agentToken, string(regBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("register after recovery = %d: %s", rec.Code, rec.Body.String())
	}
	refs, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", body.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Snapshot != "失败注入的自动来源" {
		t.Fatalf("original task refs after recovery = %+v, want 1", refs)
	}
}

// 同一终端的第二个目标：新粘贴不得挂到旧任务；新提交（或新 agent 代际）
// 的来源归新任务。
func TestHandoffSecondGoalGetsOwnSource(t *testing.T) {
	f := newHandoffFixture(t)
	// B1：第一轮粘贴 + 提交。
	if rec := f.copy(t, f.t1.ID, "第一目标内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy 1 = %d", rec.Code)
	}
	if rec := f.paste(t, f.t2.ID, "第一目标内容"); rec.Code != http.StatusOK {
		t.Fatalf("paste 1 = %d", rec.Code)
	}
	agentToken := agentTokenFor(t, f.srv, f.token)
	regBody, _ := json.Marshal(registerTaskRequest{Title: "目标 B1", TerminalID: f.t2.ID})
	if rec := agentAPIPost(t, f.srv, "/api/agent/v1/register-task", agentToken, string(regBody)); rec.Code != http.StatusOK {
		t.Fatalf("register B1 = %d", rec.Code)
	}
	state, err := f.srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	b1 := tasksForTerminalInState(state, f.t2.ID)
	if len(b1) != 1 {
		t.Fatalf("after B1: %d tasks on terminal", len(b1))
	}
	b1ID := b1[0].ID

	// 第二轮：新内容粘贴（终端仍是普通 shell，无 agent）→ 必须 pending，
	// 不得立刻补到 B1。
	if rec := f.copy(t, f.t1.ID, "第二目标内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy 2 = %d", rec.Code)
	}
	if rec := f.paste(t, f.t2.ID, "第二目标内容"); rec.Code != http.StatusOK {
		t.Fatalf("paste 2 = %d", rec.Code)
	}
	refs1, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", b1ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs1 {
		if ref.Snapshot == "第二目标内容" {
			t.Fatal("second goal's paste was attributed to the first task")
		}
	}

	// 提交 B2 → 来源归 B2。
	regBody, _ = json.Marshal(registerTaskRequest{Title: "目标 B2", TerminalID: f.t2.ID})
	if rec := agentAPIPost(t, f.srv, "/api/agent/v1/register-task", agentToken, string(regBody)); rec.Code != http.StatusOK {
		t.Fatalf("register B2 = %d", rec.Code)
	}
	state, err = f.srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	bound := tasksForTerminalInState(state, f.t2.ID)
	if len(bound) != 2 {
		t.Fatalf("terminal bound to %d tasks, want 2", len(bound))
	}
	var b1Refs, b2Refs []controlplane.SourceRef
	for _, task := range bound {
		refs, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if task.ID == b1ID {
			b1Refs = refs
		} else {
			b2Refs = refs
		}
	}
	if len(b1Refs) != 1 || b1Refs[0].Snapshot != "第一目标内容" {
		t.Fatalf("B1 refs changed: %+v", b1Refs)
	}
	if len(b2Refs) != 1 || b2Refs[0].Snapshot != "第二目标内容" {
		t.Fatalf("B2 refs = %+v, want the second selection", b2Refs)
	}
}

// 相同文本、不同来源：按粘贴事件身份（copyId）保留两条 pending，提交后两个来源都在。
func TestHandoffSameTextDifferentSourcesKept(t *testing.T) {
	f := newHandoffFixture(t)
	t3, err := f.mgr.CreateSessionWithOptions("ian", terminal.SessionCreateOptions{WorkDir: t.TempDir(), Name: "handoff-src-c"})
	if err != nil {
		t.Fatal(err)
	}
	if rec := f.copy(t, f.t1.ID, "完全相同的文本"); rec.Code != http.StatusOK {
		t.Fatalf("copy A = %d", rec.Code)
	}
	if rec := f.paste(t, f.t2.ID, "完全相同的文本"); rec.Code != http.StatusOK {
		t.Fatalf("paste A = %d", rec.Code)
	}
	if rec := f.copy(t, t3.ID, "完全相同的文本"); rec.Code != http.StatusOK {
		t.Fatalf("copy C = %d", rec.Code)
	}
	rec := f.paste(t, f.t2.ID, "完全相同的文本")
	if rec.Code != http.StatusOK {
		t.Fatalf("paste C = %d", rec.Code)
	}

	agentToken := agentTokenFor(t, f.srv, f.token)
	regBody, _ := json.Marshal(registerTaskRequest{Title: "同文目标", TerminalID: f.t2.ID})
	if rec := agentAPIPost(t, f.srv, "/api/agent/v1/register-task", agentToken, string(regBody)); rec.Code != http.StatusOK {
		t.Fatalf("register = %d", rec.Code)
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
	sources := map[string]bool{}
	for _, ref := range refs {
		sources[ref.SourceTaskID] = true
	}
	// 来源 A 是已登记任务；来源 C 的终端未登记，粘贴时被持久补登记。
	if len(sources) != 2 {
		t.Fatalf("sources = %v, want both real copy events", sources)
	}
}

// agent 代际身份：提交确立代际的当前目标，同一进程的后续粘贴补充该任务
// （新选区覆盖同对关系的快照）；同代际显式登记新目标后，粘贴改随新目标。
func TestHandoffAgentGenerationSupplements(t *testing.T) {
	f := newHandoffFixture(t)
	f.startFakeAgent(t, f.t2.ID, "claude")
	if rec := f.copy(t, f.t1.ID, "代际第一段"); rec.Code != http.StatusOK {
		t.Fatalf("copy 1 = %d", rec.Code)
	}
	rec := f.submit(t, f.t2.ID, "代际第一段")
	if rec.Code != http.StatusOK {
		t.Fatalf("submit 1 = %d: %s", rec.Code, rec.Body.String())
	}
	var first struct {
		TaskID string `json:"taskId"`
		RefID  string `json:"refId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}

	// 同一代际的第二段：仍补充同一任务。
	if rec := f.copy(t, f.t1.ID, "代际第二段"); rec.Code != http.StatusOK {
		t.Fatalf("copy 2 = %d", rec.Code)
	}
	rec = f.paste(t, f.t2.ID, "代际第二段")
	if rec.Code != http.StatusOK {
		t.Fatalf("paste 2 = %d: %s", rec.Code, rec.Body.String())
	}
	var second struct {
		TaskID   string `json:"taskId"`
		RefID    string `json:"refId"`
		Relation string `json:"relation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.TaskID != first.TaskID {
		t.Fatalf("second paste created another card %s (first %s)", second.TaskID, first.TaskID)
	}
	if second.RefID != first.RefID || second.Relation != "supplemented" {
		t.Fatalf("second paste = %+v, want same-ref supplement", second)
	}
	refs, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", first.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Snapshot != "代际第二段" {
		t.Fatalf("refs = %+v, want one ref with the newest snapshot", refs)
	}

	// 同一代际里显式登记新目标 B2：代际绑定更新为最新提交，之后的粘贴归 B2。
	agentToken := agentTokenFor(t, f.srv, f.token)
	regBody, _ := json.Marshal(registerTaskRequest{Title: "同代际第二目标", TerminalID: f.t2.ID})
	rec = agentAPIPost(t, f.srv, "/api/agent/v1/register-task", agentToken, string(regBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("register B2 = %d: %s", rec.Code, rec.Body.String())
	}
	var reg struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reg); err != nil {
		t.Fatal(err)
	}
	if rec := f.copy(t, f.t1.ID, "归 B2 的来源"); rec.Code != http.StatusOK {
		t.Fatalf("copy 3 = %d", rec.Code)
	}
	rec = f.paste(t, f.t2.ID, "归 B2 的来源")
	if rec.Code != http.StatusOK {
		t.Fatalf("paste 3 = %d: %s", rec.Code, rec.Body.String())
	}
	var third struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &third); err != nil {
		t.Fatal(err)
	}
	if third.TaskID != reg.TaskID {
		t.Fatalf("paste after B2 went to %s, want the latest goal %s", third.TaskID, reg.TaskID)
	}
	refs1, _ := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", first.TaskID)
	for _, ref := range refs1 {
		if ref.Snapshot == "归 B2 的来源" {
			t.Fatal("B2's source leaked onto the first goal")
		}
	}
	refs2, _ := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", reg.TaskID)
	if len(refs2) != 1 {
		t.Fatalf("B2 refs = %+v, want 1", refs2)
	}
}

// 无归属新终端的提交：审查卡继承来源项目，不落当前浏览项目。
func TestRegisterTaskInheritsSourceProject(t *testing.T) {
	f := newHandoffFixture(t)
	// 活动项目是种子默认（project-roambench-control）；来源任务属于
	// project-handoff。
	if rec := f.copy(t, f.t1.ID, "跨项目审查内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d", rec.Code)
	}
	if rec := f.paste(t, f.t2.ID, "跨项目审查内容"); rec.Code != http.StatusOK {
		t.Fatalf("paste = %d", rec.Code)
	}
	agentToken := agentTokenFor(t, f.srv, f.token)
	regBody, _ := json.Marshal(registerTaskRequest{Title: "跨项目审查", TerminalID: f.t2.ID})
	rec := agentAPIPost(t, f.srv, "/api/agent/v1/register-task", agentToken, string(regBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("register = %d: %s", rec.Code, rec.Body.String())
	}
	var reg struct {
		TaskID    string `json:"taskId"`
		ProjectID string `json:"projectId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reg); err != nil {
		t.Fatal(err)
	}
	if reg.ProjectID != "project-handoff" {
		t.Fatalf("task landed in %q, want the source project", reg.ProjectID)
	}
}

// 提升失败必须显式回报（207），且重试恢复到原提交的任务，不泄漏给下一个目标。
func TestRegisterTaskPromotionFailureReported(t *testing.T) {
	f := newHandoffFixture(t)
	if rec := f.copy(t, f.t1.ID, "失败注入内容"); rec.Code != http.StatusOK {
		t.Fatalf("copy = %d", rec.Code)
	}
	if rec := f.paste(t, f.t2.ID, "失败注入内容"); rec.Code != http.StatusOK {
		t.Fatalf("paste = %d", rec.Code)
	}
	// 隔离库内注入来源关系写入失败。
	inject := `CREATE TRIGGER fail_reviews_insert BEFORE INSERT ON task_source_refs
	           WHEN NEW.relation='reviews' AND NEW.source_task_id='task-src-a'
	           BEGIN SELECT RAISE(ABORT, 'injected failure'); END`
	if err := f.srv.controlPlane.ExecAdmin(context.Background(), inject); err != nil {
		t.Fatal(err)
	}
	agentToken := agentTokenFor(t, f.srv, f.token)
	regBody, _ := json.Marshal(registerTaskRequest{Title: "失败目标", TerminalID: f.t2.ID})
	rec := agentAPIPost(t, f.srv, "/api/agent/v1/register-task", agentToken, string(regBody))
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("register with failed promotion = %d: %s", rec.Code, rec.Body.String())
	}
	var first struct {
		TaskID    string `json:"taskId"`
		Promotion struct {
			Failed int `json:"failed"`
		} `json:"promotion"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Promotion.Failed == 0 {
		t.Fatal("promotion failure not reported")
	}

	// 解除注入，同一终端再次提交：pending 恢复到原任务 B1，
	// 新任务 B2 不继承这次来源。
	if err := f.srv.controlPlane.ExecAdmin(context.Background(), `DROP TRIGGER fail_reviews_insert`); err != nil {
		t.Fatal(err)
	}
	regBody, _ = json.Marshal(registerTaskRequest{Title: "下一个目标", TerminalID: f.t2.ID})
	rec = agentAPIPost(t, f.srv, "/api/agent/v1/register-task", agentToken, string(regBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("recovery register = %d: %s", rec.Code, rec.Body.String())
	}
	state, err := f.srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	bound := tasksForTerminalInState(state, f.t2.ID)
	if len(bound) != 2 {
		t.Fatalf("terminal bound to %d tasks", len(bound))
	}
	b1Refs, _ := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", first.TaskID)
	if len(b1Refs) != 1 || b1Refs[0].Snapshot != "失败注入内容" {
		t.Fatalf("recovery did not reattach to the original task: %+v", b1Refs)
	}
	var b2ID string
	for _, task := range bound {
		if task.ID != first.TaskID {
			b2ID = task.ID
		}
	}
	b2Refs, _ := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", b2ID)
	if len(b2Refs) != 0 {
		t.Fatalf("new goal inherited recovered provenance: %+v", b2Refs)
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
