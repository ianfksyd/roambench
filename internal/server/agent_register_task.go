package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ianf339/roambench/internal/controlplane"
	"github.com/ianf339/roambench/internal/terminal"
)

// registerTaskRequest is the payload for registering a new task from a
// terminal context, optionally bound to a live tmux session.
type registerTaskRequest struct {
	Title      string `json:"title"`
	Goal       string `json:"goal,omitempty"`
	TerminalID string `json:"terminalId,omitempty"`
}

// handleAgentRegisterTask creates a task card and, when a live tmux session
// id is supplied, binds that session to the new task — so work started in a
// terminal immediately appears as a managed card on the board.
//
// POST /api/agent/v1/register-task
func (s *Server) handleAgentRegisterTask(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req registerTaskRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}

	// Validate ownership before creating anything. An invalid terminal must
	// not leave an unbound task behind or be echoed back as a successful bind.
	terminalID := strings.TrimSpace(req.TerminalID)
	var live *terminal.SessionInfo
	if terminalID != "" {
		if s.terminals == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal runtime unavailable"})
			return
		}
		for _, info := range s.terminals.ListSessions(username) {
			if info.ID == terminalID {
				live = &info
				break
			}
		}
		if live == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "terminal not found"})
			return
		}
	}

	// Persist the new card and its binding together. Origin=agent preserves
	// the approval gate, so registration cannot start a duplicate executor.
	var created projectControlTask
	var requestErr error
	_, err := s.projectControl.withStateLocked(username, func(state *projectControlState) error {
		projectID, workstreamID, contextErr := registrationContext(*state, live)
		// Handoff project inheritance: an unbound terminal whose pasted
		// sources live in a known project registers there — not in
		// whatever project the UI is currently browsing. Explicit target
		// attribution (session metadata or an existing binding) wins.
		if live != nil && live.ProjectID == "" && taskIDForTerminalInState(*state, live.ID) == "" {
			pendings, pendingErr := s.controlPlane.ListPendingHandoffs(r.Context(), username, live.ID)
			if pendingErr == nil {
				if pID, wID := handoffProjectFromState(state, pendings, ""); pID != "" {
					projectID, workstreamID, contextErr = pID, wID, nil
				}
			}
		}
		if contextErr != nil {
			requestErr = contextErr
			return contextErr
		}
		var createErr error
		created, createErr = s.projectControl.createTaskInState(username, state, projectControlTaskCreateRequest{
			ProjectID: projectID, WorkstreamID: workstreamID, Title: title,
			Goal: strings.TrimSpace(req.Goal), Origin: "agent",
		}, s.terminals)
		if createErr != nil {
			requestErr = createErr
			return createErr
		}
		if live != nil {
			sessionID := projectControlSessionIDForTerminal(live.ID)
			for i := range state.Tasks {
				if state.Tasks[i].ID == created.ID {
					state.Tasks[i].SessionIDs = appendUniqueString(state.Tasks[i].SessionIDs, sessionID)
					break
				}
			}
			projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
				ID:        projectControlID("event", "task-terminal-linked"),
				Timestamp: time.Now().UTC().Format(time.RFC3339), Actor: "agent",
				Action: "task_terminal_linked", Detail: "Registered terminal " + sessionID + ".",
				ProjectID: projectID, WorkstreamID: workstreamID, TaskID: created.ID,
			})
		}
		return nil
	})
	if err != nil {
		status := http.StatusInternalServerError
		if requestErr != nil {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	// A validated submission event: pin the waiting handoffs to THIS task
	// (a failed promotion must recover against the same submission, never
	// leak to the next goal), then promote and report the outcome. The
	// running agent generation, if any, becomes this task's executor so
	// later pastes feed it.
	promotion := promotionSummary{}
	if live != nil {
		_ = s.controlPlane.SetPendingIntendedTask(r.Context(), username, live.ID, created.ID)
		promotion = s.promotePendingHandoffs(r.Context(), username, live.ID, created.ID)
		if agent, ok := s.terminals.AgentProcessInSession(live.ID); ok {
			if exec, found, _ := s.controlPlane.AgentExecutionBySig(r.Context(), username, live.ID, agent.Signature()); !found || exec.TaskID == "" {
				_, _ = s.controlPlane.RecordAgentExecution(r.Context(), username, controlplane.AgentExecution{
					TerminalID: live.ID, AgentSig: agent.Signature(), AgentName: agent.Name, TaskID: created.ID,
				})
			}
		}
	}

	body := map[string]interface{}{
		"taskId":     created.ID,
		"projectId":  created.ProjectID,
		"terminalId": terminalID,
		"promotion":  promotion,
	}
	status := http.StatusOK
	if promotion.Failed > 0 {
		// The card exists but its provenance is incomplete: not a clean 200.
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, body)
}

// Use the terminal's explicit task/project binding before the active UI
// project. Legacy bindings stored only in task.sessionIds are also valid.
func registrationContext(state projectControlState, live *terminal.SessionInfo) (string, string, error) {
	projectID, workstreamID := state.ActiveProjectID, ""
	if live != nil {
		if live.ProjectID != "" {
			projectID = live.ProjectID
		}
		var boundTask *projectControlTask
		if live.TaskID != "" {
			task, ok := controlTask(state, live.TaskID)
			if !ok {
				return "", "", errors.New("terminal task not found")
			}
			boundTask = &task
		} else {
			for _, task := range state.Tasks {
				for _, sessionID := range task.SessionIDs {
					if sessionID == projectControlSessionIDForTerminal(live.ID) {
						if boundTask != nil && (boundTask.ProjectID != task.ProjectID || boundTask.WorkstreamID != task.WorkstreamID) {
							return "", "", errors.New("terminal has conflicting task bindings")
						}
						matched := task
						boundTask = &matched
					}
				}
			}
		}
		if boundTask != nil {
			if live.ProjectID != "" && live.ProjectID != boundTask.ProjectID {
				return "", "", errors.New("terminal project does not match its task")
			}
			projectID, workstreamID = boundTask.ProjectID, boundTask.WorkstreamID
		}
	}
	if !projectControlProjectExists(state, projectID) {
		return "", "", errors.New("terminal or active project not found")
	}
	if workstreamID != "" {
		if !projectControlWorkstreamExists(state, workstreamID, projectID) {
			return "", "", errors.New("terminal workstream does not belong to its project")
		}
		return projectID, workstreamID, nil
	}
	for _, ws := range state.Workstreams {
		if ws.ProjectID == projectID {
			return projectID, ws.ID, nil
		}
	}
	return "", "", errors.New("no workstream available in the terminal or active project")
}
