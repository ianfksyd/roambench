package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/controlplane"
)

func mutateI2(t *testing.T, srv *Server, fn func(*projectControlState)) {
	t.Helper()
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error { fn(state); return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestR1CriticalTaskGates(t *testing.T) {
	for _, level := range []string{"high", "critical"} {
		t.Run(level, func(t *testing.T) {
			srv, _, agent := setupTakeover(t)
			mutateI2(t, srv, func(state *projectControlState) {
				for i := range state.Tasks {
					if state.Tasks[i].ID == "task-i2" {
						state.Tasks[i].RiskLevel = level
					}
				}
			})
			state := i2State(t, srv)
			task, _ := controlTask(state, "task-i2")
			if taskClaimAllowed(state, task) {
				t.Fatal("high-risk task passed claim predicate")
			}
			rec := agentAPIPost(t, srv, "/api/agent/v1/claim", agent, `{"runtimeId":"agent","agentLabel":"Hermes"}`)
			if rec.Code != 204 {
				t.Fatal(rec.Code, rec.Body.String())
			}
			p, _ := controlProject(i2State(t, srv), "project-i2")
			if p.Takeover.Mode != "paused" || p.Takeover.Reason != "high_risk" {
				t.Fatal(p)
			}
		})
	}
	t.Run("update-running-task", func(t *testing.T) {
		srv, cookie, agent := setupTakeover(t)
		lease := i2Claim(t, srv, agent)
		task, _ := controlTask(i2State(t, srv), "task-i2")
		patchProjectControlTask(t, srv, cookie, task.ID, fmt.Sprintf(`{"expectedRowVersion":%d,"riskLevel":"critical"}`, task.RowVersion))
		p, _ := controlProject(i2State(t, srv), "project-i2")
		claim, err := srv.projectControl.claimStoreFor().GetClaim(context.Background(), "ian", lease)
		if err != nil || p.Takeover.Mode != "paused" || claim.Status != "released" {
			t.Fatalf("%+v %+v %v", p, claim, err)
		}
	})
}

func TestR1ManualCheckpointDoesNotPauseHumanWork(t *testing.T) {
	for _, reason := range []string{"agent_request", "path_violation"} {
		t.Run(reason, func(t *testing.T) {
			srv, cookie, agent := setupTakeover(t)
			mutateI2(t, srv, func(state *projectControlState) {
				for i := range state.Projects {
					if state.Projects[i].ID == "project-i2" {
						state.Projects[i].Takeover.Mode = "manual"
						state.Projects[i].Lifecycle.State = "planning"
					}
				}
				state.PhaseAttempts = append(state.PhaseAttempts, projectControlPhaseAttempt{ID: "human-phase", TaskID: "task-i2", Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano)})
				state.ToolRuns = append(state.ToolRuns, projectControlToolRun{ID: "human-tool", TaskID: "task-i2", Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano)})
			})
			rec := agentAPIPost(t, srv, "/api/agent/v1/checkpoint", agent, fmt.Sprintf(`{"taskId":"task-i2","reason":"Need human input","reasonCode":%q}`, reason))
			if rec.Code != 200 {
				t.Fatal(rec.Code, rec.Body.String())
			}
			state := i2State(t, srv)
			task, _ := controlTask(state, "task-i2")
			p, _ := controlProject(state, "project-i2")
			if p.Takeover.Mode != "manual" || p.Lifecycle.State != "planning" || task.PauseReason != "" {
				t.Fatalf("%+v %+v", p, task)
			}
			for _, a := range state.PhaseAttempts {
				if a.ID == "human-phase" && a.Status != "running" {
					t.Fatal(a)
				}
			}
			for _, run := range state.ToolRuns {
				if run.ID == "human-tool" && run.Status != "running" {
					t.Fatal(run)
				}
			}
			interactions, err := srv.controlPlane.ListInteractions(context.Background(), "ian", 500)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range interactions {
				if item.TaskID == task.ID && item.RequestKind == "agent_request" && item.Status == "pending" {
					found = true
				}
			}
			if !found {
				t.Fatal("checkpoint not persisted")
			}
			rec = i2Human(t, srv, cookie, "POST", "/api/project-control/projects/project-i2/takeover", `{"action":"resume","restart":true}`)
			if rec.Code != 400 {
				t.Fatal("manual resume bypassed start", rec.Code)
			}
		})
	}
}

func TestR1HighRiskInteractionsRequireIndividualApproval(t *testing.T) {
	for _, mode := range []string{"active", "manual"} {
		for _, risk := range []string{"R2", "R3", " HIGH_RISK ", "critical"} {
			t.Run(mode+"/"+risk, func(t *testing.T) {
				srv, cookie, agent := setupTakeover(t)
				mutateI2(t, srv, func(state *projectControlState) {
					for i := range state.Projects {
						if state.Projects[i].ID == "project-i2" {
							state.Projects[i].Takeover.Mode = mode
						}
					}
				})
				body := fmt.Sprintf(`{"taskId":"task-i2","sessionId":"s","adapterKind":"generic","vendorRequestId":"v","requestKind":"permission","riskClass":%q,"title":"Approve operation","allowedActions":["approve_once","approve_session","reject"],"responseSchema":{"type":"action"},"inputHash":"sha256:abc"}`, risk)
				req := httptest.NewRequest("POST", "/api/agent/v1/interactions", strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+agent)
				req.Header.Set("Idempotency-Key", "risk-request")
				rec := httptest.NewRecorder()
				srv.mux.ServeHTTP(rec, req)
				if rec.Code != 201 {
					t.Fatal(rec.Code, rec.Body.String())
				}
				var item controlplane.Interaction
				if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
					t.Fatal(err)
				}
				if item.RiskClass != controlplane.NormalizeRiskClass(risk) || !reflect.DeepEqual(item.AllowedActions, []string{"approve_once", "reject"}) {
					t.Fatal(item)
				}
				state := i2State(t, srv)
				p, _ := controlProject(state, "project-i2")
				task, _ := controlTask(state, "task-i2")
				if mode == "active" {
					if p.Takeover.Mode != "paused" || task.PauseReason != "high_risk" {
						t.Fatalf("%+v %+v", p, task)
					}
				} else if p.Takeover.Mode != "manual" || task.PauseReason != "" {
					t.Fatalf("%+v %+v", p, task)
				}
				rec = i2Human(t, srv, cookie, "POST", "/api/mobile/interactions/"+item.RequestID+"/respond", `{"action":"approve_session","expectedRowVersion":1,"idempotencyKey":"session","inputHash":"sha256:abc"}`)
				if rec.Code != 400 {
					t.Fatal("session approval accepted", rec.Code, rec.Body.String())
				}
				rec = i2Human(t, srv, cookie, "POST", "/api/mobile/interactions/"+item.RequestID+"/respond", `{"action":"approve_once","expectedRowVersion":1,"idempotencyKey":"once","inputHash":"sha256:abc"}`)
				if rec.Code != 200 {
					t.Fatal(rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestR1ReleaseOutcomeCaseAndCleanEnvironment(t *testing.T) {
	for _, outcome := range []string{" Done ", "FAILED", " Released "} {
		t.Run(outcome, func(t *testing.T) {
			srv, _, agent := setupTakeover(t)
			t.Setenv("ROAMBENCH_PASSWORD_HASH", "must-not-inherit")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "must-not-inherit")
			t.Setenv("BASH_ENV", "/should/not/be/sourced")
			mutateI2(t, srv, func(state *projectControlState) {
				for i := range state.Tasks {
					if state.Tasks[i].ID == "task-i2" {
						state.Tasks[i].AcceptanceCommand = []string{"/bin/sh", "-c", `test -z "${ROAMBENCH_PASSWORD_HASH+x}" && test -z "${AWS_SECRET_ACCESS_KEY+x}" && test -z "${BASH_ENV+x}" && test -n "$HOME" && printf ran > acceptance-ran`}
					}
				}
			})
			lease := i2Claim(t, srv, agent)
			rec := agentAPIPost(t, srv, "/api/agent/v1/release", agent, fmt.Sprintf(`{"leaseId":%q,"outcome":%q}`, lease, outcome))
			if rec.Code != 200 {
				t.Fatal(rec.Code, rec.Body.String())
			}
			task, _ := controlTask(i2State(t, srv), "task-i2")
			_, err := os.Stat(filepath.Join(srv.projectControl.runtimeRootDir, "acceptance-ran"))
			done := strings.TrimSpace(strings.ToLower(outcome)) == "done"
			if done {
				if err != nil || !task.AcceptancePassed || task.DispatchState != "done" {
					t.Fatalf("%+v %v", task, err)
				}
			} else if !os.IsNotExist(err) || task.AcceptancePassed || task.PauseReason != "task_failed" {
				t.Fatalf("%+v %v", task, err)
			}
		})
	}
}

func TestR1PauseCancelsAcceptanceCommand(t *testing.T) {
	srv, cookie, agent := setupTakeover(t)
	mutateI2(t, srv, func(state *projectControlState) {
		for i := range state.Tasks {
			if state.Tasks[i].ID == "task-i2" {
				state.Tasks[i].AcceptanceCommand = []string{"/bin/sh", "-c", "printf started > acceptance-started; sleep 30; printf escaped > acceptance-escaped"}
			}
		}
	})
	lease := i2Claim(t, srv, agent)
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		finished <- agentAPIPost(t, srv, "/api/agent/v1/release", agent, fmt.Sprintf(`{"leaseId":%q,"outcome":"done"}`, lease))
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(srv.projectControl.runtimeRootDir, "acceptance-started")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("acceptance command did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	rec := i2Human(t, srv, cookie, "POST", "/api/project-control/projects/project-i2/takeover", `{"action":"pause","reason":"human_pause"}`)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	select {
	case rec := <-finished:
		if rec.Code != 409 {
			t.Fatal(rec.Code, rec.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pause did not cancel acceptance process group")
	}
	task, _ := controlTask(i2State(t, srv), "task-i2")
	if task.AcceptancePassed || task.PauseReason != "human_pause" || task.DispatchState == "done" {
		t.Fatal(task)
	}
	if _, err := os.Stat(filepath.Join(srv.projectControl.runtimeRootDir, "acceptance-escaped")); !os.IsNotExist(err) {
		t.Fatal("cancelled acceptance continued", err)
	}
	srv.projectControl.mu.Lock()
	count := len(srv.projectControl.acceptanceRuns)
	srv.projectControl.mu.Unlock()
	if count != 0 {
		t.Fatal("acceptance cancellation registry leaked")
	}
}

func TestR1AcceptanceInputsRequireHumanReview(t *testing.T) {
	for _, change := range []string{"modify", "add", "delete"} {
		t.Run(change, func(t *testing.T) {
			srv, cookie, agent := setupTakeover(t)
			root := srv.projectControl.runtimeRootDir
			script := filepath.Join(root, "run_tests.sh")
			if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ran > acceptance-ran\n"), 0700); err != nil {
				t.Fatal(err)
			}
			mutateI2(t, srv, func(state *projectControlState) {
				for i := range state.Tasks {
					if state.Tasks[i].ID == "task-i2" {
						state.Tasks[i].AcceptanceCommand = []string{"./run_tests.sh"}
					}
				}
				state.Checkpoints = append(state.Checkpoints, projectControlCheckpoint{ID: "dispatch-review", TaskID: "task-i2", Kind: "dispatch", Status: "pending", AllowedActions: []string{"approve", "reject"}, RowVersion: 1})
			})
			rec := i2Human(t, srv, cookie, "POST", "/api/project-control/checkpoints/dispatch-review/decision", `{"action":"approve"}`)
			if rec.Code != 200 {
				t.Fatal(rec.Code, rec.Body.String())
			}
			task, _ := controlTask(i2State(t, srv), "task-i2")
			if task.AcceptanceBaseline == "" {
				t.Fatal("human approval did not capture baseline")
			}
			// Change before claim: the approval baseline must survive granting a lease.
			var err error
			switch change {
			case "modify":
				err = os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0700)
			case "add":
				err = os.WriteFile(filepath.Join(root, "extra_test.go"), []byte("package example\n"), 0600)
			case "delete":
				err = os.Remove(script)
			}
			if err != nil {
				t.Fatal(err)
			}
			lease := i2Claim(t, srv, agent)
			rec = agentAPIPost(t, srv, "/api/agent/v1/release", agent, fmt.Sprintf(`{"leaseId":%q,"outcome":"done"}`, lease))
			if rec.Code != 409 || !strings.Contains(rec.Body.String(), "human review") {
				t.Fatal(rec.Code, rec.Body.String())
			}
			task, _ = controlTask(i2State(t, srv), "task-i2")
			if task.AcceptancePassed || task.PauseReason != "acceptance_inputs_changed" {
				t.Fatal(task)
			}
			if _, err := os.Stat(filepath.Join(root, "acceptance-ran")); !os.IsNotExist(err) {
				t.Fatal("changed acceptance executed", err)
			}
			rec = i2Human(t, srv, cookie, "POST", "/api/project-control/projects/project-i2/takeover", `{"action":"resume","taskId":"task-i2"}`)
			if rec.Code != 400 {
				t.Fatal("input review bypassed restart")
			}
			if change == "modify" {
				rec = i2Human(t, srv, cookie, "POST", "/api/project-control/projects/project-i2/takeover", `{"action":"resume","taskId":"task-i2","restart":true}`)
				if rec.Code != 200 {
					t.Fatal(rec.Code, rec.Body.String())
				}
				lease = i2Claim(t, srv, agent)
				rec = agentAPIPost(t, srv, "/api/agent/v1/release", agent, fmt.Sprintf(`{"leaseId":%q,"outcome":"done"}`, lease))
				if rec.Code != 200 {
					t.Fatal(rec.Code, rec.Body.String())
				}
				task, _ = controlTask(i2State(t, srv), "task-i2")
				if !task.AcceptancePassed {
					t.Fatal(task)
				}
			}
		})
	}
}

func TestR1CheckpointRejectsInvalidIDAndOversizeBody(t *testing.T) {
	srv, _, agent := setupTakeover(t)
	for _, body := range []string{
		`{"taskId":"../escape","reason":"input"}`,
		`{"taskId":"` + strings.Repeat("a", 129) + `","reason":"input"}`,
		`{"taskId":"task-i2","reason":"` + strings.Repeat("x", 1<<20) + `"}`,
	} {
		rec := agentAPIPost(t, srv, "/api/agent/v1/checkpoint", agent, body)
		if rec.Code != 400 {
			t.Fatal(rec.Code)
		}
	}
	items, err := srv.controlPlane.ListInteractions(context.Background(), "ian", 500)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.RequestKind == "agent_request" {
			t.Fatal("invalid request persisted", item)
		}
	}
}

func TestR1RepoValidationResolvesHomeAndTarget(t *testing.T) {
	root := t.TempDir()
	realHome := filepath.Join(root, "real-home")
	linkedHome := filepath.Join(root, "home")
	outside := filepath.Join(root, "outside")
	for _, dir := range []string{realHome, outside} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(realHome, linkedHome); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(realHome, "escape")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", linkedHome)
	for _, path := range []string{linkedHome, realHome} {
		if got, err := validateProjectControlRepoPath(path); err != nil || got != realHome {
			t.Fatal(got, err)
		}
	}
	// A symlink is resolved to its real target, and the target is no longer
	// required to sit under the home directory.
	if got, err := validateProjectControlRepoPath(filepath.Join(linkedHome, "escape")); err != nil || got != outside {
		t.Fatalf("symlink target = %q, err %v; want %q", got, err, outside)
	}
}

func TestR1ActivityRetentionAndDeduplication(t *testing.T) {
	state := projectControlState{}
	for i := 0; i < projectActivityLimit+10; i++ {
		recordProjectActivity(&state, projectControlRecordedEvent{ID: fmt.Sprint(i), Action: "artifact_recorded", SourceID: fmt.Sprint(i)})
	}
	if len(state.Activity) != projectActivityLimit || state.Activity[0].ID != "artifact_recorded:10" {
		t.Fatal("activity not capped")
	}
	before := append([]projectActivityEvent(nil), state.Activity...)
	recordProjectActivity(&state, projectControlRecordedEvent{ID: "replay", Action: "artifact_recorded", SourceID: "10"})
	if !reflect.DeepEqual(before, state.Activity) {
		t.Fatal("retained source counted twice")
	}
}
