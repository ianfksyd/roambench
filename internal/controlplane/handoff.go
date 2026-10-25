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
	PromotedTaskID string `json:"promotedTaskId,omitempty"`
	CreatedAt      string `json:"createdAt"`
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
		`INSERT OR IGNORE INTO pending_handoffs(id, username, source_task_id, source_terminal, target_terminal, fingerprint, snapshot, copy_id, promoted_task_id, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		h.ID, username, h.SourceTaskID, h.SourceTerminal, h.TargetTerminal,
		h.Fingerprint, h.Snapshot, h.CopyID, "", h.CreatedAt)
	if err != nil {
		return PendingHandoff{}, err
	}
	// Concurrent duplicates keep the first row for the same target+content.
	row := r.db.QueryRowContext(ctx,
		`SELECT id, source_task_id, snapshot, copy_id, created_at FROM pending_handoffs
		 WHERE username=? AND target_terminal=? AND fingerprint=? AND promoted_task_id=''
		 ORDER BY created_at DESC LIMIT 1`, username, h.TargetTerminal, h.Fingerprint)
	var out PendingHandoff
	if err := row.Scan(&out.ID, &out.SourceTaskID, &out.Snapshot, &out.CopyID, &out.CreatedAt); err != nil {
		return PendingHandoff{}, err
	}
	out.Username = username
	out.SourceTerminal = h.SourceTerminal
	out.TargetTerminal = h.TargetTerminal
	out.Fingerprint = h.Fingerprint
	return out, nil
}

// ListPendingHandoffs returns the not-yet-promoted handoffs waiting on a
// target terminal, oldest first.
func (r *Repository) ListPendingHandoffs(ctx context.Context, username, targetTerminal string) ([]PendingHandoff, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, source_task_id, source_terminal, target_terminal, fingerprint, snapshot, copy_id, created_at
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
			&h.Fingerprint, &h.Snapshot, &h.CopyID, &h.CreatedAt); err != nil {
			return nil, err
		}
		h.Username = username
		out = append(out, h)
	}
	return out, rows.Err()
}

// MarkHandoffPromoted records the target task a pending handoff became a
// source relationship of.
func (r *Repository) MarkHandoffPromoted(ctx context.Context, username, handoffID, taskID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE pending_handoffs SET promoted_task_id=? WHERE username=? AND id=? AND promoted_task_id=''`,
		strings.TrimSpace(taskID), username, strings.TrimSpace(handoffID))
	return err
}
