package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ianf339/roambench/internal/controlplane"
	"github.com/ianf339/roambench/internal/terminal"
)

// Terminal handoff: automatic cross-terminal provenance driven by
// copy/paste behaviour. No confirmation dialogs, no extra clicks — but
// also no fabricated work: a copy records provenance, a paste records a
// pending handoff, and only a validated submission event (agent
// register-task on the target terminal) creates the task card and promotes
// the pending handoff to a source relationship. Mistakes are correctable
// after the fact via the source-links delete endpoint.
//
// Boundaries enforced here:
//   - terminalId must be a live session of the requesting user (source and
//     target alike); unknown terminals are rejected, never invented.
//   - an unmatched paste records nothing — a paste cannot forge a copy, so
//     repeated pastes of external text never acquire a phantom source.
//   - a paste from the same terminal never matches its own copy.
//   - a paste onto a terminal without a task changes nothing visible until
//     the operator actually submits the goal on that terminal.

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

// handoffTerminal resolves a terminal the user actually owns. A missing or
// foreign session is an error, never a silently accepted guess.
func (s *Server) handoffTerminal(username, terminalID string) (terminal.SessionInfo, int, string) {
	terminalID = strings.TrimSpace(terminalID)
	if terminalID == "" {
		return terminal.SessionInfo{}, http.StatusBadRequest, "terminalId is required"
	}
	if s.terminals == nil {
		return terminal.SessionInfo{}, http.StatusServiceUnavailable, "terminal runtime unavailable"
	}
	for _, info := range s.terminals.ListSessions(username) {
		if info.ID == terminalID {
			return info, http.StatusOK, ""
		}
	}
	return terminal.SessionInfo{}, http.StatusNotFound, "terminal not found"
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
	live, status, message := s.handoffTerminal(username, req.TerminalID)
	if status != http.StatusOK {
		writeJSON(w, status, map[string]string{"error": message})
		return
	}

	// Resolve the task bound to the source terminal (if any).
	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	sourceTaskID := taskIDForTerminalInState(state, live.ID)

	copyRecord, err := s.controlPlane.RecordTerminalCopy(r.Context(), username, live.ID, sourceTaskID, req.Text)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"copyId":       copyRecord.ID,
		"sourceTaskId": copyRecord.TaskID,
	})
}

// handleHandoffPaste matches a paste against recent copies and records the
// provenance — a source reference on an already-registered target task, or
// a pending handoff that a later submission event will promote. It never
// creates a task card by itself.
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
	live, status, message := s.handoffTerminal(username, req.TerminalID)
	if status != http.StatusOK {
		writeJSON(w, status, map[string]string{"error": message})
		return
	}

	match, matched, err := s.controlPlane.MatchTerminalPaste(r.Context(), username, req.Text, pasteMatchLookback, live.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !matched {
		// External or stale text: nothing to link, and nothing to record —
		// a paste must never impersonate a copy event.
		writeJSON(w, http.StatusOK, map[string]interface{}{"matched": false})
		return
	}

	sourceTaskID := match.TaskID
	if sourceTaskID == "" {
		// The copy came from a terminal that never became a task: register it
		// now, persistently, so the relationship never points at a phantom.
		sourceTaskID = s.registerTaskForTerminalPersisted(username, match.TerminalID)
		if sourceTaskID == "" {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "source terminal no longer available; no source registered"})
			return
		}
	}

	// The source task must actually exist in the persisted state before any
	// relationship is written.
	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if _, ok := controlTask(state, sourceTaskID); !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "source task does not exist"})
		return
	}

	// Validated submission boundary: an agent CLI is running in the target
	// pane, so the paste is that agent's input by construction — not a
	// half-composed draft. This is what connects a bare Codex/Claude
	// session without any in-agent adapter.
	if agent, hasAgent := s.terminals.AgentProcessInSession(live.ID); hasAgent {
		s.handleAgentPaste(w, r, username, live, agent, match, sourceTaskID)
		return
	}

	// No agent running: the paste alone commits nothing visible. It is
	// recorded as a pending handoff and becomes a source relationship when
	// the goal is actually submitted (agent CLI started and fed, or an
	// explicit register-task). Repeats collapse by paste-event identity;
	// the same text from a different source stays a separate event.
	pending, err := s.controlPlane.RecordPendingHandoff(r.Context(), username, controlplane.PendingHandoff{
		SourceTaskID:   sourceTaskID,
		SourceTerminal: match.TerminalID,
		TargetTerminal: live.ID,
		Fingerprint:    match.Fingerprint,
		Snapshot:       match.Snapshot,
		CopyID:         match.ID,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"matched":      true,
		"pending":      true,
		"sourceTaskId": sourceTaskID,
		"copyId":       pending.CopyID,
	})
}

// handleAgentPaste routes a paste delivered into a live agent session.
// The running process generation owns the terminal's current execution:
// pastes feed that task; a new generation starts a new goal.
func (s *Server) handleAgentPaste(w http.ResponseWriter, r *http.Request, username string, live terminal.SessionInfo, agent terminal.AgentProcess, match controlplane.TerminalCopy, sourceTaskID string) {
	ctx := r.Context()

	// Known generation → this paste feeds the task it carries.
	if exec, found, _ := s.controlPlane.AgentExecutionBySig(ctx, username, live.ID, agent.Signature()); found && exec.TaskID != "" {
		state, err := s.projectControl.loadOrSeed(username)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if _, ok := controlTask(state, exec.TaskID); ok {
			s.supplementSourceRef(w, r, username, exec.TaskID, match, sourceTaskID, live.ID)
			return
		}
	}

	// A worker/agent-terminal created for a specific task carries the task
	// in its session metadata: adopt that task for this generation.
	if live.TaskID != "" {
		state, err := s.projectControl.loadOrSeed(username)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if _, ok := controlTask(state, live.TaskID); ok {
			_, _ = s.controlPlane.RecordAgentExecution(ctx, username, controlplane.AgentExecution{
				TerminalID: live.ID, AgentSig: agent.Signature(), AgentName: agent.Name, TaskID: live.TaskID,
			})
			s.supplementSourceRef(w, r, username, live.TaskID, match, sourceTaskID, live.ID)
			return
		}
	}

	// Unknown generation, unbound terminal: the submission starts a new
	// goal. Create the card, consume every waiting handoff of this
	// terminal, and record the generation as its executor.
	pendings, _ := s.controlPlane.ListPendingHandoffs(ctx, username, live.ID)
	created := projectControlTask{}
	_, err := s.projectControl.withStateLocked(username, func(state *projectControlState) error {
		projectID, workstreamID := handoffProjectFromState(state, pendings, sourceTaskID)
		if projectID == "" {
			liveCopy := live
			pID, wID, ctxErr := registrationContext(*state, &liveCopy)
			if ctxErr != nil {
				return ctxErr
			}
			projectID, workstreamID = pID, wID
		}
		sourceTitle := ""
		if task, ok := controlTask(*state, sourceTaskID); ok {
			sourceTitle = task.Title
		}
		var createErr error
		created, createErr = s.projectControl.createTaskInState(username, state, projectControlTaskCreateRequest{
			ProjectID:    projectID,
			WorkstreamID: workstreamID,
			Title:        "审查: " + firstNonEmpty(sourceTitle, sourceTaskID),
			Goal:         "审查来自 " + firstNonEmpty(sourceTitle, sourceTaskID) + " 的内容快照",
			Origin:       "agent",
		}, s.terminals)
		if createErr != nil {
			return createErr
		}
		for i := range state.Tasks {
			if state.Tasks[i].ID == created.ID {
				state.Tasks[i].SessionIDs = appendUniqueString(state.Tasks[i].SessionIDs, "session-live-"+live.ID)
				break
			}
		}
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	_, _ = s.controlPlane.RecordAgentExecution(ctx, username, controlplane.AgentExecution{
		TerminalID: live.ID, AgentSig: agent.Signature(), AgentName: agent.Name, TaskID: created.ID,
	})
	_ = s.controlPlane.SetPendingIntendedTask(ctx, username, live.ID, created.ID)
	promotion := s.promotePendingHandoffs(ctx, username, live.ID, created.ID)
	ref, refErr := s.controlPlane.AddSourceRef(ctx, username, controlplane.SourceRef{
		TargetTaskID:   created.ID,
		SourceTaskID:   sourceTaskID,
		Relation:       "reviews",
		Snapshot:       match.Snapshot,
		SourceTerminal: match.TerminalID,
		TargetTerminal: live.ID,
		CopyID:         match.ID,
	})
	body := map[string]interface{}{
		"matched":      true,
		"taskId":       created.ID,
		"sourceTaskId": sourceTaskID,
		"relation":     "reviews",
		"auto":         true,
		"promotion":    promotion,
	}
	if refErr == nil {
		body["refId"] = ref.ID
	}
	writeJSON(w, http.StatusOK, body)
}

// supplementSourceRef writes one reviews reference onto an existing task
// and answers with the supplemented payload.
func (s *Server) supplementSourceRef(w http.ResponseWriter, r *http.Request, username, targetTaskID string, match controlplane.TerminalCopy, sourceTaskID, targetTerminal string) {
	ref, refErr := s.controlPlane.AddSourceRef(r.Context(), username, controlplane.SourceRef{
		TargetTaskID:   targetTaskID,
		SourceTaskID:   sourceTaskID,
		Relation:       "reviews",
		Snapshot:       match.Snapshot,
		SourceTerminal: match.TerminalID,
		TargetTerminal: targetTerminal,
		CopyID:         match.ID,
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
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// handoffProjectFromState picks the project an unbound handoff target
// belongs to: the oldest waiting handoff's source project, else the
// current paste's source project. The UI's currently-browsed project is
// never used as a fallback for handoff work.
func handoffProjectFromState(state *projectControlState, pendings []controlplane.PendingHandoff, sourceTaskID string) (string, string) {
	for _, pending := range pendings {
		if pending.IntendedTaskID != "" || pending.SourceTaskID == "" {
			continue
		}
		if task, ok := controlTask(*state, pending.SourceTaskID); ok && task.ProjectID != "" {
			return task.ProjectID, task.WorkstreamID
		}
	}
	if task, ok := controlTask(*state, sourceTaskID); ok && task.ProjectID != "" {
		return task.ProjectID, task.WorkstreamID
	}
	return "", ""
}

// promotionSummary reports what a submission did with the handoffs that
// were waiting on its terminal. Failures are explicit: the caller must
// not answer a submission with an unqualified success when its provenance
// was not persisted.
type promotionSummary struct {
	Promoted int      `json:"promoted"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

// promotePendingHandoffs attaches every waiting handoff of a target
// terminal to the task a validated submission event just confirmed, as
// reviews source relationships.
//
// Handoffs pinned to an earlier task of the same terminal (a promotion
// that failed back then) are retried against that original task first —
// a new submission never inherits another goal's provenance.
func (s *Server) promotePendingHandoffs(ctx context.Context, username, targetTerminal, targetTaskID string) promotionSummary {
	summary := promotionSummary{}
	if username == "" || targetTerminal == "" || targetTaskID == "" {
		summary.Failed = 1
		summary.Errors = append(summary.Errors, "missing username, terminal or task")
		return summary
	}
	pendings, err := s.controlPlane.ListPendingHandoffs(ctx, username, targetTerminal)
	if err != nil {
		summary.Failed = 1
		summary.Errors = append(summary.Errors, err.Error())
		return summary
	}
	state, stateErr := s.projectControl.loadOrSeed(username)
	attach := func(pending controlplane.PendingHandoff, taskID string) {
		if _, err := s.controlPlane.AddSourceRef(ctx, username, controlplane.SourceRef{
			TargetTaskID:   taskID,
			SourceTaskID:   pending.SourceTaskID,
			Relation:       "reviews",
			Snapshot:       pending.Snapshot,
			SourceTerminal: pending.SourceTerminal,
			TargetTerminal: pending.TargetTerminal,
			CopyID:         pending.CopyID,
		}); err != nil {
			summary.Failed++
			summary.Errors = append(summary.Errors, err.Error())
			return
		}
		if err := s.controlPlane.MarkHandoffPromoted(ctx, username, pending.ID, taskID); err != nil {
			summary.Failed++
			summary.Errors = append(summary.Errors, err.Error())
			return
		}
		summary.Promoted++
	}
	for _, pending := range pendings {
		if pending.SourceTaskID == "" || pending.IntendedTaskID == "" || pending.IntendedTaskID == targetTaskID {
			continue
		}
		// Recovery of an earlier failed submission: reattach to its own
		// task, which must still exist in the persisted state.
		if stateErr != nil {
			summary.Failed++
			summary.Errors = append(summary.Errors, stateErr.Error())
			continue
		}
		if _, ok := controlTask(state, pending.IntendedTaskID); !ok {
			summary.Failed++
			summary.Errors = append(summary.Errors, "intended task "+pending.IntendedTaskID+" no longer exists")
			continue
		}
		attach(pending, pending.IntendedTaskID)
	}
	for _, pending := range pendings {
		if pending.SourceTaskID == "" || (pending.IntendedTaskID != "" && pending.IntendedTaskID != targetTaskID) {
			continue
		}
		attach(pending, targetTaskID)
	}
	return summary
}

// registerTaskForTerminalPersisted registers a live terminal as a planned
// task in the persisted project state (the source side of a handoff whose
// origin terminal was never registered). If the terminal already maps to a
// task, that task is returned. Returns "" when the terminal is gone or has
// no project context — the caller must then fail instead of inventing a
// source.
func (s *Server) registerTaskForTerminalPersisted(username, terminalID string) string {
	if strings.TrimSpace(terminalID) == "" {
		return ""
	}
	if _, status, _ := s.handoffTerminal(username, terminalID); status != http.StatusOK {
		return ""
	}
	created := ""
	_, err := s.projectControl.withStateLocked(username, func(state *projectControlState) error {
		if existing := taskIDForTerminalInState(*state, terminalID); existing != "" {
			created = existing
			return nil
		}
		projectID := state.ActiveProjectID
		workstreamID := ""
		for _, ws := range state.Workstreams {
			if ws.ProjectID == projectID {
				workstreamID = ws.ID
				break
			}
		}
		if projectID == "" || workstreamID == "" {
			return nil
		}
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
			return err
		}
		state.Tasks = append(state.Tasks, task)
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
			ID:        projectControlID("event", "terminal-registered"),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Actor:     "runtime",
			Action:    "terminal_registered",
			Detail:    "Terminal " + terminalID + " registered as a task (handoff source).",
			ProjectID: projectID,
			TaskID:    task.ID,
		})
		created = task.ID
		return nil
	})
	if err != nil {
		return ""
	}
	return created
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
