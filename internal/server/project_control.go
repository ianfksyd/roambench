package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ianf339/roambench/internal/terminal"
)

const (
	projectControlProjectID              = "project-roambench-control"
	projectControlRuntimeID              = "runtime-local"
	projectControlWorkstreamUXID         = "workstream-ui-ia"
	projectControlWorkstreamRuntimeID    = "workstream-runtime"
	projectControlTaskIAID               = "task-information-architecture"
	projectControlTaskPanelID            = "task-project-panel-shell"
	projectControlTaskAttachID           = "task-terminal-attach"
	projectControlTaskApprovalsID        = "task-approvals-inbox"
	projectControlCheckpointAcceptanceID = "checkpoint-final-acceptance"
)

type projectControlSnapshot struct {
	GeneratedAt     string                     `json:"generatedAt"`
	ActiveProjectID string                     `json:"activeProjectId"`
	ApprovalsCount  int                        `json:"approvalsCount"`
	Projects        []projectControlProject    `json:"projects"`
	Workstreams     []projectControlWorkstream `json:"workstreams"`
	Tasks           []projectControlTask       `json:"tasks"`
	Sessions        []projectControlSession    `json:"sessions"`
	Runtimes        []projectControlRuntime    `json:"runtimes"`
	Checkpoints     []projectControlCheckpoint `json:"checkpoints"`
	Dashboard       projectControlDashboard    `json:"dashboard"`
}

type projectControlProject struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CurrentGoal string `json:"currentGoal"`
	RowVersion  int    `json:"rowVersion,omitempty"`
}

type projectControlWorkstream struct {
	ID           string `json:"id"`
	ProjectID    string `json:"projectId"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Priority     string `json:"priority"`
	Status       string `json:"status"`
	ScopeSummary string `json:"scopeSummary"`
	RowVersion   int    `json:"rowVersion,omitempty"`
}

type projectControlTask struct {
	ID               string                    `json:"id"`
	ProjectID        string                    `json:"projectId"`
	WorkstreamID     string                    `json:"workstreamId"`
	Title            string                    `json:"title"`
	Goal             string                    `json:"goal"`
	State            string                    `json:"state"`
	AcceptanceStatus string                    `json:"acceptanceStatus"`
	RiskLevel        string                    `json:"riskLevel"`
	Priority         string                    `json:"priority"`
	AgentLabel       string                    `json:"agentLabel"`
	RuntimeID        string                    `json:"runtimeId"`
	RecentSummary    string                    `json:"recentSummary"`
	NextStep         string                    `json:"nextStep"`
	FilesChanged     []string                  `json:"filesChanged"`
	DiffSummary      string                    `json:"diffSummary"`
	SessionIDs       []string                  `json:"sessionIds"`
	Timeline         []projectControlEvent     `json:"timeline"`
	Evidence         []projectControlEvidence  `json:"evidence"`
	Audit            []projectControlAuditItem `json:"audit"`
	RowVersion       int                       `json:"rowVersion,omitempty"`
}

type projectControlSession struct {
	ID             string   `json:"id"`
	TaskID         string   `json:"taskId"`
	TerminalID     string   `json:"terminalId,omitempty"`
	Name           string   `json:"name"`
	AgentType      string   `json:"agentType"`
	RuntimeID      string   `json:"runtimeId"`
	State          string   `json:"state"`
	Role           string   `json:"role"`
	DurationLabel  string   `json:"durationLabel"`
	StartedAt      string   `json:"startedAt"`
	Summary        string   `json:"summary"`
	Claims         []string `json:"claims"`
	Artifacts      []string `json:"artifacts"`
	SupportsAttach bool     `json:"supportsAttach"`
}

type projectControlRuntime struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	Status            string `json:"status"`
	InteractiveAttach bool   `json:"interactiveAttach"`
	HealthSummary     string `json:"healthSummary"`
}

type projectControlCheckpoint struct {
	ID              string   `json:"id"`
	TaskID          string   `json:"taskId"`
	Kind            string   `json:"kind"`
	Title           string   `json:"title"`
	Reason          string   `json:"reason"`
	Status          string   `json:"status"`
	RequestedAt     string   `json:"requestedAt"`
	AllowedActions  []string `json:"allowedActions"`
	DecisionSummary string   `json:"decisionSummary,omitempty"`
	RowVersion      int      `json:"rowVersion,omitempty"`
}

type projectControlDashboard struct {
	RunningWorkstreams int      `json:"runningWorkstreams"`
	RunningTasks       int      `json:"runningTasks"`
	BlockedTasks       int      `json:"blockedTasks"`
	PendingApprovals   int      `json:"pendingApprovals"`
	RecentFailures     []string `json:"recentFailures"`
	RecentDecisions    []string `json:"recentDecisions"`
	RuntimeHealth      []string `json:"runtimeHealth"`
	ProjectTimeline    []string `json:"projectTimeline"`
}

type projectControlEvent struct {
	Timestamp string `json:"timestamp"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
}

type projectControlEvidence struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type projectControlAuditItem struct {
	Timestamp string `json:"timestamp"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
}

type projectControlState struct {
	ActiveProjectID string                     `json:"activeProjectId"`
	Projects        []projectControlProject    `json:"projects"`
	Workstreams     []projectControlWorkstream `json:"workstreams"`
	Tasks           []projectControlTask       `json:"tasks"`
	Checkpoints     []projectControlCheckpoint `json:"checkpoints"`
	UpdatedAt       string                     `json:"updatedAt,omitempty"`
}

type projectControlDecisionRequest struct {
	Action string `json:"action"`
}

type projectControlProjectCreateRequest struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CurrentGoal string `json:"currentGoal"`
}

type projectControlWorkstreamCreateRequest struct {
	ProjectID    string `json:"projectId"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Priority     string `json:"priority"`
	ScopeSummary string `json:"scopeSummary"`
}

type projectControlTaskCreateRequest struct {
	ProjectID    string `json:"projectId"`
	WorkstreamID string `json:"workstreamId"`
	Title        string `json:"title"`
	Goal         string `json:"goal"`
	Priority     string `json:"priority"`
	RiskLevel    string `json:"riskLevel"`
}

type projectControlStore struct {
	rootDir string
	mu      sync.Mutex
}

func newProjectControlStore(basePersistDir string) *projectControlStore {
	root := filepath.Join(basePersistDir, ".project-control", "users")
	_ = os.MkdirAll(root, 0700)
	return &projectControlStore{rootDir: root}
}

func (s *projectControlStore) snapshotForUser(username string, terminals *terminal.Manager) (projectControlSnapshot, error) {
	state, err := s.loadOrSeed(username)
	if err != nil {
		return projectControlSnapshot{}, err
	}
	return buildProjectControlSnapshot(state, username, terminals), nil
}

func (s *projectControlStore) createProject(username string, req projectControlProjectCreateRequest, terminals *terminal.Manager) (projectControlSnapshot, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return projectControlSnapshot{}, errors.New("project name is required")
	}
	state, err := s.withStateLocked(username, func(state *projectControlState) error {
		key := strings.TrimSpace(req.Key)
		if key == "" {
			key = slugifyProjectControl(name)
		}
		state.Projects = append(state.Projects, projectControlProject{
			ID:          projectControlID("project", name),
			Key:         key,
			Name:        name,
			Description: strings.TrimSpace(req.Description),
			Status:      "active",
			CurrentGoal: strings.TrimSpace(req.CurrentGoal),
			RowVersion:  1,
		})
		state.ActiveProjectID = state.Projects[len(state.Projects)-1].ID
		return nil
	})
	if err != nil {
		return projectControlSnapshot{}, err
	}
	return buildProjectControlSnapshot(state, username, terminals), nil
}

func (s *projectControlStore) createWorkstream(username string, req projectControlWorkstreamCreateRequest, terminals *terminal.Manager) (projectControlSnapshot, error) {
	state, err := s.withStateLocked(username, func(state *projectControlState) error {
		projectID := strings.TrimSpace(req.ProjectID)
		if projectID == "" || !projectControlProjectExists(*state, projectID) {
			return errors.New("valid projectId is required")
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			return errors.New("workstream title is required")
		}
		state.Workstreams = append(state.Workstreams, projectControlWorkstream{
			ID:           projectControlID("workstream", title),
			ProjectID:    projectID,
			Title:        title,
			Description:  strings.TrimSpace(req.Description),
			Priority:     normalizeProjectControlPriority(req.Priority),
			Status:       "planned",
			ScopeSummary: strings.TrimSpace(req.ScopeSummary),
			RowVersion:   1,
		})
		state.ActiveProjectID = projectID
		return nil
	})
	if err != nil {
		return projectControlSnapshot{}, err
	}
	return buildProjectControlSnapshot(state, username, terminals), nil
}

func (s *projectControlStore) createTask(username string, req projectControlTaskCreateRequest, terminals *terminal.Manager) (projectControlSnapshot, error) {
	state, err := s.withStateLocked(username, func(state *projectControlState) error {
		projectID := strings.TrimSpace(req.ProjectID)
		workstreamID := strings.TrimSpace(req.WorkstreamID)
		if projectID == "" || !projectControlProjectExists(*state, projectID) {
			return errors.New("valid projectId is required")
		}
		if workstreamID == "" || !projectControlWorkstreamExists(*state, workstreamID, projectID) {
			return errors.New("valid workstreamId is required")
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			return errors.New("task title is required")
		}
		now := time.Now().UTC().Format(time.RFC3339)
		state.Tasks = append(state.Tasks, projectControlTask{
			ID:               projectControlID("task", title),
			ProjectID:        projectID,
			WorkstreamID:     workstreamID,
			Title:            title,
			Goal:             strings.TrimSpace(req.Goal),
			State:            "planned",
			AcceptanceStatus: "not_ready",
			RiskLevel:        normalizeProjectControlRisk(req.RiskLevel),
			Priority:         normalizeProjectControlPriority(req.Priority),
			AgentLabel:       "worker",
			RuntimeID:        projectControlRuntimeID,
			RecentSummary:    "Task created from the Project Panel and waiting to be scheduled.",
			NextStep:         "Open a session or connect a runtime-backed workflow to start execution.",
			FilesChanged:     []string{},
			DiffSummary:      "",
			SessionIDs:       []string{},
			Timeline: []projectControlEvent{{
				Timestamp: now,
				Actor:     "human",
				Action:    "task_created",
				Detail:    "Created from the Project Panel.",
			}},
			Evidence:   []projectControlEvidence{},
			Audit:      []projectControlAuditItem{},
			RowVersion: 1,
		})
		state.ActiveProjectID = projectID
		return nil
	})
	if err != nil {
		return projectControlSnapshot{}, err
	}
	return buildProjectControlSnapshot(state, username, terminals), nil
}

func (s *projectControlStore) recordCheckpointDecision(username, checkpointID, action string, terminals *terminal.Manager) (projectControlSnapshot, error) {
	action = strings.TrimSpace(strings.ToLower(action))
	if action != "approve" && action != "reject" && action != "reroute" {
		return projectControlSnapshot{}, errors.New("invalid action")
	}
	state, err := s.withStateLocked(username, func(state *projectControlState) error {
		idx := -1
		for index, checkpoint := range state.Checkpoints {
			if checkpoint.ID == checkpointID {
				idx = index
				break
			}
		}
		if idx == -1 {
			return errors.New("checkpoint not found")
		}
		checkpoint := state.Checkpoints[idx]
		now := time.Now().UTC().Format(time.RFC3339)
		switch action {
		case "approve":
			checkpoint.Status = "approved"
			checkpoint.AllowedActions = []string{}
			checkpoint.DecisionSummary = "Accepted by human operator"
		case "reject":
			checkpoint.Status = "rejected"
			checkpoint.AllowedActions = []string{}
			checkpoint.DecisionSummary = "Rejected by human operator"
		case "reroute":
			checkpoint.Status = "rerouted"
			checkpoint.AllowedActions = []string{}
			checkpoint.DecisionSummary = "Rerouted by human operator"
		}
		checkpoint.RowVersion += 1
		state.Checkpoints[idx] = checkpoint
		for index, task := range state.Tasks {
			if task.ID != checkpoint.TaskID {
				continue
			}
			switch action {
			case "approve":
				task.AcceptanceStatus = "accepted"
				task.RecentSummary = "Task accepted by human operator; execution remains visible for audit."
				task.NextStep = "Archive or hide accepted work with board filters."
			case "reject":
				task.State = "running"
				task.AcceptanceStatus = "rejected"
				task.RecentSummary = "Task rejected during final acceptance and sent back for revision."
				task.NextStep = "Revise the work, regenerate evidence, and resubmit for acceptance."
			case "reroute":
				task.State = "blocked"
				task.AcceptanceStatus = "not_ready"
				task.RecentSummary = "Task rerouted back into planning."
				task.NextStep = "Clarify direction, then reopen execution with updated scope."
			}
			task.RowVersion += 1
			task.Audit = append(task.Audit, projectControlAuditItem{
				Timestamp: now,
				Actor:     "human",
				Action:    action,
				Detail:    checkpoint.DecisionSummary,
			})
			state.Tasks[index] = task
			break
		}
		return nil
	})
	if err != nil {
		return projectControlSnapshot{}, err
	}
	return buildProjectControlSnapshot(state, username, terminals), nil
}

func (s *projectControlStore) loadOrSeed(username string) (projectControlState, error) {
	if strings.TrimSpace(username) == "" {
		return projectControlState{}, errors.New("missing username")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists, err := s.loadLocked(username)
	if err != nil {
		return projectControlState{}, err
	}
	if exists {
		return state, nil
	}
	state = defaultProjectControlState()
	if err := s.saveLocked(username, state); err != nil {
		return projectControlState{}, err
	}
	return state, nil
}

// withStateLocked loads the state, calls fn to mutate it, and saves it back
// — all under a single mutex scope to prevent TOCTOU races.
func (s *projectControlStore) withStateLocked(username string, fn func(*projectControlState) error) (projectControlState, error) {
	if strings.TrimSpace(username) == "" {
		return projectControlState{}, errors.New("missing username")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists, err := s.loadLocked(username)
	if err != nil {
		return projectControlState{}, err
	}
	if !exists {
		state = defaultProjectControlState()
	}
	if err := fn(&state); err != nil {
		return projectControlState{}, err
	}
	projectControlNormalizeState(&state)
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.saveLocked(username, state); err != nil {
		return projectControlState{}, err
	}
	return state, nil
}

func (s *projectControlStore) save(username string, state projectControlState) error {
	if strings.TrimSpace(username) == "" {
		return errors.New("missing username")
	}
	projectControlNormalizeState(&state)
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(username, state)
}

func (s *projectControlStore) loadLocked(username string) (projectControlState, bool, error) {
	path := s.pathFor(username)
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		legacyPath := s.legacyPathFor(username)
		legacyPayload, legacyErr := os.ReadFile(legacyPath)
		if errors.Is(legacyErr, os.ErrNotExist) {
			return projectControlState{}, false, nil
		}
		if legacyErr != nil {
			return projectControlState{}, false, legacyErr
		}
		var legacyState projectControlState
		if err := json.Unmarshal(legacyPayload, &legacyState); err != nil {
			return projectControlState{}, false, err
		}
		projectControlNormalizeState(&legacyState)
		if saveErr := s.saveLocked(username, legacyState); saveErr != nil {
			return projectControlState{}, false, saveErr
		}
		return legacyState, true, nil
	}
	if err != nil {
		return projectControlState{}, false, err
	}
	var state projectControlState
	if err := json.Unmarshal(payload, &state); err != nil {
		return projectControlState{}, false, err
	}
	projectControlNormalizeState(&state)
	return state, true, nil
}

func (s *projectControlStore) saveLocked(username string, state projectControlState) error {
	if err := os.MkdirAll(s.rootDir, 0700); err != nil {
		return err
	}
	path := s.pathFor(username)
	tmpPath := path + ".tmp"
	backupPath := path + ".bak"
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(backupPath, existing, 0600); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(tmpPath, payload, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *projectControlStore) pathFor(username string) string {
	return filepath.Join(s.rootDir, storagePathSegment(username)+".json")
}

func (s *projectControlStore) legacyPathFor(username string) string {
	return filepath.Join(filepath.Dir(s.rootDir), storagePathSegment(username)+".json")
}

func defaultProjectControlState() projectControlState {
	now := time.Now().UTC()
	stampA := now.Add(-2 * time.Hour).Format(time.RFC3339)
	stampB := now.Add(-90 * time.Minute).Format(time.RFC3339)
	stampC := now.Add(-85 * time.Minute).Format(time.RFC3339)
	stampD := now.Add(-70 * time.Minute).Format(time.RFC3339)
	stampE := now.Add(-50 * time.Minute).Format(time.RFC3339)
	stampF := now.Add(-45 * time.Minute).Format(time.RFC3339)

	return projectControlState{
		ActiveProjectID: projectControlProjectID,
		Projects: []projectControlProject{{
			ID:          projectControlProjectID,
			Key:         "roambench",
			Name:        "RoamBench Control Plane",
			Description: "Incremental project-control prototype layered on top of the existing terminal workspace.",
			Status:      "active",
			CurrentGoal: "Validate the dual-mode Terminal / Project Panel information architecture without breaking the current terminal UX.",
			RowVersion:  1,
		}},
		Workstreams: []projectControlWorkstream{
			{ID: projectControlWorkstreamUXID, ProjectID: projectControlProjectID, Title: "UI & Information Architecture", Description: "Project dashboard, workstream board, task detail, and session detail navigation.", Priority: "high", Status: "running", ScopeSummary: "Keep Terminal as the default mode and add Project Panel as a progressive drill-down control surface.", RowVersion: 1},
			{ID: projectControlWorkstreamRuntimeID, ProjectID: projectControlProjectID, Title: "Runtime Integration", Description: "Capability-gated terminal attach, approvals inbox, and runtime health visibility.", Priority: "high", Status: "running", ScopeSummary: "Reuse existing tmux/local shell infrastructure as the first runtime adapter.", RowVersion: 1},
		},
		Tasks: []projectControlTask{
			{
				ID:               projectControlTaskIAID,
				ProjectID:        projectControlProjectID,
				WorkstreamID:     projectControlWorkstreamUXID,
				Title:            "Review information architecture proposal",
				Goal:             "Confirm that the Project Panel drill-down matches the docs and is ready for explicit human acceptance.",
				State:            "execution_complete",
				AcceptanceStatus: "ready_for_acceptance",
				RiskLevel:        "medium",
				Priority:         "high",
				AgentLabel:       "reviewer",
				RuntimeID:        projectControlRuntimeID,
				RecentSummary:    "IA doc updated; approval still pending because business acceptance is tracked separately from execution completion.",
				NextStep:         "Approve, reject, or reroute the pending final acceptance checkpoint.",
				FilesChanged:     []string{"docs/project-control-discussions/information-architecture.md"},
				DiffSummary:      "Refined the top-level dual-mode navigation and clarified the Project → Workstream → Task → Session drill-down.",
				SessionIDs:       []string{"session-review-ia"},
				Timeline:         []projectControlEvent{{Timestamp: stampA, Actor: "human", Action: "submitted_doc", Detail: "Updated the information architecture proposal."}, {Timestamp: stampB, Actor: "system", Action: "completion_check_passed", Detail: "Execution marked complete; final acceptance kept separate."}},
				Evidence:         []projectControlEvidence{{Label: "Submitted file", Value: "docs/project-control-discussions/information-architecture.md"}, {Label: "Committed change", Value: "f62e6a6 docs: refine information architecture"}, {Label: "Acceptance note", Value: "Execution complete does not imply business acceptance."}},
				Audit:            []projectControlAuditItem{{Timestamp: stampC, Actor: "policy_engine", Action: "raised_checkpoint", Detail: "Generated final_acceptance checkpoint for explicit human sign-off."}},
				RowVersion:       1,
			},
			{
				ID:               projectControlTaskPanelID,
				ProjectID:        projectControlProjectID,
				WorkstreamID:     projectControlWorkstreamUXID,
				Title:            "Build Project Panel shell",
				Goal:             "Add a second top-level mode for project control while preserving Terminal as the default workspace.",
				State:            "planned",
				AcceptanceStatus: "not_ready",
				RiskLevel:        "high",
				Priority:         "high",
				AgentLabel:       "worker",
				RuntimeID:        projectControlRuntimeID,
				RecentSummary:    "Waiting for an active execution session to attach real UI work to this task.",
				NextStep:         "Start or attach a terminal session, then wire the Project Panel into the existing header and workspace shell.",
				FilesChanged:     []string{"web/index.html", "web/css/style.css", "web/js/app.js", "web/js/project-panel.js"},
				DiffSummary:      "Introduce the new Project Panel mode without regressing terminal layouts, files, or editor interactions.",
				SessionIDs:       []string{},
				Timeline:         []projectControlEvent{{Timestamp: stampD, Actor: "orchestrator", Action: "queued", Detail: "Reserved for the live implementation session."}},
				Evidence:         []projectControlEvidence{{Label: "Constraint", Value: "Terminal remains the default landing experience."}, {Label: "Primary bridge", Value: "Session Detail → Attach Terminal (capability gated)"}},
				Audit:            []projectControlAuditItem{},
				RowVersion:       1,
			},
			{
				ID:               projectControlTaskAttachID,
				ProjectID:        projectControlProjectID,
				WorkstreamID:     projectControlWorkstreamRuntimeID,
				Title:            "Capability-gate terminal attach",
				Goal:             "Only show Attach Terminal when the runtime supports interactive re-attachment.",
				State:            "planned",
				AcceptanceStatus: "not_ready",
				RiskLevel:        "medium",
				Priority:         "medium",
				AgentLabel:       "worker",
				RuntimeID:        projectControlRuntimeID,
				RecentSummary:    "Waiting for a runtime-backed session; current terminal infrastructure is the intended execution bridge.",
				NextStep:         "Expose live terminal sessions as Project Panel sessions and switch back into Terminal mode on attach.",
				FilesChanged:     []string{"internal/server/project_control.go", "web/js/app.js", "web/js/project-panel.js"},
				DiffSummary:      "Map live terminal sessions into Project Panel session cards and attach using the existing websocket path.",
				SessionIDs:       []string{},
				Timeline:         []projectControlEvent{{Timestamp: stampE, Actor: "system", Action: "runtime_scanned", Detail: "Interactive attach available via the existing terminal workspace"}},
				Evidence:         []projectControlEvidence{{Label: "Runtime health", Value: "Interactive attach available via the existing terminal workspace"}},
				Audit:            []projectControlAuditItem{},
				RowVersion:       1,
			},
			{
				ID:               projectControlTaskApprovalsID,
				ProjectID:        projectControlProjectID,
				WorkstreamID:     projectControlWorkstreamRuntimeID,
				Title:            "Wire approvals inbox",
				Goal:             "Use pending checkpoints as the single canonical record source for approval decisions.",
				State:            "blocked",
				AcceptanceStatus: "not_ready",
				RiskLevel:        "medium",
				Priority:         "medium",
				AgentLabel:       "policy_engine",
				RuntimeID:        projectControlRuntimeID,
				RecentSummary:    "Waiting for the first checkpoint action flow to be connected end-to-end.",
				NextStep:         "Submit an approve / reject / reroute decision and reflect it in task acceptance state plus audit history.",
				FilesChanged:     []string{"internal/server/project_control.go", "web/js/project-panel.js"},
				DiffSummary:      "Keep approvals as a filtered view over checkpoint records instead of a separate source of truth.",
				SessionIDs:       []string{},
				Timeline:         []projectControlEvent{{Timestamp: stampF, Actor: "system", Action: "blocked", Detail: "Waiting for decision handling API."}},
				Evidence:         []projectControlEvidence{{Label: "Canonical source", Value: "pending checkpoints"}},
				Audit:            []projectControlAuditItem{},
				RowVersion:       1,
			},
		},
		Checkpoints: []projectControlCheckpoint{{
			ID:              projectControlCheckpointAcceptanceID,
			TaskID:          projectControlTaskIAID,
			Kind:            "final_acceptance",
			Title:           "Final acceptance required",
			Reason:          "The IA redesign is execution-complete, but business acceptance must remain an explicit human decision.",
			Status:          "pending",
			RequestedAt:     stampC,
			AllowedActions:  []string{"approve", "reject", "reroute"},
			DecisionSummary: "",
			RowVersion:      1,
		}},
		UpdatedAt: now.Format(time.RFC3339Nano),
	}
}

func buildProjectControlSnapshot(state projectControlState, username string, terminals *terminal.Manager) projectControlSnapshot {
	now := time.Now().UTC()
	hasTmux := false

	if terminals != nil {
		hasTmux = terminals.HasTmux()
	}

	runtimeName := "Local shell runtime"
	runtimeKind := "local"
	healthSummary := "Interactive attach available via the existing terminal workspace"
	if hasTmux {
		runtimeName = "Local tmux runtime"
		healthSummary = "tmux detected — interactive attach and persistent sessions available"
	}
	if terminals == nil {
		healthSummary = "Runtime adapter not connected in this server instance"
	}

	projects := cloneProjectControlProjects(state.Projects)
	workstreams := cloneProjectControlWorkstreams(state.Workstreams)
	tasks := cloneProjectControlTasks(state.Tasks)
	checkpoints := cloneProjectControlCheckpoints(state.Checkpoints)
	runtimes := []projectControlRuntime{{ID: projectControlRuntimeID, Name: runtimeName, Kind: runtimeKind, Status: "online", InteractiveAttach: terminals != nil, HealthSummary: healthSummary}}
	sessions := []projectControlSession{{
		ID:             "session-review-ia",
		TaskID:         projectControlTaskIAID,
		Name:           "IA review session",
		AgentType:      "reviewer",
		RuntimeID:      projectControlRuntimeID,
		State:          "completed",
		Role:           "review",
		DurationLabel:  "18m",
		StartedAt:      now.Add(-2 * time.Hour).Format(time.RFC3339),
		Summary:        "Reviewed the information architecture draft and requested explicit final acceptance before archive.",
		Claims:         []string{"Execution is complete.", "Human acceptance is still required."},
		Artifacts:      []string{"Mermaid diagram update", "Acceptance readiness note"},
		SupportsAttach: false,
	}}

	liveTerminalSessions := []terminal.SessionInfo{}
	if terminals != nil {
		liveTerminalSessions = terminals.ListSessions(username)
		if len(liveTerminalSessions) > 1 {
			sort.Slice(liveTerminalSessions, func(i, j int) bool {
				return liveTerminalSessions[i].CreatedAt.Before(liveTerminalSessions[j].CreatedAt)
			})
		}
	}
	for index, live := range liveTerminalSessions {
		pcSession := projectControlSession{
			ID:             "session-live-" + live.ID,
			TerminalID:     live.ID,
			Name:           live.Name,
			AgentType:      "worker",
			RuntimeID:      projectControlRuntimeID,
			State:          "active",
			Role:           "implement",
			DurationLabel:  humanizeSince(live.CreatedAt, now),
			StartedAt:      live.CreatedAt.UTC().Format(time.RFC3339),
			Summary:        "Live terminal session surfaced through the Project Panel so the operator can attach back into Terminal mode.",
			Claims:         []string{"Interactive execution still in progress."},
			Artifacts:      []string{"Terminal transcript available in live session", "Existing workspace + shell context"},
			SupportsAttach: terminals != nil,
		}
		targetTaskIndex := 1
		if index == 1 {
			targetTaskIndex = 2
		} else if index >= 2 {
			targetTaskIndex = 3
			pcSession.Role = "verify"
		}
		if targetTaskIndex < len(tasks) {
			pcSession.TaskID = tasks[targetTaskIndex].ID
			tasks[targetTaskIndex].SessionIDs = appendUniqueString(tasks[targetTaskIndex].SessionIDs, pcSession.ID)
			// Only promote task state if it hasn't already been decided via approvals.
			// Tasks that were accepted, rejected, or rerouted keep their persisted state.
			if tasks[targetTaskIndex].AcceptanceStatus == "not_ready" {
				switch targetTaskIndex {
				case 1:
					tasks[targetTaskIndex].State = "running"
					tasks[targetTaskIndex].RecentSummary = "A live implementation session is attached to the Project Panel shell task."
				case 2:
					tasks[targetTaskIndex].State = "running"
					tasks[targetTaskIndex].RecentSummary = "A second live session is available to validate capability-gated attach behavior."
				case 3:
					tasks[targetTaskIndex].State = "waiting_review"
					tasks[targetTaskIndex].RecentSummary = "Additional live session detected; the approvals flow can use it as a verification lane."
				}
			}
			tasks[targetTaskIndex].Timeline = append(tasks[targetTaskIndex].Timeline, projectControlEvent{Timestamp: live.CreatedAt.UTC().Format(time.RFC3339), Actor: "worker", Action: "session_started", Detail: "Live terminal session became visible in the Project Panel."})
		}
		sessions = append(sessions, pcSession)
	}

	dashboard := buildProjectControlDashboard(tasks, checkpoints, runtimes)
	return projectControlSnapshot{
		GeneratedAt:     now.Format(time.RFC3339),
		ActiveProjectID: state.ActiveProjectID,
		ApprovalsCount:  countPendingProjectControlCheckpoints(checkpoints),
		Projects:        projects,
		Workstreams:     workstreams,
		Tasks:           tasks,
		Sessions:        sessions,
		Runtimes:        runtimes,
		Checkpoints:     checkpoints,
		Dashboard:       dashboard,
	}
}

func projectControlProjectExists(state projectControlState, projectID string) bool {
	for _, project := range state.Projects {
		if project.ID == projectID {
			return true
		}
	}
	return false
}

func projectControlWorkstreamExists(state projectControlState, workstreamID, projectID string) bool {
	for _, workstream := range state.Workstreams {
		if workstream.ID == workstreamID && workstream.ProjectID == projectID {
			return true
		}
	}
	return false
}

func buildProjectControlDashboard(tasks []projectControlTask, checkpoints []projectControlCheckpoint, runtimes []projectControlRuntime) projectControlDashboard {
	runningWorkstreams := map[string]bool{}
	runningTasks := 0
	blockedTasks := 0
	recentFailures := []string{}
	recentDecisions := []string{}
	runtimeHealth := []string{}
	projectTimeline := []string{}
	for _, task := range tasks {
		switch task.State {
		case "running", "waiting_review", "waiting_human":
			runningTasks++
			runningWorkstreams[task.WorkstreamID] = true
		case "blocked", "failed":
			blockedTasks++
			if task.State == "failed" {
				recentFailures = append(recentFailures, task.Title)
			}
		}
		if task.RecentSummary != "" {
			projectTimeline = append(projectTimeline, task.Title+": "+task.RecentSummary)
		}
		if len(task.Audit) > 0 {
			last := task.Audit[len(task.Audit)-1]
			recentDecisions = append(recentDecisions, task.Title+": "+last.Action)
		}
	}
	for _, runtime := range runtimes {
		runtimeHealth = append(runtimeHealth, runtime.Name+": "+runtime.HealthSummary)
	}
	for _, checkpoint := range checkpoints {
		if checkpoint.Status != "pending" && checkpoint.DecisionSummary != "" {
			recentDecisions = append(recentDecisions, checkpoint.Title+": "+checkpoint.DecisionSummary)
		}
	}
	if len(recentFailures) == 0 {
		recentFailures = []string{"No recent failures recorded"}
	}
	if len(recentDecisions) == 0 {
		recentDecisions = []string{"No decisions recorded yet"}
	}
	if len(projectTimeline) > 6 {
		projectTimeline = projectTimeline[:6]
	}
	return projectControlDashboard{
		RunningWorkstreams: len(runningWorkstreams),
		RunningTasks:       runningTasks,
		BlockedTasks:       blockedTasks,
		PendingApprovals:   countPendingProjectControlCheckpoints(checkpoints),
		RecentFailures:     recentFailures,
		RecentDecisions:    recentDecisions,
		RuntimeHealth:      runtimeHealth,
		ProjectTimeline:    projectTimeline,
	}
}

func countPendingProjectControlCheckpoints(checkpoints []projectControlCheckpoint) int {
	count := 0
	for _, checkpoint := range checkpoints {
		if checkpoint.Status == "pending" {
			count++
		}
	}
	return count
}

func isValidProjectControlID(id string) bool {
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func humanizeSince(start, now time.Time) string {
	if start.IsZero() || now.Before(start) {
		return "just now"
	}
	delta := now.Sub(start)
	if delta < time.Minute {
		return "<1m"
	}
	if delta < time.Hour {
		return fmt.Sprintf("%dm", int(delta.Minutes()))
	}
	hours := int(delta.Hours())
	minutes := int(delta.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func normalizeProjectControlPriority(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "low", "medium", "high", "critical":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return "medium"
	}
}

func normalizeProjectControlRisk(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "low", "medium", "high", "critical":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return "medium"
	}
}

func projectControlNormalizeState(state *projectControlState) {
	if state == nil {
		return
	}
	if state.Projects == nil {
		state.Projects = []projectControlProject{}
	}
	if state.Workstreams == nil {
		state.Workstreams = []projectControlWorkstream{}
	}
	if state.Tasks == nil {
		state.Tasks = []projectControlTask{}
	}
	if state.Checkpoints == nil {
		state.Checkpoints = []projectControlCheckpoint{}
	}
	for i := range state.Projects {
		state.Projects[i].ID = strings.TrimSpace(state.Projects[i].ID)
		state.Projects[i].Key = strings.TrimSpace(state.Projects[i].Key)
		state.Projects[i].Name = strings.TrimSpace(state.Projects[i].Name)
		state.Projects[i].Status = strings.TrimSpace(state.Projects[i].Status)
		if state.Projects[i].RowVersion < 1 {
			state.Projects[i].RowVersion = 1
		}
	}
	for i := range state.Workstreams {
		state.Workstreams[i].ID = strings.TrimSpace(state.Workstreams[i].ID)
		state.Workstreams[i].ProjectID = strings.TrimSpace(state.Workstreams[i].ProjectID)
		state.Workstreams[i].Title = strings.TrimSpace(state.Workstreams[i].Title)
		state.Workstreams[i].Priority = normalizeProjectControlPriority(state.Workstreams[i].Priority)
		if state.Workstreams[i].Status == "" {
			state.Workstreams[i].Status = "planned"
		}
		if state.Workstreams[i].RowVersion < 1 {
			state.Workstreams[i].RowVersion = 1
		}
	}
	for i := range state.Tasks {
		state.Tasks[i].ID = strings.TrimSpace(state.Tasks[i].ID)
		state.Tasks[i].ProjectID = strings.TrimSpace(state.Tasks[i].ProjectID)
		state.Tasks[i].WorkstreamID = strings.TrimSpace(state.Tasks[i].WorkstreamID)
		state.Tasks[i].Title = strings.TrimSpace(state.Tasks[i].Title)
		if state.Tasks[i].State == "" {
			state.Tasks[i].State = "planned"
		}
		if state.Tasks[i].AcceptanceStatus == "" {
			state.Tasks[i].AcceptanceStatus = "not_ready"
		}
		state.Tasks[i].Priority = normalizeProjectControlPriority(state.Tasks[i].Priority)
		state.Tasks[i].RiskLevel = normalizeProjectControlRisk(state.Tasks[i].RiskLevel)
		if state.Tasks[i].FilesChanged == nil {
			state.Tasks[i].FilesChanged = []string{}
		}
		if state.Tasks[i].SessionIDs == nil {
			state.Tasks[i].SessionIDs = []string{}
		}
		if state.Tasks[i].Timeline == nil {
			state.Tasks[i].Timeline = []projectControlEvent{}
		}
		if state.Tasks[i].Evidence == nil {
			state.Tasks[i].Evidence = []projectControlEvidence{}
		}
		if state.Tasks[i].Audit == nil {
			state.Tasks[i].Audit = []projectControlAuditItem{}
		}
		if state.Tasks[i].RowVersion < 1 {
			state.Tasks[i].RowVersion = 1
		}
	}
	for i := range state.Checkpoints {
		state.Checkpoints[i].ID = strings.TrimSpace(state.Checkpoints[i].ID)
		state.Checkpoints[i].TaskID = strings.TrimSpace(state.Checkpoints[i].TaskID)
		if state.Checkpoints[i].AllowedActions == nil {
			state.Checkpoints[i].AllowedActions = []string{}
		}
		if state.Checkpoints[i].RowVersion < 1 {
			state.Checkpoints[i].RowVersion = 1
		}
	}
	if state.ActiveProjectID == "" && len(state.Projects) > 0 {
		state.ActiveProjectID = state.Projects[0].ID
	}
}

func projectControlID(prefix, seed string) string {
	stamp := time.Now().UTC().Format("20060102-150405.000000000")
	return prefix + "-" + slugifyProjectControl(seed) + "-" + strings.ReplaceAll(stamp, ".", "")
}

func slugifyProjectControl(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "item"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "item"
	}
	return result
}

func appendUniqueString(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func cloneProjectControlProjects(items []projectControlProject) []projectControlProject {
	out := make([]projectControlProject, len(items))
	copy(out, items)
	return out
}

func cloneProjectControlWorkstreams(items []projectControlWorkstream) []projectControlWorkstream {
	out := make([]projectControlWorkstream, len(items))
	copy(out, items)
	return out
}

func cloneProjectControlTasks(items []projectControlTask) []projectControlTask {
	out := make([]projectControlTask, len(items))
	for i, item := range items {
		out[i] = item
		out[i].FilesChanged = append([]string{}, item.FilesChanged...)
		out[i].SessionIDs = append([]string{}, item.SessionIDs...)
		out[i].Timeline = append([]projectControlEvent{}, item.Timeline...)
		out[i].Evidence = append([]projectControlEvidence{}, item.Evidence...)
		out[i].Audit = append([]projectControlAuditItem{}, item.Audit...)
	}
	return out
}

func cloneProjectControlCheckpoints(items []projectControlCheckpoint) []projectControlCheckpoint {
	out := make([]projectControlCheckpoint, len(items))
	for i, item := range items {
		out[i] = item
		out[i].AllowedActions = append([]string{}, item.AllowedActions...)
	}
	return out
}

func (s *Server) handleProjectControlSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snapshot, err := s.projectControl.snapshotForUser(GetUsername(r), s.terminals)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load project control state"})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleProjectControlProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req projectControlProjectCreateRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	snapshot, err := s.projectControl.createProject(GetUsername(r), req, s.terminals)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, snapshot)
}

func (s *Server) handleProjectControlWorkstreams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req projectControlWorkstreamCreateRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	snapshot, err := s.projectControl.createWorkstream(GetUsername(r), req, s.terminals)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, snapshot)
}

func (s *Server) handleProjectControlTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req projectControlTaskCreateRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	snapshot, err := s.projectControl.createTask(GetUsername(r), req, s.terminals)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, snapshot)
}

func (s *Server) handleProjectControlCheckpointDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	prefix := "/api/project-control/checkpoints/"
	id := strings.TrimPrefix(r.URL.Path, prefix)
	if id == "" || !strings.HasSuffix(id, "/decision") {
		http.NotFound(w, r)
		return
	}
	id = strings.TrimSpace(strings.Trim(strings.TrimSuffix(id, "/decision"), "/"))
	if id == "" || len(id) > 128 || !isValidProjectControlID(id) {
		http.NotFound(w, r)
		return
	}
	var req projectControlDecisionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	snapshot, err := s.projectControl.recordCheckpointDecision(GetUsername(r), id, req.Action, s.terminals)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "checkpoint not found" {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}
