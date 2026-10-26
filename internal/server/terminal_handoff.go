package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
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
	DraftID    string `json:"draftId,omitempty"`
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

	// Tracked pastes remain draft provenance until the input is submitted.
	// Older clients have no draft lifecycle and keep their supplement API.
	// A tracked draft always stays pending, even on an active task: cancelling
	// the editor must not have changed persisted provenance already.
	if req.DraftID == "" {
		if agent, hasAgent := s.terminals.AgentProcessInSession(live.ID); hasAgent {
			if taskID, err := s.handoffTargetTask(r.Context(), username, live, agent); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			} else if taskID != "" {
				s.supplementSourceRef(w, r, username, taskID, match, sourceTaskID, live.ID)
				return
			}
		}
	}

	pending, err := s.controlPlane.RecordPendingHandoff(r.Context(), username, controlplane.PendingHandoff{
		SourceTaskID:   sourceTaskID,
		SourceTerminal: match.TerminalID,
		TargetTerminal: live.ID,
		Fingerprint:    match.Fingerprint,
		Snapshot:       match.Snapshot,
		CopyID:         match.ID,
		DraftID:        strings.TrimSpace(req.DraftID),
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

// handoffSubmitMu serializes the submission decision (execution lookup →
// task creation → generation mapping) per process: concurrent reports of
// the same submission must agree on exactly one task.
var handoffSubmitMu sync.Mutex

// handleHandoffCancel invalidates a particular editor draft, never every
// pending source in the terminal and never a previously accepted submission.
func (s *Server) handleHandoffCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req handoffSubmitRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<18)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.DraftID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "draftId is required"})
		return
	}
	username := GetUsername(r)
	live, status, message := s.handoffTerminal(username, req.TerminalID)
	if status != http.StatusOK {
		writeJSON(w, status, map[string]string{"error": message})
		return
	}
	handoffSubmitMu.Lock()
	defer handoffSubmitMu.Unlock()
	if err := s.controlPlane.CancelHandoffDraft(r.Context(), username, live.ID, req.DraftID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
}

// handoffSubmitRequest is the payload reporting that the user actually
// submitted a drafted goal (Enter after a tracked paste) into a terminal.
type handoffSubmitRequest struct {
	TerminalID string `json:"terminalId"`
	Text       string `json:"text"`
	DraftID    string `json:"draftId,omitempty"`
}

// handleHandoffSubmit accepts a verified submission event: the user sent
// the drafted goal into a terminal where an agent CLI is running. It
// creates (or reuses) the goal task, consumes the terminal's waiting
// handoffs and links the submitted text back to its copy provenance. All
// provenance writes are pinned to the task first so a failed write is
// recoverable against the same submission.
func (s *Server) handleHandoffSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := GetUsername(r)
	var req handoffSubmitRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<18)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	live, status, message := s.handoffTerminal(username, req.TerminalID)
	if status != http.StatusOK {
		writeJSON(w, status, map[string]string{"error": message})
		return
	}
	agent, hasAgent := s.terminals.AgentProcessInSession(live.ID)
	if !hasAgent {
		// The "goal" went into a plain shell; nothing accepted it.
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"submitted": false, "reason": "no agent session in the target terminal"})
		return
	}

	handoffSubmitMu.Lock()
	defer handoffSubmitMu.Unlock()
	ctx := r.Context()

	match, _, err := s.controlPlane.MatchTerminalPaste(ctx, username, req.Text, pasteMatchLookback, live.ID)
	sourceTaskID := ""
	if err == nil {
		if match.TaskID != "" {
			sourceTaskID = match.TaskID
		} else if match.ID != "" {
			sourceTaskID = s.registerTaskForTerminalPersisted(username, match.TerminalID)
		}
		if sourceTaskID != "" {
			state, err := s.projectControl.loadOrSeed(username)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if _, ok := controlTask(state, sourceTaskID); !ok {
				sourceTaskID = ""
			}
		}
	}

	targetTaskID, targetErr := s.handoffTargetTask(ctx, username, live, agent)
	if targetErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": targetErr.Error()})
		return
	}
	if targetTaskID == "" {
		pendings, _ := s.controlPlane.ListPendingHandoffs(ctx, username, live.ID)
		if req.DraftID != "" {
			filtered := pendings[:0]
			for _, pending := range pendings {
				if pending.DraftID == req.DraftID {
					filtered = append(filtered, pending)
				}
			}
			pendings = filtered
		}
		created := projectControlTask{}
		_, err := s.projectControl.withStateLocked(username, func(state *projectControlState) error {
			// Explicit target attribution first: session metadata or an
			// existing binding. Source inheritance only for unbound targets.
			projectID, workstreamID, ctxErr := registrationContext(*state, &live)
			if ctxErr != nil || (live.ProjectID == "" && taskIDForTerminalInState(*state, live.ID) == "") {
				if pID, wID := handoffProjectFromState(state, pendings, sourceTaskID); pID != "" {
					projectID, workstreamID = pID, wID
				} else if ctxErr != nil {
					return ctxErr
				}
			}
			sourceTitle := ""
			if sourceTaskID != "" {
				if task, ok := controlTask(*state, sourceTaskID); ok {
					sourceTitle = task.Title
				}
			}
			title := "审查: " + firstNonEmpty(sourceTitle, sourceTaskID)
			if title == "审查: " {
				title = "终端提交目标"
			}
			var createErr error
			created, createErr = s.projectControl.createTaskInState(username, state, projectControlTaskCreateRequest{
				ProjectID:    projectID,
				WorkstreamID: workstreamID,
				Title:        title,
				Goal:         "审查来自 " + firstNonEmpty(sourceTitle, "终端粘贴内容") + " 的内容快照",
				Origin:       "agent",
			}, s.terminals)
			if createErr != nil {
				return createErr
			}
			bindTaskSessionExclusive(state, created.ID, live.ID)
			return nil
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		targetTaskID = created.ID
	}

	// The generation now carries this goal; later pastes feed it.
	if _, err := s.controlPlane.UpsertAgentExecution(ctx, username, controlplane.AgentExecution{
		TerminalID: live.ID, AgentSig: agent.Signature(), AgentName: agent.Name, TaskID: targetTaskID,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var drafts []string
	if req.DraftID != "" {
		drafts = []string{req.DraftID}
	}
	promotion := promotionSummary{}
	if pinErr := s.controlPlane.SetPendingIntendedTask(ctx, username, live.ID, targetTaskID, drafts...); pinErr != nil {
		promotion.Failed++
		promotion.Errors = append(promotion.Errors, pinErr.Error())
	} else {
		promotion = s.promotePendingHandoffs(ctx, username, live.ID, targetTaskID, drafts...)
	}

	body := map[string]interface{}{
		"submitted": true,
		"taskId":    targetTaskID,
		"promotion": promotion,
	}
	if err == nil && match.ID != "" {
		refID, refErr := s.linkSourceWithRecovery(ctx, username, targetTaskID, live.ID, match, sourceTaskID, req.DraftID)
		if refErr != nil {
			promotion.Failed++
			promotion.Errors = append(promotion.Errors, refErr.Error())
			body["promotion"] = promotion
		} else if refID != "" {
			body["refId"] = refID
			body["sourceTaskId"] = sourceTaskID
		}
	}
	// The response status reflects EVERY provenance outcome of this
	// submission — a successful current link must not mask an earlier
	// pending's failed promotion.
	respStatus := http.StatusOK
	if promotion.Failed > 0 {
		respStatus = http.StatusMultiStatus
	}
	// Keep the terminal's session binding pointing at the current goal so
	// copy-side attribution follows the latest accepted work.
	boundState, bindErr := s.projectControl.withStateLocked(username, func(state *projectControlState) error {
		if _, ok := controlTask(*state, targetTaskID); ok {
			bindTaskSessionExclusive(state, targetTaskID, live.ID)
		}
		return nil
	})
	if bindErr == nil {
		if task, ok := controlTask(boundState, targetTaskID); ok {
			bindErr = s.terminals.BindSessionTask(username, live.ID, targetTaskID, task.ProjectID)
		}
	}
	if bindErr != nil {
		promotion.Failed++
		promotion.Errors = append(promotion.Errors, bindErr.Error())
		body["promotion"] = promotion
		respStatus = http.StatusMultiStatus
	}
	writeJSON(w, respStatus, body)
}

// linkSourceWithRecovery persists the provenance of one submitted paste:
// the pending row is pinned to the target task before the reference write
// is attempted, so a failed write leaves a recoverable record instead of
// vanishing. Returns "" when the paste had no matching copy (source
// undetermined — the goal is still registered, without invented origin).
func (s *Server) linkSourceWithRecovery(ctx context.Context, username, targetTaskID, targetTerminal string, match controlplane.TerminalCopy, sourceTaskID, draftID string) (string, error) {
	if match.ID == "" || sourceTaskID == "" {
		return "", nil
	}
	pending, err := s.controlPlane.RecordPendingHandoff(ctx, username, controlplane.PendingHandoff{
		SourceTaskID:   sourceTaskID,
		SourceTerminal: match.TerminalID,
		TargetTerminal: targetTerminal,
		Fingerprint:    match.Fingerprint,
		Snapshot:       match.Snapshot,
		CopyID:         match.ID,
		DraftID:        draftID,
		IntendedTaskID: targetTaskID,
	})
	if err != nil {
		return "", err
	}
	ref, err := s.controlPlane.AddSourceRef(ctx, username, controlplane.SourceRef{
		TargetTaskID:   targetTaskID,
		SourceTaskID:   sourceTaskID,
		Relation:       "reviews",
		Snapshot:       match.Snapshot,
		SourceTerminal: match.TerminalID,
		TargetTerminal: targetTerminal,
		CopyID:         match.ID,
	})
	if err != nil {
		return "", err // pending stays pinned for recovery
	}
	if pending.ID != "" {
		_ = s.controlPlane.MarkHandoffPromoted(ctx, username, pending.ID, targetTaskID)
	}
	return ref.ID, nil
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

// projectControlTaskGoalFinished reports whether a task's goal is closed:
// a later submission on the same terminal must get its own card instead of
// reusing (and rewriting the provenance of) a completed one.
func projectControlTaskGoalFinished(task projectControlTask) bool {
	switch task.State {
	case "done", "execution_complete", "cancelled", "archived":
		return true
	}
	return strings.TrimSpace(task.ArchiveDecisionID) != ""
}

// The persisted exclusive binding is updated by every accepted goal. Session
// metadata is a fallback for worker terminals that have not registered yet;
// it must never override a newer accepted task or revive a finished one.
func (s *Server) handoffTargetTask(ctx context.Context, username string, live terminal.SessionInfo, agent terminal.AgentProcess) (string, error) {
	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		return "", err
	}
	taskID := taskIDForTerminalInState(state, live.ID)
	if taskID == "" {
		if execution, found, lookupErr := s.controlPlane.AgentExecutionBySig(ctx, username, live.ID, agent.Signature()); lookupErr != nil {
			return "", lookupErr
		} else if found {
			taskID = execution.TaskID
		}
	}
	if taskID == "" {
		taskID = live.TaskID
	}
	if task, ok := controlTask(state, taskID); ok && !projectControlTaskGoalFinished(task) {
		return task.ID, nil
	}
	return "", nil
}

// bindTaskSessionExclusive makes terminalID map to exactly taskID: stale
// bindings on earlier tasks are removed, so copy/paste attribution
// resolves to the terminal's current work, not the first history entry.
func bindTaskSessionExclusive(state *projectControlState, taskID, terminalID string) {
	sessionID := "session-live-" + strings.TrimSpace(terminalID)
	for i := range state.Tasks {
		kept := state.Tasks[i].SessionIDs[:0]
		for _, sid := range state.Tasks[i].SessionIDs {
			if sid == sessionID && state.Tasks[i].ID != taskID {
				continue
			}
			kept = append(kept, sid)
		}
		state.Tasks[i].SessionIDs = kept
	}
	for i := range state.Tasks {
		if state.Tasks[i].ID == taskID {
			state.Tasks[i].SessionIDs = appendUniqueString(state.Tasks[i].SessionIDs, sessionID)
			break
		}
	}
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
func (s *Server) promotePendingHandoffs(ctx context.Context, username, targetTerminal, targetTaskID string, draftIDs ...string) promotionSummary {
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
		currentDraft := len(draftIDs) == 0 || pending.DraftID == draftIDs[0]
		if pending.SourceTaskID == "" || pending.IntendedTaskID == "" || (pending.IntendedTaskID == targetTaskID && currentDraft) {
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
		if len(draftIDs) > 0 && pending.DraftID != draftIDs[0] {
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
