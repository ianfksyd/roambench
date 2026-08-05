package controlplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type LegacyCheckpoint struct {
	ID              string
	TaskID          string
	Kind            string
	Title           string
	Reason          string
	Status          string
	RequestedAt     string
	AllowedActions  []string
	DecisionSummary string
	RowVersion      int
	InputHash       string
}

type LegacyDecision struct {
	ID           string
	DecisionType string
	Actor        string
	Timestamp    string
	Summary      string
	TaskID       string
	CheckpointID string
}

type LegacyAuditEvent struct {
	ID           string
	Timestamp    string
	Actor        string
	Action       string
	Detail       string
	CheckpointID string
}

type LegacyApprovalSnapshot struct {
	Username        string
	SourceHash      string
	SourceUpdatedAt string
	Checkpoints     []LegacyCheckpoint
	Decisions       []LegacyDecision
	AuditEvents     []LegacyAuditEvent
}

func (r *Repository) ImportLegacyApprovals(ctx context.Context, snapshot LegacyApprovalSnapshot) error {
	username := strings.TrimSpace(snapshot.Username)
	if username == "" || strings.TrimSpace(snapshot.SourceHash) == "" {
		return fmt.Errorf("%w: migration username and source hash are required", ErrValidation)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS legacy_approval_migrations (
		username TEXT PRIMARY KEY, source_hash TEXT NOT NULL, source_updated_at TEXT NOT NULL,
		checkpoint_count INTEGER NOT NULL, decision_count INTEGER NOT NULL, applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}

	for _, checkpoint := range snapshot.Checkpoints {
		if err := importLegacyCheckpoint(ctx, tx, username, checkpoint); err != nil {
			return err
		}
	}
	for _, decision := range snapshot.Decisions {
		if err := importLegacyDecision(ctx, tx, username, decision); err != nil {
			return err
		}
	}
	for _, event := range snapshot.AuditEvents {
		if strings.TrimSpace(event.CheckpointID) == "" {
			continue
		}
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO audit_events(event_id,username,request_id,event_type,actor,detail,created_at) VALUES(?,?,?,?,?,?,?)`,
			"legacy-"+event.ID, username, event.CheckpointID, event.Action, event.Actor, event.Detail, normalizedLegacyTime(event.Timestamp))
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO legacy_approval_migrations(username,source_hash,source_updated_at,checkpoint_count,decision_count,applied_at)
		VALUES(?,?,?,?,?,?) ON CONFLICT(username) DO UPDATE SET source_hash=excluded.source_hash,source_updated_at=excluded.source_updated_at,
		checkpoint_count=excluded.checkpoint_count,decision_count=excluded.decision_count`, username, snapshot.SourceHash, snapshot.SourceUpdatedAt,
		len(snapshot.Checkpoints), len(snapshot.Decisions), formatTime(time.Now().UTC()))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func importLegacyCheckpoint(ctx context.Context, tx *sql.Tx, username string, checkpoint LegacyCheckpoint) error {
	status, finalAction := legacyInteractionStatus(checkpoint.Status)
	rowVersion := checkpoint.RowVersion
	if rowVersion < 1 {
		rowVersion = 1
	}
	actions, _ := json.Marshal(normalizedStrings(checkpoint.AllowedActions))
	schemaPayload, _ := json.Marshal(ResponseSchema{Type: "action", MaxFeedbackLength: 500})
	created := normalizedLegacyTime(checkpoint.RequestedAt)
	insertResult, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO interactions(
		request_id,username,checkpoint_id,task_id,runtime_id,session_id,adapter_kind,vendor_request_id,request_kind,risk_class,title,summary,preview,
		artifact_refs,allowed_actions,response_schema,ui_hints,status,final_action,created_at,resolved_at,row_version,input_hash
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		checkpoint.ID, username, checkpoint.ID, checkpoint.TaskID, "project-control", "project-control", "project-control", checkpoint.ID,
		checkpoint.Kind, "R1", checkpoint.Title, checkpoint.Reason, checkpoint.Reason, "[]", string(actions), string(schemaPayload), "{}",
		status, finalAction, created, nullableLegacyResolved(status, created), rowVersion, checkpoint.InputHash)
	if err != nil {
		return err
	}
	inserted, _ := insertResult.RowsAffected()
	if inserted == 1 {
		payload, _ := json.Marshal(map[string]interface{}{"requestId": checkpoint.ID, "migrated": true})
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO outbox_events(event_id,username,aggregate_id,event_type,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
			"legacy-created-"+checkpoint.ID, username, checkpoint.ID, "interaction.created", string(payload), created, created)
		return err
	}

	var currentStatus string
	var currentRowVersion int
	if err := tx.QueryRowContext(ctx, `SELECT status,row_version FROM interactions WHERE username=? AND request_id=?`, username, checkpoint.ID).Scan(&currentStatus, &currentRowVersion); err != nil {
		return err
	}
	shouldUpdate := rowVersion > currentRowVersion || (currentStatus == "pending" && status != "pending" && rowVersion >= currentRowVersion)
	if !shouldUpdate || (currentStatus != "pending" && status == "pending") {
		return nil
	}
	updateResult, err := tx.ExecContext(ctx, `UPDATE interactions SET task_id=?,request_kind=?,title=?,summary=?,preview=?,allowed_actions=?,status=?,final_action=?,resolved_at=?,row_version=?,input_hash=?
		WHERE username=? AND request_id=? AND status=? AND row_version=?`, checkpoint.TaskID, checkpoint.Kind, checkpoint.Title,
		checkpoint.Reason, checkpoint.Reason, string(actions), status, finalAction, nullableLegacyResolved(status, created), rowVersion,
		checkpoint.InputHash, username, checkpoint.ID, currentStatus, currentRowVersion)
	if err != nil {
		return err
	}
	updated, _ := updateResult.RowsAffected()
	if updated != 1 {
		return nil
	}
	eventType := "interaction.updated"
	if status == "expired" {
		eventType = "interaction.expired"
	} else if status == "cancelled" {
		eventType = "interaction.cancelled"
	}
	payload, _ := json.Marshal(map[string]interface{}{"requestId": checkpoint.ID, "status": status, "rowVersion": rowVersion, "migrated": true})
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO outbox_events(event_id,username,aggregate_id,event_type,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		fmt.Sprintf("legacy-updated-%s-%d", checkpoint.ID, rowVersion), username, checkpoint.ID, eventType, string(payload), created, created)
	return err
}

func importLegacyDecision(ctx context.Context, tx *sql.Tx, username string, decision LegacyDecision) error {
	action := legacyDecisionAction(decision.DecisionType)
	created := normalizedLegacyTime(decision.Timestamp)
	responseID := decision.ID
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO responses(response_id,request_id,username,checkpoint_id,action,selected_option_ids,feedback,actor,device_id,auth_evidence,expected_row_version,idempotency_key,input_hash,created_at)
		SELECT ?,request_id,username,checkpoint_id,?,'[]',?,?, 'migration','legacy-import',row_version-1,?,input_hash,? FROM interactions WHERE username=? AND request_id=?`,
		responseID, action, decision.Summary, decision.Actor, "legacy-"+decision.ID, created, username, decision.CheckpointID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO decisions(decision_id,request_id,response_id,action,actor,input_hash,created_at)
		SELECT ?,request_id,?,?,?,input_hash,? FROM interactions WHERE username=? AND request_id=?`,
		decision.ID, responseID, action, decision.Actor, created, username, decision.CheckpointID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE interactions SET status='resolved',final_action=?,resolved_at=? WHERE username=? AND request_id=?`, action, created, username, decision.CheckpointID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO task_projections(decision_id,username,request_id,response_id,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, decision.ID, username, decision.CheckpointID, responseID, "applied", created, created)
	return err
}

func legacyInteractionStatus(status string) (string, string) {
	switch strings.TrimSpace(status) {
	case "approved":
		return "resolved", "approve"
	case "rejected":
		return "resolved", "reject"
	case "rerouted":
		return "resolved", "reroute"
	case "expired":
		return "expired", "expire"
	case "cancelled":
		return "cancelled", "cancel"
	default:
		return "pending", ""
	}
}

func legacyDecisionAction(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(value, "approved"):
		return "approve"
	case strings.Contains(value, "rejected"):
		return "reject"
	case strings.Contains(value, "rerouted"):
		return "reroute"
	default:
		return value
	}
}

func normalizedLegacyTime(value string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value)); err == nil {
		return formatTime(parsed)
	}
	return formatTime(time.Now().UTC())
}

func nullableLegacyResolved(status, created string) interface{} {
	if status == "pending" {
		return nil
	}
	return created
}
