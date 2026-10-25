// Task progress feed: structured execution events reported by the local
// worker while a task runs. Powers the panel's "latest progress" card
// without parsing terminal output or calling a model.
package controlplane

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TaskProgress is one reported execution event.
type TaskProgress struct {
	Seq       int    `json:"seq"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}

// taskProgressLimit caps the stored events per task; older rows are pruned
// on every write.
const taskProgressLimit = 50

// AddTaskProgress stores one progress event for a task and prunes the feed
// to the most recent taskProgressLimit entries.
func (r *Repository) AddTaskProgress(ctx context.Context, username, taskID string, seq int, kind, text string) error {
	username = strings.TrimSpace(username)
	taskID = strings.TrimSpace(taskID)
	kind = strings.TrimSpace(kind)
	if username == "" || taskID == "" {
		return fmt.Errorf("%w: username and taskId are required", ErrValidation)
	}
	if seq < 1 {
		return fmt.Errorf("%w: seq must be >= 1", ErrValidation)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil // nothing to store; not an error for a heartbeat-style ping
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "raw"
	}
	if len(text) > 8192 {
		text = text[:8192]
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := clockNowUTC()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO task_progress(username, task_id, seq, kind, text, created_at) VALUES(?,?,?,?,?,?)
		 ON CONFLICT(username, task_id, seq) DO UPDATE SET kind=excluded.kind, text=excluded.text, created_at=excluded.created_at`,
		username, taskID, seq, kind, text, now); err != nil {
		return err
	}
	// Prune: keep only the newest taskProgressLimit rows for this task.
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_progress
		WHERE username=? AND task_id=? AND seq NOT IN (
			SELECT seq FROM task_progress WHERE username=? AND task_id=? ORDER BY seq DESC LIMIT ?
		)`, username, taskID, username, taskID, taskProgressLimit); err != nil {
		return err
	}
	return tx.Commit()
}

// ListTaskProgress returns the stored events of a task in descending seq
// order (newest first), optionally skipping everything newer than afterSeq.
func (r *Repository) ListTaskProgress(ctx context.Context, username, taskID string, afterSeq int) ([]TaskProgress, error) {
	username = strings.TrimSpace(username)
	taskID = strings.TrimSpace(taskID)
	if username == "" || taskID == "" {
		return nil, fmt.Errorf("%w: username and taskId are required", ErrValidation)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT seq, kind, text, created_at FROM task_progress
		 WHERE username=? AND task_id=? AND seq>? ORDER BY seq DESC LIMIT ?`,
		username, taskID, afterSeq, taskProgressLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskProgress
	for rows.Next() {
		var p TaskProgress
		if err := rows.Scan(&p.Seq, &p.Kind, &p.Text, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func clockNowUTC() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z") }

// ClearTaskProgress removes every stored event of a task (used when a new
// execution claims the task, so the panel never shows stale output).
func (r *Repository) ClearTaskProgress(ctx context.Context, username, taskID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM task_progress WHERE username=? AND task_id=?`, username, taskID)
	return err
}
