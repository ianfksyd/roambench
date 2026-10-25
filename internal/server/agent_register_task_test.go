package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/ianf339/roambench/internal/terminal"
)

func TestRegisterTaskValidatesTerminalBeforeCreatingCard(t *testing.T) {
	for _, kind := range []string{"unavailable", "unknown", "another-user", "conflicting-project"} {
		t.Run(kind, func(t *testing.T) {
			srv, token, sessions := progressServer(t)
			defer sessions.Stop()
			defer srv.Shutdown(context.Background())
			terminalID := "missing-terminal"
			if kind != "unavailable" {
				mgr := terminal.NewManager(&srv.cfg.Terminal)
				defer mgr.Stop()
				srv.terminals = mgr
				if kind == "another-user" || kind == "conflicting-project" {
					username := "someone-else"
					opts := terminal.SessionCreateOptions{WorkDir: t.TempDir()}
					if kind == "conflicting-project" {
						username = "ian"
						opts.TaskID = seedP3Task(t, srv, token, "source")
						opts.ProjectID = "different-project"
					}
					live, err := mgr.CreateSessionWithOptions(username, opts)
					if err != nil {
						t.Fatal(err)
					}
					defer mgr.KillSession(live.ID)
					terminalID = live.ID
				}
			}
			before, err := srv.projectControl.loadOrSeed("ian")
			if err != nil {
				t.Fatal(err)
			}
			agentToken := agentTokenFor(t, srv, token)
			body, _ := json.Marshal(registerTaskRequest{Title: "invalid registration", TerminalID: terminalID})
			rec := agentAPIPost(t, srv, "/api/agent/v1/register-task", agentToken, string(body))
			want := http.StatusNotFound
			if kind == "unavailable" {
				want = http.StatusServiceUnavailable
			}
			if kind == "conflicting-project" {
				want = http.StatusBadRequest
			}
			if rec.Code != want {
				t.Fatalf("status=%d want=%d: %s", rec.Code, want, rec.Body.String())
			}
			after, err := srv.projectControl.loadOrSeed("ian")
			if err != nil {
				t.Fatal(err)
			}
			if len(after.Tasks) != len(before.Tasks) {
				t.Fatal("failed registration left a task behind")
			}
		})
	}
}

func TestRegisterTaskKeepsTerminalProjectAndWorkstream(t *testing.T) {
	for _, metadata := range []bool{true, false} {
		t.Run(map[bool]string{true: "terminal-metadata", false: "persisted-session-binding"}[metadata], func(t *testing.T) {
			srv, token, sessions := progressServer(t)
			defer sessions.Stop()
			defer srv.Shutdown(context.Background())
			sourceID := seedP3Task(t, srv, token, "same title")
			mgr := terminal.NewManager(&srv.cfg.Terminal)
			defer mgr.Stop()
			srv.terminals = mgr
			opts := terminal.SessionCreateOptions{WorkDir: t.TempDir()}
			if metadata {
				opts.TaskID = sourceID
				opts.ProjectID = "project-roambench-control"
			}
			live, err := mgr.CreateSessionWithOptions("ian", opts)
			if err != nil {
				t.Fatal(err)
			}
			defer mgr.KillSession(live.ID)
			srv.projectControl.bindTaskTerminal("ian", sourceID, live.ID, "project-roambench-control")
			_, err = srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
				state.Projects = append(state.Projects, projectControlProject{ID: "project-other", Name: "Other"})
				state.Workstreams = append(state.Workstreams, projectControlWorkstream{ID: "workstream-other", ProjectID: "project-other", Title: "Other"})
				state.ActiveProjectID = "project-other"
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			agentToken := agentTokenFor(t, srv, token)
			body, _ := json.Marshal(registerTaskRequest{Title: "same title", TerminalID: live.ID})
			rec := agentAPIPost(t, srv, "/api/agent/v1/register-task", agentToken, string(body))
			if rec.Code != http.StatusOK {
				t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
			}
			var result struct {
				TaskID     string `json:"taskId"`
				ProjectID  string `json:"projectId"`
				TerminalID string `json:"terminalId"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			state, err := srv.projectControl.loadOrSeed("ian")
			if err != nil {
				t.Fatal(err)
			}
			task, ok := controlTask(state, result.TaskID)
			if !ok || task.ID == sourceID || task.ProjectID != "project-roambench-control" || task.WorkstreamID != "workstream-ui-ia" || result.ProjectID != task.ProjectID || result.TerminalID != live.ID {
				t.Fatalf("incorrect task context: result=%+v task=%+v", result, task)
			}
			if len(task.SessionIDs) != 1 || task.SessionIDs[0] != projectControlSessionIDForTerminal(live.ID) {
				t.Fatalf("binding was not persisted: %v", task.SessionIDs)
			}
			if task.Origin != "agent" || projectControlTaskClaimable(state, task) {
				t.Fatal("registration can start a duplicate worker execution")
			}
		})
	}
}

func TestRegisterTaskReportsPersistenceFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions failure requires an unprivileged user")
	}
	srv, token, sessions := progressServer(t)
	defer sessions.Stop()
	defer srv.Shutdown(context.Background())
	agentToken := agentTokenFor(t, srv, token)
	before, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(srv.projectControl.rootDir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(srv.projectControl.rootDir, 0700)
	rec := agentAPIPost(t, srv, "/api/agent/v1/register-task", agentToken, `{"title":"must not report success"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d: %s", rec.Code, rec.Body.String())
	}
	if err := os.Chmod(srv.projectControl.rootDir, 0700); err != nil {
		t.Fatal(err)
	}
	after, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Tasks) != len(before.Tasks) {
		t.Fatal("task survived failed persistence")
	}
}
