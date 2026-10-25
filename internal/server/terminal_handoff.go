package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ianf339/roambench/internal/controlplane"
)

// Terminal handoff: automatic cross-terminal task registration driven by
// copy/paste behaviour. No confirmation dialogs, no extra clicks — the
// operator copies from one agent terminal, pastes into another and submits
// the goal; the review card and its source link appear on the board.
//
// Boundaries: copy alone records provenance only; paste alone matches and
// links; the review task is created at most once per source+target pair and
// never as a running execution. Mistakes are correctable after the fact via
// the source-links delete endpoint.

// pasteMatchLookback bounds how far back a paste can match a copy.
const pasteMatchLookback = 30 * time.Minute

// handoffCopyRequest is the copy payload from the terminal UI.
type handoffCopyRequest struct {
	TerminalID string `json:"terminalId"`
	Text       string `json:"text"`
}

// handoffPasteRequest is the paste payload from the terminal UI.
type handoffPasteRequest struct {
	TerminalID string `json:"terminalId"`
	Text       string `json:"text"`
}

// handleHandoffCopy records a copy event with source attribution.
func (s *Server) handleHandoffCopy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := GetUsername(r)
	var req handoffCopyRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<18)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}

	// Resolve the task bound to the source terminal (if any).
	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	sourceTaskID := taskIDForTerminalInState(state, strings.TrimSpace(req.TerminalID))

	copyRecord, err := s.controlPlane.RecordTerminalCopy(r.Context(), username,
		strings.TrimSpace(req.TerminalID), sourceTaskID, req.Text)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"copyId":       copyRecord.ID,
		"sourceTaskId": copyRecord.TaskID,
	})
}

// handleHandoffPaste matches a paste against recent copies and creates or
// links the review task.
func (s *Server) handleHandoffPaste(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := GetUsername(r)
	var req handoffPasteRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<18)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}

	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	targetTerminal := strings.TrimSpace(req.TerminalID)
	targetTaskID := taskIDForTerminalInState(state, targetTerminal)

	match, matched, err := s.controlPlane.MatchTerminalPaste(r.Context(), username, req.Text, pasteMatchLookback)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !matched {
		// Unmatched text (external or old clipboard): record the paste as a
		// copy from this terminal so a later submission can still link it.
		copyRecord, copyErr := s.controlPlane.RecordTerminalCopy(r.Context(), username,
			targetTerminal, taskIDForTerminalInState(state, targetTerminal), req.Text)
		if copyErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": copyErr.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"matched": false,
			"copyId":  copyRecord.ID,
		})
		return
	}

	sourceTaskID := match.TaskID
	if sourceTaskID == "" {
		// The source terminal was never registered as a task: register it now
		// so the target is not an orphan card.
		sourceTaskID = registerTaskForTerminalState(username, &state, match.TerminalID, match.CreatedAt)
	}

	// Target already has a task → supplement the source reference, never
	// create a duplicate card per paste.
	if targetTaskID != "" {
		ref, refErr := s.controlPlane.AddSourceRef(r.Context(), username, controlplane.SourceRef{
			TargetTaskID:   targetTaskID,
			SourceTaskID:   sourceTaskID,
			Relation:       "reviews",
			Snapshot:       match.Snapshot,
			SourceTerminal: match.TerminalID,
			TargetTerminal: targetTerminal,
		})
		if refErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": refErr.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"matched":      true,
			"taskId":       targetTaskID,
			"sourceTaskId": sourceTaskID,
			"relation":     "supplemented",
			"refId":        ref.ID,
		})
		return
	}

	// No task on the target terminal yet: create the review card (planned,
	// origin agent → waits in the approvals inbox; never auto-running).
	sourceTitle := taskTitleInState(state, sourceTaskID)
	createReq := projectControlTaskCreateRequest{
		ProjectID:    projectIDOfTask(state, sourceTaskID),
		WorkstreamID: workstreamIDOfTask(state, sourceTaskID),
		Title:        "审查: " + firstNonEmpty(sourceTitle, sourceTaskID),
		Goal:         "审查来自 " + firstNonEmpty(sourceTitle, sourceTaskID) + " 的内容快照",
		Origin:       "agent",
	}
	snapshot, err := s.projectControl.createTask(username, createReq, s.terminals)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	targetTaskID = snapshot.CreatedTaskID
	ref, refErr := s.controlPlane.AddSourceRef(r.Context(), username, controlplane.SourceRef{
		TargetTaskID:   targetTaskID,
		SourceTaskID:   sourceTaskID,
		Relation:       "reviews",
		Snapshot:       match.Snapshot,
		SourceTerminal: match.TerminalID,
		TargetTerminal: targetTerminal,
	})
	if refErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": refErr.Error()})
		return
	}
	// Bind the paste target terminal to the new review task so the panel can
	// attach to it.
	if targetTerminal != "" {
		s.projectControl.bindTaskTerminal(username, targetTaskID, targetTerminal, projectIDOfTask(state, sourceTaskID))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"matched":      true,
		"taskId":       targetTaskID,
		"sourceTaskId": sourceTaskID,
		"relation":     "reviews",
		"refId":        ref.ID,
	})
}

// handleSourceLinksList lists the source relationships of a task.
func (s *Server) handleSourceLinksList(w http.ResponseWriter, r *http.Request, username, taskID string) {
	refs, err := s.controlPlane.ListSourceRefs(r.Context(), username, taskID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"refs": refs})
}

// handleSourceLinkDelete removes one source relationship (undo).
func (s *Server) handleSourceLinkDelete(w http.ResponseWriter, r *http.Request, username, linkID string) {
	removed, err := s.controlPlane.DeleteSourceRef(r.Context(), username, linkID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"removed": removed})
}

// ---- state helpers ----

func taskIDForTerminalInState(state projectControlState, terminalID string) string {
	sessionID := "session-live-" + strings.TrimSpace(terminalID)
	for _, task := range state.Tasks {
		for _, sid := range task.SessionIDs {
			if sid == sessionID {
				return task.ID
			}
		}
	}
	return ""
}

func taskTitleInState(state projectControlState, taskID string) string {
	task, ok := controlTask(state, taskID)
	if !ok {
		return ""
	}
	return task.Title
}

func projectIDOfTask(state projectControlState, taskID string) string {
	task, ok := controlTask(state, taskID)
	if !ok {
		return ""
	}
	return task.ProjectID
}

func workstreamIDOfTask(state projectControlState, taskID string) string {
	task, ok := controlTask(state, taskID)
	if !ok {
		return ""
	}
	return task.WorkstreamID
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// handleSourceLinkRoutes dispatches /api/source-links/{id} delete.
func (s *Server) handleSourceLinkRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := GetUsername(r)
	linkID := strings.TrimPrefix(r.URL.Path, "/api/source-links/")
	if linkID == "" || strings.Contains(linkID, "/") {
		http.NotFound(w, r)
		return
	}
	s.handleSourceLinkDelete(w, r, username, linkID)
}

// handleSourceLinksListAll lists every source relationship of the user
// (board and graph projection).
func (s *Server) handleSourceLinksListAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	refs, err := s.controlPlane.ListSourceRefsForUser(r.Context(), GetUsername(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"refs": refs})
}

// registerTaskForTerminal registers the source terminal as a task when the
// operator started work in a terminal that was never registered (P1 review:
// the target must not become an orphan card). The source terminal keeps its
// own binding; the created task stays planned until explicitly dispatched.
func registerTaskForTerminalState(username string, state *projectControlState, terminalID, createdAt string) string {
	// Resolve the active project and a workstream in it.
	projectID := state.ActiveProjectID
	workstreamID := ""
	for _, ws := range state.Workstreams {
		if ws.ProjectID == projectID {
			workstreamID = ws.ID
			break
		}
	}
	if projectID == "" || workstreamID == "" {
		return ""
	}
	terminalID = strings.TrimSpace(terminalID)
	title := "终端工作: " + terminalID
	task := projectControlTask{
		ID:               projectControlID("task", title),
		ProjectID:        projectID,
		WorkstreamID:     workstreamID,
		Title:            title,
		State:            "planned",
		AcceptanceStatus: "not_ready",
		Origin:           "agent",
		SessionIDs:       []string{"session-live-" + terminalID},
		CurrentPhase:     "plan",
		MissingEvidence:  []string{},
		FilesChanged:     []string{},
		Timeline:         []projectControlEvent{},
		Evidence:         []projectControlEvidence{},
		Audit:            []projectControlAuditItem{},
		RowVersion:       1,
	}
	if err := validateTaskContract(*state, task); err != nil {
		return ""
	}
	state.Tasks = append(state.Tasks, task)
	projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
		ID:        projectControlID("event", "terminal-registered"),
		Timestamp: createdAt,
		Actor:     "runtime",
		Action:    "terminal_registered",
		Detail:    "Terminal " + terminalID + " registered as a task (auto handoff).",
		ProjectID: projectID,
		TaskID:    task.ID,
	})
	return task.ID
}
