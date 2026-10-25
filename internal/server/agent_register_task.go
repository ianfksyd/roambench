package server

import (
	"encoding/json"
	"net/http"
	"strings"
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

	// Locate the project: prefer the terminal's bound task's project, else
	// the active project.
	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	projectID := state.ActiveProjectID
	workstreamID := ""
	for _, ws := range state.Workstreams {
		if ws.ProjectID == projectID {
			workstreamID = ws.ID
			break
		}
	}
	if workstreamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no workstream available in the active project"})
		return
	}

	// Origin "agent": the task waits for a human decision in the approvals
	// inbox instead of entering the worker queue — registering work observed
	// in a terminal must not silently start another executor on it.
	// The created task id is taken from the dedicated task record (matching
	// by title would return a pre-existing task of the same name).
	createReq := projectControlTaskCreateRequest{
		ProjectID:    projectID,
		WorkstreamID: workstreamID,
		Title:        title,
		Goal:         strings.TrimSpace(req.Goal),
		Origin:       "agent",
	}
	snapshot, err := s.projectControl.createTask(username, createReq, s.terminals)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	createdID := snapshot.CreatedTaskID

	// Bind the tmux session when provided: it must exist and be owned by the
	// same user, so a caller cannot hijack another session's identity.
	terminalID := strings.TrimSpace(req.TerminalID)
	if terminalID != "" && s.terminals != nil {
		found := false
		for _, live := range s.terminals.ListSessions(username) {
			if live.ID == terminalID {
				found = true
				break
			}
		}
		if found {
			s.projectControl.bindTaskTerminal(username, createdID, terminalID, projectID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"taskId":     createdID,
		"projectId":  projectID,
		"terminalId": terminalID,
	})
}
