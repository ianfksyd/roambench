// Terminal handoff: clipboard-driven cross-terminal task handoff.
//
// Copy in one agent terminal records the source (terminal, bound task,
// snapshot, fingerprint). Pasting the same content into another agent
// terminal records a pending handoff — never a task card. The pending
// handoff is promoted onto source relationships when a validated
// submission event (agent register-task) confirms the goal on the target
// terminal. Unmatched pastes record nothing: a paste cannot forge a copy,
// and a same-terminal paste cannot link to itself.
package controlplane

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// TerminalCopy is one recorded copy event.
type TerminalCopy struct {
	ID          string `json:"id"`
	TerminalID  string `json:"terminalId"`
	TaskID      string `json:"taskId,omitempty"`
	Fingerprint string `json:"fingerprint"`
	Snapshot    string `json:"snapshot"`
	CreatedAt   string `json:"createdAt"`
}

// SourceRef is a directed source relationship between two tasks
// (relation "reviews" and similar), independent of DependsOn.
type SourceRef struct {
	ID             string `json:"id"`
	Username       string `json:"-"`
	TargetTaskID   string `json:"targetTaskId"`
	SourceTaskID   string `json:"sourceTaskId"`
	Relation       string `json:"relation"`
	Snapshot       string `json:"snapshot,omitempty"`
	SourceTerminal string `json:"sourceTerminal,omitempty"`
	TargetTerminal string `json:"targetTerminal,omitempty"`
	CopyID         string `json:"copyId,omitempty"`
	CreatedAt      string `json:"createdAt"`
}

// PendingHandoff is a matched copy→paste pair on a not-yet-registered
// target terminal. It carries the provenance until a validated submission
// event creates the target task, then it is promoted to a SourceRef.
type PendingHandoff struct {
	ID             string `json:"id"`
	Username       string `json:"-"`
	SourceTaskID   string `json:"sourceTaskId,omitempty"`
	SourceTerminal string `json:"sourceTerminal"`
	TargetTerminal string `json:"targetTerminal"`
	Fingerprint    string `json:"fingerprint"`
	Snapshot       string `json:"snapshot,omitempty"`
	CopyID         string `json:"copyId,omitempty"`
	// IntendedTaskID pins a pending to the one submission it belongs to:
	// a failed promotion is retried against this task, never handed to the
	// next unrelated goal on the same terminal.
	IntendedTaskID string `json:"intendedTaskId,omitempty"`
	PromotedTaskID string `json:"promotedTaskId,omitempty"`
	CreatedAt      string `json:"createdAt"`
}

// AgentExecution binds one agent process generation in one terminal to
// the task whose execution it carries: pastes delivered while that exact
// process runs are inputs to that task, not new goals.
type AgentExecution struct {
	ID         string `json:"id"`
	Username   string `json:"-"`
	TerminalID string `json:"terminalId"`
	AgentSig   string `json:"agentSig"`
	AgentName  string `json:"agentName,omitempty"`
	TaskID     string `json:"taskId"`
	CreatedAt  string `json:"createdAt"`
}

// fingerprintText hashes text for paste matching.
func fingerprintText(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(text, "\r\n")))
	return hex.EncodeToString(sum[:])[:32]
}

// snapshotOf truncates copied text for storage.
func snapshotOf(text string) string {
	text = strings.TrimRight(text, "\r\n")
	if len(text) > 8000 {
		return text[:8000]
	}
	return text
}

func newRandomID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + "-" + hex.EncodeToString(b[:])
}

// RecordTerminalCopy stores one copy event and returns its record.
func (r *Repository) RecordTerminalCopy(ctx context.Context, username, terminalID, taskID, text string) (TerminalCopy, error) {
	username = strings.TrimSpace(username)
	terminalID = strings.TrimSpace(terminalID)
	taskID = strings.TrimSpace(taskID)
	if username == "" || terminalID == "" {
		return TerminalCopy{}, fmt.Errorf("%w: username and terminalId are required", ErrValidation)
	}
	if strings.TrimSpace(text) == "" {
		return TerminalCopy{}, fmt.Errorf("%w: copied text is empty", ErrValidation)
	}
	copyRecord := TerminalCopy{
		ID:          newRandomID("copy"),
		TerminalID:  terminalID,
		TaskID:      taskID,
		Fingerprint: fingerprintText(text),
		Snapshot:    snapshotOf(text),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO terminal_copies(id, username, terminal_id, task_id, fingerprint, snapshot, created_at)
		 VALUES(?,?,?,?,?,?,?)`,
		copyRecord.ID, username, copyRecord.TerminalID, copyRecord.TaskID,
		copyRecord.Fingerprint, copyRecord.Snapshot, copyRecord.CreatedAt)
	if err != nil {
		return TerminalCopy{}, err
	}
	return copyRecord, nil
}

// MatchTerminalPaste finds the newest copy with the same fingerprint by the
// same user within the lookback window, resolving its source task. Copies
// from the pasting terminal itself never match: a same-session paste is
// not a cross-terminal handoff and must not forge a source.
func (r *Repository) MatchTerminalPaste(ctx context.Context, username, text string, lookback time.Duration, excludeTerminalID string) (TerminalCopy, bool, error) {
	fp := fingerprintText(text)
	cutoff := time.Now().UTC().Add(-lookback).Format(time.RFC3339Nano)
	row := r.db.QueryRowContext(ctx,
		`SELECT id, terminal_id, task_id, fingerprint, snapshot, created_at FROM terminal_copies
		 WHERE username=? AND fingerprint=? AND created_at>=? AND terminal_id<>?
		 ORDER BY created_at DESC LIMIT 1`, username, fp, cutoff, strings.TrimSpace(excludeTerminalID))
	var c TerminalCopy
	err := row.Scan(&c.ID, &c.TerminalID, &c.TaskID, &c.Fingerprint, &c.Snapshot, &c.CreatedAt)
	if err != nil {
		// No matching copy is a normal outcome, not an error.
		return TerminalCopy{}, false, nil
	}
	return c, true, nil
}

// AddSourceRef records a directed source relationship. The pair
// (target, source, relation) is unique; a repeat records the newest
// referenced content instead of silently dropping it. Written in one
// transaction so concurrent supplements cannot duplicate or lose data.
func (r *Repository) AddSourceRef(ctx context.Context, username string, ref SourceRef) (SourceRef, error) {
	ref.Username = username
	ref.Relation = strings.TrimSpace(ref.Relation)
	if ref.Relation == "" {
		ref.Relation = "reviews"
	}
	ref.SourceTaskID = strings.TrimSpace(ref.SourceTaskID)
	ref.TargetTaskID = strings.TrimSpace(ref.TargetTaskID)
	if ref.TargetTaskID == "" || ref.SourceTaskID == "" {
		return SourceRef{}, fmt.Errorf("%w: target and source tasks are required", ErrValidation)
	}
	if ref.TargetTaskID == ref.SourceTaskID {
		return SourceRef{}, fmt.Errorf("%w: target and source must differ", ErrValidation)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return SourceRef{}, err
	}
	defer tx.Rollback()

	var existingID, existingSnapshot, existingCreated string
	err = tx.QueryRowContext(ctx,
		`SELECT id, snapshot, created_at FROM task_source_refs
		 WHERE username=? AND target_task_id=? AND source_task_id=? AND relation=?`,
		username, ref.TargetTaskID, ref.SourceTaskID, ref.Relation).
		Scan(&existingID, &existingSnapshot, &existingCreated)
	if err == nil {
		ref.ID = existingID
		ref.CreatedAt = existingCreated
		// A second selection from the same source replaces the stored
		// snapshot: the relationship points at what was referenced now.
		if strings.TrimSpace(ref.Snapshot) != "" && ref.Snapshot != existingSnapshot {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if _, err := tx.ExecContext(ctx,
				`UPDATE task_source_refs SET snapshot=?, source_terminal=?, target_terminal=?, copy_id=?, created_at=? WHERE id=?`,
				ref.Snapshot, ref.SourceTerminal, ref.TargetTerminal, ref.CopyID, now, existingID); err != nil {
				return SourceRef{}, err
			}
			ref.CreatedAt = now
		}
		return ref, tx.Commit()
	}
	ref.ID = newRandomID("ref")
	ref.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO task_source_refs(id, username, target_task_id, source_task_id, relation, snapshot, source_terminal, target_terminal, copy_id, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		ref.ID, username, ref.TargetTaskID, ref.SourceTaskID, ref.Relation,
		ref.Snapshot, ref.SourceTerminal, ref.TargetTerminal, ref.CopyID, ref.CreatedAt)
	if err != nil {
		return SourceRef{}, err
	}
	return ref, tx.Commit()
}

// ListSourceRefs returns the source relationships of a task (as target).
func (r *Repository) ListSourceRefs(ctx context.Context, username, taskID string) ([]SourceRef, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, target_task_id, source_task_id, relation, snapshot, source_terminal, target_terminal, copy_id, created_at
		 FROM task_source_refs WHERE username=? AND target_task_id=? ORDER BY created_at DESC`, username, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceRef
	for rows.Next() {
		var ref SourceRef
		if err := rows.Scan(&ref.ID, &ref.TargetTaskID, &ref.SourceTaskID, &ref.Relation,
			&ref.Snapshot, &ref.SourceTerminal, &ref.TargetTerminal, &ref.CopyID, &ref.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// ListSourceRefsForUser returns every source relationship of a user
// (board and graph projection).
func (r *Repository) ListSourceRefsForUser(ctx context.Context, username string) ([]SourceRef, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, target_task_id, source_task_id, relation, snapshot, source_terminal, target_terminal, copy_id, created_at
		 FROM task_source_refs WHERE username=? ORDER BY created_at DESC`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceRef
	for rows.Next() {
		var ref SourceRef
		if err := rows.Scan(&ref.ID, &ref.TargetTaskID, &ref.SourceTaskID, &ref.Relation,
			&ref.Snapshot, &ref.SourceTerminal, &ref.TargetTerminal, &ref.CopyID, &ref.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// DeleteSourceRef removes one source relationship by id, scoped to the user.
func (r *Repository) DeleteSourceRef(ctx context.Context, username, refID string) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM task_source_refs WHERE username=? AND id=?`, username, refID)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	return affected == 1, nil
}

// RecordPendingHandoff stores a matched copy→paste pair for a target
// terminal that has no task yet. Identical repeats are deduplicated (first
// row wins), and rows already promoted do not block a later paste of the
// same content from being recorded again.
func (r *Repository) RecordPendingHandoff(ctx context.Context, username string, h PendingHandoff) (PendingHandoff, error) {
	username = strings.TrimSpace(username)
	h.Username = username
	h.SourceTerminal = strings.TrimSpace(h.SourceTerminal)
	h.TargetTerminal = strings.TrimSpace(h.TargetTerminal)
	h.SourceTaskID = strings.TrimSpace(h.SourceTaskID)
	h.CopyID = strings.TrimSpace(h.CopyID)
	if username == "" || h.SourceTerminal == "" || h.TargetTerminal == "" {
		return PendingHandoff{}, fmt.Errorf("%w: username and terminals are required", ErrValidation)
	}
	if h.SourceTerminal == h.TargetTerminal {
		return PendingHandoff{}, fmt.Errorf("%w: source and target terminals must differ", ErrValidation)
	}
	h.Fingerprint = strings.TrimSpace(h.Fingerprint)
	if h.Fingerprint == "" {
		return PendingHandoff{}, fmt.Errorf("%w: fingerprint is required", ErrValidation)
	}
	h.ID = newRandomID("pend")
	h.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO pending_handoffs(id, username, source_task_id, source_terminal, target_terminal, fingerprint, snapshot, copy_id, intended_task_id, promoted_task_id, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		h.ID, username, h.SourceTaskID, h.SourceTerminal, h.TargetTerminal,
		h.Fingerprint, h.Snapshot, h.CopyID, h.IntendedTaskID, "", h.CreatedAt)
	if err != nil {
		return PendingHandoff{}, err
	}
	// Deduplication is by paste-event identity (the matched copy), not by
	// content: the same text copied from two different sources is two real
	// events and both survive. A retransmitted paste matches the same copy
	// record and collapses onto the first pending row.
	row := r.db.QueryRowContext(ctx,
		`SELECT id, source_task_id, source_terminal, snapshot, copy_id, created_at FROM pending_handoffs
		 WHERE username=? AND target_terminal=? AND copy_id=? AND promoted_task_id=''
		 ORDER BY created_at DESC LIMIT 1`, username, h.TargetTerminal, h.CopyID)
	var out PendingHandoff
	if err := row.Scan(&out.ID, &out.SourceTaskID, &out.SourceTerminal, &out.Snapshot, &out.CopyID, &out.CreatedAt); err != nil {
		return PendingHandoff{}, err
	}
	out.Username = username
	out.TargetTerminal = h.TargetTerminal
	out.Fingerprint = h.Fingerprint
	return out, nil
}

// RecordAgentExecution binds an agent process generation to a task.
// Identical (user, terminal, signature) repeats collapse onto the first
// row so concurrent pastes agree on the execution owner.
func (r *Repository) RecordAgentExecution(ctx context.Context, username string, e AgentExecution) (AgentExecution, error) {
	username = strings.TrimSpace(username)
	e.Username = username
	e.TerminalID = strings.TrimSpace(e.TerminalID)
	e.AgentSig = strings.TrimSpace(e.AgentSig)
	if username == "" || e.TerminalID == "" || e.AgentSig == "" {
		return AgentExecution{}, fmt.Errorf("%w: username, terminal and agent signature are required", ErrValidation)
	}
	e.ID = newRandomID("exec")
	e.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO terminal_agent_executions(id, username, terminal_id, agent_sig, agent_name, task_id, created_at)
		 VALUES(?,?,?,?,?,?,?)`,
		e.ID, username, e.TerminalID, e.AgentSig, e.AgentName, e.TaskID, e.CreatedAt)
	if err != nil {
		return AgentExecution{}, err
	}
	var out AgentExecution
	err = r.db.QueryRowContext(ctx,
		`SELECT id, terminal_id, agent_sig, agent_name, task_id, created_at FROM terminal_agent_executions
		 WHERE username=? AND terminal_id=? AND agent_sig=?`, username, e.TerminalID, e.AgentSig).
		Scan(&out.ID, &out.TerminalID, &out.AgentSig, &out.AgentName, &out.TaskID, &out.CreatedAt)
	if err != nil {
		return AgentExecution{}, err
	}
	out.Username = username
	return out, nil
}

// AgentExecutionBySig resolves the execution owning an agent generation.
func (r *Repository) AgentExecutionBySig(ctx context.Context, username, terminalID, agentSig string) (AgentExecution, bool, error) {
	var out AgentExecution
	err := r.db.QueryRowContext(ctx,
		`SELECT id, terminal_id, agent_sig, agent_name, task_id, created_at FROM terminal_agent_executions
		 WHERE username=? AND terminal_id=? AND agent_sig=?`,
		username, strings.TrimSpace(terminalID), strings.TrimSpace(agentSig)).
		Scan(&out.ID, &out.TerminalID, &out.AgentSig, &out.AgentName, &out.TaskID, &out.CreatedAt)
	if err != nil {
		return AgentExecution{}, false, nil
	}
	out.Username = username
	return out, true, nil
}

// ListPendingHandoffs returns the not-yet-promoted handoffs waiting on a
// target terminal, oldest first.
func (r *Repository) ListPendingHandoffs(ctx context.Context, username, targetTerminal string) ([]PendingHandoff, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, source_task_id, source_terminal, target_terminal, fingerprint, snapshot, copy_id, intended_task_id, created_at
		 FROM pending_handoffs
		 WHERE username=? AND target_terminal=? AND promoted_task_id='' ORDER BY created_at ASC`,
		username, strings.TrimSpace(targetTerminal))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingHandoff
	for rows.Next() {
		var h PendingHandoff
		if err := rows.Scan(&h.ID, &h.SourceTaskID, &h.SourceTerminal, &h.TargetTerminal,
			&h.Fingerprint, &h.Snapshot, &h.CopyID, &h.IntendedTaskID, &h.CreatedAt); err != nil {
			return nil, err
		}
		h.Username = username
		out = append(out, h)
	}
	return out, rows.Err()
}

// SetPendingIntendedTask pins unpromoted handoffs to one submission's task
// before promotion is attempted, so a failure is recoverable against the
// same submission and never leaks to a later goal.
func (r *Repository) SetPendingIntendedTask(ctx context.Context, username, targetTerminal, taskID string) error {
	// Only unpinned rows: a pending pinned to an earlier failed submission
	// keeps its owner, so recovery reattaches to the original task.
	_, err := r.db.ExecContext(ctx,
		`UPDATE pending_handoffs SET intended_task_id=? WHERE username=? AND target_terminal=? AND promoted_task_id='' AND intended_task_id=''`,
		strings.TrimSpace(taskID), username, strings.TrimSpace(targetTerminal))
	return err
}

// MarkHandoffPromoted records the target task a pending handoff became a
// source relationship of.
func (r *Repository) MarkHandoffPromoted(ctx context.Context, username, handoffID, taskID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE pending_handoffs SET promoted_task_id=? WHERE username=? AND id=? AND promoted_task_id=''`,
		strings.TrimSpace(taskID), username, strings.TrimSpace(handoffID))
	return err
}
