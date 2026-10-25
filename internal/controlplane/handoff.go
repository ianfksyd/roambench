// Terminal handoff: clipboard-driven cross-terminal task handoff.
//
// Copy in one agent terminal records the source (terminal, bound task,
// snapshot, fingerprint). Pasting the same content into another agent
// terminal links that target to the source. A review task card is created
// once per paste-match (idempotent per source+target pair), never for
// unmatched text, and never as an already-running task.
package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"crypto/rand"
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
// same user within the lookback window, resolving its source task.
func (r *Repository) MatchTerminalPaste(ctx context.Context, username, text string, lookback time.Duration) (TerminalCopy, bool, error) {
	fp := fingerprintText(text)
	cutoff := time.Now().UTC().Add(-lookback).Format(time.RFC3339Nano)
	row := r.db.QueryRowContext(ctx,
		`SELECT id, terminal_id, task_id, fingerprint, snapshot, created_at FROM terminal_copies
		 WHERE username=? AND fingerprint=? AND created_at>=?
		 ORDER BY created_at DESC LIMIT 1`, username, fp, cutoff)
	var c TerminalCopy
	err := row.Scan(&c.ID, &c.TerminalID, &c.TaskID, &c.Fingerprint, &c.Snapshot, &c.CreatedAt)
	if err != nil {
		// No matching copy is a normal outcome, not an error.
		return TerminalCopy{}, false, nil
	}
	return c, true, nil
}

// AddSourceRef records a directed source relationship, deduplicating on
// (target, source, relation).
func (r *Repository) AddSourceRef(ctx context.Context, username string, ref SourceRef) (SourceRef, error) {
	ref.Username = username
	ref.ID = newRandomID("ref")
	ref.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	ref.Relation = strings.TrimSpace(ref.Relation)
	if ref.Relation == "" {
		ref.Relation = "reviews"
	}
	if ref.TargetTaskID == "" || ref.SourceTaskID == "" {
		return SourceRef{}, fmt.Errorf("%w: target and source tasks are required", ErrValidation)
	}
	if ref.TargetTaskID == ref.SourceTaskID {
		return SourceRef{}, fmt.Errorf("%w: target and source must differ", ErrValidation)
	}
	var existing string
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM task_source_refs WHERE username=? AND target_task_id=? AND source_task_id=? AND relation=?`,
		username, ref.TargetTaskID, ref.SourceTaskID, ref.Relation).Scan(&existing)
	if err == nil {
		ref.ID = existing
		return ref, nil // deduplicated
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO task_source_refs(id, username, target_task_id, source_task_id, relation, snapshot, source_terminal, target_terminal, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		ref.ID, username, ref.TargetTaskID, ref.SourceTaskID, ref.Relation,
		ref.Snapshot, ref.SourceTerminal, ref.TargetTerminal, ref.CreatedAt)
	if err != nil {
		return SourceRef{}, err
	}
	return ref, nil
}

// ListSourceRefs returns the source relationships of a task (as target).
func (r *Repository) ListSourceRefs(ctx context.Context, username, taskID string) ([]SourceRef, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, target_task_id, source_task_id, relation, snapshot, source_terminal, target_terminal, created_at
		 FROM task_source_refs WHERE username=? AND target_task_id=? ORDER BY created_at DESC`, username, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceRef
	for rows.Next() {
		var ref SourceRef
		if err := rows.Scan(&ref.ID, &ref.TargetTaskID, &ref.SourceTaskID, &ref.Relation,
			&ref.Snapshot, &ref.SourceTerminal, &ref.TargetTerminal, &ref.CreatedAt); err != nil {
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
		`SELECT id, target_task_id, source_task_id, relation, snapshot, source_terminal, target_terminal, created_at
		 FROM task_source_refs WHERE username=? ORDER BY created_at DESC`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceRef
	for rows.Next() {
		var ref SourceRef
		if err := rows.Scan(&ref.ID, &ref.TargetTaskID, &ref.SourceTaskID, &ref.Relation,
			&ref.Snapshot, &ref.SourceTerminal, &ref.TargetTerminal, &ref.CreatedAt); err != nil {
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
