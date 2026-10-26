package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/terminal"
)

func handoffShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

// Observe only process identities belonging to this isolated test pane.
// Unlike the production traversal, intermediate parents are always visited.
func handoffAgentDescendants(t *testing.T, terminalID string) []string {
	t.Helper()
	rootBytes, err := exec.Command("tmux", "display-message", "-p", "-t", terminalID, "#{pane_pid}").Output()
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSpace(string(rootBytes))
	out, err := exec.Command("ps", "-eo", "pid=,ppid=,stat=,args=").Output()
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string][]string{}
	children := map[string][]string{}
	for _, line := range strings.Split(string(out), "\n") {
		p := strings.Fields(line)
		if len(p) < 4 {
			continue
		}
		rows[p[0]] = p
		children[p[1]] = append(children[p[1]], p[0])
	}
	var result []string
	queue := []string{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		p := rows[pid]
		if len(p) < 4 {
			continue
		}
		if pid == root || filepath.Base(p[3]) == "codex" {
			result = append(result, fmt.Sprintf("pid=%s ppid=%s stat=%s name=%s", p[0], p[1], p[2], filepath.Base(p[3])))
		}
		queue = append(queue, children[pid]...)
	}
	return result
}

func TestHandoffNormalForegroundCodexIsDetected(t *testing.T) {
	f := newHandoffFixture(t)
	codexPath, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("Codex CLI is not installed")
	}
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "local review stub", 503) }))
	t.Cleanup(stub.Close)
	codexHome := t.TempDir()
	workspace := t.TempDir()
	config := fmt.Sprintf("model = \"gpt-5\"\nmodel_provider = \"review\"\n[model_providers.review]\nname = \"Local review only\"\nbase_url = %q\nwire_api = \"responses\"\nenv_key = \"ROAMBENCH_REVIEW_DUMMY_KEY\"\n[projects.%q]\ntrust_level = \"trusted\"\n[analytics]\nenabled = false\n", stub.URL, workspace)
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	// The user's normal shell command: do not replace the pane's shell with exec.
	command := "CODEX_HOME=" + handoffShellQuote(codexHome) + " ROAMBENCH_REVIEW_DUMMY_KEY=local-only-dummy-key " + handoffShellQuote(codexPath) + " --no-daemon --no-alt-screen -C " + handoffShellQuote(workspace)
	if err := f.mgr.SendKeys("ian", f.t2.ID, command, true); err != nil {
		t.Fatal(err)
	}
	var identities []string
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		identities = handoffAgentDescendants(t, f.t2.ID)
		if len(identities) >= 2 && strings.Contains(identities[len(identities)-1], "+") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(identities) < 2 || !strings.Contains(identities[len(identities)-1], "+") {
		t.Fatalf("no real foreground Codex child started: %v", identities)
	}
	agent, found := f.mgr.AgentProcessInSession(f.t2.ID)
	if !found {
		handoffRegressionCopy(t, f, f.t1.ID, "review source")
		rec := f.submit(t, f.t2.ID, "review source")
		t.Fatalf("real Codex is foreground, manager found=%v agent=%+v; identities=%v; submit=%d %s", found, agent, identities, rec.Code, rec.Body.String())
	}
	handoffRegressionCopy(t, f, f.t1.ID, "review source")
	if rec := handoffDraftPost(t, f, "paste", f.t2.ID, "review source", "real-codex"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec := handoffDraftPost(t, f, "submit", f.t2.ID, "review source", "real-codex")
	var body struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || body.TaskID == "" {
		t.Fatalf("real Codex submit=%d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandoffForegroundChildUnderShellIsDetected(t *testing.T) {
	f := newHandoffFixture(t)
	if err := f.mgr.SendKeys("ian", f.t2.ID, "bash -c 'exec -a codex sleep 60'", true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if agent, found := f.mgr.AgentProcessInSession(f.t2.ID); found && agent.Name == "codex" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("foreground child was missed: %v", handoffAgentDescendants(t, f.t2.ID))
}

func TestHandoffDraftSubmissionDoesNotConsumeOtherDrafts(t *testing.T) {
	f := newHandoffFixture(t)
	f.startFakeAgent(t, f.t2.ID, "codex")
	handoffRegressionCopy(t, f, f.t1.ID, "first draft source")
	if rec := handoffDraftPost(t, f, "paste", f.t2.ID, "first draft source", "other-window"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	handoffRegressionCopy(t, f, f.t1.ID, "current draft source")
	if rec := handoffDraftPost(t, f, "paste", f.t2.ID, "current draft source", "current-window"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec := handoffDraftPost(t, f, "submit", f.t2.ID, "current draft source", "current-window")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var body struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	refs, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", body.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Snapshot != "current draft source" {
		t.Fatalf("wrong draft consumed: %+v", refs)
	}
	pendings, err := f.srv.controlPlane.ListPendingHandoffs(context.Background(), "ian", f.t2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendings) != 1 || pendings[0].DraftID != "other-window" || pendings[0].IntendedTaskID != "" {
		t.Fatalf("other draft lost: %+v", pendings)
	}
}

func TestHandoffDraftCancellationPreservesFailedSubmission(t *testing.T) {
	f := newHandoffFixture(t)
	f.startFakeAgent(t, f.t2.ID, "codex")
	handoffRegressionCopy(t, f, f.t1.ID, "recover this source")
	if rec := handoffDraftPost(t, f, "paste", f.t2.ID, "recover this source", "failed-draft"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if err := f.srv.controlPlane.ExecAdmin(context.Background(), `CREATE TRIGGER fail_draft_ref BEFORE INSERT ON task_source_refs BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	rec := handoffDraftPost(t, f, "submit", f.t2.ID, "recover this source", "failed-draft")
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("submit=%d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec := handoffDraftPost(t, f, "cancel", f.t2.ID, "", "failed-draft"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if err := f.srv.controlPlane.ExecAdmin(context.Background(), `DROP TRIGGER fail_draft_ref`); err != nil {
		t.Fatal(err)
	}
	// A different draft in the same active goal still recovers the pinned input.
	rec = handoffDraftPost(t, f, "submit", f.t2.ID, "unrelated external text", "new-draft")
	if rec.Code != http.StatusOK {
		t.Fatalf("recovery=%d: %s", rec.Code, rec.Body.String())
	}
	refs, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", body.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Snapshot != "recover this source" {
		t.Fatalf("accepted provenance was lost: %+v", refs)
	}
}

func TestHandoffCancelledDraftDoesNotLeakIntoNextGoal(t *testing.T) {
	f := newHandoffFixture(t)
	f.startFakeAgent(t, f.t2.ID, "codex")
	handoffRegressionCopy(t, f, f.t1.ID, "discarded source A")
	if rec := handoffDraftPost(t, f, "paste", f.t2.ID, "discarded source A", "abandoned"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	// Ctrl-C cancels that draft at both ends before another draft starts.
	if rec := handoffDraftPost(t, f, "cancel", f.t2.ID, "", "abandoned"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	c, err := f.mgr.CreateSessionWithOptions("ian", terminal.SessionCreateOptions{WorkDir: t.TempDir(), Name: "fresh-source-c"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: "task-source-c", ProjectID: "project-handoff", WorkstreamID: "ws-handoff", Title: "Source C",
			State: "running", SessionIDs: []string{"session-live-" + c.ID}, Timeline: []projectControlEvent{}, Audit: []projectControlAuditItem{},
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	handoffRegressionCopy(t, f, c.ID, "fresh source C")
	if rec := handoffDraftPost(t, f, "paste", f.t2.ID, "fresh source C", "current"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec := handoffDraftPost(t, f, "submit", f.t2.ID, "fresh source C", "current")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var body struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	id := body.TaskID
	refs, err := f.srv.controlPlane.ListSourceRefs(context.Background(), "ian", id)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].SourceTaskID != "task-source-c" {
		t.Fatalf("discarded input leaked into new goal: %+v", refs)
	}
}

func TestHandoffWorkerBindingFollowsExplicitNewGoal(t *testing.T) {
	f := newHandoffFixture(t)
	const b1 = "task-worker-original"
	worker, err := f.mgr.CreateSessionWithOptions("ian", terminal.SessionCreateOptions{
		WorkDir: t.TempDir(), Name: "worker-reused", ProjectID: "project-handoff", TaskID: b1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Tasks = append(state.Tasks, projectControlTask{ID: b1, ProjectID: "project-handoff", WorkstreamID: "ws-handoff", Title: "Original worker goal", State: "running", DispatchState: "claimed", SessionIDs: []string{"session-live-" + worker.ID}, Timeline: []projectControlEvent{}, Audit: []projectControlAuditItem{}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.startFakeAgent(t, worker.ID, "codex")
	b2 := handoffRegressionRegister(t, f, "Explicit second goal", worker.ID)
	handoffRegressionCopy(t, f, f.t1.ID, "source for B2")
	paste := f.paste(t, worker.ID, "source for B2")
	var pasteResult struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(paste.Body.Bytes(), &pasteResult); err != nil {
		t.Fatal(err)
	}
	submitted := handoffRegressionSubmittedID(t, f, worker.ID, "source for B2")
	live, status, _ := f.srv.handoffTerminal("ian", worker.ID)
	if status != http.StatusOK || live.TaskID != b2 {
		t.Fatalf("session metadata did not follow B2: %+v", live)
	}
	if pasteResult.TaskID != b2 || submitted != b2 {
		t.Fatalf("current accepted B2=%s; paste=%s; submit=%s (stale metadata B1=%s)", b2, pasteResult.TaskID, submitted, b1)
	}
}

func handoffRegressionCopy(t *testing.T, f *handoffFixture, terminalID, text string) {
	t.Helper()
	if rec := f.copy(t, terminalID, text); rec.Code != http.StatusOK {
		t.Fatalf("copy=%d: %s", rec.Code, rec.Body.String())
	}
}

func handoffRegressionRegister(t *testing.T, f *handoffFixture, title, terminalID string) string {
	t.Helper()
	token := agentTokenFor(t, f.srv, f.token)
	body, _ := json.Marshal(registerTaskRequest{Title: title, TerminalID: terminalID})
	rec := agentAPIPost(t, f.srv, "/api/agent/v1/register-task", token, string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("register=%d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.TaskID
}

func handoffRegressionSubmittedID(t *testing.T, f *handoffFixture, terminalID, text string) string {
	t.Helper()
	rec := f.submit(t, terminalID, text)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit=%d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.TaskID
}

func handoffDraftPost(t *testing.T, f *handoffFixture, action, terminalID, text, draftID string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"terminalId": terminalID, "text": text, "draftId": draftID})
	return f.post(t, "/api/terminal-handoff/"+action, string(body))
}
