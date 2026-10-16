package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ianf339/roambench/internal/controlplane"
)

type projectBrief struct {
	Understanding string `json:"understanding"`
	Deliverables  string `json:"deliverables"`
	OutOfScope    string `json:"outOfScope"`
	Assumptions   string `json:"assumptions"`
}

type projectMilestone struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	AcceptanceEvidence string `json:"acceptanceEvidence"`
	Status             string `json:"status"`
}

type projectLifecycle struct {
	State         string `json:"state"`
	MilestoneID   string `json:"milestoneId,omitempty"`
	Reason        string `json:"reason,omitempty"`
	PreviousState string `json:"previousState,omitempty"`
}

type projectTakeover struct {
	Mode   string `json:"mode"` // active, paused, manual; empty legacy projects require explicit activation
	Reason string `json:"reason,omitempty"`
}

type projectNextAction struct {
	Actor string `json:"actor"`
	Text  string `json:"text"`
}

type projectContractRequest struct {
	ExpectedRowVersion int                 `json:"expectedRowVersion"`
	Brief              *projectBrief       `json:"brief,omitempty"`
	Milestones         *[]projectMilestone `json:"milestones,omitempty"`
	Lifecycle          string              `json:"lifecycle,omitempty"`
	Reason             string              `json:"reason,omitempty"`
}

type projectTakeoverRequest struct {
	Action  string `json:"action"`
	TaskID  string `json:"taskId,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Restart bool   `json:"restart,omitempty"`
}

func controlProject(state projectControlState, id string) (projectControlProject, bool) {
	for _, p := range state.Projects {
		if p.ID == id {
			return p, true
		}
	}
	return projectControlProject{}, false
}
func controlTask(state projectControlState, id string) (projectControlTask, bool) {
	for _, t := range state.Tasks {
		if t.ID == id {
			return t, true
		}
	}
	return projectControlTask{}, false
}
func managedProject(state projectControlState, task projectControlTask) bool {
	p, ok := controlProject(state, task.ProjectID)
	return ok && p.Takeover.Mode != ""
}

func validateMilestones(ms []projectMilestone) error {
	seen := map[string]bool{}
	current, todo := 0, false
	for _, m := range ms {
		if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.Name) == "" || seen[m.ID] {
			return errors.New("milestones require unique id and name")
		}
		seen[m.ID] = true
		switch m.Status {
		case "done":
			if current > 0 || todo || strings.TrimSpace(m.AcceptanceEvidence) == "" {
				return errors.New("done milestones require evidence and must precede current/todo")
			}
		case "current":
			current++
			if current > 1 || todo {
				return errors.New("only one current milestone may precede todo milestones")
			}
		case "todo":
			todo = true
		default:
			return errors.New("milestone status must be todo, current or done")
		}
	}
	return nil
}

func validateTaskContract(state projectControlState, task projectControlTask) error {
	if len(task.AcceptanceCommand) > 0 && strings.TrimSpace(task.AcceptanceCommand[0]) == "" {
		return errors.New("acceptanceCommand requires an executable")
	}
	if task.MilestoneID != "" {
		p, _ := controlProject(state, task.ProjectID)
		found := false
		for _, m := range p.Milestones {
			if m.ID == task.MilestoneID {
				found = true
			}
		}
		if !found {
			return errors.New("milestone does not belong to project")
		}
	}
	visiting := map[string]bool{task.ID: true}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return errors.New("cyclic task dependencies")
		}
		dep, ok := controlTask(state, id)
		if !ok || dep.ProjectID != task.ProjectID {
			return errors.New("dependency does not belong to project")
		}
		visiting[id] = true
		for _, next := range dep.DependsOn {
			if err := visit(next); err != nil {
				return err
			}
		}
		delete(visiting, id)
		return nil
	}
	for _, id := range task.DependsOn {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func taskDependenciesPassed(state projectControlState, task projectControlTask) bool {
	for _, id := range task.DependsOn {
		dep, ok := controlTask(state, id)
		if !ok || dep.DispatchState != "done" || !dep.AcceptancePassed || dep.PauseReason != "" {
			return false
		}
	}
	return true
}

// The same predicate is checked before INSERT and again under the JSON store
// mutex, so a concurrent pause/stop wins over a candidate snapshot.
func taskClaimAllowed(state projectControlState, task projectControlTask) bool {
	if task.PauseReason != "" {
		return false
	}
	p, _ := controlProject(state, task.ProjectID)

	if p.Takeover.Mode != "active" || p.Lifecycle.State != "executing" || len(task.AcceptanceCommand) == 0 || task.RiskLevel == "high" {
		return false
	}
	if p.Lifecycle.MilestoneID != "" && task.MilestoneID != p.Lifecycle.MilestoneID {
		return false
	}
	return taskDependenciesPassed(state, task)
}

func (s *projectControlStore) executionGate(username string, state projectControlState, task projectControlTask) error {
	if !managedProject(state, task) {
		return nil
	}
	p, _ := controlProject(state, task.ProjectID)
	if p.Takeover.Mode == "manual" && task.PauseReason == "" {
		return nil
	} // human execution only
	if !taskClaimAllowed(state, task) || task.DispatchState != "claimed" {
		return errors.New("execution paused; resume and claim the task first")
	}
	claims := s.claimStoreFor()
	if claims == nil {
		return errors.New("claim store unavailable")
	}
	claim, err := claims.ActiveClaimForTask(context.Background(), username, task.ID)
	if err != nil || !claim.LeaseExpiresAt.After(projectControlClaimNow()) {
		return errors.New("active lease required")
	}
	return nil
}

func (s *projectControlStore) requireManagedLease(username string, state projectControlState, task projectControlTask, leaseID string) error {
	if !managedProject(state, task) {
		return nil
	}
	if !taskClaimAllowed(state, task) || task.DispatchState != "claimed" {
		return errors.New("agent execution entrance is closed")
	}
	claims := s.claimStoreFor()
	if claims == nil {
		return errors.New("claim store unavailable")
	}
	claim, err := claims.GetClaim(context.Background(), username, leaseID)
	if err != nil || claim.TaskID != task.ID || claim.Status != "active" || !claim.LeaseExpiresAt.After(projectControlClaimNow()) {
		return errors.New("active task lease required")
	}
	return nil
}

func lifecycleTransition(p *projectControlProject, to, reason string) error {
	from := p.Lifecycle.State
	if from == "" {
		from = "delegated"
	}
	if to == "" || to == from {
		return nil
	}
	if to == "blocked" {
		if strings.TrimSpace(reason) == "" {
			return errors.New("blocked lifecycle requires reason")
		}
		p.Lifecycle.PreviousState, p.Lifecycle.State, p.Lifecycle.Reason = from, to, reason
		return nil
	}
	allowed := map[string]string{"delegated": "reconnaissance", "reconnaissance": "planning", "planning": "awaiting_confirmation", "awaiting_confirmation": "executing", "executing": "awaiting_milestone_acceptance"}
	if from == "blocked" {
		if to != p.Lifecycle.PreviousState {
			return errors.New("blocked lifecycle must return to previous state")
		}
	} else if from == "awaiting_milestone_acceptance" {
		if to != "executing" && to != "completed" {
			return errors.New("milestone must be accepted before continuing")
		}
	} else if allowed[from] != to {
		return fmt.Errorf("illegal lifecycle transition: %s -> %s", from, to)
	}
	if to == "executing" {
		current := ""
		for _, m := range p.Milestones {
			if m.Status == "current" {
				current = m.ID
			}
		}
		if current == "" {
			return errors.New("execution requires a current milestone")
		}
		if from == "awaiting_milestone_acceptance" && current == p.Lifecycle.MilestoneID {
			return errors.New("accept current milestone before continuing")
		}
		p.Lifecycle.MilestoneID = current
	}
	if to == "completed" {
		if len(p.Milestones) == 0 {
			return errors.New("completion requires milestones")
		}
		for _, m := range p.Milestones {
			if m.Status != "done" {
				return errors.New("all milestones must be accepted")
			}
		}
	}
	p.Lifecycle.State, p.Lifecycle.Reason, p.Lifecycle.PreviousState = to, "", ""
	return nil
}

func (s *projectControlStore) updateProjectContract(username, id string, req projectContractRequest) (projectControlProject, error) {
	var result projectControlProject
	_, err := s.withStateLocked(username, func(state *projectControlState) error {
		for i := range state.Projects {
			p := &state.Projects[i]
			if p.ID != id {
				continue
			}
			if req.ExpectedRowVersion < 1 || req.ExpectedRowVersion != p.RowVersion {
				return projectControlRowVersionConflict("project", req.ExpectedRowVersion, p.RowVersion)
			}
			if p.Takeover.Mode == "paused" && req.Lifecycle != "" && req.Lifecycle != "blocked" {
				return errors.New("resume takeover before changing blocked lifecycle")
			}
			if req.Brief != nil {
				p.Brief = *req.Brief
			}
			if req.Milestones != nil {
				if err := validateMilestones(*req.Milestones); err != nil {
					return err
				}
				p.Milestones = *req.Milestones
				for _, task := range state.Tasks {
					if task.ProjectID == id {
						if err := validateTaskContract(*state, task); err != nil {
							return err
						}
					}
				}
			}
			if req.Lifecycle == "blocked" {
				if strings.TrimSpace(req.Reason) == "" {
					return errors.New("blocked lifecycle requires reason")
				}
				pauseControlState(state, id, "", req.Reason)
			}
			if err := lifecycleTransition(p, req.Lifecycle, req.Reason); err != nil {
				return err
			}
			if p.Lifecycle.State == "executing" {
				found := false
				for _, m := range p.Milestones {
					if m.ID == p.Lifecycle.MilestoneID && m.Status == "current" {
						found = true
					}
				}
				if !found {
					return errors.New("executing milestone must remain current")
				}
			}
			p.RowVersion++
			result = *p
			appendControlEvent(state, id, "", "project_contract_updated", "Updated project contract and lifecycle.")
			return nil
		}
		return controlplane.ErrNotFound
	})
	return result, err
}

func appendControlEvent(state *projectControlState, projectID, taskID, action, detail string) {
	projectControlAppendRecordedEvent(state, projectControlRecordedEvent{ID: projectControlID("event", action), Timestamp: projectControlClaimNow().UTC().Format(time.RFC3339Nano), Actor: "server", Action: action, Detail: detail, ProjectID: projectID, TaskID: taskID})
}

func pauseProjectScope(reason string) bool {
	switch reason {
	case "path_violation", "tool_violation", "shared_directory_conflict", "high_risk":
		return true
	}
	return false
}

// No caller-provided scope: server policy decides task/dependency or project.
func pauseControlState(state *projectControlState, projectID, taskID, reason string) {
	projectWide := taskID == "" || pauseProjectScope(reason)
	affected := map[string]bool{taskID: true}
	for changed := true; changed; {
		changed = false
		for _, t := range state.Tasks {
			if t.ProjectID != projectID || affected[t.ID] {
				continue
			}
			for _, dep := range t.DependsOn {
				if affected[dep] {
					affected[t.ID] = true
					changed = true
					break
				}
			}
		}
	}
	for i := range state.Projects {
		p := &state.Projects[i]
		if p.ID != projectID || !projectWide {
			continue
		}
		p.Takeover = projectTakeover{Mode: "paused", Reason: reason}
		if p.Lifecycle.State != "blocked" {
			p.Lifecycle.PreviousState = p.Lifecycle.State
		}
		p.Lifecycle.State, p.Lifecycle.Reason = "blocked", reason
		p.RowVersion++
	}
	for i := range state.Tasks {
		t := &state.Tasks[i]
		if t.ProjectID != projectID || (!projectWide && !affected[t.ID]) || t.DispatchState == "done" {
			continue
		}
		t.PauseReason = reason
		if t.DispatchState == "claimed" || t.DispatchState == "ready" {
			t.DispatchState = "released"
		}
		t.ClaimedByRuntimeID = ""
		t.RowVersion++
	}
	for i := range state.PhaseAttempts {
		a := &state.PhaseAttempts[i]
		t, ok := controlTask(*state, a.TaskID)
		if ok && t.ProjectID == projectID && (projectWide || affected[t.ID]) && a.Status == "running" {
			a.Status, a.FailureReason, a.CompletedAt = "failed", reason, projectControlClaimNow().UTC().Format(time.RFC3339Nano)
		}
	}
	for i := range state.ToolRuns {
		r := &state.ToolRuns[i]
		t, ok := controlTask(*state, r.TaskID)
		if ok && t.ProjectID == projectID && (projectWide || affected[t.ID]) && (r.Status == "running" || r.Status == "waiting") {
			r.Status, r.Error, r.CompletedAt = "failed", reason, projectControlClaimNow().UTC().Format(time.RFC3339Nano)
		}
	}
	appendControlEvent(state, projectID, taskID, "takeover_paused", reason)
}

func durableCheckpoint(state projectControlState, taskID string) string {
	task, _ := controlTask(state, taskID)
	for i := len(state.Artifacts) - 1; i >= 0; i-- {
		a := state.Artifacts[i]
		if a.TaskID == taskID && a.Kind == "checkpoint" && a.Outcome == "pass" && strings.TrimSpace(a.Value) != "" && (task.CheckpointAfter == "" || parseProjectControlTimestamp(a.CreatedAt).After(parseProjectControlTimestamp(task.CheckpointAfter))) {
			return a.ID
		}
	}
	return ""
}

// Revoke after closing JSON gates, inside their mutex. On an IO failure the
// caller receives an error; no new authority is granted. Old leases also fail
// the JSON execution gate even if revocation needs to be retried.
func (s *projectControlStore) revokeClosedClaims(username string, state projectControlState) error {
	claims := s.claimStoreFor()
	if claims == nil {
		return nil
	}
	for _, t := range state.Tasks {
		p, _ := controlProject(state, t.ProjectID)
		if t.PauseReason == "" && p.Takeover.Mode != "manual" && p.Takeover.Mode != "paused" {
			continue
		}
		if err := claims.RevokeTaskClaims(context.Background(), username, t.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *projectControlStore) controlTakeover(username, projectID string, req projectTakeoverRequest, agent bool) (projectControlProject, error) {
	if agent && req.Action != "pause" {
		return projectControlProject{}, errors.New("agents can only request pause")
	}
	var result projectControlProject
	_, err := s.withStateLocked(username, func(state *projectControlState) error {
		pi := -1
		for i := range state.Projects {
			if state.Projects[i].ID == projectID {
				pi = i
			}
		}
		if pi < 0 {
			return controlplane.ErrNotFound
		}
		p := &state.Projects[pi]
		if req.TaskID != "" {
			t, ok := controlTask(*state, req.TaskID)
			if !ok || t.ProjectID != projectID {
				return errors.New("task does not belong to project")
			}
		}
		switch req.Action {
		case "start":
			if p.Takeover.Mode == "paused" {
				return errors.New("paused takeover requires resume")
			}
			if p.Lifecycle.State != "executing" {
				return errors.New("confirm project plan before starting takeover")
			}
			p.Takeover = projectTakeover{Mode: "active"}
		case "pause":
			if agent && p.Takeover.Mode != "active" && p.Takeover.Mode != "paused" {
				return errors.New("project is not under takeover")
			}
			if strings.TrimSpace(req.Reason) == "" {
				return errors.New("pause requires reason")
			}
			pauseControlState(state, projectID, req.TaskID, req.Reason)
		case "resume":
			if p.Takeover.Mode == "manual" {
				return errors.New("start takeover before resuming tasks")
			}
			if p.Takeover.Mode == "paused" && req.TaskID != "" {
				return errors.New("project pause requires project resume")
			}
			resumed := false
			for i := range state.Tasks {
				t := &state.Tasks[i]
				if t.ProjectID != projectID || (req.TaskID != "" && t.ID != req.TaskID) || t.PauseReason == "" {
					continue
				}
				checkpoint := durableCheckpoint(*state, t.ID)
				// Tasks never claimed have no execution to restart; paused running tasks
				// and their dependencies still require an explicit restart if no checkpoint.
				if checkpoint == "" && !req.Restart {
					return errors.New("no persisted checkpoint; explicit restart=true required")
				}
				t.ResumeCheckpointID, t.Restart, t.PauseReason = checkpoint, req.Restart, ""
				if req.Restart {
					t.ResumeCheckpointID = ""
					t.CheckpointAfter = time.Now().UTC().Format(time.RFC3339Nano)
				}
				if t.DispatchState == "released" {
					t.DispatchState = "ready"
				}
				t.ClaimedByRuntimeID = ""
				t.RowVersion++
				resumed = true
			}
			if !resumed && p.Takeover.Mode != "paused" {
				return errors.New("nothing is paused")
			}
			if p.Takeover.Mode == "paused" {
				p.Takeover = projectTakeover{Mode: "active"}
				p.Lifecycle.State, p.Lifecycle.PreviousState, p.Lifecycle.Reason = p.Lifecycle.PreviousState, "", ""
			}
		case "stop":
			p.Takeover = projectTakeover{Mode: "manual"}
			if p.Lifecycle.State == "blocked" {
				p.Lifecycle.State, p.Lifecycle.PreviousState, p.Lifecycle.Reason = p.Lifecycle.PreviousState, "", ""
			}
			for i := range state.Tasks {
				t := &state.Tasks[i]
				if t.ProjectID == projectID {
					if t.DispatchState == "claimed" || t.DispatchState == "ready" {
						t.DispatchState = "released"
					}
					t.ClaimedByRuntimeID = ""
					t.PauseReason = ""
					t.RowVersion++
				}
			}
			for i := range state.PhaseAttempts {
				a := &state.PhaseAttempts[i]
				t, ok := controlTask(*state, a.TaskID)
				if ok && t.ProjectID == projectID {
					a.Released = true
					if a.Status == "running" {
						a.Status = "failed"
						a.FailureReason = "takeover stopped"
					}
				}
			}
			for i := range state.ToolRuns {
				r := &state.ToolRuns[i]
				t, ok := controlTask(*state, r.TaskID)
				if ok && t.ProjectID == projectID && (r.Status == "running" || r.Status == "waiting") {
					r.Status = "failed"
					r.Error = "takeover stopped"
				}
			}
		default:
			return errors.New("action must be start, pause, resume or stop")
		}
		p.RowVersion++
		result = *p
		appendControlEvent(state, projectID, req.TaskID, "takeover_"+req.Action, req.Reason)
		return nil
	})
	return result, err
}

func deriveNextAction(p projectControlProject, tasks []projectControlTask, claims []controlplane.TaskClaim) projectNextAction {
	human := func(text string) projectNextAction { return projectNextAction{"human", text} }
	hermes := func(text string) projectNextAction { return projectNextAction{"Hermes", text} }
	if p.Lifecycle.State == "blocked" || p.Takeover.Mode == "paused" {
		return human("处理阻塞原因并恢复接管：" + p.Lifecycle.Reason)
	}
	switch p.Lifecycle.State {
	case "completed":
		return human("项目已完成，请查看交付物。")
	case "delegated", "reconnaissance":
		return hermes("侦察仓库并整理项目 brief。")
	case "planning":
		return hermes("完善里程碑和验收证据，提交计划确认。")
	case "awaiting_confirmation":
		return human("确认项目 brief、里程碑及执行范围。")
	case "awaiting_milestone_acceptance":
		return human("检查验收证据并验收当前里程碑。")
	}
	for _, t := range tasks {
		if t.ProjectID == p.ID && t.PauseReason != "" {
			return human("处理任务「" + t.Title + "」的暂停原因并选择续跑或重跑。")
		}
	}
	if p.Takeover.Mode == "manual" {
		return human("项目处于人工模式，请安排下一步或启动接管。")
	}
	for _, c := range claims {
		if c.Status == "active" && c.LeaseExpiresAt.After(projectControlClaimNow()) {
			for _, t := range tasks {
				if t.ProjectID == p.ID && t.ID == c.TaskID && t.DispatchState == "claimed" {
					return hermes("继续执行任务「" + t.Title + "」并提交验收证据。")
				}
			}
		}
	}
	state := projectControlState{Projects: []projectControlProject{p}, Tasks: tasks}
	for _, t := range tasks {
		if t.ProjectID != p.ID {
			continue
		}
		if t.DispatchState == "ready" {
			if len(t.AcceptanceCommand) == 0 {
				return human("为任务「" + t.Title + "」补充验收命令后重新批准。")
			}
			if taskClaimAllowed(state, t) {
				return hermes("领取已批准任务「" + t.Title + "」。")
			}
		}
	}
	return human("检查依赖、验收结果并批准下一步任务。")
}

func (s *Server) handleProjectLifecycle(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/project-control/projects/"), "/")
	if len(parts) != 2 || !isValidProjectControlID(parts[0]) {
		http.NotFound(w, r)
		return
	}
	username, id := GetUsername(r), parts[0]
	if parts[1] == "detail" && r.Method == http.MethodGet {
		s.serveProjectDetail(w, r, id)
		return
	}
	var result projectControlProject
	var err error
	switch {
	case parts[1] == "contract" && r.Method == http.MethodPatch:
		var req projectContractRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err = s.projectControl.updateProjectContract(username, id, req)
	case parts[1] == "takeover" && r.Method == http.MethodPost:
		var req projectTakeoverRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err = s.projectControl.controlTakeover(username, id, req, false)
	default:
		http.Error(w, "method not allowed", 405)
		return
	}
	if err != nil {
		status := 400
		if errors.Is(err, controlplane.ErrNotFound) {
			status = 404
		}
		var conflict projectControlConflictError
		if errors.As(err, &conflict) {
			status = 409
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) handleAgentPause(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		ProjectID string `json:"projectId"`
		projectTakeoverRequest
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || (req.Action != "" && req.Action != "pause") {
		writeJSON(w, 400, map[string]string{"error": "only pause requests are allowed"})
		return
	}
	req.Action = "pause"
	result, err := s.projectControl.controlTakeover(username, req.ProjectID, req.projectTakeoverRequest, true)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, result)
}
