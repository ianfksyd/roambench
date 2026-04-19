package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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
	projectControlDefaultSkillID         = "code_change"
	projectControlDefaultRunbookID       = "code_change_default"
	projectControlDocsUpdateSkillID      = "docs_update"
	projectControlDocsUpdateRunbookID    = "docs_update_default"
)

const (
	projectControlToolRunTimeout       = 2 * time.Minute
	projectControlToolRunRecoveryGrace = 30 * time.Second
)

const (
	projectControlWorkspaceSharedRepo       = "shared_repo"
	projectControlWorkspaceReadOnlySnapshot = "read_only_snapshot"
)

const (
	projectControlRetainedTaskEvents        = 64
	projectControlRetainedTaskArtifacts     = 24
	projectControlRetainedTaskPhaseAttempts = 12
	projectControlRetainedTaskToolRuns      = 12
	projectControlRetainedMemoryHighlights  = 8
)

var projectControlProcessStartedAt = time.Now().UTC().Truncate(time.Second)

type projectControlSnapshot struct {
	GeneratedAt     string                       `json:"generatedAt"`
	ActiveProjectID string                       `json:"activeProjectId"`
	ApprovalsCount  int                          `json:"approvalsCount"`
	Projects        []projectControlProject      `json:"projects"`
	Workstreams     []projectControlWorkstream   `json:"workstreams"`
	Tasks           []projectControlTask         `json:"tasks"`
	Sessions        []projectControlSession      `json:"sessions"`
	Runtimes        []projectControlRuntime      `json:"runtimes"`
	Skills          []projectControlSkill        `json:"skills"`
	Runbooks        []projectControlRunbook      `json:"runbooks"`
	Tools           []projectControlToolDef      `json:"tools"`
	PhaseAttempts   []projectControlPhaseAttempt `json:"phaseAttempts"`
	ToolRuns        []projectControlToolRun      `json:"toolRuns,omitempty"`
	Artifacts       []projectControlArtifact     `json:"artifacts"`
	Checkpoints     []projectControlCheckpoint   `json:"checkpoints"`
	Decisions       []projectControlDecision     `json:"decisions,omitempty"`
	Memories        []projectControlTaskMemory   `json:"memories,omitempty"`
	Dashboard       projectControlDashboard      `json:"dashboard"`
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
	ID                   string                    `json:"id"`
	ProjectID            string                    `json:"projectId"`
	WorkstreamID         string                    `json:"workstreamId"`
	Title                string                    `json:"title"`
	Goal                 string                    `json:"goal"`
	State                string                    `json:"state"`
	AcceptanceStatus     string                    `json:"acceptanceStatus"`
	AcceptanceDecisionID string                    `json:"acceptanceDecisionId,omitempty"`
	ArchiveDecisionID    string                    `json:"archiveDecisionId,omitempty"`
	RiskLevel            string                    `json:"riskLevel"`
	Priority             string                    `json:"priority"`
	AgentLabel           string                    `json:"agentLabel"`
	RuntimeID            string                    `json:"runtimeId"`
	SelectedSkill        string                    `json:"selectedSkill"`
	RunbookID            string                    `json:"runbookId"`
	CurrentPhase         string                    `json:"currentPhase"`
	RunbookState         string                    `json:"runbookState"`
	AutoProgress         *bool                     `json:"autoProgress,omitempty"`
	MissingEvidence      []string                  `json:"missingEvidence"`
	RecentSummary        string                    `json:"recentSummary"`
	NextStep             string                    `json:"nextStep"`
	FilesChanged         []string                  `json:"filesChanged"`
	DiffSummary          string                    `json:"diffSummary"`
	SessionIDs           []string                  `json:"sessionIds"`
	Timeline             []projectControlEvent     `json:"timeline"`
	Evidence             []projectControlEvidence  `json:"evidence"`
	Audit                []projectControlAuditItem `json:"audit"`
	RowVersion           int                       `json:"rowVersion,omitempty"`
}

type projectControlSession struct {
	ID             string   `json:"id"`
	TaskID         string   `json:"taskId"`
	PhaseAttemptID string   `json:"phaseAttemptId,omitempty"`
	TerminalID     string   `json:"terminalId,omitempty"`
	Name           string   `json:"name"`
	AgentType      string   `json:"agentType"`
	RuntimeID      string   `json:"runtimeId"`
	State          string   `json:"state"`
	ExecutionRole  string   `json:"executionRole"`
	SystemRole     string   `json:"systemRole"`
	WorkspaceRef   string   `json:"workspaceRef,omitempty"`
	DurationLabel  string   `json:"durationLabel"`
	StartedAt      string   `json:"startedAt"`
	Summary        string   `json:"summary"`
	Claims         []string `json:"claims"`
	Artifacts      []string `json:"artifacts"`
	SupportsAttach bool     `json:"supportsAttach"`
}

type projectControlSkill struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Version            string            `json:"version"`
	DefaultRunbookID   string            `json:"defaultRunbookId"`
	AllowedRunbookIDs  []string          `json:"allowedRunbookIds"`
	RequiredArtifacts  []string          `json:"requiredArtifacts"`
	PermissionsByPhase map[string]string `json:"permissionsByPhase"`
}

type projectControlRunbook struct {
	ID              string                       `json:"id"`
	Skill           string                       `json:"skill"`
	Version         string                       `json:"version"`
	Phases          []projectControlRunbookPhase `json:"phases"`
	CompletionRules []string                     `json:"completionRules"`
}

type projectControlRunbookPhase struct {
	ID                string   `json:"id"`
	ExecutionRole     string   `json:"executionRole"`
	WriteAccess       string   `json:"writeAccess"`
	RequiredArtifacts []string `json:"requiredArtifacts"`
}

type projectControlPhaseAttempt struct {
	ID            string   `json:"id"`
	TaskID        string   `json:"taskId"`
	RunbookID     string   `json:"runbookId"`
	PhaseID       string   `json:"phaseId"`
	SessionID     string   `json:"sessionId,omitempty"`
	AgentType     string   `json:"agentType"`
	RuntimeID     string   `json:"runtimeId"`
	WorkspaceRef  string   `json:"workspaceRef"`
	StartedAt     string   `json:"startedAt"`
	CompletedAt   string   `json:"completedAt,omitempty"`
	Status        string   `json:"status"`
	ArtifactIDs   []string `json:"artifactIds"`
	FailureReason string   `json:"failureReason,omitempty"`
}

type projectControlArtifact struct {
	ID             string `json:"id"`
	TaskID         string `json:"taskId"`
	PhaseAttemptID string `json:"phaseAttemptId,omitempty"`
	Kind           string `json:"kind"`
	Outcome        string `json:"outcome"`
	Label          string `json:"label"`
	Value          string `json:"value"`
	CreatedAt      string `json:"createdAt"`
}

type projectControlToolRun struct {
	ID             string `json:"id"`
	TaskID         string `json:"taskId"`
	PhaseAttemptID string `json:"phaseAttemptId"`
	PhaseID        string `json:"phaseId"`
	ToolID         string `json:"toolId"`
	WorkspaceRef   string `json:"workspaceRef,omitempty"`
	Status         string `json:"status"`
	StartedAt      string `json:"startedAt"`
	CompletedAt    string `json:"completedAt,omitempty"`
	ArtifactID     string `json:"artifactId,omitempty"`
	Outcome        string `json:"outcome,omitempty"`
	Summary        string `json:"summary,omitempty"`
	Error          string `json:"error,omitempty"`
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
	ID                   string   `json:"id"`
	TaskID               string   `json:"taskId"`
	Kind                 string   `json:"kind"`
	Title                string   `json:"title"`
	Reason               string   `json:"reason"`
	Status               string   `json:"status"`
	RequestedAt          string   `json:"requestedAt"`
	ResolvedByDecisionID string   `json:"resolvedByDecisionId,omitempty"`
	AllowedActions       []string `json:"allowedActions"`
	DecisionSummary      string   `json:"decisionSummary,omitempty"`
	RowVersion           int      `json:"rowVersion,omitempty"`
}

type projectControlDecision struct {
	ID           string `json:"id"`
	DecisionType string `json:"decisionType"`
	Actor        string `json:"actor"`
	Timestamp    string `json:"timestamp"`
	Summary      string `json:"summary"`
	ProjectID    string `json:"projectId,omitempty"`
	WorkstreamID string `json:"workstreamId,omitempty"`
	TaskID       string `json:"taskId,omitempty"`
	CheckpointID string `json:"checkpointId,omitempty"`
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

type projectControlRecordedEvent struct {
	ID           string `json:"id"`
	Timestamp    string `json:"timestamp"`
	Actor        string `json:"actor"`
	Action       string `json:"action"`
	Detail       string `json:"detail"`
	ProjectID    string `json:"projectId,omitempty"`
	WorkstreamID string `json:"workstreamId,omitempty"`
	TaskID       string `json:"taskId,omitempty"`
	CheckpointID string `json:"checkpointId,omitempty"`
}

type projectControlTaskMemory struct {
	TaskID            string   `json:"taskId"`
	ProjectID         string   `json:"projectId,omitempty"`
	WorkstreamID      string   `json:"workstreamId,omitempty"`
	WindowStart       string   `json:"windowStart,omitempty"`
	WindowEnd         string   `json:"windowEnd,omitempty"`
	EventCount        int      `json:"eventCount,omitempty"`
	ArtifactCount     int      `json:"artifactCount,omitempty"`
	PhaseAttemptCount int      `json:"phaseAttemptCount,omitempty"`
	ToolRunCount      int      `json:"toolRunCount,omitempty"`
	Summary           string   `json:"summary"`
	Highlights        []string `json:"highlights,omitempty"`
	UpdatedAt         string   `json:"updatedAt,omitempty"`
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
	ActiveProjectID string                        `json:"activeProjectId"`
	Projects        []projectControlProject       `json:"projects"`
	Workstreams     []projectControlWorkstream    `json:"workstreams"`
	Tasks           []projectControlTask          `json:"tasks"`
	Tools           []projectControlToolDef       `json:"tools,omitempty"`
	AgentToken      string                        `json:"agentToken,omitempty"`
	PhaseAttempts   []projectControlPhaseAttempt  `json:"phaseAttempts,omitempty"`
	ToolRuns        []projectControlToolRun       `json:"toolRuns,omitempty"`
	Artifacts       []projectControlArtifact      `json:"artifacts,omitempty"`
	Checkpoints     []projectControlCheckpoint    `json:"checkpoints"`
	Decisions       []projectControlDecision      `json:"decisions,omitempty"`
	Events          []projectControlRecordedEvent `json:"events,omitempty"`
	Memories        []projectControlTaskMemory    `json:"memories,omitempty"`
	UpdatedAt       string                        `json:"updatedAt,omitempty"`
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
	ProjectID     string `json:"projectId"`
	WorkstreamID  string `json:"workstreamId"`
	Title         string `json:"title"`
	Goal          string `json:"goal"`
	Priority      string `json:"priority"`
	RiskLevel     string `json:"riskLevel"`
	SelectedSkill string `json:"selectedSkill"`
	RunbookID     string `json:"runbookId"`
}

type projectControlWorkstreamUpdateRequest struct {
	ExpectedRowVersion int    `json:"expectedRowVersion"`
	Action             string `json:"action"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	Priority           string `json:"priority"`
	Status             string `json:"status"`
	ScopeSummary       string `json:"scopeSummary"`
}

type projectControlTaskUpdateRequest struct {
	ExpectedRowVersion int    `json:"expectedRowVersion"`
	Action             string `json:"action"`
	Title              string `json:"title"`
	Goal               string `json:"goal"`
	Priority           string `json:"priority"`
	RiskLevel          string `json:"riskLevel"`
	State              string `json:"state"`
	AcceptanceStatus   string `json:"acceptanceStatus"`
	PhaseID            string `json:"phaseId"`
	ArtifactKind       string `json:"artifactKind"`
	ArtifactOutcome    string `json:"artifactOutcome"`
	ArtifactLabel      string `json:"artifactLabel"`
	ArtifactValue      string `json:"artifactValue"`
	FailureReason      string `json:"failureReason"`
	ToolID             string `json:"toolId"`
}

type projectControlEventsResponse struct {
	Events     []projectControlRecordedEvent `json:"events"`
	NextCursor string                        `json:"nextCursor,omitempty"`
}

type projectControlReplaySection struct {
	Kind  string                        `json:"kind"`
	Title string                        `json:"title"`
	Steps []projectControlRecordedEvent `json:"steps"`
}

type projectControlReplayTransition struct {
	Type   string `json:"type"`
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

type projectControlReplayResponse struct {
	TaskID             string                           `json:"taskId"`
	ProjectID          string                           `json:"projectId"`
	WorkstreamID       string                           `json:"workstreamId"`
	Title              string                           `json:"title"`
	CurrentState       string                           `json:"currentState"`
	AcceptanceState    string                           `json:"acceptanceState"`
	AcceptanceDecision *projectControlDecision          `json:"acceptanceDecision,omitempty"`
	ArchiveDecision    *projectControlDecision          `json:"archiveDecision,omitempty"`
	Steps              []projectControlRecordedEvent    `json:"steps"`
	Sections           []projectControlReplaySection    `json:"sections"`
	Transitions        []projectControlReplayTransition `json:"transitions"`
}

type projectControlStore struct {
	rootDir          string
	runtimeRootDir   string
	workspaceRootDir string
	mu               sync.Mutex
}

type projectControlConflictError struct {
	message string
}

func (e projectControlConflictError) Error() string {
	return e.message
}

func projectControlRowVersionConflict(entity string, expected, actual int) error {
	return projectControlConflictError{message: fmt.Sprintf("row version conflict for %s: expected %d, current %d", entity, expected, actual)}
}

func applyProjectControlWorkstreamAction(workstream *projectControlWorkstream, req *projectControlWorkstreamUpdateRequest) error {
	action := strings.TrimSpace(strings.ToLower(req.Action))
	if action == "" {
		return nil
	}
	switch action {
	case "start_execution", "resume_execution", "resume":
		req.Status = "running"
	case "mark_blocked":
		req.Status = "blocked"
	case "request_human_input":
		req.Status = "waiting_human"
	case "mark_completed":
		req.Status = "completed"
	case "archive":
		req.Status = "archived"
	default:
		return fmt.Errorf("invalid workstream action: %s", action)
	}
	if workstream != nil && strings.TrimSpace(req.Priority) == "" {
		req.Priority = workstream.Priority
	}
	return nil
}

func applyProjectControlTaskAction(task *projectControlTask, req *projectControlTaskUpdateRequest) error {
	action := strings.TrimSpace(strings.ToLower(req.Action))
	if action == "" {
		return nil
	}
	switch action {
	case "queue_task":
		req.State = "queued"
		req.AcceptanceStatus = "not_ready"
	case "resume_execution":
		req.State = "running"
		req.AcceptanceStatus = "not_ready"
	case "request_human_input":
		req.State = "waiting_human"
	case "mark_waiting_review":
		req.State = "waiting_review"
	case "mark_blocked":
		req.State = "blocked"
	case "mark_execution_complete":
		req.State = "execution_complete"
	case "mark_ready_for_acceptance":
		req.State = "execution_complete"
		req.AcceptanceStatus = "ready_for_acceptance"
	case "request_acceptance_review":
		req.State = "execution_complete"
		req.AcceptanceStatus = "under_human_review"
	case "reopen_task":
		req.State = "running"
		req.AcceptanceStatus = "not_ready"
	case "archive":
		req.State = "archived"
	case "request_archive_override":
		req.State = "execution_complete"
	case "unarchive":
		req.State = "planned"
		if task == nil || task.AcceptanceStatus != "accepted" {
			req.AcceptanceStatus = "not_ready"
		}
	case "start_phase", "complete_phase", "fail_phase", "run_tool", "start_execution":
		// Phase actions are handled by the runbook helpers because they need
		// access to PhaseAttempt and Artifact state.
	default:
		return fmt.Errorf("invalid task action: %s", action)
	}
	if task != nil {
		if strings.TrimSpace(req.Priority) == "" {
			req.Priority = task.Priority
		}
		if strings.TrimSpace(req.RiskLevel) == "" {
			req.RiskLevel = task.RiskLevel
		}
	}
	return nil
}

func isProjectControlPhaseAction(action string) bool {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "start_phase", "complete_phase", "fail_phase", "run_tool", "start_execution":
		return true
	default:
		return false
	}
}

func isAllowedProjectControlWorkstreamTransition(from, to string) bool {
	from = normalizeProjectControlWorkstreamStatus(from)
	to = normalizeProjectControlWorkstreamStatus(to)
	if from == to {
		return true
	}
	switch from {
	case "planned":
		return to == "running"
	case "running":
		return to == "blocked" || to == "waiting_human" || to == "failed" || to == "completed"
	case "blocked", "waiting_human", "failed":
		return to == "running"
	case "completed":
		return to == "archived"
	default:
		return false
	}
}

func isAllowedProjectControlTaskTransition(from, to string) bool {
	from = normalizeProjectControlTaskState(from)
	to = normalizeProjectControlTaskState(to)
	if from == to {
		return true
	}
	switch from {
	case "planned":
		return to == "queued" || to == "running"
	case "queued":
		return to == "planned" || to == "running"
	case "running":
		return to == "waiting_review" || to == "waiting_human" || to == "blocked" || to == "failed" || to == "execution_complete"
	case "waiting_review", "waiting_human", "blocked", "failed":
		return to == "running"
	case "execution_complete":
		if to == "archived" {
			return true
		}
		return to == "running" || to == "blocked"
	case "archived":
		return to == "planned"
	default:
		return false
	}
}

func validateProjectControlAcceptanceTransition(from, to, resultingTaskState string, allowDecisionTerminalStates bool) error {
	from = normalizeProjectControlAcceptanceStatus(from)
	to = normalizeProjectControlAcceptanceStatus(to)
	if from == to {
		return nil
	}
	if to == "accepted" || to == "rejected" {
		if allowDecisionTerminalStates {
			return nil
		}
		return errors.New("accepted and rejected require explicit final acceptance decision")
	}
	if to == "ready_for_acceptance" && normalizeProjectControlTaskState(resultingTaskState) != "execution_complete" {
		return errors.New("ready_for_acceptance requires execution_complete state")
	}
	if to == "under_human_review" && normalizeProjectControlTaskState(resultingTaskState) != "execution_complete" {
		return errors.New("under_human_review requires execution_complete state")
	}
	switch from {
	case "not_ready":
		if to == "ready_for_acceptance" {
			return nil
		}
	case "ready_for_acceptance":
		if to == "not_ready" {
			return nil
		}
		if to == "under_human_review" {
			return nil
		}
		return errors.New("under_human_review requires explicit checkpoint workflow")
	case "under_human_review":
		if to == "not_ready" {
			return nil
		}
	case "rejected":
		if to == "not_ready" {
			return nil
		}
	}
	return fmt.Errorf("illegal acceptance transition: %s -> %s", from, to)
}

func defaultProjectControlSkills() []projectControlSkill {
	return []projectControlSkill{
		{
			ID:               projectControlDefaultSkillID,
			Name:             "Code Change",
			Version:          "0.1",
			DefaultRunbookID: projectControlDefaultRunbookID,
			AllowedRunbookIDs: []string{
				projectControlDefaultRunbookID,
			},
			RequiredArtifacts: []string{
				"plan",
				"diff_summary",
				"test_result",
				"review_result",
				"completion_check",
			},
			PermissionsByPhase: map[string]string{
				"plan":             "read_only",
				"implement":        "scoped_write",
				"test":             "read_only",
				"review":           "read_only",
				"fix_or_replan":    "scoped_write",
				"final_validation": "read_only",
			},
		},
		{
			ID:               projectControlDocsUpdateSkillID,
			Name:             "Docs Update",
			Version:          "0.1",
			DefaultRunbookID: projectControlDocsUpdateRunbookID,
			AllowedRunbookIDs: []string{
				projectControlDocsUpdateRunbookID,
			},
			RequiredArtifacts: []string{
				"plan",
				"doc_summary",
				"review_result",
				"completion_check",
			},
			PermissionsByPhase: map[string]string{
				"plan":             "read_only",
				"write":            "scoped_write",
				"fix_or_replan":    "scoped_write",
				"review":           "read_only",
				"final_validation": "read_only",
			},
		},
	}
}

func defaultProjectControlSkill() projectControlSkill {
	skills := defaultProjectControlSkills()
	for _, skill := range skills {
		if skill.ID == projectControlDefaultSkillID {
			return skill
		}
	}
	return skills[0]
}

func defaultProjectControlRunbooks() []projectControlRunbook {
	return []projectControlRunbook{
		{
			ID:      projectControlDefaultRunbookID,
			Skill:   projectControlDefaultSkillID,
			Version: "0.1",
			Phases: []projectControlRunbookPhase{
				{ID: "plan", ExecutionRole: "plan", WriteAccess: "read_only", RequiredArtifacts: []string{"plan"}},
				{ID: "implement", ExecutionRole: "implement", WriteAccess: "scoped_write", RequiredArtifacts: []string{"diff_summary"}},
				{ID: "test", ExecutionRole: "test", WriteAccess: "read_only", RequiredArtifacts: []string{"test_result"}},
				{ID: "review", ExecutionRole: "review", WriteAccess: "read_only", RequiredArtifacts: []string{"review_result"}},
				{ID: "fix_or_replan", ExecutionRole: "implement", WriteAccess: "scoped_write", RequiredArtifacts: []string{"diff_summary"}},
				{ID: "final_validation", ExecutionRole: "verify", WriteAccess: "read_only", RequiredArtifacts: []string{"completion_check"}},
			},
			CompletionRules: []string{"plan", "diff_summary", "test_result", "review_result", "completion_check"},
		},
		{
			ID:      projectControlDocsUpdateRunbookID,
			Skill:   projectControlDocsUpdateSkillID,
			Version: "0.1",
			Phases: []projectControlRunbookPhase{
				{ID: "plan", ExecutionRole: "plan", WriteAccess: "read_only", RequiredArtifacts: []string{"plan"}},
				{ID: "write", ExecutionRole: "implement", WriteAccess: "scoped_write", RequiredArtifacts: []string{"doc_summary"}},
				{ID: "review", ExecutionRole: "review", WriteAccess: "read_only", RequiredArtifacts: []string{"review_result"}},
				{ID: "fix_or_replan", ExecutionRole: "implement", WriteAccess: "scoped_write", RequiredArtifacts: []string{"doc_summary"}},
				{ID: "final_validation", ExecutionRole: "verify", WriteAccess: "read_only", RequiredArtifacts: []string{"completion_check"}},
			},
			CompletionRules: []string{"plan", "doc_summary", "review_result", "completion_check"},
		},
	}
}

func defaultProjectControlRunbook() projectControlRunbook {
	runbooks := defaultProjectControlRunbooks()
	for _, runbook := range runbooks {
		if runbook.ID == projectControlDefaultRunbookID {
			return runbook
		}
	}
	return runbooks[0]
}

func normalizeProjectControlSkillID(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func normalizeProjectControlRunbookID(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func findProjectControlSkill(skills []projectControlSkill, skillID string) (projectControlSkill, bool) {
	skillID = normalizeProjectControlSkillID(skillID)
	for _, skill := range skills {
		if normalizeProjectControlSkillID(skill.ID) == skillID {
			return skill, true
		}
	}
	return projectControlSkill{}, false
}

func findProjectControlRunbook(runbooks []projectControlRunbook, runbookID string) (projectControlRunbook, bool) {
	runbookID = normalizeProjectControlRunbookID(runbookID)
	for _, runbook := range runbooks {
		if normalizeProjectControlRunbookID(runbook.ID) == runbookID {
			return runbook, true
		}
	}
	return projectControlRunbook{}, false
}

func projectControlSkillAllowsRunbook(skill projectControlSkill, runbookID string) bool {
	runbookID = normalizeProjectControlRunbookID(runbookID)
	if runbookID == "" {
		return false
	}
	if normalizeProjectControlRunbookID(skill.DefaultRunbookID) == runbookID {
		return true
	}
	for _, allowed := range skill.AllowedRunbookIDs {
		if normalizeProjectControlRunbookID(allowed) == runbookID {
			return true
		}
	}
	return false
}

func projectControlResolveSkillAndRunbook(task *projectControlTask) (projectControlSkill, projectControlRunbook) {
	skills := defaultProjectControlSkills()
	runbooks := defaultProjectControlRunbooks()
	skill := defaultProjectControlSkill()
	if task != nil {
		if candidate, ok := findProjectControlSkill(skills, task.SelectedSkill); ok {
			skill = candidate
		}
		task.SelectedSkill = skill.ID
	}
	runbookID := skill.DefaultRunbookID
	if task != nil && strings.TrimSpace(task.RunbookID) != "" {
		runbookID = task.RunbookID
	}
	runbook, ok := findProjectControlRunbook(runbooks, runbookID)
	if !ok || normalizeProjectControlSkillID(runbook.Skill) != normalizeProjectControlSkillID(skill.ID) || !projectControlSkillAllowsRunbook(skill, runbook.ID) {
		runbook, ok = findProjectControlRunbook(runbooks, skill.DefaultRunbookID)
		if !ok {
			runbook = defaultProjectControlRunbook()
		}
	}
	if task != nil {
		task.RunbookID = runbook.ID
	}
	return skill, runbook
}

func validateProjectControlSkillRunbookSelection(selectedSkill, runbookID string) (projectControlSkill, projectControlRunbook, error) {
	skills := defaultProjectControlSkills()
	runbooks := defaultProjectControlRunbooks()
	skillID := normalizeProjectControlSkillID(selectedSkill)
	if skillID == "" {
		skillID = projectControlDefaultSkillID
	}
	skill, ok := findProjectControlSkill(skills, skillID)
	if !ok {
		return projectControlSkill{}, projectControlRunbook{}, fmt.Errorf("unknown skill: %s", skillID)
	}
	selectedRunbookID := normalizeProjectControlRunbookID(runbookID)
	if selectedRunbookID == "" {
		selectedRunbookID = skill.DefaultRunbookID
	}
	runbook, ok := findProjectControlRunbook(runbooks, selectedRunbookID)
	if !ok {
		return projectControlSkill{}, projectControlRunbook{}, fmt.Errorf("unknown runbook: %s", selectedRunbookID)
	}
	if normalizeProjectControlSkillID(runbook.Skill) != normalizeProjectControlSkillID(skill.ID) || !projectControlSkillAllowsRunbook(skill, runbook.ID) {
		return projectControlSkill{}, projectControlRunbook{}, fmt.Errorf("runbook %s is not allowed for skill %s", runbook.ID, skill.ID)
	}
	return skill, runbook, nil
}

func projectControlRunbookForTask(task projectControlTask) projectControlRunbook {
	_, runbook := projectControlResolveSkillAndRunbook(&task)
	return runbook
}

func findProjectControlRunbookPhase(runbook projectControlRunbook, phaseID string) (projectControlRunbookPhase, bool) {
	phaseID = normalizeProjectControlPhaseID(phaseID)
	for _, phase := range runbook.Phases {
		if phase.ID == phaseID {
			return phase, true
		}
	}
	return projectControlRunbookPhase{}, false
}

func isProjectControlRecoveryPhase(phaseID string) bool {
	return normalizeProjectControlPhaseID(phaseID) == "fix_or_replan"
}

func projectControlRunbookHasPhase(runbook projectControlRunbook, phaseID string) bool {
	_, ok := findProjectControlRunbookPhase(runbook, phaseID)
	return ok
}

func projectControlRecoveryTargetPhaseID(runbook projectControlRunbook) string {
	for _, preferred := range []string{"test", "review", "final_validation"} {
		if projectControlRunbookHasPhase(runbook, preferred) {
			return preferred
		}
	}
	for i, phase := range runbook.Phases {
		if !isProjectControlRecoveryPhase(phase.ID) {
			continue
		}
		for nextIndex := i + 1; nextIndex < len(runbook.Phases); nextIndex++ {
			nextPhaseID := normalizeProjectControlPhaseID(runbook.Phases[nextIndex].ID)
			if nextPhaseID == "" || isProjectControlRecoveryPhase(nextPhaseID) {
				continue
			}
			return nextPhaseID
		}
		return "ready_for_acceptance"
	}
	return ""
}

func nextProjectControlRunbookPhaseID(runbook projectControlRunbook, phaseID string) string {
	phaseID = normalizeProjectControlPhaseID(phaseID)
	if phaseID == "" || phaseID == "ready_for_acceptance" {
		return ""
	}
	if phaseID == "fix_or_replan" {
		return projectControlRecoveryTargetPhaseID(runbook)
	}
	for i, phase := range runbook.Phases {
		if normalizeProjectControlPhaseID(phase.ID) != phaseID {
			continue
		}
		for nextIndex := i + 1; nextIndex < len(runbook.Phases); nextIndex++ {
			nextPhaseID := normalizeProjectControlPhaseID(runbook.Phases[nextIndex].ID)
			if nextPhaseID == "" || isProjectControlRecoveryPhase(nextPhaseID) {
				continue
			}
			return nextPhaseID
		}
		return "ready_for_acceptance"
	}
	return ""
}

func inferProjectControlCurrentPhase(task projectControlTask) string {
	if phase := normalizeProjectControlPhaseID(task.CurrentPhase); phase != "" {
		return phase
	}
	switch normalizeProjectControlTaskState(task.State) {
	case "planned", "queued":
		return "plan"
	case "waiting_review":
		return "review"
	case "blocked", "failed":
		return "fix_or_replan"
	case "execution_complete", "archived":
		return "ready_for_acceptance"
	default:
		return "implement"
	}
}

func inferProjectControlRunbookState(task projectControlTask) string {
	if state := normalizeProjectControlRunbookState(task.RunbookState); state != "" {
		return state
	}
	switch normalizeProjectControlTaskState(task.State) {
	case "planned", "queued":
		return "not_started"
	case "waiting_review":
		return "waiting_review"
	case "blocked", "failed":
		return "needs_fix"
	case "execution_complete":
		if task.AcceptanceStatus == "accepted" {
			return "accepted"
		}
		return "ready_for_acceptance"
	case "archived":
		return "archived"
	default:
		return "in_progress"
	}
}

func refreshProjectControlTaskRunbookFields(task *projectControlTask, artifacts []projectControlArtifact) {
	if task == nil {
		return
	}
	_, runbook := projectControlResolveSkillAndRunbook(task)
	task.CurrentPhase = inferProjectControlCurrentPhase(*task)
	task.RunbookState = inferProjectControlRunbookState(*task)
	task.MissingEvidence = projectControlMissingCompletionEvidence(*task, artifacts, runbook, "", "")
}

func projectControlPhaseAgentType(phase projectControlRunbookPhase) string {
	switch phase.ExecutionRole {
	case "review":
		return "reviewer"
	case "verify":
		return "verifier"
	case "test":
		return "tester"
	default:
		return "worker"
	}
}

func normalizeProjectControlWorkspaceKind(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case projectControlWorkspaceSharedRepo:
		return projectControlWorkspaceSharedRepo
	case projectControlWorkspaceReadOnlySnapshot:
		return projectControlWorkspaceReadOnlySnapshot
	default:
		return ""
	}
}

func projectControlPhaseWorkspaceKind(phase projectControlRunbookPhase) string {
	switch phase.WriteAccess {
	case "scoped_write":
		return projectControlWorkspaceSharedRepo
	default:
		return projectControlWorkspaceReadOnlySnapshot
	}
}

func projectControlWorkspaceRef(kind, workspaceDir string) string {
	kind = normalizeProjectControlWorkspaceKind(kind)
	if kind == "" {
		return ""
	}
	workspaceDir = strings.TrimSpace(workspaceDir)
	if workspaceDir == "" {
		return kind
	}
	return kind + ":" + filepath.Clean(workspaceDir)
}

func projectControlParseWorkspaceRef(ref string) (string, string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", ""
	}
	if index := strings.Index(ref, ":"); index > 0 {
		kind := normalizeProjectControlWorkspaceKind(ref[:index])
		if kind != "" {
			workspaceDir := strings.TrimSpace(ref[index+1:])
			if workspaceDir != "" {
				return kind, filepath.Clean(workspaceDir)
			}
			return kind, ""
		}
	}
	return normalizeProjectControlWorkspaceKind(ref), ""
}

func projectControlWorkspaceDescription(ref string) string {
	kind, workspaceDir := projectControlParseWorkspaceRef(ref)
	label := ""
	switch kind {
	case projectControlWorkspaceSharedRepo:
		label = "shared repo"
	case projectControlWorkspaceReadOnlySnapshot:
		label = "read only snapshot"
	}
	if label == "" {
		label = strings.TrimSpace(ref)
	}
	if label == "" {
		return ""
	}
	if workspaceDir == "" {
		return label
	}
	return label + " (" + workspaceDir + ")"
}

func projectControlPhaseAllowsTerminalAttach(phase projectControlRunbookPhase) bool {
	return strings.TrimSpace(strings.ToLower(phase.WriteAccess)) == "scoped_write"
}

func projectControlSessionIDForTerminal(terminalID string) string {
	terminalID = strings.TrimSpace(terminalID)
	if terminalID == "" {
		return ""
	}
	return "session-live-" + terminalID
}

func projectControlTerminalIDFromSessionID(sessionID string) string {
	const prefix = "session-live-"
	sessionID = strings.TrimSpace(sessionID)
	if strings.HasPrefix(sessionID, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(sessionID, prefix))
	}
	return ""
}

func truncateProjectControlLabel(value string, maxBytes int) string {
	value = strings.Join(strings.Fields(value), " ")
	if maxBytes < 1 || len(value) <= maxBytes {
		return value
	}
	runes := []rune(value)
	for len(runes) > 0 && len(string(runes)) > maxBytes {
		runes = runes[:len(runes)-1]
	}
	return strings.TrimSpace(string(runes))
}

func projectControlPhaseSessionName(task projectControlTask, phaseID string) string {
	phaseName := strings.ReplaceAll(normalizeProjectControlPhaseID(phaseID), "_", " ")
	if phaseName == "" {
		phaseName = "phase"
	}
	name := strings.TrimSpace(phaseName + " - " + strings.TrimSpace(task.Title))
	if name == "" {
		name = "Runbook phase"
	}
	return truncateProjectControlLabel(name, 80)
}

func createProjectControlPhaseSession(username string, task projectControlTask, phase projectControlRunbookPhase, workspaceDir string, terminals *terminal.Manager) (string, error) {
	phaseID := normalizeProjectControlPhaseID(phase.ID)
	if terminals == nil || !projectControlPhaseAllowsTerminalAttach(phase) || strings.TrimSpace(workspaceDir) == "" {
		return projectControlID("session", task.ID+"-"+phaseID), nil
	}
	session, err := terminals.CreateSessionWithOptions(username, terminal.SessionCreateOptions{
		WorkDir: workspaceDir,
		Name:    projectControlPhaseSessionName(task, phaseID),
	})
	if err != nil {
		return "", fmt.Errorf("create phase session: %w", err)
	}
	return projectControlSessionIDForTerminal(session.ID), nil
}

func projectControlTaskStateForPhase(phaseID string) string {
	switch normalizeProjectControlPhaseID(phaseID) {
	case "review":
		return "waiting_review"
	case "fix_or_replan":
		return "running"
	case "write":
		return "running"
	default:
		return "running"
	}
}

func projectControlCanStartPhase(task projectControlTask, phaseID string, runbook projectControlRunbook) error {
	phaseID = normalizeProjectControlPhaseID(phaseID)
	if phaseID == "" || phaseID == "ready_for_acceptance" {
		return errors.New("valid phaseId is required")
	}
	if _, ok := findProjectControlRunbookPhase(runbook, phaseID); !ok {
		return fmt.Errorf("unknown runbook phase: %s", phaseID)
	}
	if normalizeProjectControlTaskState(task.State) == "archived" {
		return errors.New("archived task cannot start a runbook phase")
	}
	if normalizeProjectControlAcceptanceStatus(task.AcceptanceStatus) == "accepted" {
		return errors.New("accepted task cannot start a runbook phase")
	}
	current := inferProjectControlCurrentPhase(task)
	if current == "ready_for_acceptance" {
		return errors.New("task is already ready for acceptance")
	}
	if phaseID != current {
		return fmt.Errorf("cannot start phase %s while current phase is %s", phaseID, current)
	}
	return nil
}

func findRunningProjectControlPhaseAttemptIndex(attempts []projectControlPhaseAttempt, taskID, phaseID string) int {
	for i := len(attempts) - 1; i >= 0; i-- {
		attempt := attempts[i]
		if attempt.TaskID == taskID && attempt.PhaseID == phaseID && attempt.Status == "running" {
			return i
		}
	}
	return -1
}

func startProjectControlTaskPhase(state *projectControlState, task *projectControlTask, phaseID, now, username string, resolveWorkspace func(projectControlTask, projectControlRunbookPhase, string) (string, error), terminals *terminal.Manager) error {
	if state == nil || task == nil {
		return errors.New("missing project control state")
	}
	refreshProjectControlTaskRunbookFields(task, state.Artifacts)
	runbook := projectControlRunbookForTask(*task)
	phaseID = normalizeProjectControlPhaseID(phaseID)
	if phaseID == "" {
		phaseID = task.CurrentPhase
	}
	if err := projectControlCanStartPhase(*task, phaseID, runbook); err != nil {
		return err
	}
	if findRunningProjectControlPhaseAttemptIndex(state.PhaseAttempts, task.ID, phaseID) != -1 {
		return fmt.Errorf("phase %s is already running", phaseID)
	}
	phase, _ := findProjectControlRunbookPhase(runbook, phaseID)
	attemptID := projectControlID("phase-attempt", task.ID+"-"+phaseID)
	workspaceDir := ""
	if resolveWorkspace != nil {
		var err error
		workspaceDir, err = resolveWorkspace(*task, phase, attemptID)
		if err != nil {
			return err
		}
	}
	sessionID, err := createProjectControlPhaseSession(username, *task, phase, workspaceDir, terminals)
	if err != nil {
		return err
	}
	attempt := projectControlPhaseAttempt{
		ID:           attemptID,
		TaskID:       task.ID,
		RunbookID:    task.RunbookID,
		PhaseID:      phaseID,
		SessionID:    sessionID,
		AgentType:    projectControlPhaseAgentType(phase),
		RuntimeID:    task.RuntimeID,
		WorkspaceRef: projectControlWorkspaceRef(projectControlPhaseWorkspaceKind(phase), workspaceDir),
		StartedAt:    now,
		Status:       "running",
		ArtifactIDs:  []string{},
	}
	state.PhaseAttempts = append(state.PhaseAttempts, attempt)
	task.State = projectControlTaskStateForPhase(phaseID)
	task.AcceptanceStatus = "not_ready"
	task.CurrentPhase = phaseID
	task.RunbookState = "in_progress"
	task.SessionIDs = appendUniqueString(task.SessionIDs, sessionID)
	task.RecentSummary = "Runbook phase " + phaseID + " started."
	if projectControlTerminalIDFromSessionID(sessionID) != "" {
		task.NextStep = "Use session " + sessionID + " to complete " + phaseID + " with required evidence."
	} else {
		task.NextStep = "Complete " + phaseID + " with required evidence."
	}
	eventDetail := "Started runbook phase " + phaseID + "."
	if projectControlTerminalIDFromSessionID(sessionID) != "" {
		eventDetail = "Started runbook phase " + phaseID + " in session " + sessionID + "."
	}
	projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
		ID:           projectControlID("event", "phase-started"),
		Timestamp:    now,
		Actor:        "runbook_engine",
		Action:       "phase_started",
		Detail:       eventDetail,
		ProjectID:    task.ProjectID,
		WorkstreamID: task.WorkstreamID,
		TaskID:       task.ID,
	})
	return nil
}

func projectControlArtifactKindAllowed(phase projectControlRunbookPhase, kind string) bool {
	if len(phase.RequiredArtifacts) == 0 {
		return true
	}
	for _, required := range phase.RequiredArtifacts {
		if normalizeProjectControlArtifactKind(required) == kind {
			return true
		}
	}
	return false
}

func normalizeProjectControlArtifactOutcome(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "pass", "passed", "success", "ok", "clean", "no_objection", "approved":
		return "pass"
	case "fail", "failed", "error", "blocked", "objection", "rejected":
		return "fail"
	default:
		return "recorded"
	}
}

func projectControlArtifactOutcomePasses(kind, outcome string) bool {
	kind = normalizeProjectControlArtifactKind(kind)
	outcome = normalizeProjectControlArtifactOutcome(outcome)
	switch kind {
	case "test_result", "review_result", "completion_check":
		return outcome == "pass"
	default:
		return outcome != "fail"
	}
}

func projectControlCompletionEvidenceRequirement(kind string) string {
	kind = normalizeProjectControlArtifactKind(kind)
	switch kind {
	case "test_result", "review_result", "completion_check":
		return kind + ":pass"
	default:
		return kind
	}
}

func projectControlMissingCompletionEvidence(task projectControlTask, artifacts []projectControlArtifact, runbook projectControlRunbook, extraKind, extraOutcome string) []string {
	present := map[string]bool{}
	for _, artifact := range artifacts {
		if artifact.TaskID == task.ID {
			kind := normalizeProjectControlArtifactKind(artifact.Kind)
			if projectControlArtifactOutcomePasses(kind, artifact.Outcome) {
				present[kind] = true
			}
		}
	}
	if extraKind != "" {
		kind := normalizeProjectControlArtifactKind(extraKind)
		if projectControlArtifactOutcomePasses(kind, extraOutcome) {
			present[kind] = true
		}
	}
	missing := []string{}
	for _, rule := range runbook.CompletionRules {
		kind := normalizeProjectControlArtifactKind(rule)
		if kind != "" && !present[kind] {
			missing = append(missing, projectControlCompletionEvidenceRequirement(kind))
		}
	}
	return missing
}

func recordProjectControlPhaseArtifact(state *projectControlState, task *projectControlTask, attempt *projectControlPhaseAttempt, phaseID, artifactKind, artifactOutcome, artifactLabel, artifactValue, actor, now string) projectControlArtifact {
	label := strings.TrimSpace(artifactLabel)
	if label == "" {
		label = artifactKind
	}
	value := strings.TrimSpace(artifactValue)
	if value == "" {
		value = "Recorded " + artifactKind + " for phase " + phaseID + "."
	}
	if strings.TrimSpace(actor) == "" {
		actor = "runbook_engine"
	}
	artifact := projectControlArtifact{
		ID:             projectControlID("artifact", task.ID+"-"+artifactKind),
		TaskID:         task.ID,
		PhaseAttemptID: attempt.ID,
		Kind:           artifactKind,
		Outcome:        artifactOutcome,
		Label:          label,
		Value:          value,
		CreatedAt:      now,
	}
	state.Artifacts = append(state.Artifacts, artifact)
	attempt.ArtifactIDs = append(attempt.ArtifactIDs, artifact.ID)
	task.Evidence = append(task.Evidence, projectControlEvidence{Label: label, Value: value})
	if artifactKind == "diff_summary" {
		task.DiffSummary = value
	}
	projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
		ID:           projectControlID("event", "artifact-recorded"),
		Timestamp:    now,
		Actor:        actor,
		Action:       "artifact_recorded",
		Detail:       "Recorded " + artifactKind + " artifact for phase " + phaseID + ".",
		ProjectID:    task.ProjectID,
		WorkstreamID: task.WorkstreamID,
		TaskID:       task.ID,
	})
	return artifact
}

type projectControlToolDef struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Command        []string `json:"command"`
	TimeoutSeconds int      `json:"timeoutSeconds,omitempty"`
	MaxOutputBytes int      `json:"maxOutputBytes,omitempty"`
	ArtifactKind   string   `json:"artifactKind"`
	ArtifactLabel  string   `json:"artifactLabel"`
	AllowedPhases  []string `json:"allowedPhases"`
}

func (d projectControlToolDef) timeout() time.Duration {
	if d.TimeoutSeconds > 0 {
		return time.Duration(d.TimeoutSeconds) * time.Second
	}
	return projectControlToolRunTimeout
}

func (d projectControlToolDef) maxOutput() int {
	if d.MaxOutputBytes > 0 {
		return d.MaxOutputBytes
	}
	return 3200
}

func defaultProjectControlTools() []projectControlToolDef {
	return []projectControlToolDef{
		{
			ID:           "repo_status",
			Name:         "Repo Status",
			Command:      []string{"git", "status", "--short"},
			ArtifactKind: "repo_status",
			ArtifactLabel: "Repo status",
			AllowedPhases: []string{"plan", "implement", "write", "test", "review", "fix_or_replan", "final_validation"},
		},
		{
			ID:           "diff_capture",
			Name:         "Diff Capture",
			Command:      []string{"git", "diff", "--stat", "--find-renames"},
			ArtifactKind: "diff_summary",
			ArtifactLabel: "Diff summary",
			AllowedPhases: []string{"implement", "write", "fix_or_replan", "review", "final_validation"},
		},
		{
			ID:             "go_test",
			Name:           "Go Test",
			Command:        []string{"go", "test", "./..."},
			TimeoutSeconds: 120,
			ArtifactKind:   "test_result",
			ArtifactLabel:  "Go test",
			AllowedPhases:  []string{"test", "final_validation"},
		},
	}
}

func findProjectControlToolDef(tools []projectControlToolDef, toolID string) (projectControlToolDef, bool) {
	toolID = strings.TrimSpace(strings.ToLower(toolID))
	for _, def := range tools {
		if strings.TrimSpace(strings.ToLower(def.ID)) == toolID {
			return def, true
		}
	}
	return projectControlToolDef{}, false
}

func projectControlToolsForState(state *projectControlState) []projectControlToolDef {
	if state != nil && len(state.Tools) > 0 {
		return state.Tools
	}
	return defaultProjectControlTools()
}

type projectControlToolResult struct {
	ToolID          string
	ArtifactKind    string
	ArtifactOutcome string
	ArtifactLabel   string
	ArtifactValue   string
}

var projectControlExecuteTool = executeLocalProjectControlTool
var projectControlRunToolAsync = func(fn func()) {
	go fn()
}

func normalizeProjectControlToolID(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func projectControlToolAllowedInPhase(toolID, phaseID string) bool {
	toolID = normalizeProjectControlToolID(toolID)
	phaseID = normalizeProjectControlPhaseID(phaseID)
	if toolID == "" || phaseID == "" {
		return false
	}
	def, ok := findProjectControlToolDef(defaultProjectControlTools(), toolID)
	if !ok {
		return false
	}
	if len(def.AllowedPhases) == 0 {
		return true
	}
	for _, allowed := range def.AllowedPhases {
		if normalizeProjectControlPhaseID(allowed) == phaseID {
			return true
		}
	}
	return false
}

func validateProjectControlToolForPhase(toolID string, phase projectControlRunbookPhase, attempt projectControlPhaseAttempt) error {
	toolID = normalizeProjectControlToolID(toolID)
	phaseID := normalizeProjectControlPhaseID(phase.ID)
	if !projectControlToolAllowedInPhase(toolID, phaseID) {
		return fmt.Errorf("tool %s is not allowed in phase %s", toolID, phaseID)
	}
	expectedWorkspaceKind := projectControlPhaseWorkspaceKind(phase)
	actualWorkspaceKind, _ := projectControlParseWorkspaceRef(attempt.WorkspaceRef)
	if actualWorkspaceKind != "" && expectedWorkspaceKind != "" && actualWorkspaceKind != expectedWorkspaceKind {
		return fmt.Errorf("tool %s cannot run in workspace %s for phase %s", toolID, attempt.WorkspaceRef, phaseID)
	}
	switch toolID {
	case "diff_capture":
		if phase.WriteAccess != "scoped_write" {
			return fmt.Errorf("tool %s requires scoped write phase context", toolID)
		}
	case "go_test", "repo_status":
		if attempt.WorkspaceRef == "" {
			return fmt.Errorf("tool %s requires a workspace reference", toolID)
		}
	}
	return nil
}

func projectControlPhaseRequiresArtifact(phase projectControlRunbookPhase, artifactKind string) bool {
	artifactKind = normalizeProjectControlArtifactKind(artifactKind)
	for _, required := range phase.RequiredArtifacts {
		if normalizeProjectControlArtifactKind(required) == artifactKind {
			return true
		}
	}
	return false
}

func projectControlToolCommand(toolID string) ([]string, string, string, error) {
	def, ok := findProjectControlToolDef(defaultProjectControlTools(), toolID)
	if !ok {
		return nil, "", "", fmt.Errorf("unknown tool: %s", strings.TrimSpace(toolID))
	}
	return def.Command, def.ArtifactKind, def.ArtifactLabel, nil
}

func compactProjectControlToolOutput(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxBytes < 1 || len(value) <= maxBytes {
		return value
	}
	runes := []rune(value)
	for len(runes) > 0 && len(string(runes)) > maxBytes {
		runes = runes[:len(runes)-1]
	}
	return strings.TrimSpace(string(runes)) + "\n[output truncated]"
}

func executeLocalProjectControlTool(toolID, workspaceDir string) (projectControlToolResult, error) {
	def, ok := findProjectControlToolDef(defaultProjectControlTools(), toolID)
	if !ok {
		return projectControlToolResult{}, fmt.Errorf("unknown tool: %s", strings.TrimSpace(toolID))
	}
	workspaceDir = strings.TrimSpace(workspaceDir)
	if workspaceDir == "" {
		return projectControlToolResult{}, errors.New("tool workspace is unavailable")
	}
	info, err := os.Stat(workspaceDir)
	if err != nil {
		return projectControlToolResult{}, fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return projectControlToolResult{}, fmt.Errorf("workspace is not a directory: %s", workspaceDir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), def.timeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, def.Command[0], def.Command[1:]...)
	cmd.Dir = workspaceDir
	output, runErr := cmd.CombinedOutput()
	outcome := "pass"
	status := "passed"
	if runErr != nil {
		outcome = "fail"
		status = "failed: " + runErr.Error()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		outcome = "fail"
		status = "timed out"
	}
	outputText := compactProjectControlToolOutput(string(output), def.maxOutput())
	if outputText == "" {
		outputText = "No output."
	}
	command := strings.Join(def.Command, " ")
	value := "Workspace: " + workspaceDir + "\nCommand: " + command + "\nStatus: " + status + "\nOutput:\n" + outputText
	if normalizeProjectControlToolID(toolID) == "repo_status" && strings.TrimSpace(string(output)) == "" {
		value = "Workspace: " + workspaceDir + "\nCommand: " + command + "\nStatus: clean\nOutput:\nWorking tree clean."
	}
	return projectControlToolResult{
		ToolID:          normalizeProjectControlToolID(toolID),
		ArtifactKind:    def.ArtifactKind,
		ArtifactOutcome: outcome,
		ArtifactLabel:   def.ArtifactLabel,
		ArtifactValue:   value,
	}, nil
}

func recordProjectControlToolEvent(state *projectControlState, task projectControlTask, phaseID string, result projectControlToolResult, now string) {
	projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
		ID:           projectControlID("event", "tool-executed"),
		Timestamp:    now,
		Actor:        "tool_gateway",
		Action:       "tool_executed",
		Detail:       "Ran " + result.ToolID + " for phase " + phaseID + " with outcome " + normalizeProjectControlArtifactOutcome(result.ArtifactOutcome) + ".",
		ProjectID:    task.ProjectID,
		WorkstreamID: task.WorkstreamID,
		TaskID:       task.ID,
	})
}

func findRunningProjectControlToolRunIndex(runs []projectControlToolRun, taskID, phaseAttemptID, toolID string) int {
	toolID = normalizeProjectControlToolID(toolID)
	for index, run := range runs {
		if run.TaskID == taskID && run.PhaseAttemptID == phaseAttemptID && (toolID == "" || run.ToolID == toolID) && run.Status == "running" {
			return index
		}
	}
	return -1
}

func startProjectControlTaskPhaseTool(state *projectControlState, task *projectControlTask, req projectControlTaskUpdateRequest, now string) (string, error) {
	if state == nil || task == nil {
		return "", errors.New("missing project control state")
	}
	refreshProjectControlTaskRunbookFields(task, state.Artifacts)
	runbook := projectControlRunbookForTask(*task)
	phaseID := normalizeProjectControlPhaseID(req.PhaseID)
	if phaseID == "" {
		phaseID = task.CurrentPhase
	}
	phase, ok := findProjectControlRunbookPhase(runbook, phaseID)
	if !ok {
		return "", fmt.Errorf("unknown runbook phase: %s", phaseID)
	}
	attemptIndex := findRunningProjectControlPhaseAttemptIndex(state.PhaseAttempts, task.ID, phaseID)
	if attemptIndex == -1 {
		return "", fmt.Errorf("phase %s has no running attempt", phaseID)
	}
	toolID := normalizeProjectControlToolID(req.ToolID)
	if toolID == "" {
		return "", errors.New("valid toolId is required")
	}
	attempt := state.PhaseAttempts[attemptIndex]
	if strings.TrimSpace(attempt.WorkspaceRef) == "" {
		return "", errors.New("workspace is unavailable for tool execution")
	}
	if err := validateProjectControlToolForPhase(toolID, phase, attempt); err != nil {
		return "", err
	}
	if runningIndex := findRunningProjectControlToolRunIndex(state.ToolRuns, task.ID, attempt.ID, ""); runningIndex != -1 {
		return "", fmt.Errorf("tool %s is already running for phase %s", state.ToolRuns[runningIndex].ToolID, phaseID)
	}
	toolRun := projectControlToolRun{
		ID:             projectControlID("tool-run", task.ID+"-"+phaseID+"-"+toolID),
		TaskID:         task.ID,
		PhaseAttemptID: attempt.ID,
		PhaseID:        phaseID,
		ToolID:         toolID,
		WorkspaceRef:   attempt.WorkspaceRef,
		Status:         "running",
		StartedAt:      now,
		Summary:        "Running " + toolID + " for " + phaseID + ".",
	}
	state.ToolRuns = append(state.ToolRuns, toolRun)
	task.RecentSummary = "Tool " + toolID + " started for " + phaseID + "."
	task.NextStep = "Wait for " + toolID + " to finish, then review the recorded evidence."
	projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
		ID:           projectControlID("event", "tool-run-started"),
		Timestamp:    now,
		Actor:        "tool_gateway",
		Action:       "tool_run_started",
		Detail:       "Started " + toolID + " for phase " + phaseID + ".",
		ProjectID:    task.ProjectID,
		WorkstreamID: task.WorkstreamID,
		TaskID:       task.ID,
	})
	return toolRun.ID, nil
}

func applyProjectControlToolResult(state *projectControlState, task *projectControlTask, toolRun *projectControlToolRun, result projectControlToolResult, now string) error {
	runbook := projectControlRunbookForTask(*task)
	phaseID := normalizeProjectControlPhaseID(toolRun.PhaseID)
	phase, ok := findProjectControlRunbookPhase(runbook, phaseID)
	if !ok {
		return fmt.Errorf("unknown runbook phase: %s", phaseID)
	}
	attemptIndex := findRunningProjectControlPhaseAttemptIndex(state.PhaseAttempts, task.ID, phaseID)
	if attemptIndex == -1 {
		return fmt.Errorf("phase %s has no running attempt", phaseID)
	}
	attempt := state.PhaseAttempts[attemptIndex]
	result.ToolID = normalizeProjectControlToolID(result.ToolID)
	if result.ToolID == "" {
		result.ToolID = toolRun.ToolID
	}
	result.ArtifactKind = normalizeProjectControlArtifactKind(result.ArtifactKind)
	result.ArtifactOutcome = normalizeProjectControlArtifactOutcome(result.ArtifactOutcome)
	if result.ArtifactKind == "" {
		return fmt.Errorf("tool %s did not produce an artifact kind", toolRun.ToolID)
	}
	recordProjectControlToolEvent(state, *task, phaseID, result, now)
	if projectControlPhaseRequiresArtifact(phase, result.ArtifactKind) && projectControlArtifactOutcomePasses(result.ArtifactKind, result.ArtifactOutcome) {
		beforeArtifacts := len(state.Artifacts)
		if err := completeProjectControlTaskPhase(state, task, projectControlTaskUpdateRequest{
			PhaseID:         phaseID,
			ArtifactKind:    result.ArtifactKind,
			ArtifactOutcome: result.ArtifactOutcome,
			ArtifactLabel:   result.ArtifactLabel,
			ArtifactValue:   result.ArtifactValue,
		}, now); err != nil {
			return err
		}
		if len(state.Artifacts) > beforeArtifacts {
			toolRun.ArtifactID = state.Artifacts[len(state.Artifacts)-1].ID
		}
		toolRun.Status = "completed"
		toolRun.Outcome = result.ArtifactOutcome
		toolRun.CompletedAt = now
		toolRun.Summary = "Tool " + toolRun.ToolID + " completed and advanced phase " + phaseID + "."
		return nil
	}
	artifact := recordProjectControlPhaseArtifact(state, task, &attempt, phaseID, result.ArtifactKind, result.ArtifactOutcome, result.ArtifactLabel, result.ArtifactValue, "tool_gateway", now)
	state.PhaseAttempts[attemptIndex] = attempt
	toolRun.ArtifactID = artifact.ID
	toolRun.Outcome = result.ArtifactOutcome
	toolRun.CompletedAt = now
	if projectControlPhaseRequiresArtifact(phase, result.ArtifactKind) && !projectControlArtifactOutcomePasses(result.ArtifactKind, result.ArtifactOutcome) {
		toolRun.Status = "failed"
		toolRun.Summary = "Tool " + toolRun.ToolID + " failed and routed the phase to recovery."
		return failProjectControlTaskPhase(state, task, phaseID, "Tool "+toolRun.ToolID+" failed.", now)
	}
	toolRun.Status = "completed"
	toolRun.Summary = "Tool " + toolRun.ToolID + " recorded " + result.ArtifactKind + " evidence."
	task.RecentSummary = "Tool " + toolRun.ToolID + " recorded " + result.ArtifactKind + " evidence."
	task.NextStep = "Continue " + phaseID + " or complete the phase with required evidence."
	task.MissingEvidence = projectControlMissingCompletionEvidence(*task, state.Artifacts, runbook, "", "")
	return nil
}

func (s *projectControlStore) completeProjectControlToolRun(username, toolRunID string) error {
	state, err := s.loadOrSeed(username)
	if err != nil {
		return err
	}
	var toolRun projectControlToolRun
	found := false
	for _, candidate := range state.ToolRuns {
		if candidate.ID == toolRunID {
			toolRun = candidate
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tool run not found: %s", toolRunID)
	}
	if toolRun.Status != "running" {
		return nil
	}
	workspaceDir := s.workspaceDirForRef(toolRun.WorkspaceRef)
	if workspaceDir == "" {
		for _, attempt := range state.PhaseAttempts {
			if attempt.ID == toolRun.PhaseAttemptID {
				workspaceDir = s.workspaceDirForRef(attempt.WorkspaceRef)
				break
			}
		}
	}
	result, runErr := projectControlExecuteTool(toolRun.ToolID, workspaceDir)
	now := time.Now().UTC().Format(time.RFC3339)
	var nextToolRunID string
	_, err = s.withStateLocked(username, func(state *projectControlState) error {
		runIndex := -1
		for i := range state.ToolRuns {
			if state.ToolRuns[i].ID == toolRunID {
				runIndex = i
				break
			}
		}
		if runIndex == -1 {
			return fmt.Errorf("tool run not found: %s", toolRunID)
		}
		if state.ToolRuns[runIndex].Status != "running" {
			return nil
		}
		taskIndex := -1
		for i := range state.Tasks {
			if state.Tasks[i].ID == state.ToolRuns[runIndex].TaskID {
				taskIndex = i
				break
			}
		}
		if taskIndex == -1 {
			return fmt.Errorf("tool run task not found: %s", state.ToolRuns[runIndex].TaskID)
		}
		if runErr != nil {
			state.ToolRuns[runIndex].Status = "failed"
			state.ToolRuns[runIndex].CompletedAt = now
			state.ToolRuns[runIndex].Outcome = "fail"
			state.ToolRuns[runIndex].Error = runErr.Error()
			state.ToolRuns[runIndex].Summary = "Tool " + state.ToolRuns[runIndex].ToolID + " failed before producing evidence."
			projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
				ID:           projectControlID("event", "tool-run-failed"),
				Timestamp:    now,
				Actor:        "tool_gateway",
				Action:       "tool_run_failed",
				Detail:       "Tool " + state.ToolRuns[runIndex].ToolID + " failed: " + runErr.Error(),
				ProjectID:    state.Tasks[taskIndex].ProjectID,
				WorkstreamID: state.Tasks[taskIndex].WorkstreamID,
				TaskID:       state.Tasks[taskIndex].ID,
			})
			state.Tasks[taskIndex].RecentSummary = "Tool " + state.ToolRuns[runIndex].ToolID + " failed before producing evidence."
			state.Tasks[taskIndex].NextStep = "Review the tool failure and retry or continue manually."
			state.Tasks[taskIndex].RowVersion += 1
			return nil
		}
		if err := applyProjectControlToolResult(state, &state.Tasks[taskIndex], &state.ToolRuns[runIndex], result, now); err != nil {
			state.ToolRuns[runIndex].Status = "failed"
			state.ToolRuns[runIndex].CompletedAt = now
			state.ToolRuns[runIndex].Outcome = "fail"
			state.ToolRuns[runIndex].Error = err.Error()
			state.ToolRuns[runIndex].Summary = "Tool " + state.ToolRuns[runIndex].ToolID + " result could not be applied."
			state.Tasks[taskIndex].RecentSummary = "Tool " + state.ToolRuns[runIndex].ToolID + " result could not be applied."
			state.Tasks[taskIndex].NextStep = "Review the tool failure and continue manually."
			state.Tasks[taskIndex].RowVersion += 1
			return nil
		}
		state.Tasks[taskIndex].RowVersion += 1
		syncProjectControlAcceptanceCheckpoint(state, state.Tasks[taskIndex], now)
		syncProjectControlArchiveOverrideCheckpoint(state, state.Tasks[taskIndex], now)
		if state.ToolRuns[runIndex].Status == "completed" && state.ToolRuns[runIndex].Outcome == "pass" {
			resolveWorkspace := func(_ projectControlTask, _ projectControlRunbookPhase, _ string) (string, error) {
				return s.runtimeWorkspaceDir()
			}
			chainedID, _ := autoProgressAfterPhaseComplete(state, &state.Tasks[taskIndex], username, now, resolveWorkspace, nil)
			if chainedID != "" {
				nextToolRunID = chainedID
				state.Tasks[taskIndex].RowVersion += 1
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if nextToolRunID != "" {
		projectControlRunToolAsync(func() {
			_ = s.completeProjectControlToolRun(username, nextToolRunID)
		})
	}
	return nil
}

func projectControlInterruptedToolRunReason(run projectControlToolRun, now time.Time) string {
	startedAt := parseProjectControlTimestamp(run.StartedAt)
	if startedAt.IsZero() {
		return "missing or invalid start timestamp"
	}
	if startedAt.Before(projectControlProcessStartedAt.Add(-time.Second)) {
		return "server restarted before the tool completed"
	}
	if now.After(startedAt.Add(projectControlToolRunTimeout + projectControlToolRunRecoveryGrace)) {
		return "tool exceeded the recorded execution window"
	}
	return ""
}

func projectControlRecoverInterruptedToolRuns(state *projectControlState, now time.Time) bool {
	if state == nil {
		return false
	}
	changed := false
	nowText := now.UTC().Format(time.RFC3339)
	for runIndex := range state.ToolRuns {
		run := &state.ToolRuns[runIndex]
		if run.Status != "running" {
			continue
		}
		reason := projectControlInterruptedToolRunReason(*run, now.UTC())
		if reason == "" {
			continue
		}
		run.Status = "failed"
		run.CompletedAt = nowText
		run.Outcome = "fail"
		run.Error = reason
		run.Summary = "Tool " + run.ToolID + " did not complete: " + reason + "."
		taskIndex := -1
		for i := range state.Tasks {
			if state.Tasks[i].ID == run.TaskID {
				taskIndex = i
				break
			}
		}
		if taskIndex != -1 {
			state.Tasks[taskIndex].RecentSummary = "Tool " + run.ToolID + " did not complete."
			state.Tasks[taskIndex].NextStep = "Retry the tool or continue the phase manually."
			state.Tasks[taskIndex].RowVersion += 1
			projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
				ID:           projectControlID("event", "tool-run-recovered"),
				Timestamp:    nowText,
				Actor:        "tool_gateway",
				Action:       "tool_run_failed",
				Detail:       "Tool " + run.ToolID + " failed: " + reason + ".",
				ProjectID:    state.Tasks[taskIndex].ProjectID,
				WorkstreamID: state.Tasks[taskIndex].WorkstreamID,
				TaskID:       state.Tasks[taskIndex].ID,
			})
		}
		changed = true
	}
	return changed
}

func completeProjectControlTaskPhase(state *projectControlState, task *projectControlTask, req projectControlTaskUpdateRequest, now string) error {
	if state == nil || task == nil {
		return errors.New("missing project control state")
	}
	refreshProjectControlTaskRunbookFields(task, state.Artifacts)
	runbook := projectControlRunbookForTask(*task)
	phaseID := normalizeProjectControlPhaseID(req.PhaseID)
	if phaseID == "" {
		phaseID = task.CurrentPhase
	}
	phase, ok := findProjectControlRunbookPhase(runbook, phaseID)
	if !ok {
		return fmt.Errorf("unknown runbook phase: %s", phaseID)
	}
	attemptIndex := findRunningProjectControlPhaseAttemptIndex(state.PhaseAttempts, task.ID, phaseID)
	if attemptIndex == -1 {
		return fmt.Errorf("phase %s has no running attempt", phaseID)
	}
	artifactKind := normalizeProjectControlArtifactKind(req.ArtifactKind)
	if len(phase.RequiredArtifacts) > 0 {
		if artifactKind == "" {
			return fmt.Errorf("phase %s requires artifact %s", phaseID, strings.Join(phase.RequiredArtifacts, ","))
		}
		if !projectControlArtifactKindAllowed(phase, artifactKind) {
			return fmt.Errorf("phase %s does not accept artifact %s", phaseID, artifactKind)
		}
	}
	artifactOutcome := normalizeProjectControlArtifactOutcome(req.ArtifactOutcome)
	if artifactKind != "" && !projectControlArtifactOutcomePasses(artifactKind, artifactOutcome) {
		return fmt.Errorf("phase %s artifact %s outcome must pass before completing the phase", phaseID, artifactKind)
	}
	if phaseID == "final_validation" {
		if missing := projectControlMissingCompletionEvidence(*task, state.Artifacts, runbook, artifactKind, artifactOutcome); len(missing) > 0 {
			return fmt.Errorf("completion rules not satisfied: missing %s", strings.Join(missing, ","))
		}
	}
	nextPhase := nextProjectControlRunbookPhaseID(runbook, phaseID)
	if nextPhase == "" {
		return fmt.Errorf("phase %s has no next phase in runbook %s", phaseID, runbook.ID)
	}
	attempt := state.PhaseAttempts[attemptIndex]
	if artifactKind != "" {
		recordProjectControlPhaseArtifact(state, task, &attempt, phaseID, artifactKind, artifactOutcome, req.ArtifactLabel, req.ArtifactValue, "runbook_engine", now)
	}
	attempt.Status = "completed"
	attempt.CompletedAt = now
	state.PhaseAttempts[attemptIndex] = attempt
	if nextPhase == "ready_for_acceptance" {
		task.State = "execution_complete"
		task.AcceptanceStatus = "ready_for_acceptance"
		task.CurrentPhase = "ready_for_acceptance"
		task.RunbookState = "ready_for_acceptance"
		task.RecentSummary = "Runbook completion rules passed; task is ready for human acceptance."
		task.NextStep = "Request final acceptance review to create the human checkpoint."
		task.MissingEvidence = []string{}
	} else {
		task.CurrentPhase = nextPhase
		task.RunbookState = "in_progress"
		task.State = projectControlTaskStateForPhase(nextPhase)
		task.RecentSummary = "Completed " + phaseID + "; next phase is " + nextPhase + "."
		task.NextStep = "Start " + nextPhase + " and record its required evidence."
		task.MissingEvidence = projectControlMissingCompletionEvidence(*task, state.Artifacts, runbook, "", "")
	}
	projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
		ID:           projectControlID("event", "phase-completed"),
		Timestamp:    now,
		Actor:        "runbook_engine",
		Action:       "phase_completed",
		Detail:       "Completed runbook phase " + phaseID + ".",
		ProjectID:    task.ProjectID,
		WorkstreamID: task.WorkstreamID,
		TaskID:       task.ID,
	})
	return nil
}

func projectControlTaskAutoProgressEnabled(task projectControlTask) bool {
	return task.AutoProgress == nil || *task.AutoProgress
}

func projectControlToolForPhase(tools []projectControlToolDef, phase projectControlRunbookPhase) (projectControlToolDef, bool) {
	if len(phase.RequiredArtifacts) == 0 {
		return projectControlToolDef{}, false
	}
	target := normalizeProjectControlArtifactKind(phase.RequiredArtifacts[0])
	for _, def := range tools {
		if normalizeProjectControlArtifactKind(def.ArtifactKind) == target {
			for _, allowed := range def.AllowedPhases {
				if normalizeProjectControlPhaseID(allowed) == normalizeProjectControlPhaseID(phase.ID) {
					return def, true
				}
			}
		}
	}
	return projectControlToolDef{}, false
}

func autoProgressAfterPhaseComplete(state *projectControlState, task *projectControlTask, username, now string, resolveWorkspace func(projectControlTask, projectControlRunbookPhase, string) (string, error), terminals *terminal.Manager) (string, error) {
	if !projectControlTaskAutoProgressEnabled(*task) {
		return "", nil
	}
	nextPhase := normalizeProjectControlPhaseID(task.CurrentPhase)
	if nextPhase == "" || nextPhase == "ready_for_acceptance" {
		return "", nil
	}
	runbook := projectControlRunbookForTask(*task)
	phase, ok := findProjectControlRunbookPhase(runbook, nextPhase)
	if !ok {
		return "", nil
	}
	tools := projectControlToolsForState(state)
	toolDef, hasToolMatch := projectControlToolForPhase(tools, phase)
	if !hasToolMatch {
		return "", nil
	}
	if err := startProjectControlTaskPhase(state, task, nextPhase, now, username, resolveWorkspace, terminals); err != nil {
		return "", nil
	}
	toolRunID, err := startProjectControlTaskPhaseTool(state, task, projectControlTaskUpdateRequest{
		PhaseID: nextPhase,
		ToolID:  toolDef.ID,
	}, now)
	if err != nil {
		return "", nil
	}
	projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
		ID:           projectControlID("event", "auto-progress"),
		Timestamp:    now,
		Actor:        "auto_progress",
		Action:       "auto_progress",
		Detail:       "Auto-progressed to phase " + nextPhase + " and started tool " + toolDef.ID + ".",
		ProjectID:    task.ProjectID,
		WorkstreamID: task.WorkstreamID,
		TaskID:       task.ID,
	})
	return toolRunID, nil
}

func failProjectControlTaskPhase(state *projectControlState, task *projectControlTask, phaseID, reason, now string) error {
	if state == nil || task == nil {
		return errors.New("missing project control state")
	}
	refreshProjectControlTaskRunbookFields(task, state.Artifacts)
	phaseID = normalizeProjectControlPhaseID(phaseID)
	if phaseID == "" {
		phaseID = task.CurrentPhase
	}
	attemptIndex := findRunningProjectControlPhaseAttemptIndex(state.PhaseAttempts, task.ID, phaseID)
	if attemptIndex == -1 {
		return fmt.Errorf("phase %s has no running attempt", phaseID)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Phase failed without a detailed reason."
	}
	attempt := state.PhaseAttempts[attemptIndex]
	attempt.Status = "failed"
	attempt.CompletedAt = now
	attempt.FailureReason = reason
	state.PhaseAttempts[attemptIndex] = attempt
	task.State = "failed"
	task.CurrentPhase = "fix_or_replan"
	task.RunbookState = "needs_fix"
	task.AcceptanceStatus = "not_ready"
	task.RecentSummary = "Runbook phase " + phaseID + " failed."
	recoveryTarget := nextProjectControlRunbookPhaseID(projectControlRunbookForTask(*task), "fix_or_replan")
	if recoveryTarget == "" || recoveryTarget == "ready_for_acceptance" {
		task.NextStep = "Start fix_or_replan and record recovery evidence."
	} else {
		task.NextStep = "Start fix_or_replan, then rerun " + recoveryTarget + " evidence."
	}
	task.MissingEvidence = projectControlMissingCompletionEvidence(*task, state.Artifacts, projectControlRunbookForTask(*task), "", "")
	projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
		ID:           projectControlID("event", "phase-failed"),
		Timestamp:    now,
		Actor:        "runbook_engine",
		Action:       "phase_failed",
		Detail:       "Phase " + phaseID + " failed: " + reason,
		ProjectID:    task.ProjectID,
		WorkstreamID: task.WorkstreamID,
		TaskID:       task.ID,
	})
	return nil
}

func syncProjectControlAcceptanceCheckpoint(state *projectControlState, task projectControlTask, now string) {
	if state == nil {
		return
	}
	pendingIndex := -1
	for i, checkpoint := range state.Checkpoints {
		if checkpoint.TaskID == task.ID && checkpoint.Kind == "final_acceptance" && checkpoint.Status == "pending" {
			pendingIndex = i
			break
		}
	}
	if task.AcceptanceStatus == "under_human_review" {
		if pendingIndex != -1 {
			return
		}
		checkpointID := projectControlID("checkpoint", task.ID+"-final-acceptance")
		checkpoint := projectControlCheckpoint{
			ID:             checkpointID,
			TaskID:         task.ID,
			Kind:           "final_acceptance",
			Title:          "Final acceptance required",
			Reason:         "Task entered human acceptance review after reaching ready_for_acceptance.",
			Status:         "pending",
			RequestedAt:    now,
			AllowedActions: []string{"approve", "reject", "reroute"},
			RowVersion:     1,
		}
		state.Checkpoints = append(state.Checkpoints, checkpoint)
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
			ID:           projectControlID("event", "checkpoint-raised"),
			Timestamp:    now,
			Actor:        "policy_engine",
			Action:       "checkpoint_raised",
			Detail:       "Generated final_acceptance checkpoint after entering under_human_review.",
			ProjectID:    task.ProjectID,
			WorkstreamID: task.WorkstreamID,
			TaskID:       task.ID,
			CheckpointID: checkpoint.ID,
		})
		return
	}
	if pendingIndex != -1 && (task.AcceptanceStatus == "not_ready" || task.AcceptanceStatus == "accepted" || task.AcceptanceStatus == "rejected") {
		checkpoint := state.Checkpoints[pendingIndex]
		checkpoint.Status = "expired"
		checkpoint.AllowedActions = []string{}
		checkpoint.DecisionSummary = "Expired after task left acceptance review prerequisites."
		checkpoint.RowVersion += 1
		state.Checkpoints[pendingIndex] = checkpoint
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
			ID:           projectControlID("event", "checkpoint-expired"),
			Timestamp:    now,
			Actor:        "policy_engine",
			Action:       "checkpoint_expired",
			Detail:       checkpoint.DecisionSummary,
			ProjectID:    task.ProjectID,
			WorkstreamID: task.WorkstreamID,
			TaskID:       task.ID,
			CheckpointID: checkpoint.ID,
		})
	}
}

func syncProjectControlArchiveOverrideCheckpoint(state *projectControlState, task projectControlTask, now string) {
	if state == nil {
		return
	}
	pendingIndex := -1
	for i, checkpoint := range state.Checkpoints {
		if checkpoint.TaskID == task.ID && checkpoint.Kind == "archive_override" && checkpoint.Status == "pending" {
			pendingIndex = i
			break
		}
	}
	if pendingIndex == -1 {
		return
	}
	if task.State == "execution_complete" && task.AcceptanceStatus == "not_ready" {
		return
	}
	checkpoint := state.Checkpoints[pendingIndex]
	checkpoint.Status = "expired"
	checkpoint.AllowedActions = []string{}
	checkpoint.DecisionSummary = "Expired after task left archive override review prerequisites."
	checkpoint.RowVersion += 1
	state.Checkpoints[pendingIndex] = checkpoint
	projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
		ID:           projectControlID("event", "checkpoint-expired"),
		Timestamp:    now,
		Actor:        "policy_engine",
		Action:       "checkpoint_expired",
		Detail:       checkpoint.DecisionSummary,
		ProjectID:    task.ProjectID,
		WorkstreamID: task.WorkstreamID,
		TaskID:       task.ID,
		CheckpointID: checkpoint.ID,
	})
}

func requestProjectControlArchiveOverrideCheckpoint(state *projectControlState, task projectControlTask, now string) error {
	if state == nil {
		return errors.New("missing state")
	}
	if task.State != "execution_complete" {
		return errors.New("archive override requires execution_complete task")
	}
	if task.AcceptanceStatus == "accepted" {
		return errors.New("archive override is only for unaccepted tasks")
	}
	if task.AcceptanceStatus == "under_human_review" {
		return errors.New("archive override unavailable while final acceptance review is pending")
	}
	for _, checkpoint := range state.Checkpoints {
		if checkpoint.TaskID == task.ID && checkpoint.Kind == "archive_override" && checkpoint.Status == "pending" {
			return errors.New("archive override review already pending")
		}
	}
	checkpointID := projectControlID("checkpoint", task.ID+"-archive-override")
	checkpoint := projectControlCheckpoint{
		ID:             checkpointID,
		TaskID:         task.ID,
		Kind:           "archive_override",
		Title:          "Archive override requested",
		Reason:         "Task is execution-complete but not accepted; explicit human archive override is required.",
		Status:         "pending",
		RequestedAt:    now,
		AllowedActions: []string{"approve", "reject"},
		RowVersion:     1,
	}
	state.Checkpoints = append(state.Checkpoints, checkpoint)
	projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
		ID:           projectControlID("event", "checkpoint-raised"),
		Timestamp:    now,
		Actor:        "policy_engine",
		Action:       "checkpoint_raised",
		Detail:       "Generated archive_override checkpoint for explicit archive approval.",
		ProjectID:    task.ProjectID,
		WorkstreamID: task.WorkstreamID,
		TaskID:       task.ID,
		CheckpointID: checkpoint.ID,
	})
	return nil
}

func checkpointProjectID(state *projectControlState, taskID string) string {
	if state == nil {
		return ""
	}
	for _, task := range state.Tasks {
		if task.ID == taskID {
			return task.ProjectID
		}
	}
	return ""
}

func checkpointWorkstreamID(state *projectControlState, taskID string) string {
	if state == nil {
		return ""
	}
	for _, task := range state.Tasks {
		if task.ID == taskID {
			return task.WorkstreamID
		}
	}
	return ""
}

func validateProjectControlFinalAcceptanceApproval(state *projectControlState, checkpoint projectControlCheckpoint) error {
	if state == nil {
		return errors.New("missing project control state")
	}
	for _, task := range state.Tasks {
		if task.ID != checkpoint.TaskID {
			continue
		}
		if normalizeProjectControlTaskState(task.State) != "execution_complete" {
			return errors.New("final acceptance approval requires execution_complete task")
		}
		if missing := projectControlMissingCompletionEvidence(task, state.Artifacts, projectControlRunbookForTask(task), "", ""); len(missing) > 0 {
			return fmt.Errorf("completion rules not satisfied: missing %s", strings.Join(missing, ","))
		}
		return nil
	}
	return errors.New("checkpoint task not found")
}

func discoverProjectControlRuntimeRoot() string {
	cwd, err := os.Getwd()
	if err == nil && strings.TrimSpace(cwd) != "" {
		cmd := exec.Command("git", "rev-parse", "--show-toplevel")
		cmd.Dir = cwd
		if output, gitErr := cmd.Output(); gitErr == nil {
			root := strings.TrimSpace(string(output))
			if root != "" {
				return root
			}
		}
		return cwd
	}
	if homeDir, homeErr := os.UserHomeDir(); homeErr == nil && strings.TrimSpace(homeDir) != "" {
		return homeDir
	}
	return os.TempDir()
}

func newProjectControlStore(basePersistDir string) *projectControlStore {
	root := filepath.Join(basePersistDir, ".project-control", "users")
	workspaces := filepath.Join(basePersistDir, ".project-control", "workspaces")
	_ = os.MkdirAll(root, 0700)
	_ = os.MkdirAll(workspaces, 0700)
	return &projectControlStore{
		rootDir:          root,
		runtimeRootDir:   discoverProjectControlRuntimeRoot(),
		workspaceRootDir: workspaces,
	}
}

func (s *projectControlStore) runtimeWorkspaceDir() (string, error) {
	if s == nil {
		return "", errors.New("project control store is unavailable")
	}
	workspaceDir := strings.TrimSpace(s.runtimeRootDir)
	if workspaceDir == "" {
		return "", errors.New("runtime workspace is unavailable")
	}
	info, err := os.Stat(workspaceDir)
	if err != nil || !info.IsDir() {
		if err != nil {
			return "", fmt.Errorf("stat runtime workspace: %w", err)
		}
		return "", fmt.Errorf("runtime workspace is not a directory: %s", workspaceDir)
	}
	return workspaceDir, nil
}

func projectControlCloneWorkspaceSnapshot(sourceDir, snapshotDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), projectControlToolRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--quiet", "--shared", sourceDir, snapshotDir)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return errors.New("clone snapshot workspace timed out")
	}
	if err != nil {
		detail := compactProjectControlToolOutput(string(output), 512)
		if detail == "" {
			return fmt.Errorf("clone snapshot workspace: %w", err)
		}
		return fmt.Errorf("clone snapshot workspace: %s", detail)
	}
	return nil
}

func projectControlCopyWorkspaceFile(sourcePath, targetPath string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
		return fmt.Errorf("create snapshot parent: %w", err)
	}
	if err := os.RemoveAll(targetPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove snapshot target: %w", err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open workspace file: %w", err)
	}
	defer source.Close()
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return fmt.Errorf("create snapshot file: %w", err)
	}
	defer target.Close()
	if _, err := io.Copy(target, source); err != nil {
		return fmt.Errorf("copy workspace file: %w", err)
	}
	return nil
}

func projectControlSyncWorkspaceSnapshot(sourceDir, snapshotDir string) error {
	seen := map[string]bool{}
	if err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		if relPath == ".git" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		seen[relPath] = true
		targetPath := filepath.Join(snapshotDir, relPath)
		if info.IsDir() {
			if existing, err := os.Stat(targetPath); err == nil && !existing.IsDir() {
				if err := os.RemoveAll(targetPath); err != nil {
					return fmt.Errorf("replace snapshot directory: %w", err)
				}
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.MkdirAll(targetPath, info.Mode().Perm()); err != nil {
				return fmt.Errorf("create snapshot directory: %w", err)
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("read workspace symlink: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
				return fmt.Errorf("create snapshot symlink parent: %w", err)
			}
			if err := os.RemoveAll(targetPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("replace snapshot symlink: %w", err)
			}
			if err := os.Symlink(linkTarget, targetPath); err != nil {
				return fmt.Errorf("create snapshot symlink: %w", err)
			}
			return nil
		}
		return projectControlCopyWorkspaceFile(path, targetPath, info.Mode())
	}); err != nil {
		return err
	}
	stalePaths := []string{}
	if err := filepath.Walk(snapshotDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, err := filepath.Rel(snapshotDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		if relPath == ".git" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if seen[relPath] {
			return nil
		}
		stalePaths = append(stalePaths, path)
		if info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Slice(stalePaths, func(i, j int) bool {
		return len(stalePaths[i]) > len(stalePaths[j])
	})
	for _, stalePath := range stalePaths {
		if err := os.RemoveAll(stalePath); err != nil {
			return fmt.Errorf("remove stale snapshot path: %w", err)
		}
	}
	return nil
}

func (s *projectControlStore) createReadOnlySnapshotWorkspace(username string, task projectControlTask, phase projectControlRunbookPhase, attemptID string) (string, error) {
	sourceDir, err := s.runtimeWorkspaceDir()
	if err != nil {
		return "", err
	}
	workspaceRootDir := strings.TrimSpace(s.workspaceRootDir)
	if workspaceRootDir == "" {
		return "", errors.New("workspace root is unavailable")
	}
	snapshotParent := filepath.Join(workspaceRootDir, slugifyProjectControl(username), slugifyProjectControl(task.ID), slugifyProjectControl(phase.ID))
	if err := os.MkdirAll(snapshotParent, 0700); err != nil {
		return "", fmt.Errorf("create snapshot workspace parent: %w", err)
	}
	snapshotDir := filepath.Join(snapshotParent, slugifyProjectControl(attemptID))
	if info, statErr := os.Stat(snapshotDir); statErr == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("snapshot workspace is not a directory: %s", snapshotDir)
		}
		if _, gitErr := os.Stat(filepath.Join(snapshotDir, ".git")); gitErr == nil {
			if err := projectControlSyncWorkspaceSnapshot(sourceDir, snapshotDir); err != nil {
				return "", err
			}
			return snapshotDir, nil
		}
		if err := os.RemoveAll(snapshotDir); err != nil {
			return "", fmt.Errorf("reset snapshot workspace: %w", err)
		}
	} else if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("stat snapshot workspace: %w", statErr)
	}
	if err := projectControlCloneWorkspaceSnapshot(sourceDir, snapshotDir); err != nil {
		return "", err
	}
	if err := projectControlSyncWorkspaceSnapshot(sourceDir, snapshotDir); err != nil {
		_ = os.RemoveAll(snapshotDir)
		return "", err
	}
	return snapshotDir, nil
}

func (s *projectControlStore) prepareWorkspaceDirForPhase(username string, task projectControlTask, phase projectControlRunbookPhase, attemptID string) (string, error) {
	switch projectControlPhaseWorkspaceKind(phase) {
	case projectControlWorkspaceReadOnlySnapshot:
		return s.createReadOnlySnapshotWorkspace(username, task, phase, attemptID)
	default:
		return s.runtimeWorkspaceDir()
	}
}

func (s *projectControlStore) workspaceDirForRef(ref string) string {
	workspaceKind, workspaceDir := projectControlParseWorkspaceRef(ref)
	if workspaceDir != "" {
		info, err := os.Stat(workspaceDir)
		if err != nil || !info.IsDir() {
			return ""
		}
		return workspaceDir
	}
	if workspaceKind == "" {
		return ""
	}
	workspaceDir, err := s.runtimeWorkspaceDir()
	if err != nil {
		return ""
	}
	return workspaceDir
}

func (s *projectControlStore) allowsTerminalAttach(username, terminalID string) bool {
	if s == nil {
		return true
	}
	terminalID = strings.TrimSpace(terminalID)
	if terminalID == "" {
		return false
	}
	state, err := s.loadOrSeed(username)
	if err != nil {
		return true
	}
	sessionID := projectControlSessionIDForTerminal(terminalID)
	for _, attempt := range state.PhaseAttempts {
		if projectControlPhaseAttemptSessionID(attempt) != sessionID {
			continue
		}
		for _, task := range state.Tasks {
			if task.ID != attempt.TaskID {
				continue
			}
			phase, ok := findProjectControlRunbookPhase(projectControlRunbookForTask(task), attempt.PhaseID)
			if !ok {
				return false
			}
			return projectControlPhaseAllowsTerminalAttach(phase)
		}
		return false
	}
	return true
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
		project := projectControlProject{
			ID:          projectControlID("project", name),
			Key:         key,
			Name:        name,
			Description: strings.TrimSpace(req.Description),
			Status:      "active",
			CurrentGoal: strings.TrimSpace(req.CurrentGoal),
			RowVersion:  1,
		}
		state.Projects = append(state.Projects, project)
		state.ActiveProjectID = project.ID
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
			ID:        projectControlID("event", "project-created"),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Actor:     "human",
			Action:    "project_created",
			Detail:    "Created project " + project.Name + ".",
			ProjectID: project.ID,
		})
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
		workstream := projectControlWorkstream{
			ID:           projectControlID("workstream", title),
			ProjectID:    projectID,
			Title:        title,
			Description:  strings.TrimSpace(req.Description),
			Priority:     normalizeProjectControlPriority(req.Priority),
			Status:       "planned",
			ScopeSummary: strings.TrimSpace(req.ScopeSummary),
			RowVersion:   1,
		}
		state.Workstreams = append(state.Workstreams, workstream)
		state.ActiveProjectID = projectID
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
			ID:           projectControlID("event", "workstream-created"),
			Timestamp:    time.Now().UTC().Format(time.RFC3339),
			Actor:        "human",
			Action:       "workstream_created",
			Detail:       "Created workstream " + workstream.Title + ".",
			ProjectID:    workstream.ProjectID,
			WorkstreamID: workstream.ID,
		})
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
		skill, runbook, err := validateProjectControlSkillRunbookSelection(req.SelectedSkill, req.RunbookID)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		task := projectControlTask{
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
			SelectedSkill:    skill.ID,
			RunbookID:        runbook.ID,
			CurrentPhase:     "plan",
			RunbookState:     "not_started",
			MissingEvidence:  []string{},
			RecentSummary:    "Task created from the Project Panel and waiting to be scheduled.",
			NextStep:         "Open a session or connect a runtime-backed workflow to start execution.",
			FilesChanged:     []string{},
			DiffSummary:      "",
			SessionIDs:       []string{},
			Timeline:         []projectControlEvent{},
			Evidence:         []projectControlEvidence{},
			Audit:            []projectControlAuditItem{},
			RowVersion:       1,
		}
		refreshProjectControlTaskRunbookFields(&task, state.Artifacts)
		state.Tasks = append(state.Tasks, task)
		state.ActiveProjectID = projectID
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
			ID:           projectControlID("event", "task-created"),
			Timestamp:    now,
			Actor:        "human",
			Action:       "task_created",
			Detail:       "Created task " + task.Title + ".",
			ProjectID:    task.ProjectID,
			WorkstreamID: task.WorkstreamID,
			TaskID:       task.ID,
		})
		return nil
	})
	if err != nil {
		return projectControlSnapshot{}, err
	}
	return buildProjectControlSnapshot(state, username, terminals), nil
}

func (s *projectControlStore) updateWorkstream(username, workstreamID string, req projectControlWorkstreamUpdateRequest, terminals *terminal.Manager) (projectControlSnapshot, error) {
	if req.ExpectedRowVersion < 1 {
		return projectControlSnapshot{}, errors.New("expectedRowVersion must be provided")
	}
	state, err := s.withStateLocked(username, func(state *projectControlState) error {
		for i, workstream := range state.Workstreams {
			if workstream.ID != workstreamID {
				continue
			}
			if req.ExpectedRowVersion != workstream.RowVersion {
				return projectControlRowVersionConflict("workstream", req.ExpectedRowVersion, workstream.RowVersion)
			}
			originalStatus := workstream.Status
			if err := applyProjectControlWorkstreamAction(&workstream, &req); err != nil {
				return err
			}
			now := time.Now().UTC().Format(time.RFC3339)
			if value := strings.TrimSpace(req.Title); value != "" {
				workstream.Title = value
			}
			if value := strings.TrimSpace(req.Description); value != "" {
				workstream.Description = value
			}
			if value := strings.TrimSpace(req.ScopeSummary); value != "" {
				workstream.ScopeSummary = value
			}
			if value := strings.TrimSpace(req.Priority); value != "" {
				workstream.Priority = normalizeProjectControlPriority(value)
			}
			if value := strings.TrimSpace(req.Status); value != "" && value != workstream.Status {
				candidate := normalizeProjectControlWorkstreamStatus(value)
				if !isAllowedProjectControlWorkstreamTransition(originalStatus, candidate) {
					return fmt.Errorf("illegal workstream transition: %s -> %s", originalStatus, candidate)
				}
				from := workstream.Status
				workstream.Status = candidate
				projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
					ID:           projectControlID("event", "workstream-state-changed"),
					Timestamp:    now,
					Actor:        "human",
					Action:       "workstream_state_changed",
					Detail:       "Workstream state changed from " + from + " to " + workstream.Status + " via " + strings.TrimSpace(strings.ToLower(req.Action)) + ".",
					ProjectID:    workstream.ProjectID,
					WorkstreamID: workstream.ID,
				})
			}
			workstream.RowVersion += 1
			state.Workstreams[i] = workstream
			projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
				ID:           projectControlID("event", "workstream-updated"),
				Timestamp:    now,
				Actor:        "human",
				Action:       "workstream_updated",
				Detail:       "Updated workstream " + workstream.Title + ".",
				ProjectID:    workstream.ProjectID,
				WorkstreamID: workstream.ID,
			})
			return nil
		}
		return errors.New("workstream not found")
	})
	if err != nil {
		return projectControlSnapshot{}, err
	}
	return buildProjectControlSnapshot(state, username, terminals), nil
}

func (s *projectControlStore) updateTask(username, taskID string, req projectControlTaskUpdateRequest, terminals *terminal.Manager) (projectControlSnapshot, error) {
	agentBypass := req.ExpectedRowVersion == -1
	if !agentBypass && req.ExpectedRowVersion < 1 {
		return projectControlSnapshot{}, errors.New("expectedRowVersion must be provided")
	}
	toolRunID := ""
	_, err := s.withStateLocked(username, func(state *projectControlState) error {
		for i, task := range state.Tasks {
			if task.ID != taskID {
				continue
			}
			if !agentBypass && req.ExpectedRowVersion != task.RowVersion {
				return projectControlRowVersionConflict("task", req.ExpectedRowVersion, task.RowVersion)
			}
			if agentBypass {
				req.ExpectedRowVersion = task.RowVersion
			}
			originalState := task.State
			originalAcceptance := task.AcceptanceStatus
			action := strings.TrimSpace(strings.ToLower(req.Action))
			if isProjectControlPhaseAction(action) && (strings.TrimSpace(req.State) != "" || strings.TrimSpace(req.AcceptanceStatus) != "") {
				return errors.New("phase actions cannot include direct state or acceptanceStatus")
			}
			if err := applyProjectControlTaskAction(&task, &req); err != nil {
				return err
			}
			now := time.Now().UTC().Format(time.RFC3339)
			switch action {
			case "start_phase":
				if err := startProjectControlTaskPhase(state, &task, req.PhaseID, now, username, func(task projectControlTask, phase projectControlRunbookPhase, attemptID string) (string, error) {
					return s.prepareWorkspaceDirForPhase(username, task, phase, attemptID)
				}, terminals); err != nil {
					return err
				}
			case "complete_phase":
				if err := completeProjectControlTaskPhase(state, &task, req, now); err != nil {
					return err
				}
			case "fail_phase":
				if err := failProjectControlTaskPhase(state, &task, req.PhaseID, req.FailureReason, now); err != nil {
					return err
				}
			case "run_tool":
				startedToolRunID, err := startProjectControlTaskPhaseTool(state, &task, req, now)
				if err != nil {
					return err
				}
				toolRunID = startedToolRunID
			case "start_execution":
				task.State = "running"
				task.AcceptanceStatus = "not_ready"
				firstPhase := normalizeProjectControlPhaseID(task.CurrentPhase)
				if firstPhase == "" || firstPhase == "ready_for_acceptance" {
					runbook := projectControlRunbookForTask(task)
					if len(runbook.Phases) > 0 {
						firstPhase = normalizeProjectControlPhaseID(runbook.Phases[0].ID)
					}
				}
				if firstPhase != "" && firstPhase != "ready_for_acceptance" {
					if err := startProjectControlTaskPhase(state, &task, firstPhase, now, username, func(t projectControlTask, p projectControlRunbookPhase, attemptID string) (string, error) {
						return s.prepareWorkspaceDirForPhase(username, t, p, attemptID)
					}, terminals); err != nil {
						return err
					}
					tools := projectControlToolsForState(state)
					runbook := projectControlRunbookForTask(task)
					if phase, ok := findProjectControlRunbookPhase(runbook, firstPhase); ok {
						if toolDef, hasMatch := projectControlToolForPhase(tools, phase); hasMatch {
							startedID, err := startProjectControlTaskPhaseTool(state, &task, projectControlTaskUpdateRequest{
								PhaseID: firstPhase,
								ToolID:  toolDef.ID,
							}, now)
							if err == nil {
								toolRunID = startedID
							}
						}
					}
				}
				projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
					ID:           projectControlID("event", "execution-started"),
					Timestamp:    now,
					Actor:        "human",
					Action:       "execution_started",
					Detail:       "Started runbook execution.",
					ProjectID:    task.ProjectID,
					WorkstreamID: task.WorkstreamID,
					TaskID:       task.ID,
				})
			}
			if value := strings.TrimSpace(req.Title); value != "" {
				task.Title = value
			}
			if value := strings.TrimSpace(req.Goal); value != "" {
				task.Goal = value
			}
			if value := strings.TrimSpace(req.Priority); value != "" {
				task.Priority = normalizeProjectControlPriority(value)
			}
			if value := strings.TrimSpace(req.RiskLevel); value != "" {
				task.RiskLevel = normalizeProjectControlRisk(value)
			}
			if value := strings.TrimSpace(req.State); value != "" && value != task.State {
				candidate := normalizeProjectControlTaskState(value)
				if !isAllowedProjectControlTaskTransition(originalState, candidate) {
					return fmt.Errorf("illegal task transition: %s -> %s", originalState, candidate)
				}
				if candidate == "archived" && task.AcceptanceStatus != "accepted" {
					return errors.New("archiving requires accepted task or explicit archive override decision")
				}
				if candidate != "archived" || task.AcceptanceStatus == "accepted" {
					task.ArchiveDecisionID = ""
				}
				from := task.State
				task.State = candidate
				projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
					ID:           projectControlID("event", "task-state-changed"),
					Timestamp:    now,
					Actor:        "human",
					Action:       "task_state_changed",
					Detail:       "Task state changed from " + from + " to " + task.State + " via " + strings.TrimSpace(strings.ToLower(req.Action)) + ".",
					ProjectID:    task.ProjectID,
					WorkstreamID: task.WorkstreamID,
					TaskID:       task.ID,
				})
			}
			if value := strings.TrimSpace(req.AcceptanceStatus); value != "" && value != task.AcceptanceStatus {
				candidate := normalizeProjectControlAcceptanceStatus(value)
				if !(strings.TrimSpace(strings.ToLower(req.Action)) == "unarchive" && originalAcceptance == "accepted" && candidate == "not_ready") {
					if err := validateProjectControlAcceptanceTransition(originalAcceptance, candidate, task.State, false); err != nil {
						return err
					}
				}
				if candidate == "ready_for_acceptance" || candidate == "under_human_review" {
					if missing := projectControlMissingCompletionEvidence(task, state.Artifacts, projectControlRunbookForTask(task), "", ""); len(missing) > 0 {
						return fmt.Errorf("completion rules not satisfied: missing %s", strings.Join(missing, ","))
					}
				}
				from := task.AcceptanceStatus
				task.AcceptanceStatus = candidate
				if candidate != "accepted" && candidate != "rejected" {
					if !(strings.TrimSpace(strings.ToLower(req.Action)) == "unarchive" && originalAcceptance == "accepted") {
						task.AcceptanceDecisionID = ""
					}
				}
				task.ArchiveDecisionID = ""
				projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
					ID:           projectControlID("event", "acceptance-state-changed"),
					Timestamp:    now,
					Actor:        "human",
					Action:       "acceptance_state_changed",
					Detail:       "Acceptance status changed from " + from + " to " + task.AcceptanceStatus + " via " + strings.TrimSpace(strings.ToLower(req.Action)) + ".",
					ProjectID:    task.ProjectID,
					WorkstreamID: task.WorkstreamID,
					TaskID:       task.ID,
				})
			}
			if strings.TrimSpace(strings.ToLower(req.Action)) == "request_archive_override" {
				if originalState != "execution_complete" {
					return errors.New("archive override requires execution_complete task")
				}
				if originalAcceptance != "not_ready" {
					return errors.New("archive override requires not_ready acceptance status")
				}
				if err := requestProjectControlArchiveOverrideCheckpoint(state, task, now); err != nil {
					return err
				}
				task.RecentSummary = "Archive override requested; waiting for explicit human approval before archiving unaccepted work."
				task.NextStep = "Wait for archive override approval or continue toward final acceptance instead."
			}
			task.RowVersion += 1
			syncProjectControlAcceptanceCheckpoint(state, task, now)
			syncProjectControlArchiveOverrideCheckpoint(state, task, now)
			state.Tasks[i] = task
			projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
				ID:           projectControlID("event", "task-updated"),
				Timestamp:    now,
				Actor:        "human",
				Action:       "task_updated",
				Detail:       "Updated task " + task.Title + ".",
				ProjectID:    task.ProjectID,
				WorkstreamID: task.WorkstreamID,
				TaskID:       task.ID,
			})
			return nil
		}
		return errors.New("task not found")
	})
	if err != nil {
		return projectControlSnapshot{}, err
	}
	if toolRunID != "" {
		projectControlRunToolAsync(func() {
			_ = s.completeProjectControlToolRun(username, toolRunID)
		})
	}
	return s.snapshotForUser(username, terminals)
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
		if checkpoint.Status != "pending" {
			return errors.New("checkpoint is not pending")
		}
		allowed := false
		for _, allowedAction := range checkpoint.AllowedActions {
			if allowedAction == action {
				allowed = true
				break
			}
		}
		if !allowed {
			return errors.New("action not allowed for checkpoint")
		}
		now := time.Now().UTC().Format(time.RFC3339)
		decisionAction := "checkpoint_decided"
		isFinalAcceptance := checkpoint.Kind == "final_acceptance"
		isArchiveOverride := checkpoint.Kind == "archive_override"
		if isArchiveOverride && action == "reroute" {
			return errors.New("reroute not allowed for archive override")
		}
		if isFinalAcceptance && action == "approve" {
			if err := validateProjectControlFinalAcceptanceApproval(state, checkpoint); err != nil {
				return err
			}
		}
		switch action {
		case "approve":
			decisionAction = "checkpoint_approved"
			if isFinalAcceptance {
				decisionAction = "final_acceptance_approved"
			}
			if isArchiveOverride {
				decisionAction = "archive_override_approved"
			}
			checkpoint.Status = "approved"
			checkpoint.AllowedActions = []string{}
			if isArchiveOverride {
				checkpoint.DecisionSummary = "Archive override approved by human operator"
			} else {
				checkpoint.DecisionSummary = "Accepted by human operator"
			}
		case "reject":
			decisionAction = "checkpoint_rejected"
			if isFinalAcceptance {
				decisionAction = "final_acceptance_rejected"
			}
			if isArchiveOverride {
				decisionAction = "archive_override_rejected"
			}
			checkpoint.Status = "rejected"
			checkpoint.AllowedActions = []string{}
			if isArchiveOverride {
				checkpoint.DecisionSummary = "Archive override rejected by human operator"
			} else {
				checkpoint.DecisionSummary = "Rejected by human operator"
			}
		case "reroute":
			decisionAction = "checkpoint_rerouted"
			checkpoint.Status = "rerouted"
			checkpoint.AllowedActions = []string{}
			checkpoint.DecisionSummary = "Rerouted by human operator"
		}
		decision := projectControlDecision{
			ID:           projectControlID("decision", decisionAction),
			DecisionType: decisionAction,
			Actor:        "human",
			Timestamp:    now,
			Summary:      checkpoint.DecisionSummary,
			ProjectID:    checkpointProjectID(state, checkpoint.TaskID),
			WorkstreamID: checkpointWorkstreamID(state, checkpoint.TaskID),
			TaskID:       checkpoint.TaskID,
			CheckpointID: checkpoint.ID,
		}
		state.Decisions = append(state.Decisions, decision)
		checkpoint.ResolvedByDecisionID = decision.ID
		checkpoint.RowVersion += 1
		state.Checkpoints[idx] = checkpoint
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
			ID:           projectControlID("event", "checkpoint-resolved"),
			Timestamp:    now,
			Actor:        "human",
			Action:       "checkpoint_resolved",
			Detail:       "Resolved checkpoint " + checkpoint.ID + " as " + checkpoint.Status + ".",
			ProjectID:    checkpointProjectID(state, checkpoint.TaskID),
			WorkstreamID: checkpointWorkstreamID(state, checkpoint.TaskID),
			TaskID:       checkpoint.TaskID,
			CheckpointID: checkpoint.ID,
		})
		for index, task := range state.Tasks {
			if task.ID != checkpoint.TaskID {
				continue
			}
			switch action {
			case "approve":
				if isArchiveOverride {
					from := task.State
					task.State = "archived"
					task.AcceptanceStatus = "not_ready"
					task.AcceptanceDecisionID = ""
					task.ArchiveDecisionID = decision.ID
					task.RecentSummary = "Task archived via explicit archive override decision after approvals inbox review."
					task.NextStep = "Review archive override rationale if this work needs to be reopened later."
					projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
						ID:           projectControlID("event", "task-state-changed"),
						Timestamp:    now,
						Actor:        "human",
						Action:       "task_state_changed",
						Detail:       "Task state changed from " + from + " to archived via approve.",
						ProjectID:    task.ProjectID,
						WorkstreamID: task.WorkstreamID,
						TaskID:       task.ID,
					})
				} else {
					task.AcceptanceStatus = "accepted"
					task.AcceptanceDecisionID = decision.ID
					task.RecentSummary = "Task accepted by human operator; execution remains visible for audit."
					task.NextStep = "Archive or hide accepted work with board filters."
				}
			case "reject":
				if isArchiveOverride {
					task.ArchiveDecisionID = decision.ID
					task.RecentSummary = "Archive override rejected; task remains execution_complete until accepted or reopened."
					task.NextStep = "Either reopen for more work or send through final acceptance instead of archiving."
				} else {
					task.State = "running"
					task.AcceptanceStatus = "rejected"
					task.AcceptanceDecisionID = decision.ID
					task.RecentSummary = "Task rejected during final acceptance and sent back for revision."
					task.NextStep = "Revise the work, regenerate evidence, and resubmit for acceptance."
				}
			case "reroute":
				task.State = "blocked"
				task.AcceptanceStatus = "not_ready"
				task.AcceptanceDecisionID = ""
				task.RecentSummary = "Task rerouted back into planning."
				task.NextStep = "Clarify direction, then reopen execution with updated scope."
			}
			task.RowVersion += 1
			syncProjectControlAcceptanceCheckpoint(state, task, now)
			syncProjectControlArchiveOverrideCheckpoint(state, task, now)
			state.Tasks[index] = task
			projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
				ID:           projectControlID("event", "decision-made"),
				Timestamp:    now,
				Actor:        "human",
				Action:       "decision_made",
				Detail:       "Recorded decision " + decisionAction + " for checkpoint " + checkpoint.ID + ".",
				ProjectID:    task.ProjectID,
				WorkstreamID: task.WorkstreamID,
				TaskID:       task.ID,
				CheckpointID: checkpoint.ID,
			})
			projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
				ID:           projectControlID("event", decisionAction),
				Timestamp:    now,
				Actor:        "human",
				Action:       decisionAction,
				Detail:       checkpoint.DecisionSummary,
				ProjectID:    task.ProjectID,
				WorkstreamID: task.WorkstreamID,
				TaskID:       task.ID,
				CheckpointID: checkpoint.ID,
			})
			break
		}
		return nil
	})
	if err != nil {
		return projectControlSnapshot{}, err
	}
	return buildProjectControlSnapshot(state, username, terminals), nil
}

func (s *projectControlStore) eventsForUser(username string, values url.Values) (projectControlEventsResponse, error) {
	// loadOrSeed returns a value copy; the lock is released after load but the
	// returned data is safe to read without further synchronisation.
	state, err := s.loadOrSeed(username)
	if err != nil {
		return projectControlEventsResponse{}, err
	}
	events := cloneProjectControlRecordedEvents(state.Events)
	events = append(events, projectControlMemoryEvents(state.Memories)...)
	projectID := strings.TrimSpace(values.Get("projectId"))
	taskID := strings.TrimSpace(values.Get("taskId"))
	checkpointID := strings.TrimSpace(values.Get("checkpointId"))
	cursor := strings.TrimSpace(values.Get("cursor"))
	limit := 0
	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		parsed, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsed < 0 {
			return projectControlEventsResponse{}, errors.New("invalid limit")
		}
		limit = parsed
	}
	filtered := make([]projectControlRecordedEvent, 0, len(events))
	for _, event := range events {
		if projectID != "" && event.ProjectID != projectID {
			continue
		}
		if taskID != "" && event.TaskID != taskID {
			continue
		}
		if checkpointID != "" && event.CheckpointID != checkpointID {
			continue
		}
		filtered = append(filtered, event)
	}
	if len(filtered) > 1 {
		sort.Slice(filtered, func(i, j int) bool {
			if filtered[i].Timestamp == filtered[j].Timestamp {
				return filtered[i].ID > filtered[j].ID
			}
			return filtered[i].Timestamp > filtered[j].Timestamp
		})
	}
	if cursor != "" {
		cursorTs, cursorID, decodeErr := decodeProjectControlCursor(cursor)
		if decodeErr != nil {
			return projectControlEventsResponse{}, errors.New("invalid cursor")
		}
		tmp := filtered[:0]
		for _, event := range filtered {
			if event.Timestamp < cursorTs || (event.Timestamp == cursorTs && event.ID < cursorID) {
				tmp = append(tmp, event)
			}
		}
		filtered = tmp
	}
	resp := projectControlEventsResponse{Events: filtered}
	if limit > 0 && len(filtered) > limit {
		resp.Events = filtered[:limit]
		last := resp.Events[len(resp.Events)-1]
		resp.NextCursor = encodeProjectControlCursor(last.Timestamp, last.ID)
	}
	return resp, nil
}

func (s *projectControlStore) replayForTask(username, taskID string, terminals *terminal.Manager) (projectControlReplayResponse, error) {
	// Capture synthetic terminal-based events first, then load persisted state
	// immediately after, so both snapshots are taken as close together as possible.
	syntheticEvents := syntheticReplayEventsForTask(taskID, username, terminals)
	state, err := s.loadOrSeed(username)
	if err != nil {
		return projectControlReplayResponse{}, err
	}
	var task *projectControlTask
	for i := range state.Tasks {
		if state.Tasks[i].ID == taskID {
			task = &state.Tasks[i]
			break
		}
	}
	if task == nil {
		return projectControlReplayResponse{}, errors.New("task not found")
	}
	steps := make([]projectControlRecordedEvent, 0)
	for _, event := range state.Events {
		if event.TaskID == taskID {
			steps = append(steps, event)
		}
	}
	for _, memory := range state.Memories {
		if strings.TrimSpace(memory.TaskID) != taskID {
			continue
		}
		if event, ok := projectControlMemoryEvent(memory); ok {
			steps = append(steps, event)
		}
	}
	steps = append(steps, syntheticEvents...)
	var acceptanceDecision *projectControlDecision
	if strings.TrimSpace(task.AcceptanceDecisionID) != "" {
		for i := range state.Decisions {
			if state.Decisions[i].ID == task.AcceptanceDecisionID {
				copyDecision := state.Decisions[i]
				acceptanceDecision = &copyDecision
				break
			}
		}
	}
	var archiveDecision *projectControlDecision
	if strings.TrimSpace(task.ArchiveDecisionID) != "" {
		for i := range state.Decisions {
			if state.Decisions[i].ID == task.ArchiveDecisionID {
				copyDecision := state.Decisions[i]
				archiveDecision = &copyDecision
				break
			}
		}
	}
	if len(steps) > 1 {
		sort.Slice(steps, func(i, j int) bool {
			if steps[i].Timestamp == steps[j].Timestamp {
				return steps[i].ID < steps[j].ID
			}
			return steps[i].Timestamp < steps[j].Timestamp
		})
	}
	sections := buildReplaySections(steps)
	transitions := buildReplayTransitions(task, steps)
	return projectControlReplayResponse{
		TaskID:             task.ID,
		ProjectID:          task.ProjectID,
		WorkstreamID:       task.WorkstreamID,
		Title:              task.Title,
		CurrentState:       task.State,
		AcceptanceState:    task.AcceptanceStatus,
		AcceptanceDecision: acceptanceDecision,
		ArchiveDecision:    archiveDecision,
		Steps:              steps,
		Sections:           sections,
		Transitions:        transitions,
	}, nil
}

func encodeProjectControlCursor(timestamp, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(timestamp + "\n" + id))
}

func syntheticReplayEventsForTask(taskID, username string, terminals *terminal.Manager) []projectControlRecordedEvent {
	if terminals == nil {
		return nil
	}
	sessions := terminals.ListSessions(username)
	if len(sessions) > 1 {
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].CreatedAt.Before(sessions[j].CreatedAt)
		})
	}
	indexByTask := map[string]int{
		projectControlTaskPanelID:     0,
		projectControlTaskAttachID:    1,
		projectControlTaskApprovalsID: 2,
	}
	mappedIndex, ok := indexByTask[taskID]
	if !ok || mappedIndex >= len(sessions) {
		return nil
	}
	session := sessions[mappedIndex]
	timestamp := session.CreatedAt.UTC().Format(time.RFC3339)
	return []projectControlRecordedEvent{{
		ID:        "synthetic-session-" + session.ID,
		Timestamp: timestamp,
		Actor:     "worker",
		Action:    "session_started",
		Detail:    "Live terminal session surfaced for replay.",
		TaskID:    taskID,
	}}
}

func replaySectionKindForAction(action string) string {
	switch {
	case strings.HasPrefix(action, "checkpoint_") || strings.HasPrefix(action, "final_acceptance_") || strings.HasPrefix(action, "archive_override_") || action == "decision_made":
		return "decision"
	case strings.Contains(action, "acceptance"):
		return "acceptance"
	case strings.Contains(action, "review"):
		return "review"
	case strings.Contains(action, "runtime") || strings.Contains(action, "session") || action == "queued":
		return "execution"
	default:
		return "execution"
	}
}

func replaySectionTitle(kind string) string {
	switch kind {
	case "decision":
		return "Decision"
	case "acceptance":
		return "Acceptance"
	case "review":
		return "Review"
	default:
		return "Execution"
	}
}

func buildReplaySections(steps []projectControlRecordedEvent) []projectControlReplaySection {
	sections := make([]projectControlReplaySection, 0)
	for _, step := range steps {
		kind := replaySectionKindForAction(step.Action)
		if len(sections) == 0 || sections[len(sections)-1].Kind != kind {
			sections = append(sections, projectControlReplaySection{Kind: kind, Title: replaySectionTitle(kind), Steps: []projectControlRecordedEvent{}})
		}
		sections[len(sections)-1].Steps = append(sections[len(sections)-1].Steps, step)
	}
	return sections
}

func buildReplayTransitions(task *projectControlTask, steps []projectControlRecordedEvent) []projectControlReplayTransition {
	if task == nil {
		return nil
	}
	transitions := []projectControlReplayTransition{{
		Type:   "task_state",
		From:   "unknown",
		To:     task.State,
		Reason: "Current persisted task state.",
	}}
	transitions = append(transitions, projectControlReplayTransition{
		Type:   "acceptance_state",
		From:   "unknown",
		To:     task.AcceptanceStatus,
		Reason: "Current persisted acceptance state.",
	})
	for _, step := range steps {
		switch step.Action {
		case "task_state_changed":
			from, to, ok := projectControlParseTransitionDetail(step.Detail)
			if ok {
				transitions = append(transitions, projectControlReplayTransition{
					Type:   "task_state",
					From:   from,
					To:     to,
					Reason: step.Detail,
				})
			}
		case "acceptance_state_changed":
			from, to, ok := projectControlParseTransitionDetail(step.Detail)
			if ok {
				transitions = append(transitions, projectControlReplayTransition{
					Type:   "acceptance_state",
					From:   from,
					To:     to,
					Reason: step.Detail,
				})
			}
		case "checkpoint_approved", "checkpoint_rejected", "checkpoint_rerouted", "final_acceptance_approved", "final_acceptance_rejected", "archive_override_approved", "archive_override_rejected":
			from := "checkpoint_pending"
			if strings.HasPrefix(step.Action, "archive_override_") {
				from = "task_decision_pending"
			}
			transitions = append(transitions, projectControlReplayTransition{
				Type:   "decision",
				From:   from,
				To:     step.Action,
				Reason: step.Detail,
			})
		case "decision_made":
			transitions = append(transitions, projectControlReplayTransition{
				Type:   "decision_recorded",
				From:   "pending_record",
				To:     "decision_made",
				Reason: step.Detail,
			})
		case "checkpoint_resolved":
			transitions = append(transitions, projectControlReplayTransition{
				Type:   "checkpoint_resolution",
				From:   "pending",
				To:     "resolved",
				Reason: step.Detail,
			})
		}
	}
	return transitions
}

func projectControlParseTransitionDetail(detail string) (string, string, bool) {
	detail = strings.TrimSpace(detail)
	start := strings.Index(detail, "from ")
	middle := strings.Index(detail, " to ")
	if start == -1 || middle == -1 || middle <= start+5 {
		return "", "", false
	}
	from := strings.TrimSpace(detail[start+5 : middle])
	rest := strings.TrimSpace(detail[middle+4:])
	to := rest
	if via := strings.Index(rest, " via "); via != -1 {
		to = strings.TrimSpace(rest[:via])
	}
	to = strings.TrimSuffix(to, ".")
	from = strings.TrimSuffix(from, ".")
	if from == "" || to == "" {
		return "", "", false
	}
	return from, to, true
}

func decodeProjectControlCursor(cursor string) (string, string, error) {
	payload, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(payload), "\n", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid cursor")
	}
	return parts[0], parts[1], nil
}

func projectControlStateRequiresCanonicalization(state projectControlState) bool {
	eventCounts := map[string]int{}
	artifactCounts := map[string]int{}
	phaseAttemptCounts := map[string]int{}
	toolRunCounts := map[string]int{}

	for _, task := range state.Tasks {
		if len(task.Timeline) > 0 || len(task.Evidence) > 0 || len(task.Audit) > 0 {
			return true
		}
	}
	for _, event := range state.Events {
		taskID := strings.TrimSpace(event.TaskID)
		if taskID == "" {
			continue
		}
		eventCounts[taskID] += 1
		if eventCounts[taskID] > projectControlRetainedTaskEvents {
			return true
		}
	}
	for _, artifact := range state.Artifacts {
		taskID := strings.TrimSpace(artifact.TaskID)
		if taskID == "" {
			continue
		}
		artifactCounts[taskID] += 1
		if artifactCounts[taskID] > projectControlRetainedTaskArtifacts {
			return true
		}
	}
	for _, attempt := range state.PhaseAttempts {
		taskID := strings.TrimSpace(attempt.TaskID)
		if taskID == "" {
			continue
		}
		phaseAttemptCounts[taskID] += 1
		if phaseAttemptCounts[taskID] > projectControlRetainedTaskPhaseAttempts {
			return true
		}
	}
	for _, run := range state.ToolRuns {
		taskID := strings.TrimSpace(run.TaskID)
		if taskID == "" {
			continue
		}
		toolRunCounts[taskID] += 1
		if toolRunCounts[taskID] > projectControlRetainedTaskToolRuns {
			return true
		}
	}
	return false
}

func projectControlTaskHistoryTimestamp(task projectControlTask, fallback string) string {
	best := strings.TrimSpace(fallback)
	for _, item := range task.Timeline {
		ts := strings.TrimSpace(item.Timestamp)
		if ts != "" && (best == "" || ts > best) {
			best = ts
		}
	}
	for _, item := range task.Audit {
		ts := strings.TrimSpace(item.Timestamp)
		if ts != "" && (best == "" || ts > best) {
			best = ts
		}
	}
	return best
}

func projectControlMigrateLegacyEvidenceToArtifacts(state *projectControlState) {
	if state == nil {
		return
	}

	existing := map[string]bool{}
	for _, artifact := range state.Artifacts {
		key := strings.TrimSpace(artifact.TaskID) + "\n" + strings.TrimSpace(artifact.Label) + "\n" + strings.TrimSpace(artifact.Value)
		existing[key] = true
	}

	fallback := strings.TrimSpace(state.UpdatedAt)
	if fallback == "" {
		fallback = time.Now().UTC().Format(time.RFC3339)
	}

	for _, task := range state.Tasks {
		createdAt := projectControlTaskHistoryTimestamp(task, fallback)
		for _, evidence := range task.Evidence {
			label := strings.TrimSpace(evidence.Label)
			value := strings.TrimSpace(evidence.Value)
			if label == "" && value == "" {
				continue
			}
			key := strings.TrimSpace(task.ID) + "\n" + label + "\n" + value
			if existing[key] {
				continue
			}
			state.Artifacts = append(state.Artifacts, projectControlArtifact{
				ID:        projectControlID("artifact", task.ID+"-evidence-note"),
				TaskID:    task.ID,
				Kind:      "evidence_note",
				Outcome:   "recorded",
				Label:     label,
				Value:     value,
				CreatedAt: createdAt,
			})
			existing[key] = true
		}
	}
}

func projectControlStripDerivedTaskFields(state *projectControlState) {
	if state == nil {
		return
	}
	for i := range state.Tasks {
		state.Tasks[i].Timeline = []projectControlEvent{}
		state.Tasks[i].Evidence = []projectControlEvidence{}
		state.Tasks[i].Audit = []projectControlAuditItem{}
	}
}

func projectControlMemoryWindowUpdate(memory *projectControlTaskMemory, timestamp string) {
	if memory == nil {
		return
	}
	timestamp = strings.TrimSpace(timestamp)
	if timestamp == "" {
		return
	}
	if memory.WindowStart == "" || timestamp < memory.WindowStart {
		memory.WindowStart = timestamp
	}
	if memory.WindowEnd == "" || timestamp > memory.WindowEnd {
		memory.WindowEnd = timestamp
	}
}

func projectControlMergeMemoryHighlights(existing []string, additions ...string) []string {
	merged := make([]string, 0, len(existing)+len(additions))
	seen := map[string]bool{}
	for _, item := range additions {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		merged = append(merged, item)
		seen[item] = true
	}
	for _, item := range existing {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		merged = append(merged, item)
		seen[item] = true
	}
	if len(merged) > projectControlRetainedMemoryHighlights {
		merged = merged[:projectControlRetainedMemoryHighlights]
	}
	return merged
}

func projectControlSummarizeCountMap(prefix string, counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	type countEntry struct {
		Key   string
		Count int
	}
	items := make([]countEntry, 0, len(counts))
	for key, count := range counts {
		key = strings.TrimSpace(key)
		if key == "" || count < 1 {
			continue
		}
		items = append(items, countEntry{Key: key, Count: count})
	}
	if len(items) == 0 {
		return ""
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	if len(items) > 4 {
		items = items[:4]
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s×%d", item.Key, item.Count))
	}
	return prefix + ": " + strings.Join(parts, ", ")
}

func projectControlFinalizeTaskMemory(memory *projectControlTaskMemory) {
	if memory == nil {
		return
	}
	parts := []string{}
	if memory.EventCount > 0 {
		parts = append(parts, fmt.Sprintf("%d events", memory.EventCount))
	}
	if memory.ArtifactCount > 0 {
		parts = append(parts, fmt.Sprintf("%d artifacts", memory.ArtifactCount))
	}
	if memory.PhaseAttemptCount > 0 {
		parts = append(parts, fmt.Sprintf("%d phase attempts", memory.PhaseAttemptCount))
	}
	if memory.ToolRunCount > 0 {
		parts = append(parts, fmt.Sprintf("%d tool runs", memory.ToolRunCount))
	}
	if len(parts) == 0 {
		memory.Summary = ""
		return
	}
	rangeLabel := ""
	switch {
	case memory.WindowStart != "" && memory.WindowEnd != "" && memory.WindowStart != memory.WindowEnd:
		rangeLabel = " between " + memory.WindowStart + " and " + memory.WindowEnd
	case memory.WindowEnd != "":
		rangeLabel = " through " + memory.WindowEnd
	}
	memory.Summary = "Compacted older task history" + rangeLabel + ": " + strings.Join(parts, ", ") + "."
}

func projectControlEnsureTaskMemory(memories map[string]*projectControlTaskMemory, task projectControlTask, now string) *projectControlTaskMemory {
	taskID := strings.TrimSpace(task.ID)
	if taskID == "" {
		return nil
	}
	if memories[taskID] == nil {
		memories[taskID] = &projectControlTaskMemory{
			TaskID:       taskID,
			ProjectID:    strings.TrimSpace(task.ProjectID),
			WorkstreamID: strings.TrimSpace(task.WorkstreamID),
		}
	}
	memory := memories[taskID]
	if memory.ProjectID == "" {
		memory.ProjectID = strings.TrimSpace(task.ProjectID)
	}
	if memory.WorkstreamID == "" {
		memory.WorkstreamID = strings.TrimSpace(task.WorkstreamID)
	}
	if strings.TrimSpace(now) != "" {
		memory.UpdatedAt = strings.TrimSpace(now)
	}
	return memory
}

func projectControlCompactEvents(state *projectControlState, memories map[string]*projectControlTaskMemory, now string) {
	if state == nil {
		return
	}
	taskByID := map[string]projectControlTask{}
	for _, task := range state.Tasks {
		taskByID[task.ID] = task
	}
	grouped := map[string][]projectControlRecordedEvent{}
	passthrough := []projectControlRecordedEvent{}
	for _, event := range state.Events {
		taskID := strings.TrimSpace(event.TaskID)
		if taskID == "" {
			passthrough = append(passthrough, event)
			continue
		}
		task, ok := taskByID[taskID]
		if !ok {
			passthrough = append(passthrough, event)
			continue
		}
		grouped[task.ID] = append(grouped[task.ID], event)
	}
	compacted := passthrough
	taskIDs := make([]string, 0, len(grouped))
	for taskID := range grouped {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	for _, taskID := range taskIDs {
		events := grouped[taskID]
		sort.Slice(events, func(i, j int) bool {
			if events[i].Timestamp == events[j].Timestamp {
				return events[i].ID < events[j].ID
			}
			return events[i].Timestamp < events[j].Timestamp
		})
		if len(events) > projectControlRetainedTaskEvents {
			pruned := events[:len(events)-projectControlRetainedTaskEvents]
			events = events[len(events)-projectControlRetainedTaskEvents:]
			task := taskByID[taskID]
			memory := projectControlEnsureTaskMemory(memories, task, now)
			counts := map[string]int{}
			for _, event := range pruned {
				memory.EventCount += 1
				projectControlMemoryWindowUpdate(memory, event.Timestamp)
				counts[strings.TrimSpace(event.Action)] += 1
			}
			memory.Highlights = projectControlMergeMemoryHighlights(memory.Highlights, projectControlSummarizeCountMap("events", counts))
		}
		compacted = append(compacted, events...)
	}
	state.Events = compacted
}

func projectControlCompactPhaseAttempts(state *projectControlState, memories map[string]*projectControlTaskMemory, now string) {
	if state == nil {
		return
	}
	taskByID := map[string]projectControlTask{}
	for _, task := range state.Tasks {
		taskByID[task.ID] = task
	}
	grouped := map[string][]projectControlPhaseAttempt{}
	passthrough := []projectControlPhaseAttempt{}
	for _, attempt := range state.PhaseAttempts {
		taskID := strings.TrimSpace(attempt.TaskID)
		if taskID == "" {
			passthrough = append(passthrough, attempt)
			continue
		}
		task, ok := taskByID[taskID]
		if !ok {
			passthrough = append(passthrough, attempt)
			continue
		}
		grouped[task.ID] = append(grouped[task.ID], attempt)
	}
	compacted := passthrough
	taskIDs := make([]string, 0, len(grouped))
	for taskID := range grouped {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	for _, taskID := range taskIDs {
		attempts := grouped[taskID]
		running := make([]projectControlPhaseAttempt, 0, len(attempts))
		closed := make([]projectControlPhaseAttempt, 0, len(attempts))
		for _, attempt := range attempts {
			if normalizeProjectControlPhaseAttemptStatus(attempt.Status) == "running" {
				running = append(running, attempt)
			} else {
				closed = append(closed, attempt)
			}
		}
		sort.Slice(closed, func(i, j int) bool {
			left := strings.TrimSpace(closed[i].CompletedAt)
			if left == "" {
				left = strings.TrimSpace(closed[i].StartedAt)
			}
			right := strings.TrimSpace(closed[j].CompletedAt)
			if right == "" {
				right = strings.TrimSpace(closed[j].StartedAt)
			}
			if left == right {
				return closed[i].ID < closed[j].ID
			}
			return left < right
		})
		if len(closed) > projectControlRetainedTaskPhaseAttempts {
			pruned := closed[:len(closed)-projectControlRetainedTaskPhaseAttempts]
			closed = closed[len(closed)-projectControlRetainedTaskPhaseAttempts:]
			task := taskByID[taskID]
			memory := projectControlEnsureTaskMemory(memories, task, now)
			counts := map[string]int{}
			for _, attempt := range pruned {
				memory.PhaseAttemptCount += 1
				if strings.TrimSpace(attempt.CompletedAt) != "" {
					projectControlMemoryWindowUpdate(memory, attempt.CompletedAt)
				} else {
					projectControlMemoryWindowUpdate(memory, attempt.StartedAt)
				}
				key := strings.TrimSpace(attempt.PhaseID)
				if status := strings.TrimSpace(attempt.Status); status != "" {
					key += " (" + status + ")"
				}
				counts[strings.TrimSpace(key)] += 1
			}
			memory.Highlights = projectControlMergeMemoryHighlights(memory.Highlights, projectControlSummarizeCountMap("phases", counts))
		}
		compacted = append(compacted, running...)
		compacted = append(compacted, closed...)
	}
	state.PhaseAttempts = compacted
}

func projectControlCompactToolRuns(state *projectControlState, memories map[string]*projectControlTaskMemory, now string) {
	if state == nil {
		return
	}
	taskByID := map[string]projectControlTask{}
	for _, task := range state.Tasks {
		taskByID[task.ID] = task
	}
	grouped := map[string][]projectControlToolRun{}
	passthrough := []projectControlToolRun{}
	for _, run := range state.ToolRuns {
		taskID := strings.TrimSpace(run.TaskID)
		if taskID == "" {
			passthrough = append(passthrough, run)
			continue
		}
		task, ok := taskByID[taskID]
		if !ok {
			passthrough = append(passthrough, run)
			continue
		}
		grouped[task.ID] = append(grouped[task.ID], run)
	}
	compacted := passthrough
	taskIDs := make([]string, 0, len(grouped))
	for taskID := range grouped {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	for _, taskID := range taskIDs {
		runs := grouped[taskID]
		running := make([]projectControlToolRun, 0, len(runs))
		closed := make([]projectControlToolRun, 0, len(runs))
		for _, run := range runs {
			if normalizeProjectControlToolRunStatus(run.Status) == "running" {
				running = append(running, run)
			} else {
				closed = append(closed, run)
			}
		}
		sort.Slice(closed, func(i, j int) bool {
			left := strings.TrimSpace(closed[i].CompletedAt)
			if left == "" {
				left = strings.TrimSpace(closed[i].StartedAt)
			}
			right := strings.TrimSpace(closed[j].CompletedAt)
			if right == "" {
				right = strings.TrimSpace(closed[j].StartedAt)
			}
			if left == right {
				return closed[i].ID < closed[j].ID
			}
			return left < right
		})
		if len(closed) > projectControlRetainedTaskToolRuns {
			pruned := closed[:len(closed)-projectControlRetainedTaskToolRuns]
			closed = closed[len(closed)-projectControlRetainedTaskToolRuns:]
			task := taskByID[taskID]
			memory := projectControlEnsureTaskMemory(memories, task, now)
			counts := map[string]int{}
			for _, run := range pruned {
				memory.ToolRunCount += 1
				if strings.TrimSpace(run.CompletedAt) != "" {
					projectControlMemoryWindowUpdate(memory, run.CompletedAt)
				} else {
					projectControlMemoryWindowUpdate(memory, run.StartedAt)
				}
				key := strings.TrimSpace(run.ToolID)
				if status := strings.TrimSpace(run.Status); status != "" {
					key += " (" + status + ")"
				}
				counts[strings.TrimSpace(key)] += 1
			}
			memory.Highlights = projectControlMergeMemoryHighlights(memory.Highlights, projectControlSummarizeCountMap("tools", counts))
		}
		compacted = append(compacted, running...)
		compacted = append(compacted, closed...)
	}
	state.ToolRuns = compacted
}

func projectControlProtectedArtifactIDs(artifacts []projectControlArtifact, phaseAttempts []projectControlPhaseAttempt, toolRuns []projectControlToolRun) map[string]bool {
	protected := map[string]bool{}
	for _, attempt := range phaseAttempts {
		for _, artifactID := range attempt.ArtifactIDs {
			artifactID = strings.TrimSpace(artifactID)
			if artifactID != "" {
				protected[artifactID] = true
			}
		}
	}
	for _, run := range toolRuns {
		artifactID := strings.TrimSpace(run.ArtifactID)
		if artifactID != "" {
			protected[artifactID] = true
		}
	}
	latestByTaskKind := map[string]projectControlArtifact{}
	for _, artifact := range artifacts {
		taskID := strings.TrimSpace(artifact.TaskID)
		if taskID == "" {
			continue
		}
		key := taskID + "\n" + normalizeProjectControlArtifactKind(artifact.Kind)
		current, ok := latestByTaskKind[key]
		if !ok || strings.TrimSpace(artifact.CreatedAt) > strings.TrimSpace(current.CreatedAt) || (strings.TrimSpace(artifact.CreatedAt) == strings.TrimSpace(current.CreatedAt) && artifact.ID > current.ID) {
			latestByTaskKind[key] = artifact
		}
	}
	for _, artifact := range latestByTaskKind {
		if strings.TrimSpace(artifact.ID) != "" {
			protected[artifact.ID] = true
		}
	}
	return protected
}

func projectControlCompactArtifacts(state *projectControlState, memories map[string]*projectControlTaskMemory, now string) {
	if state == nil {
		return
	}
	taskByID := map[string]projectControlTask{}
	for _, task := range state.Tasks {
		taskByID[task.ID] = task
	}
	protected := projectControlProtectedArtifactIDs(state.Artifacts, state.PhaseAttempts, state.ToolRuns)
	grouped := map[string][]projectControlArtifact{}
	passthrough := []projectControlArtifact{}
	for _, artifact := range state.Artifacts {
		taskID := strings.TrimSpace(artifact.TaskID)
		if taskID == "" {
			passthrough = append(passthrough, artifact)
			continue
		}
		task, ok := taskByID[taskID]
		if !ok {
			passthrough = append(passthrough, artifact)
			continue
		}
		grouped[task.ID] = append(grouped[task.ID], artifact)
	}
	compacted := passthrough
	taskIDs := make([]string, 0, len(grouped))
	for taskID := range grouped {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	for _, taskID := range taskIDs {
		artifacts := grouped[taskID]
		sort.Slice(artifacts, func(i, j int) bool {
			if artifacts[i].CreatedAt == artifacts[j].CreatedAt {
				return artifacts[i].ID < artifacts[j].ID
			}
			return artifacts[i].CreatedAt < artifacts[j].CreatedAt
		})
		prunableIndexes := []int{}
		for index, artifact := range artifacts {
			if !protected[strings.TrimSpace(artifact.ID)] {
				prunableIndexes = append(prunableIndexes, index)
			}
		}
		prunedByIndex := map[int]bool{}
		if len(artifacts) > projectControlRetainedTaskArtifacts {
			toPrune := len(artifacts) - projectControlRetainedTaskArtifacts
			if toPrune > len(prunableIndexes) {
				toPrune = len(prunableIndexes)
			}
			if toPrune > 0 {
				task := taskByID[taskID]
				memory := projectControlEnsureTaskMemory(memories, task, now)
				counts := map[string]int{}
				for _, index := range prunableIndexes[:toPrune] {
					artifact := artifacts[index]
					prunedByIndex[index] = true
					memory.ArtifactCount += 1
					projectControlMemoryWindowUpdate(memory, artifact.CreatedAt)
					key := strings.TrimSpace(artifact.Kind)
					if outcome := strings.TrimSpace(artifact.Outcome); outcome != "" {
						key += " (" + outcome + ")"
					}
					counts[strings.TrimSpace(key)] += 1
				}
				memory.Highlights = projectControlMergeMemoryHighlights(memory.Highlights, projectControlSummarizeCountMap("artifacts", counts))
			}
		}
		for index, artifact := range artifacts {
			if prunedByIndex[index] {
				continue
			}
			compacted = append(compacted, artifact)
		}
	}
	state.Artifacts = compacted
}

func projectControlCompactState(state *projectControlState) {
	if state == nil {
		return
	}
	projectControlMigrateLegacyEvidenceToArtifacts(state)
	memories := map[string]*projectControlTaskMemory{}
	for i := range state.Memories {
		memory := state.Memories[i]
		taskID := strings.TrimSpace(memory.TaskID)
		if taskID == "" {
			continue
		}
		memory.TaskID = taskID
		if memory.Highlights == nil {
			memory.Highlights = []string{}
		}
		memories[taskID] = &memory
	}
	now := time.Now().UTC().Format(time.RFC3339)
	projectControlCompactEvents(state, memories, now)
	projectControlCompactPhaseAttempts(state, memories, now)
	projectControlCompactToolRuns(state, memories, now)
	projectControlCompactArtifacts(state, memories, now)
	state.Memories = make([]projectControlTaskMemory, 0, len(memories))
	for _, task := range state.Tasks {
		memory := memories[strings.TrimSpace(task.ID)]
		if memory == nil {
			continue
		}
		projectControlFinalizeTaskMemory(memory)
		if strings.TrimSpace(memory.Summary) == "" {
			continue
		}
		memory.Highlights = projectControlMergeMemoryHighlights(nil, memory.Highlights...)
		state.Memories = append(state.Memories, *memory)
	}
	sort.Slice(state.Memories, func(i, j int) bool {
		if state.Memories[i].UpdatedAt == state.Memories[j].UpdatedAt {
			return state.Memories[i].TaskID < state.Memories[j].TaskID
		}
		return state.Memories[i].UpdatedAt > state.Memories[j].UpdatedAt
	})
	projectControlStripDerivedTaskFields(state)
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

func projectControlReadStateFile(path string) (projectControlState, bool, error) {
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return projectControlState{}, false, nil
	}
	if err != nil {
		return projectControlState{}, false, err
	}
	var state projectControlState
	if err := json.Unmarshal(payload, &state); err != nil {
		return projectControlState{}, false, err
	}
	return state, true, nil
}

func projectControlParseStateUpdatedAt(state projectControlState) (time.Time, bool) {
	value := strings.TrimSpace(state.UpdatedAt)
	if value == "" {
		return time.Time{}, false
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts, true
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts, true
	}
	return time.Time{}, false
}

func projectControlPreferState(candidate, current projectControlState) bool {
	candidateTime, candidateOK := projectControlParseStateUpdatedAt(candidate)
	currentTime, currentOK := projectControlParseStateUpdatedAt(current)
	switch {
	case candidateOK && currentOK:
		if candidateTime.Equal(currentTime) {
			return true
		}
		return candidateTime.After(currentTime)
	case candidateOK && !currentOK:
		return true
	case !candidateOK && currentOK:
		return false
	default:
		return true
	}
}

func projectControlWriteFileAtomically(path string, payload []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func (s *projectControlStore) loadLocked(username string) (projectControlState, bool, error) {
	path := s.pathFor(username)
	walPath := s.walPathFor(username)

	state, stateExists, err := projectControlReadStateFile(path)
	if err != nil {
		return projectControlState{}, false, err
	}
	walState, walExists, err := projectControlReadStateFile(walPath)
	if err != nil {
		return projectControlState{}, false, err
	}
	if !stateExists && !walExists {
		legacyState, legacyExists, legacyErr := projectControlReadStateFile(s.legacyPathFor(username))
		if legacyErr != nil {
			return projectControlState{}, false, legacyErr
		}
		if !legacyExists {
			return projectControlState{}, false, nil
		}
		needsSave := projectControlStateRequiresCanonicalization(legacyState)
		projectControlNormalizeState(&legacyState)
		if projectControlRecoverInterruptedToolRuns(&legacyState, time.Now().UTC()) {
			legacyState.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if !needsSave {
			needsSave = true
		}
		if saveErr := s.saveLocked(username, legacyState); saveErr != nil {
			return projectControlState{}, false, saveErr
		}
		return legacyState, true, nil
	}

	stateSourceIsWAL := false
	clearWAL := false
	switch {
	case walExists && (!stateExists || projectControlPreferState(walState, state)):
		state = walState
		stateExists = true
		stateSourceIsWAL = true
	case walExists:
		clearWAL = true
	}

	needsSave := projectControlStateRequiresCanonicalization(state)
	projectControlNormalizeState(&state)
	if projectControlRecoverInterruptedToolRuns(&state, time.Now().UTC()) {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		needsSave = true
	}
	if stateSourceIsWAL {
		needsSave = true
	}
	if needsSave {
		if saveErr := s.saveLocked(username, state); saveErr != nil {
			return projectControlState{}, false, saveErr
		}
	} else if clearWAL {
		_ = os.Remove(walPath)
	}
	return state, stateExists, nil
}

func (s *projectControlStore) saveLocked(username string, state projectControlState) error {
	if err := os.MkdirAll(s.rootDir, 0700); err != nil {
		return err
	}
	path := s.pathFor(username)
	walPath := s.walPathFor(username)
	backupPath := path + ".bak"
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := projectControlWriteFileAtomically(walPath, payload, 0600); err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if err := projectControlWriteFileAtomically(backupPath, existing, 0600); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := projectControlWriteFileAtomically(path, payload, 0600); err != nil {
		return err
	}
	_ = os.Remove(walPath)
	return nil
}

func (s *projectControlStore) pathFor(username string) string {
	return filepath.Join(s.rootDir, storagePathSegment(username)+".json")
}

func (s *projectControlStore) walPathFor(username string) string {
	return filepath.Join(s.rootDir, storagePathSegment(username)+".wal.json")
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

	state := projectControlState{
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
				AcceptanceStatus: "under_human_review",
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
		Artifacts: []projectControlArtifact{
			{ID: "artifact-seed-ia-plan", TaskID: projectControlTaskIAID, Kind: "plan", Outcome: "recorded", Label: "Plan", Value: "Information architecture review plan recorded.", CreatedAt: stampA},
			{ID: "artifact-seed-ia-diff", TaskID: projectControlTaskIAID, Kind: "diff_summary", Outcome: "recorded", Label: "Diff summary", Value: "Refined Project Panel drill-down documentation and acceptance split.", CreatedAt: stampB},
			{ID: "artifact-seed-ia-test", TaskID: projectControlTaskIAID, Kind: "test_result", Outcome: "pass", Label: "Test result", Value: "Documentation review completed without blocking validation findings.", CreatedAt: stampB},
			{ID: "artifact-seed-ia-review", TaskID: projectControlTaskIAID, Kind: "review_result", Outcome: "pass", Label: "Review result", Value: "No blocking objections before final human acceptance.", CreatedAt: stampC},
			{ID: "artifact-seed-ia-completion", TaskID: projectControlTaskIAID, Kind: "completion_check", Outcome: "pass", Label: "Completion check", Value: "Seeded IA task has all completion evidence required for final acceptance review.", CreatedAt: stampC},
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
		Events: []projectControlRecordedEvent{
			{ID: "event-seed-ia-submitted", Timestamp: stampA, Actor: "human", Action: "submitted_doc", Detail: "Updated the information architecture proposal.", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamUXID, TaskID: projectControlTaskIAID},
			{ID: "event-seed-ia-complete", Timestamp: stampB, Actor: "system", Action: "completion_check_passed", Detail: "Execution marked complete; final acceptance kept separate.", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamUXID, TaskID: projectControlTaskIAID},
			{ID: "event-seed-acceptance-checkpoint", Timestamp: stampC, Actor: "policy_engine", Action: "checkpoint_raised", Detail: "Generated final_acceptance checkpoint for explicit human sign-off.", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamUXID, TaskID: projectControlTaskIAID, CheckpointID: projectControlCheckpointAcceptanceID},
			{ID: "event-seed-panel-queued", Timestamp: stampD, Actor: "orchestrator", Action: "queued", Detail: "Reserved for the live implementation session.", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamUXID, TaskID: projectControlTaskPanelID},
			{ID: "event-seed-runtime-scanned", Timestamp: stampE, Actor: "system", Action: "runtime_scanned", Detail: "Interactive attach available via the existing terminal workspace", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamRuntimeID, TaskID: projectControlTaskAttachID},
			{ID: "event-seed-approvals-blocked", Timestamp: stampF, Actor: "system", Action: "blocked", Detail: "Waiting for decision handling API.", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamRuntimeID, TaskID: projectControlTaskApprovalsID},
		},
		UpdatedAt: now.Format(time.RFC3339Nano),
	}
	projectControlNormalizeState(&state)
	return state
}

func parseProjectControlTimestamp(value string) time.Time {
	timestamp, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}
	}
	return timestamp
}

func projectControlPhaseAttemptSessionID(attempt projectControlPhaseAttempt) string {
	if sessionID := strings.TrimSpace(attempt.SessionID); sessionID != "" {
		return sessionID
	}
	if attempt.ID != "" {
		return "session-" + attempt.ID
	}
	return projectControlID("session", attempt.TaskID+"-"+attempt.PhaseID)
}

func projectControlPhaseAttemptSessionState(attempt projectControlPhaseAttempt) string {
	switch normalizeProjectControlPhaseAttemptStatus(attempt.Status) {
	case "completed":
		return "completed"
	case "failed":
		return "failed"
	case "cancelled":
		return "cancelled"
	default:
		return "active"
	}
}

func projectControlPhaseAttemptDurationLabel(attempt projectControlPhaseAttempt, now time.Time) string {
	startedAt := parseProjectControlTimestamp(attempt.StartedAt)
	if completedAt := parseProjectControlTimestamp(attempt.CompletedAt); !completedAt.IsZero() {
		return humanizeSince(startedAt, completedAt)
	}
	return humanizeSince(startedAt, now)
}

func projectControlArtifactSessionText(artifact projectControlArtifact) string {
	label := strings.TrimSpace(artifact.Label)
	value := strings.TrimSpace(artifact.Value)
	if label != "" && value != "" {
		return label + ": " + value
	}
	if value != "" {
		return value
	}
	if label != "" {
		return label
	}
	return normalizeProjectControlArtifactKind(artifact.Kind)
}

func projectControlPhaseAttemptArtifactTexts(attempt projectControlPhaseAttempt, artifacts []projectControlArtifact) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, artifactID := range attempt.ArtifactIDs {
		artifactID = strings.TrimSpace(artifactID)
		if artifactID == "" || seen[artifactID] {
			continue
		}
		for _, artifact := range artifacts {
			if artifact.ID == artifactID {
				result = append(result, projectControlArtifactSessionText(artifact))
				seen[artifactID] = true
				break
			}
		}
	}
	for _, artifact := range artifacts {
		if artifact.PhaseAttemptID != attempt.ID || seen[artifact.ID] {
			continue
		}
		result = append(result, projectControlArtifactSessionText(artifact))
		seen[artifact.ID] = true
	}
	if result == nil {
		return []string{}
	}
	return result
}

func projectControlPhaseAttemptSummary(attempt projectControlPhaseAttempt) string {
	phaseName := strings.ReplaceAll(normalizeProjectControlPhaseID(attempt.PhaseID), "_", " ")
	if phaseName == "" {
		phaseName = "phase"
	}
	switch normalizeProjectControlPhaseAttemptStatus(attempt.Status) {
	case "completed":
		return "Runbook phase " + phaseName + " completed with recorded artifacts."
	case "failed":
		reason := strings.TrimSpace(attempt.FailureReason)
		if reason == "" {
			reason = "No failure reason recorded."
		}
		return "Runbook phase " + phaseName + " failed: " + reason
	case "cancelled":
		return "Runbook phase " + phaseName + " was cancelled."
	default:
		workspaceRef := strings.TrimSpace(attempt.WorkspaceRef)
		if workspaceRef == "" {
			workspaceRef = "the configured workspace"
		} else if description := projectControlWorkspaceDescription(workspaceRef); description != "" {
			workspaceRef = description
		}
		return "Runbook phase " + phaseName + " is running in " + workspaceRef + "."
	}
}

func projectControlPhaseAttemptClaims(attempt projectControlPhaseAttempt) []string {
	switch normalizeProjectControlPhaseAttemptStatus(attempt.Status) {
	case "completed":
		return []string{"Phase completed.", "Artifacts were recorded against the runbook attempt."}
	case "failed":
		return []string{"Phase failed.", "Recovery should continue through the runbook."}
	case "cancelled":
		return []string{"Phase was cancelled before completion."}
	default:
		return []string{"Interactive phase execution is in progress."}
	}
}

func projectControlSessionForPhaseAttempt(attempt projectControlPhaseAttempt, task projectControlTask, runbook projectControlRunbook, artifacts []projectControlArtifact, live terminal.SessionInfo, hasLive bool, terminalsConnected bool, now time.Time) projectControlSession {
	phase, ok := findProjectControlRunbookPhase(runbook, attempt.PhaseID)
	executionRole := strings.TrimSpace(phase.ExecutionRole)
	if !ok || executionRole == "" {
		executionRole = normalizeProjectControlPhaseID(attempt.PhaseID)
	}
	sessionID := projectControlPhaseAttemptSessionID(attempt)
	terminalID := ""
	name := projectControlPhaseSessionName(task, attempt.PhaseID)
	startedAt := attempt.StartedAt
	if hasLive {
		terminalID = live.ID
		if strings.TrimSpace(live.Name) != "" {
			name = live.Name
		}
		if strings.TrimSpace(startedAt) == "" {
			startedAt = live.CreatedAt.UTC().Format(time.RFC3339)
		}
	}
	return projectControlSession{
		ID:             sessionID,
		TaskID:         attempt.TaskID,
		PhaseAttemptID: attempt.ID,
		TerminalID:     terminalID,
		Name:           name,
		AgentType:      attempt.AgentType,
		RuntimeID:      attempt.RuntimeID,
		State:          projectControlPhaseAttemptSessionState(attempt),
		ExecutionRole:  executionRole,
		SystemRole:     "worker",
		WorkspaceRef:   attempt.WorkspaceRef,
		DurationLabel:  projectControlPhaseAttemptDurationLabel(attempt, now),
		StartedAt:      startedAt,
		Summary:        projectControlPhaseAttemptSummary(attempt),
		Claims:         projectControlPhaseAttemptClaims(attempt),
		Artifacts:      projectControlPhaseAttemptArtifactTexts(attempt, artifacts),
		SupportsAttach: terminalsConnected && hasLive && projectControlPhaseAllowsTerminalAttach(phase),
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
	skills := cloneProjectControlSkills(defaultProjectControlSkills())
	runbooks := defaultProjectControlRunbooks()
	phaseAttempts := cloneProjectControlPhaseAttempts(state.PhaseAttempts)
	toolRuns := cloneProjectControlToolRuns(state.ToolRuns)
	artifacts := cloneProjectControlArtifacts(state.Artifacts)
	checkpoints := cloneProjectControlCheckpoints(state.Checkpoints)
	decisions := cloneProjectControlDecisions(state.Decisions)
	memories := cloneProjectControlMemories(state.Memories)
	events := cloneProjectControlRecordedEvents(state.Events)
	timelineEvents := append(cloneProjectControlRecordedEvents(state.Events), projectControlMemoryEvents(memories)...)
	applyRecordedEventsToTasks(tasks, timelineEvents)
	applyProjectControlArtifactsToTasks(tasks, artifacts, memories)
	for i := range tasks {
		refreshProjectControlTaskRunbookFields(&tasks[i], artifacts)
	}
	taskIndexByID := map[string]int{}
	for i := range tasks {
		taskIndexByID[tasks[i].ID] = i
	}
	runtimes := []projectControlRuntime{{ID: projectControlRuntimeID, Name: runtimeName, Kind: runtimeKind, Status: "online", InteractiveAttach: terminals != nil, HealthSummary: healthSummary}}
	sessions := []projectControlSession{{
		ID:             "session-review-ia",
		TaskID:         projectControlTaskIAID,
		Name:           "IA review session",
		AgentType:      "reviewer",
		RuntimeID:      projectControlRuntimeID,
		State:          "completed",
		ExecutionRole:  "review",
		SystemRole:     "worker",
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
	liveTerminalByID := map[string]terminal.SessionInfo{}
	for _, live := range liveTerminalSessions {
		liveTerminalByID[live.ID] = live
	}
	linkedTerminalIDs := map[string]bool{}
	for _, attempt := range phaseAttempts {
		taskIndex, ok := taskIndexByID[attempt.TaskID]
		if !ok {
			continue
		}
		sessionID := projectControlPhaseAttemptSessionID(attempt)
		terminalID := projectControlTerminalIDFromSessionID(sessionID)
		live, hasLive := liveTerminalByID[terminalID]
		if hasLive {
			linkedTerminalIDs[terminalID] = true
		}
		task := tasks[taskIndex]
		runbook := projectControlRunbookForTask(task)
		phaseSession := projectControlSessionForPhaseAttempt(attempt, task, runbook, artifacts, live, hasLive, terminals != nil, now)
		tasks[taskIndex].SessionIDs = appendUniqueString(tasks[taskIndex].SessionIDs, phaseSession.ID)
		sessions = append(sessions, phaseSession)
	}
	unlinkedLiveIndex := 0
	for _, live := range liveTerminalSessions {
		if linkedTerminalIDs[live.ID] {
			continue
		}
		pcSession := projectControlSession{
			ID:             projectControlSessionIDForTerminal(live.ID),
			TerminalID:     live.ID,
			Name:           live.Name,
			AgentType:      "worker",
			RuntimeID:      projectControlRuntimeID,
			State:          "active",
			ExecutionRole:  "implement",
			SystemRole:     "worker",
			DurationLabel:  humanizeSince(live.CreatedAt, now),
			StartedAt:      live.CreatedAt.UTC().Format(time.RFC3339),
			Summary:        "Live terminal session surfaced through the Project Panel so the operator can attach back into Terminal mode.",
			Claims:         []string{"Interactive execution still in progress."},
			Artifacts:      []string{"Terminal transcript available in live session", "Existing workspace + shell context"},
			SupportsAttach: terminals != nil,
		}
		targetTaskIndex := 1
		if unlinkedLiveIndex == 1 {
			targetTaskIndex = 2
		} else if unlinkedLiveIndex >= 2 {
			targetTaskIndex = 3
			pcSession.ExecutionRole = "verify"
		}
		unlinkedLiveIndex += 1
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

	dashboard := buildProjectControlDashboard(workstreams, tasks, checkpoints, runtimes, events)
	tools := projectControlToolsForState(&state)
	return projectControlSnapshot{
		GeneratedAt:     now.Format(time.RFC3339),
		ActiveProjectID: state.ActiveProjectID,
		ApprovalsCount:  countPendingProjectControlCheckpoints(checkpoints),
		Projects:        projects,
		Workstreams:     workstreams,
		Tasks:           tasks,
		Sessions:        sessions,
		Runtimes:        runtimes,
		Skills:          skills,
		Runbooks:        runbooks,
		Tools:           tools,
		PhaseAttempts:   phaseAttempts,
		ToolRuns:        toolRuns,
		Artifacts:       artifacts,
		Checkpoints:     checkpoints,
		Decisions:       decisions,
		Memories:        memories,
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

func buildProjectControlDashboard(workstreams []projectControlWorkstream, tasks []projectControlTask, checkpoints []projectControlCheckpoint, runtimes []projectControlRuntime, events []projectControlRecordedEvent) projectControlDashboard {
	runningWorkstreams := 0
	runningTasks := 0
	blockedTasks := 0
	recentFailures := []string{}
	recentDecisions := []string{}
	runtimeHealth := []string{}
	projectTimeline := []string{}
	for _, workstream := range workstreams {
		if workstream.Status == "running" {
			runningWorkstreams++
		}
	}
	for _, task := range tasks {
		switch task.State {
		case "running", "waiting_review", "waiting_human":
			runningTasks++
		case "blocked", "failed":
			blockedTasks++
			if task.State == "failed" {
				recentFailures = append(recentFailures, task.Title)
			}
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
	if len(events) > 1 {
		sort.Slice(events, func(i, j int) bool {
			return events[i].Timestamp > events[j].Timestamp
		})
	}
	for _, event := range events {
		projectTimeline = append(projectTimeline, event.Action+": "+event.Detail)
		if len(projectTimeline) >= 6 {
			break
		}
	}
	if len(recentFailures) == 0 {
		recentFailures = []string{"No recent failures recorded"}
	}
	if len(recentDecisions) == 0 {
		recentDecisions = []string{"No decisions recorded yet"}
	}
	return projectControlDashboard{
		RunningWorkstreams: runningWorkstreams,
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

func normalizeProjectControlTaskState(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "planned", "queued", "running", "waiting_review", "waiting_human", "blocked", "failed", "execution_complete", "archived":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return "planned"
	}
}

func normalizeProjectControlPhaseID(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "plan", "implement", "write", "test", "review", "fix_or_replan", "final_validation", "ready_for_acceptance":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return ""
	}
}

func normalizeProjectControlRunbookState(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "not_started", "in_progress", "waiting_review", "needs_fix", "ready_for_acceptance", "accepted", "archived":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return ""
	}
}

func normalizeProjectControlPhaseAttemptStatus(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "running", "completed", "failed", "cancelled":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return "running"
	}
}

func normalizeProjectControlToolRunStatus(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "running", "completed", "failed", "cancelled":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return "running"
	}
}

func normalizeProjectControlArtifactKind(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "plan", "diff_summary", "doc_summary", "test_result", "review_result", "completion_check":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
}

func normalizeProjectControlAcceptanceStatus(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "not_ready", "ready_for_acceptance", "under_human_review", "accepted", "rejected":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return "not_ready"
	}
}

func normalizeProjectControlWorkstreamStatus(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "planned", "running", "blocked", "waiting_human", "failed", "completed", "archived":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return "planned"
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
	if state.PhaseAttempts == nil {
		state.PhaseAttempts = []projectControlPhaseAttempt{}
	}
	if state.ToolRuns == nil {
		state.ToolRuns = []projectControlToolRun{}
	}
	if state.Artifacts == nil {
		state.Artifacts = []projectControlArtifact{}
	}
	if state.Checkpoints == nil {
		state.Checkpoints = []projectControlCheckpoint{}
	}
	if state.Decisions == nil {
		state.Decisions = []projectControlDecision{}
	}
	if state.Events == nil {
		state.Events = []projectControlRecordedEvent{}
	}
	if state.Memories == nil {
		state.Memories = []projectControlTaskMemory{}
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
		state.Workstreams[i].Status = normalizeProjectControlWorkstreamStatus(state.Workstreams[i].Status)
		if state.Workstreams[i].RowVersion < 1 {
			state.Workstreams[i].RowVersion = 1
		}
	}
	for i := range state.Tasks {
		state.Tasks[i].ID = strings.TrimSpace(state.Tasks[i].ID)
		state.Tasks[i].ProjectID = strings.TrimSpace(state.Tasks[i].ProjectID)
		state.Tasks[i].WorkstreamID = strings.TrimSpace(state.Tasks[i].WorkstreamID)
		state.Tasks[i].Title = strings.TrimSpace(state.Tasks[i].Title)
		state.Tasks[i].AcceptanceDecisionID = strings.TrimSpace(state.Tasks[i].AcceptanceDecisionID)
		state.Tasks[i].ArchiveDecisionID = strings.TrimSpace(state.Tasks[i].ArchiveDecisionID)
		state.Tasks[i].State = normalizeProjectControlTaskState(state.Tasks[i].State)
		state.Tasks[i].AcceptanceStatus = normalizeProjectControlAcceptanceStatus(state.Tasks[i].AcceptanceStatus)
		state.Tasks[i].Priority = normalizeProjectControlPriority(state.Tasks[i].Priority)
		state.Tasks[i].RiskLevel = normalizeProjectControlRisk(state.Tasks[i].RiskLevel)
		state.Tasks[i].SelectedSkill = strings.TrimSpace(state.Tasks[i].SelectedSkill)
		state.Tasks[i].RunbookID = strings.TrimSpace(state.Tasks[i].RunbookID)
		state.Tasks[i].CurrentPhase = normalizeProjectControlPhaseID(state.Tasks[i].CurrentPhase)
		state.Tasks[i].RunbookState = normalizeProjectControlRunbookState(state.Tasks[i].RunbookState)
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
		refreshProjectControlTaskRunbookFields(&state.Tasks[i], state.Artifacts)
		if state.Tasks[i].RowVersion < 1 {
			state.Tasks[i].RowVersion = 1
		}
	}
	for i := range state.PhaseAttempts {
		state.PhaseAttempts[i].ID = strings.TrimSpace(state.PhaseAttempts[i].ID)
		state.PhaseAttempts[i].TaskID = strings.TrimSpace(state.PhaseAttempts[i].TaskID)
		state.PhaseAttempts[i].RunbookID = strings.TrimSpace(state.PhaseAttempts[i].RunbookID)
		if state.PhaseAttempts[i].RunbookID == "" {
			state.PhaseAttempts[i].RunbookID = projectControlDefaultRunbookID
		}
		state.PhaseAttempts[i].PhaseID = normalizeProjectControlPhaseID(state.PhaseAttempts[i].PhaseID)
		state.PhaseAttempts[i].AgentType = strings.TrimSpace(state.PhaseAttempts[i].AgentType)
		state.PhaseAttempts[i].RuntimeID = strings.TrimSpace(state.PhaseAttempts[i].RuntimeID)
		state.PhaseAttempts[i].WorkspaceRef = strings.TrimSpace(state.PhaseAttempts[i].WorkspaceRef)
		state.PhaseAttempts[i].Status = normalizeProjectControlPhaseAttemptStatus(state.PhaseAttempts[i].Status)
		if state.PhaseAttempts[i].ArtifactIDs == nil {
			state.PhaseAttempts[i].ArtifactIDs = []string{}
		}
	}
	for i := range state.ToolRuns {
		state.ToolRuns[i].ID = strings.TrimSpace(state.ToolRuns[i].ID)
		state.ToolRuns[i].TaskID = strings.TrimSpace(state.ToolRuns[i].TaskID)
		state.ToolRuns[i].PhaseAttemptID = strings.TrimSpace(state.ToolRuns[i].PhaseAttemptID)
		state.ToolRuns[i].PhaseID = normalizeProjectControlPhaseID(state.ToolRuns[i].PhaseID)
		state.ToolRuns[i].ToolID = normalizeProjectControlToolID(state.ToolRuns[i].ToolID)
		state.ToolRuns[i].WorkspaceRef = strings.TrimSpace(state.ToolRuns[i].WorkspaceRef)
		state.ToolRuns[i].Status = normalizeProjectControlToolRunStatus(state.ToolRuns[i].Status)
		state.ToolRuns[i].StartedAt = strings.TrimSpace(state.ToolRuns[i].StartedAt)
		state.ToolRuns[i].CompletedAt = strings.TrimSpace(state.ToolRuns[i].CompletedAt)
		state.ToolRuns[i].ArtifactID = strings.TrimSpace(state.ToolRuns[i].ArtifactID)
		if strings.TrimSpace(state.ToolRuns[i].Outcome) != "" {
			state.ToolRuns[i].Outcome = normalizeProjectControlArtifactOutcome(state.ToolRuns[i].Outcome)
		}
		state.ToolRuns[i].Summary = strings.TrimSpace(state.ToolRuns[i].Summary)
		state.ToolRuns[i].Error = strings.TrimSpace(state.ToolRuns[i].Error)
	}
	for i := range state.Artifacts {
		state.Artifacts[i].ID = strings.TrimSpace(state.Artifacts[i].ID)
		state.Artifacts[i].TaskID = strings.TrimSpace(state.Artifacts[i].TaskID)
		state.Artifacts[i].PhaseAttemptID = strings.TrimSpace(state.Artifacts[i].PhaseAttemptID)
		state.Artifacts[i].Kind = normalizeProjectControlArtifactKind(state.Artifacts[i].Kind)
		state.Artifacts[i].Outcome = normalizeProjectControlArtifactOutcome(state.Artifacts[i].Outcome)
		state.Artifacts[i].Label = strings.TrimSpace(state.Artifacts[i].Label)
		state.Artifacts[i].Value = strings.TrimSpace(state.Artifacts[i].Value)
	}
	for i := range state.Checkpoints {
		state.Checkpoints[i].ID = strings.TrimSpace(state.Checkpoints[i].ID)
		state.Checkpoints[i].TaskID = strings.TrimSpace(state.Checkpoints[i].TaskID)
		state.Checkpoints[i].ResolvedByDecisionID = strings.TrimSpace(state.Checkpoints[i].ResolvedByDecisionID)
		if state.Checkpoints[i].AllowedActions == nil {
			state.Checkpoints[i].AllowedActions = []string{}
		}
		if state.Checkpoints[i].RowVersion < 1 {
			state.Checkpoints[i].RowVersion = 1
		}
	}
	for i := range state.Events {
		state.Events[i].ID = strings.TrimSpace(state.Events[i].ID)
		state.Events[i].Actor = strings.TrimSpace(state.Events[i].Actor)
		state.Events[i].Action = strings.TrimSpace(state.Events[i].Action)
		state.Events[i].ProjectID = strings.TrimSpace(state.Events[i].ProjectID)
		state.Events[i].WorkstreamID = strings.TrimSpace(state.Events[i].WorkstreamID)
		state.Events[i].TaskID = strings.TrimSpace(state.Events[i].TaskID)
		state.Events[i].CheckpointID = strings.TrimSpace(state.Events[i].CheckpointID)
	}
	for i := range state.Decisions {
		state.Decisions[i].ID = strings.TrimSpace(state.Decisions[i].ID)
		state.Decisions[i].DecisionType = strings.TrimSpace(state.Decisions[i].DecisionType)
		state.Decisions[i].Actor = strings.TrimSpace(state.Decisions[i].Actor)
		state.Decisions[i].ProjectID = strings.TrimSpace(state.Decisions[i].ProjectID)
		state.Decisions[i].WorkstreamID = strings.TrimSpace(state.Decisions[i].WorkstreamID)
		state.Decisions[i].TaskID = strings.TrimSpace(state.Decisions[i].TaskID)
		state.Decisions[i].CheckpointID = strings.TrimSpace(state.Decisions[i].CheckpointID)
	}
	for i := range state.Memories {
		state.Memories[i].TaskID = strings.TrimSpace(state.Memories[i].TaskID)
		state.Memories[i].ProjectID = strings.TrimSpace(state.Memories[i].ProjectID)
		state.Memories[i].WorkstreamID = strings.TrimSpace(state.Memories[i].WorkstreamID)
		state.Memories[i].WindowStart = strings.TrimSpace(state.Memories[i].WindowStart)
		state.Memories[i].WindowEnd = strings.TrimSpace(state.Memories[i].WindowEnd)
		state.Memories[i].Summary = strings.TrimSpace(state.Memories[i].Summary)
		state.Memories[i].UpdatedAt = strings.TrimSpace(state.Memories[i].UpdatedAt)
		if state.Memories[i].Highlights == nil {
			state.Memories[i].Highlights = []string{}
		}
		for j := range state.Memories[i].Highlights {
			state.Memories[i].Highlights[j] = strings.TrimSpace(state.Memories[i].Highlights[j])
		}
	}
	if len(state.Events) == 0 {
		projectControlMigrateLegacyHistoryToEvents(state)
	}
	projectControlCompactState(state)
	if state.ActiveProjectID == "" && len(state.Projects) > 0 {
		state.ActiveProjectID = state.Projects[0].ID
	}
	for i := range state.Tasks {
		refreshProjectControlTaskRunbookFields(&state.Tasks[i], state.Artifacts)
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

func projectControlAppendRecordedEvent(state *projectControlState, event projectControlRecordedEvent) {
	if state == nil {
		return
	}
	if strings.TrimSpace(event.ID) == "" {
		event.ID = projectControlID("event", event.Action)
	}
	state.Events = append(state.Events, event)
}

func projectControlMigrateLegacyHistoryToEvents(state *projectControlState) {
	if state == nil {
		return
	}
	for _, task := range state.Tasks {
		for _, item := range task.Timeline {
			projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
				Timestamp:    item.Timestamp,
				Actor:        item.Actor,
				Action:       item.Action,
				Detail:       item.Detail,
				ProjectID:    task.ProjectID,
				WorkstreamID: task.WorkstreamID,
				TaskID:       task.ID,
			})
		}
		for _, item := range task.Audit {
			projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
				Timestamp:    item.Timestamp,
				Actor:        item.Actor,
				Action:       item.Action,
				Detail:       item.Detail,
				ProjectID:    task.ProjectID,
				WorkstreamID: task.WorkstreamID,
				TaskID:       task.ID,
			})
		}
	}
}

func projectControlMemoryEvent(memory projectControlTaskMemory) (projectControlRecordedEvent, bool) {
	taskID := strings.TrimSpace(memory.TaskID)
	summary := strings.TrimSpace(memory.Summary)
	if taskID == "" || summary == "" {
		return projectControlRecordedEvent{}, false
	}
	timestamp := strings.TrimSpace(memory.WindowEnd)
	if timestamp == "" {
		timestamp = strings.TrimSpace(memory.UpdatedAt)
	}
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	detail := summary
	if len(memory.Highlights) > 0 {
		detail += " Highlights: " + strings.Join(memory.Highlights, "; ")
	}
	return projectControlRecordedEvent{
		ID:           "memory-" + slugifyProjectControl(taskID),
		Timestamp:    timestamp,
		Actor:        "system",
		Action:       "history_compacted",
		Detail:       detail,
		ProjectID:    strings.TrimSpace(memory.ProjectID),
		WorkstreamID: strings.TrimSpace(memory.WorkstreamID),
		TaskID:       taskID,
	}, true
}

func projectControlMemoryEvents(memories []projectControlTaskMemory) []projectControlRecordedEvent {
	events := make([]projectControlRecordedEvent, 0, len(memories))
	for _, memory := range memories {
		event, ok := projectControlMemoryEvent(memory)
		if !ok {
			continue
		}
		events = append(events, event)
	}
	return events
}

func applyProjectControlArtifactsToTasks(tasks []projectControlTask, artifacts []projectControlArtifact, memories []projectControlTaskMemory) {
	if len(tasks) == 0 {
		return
	}
	indexByTaskID := make(map[string]int, len(tasks))
	for i := range tasks {
		indexByTaskID[tasks[i].ID] = i
		tasks[i].Evidence = []projectControlEvidence{}
	}
	if len(artifacts) > 1 {
		sort.Slice(artifacts, func(i, j int) bool {
			if artifacts[i].CreatedAt == artifacts[j].CreatedAt {
				return artifacts[i].ID > artifacts[j].ID
			}
			return artifacts[i].CreatedAt > artifacts[j].CreatedAt
		})
	}
	for _, artifact := range artifacts {
		idx, ok := indexByTaskID[artifact.TaskID]
		if !ok {
			continue
		}
		label := strings.TrimSpace(artifact.Label)
		if label == "" {
			label = normalizeProjectControlArtifactKind(artifact.Kind)
		}
		tasks[idx].Evidence = append(tasks[idx].Evidence, projectControlEvidence{
			Label: label,
			Value: strings.TrimSpace(artifact.Value),
		})
	}
	for _, memory := range memories {
		idx, ok := indexByTaskID[strings.TrimSpace(memory.TaskID)]
		if !ok || strings.TrimSpace(memory.Summary) == "" {
			continue
		}
		tasks[idx].Evidence = append(tasks[idx].Evidence, projectControlEvidence{
			Label: "History memory",
			Value: memory.Summary,
		})
	}
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
		out[i].MissingEvidence = append([]string{}, item.MissingEvidence...)
		out[i].FilesChanged = append([]string{}, item.FilesChanged...)
		out[i].SessionIDs = append([]string{}, item.SessionIDs...)
		out[i].Timeline = append([]projectControlEvent{}, item.Timeline...)
		out[i].Evidence = append([]projectControlEvidence{}, item.Evidence...)
		out[i].Audit = append([]projectControlAuditItem{}, item.Audit...)
	}
	return out
}

func cloneProjectControlSkills(items []projectControlSkill) []projectControlSkill {
	out := make([]projectControlSkill, len(items))
	for i, item := range items {
		out[i] = item
		out[i].AllowedRunbookIDs = append([]string{}, item.AllowedRunbookIDs...)
		out[i].RequiredArtifacts = append([]string{}, item.RequiredArtifacts...)
		out[i].PermissionsByPhase = map[string]string{}
		for phaseID, permission := range item.PermissionsByPhase {
			out[i].PermissionsByPhase[phaseID] = permission
		}
	}
	return out
}

func cloneProjectControlPhaseAttempts(items []projectControlPhaseAttempt) []projectControlPhaseAttempt {
	out := make([]projectControlPhaseAttempt, len(items))
	for i, item := range items {
		out[i] = item
		out[i].ArtifactIDs = append([]string{}, item.ArtifactIDs...)
	}
	return out
}

func cloneProjectControlToolRuns(items []projectControlToolRun) []projectControlToolRun {
	out := make([]projectControlToolRun, len(items))
	copy(out, items)
	return out
}

func cloneProjectControlArtifacts(items []projectControlArtifact) []projectControlArtifact {
	out := make([]projectControlArtifact, len(items))
	copy(out, items)
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

func cloneProjectControlDecisions(items []projectControlDecision) []projectControlDecision {
	out := make([]projectControlDecision, len(items))
	copy(out, items)
	return out
}

func cloneProjectControlRecordedEvents(items []projectControlRecordedEvent) []projectControlRecordedEvent {
	out := make([]projectControlRecordedEvent, len(items))
	copy(out, items)
	return out
}

func cloneProjectControlMemories(items []projectControlTaskMemory) []projectControlTaskMemory {
	out := make([]projectControlTaskMemory, len(items))
	for i, item := range items {
		out[i] = item
		out[i].Highlights = append([]string{}, item.Highlights...)
	}
	return out
}

func applyRecordedEventsToTasks(tasks []projectControlTask, events []projectControlRecordedEvent) {
	if len(tasks) == 0 {
		return
	}
	indexByTaskID := make(map[string]int, len(tasks))
	for i := range tasks {
		indexByTaskID[tasks[i].ID] = i
		tasks[i].Timeline = []projectControlEvent{}
		tasks[i].Audit = []projectControlAuditItem{}
	}
	if len(events) > 1 {
		sort.Slice(events, func(i, j int) bool {
			return events[i].Timestamp < events[j].Timestamp
		})
	}
	for _, event := range events {
		idx, ok := indexByTaskID[event.TaskID]
		if !ok {
			continue
		}
		tasks[idx].Timeline = append(tasks[idx].Timeline, projectControlEvent{
			Timestamp: event.Timestamp,
			Actor:     event.Actor,
			Action:    event.Action,
			Detail:    event.Detail,
		})
		if strings.HasPrefix(event.Action, "checkpoint_") {
			tasks[idx].Audit = append(tasks[idx].Audit, projectControlAuditItem{
				Timestamp: event.Timestamp,
				Actor:     event.Actor,
				Action:    event.Action,
				Detail:    event.Detail,
			})
		}
	}
}

func generateAgentToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

func (s *projectControlStore) ensureAgentToken(username string) (string, error) {
	state, err := s.loadOrSeed(username)
	if err != nil {
		return "", err
	}
	if state.AgentToken != "" {
		return state.AgentToken, nil
	}
	token := generateAgentToken()
	_, err = s.withStateLocked(username, func(state *projectControlState) error {
		if state.AgentToken == "" {
			state.AgentToken = token
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *projectControlStore) validateAgentToken(username, token string) bool {
	state, err := s.loadOrSeed(username)
	if err != nil || state.AgentToken == "" {
		return false
	}
	return state.AgentToken == token
}

func (s *projectControlStore) agentGetTask(username string) (map[string]interface{}, error) {
	state, err := s.loadOrSeed(username)
	if err != nil {
		return nil, err
	}
	for _, task := range state.Tasks {
		st := normalizeProjectControlTaskState(task.State)
		if st == "running" || st == "waiting_review" {
			runbook := projectControlRunbookForTask(task)
			var phase projectControlRunbookPhase
			if p, ok := findProjectControlRunbookPhase(runbook, task.CurrentPhase); ok {
				phase = p
			}
			return map[string]interface{}{
				"taskId":            task.ID,
				"title":             task.Title,
				"goal":              task.Goal,
				"state":             task.State,
				"currentPhase":      task.CurrentPhase,
				"runbookState":      task.RunbookState,
				"requiredArtifacts": phase.RequiredArtifacts,
				"missingEvidence":   task.MissingEvidence,
				"nextStep":          task.NextStep,
			}, nil
		}
	}
	return map[string]interface{}{"taskId": "", "message": "no active task"}, nil
}

type agentArtifactRequest struct {
	TaskID       string `json:"taskId"`
	PhaseID      string `json:"phaseId"`
	ArtifactKind string `json:"artifactKind"`
	Outcome      string `json:"outcome"`
	Label        string `json:"label"`
	Value        string `json:"value"`
}

func (s *projectControlStore) agentSubmitArtifact(username string, req agentArtifactRequest, terminals *terminal.Manager) (projectControlSnapshot, error) {
	if req.TaskID == "" || req.ArtifactKind == "" {
		return projectControlSnapshot{}, errors.New("taskId and artifactKind are required")
	}
	return s.updateTask(username, req.TaskID, projectControlTaskUpdateRequest{
		ExpectedRowVersion: -1, // agent bypass
		Action:             "complete_phase",
		PhaseID:            req.PhaseID,
		ArtifactKind:       req.ArtifactKind,
		ArtifactOutcome:    req.Outcome,
		ArtifactLabel:      req.Label,
		ArtifactValue:      req.Value,
	}, terminals)
}

type agentCheckpointRequest struct {
	TaskID string `json:"taskId"`
	Reason string `json:"reason"`
}

func (s *projectControlStore) agentRequestCheckpoint(username string, req agentCheckpointRequest) error {
	if req.TaskID == "" || req.Reason == "" {
		return errors.New("taskId and reason are required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.withStateLocked(username, func(state *projectControlState) error {
		checkpoint := projectControlCheckpoint{
			ID:             projectControlID("checkpoint", "agent-"+req.TaskID),
			TaskID:         req.TaskID,
			Kind:           "agent_request",
			Title:          "Agent requests human input",
			Reason:         req.Reason,
			Status:         "pending",
			RequestedAt:    now,
			AllowedActions: []string{"approve", "reject"},
		}
		state.Checkpoints = append(state.Checkpoints, checkpoint)
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
			ID:        projectControlID("event", "agent-checkpoint"),
			Timestamp: now,
			Actor:     "agent",
			Action:    "checkpoint_requested",
			Detail:    "Agent requested checkpoint: " + req.Reason,
			TaskID:    req.TaskID,
		})
		return nil
	})
	return err
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

func (s *Server) handleProjectControlEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload, err := s.projectControl.eventsForUser(GetUsername(r), r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleProjectControlTaskReplay(w http.ResponseWriter, r *http.Request) {
	prefix := "/api/project-control/tasks/"
	path := strings.TrimPrefix(r.URL.Path, prefix)
	if path == "" {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodGet {
		if !strings.HasSuffix(path, "/replay") {
			http.NotFound(w, r)
			return
		}
		taskID := strings.TrimSpace(strings.Trim(strings.TrimSuffix(path, "/replay"), "/"))
		if taskID == "" || len(taskID) > 128 || !isValidProjectControlID(taskID) {
			http.NotFound(w, r)
			return
		}
		payload, err := s.projectControl.replayForTask(GetUsername(r), taskID, s.terminals)
		if err != nil {
			status := http.StatusBadRequest
			if err.Error() == "task not found" {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
		return
	}

	if r.Method == http.MethodPatch {
		taskID := strings.TrimSpace(strings.Trim(path, "/"))
		if taskID == "" || strings.Contains(taskID, "/") || len(taskID) > 128 || !isValidProjectControlID(taskID) {
			http.NotFound(w, r)
			return
		}
		var req projectControlTaskUpdateRequest
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		snapshot, err := s.projectControl.updateTask(GetUsername(r), taskID, req, s.terminals)
		if err != nil {
			status := http.StatusBadRequest
			if err.Error() == "task not found" {
				status = http.StatusNotFound
			} else if _, ok := err.(projectControlConflictError); ok {
				status = http.StatusConflict
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

func (s *Server) handleProjectControlWorkstreamUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	prefix := "/api/project-control/workstreams/"
	workstreamID := strings.TrimSpace(strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/"))
	if workstreamID == "" || len(workstreamID) > 128 || !isValidProjectControlID(workstreamID) {
		http.NotFound(w, r)
		return
	}
	var req projectControlWorkstreamUpdateRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	snapshot, err := s.projectControl.updateWorkstream(GetUsername(r), workstreamID, req, s.terminals)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "workstream not found" {
			status = http.StatusNotFound
		} else if _, ok := err.(projectControlConflictError); ok {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleProjectControlTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
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
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
