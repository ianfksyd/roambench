package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ianf339/roambench/internal/terminal"
)

// Agent runtime terminal.
//
// The local worker creates its execution terminal through this endpoint
// instead of shelling out to tmux itself, so the session is registered in
// the terminal manager and the web UI can attach to it like any other
// workspace terminal — the "see what the agent is doing" path stays the
// same for every session.

// agentTerminalRequest is the creation payload.
type agentTerminalRequest struct {
	TaskID    string `json:"taskId"`
	WorkDir   string `json:"workDir,omitempty"`
	Name      string `json:"name,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
}

// handleAgentTerminal creates a terminal session bound to a claimed task.
//
// POST /api/agent/v1/terminal {"taskId":"...","workDir":"/data/x"}
func (s *Server) handleAgentTerminal(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.terminals == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal runtime unavailable"})
		return
	}
	var req agentTerminalRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" || len(taskID) > maxProjectControlRouteIDLength || !isValidProjectControlID(taskID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid taskId is required"})
		return
	}
	// The workdir must belong to the claimed task: the payload from the
	// claim response is the source of truth, a client-supplied path is only
	// accepted when it matches.
	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	workspace := ""
	found := false
	for _, task := range state.Tasks {
		if task.ID == taskID {
			found = true
			if dir, dirErr := s.projectControl.controlledProjectDir(state, task); dirErr == nil {
				workspace = dir
			}
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	if strings.TrimSpace(req.WorkDir) != "" {
		reqWork, err := filepath.EvalSymlinks(filepath.Clean(strings.TrimSpace(req.WorkDir)))
		if err != nil || (workspace != "" && reqWork != workspace) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "workDir does not match the claimed task workspace"})
			return
		}
	}
	if workspace == "" {
		workspace = strings.TrimSpace(req.WorkDir)
	}
	if workspace == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task has no workspace"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "roambench-task"
	}
	session, err := s.terminals.CreateSessionWithOptions(username, terminal.SessionCreateOptions{
		WorkDir:   workspace,
		Name:      name,
		TaskID:    taskID,
		ProjectID: strings.TrimSpace(req.ProjectID),
	})
	if err != nil {
		status, message, retryAfter := terminalCreateErrorResponse(err)
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		writeJSON(w, status, map[string]string{"error": message})
		return
	}
	// Bind the terminal to the task, so the board shows the linkage and the
	// detail view can offer attach.
	s.projectControl.bindTaskTerminal(username, taskID, session.ID, strings.TrimSpace(req.ProjectID))
	writeJSON(w, http.StatusCreated, session)
}

// bindTaskTerminal links a terminal session to a task in the project state.
func (s *projectControlStore) bindTaskTerminal(username, taskID, terminalSessionID, projectID string) {
	if s == nil || strings.TrimSpace(terminalSessionID) == "" {
		return
	}
	sessionID := "session-live-" + strings.TrimSpace(terminalSessionID)
	_, err := s.withStateLocked(username, func(state *projectControlState) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID != taskID {
				continue
			}
			state.Tasks[i].SessionIDs = appendUniqueString(state.Tasks[i].SessionIDs, sessionID)
			projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
				ID:        projectControlID("event", "task-terminal-linked"),
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Actor:     "runtime",
				Action:    "task_terminal_linked",
				Detail:    "Runtime terminal " + sessionID + " attached to the task.",
				ProjectID: projectID,
				TaskID:    taskID,
			})
			return nil
		}
		return nil
	})
	_ = err
}
