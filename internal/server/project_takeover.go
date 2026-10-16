package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ianf339/roambench/internal/controlplane"
)

func (s *projectControlStore) applyExpiredClaim(username string, claim controlplane.TaskClaim) error {
	_, err := s.withStateLocked(username, func(state *projectControlState) error {
		for i := range state.Tasks {
			t := &state.Tasks[i]
			if t.ID != claim.TaskID {
				continue
			}
			// A newer lease must never be overwritten by an older expiry event.
			if active, err := s.claimStoreFor().ActiveClaimForTask(context.Background(), username, t.ID); err == nil && active.ClaimID != claim.ClaimID {
				return nil
			}
			if managedProject(*state, *t) {
				pauseControlState(state, t.ProjectID, t.ID, "heartbeat_lost")
			} else {
				t.DispatchState, t.ClaimedByRuntimeID = "ready", ""
				t.RowVersion++
			}
			return nil
		}
		return nil
	})
	return err
}

// The agent's "done" is only a request to validate. Only the server's approved
// command can produce AcceptancePassed; an artifact or dispatch approval cannot.
func (s *projectControlStore) releaseManagedTask(username string, req agentReleaseRequest) (int, map[string]interface{}, error, bool) {
	state, err := s.loadOrSeed(username)
	if err != nil {
		return 500, nil, err, true
	}
	claims := s.claimStoreFor()
	claim, err := claims.GetClaim(context.Background(), username, req.LeaseID)
	if err != nil {
		return 404, nil, err, true
	}
	task, ok := controlTask(state, claim.TaskID)
	if !ok || !managedProject(state, task) {
		return 0, nil, nil, false
	}
	if err := s.requireManagedLease(username, state, task, req.LeaseID); err != nil {
		return 409, nil, err, true
	}

	var runErr error
	if req.Outcome == "done" {
		p, _ := controlProject(state, task.ProjectID)
		dir := p.RepoPath
		if dir == "" {
			dir = s.runtimeRootDir
		}
		real, err := filepath.EvalSymlinks(dir)
		if err == nil {
			real, err = validateProjectControlRepoPath(real)
		}
		if err != nil {
			_, pauseErr := s.controlTakeover(username, task.ProjectID, projectTakeoverRequest{Action: "pause", TaskID: task.ID, Reason: "path_violation"}, true)
			if pauseErr != nil {
				return 500, nil, pauseErr, true
			}
			return 409, nil, errors.New("acceptance workspace is outside allowed paths"), true
		}
		ctx, cancel := context.WithTimeout(context.Background(), controlplane.LeaseTTL)
		defer cancel()
		// Serialize entry with pause/stop, and allow at most one validator per lease.
		_, err = s.withStateLocked(username, func(state *projectControlState) error {
			current, ok := controlTask(*state, task.ID)
			if !ok {
				return controlplane.ErrNotFound
			}
			if err := s.requireManagedLease(username, *state, current, req.LeaseID); err != nil {
				return err
			}
			if s.acceptanceRuns == nil {
				s.acceptanceRuns = map[string]context.CancelFunc{}
			}
			key := username + "/" + task.ID
			if _, exists := s.acceptanceRuns[key]; exists {
				return errors.New("acceptance already running")
			}
			s.acceptanceRuns[key] = cancel
			return nil
		})
		if err != nil {
			return 409, nil, err, true
		}
		defer func() { s.mu.Lock(); delete(s.acceptanceRuns, username+"/"+task.ID); s.mu.Unlock() }()
		cmd := exec.CommandContext(ctx, task.AcceptanceCommand[0], task.AcceptanceCommand[1:]...)
		cmd.Dir = real
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error {
			if cmd.Process != nil {
				return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			return nil
		}
		cmd.WaitDelay = 250 * time.Millisecond
		runErr = cmd.Run()
	}
	var dispatch string
	_, err = s.withStateLocked(username, func(state *projectControlState) error {
		for i := range state.Tasks {
			t := &state.Tasks[i]
			if t.ID != task.ID {
				continue
			}
			if err := s.requireManagedLease(username, *state, *t, req.LeaseID); err != nil {
				return err
			}
			outcome := req.Outcome
			if runErr != nil {
				outcome = "failed"
			}
			if _, err := claims.ReleaseClaim(context.Background(), username, req.LeaseID, outcome); err != nil {
				return err
			}
			if outcome == "done" {
				t.AcceptancePassed, t.AcceptanceFailures = true, 0
				t.DispatchState, t.ClaimedByRuntimeID = "done", ""
				appendControlEvent(state, t.ProjectID, t.ID, "acceptance_passed", "Server acceptance command passed.")
				advanceMilestoneReview(state, t.ProjectID)
			} else {
				reason := "task_failed"
				if runErr != nil {
					t.AcceptanceFailures++
					reason = "acceptance_failed"
					if t.AcceptanceFailures >= 2 {
						reason = "acceptance_failed_twice"
					}
				}
				t.AcceptancePassed = false
				pauseControlState(state, t.ProjectID, t.ID, reason)
			}
			dispatch = t.DispatchState
			t.RowVersion++
			return nil
		}
		return controlplane.ErrNotFound
	})
	if err != nil {
		return http.StatusConflict, nil, err, true
	}
	return 200, map[string]interface{}{"leaseId": req.LeaseID, "taskId": task.ID, "dispatchState": dispatch, "acceptancePassed": req.Outcome == "done" && runErr == nil}, nil, true
}

func advanceMilestoneReview(state *projectControlState, projectID string) {
	for i := range state.Projects {
		p := &state.Projects[i]
		if p.ID != projectID || p.Lifecycle.State != "executing" {
			continue
		}
		count := 0
		for _, t := range state.Tasks {
			if t.ProjectID != projectID || t.MilestoneID != p.Lifecycle.MilestoneID {
				continue
			}
			count++
			if t.DispatchState != "done" || !t.AcceptancePassed {
				return
			}
		}
		if count > 0 {
			p.Lifecycle.State = "awaiting_milestone_acceptance"
			p.RowVersion++
			appendControlEvent(state, projectID, "", "milestone_ready", "Current milestone awaits human acceptance.")
		}
	}
}

// Apply failures generated by the existing runbook/checkpoint paths as well as
// the claim path. Only new events are considered, so reads/replays cannot count
// the same failure twice.
func applyTakeoverEvents(state *projectControlState, previous map[string]bool) {
	events := append([]projectControlRecordedEvent{}, state.Events...)
	for _, event := range events {
		if previous[event.ID] {
			continue
		}
		task, ok := controlTask(*state, event.TaskID)
		if !ok || !managedProject(*state, task) {
			continue
		}
		p, _ := controlProject(*state, task.ProjectID)
		if p.Takeover.Mode != "active" {
			continue
		}
		reason := ""
		switch event.Action {
		case "task_updated":
			if task.RiskLevel == "high" {
				reason = "high_risk"
			}
		case "tool_run_failed", "phase_failed":
			reason = "task_failed"
		case "final_acceptance_rejected":
			reason = "acceptance_failed"
		case "artifact_recorded":
			for _, a := range state.Artifacts {
				if a.ID == event.SourceID && a.Outcome == "fail" {
					reason = "task_failed"
					if activityKind(a.Kind) == "test" {
						reason = "acceptance_failed"
					}
					break
				}
			}
		}
		if reason == "" {
			continue
		}
		if reason == "acceptance_failed" {
			for i := range state.Tasks {
				if state.Tasks[i].ID == task.ID {
					state.Tasks[i].AcceptanceFailures++
					state.Tasks[i].AcceptancePassed = false
					if state.Tasks[i].AcceptanceFailures >= 2 {
						reason = "acceptance_failed_twice"
					}
				}
			}
		}
		pauseControlState(state, task.ProjectID, task.ID, reason)
	}
}

func activityKind(kind string) string {
	switch kind {
	case "plan", "plan_summary", "design", "design_review", "requirements":
		return "design"
	case "test", "test_result", "test_report", "test_output", "validation", "validation_result", "final_validation", "final_acceptance":
		return "test"
	case "coding", "diff_summary", "code", "patch", "implementation":
		return "coding"
	default:
		return "uncategorized"
	}
}

func (s *projectControlStore) pauseExecutionViolation(username, taskID, reason string) error {
	state, err := s.loadOrSeed(username)
	if err != nil {
		return err
	}
	task, ok := controlTask(state, taskID)
	if !ok {
		return fmt.Errorf("task not found")
	}
	if !managedProject(state, task) {
		return nil
	}
	_, err = s.controlTakeover(username, task.ProjectID, projectTakeoverRequest{Action: "pause", TaskID: taskID, Reason: reason}, true)
	return err
}

type takeoverViolationError struct{ reason, detail string }

func (e takeoverViolationError) Error() string { return e.detail }

func checkpointPayload(state projectControlState, id string) *projectControlArtifact {
	if id == "" {
		return nil
	}
	for _, a := range state.Artifacts {
		if a.ID == id && a.Kind == "checkpoint" {
			return &a
		}
	}
	return nil
}

func sharedDirectoryConflict(state projectControlState, task projectControlTask) bool {
	// Snapshot/read phases may run concurrently; two writable phases may not
	// hold the same project's shared worktree at once.
	writable := func(t projectControlTask) bool {
		return t.CurrentPhase == "implement" || t.CurrentPhase == "write" || t.CurrentPhase == "fix_or_replan"
	}
	if !writable(task) {
		return false
	}
	for _, other := range state.Tasks {
		if other.ID != task.ID && other.ProjectID == task.ProjectID && other.DispatchState == "claimed" && writable(other) {
			return true
		}
	}
	return false
}

func (s *projectControlStore) controlledProjectDir(state projectControlState, task projectControlTask) (string, error) {
	p, _ := controlProject(state, task.ProjectID)
	dir := p.RepoPath
	if dir == "" {
		dir = s.runtimeRootDir
	}
	real, err := filepath.EvalSymlinks(dir)
	if err == nil {
		real, err = validateProjectControlRepoPath(real)
	}
	if err != nil {
		return "", takeoverViolationError{reason: "path_violation", detail: err.Error()}
	}
	return real, nil
}

func (s *projectControlStore) prepareControlledWorkspace(username string, state projectControlState, task projectControlTask, phase projectControlRunbookPhase, attemptID string) (string, error) {
	if !managedProject(state, task) {
		return s.prepareWorkspaceDirForPhase(username, task, phase, attemptID)
	}
	dir, err := s.controlledProjectDir(state, task)
	if err != nil {
		return "", err
	}
	if projectControlPhaseWorkspaceKind(phase) == projectControlWorkspaceReadOnlySnapshot {
		return s.createReadOnlySnapshotFromSource(username, task, phase, attemptID, dir)
	}
	return dir, nil
}

func (s *projectControlStore) validateControlledWorkspace(state projectControlState, task projectControlTask, dir string) error {
	root, err := s.controlledProjectDir(state, task)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	if real == root {
		return nil
	}
	// A server-created snapshot must stay inside this task's private workspace.
	for _, attempt := range state.PhaseAttempts {
		if attempt.TaskID != task.ID {
			continue
		}
		kind, path := projectControlParseWorkspaceRef(attempt.WorkspaceRef)
		if kind != projectControlWorkspaceReadOnlySnapshot || path != dir {
			continue
		}
		workspaceRoot, err := filepath.EvalSymlinks(s.workspaceRootDir)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(workspaceRoot, real)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return errors.New("tool workspace is outside approved project and snapshot paths")
}
