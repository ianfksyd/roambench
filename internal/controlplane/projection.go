package controlplane

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type TaskProjection struct {
	DecisionID   string    `json:"decisionId"`
	RequestID    string    `json:"requestId"`
	ResponseID   string    `json:"responseId"`
	State        string    `json:"state"`
	AttemptCount int       `json:"attemptCount"`
	LastError    string    `json:"lastError,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (r *Repository) ListPendingTaskProjections(ctx context.Context, username string, limit int) ([]TaskProjection, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT decision_id,request_id,response_id,state,attempt_count,last_error,created_at,updated_at
		FROM task_projections WHERE username=? AND state IN ('pending','failed') ORDER BY created_at LIMIT ?`, strings.TrimSpace(username), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskProjection
	for rows.Next() {
		var item TaskProjection
		var created, updated string
		if err := rows.Scan(&item.DecisionID, &item.RequestID, &item.ResponseID, &item.State, &item.AttemptCount, &item.LastError, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(created)
		item.UpdatedAt = parseTime(updated)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) GetResponseByID(ctx context.Context, username, responseID string) (Response, error) {
	return scanResponse(r.db.QueryRowContext(ctx, responseSelect+` WHERE username=? AND response_id=?`, strings.TrimSpace(username), strings.TrimSpace(responseID)))
}

func (r *Repository) MarkTaskProjectionApplied(ctx context.Context, username, decisionID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE task_projections SET state='applied',attempt_count=attempt_count+1,last_error='',updated_at=? WHERE username=? AND decision_id=? AND state IN ('pending','failed')`, formatTime(time.Now().UTC()), strings.TrimSpace(username), strings.TrimSpace(decisionID))
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("%w: task projection is not pending", ErrConflict)
	}
	return nil
}

func (r *Repository) MarkTaskProjectionFailed(ctx context.Context, username, decisionID string, projectionErr error) error {
	message := "projection failed"
	if projectionErr != nil {
		message = projectionErr.Error()
	}
	_, err := r.db.ExecContext(ctx, `UPDATE task_projections SET state='failed',attempt_count=attempt_count+1,last_error=?,updated_at=? WHERE username=? AND decision_id=? AND state IN ('pending','failed')`, message, formatTime(time.Now().UTC()), strings.TrimSpace(username), strings.TrimSpace(decisionID))
	return err
}
