package controlplane

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound   = errors.New("control-plane record not found")
	ErrConflict   = errors.New("control-plane conflict")
	ErrValidation = errors.New("control-plane validation failed")
)

type ResponseOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type ResponseSchema struct {
	Type              string           `json:"type"`
	Options           []ResponseOption `json:"options,omitempty"`
	MinSelections     int              `json:"minSelections,omitempty"`
	MaxSelections     int              `json:"maxSelections,omitempty"`
	MaxFeedbackLength int              `json:"maxFeedbackLength,omitempty"`
	AllowCustomText   bool             `json:"allowCustomText,omitempty"`
}

type CreateInteraction struct {
	Username        string
	CheckpointID    string
	TaskID          string
	RuntimeID       string
	SessionID       string
	AdapterKind     string
	VendorRequestID string
	RequestKind     string
	RiskClass       string
	Title           string
	Summary         string
	Preview         string
	ArtifactRefs    []string
	AllowedActions  []string
	ResponseSchema  ResponseSchema
	UIHints         map[string]interface{}
	ExpiresAt       time.Time
	InputHash       string
}

type Interaction struct {
	RequestID       string                 `json:"requestId"`
	CheckpointID    string                 `json:"checkpointId,omitempty"`
	TaskID          string                 `json:"taskId,omitempty"`
	RuntimeID       string                 `json:"runtimeId,omitempty"`
	SessionID       string                 `json:"sessionId,omitempty"`
	AdapterKind     string                 `json:"adapterKind"`
	VendorRequestID string                 `json:"vendorRequestId"`
	RequestKind     string                 `json:"requestKind"`
	RiskClass       string                 `json:"riskClass"`
	Title           string                 `json:"title"`
	Summary         string                 `json:"summary"`
	Preview         string                 `json:"preview"`
	ArtifactRefs    []string               `json:"artifactRefs"`
	AllowedActions  []string               `json:"allowedActions"`
	ResponseSchema  ResponseSchema         `json:"responseSchema"`
	UIHints         map[string]interface{} `json:"uiHints,omitempty"`
	Status          string                 `json:"status"`
	FinalAction     string                 `json:"finalAction,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
	ExpiresAt       time.Time              `json:"expiresAt,omitempty"`
	ResolvedAt      time.Time              `json:"resolvedAt,omitempty"`
	RowVersion      int                    `json:"rowVersion"`
	InputHash       string                 `json:"inputHash"`
}

type RespondInput struct {
	Action             string   `json:"action"`
	SelectedOptionIDs  []string `json:"selectedOptionIds"`
	Feedback           string   `json:"feedback"`
	Actor              string   `json:"actor"`
	DeviceID           string   `json:"deviceId"`
	AuthEvidence       string   `json:"authEvidence"`
	ExpectedRowVersion int      `json:"expectedRowVersion"`
	IdempotencyKey     string   `json:"idempotencyKey"`
	InputHash          string   `json:"inputHash"`
}

type Response struct {
	ResponseID         string    `json:"responseId"`
	RequestID          string    `json:"requestId"`
	CheckpointID       string    `json:"checkpointId,omitempty"`
	Action             string    `json:"action"`
	SelectedOptionIDs  []string  `json:"selectedOptionIds"`
	Feedback           string    `json:"feedback,omitempty"`
	Actor              string    `json:"actor"`
	DeviceID           string    `json:"deviceId,omitempty"`
	AuthEvidence       string    `json:"authEvidence,omitempty"`
	ExpectedRowVersion int       `json:"expectedRowVersion"`
	IdempotencyKey     string    `json:"idempotencyKey"`
	InputHash          string    `json:"inputHash"`
	CreatedAt          time.Time `json:"createdAt"`
}

type OutboxEvent struct {
	EventID     string          `json:"eventId"`
	EventType   string          `json:"eventType"`
	AggregateID string          `json:"aggregateId"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type Repository struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS interactions (
  request_id TEXT PRIMARY KEY,
  username TEXT NOT NULL,
  checkpoint_id TEXT,
  task_id TEXT,
  runtime_id TEXT,
  session_id TEXT NOT NULL,
  adapter_kind TEXT NOT NULL,
  vendor_request_id TEXT NOT NULL,
  request_kind TEXT NOT NULL,
  risk_class TEXT NOT NULL,
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  preview TEXT NOT NULL,
  artifact_refs TEXT NOT NULL,
  allowed_actions TEXT NOT NULL,
  response_schema TEXT NOT NULL,
  ui_hints TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('pending','resolved','expired','cancelled')),
  final_action TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  expires_at TEXT,
  resolved_at TEXT,
  row_version INTEGER NOT NULL CHECK(row_version >= 1),
  input_hash TEXT NOT NULL,
  UNIQUE(username, adapter_kind, session_id, vendor_request_id),
  UNIQUE(username, checkpoint_id)
);
CREATE INDEX IF NOT EXISTS interactions_pending_idx ON interactions(username, status, created_at);
CREATE TABLE IF NOT EXISTS responses (
  response_id TEXT PRIMARY KEY,
  request_id TEXT NOT NULL UNIQUE REFERENCES interactions(request_id) ON DELETE CASCADE,
  username TEXT NOT NULL,
  checkpoint_id TEXT,
  action TEXT NOT NULL,
  selected_option_ids TEXT NOT NULL,
  feedback TEXT NOT NULL,
  actor TEXT NOT NULL,
  device_id TEXT NOT NULL,
  auth_evidence TEXT NOT NULL,
  expected_row_version INTEGER NOT NULL,
  idempotency_key TEXT NOT NULL,
  input_hash TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS decisions (
  decision_id TEXT PRIMARY KEY,
  request_id TEXT NOT NULL UNIQUE REFERENCES interactions(request_id) ON DELETE CASCADE,
  response_id TEXT NOT NULL UNIQUE REFERENCES responses(response_id) ON DELETE CASCADE,
  action TEXT NOT NULL,
  actor TEXT NOT NULL,
  input_hash TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS audit_events (
  event_id TEXT PRIMARY KEY,
  username TEXT NOT NULL,
  request_id TEXT NOT NULL REFERENCES interactions(request_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  actor TEXT NOT NULL,
  detail TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS outbox_events (
  event_id TEXT PRIMARY KEY,
  username TEXT NOT NULL,
  aggregate_id TEXT NOT NULL REFERENCES interactions(request_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  payload TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'pending',
  attempt_count INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TEXT,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS outbox_pending_idx ON outbox_events(state, next_attempt_at, created_at);
CREATE TABLE IF NOT EXISTS deliveries (
  delivery_id TEXT PRIMARY KEY,
  request_id TEXT NOT NULL REFERENCES interactions(request_id) ON DELETE CASCADE,
  outbox_event_id TEXT REFERENCES outbox_events(event_id) ON DELETE SET NULL,
  channel TEXT NOT NULL,
  device_id TEXT NOT NULL,
  state TEXT NOT NULL,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TEXT,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS idempotency_keys (
  username TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  response_id TEXT NOT NULL REFERENCES responses(response_id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY(username, actor_scope, idempotency_key)
);
CREATE TABLE IF NOT EXISTS task_projections (
  decision_id TEXT PRIMARY KEY,
  username TEXT NOT NULL,
  request_id TEXT NOT NULL REFERENCES interactions(request_id) ON DELETE CASCADE,
  response_id TEXT NOT NULL UNIQUE REFERENCES responses(response_id) ON DELETE CASCADE,
  state TEXT NOT NULL CHECK(state IN ('pending','applied','failed')),
  attempt_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS task_projections_pending_idx ON task_projections(username,state,created_at);
INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, CURRENT_TIMESTAMP);
`

func Open(path string) (*Repository, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%w: database path is required", ErrValidation)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = FULL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		db.Close()
		return nil, err
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *Repository) CreateInteraction(ctx context.Context, input CreateInteraction) (Interaction, error) {
	if err := validateCreate(input); err != nil {
		return Interaction{}, err
	}
	now := time.Now().UTC()
	interaction := Interaction{
		RequestID: newID("interaction"), CheckpointID: strings.TrimSpace(input.CheckpointID),
		TaskID: strings.TrimSpace(input.TaskID), RuntimeID: strings.TrimSpace(input.RuntimeID), SessionID: strings.TrimSpace(input.SessionID),
		AdapterKind: strings.TrimSpace(input.AdapterKind), VendorRequestID: strings.TrimSpace(input.VendorRequestID),
		RequestKind: strings.TrimSpace(input.RequestKind), RiskClass: strings.TrimSpace(input.RiskClass),
		Title: strings.TrimSpace(input.Title), Summary: strings.TrimSpace(input.Summary), Preview: strings.TrimSpace(input.Preview),
		ArtifactRefs: append([]string(nil), input.ArtifactRefs...), AllowedActions: normalizedStrings(input.AllowedActions),
		ResponseSchema: input.ResponseSchema, UIHints: input.UIHints, Status: "pending", CreatedAt: now, ExpiresAt: input.ExpiresAt.UTC(),
		RowVersion: 1, InputHash: strings.TrimSpace(input.InputHash),
	}
	artifactJSON, _ := json.Marshal(interaction.ArtifactRefs)
	actionsJSON, _ := json.Marshal(interaction.AllowedActions)
	schemaJSON, _ := json.Marshal(interaction.ResponseSchema)
	hintsJSON, _ := json.Marshal(interaction.UIHints)
	expiresAt := nullableTime(interaction.ExpiresAt)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Interaction{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO interactions(
		request_id,username,checkpoint_id,task_id,runtime_id,session_id,adapter_kind,vendor_request_id,request_kind,risk_class,title,summary,preview,
		artifact_refs,allowed_actions,response_schema,ui_hints,status,created_at,expires_at,row_version,input_hash
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		interaction.RequestID, strings.TrimSpace(input.Username), nullString(interaction.CheckpointID), interaction.TaskID, interaction.RuntimeID,
		interaction.SessionID, interaction.AdapterKind, interaction.VendorRequestID, interaction.RequestKind, interaction.RiskClass,
		interaction.Title, interaction.Summary, interaction.Preview, string(artifactJSON), string(actionsJSON), string(schemaJSON), string(hintsJSON),
		interaction.Status, formatTime(now), expiresAt, interaction.RowVersion, interaction.InputHash)
	if err != nil {
		if existing, getErr := getByVendor(ctx, tx, strings.TrimSpace(input.Username), interaction.AdapterKind, interaction.SessionID, interaction.VendorRequestID); getErr == nil && existing.InputHash == interaction.InputHash {
			return existing, nil
		}
		return Interaction{}, fmt.Errorf("%w: duplicate or invalid interaction: %v", ErrConflict, err)
	}
	if err := insertAudit(ctx, tx, strings.TrimSpace(input.Username), interaction.RequestID, "interaction.created", "adapter", interaction.Summary, now); err != nil {
		return Interaction{}, err
	}
	if err := insertOutbox(ctx, tx, strings.TrimSpace(input.Username), interaction.RequestID, "interaction.created", interaction, now); err != nil {
		return Interaction{}, err
	}
	if err := tx.Commit(); err != nil {
		return Interaction{}, err
	}
	return interaction, nil
}

func (r *Repository) GetInteraction(ctx context.Context, username, requestID string) (Interaction, error) {
	row := r.db.QueryRowContext(ctx, interactionSelect+` WHERE username=? AND request_id=?`, strings.TrimSpace(username), strings.TrimSpace(requestID))
	return scanInteraction(row)
}

func (r *Repository) ListPendingInteractions(ctx context.Context, username string, limit int) ([]Interaction, error) {
	return r.listInteractions(ctx, username, "pending", limit)
}

func (r *Repository) ListInteractions(ctx context.Context, username string, limit int) ([]Interaction, error) {
	return r.listInteractions(ctx, username, "", limit)
}

func (r *Repository) listInteractions(ctx context.Context, username, status string, limit int) ([]Interaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := interactionSelect + ` WHERE username=?`
	args := []interface{}{strings.TrimSpace(username)}
	if status != "" {
		query += ` AND status=?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Interaction
	for rows.Next() {
		item, err := scanInteraction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) Respond(ctx context.Context, username, requestID string, input RespondInput) (Response, bool, error) {
	username = strings.TrimSpace(username)
	requestID = strings.TrimSpace(requestID)
	input.Action = strings.TrimSpace(input.Action)
	input.Actor = strings.TrimSpace(input.Actor)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Actor == "" || input.IdempotencyKey == "" || input.ExpectedRowVersion < 1 {
		return Response{}, false, fmt.Errorf("%w: actor, idempotencyKey and expectedRowVersion are required", ErrValidation)
	}
	actorScope := input.Actor + ":" + input.DeviceID
	requestHash := hashResponseInput(requestID, input)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Response{}, false, err
	}
	defer tx.Rollback()

	var storedHash, storedResponseID string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,response_id FROM idempotency_keys WHERE username=? AND actor_scope=? AND idempotency_key=?`, username, actorScope, input.IdempotencyKey).Scan(&storedHash, &storedResponseID)
	if err == nil {
		if storedHash != requestHash {
			return Response{}, false, fmt.Errorf("%w: idempotency key reused with different input", ErrConflict)
		}
		response, err := getResponse(ctx, tx, username, storedResponseID)
		return response, true, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Response{}, false, err
	}

	interaction, err := getInteractionTx(ctx, tx, username, requestID)
	if err != nil {
		return Response{}, false, err
	}
	if interaction.Status != "pending" || interaction.RowVersion != input.ExpectedRowVersion {
		return Response{}, false, fmt.Errorf("%w: interaction is %s at row version %d", ErrConflict, interaction.Status, interaction.RowVersion)
	}
	if input.InputHash != interaction.InputHash {
		return Response{}, false, fmt.Errorf("%w: reviewed input changed", ErrConflict)
	}
	if err := validateResponse(interaction, input); err != nil {
		return Response{}, false, err
	}

	now := time.Now().UTC()
	response := Response{
		ResponseID: newID("response"), RequestID: requestID, CheckpointID: interaction.CheckpointID,
		Action: input.Action, SelectedOptionIDs: append([]string(nil), input.SelectedOptionIDs...), Feedback: input.Feedback,
		Actor: input.Actor, DeviceID: input.DeviceID, AuthEvidence: input.AuthEvidence,
		ExpectedRowVersion: input.ExpectedRowVersion, IdempotencyKey: input.IdempotencyKey, InputHash: input.InputHash, CreatedAt: now,
	}
	selectedJSON, _ := json.Marshal(response.SelectedOptionIDs)
	_, err = tx.ExecContext(ctx, `INSERT INTO responses(response_id,request_id,username,checkpoint_id,action,selected_option_ids,feedback,actor,device_id,auth_evidence,expected_row_version,idempotency_key,input_hash,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		response.ResponseID, requestID, username, nullString(response.CheckpointID), response.Action, string(selectedJSON), response.Feedback,
		response.Actor, response.DeviceID, response.AuthEvidence, response.ExpectedRowVersion, response.IdempotencyKey, response.InputHash, formatTime(now))
	if err != nil {
		return Response{}, false, fmt.Errorf("%w: interaction already has a final response", ErrConflict)
	}
	decisionID := newID("decision")
	if _, err := tx.ExecContext(ctx, `INSERT INTO decisions(decision_id,request_id,response_id,action,actor,input_hash,created_at) VALUES(?,?,?,?,?,?,?)`, decisionID, requestID, response.ResponseID, response.Action, response.Actor, response.InputHash, formatTime(now)); err != nil {
		return Response{}, false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE interactions SET status='resolved',final_action=?,resolved_at=?,row_version=row_version+1 WHERE username=? AND request_id=? AND status='pending' AND row_version=?`, response.Action, formatTime(now), username, requestID, input.ExpectedRowVersion)
	if err != nil {
		return Response{}, false, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return Response{}, false, fmt.Errorf("%w: interaction changed while responding", ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO idempotency_keys(username,actor_scope,idempotency_key,request_hash,response_id,created_at) VALUES(?,?,?,?,?,?)`, username, actorScope, input.IdempotencyKey, requestHash, response.ResponseID, formatTime(now)); err != nil {
		return Response{}, false, fmt.Errorf("%w: idempotency key already used", ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO task_projections(decision_id,username,request_id,response_id,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, response.ResponseID, username, requestID, response.ResponseID, "pending", formatTime(now), formatTime(now)); err != nil {
		return Response{}, false, err
	}
	if err := insertAudit(ctx, tx, username, requestID, "interaction.resolved", response.Actor, response.Action, now); err != nil {
		return Response{}, false, err
	}
	if err := insertOutbox(ctx, tx, username, requestID, "interaction.resolved", response, now); err != nil {
		return Response{}, false, err
	}
	if err := insertOutbox(ctx, tx, username, requestID, "adapter.decision.ready", response, now); err != nil {
		return Response{}, false, err
	}
	if err := insertOutbox(ctx, tx, username, requestID, "project_task_projection.requested", map[string]string{"decisionId": response.ResponseID, "responseId": response.ResponseID}, now); err != nil {
		return Response{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Response{}, false, err
	}
	return response, false, nil
}

func (r *Repository) CancelInteraction(ctx context.Context, username, requestID string, expectedRowVersion int, reason string) (Interaction, error) {
	if expectedRowVersion < 1 {
		return Interaction{}, fmt.Errorf("%w: expected row version is required", ErrValidation)
	}
	now := time.Now().UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Interaction{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE interactions SET status='cancelled',final_action='cancel',resolved_at=?,row_version=row_version+1 WHERE username=? AND request_id=? AND status='pending' AND row_version=?`, formatTime(now), strings.TrimSpace(username), strings.TrimSpace(requestID), expectedRowVersion)
	if err != nil {
		return Interaction{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return Interaction{}, fmt.Errorf("%w: interaction is not pending at expected version", ErrConflict)
	}
	if err := insertAudit(ctx, tx, strings.TrimSpace(username), strings.TrimSpace(requestID), "interaction.cancelled", "adapter", strings.TrimSpace(reason), now); err != nil {
		return Interaction{}, err
	}
	if err := insertOutbox(ctx, tx, strings.TrimSpace(username), strings.TrimSpace(requestID), "interaction.cancelled", map[string]string{"reason": strings.TrimSpace(reason)}, now); err != nil {
		return Interaction{}, err
	}
	if err := tx.Commit(); err != nil {
		return Interaction{}, err
	}
	return r.GetInteraction(ctx, username, requestID)
}

func (r *Repository) WaitInteraction(ctx context.Context, username, requestID string, timeout time.Duration) (Interaction, error) {
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		interaction, err := r.GetInteraction(ctx, username, requestID)
		if err != nil {
			return Interaction{}, err
		}
		if interaction.Status != "pending" {
			return interaction, nil
		}
		select {
		case <-ctx.Done():
			return Interaction{}, ctx.Err()
		case <-deadline.C:
			return interaction, nil
		case <-ticker.C:
		}
	}
}

func (r *Repository) ListResponses(ctx context.Context, username, requestID string) ([]Response, error) {
	rows, err := r.db.QueryContext(ctx, responseSelect+` WHERE username=? AND request_id=? ORDER BY created_at`, strings.TrimSpace(username), strings.TrimSpace(requestID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Response
	for rows.Next() {
		response, err := scanResponse(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, response)
	}
	return out, rows.Err()
}

func (r *Repository) ListOutbox(ctx context.Context, username string, limit int) ([]OutboxEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT event_id,event_type,aggregate_id,payload,created_at FROM outbox_events WHERE username=? ORDER BY created_at,event_id LIMIT ?`, strings.TrimSpace(username), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxEvent
	for rows.Next() {
		var item OutboxEvent
		var payload, created string
		if err := rows.Scan(&item.EventID, &item.EventType, &item.AggregateID, &payload, &created); err != nil {
			return nil, err
		}
		item.Payload = json.RawMessage(payload)
		item.CreatedAt = parseTime(created)
		out = append(out, item)
	}
	return out, rows.Err()
}

const interactionSelect = `SELECT request_id,checkpoint_id,task_id,runtime_id,session_id,adapter_kind,vendor_request_id,request_kind,risk_class,title,summary,preview,artifact_refs,allowed_actions,response_schema,ui_hints,status,final_action,created_at,expires_at,resolved_at,row_version,input_hash FROM interactions`

type scanner interface{ Scan(...interface{}) error }

func scanInteraction(row scanner) (Interaction, error) {
	var item Interaction
	var checkpoint sql.NullString
	var artifactJSON, actionsJSON, schemaJSON, hintsJSON string
	var created string
	var expires, resolved sql.NullString
	if err := row.Scan(&item.RequestID, &checkpoint, &item.TaskID, &item.RuntimeID, &item.SessionID, &item.AdapterKind, &item.VendorRequestID,
		&item.RequestKind, &item.RiskClass, &item.Title, &item.Summary, &item.Preview, &artifactJSON, &actionsJSON, &schemaJSON, &hintsJSON,
		&item.Status, &item.FinalAction, &created, &expires, &resolved, &item.RowVersion, &item.InputHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Interaction{}, ErrNotFound
		}
		return Interaction{}, err
	}
	item.CheckpointID = checkpoint.String
	item.CreatedAt = parseTime(created)
	item.ExpiresAt = parseTime(expires.String)
	item.ResolvedAt = parseTime(resolved.String)
	_ = json.Unmarshal([]byte(artifactJSON), &item.ArtifactRefs)
	_ = json.Unmarshal([]byte(actionsJSON), &item.AllowedActions)
	_ = json.Unmarshal([]byte(schemaJSON), &item.ResponseSchema)
	_ = json.Unmarshal([]byte(hintsJSON), &item.UIHints)
	return item, nil
}

func getInteractionTx(ctx context.Context, tx *sql.Tx, username, requestID string) (Interaction, error) {
	return scanInteraction(tx.QueryRowContext(ctx, interactionSelect+` WHERE username=? AND request_id=?`, username, requestID))
}

func getByVendor(ctx context.Context, tx *sql.Tx, username, adapter, session, vendorID string) (Interaction, error) {
	return scanInteraction(tx.QueryRowContext(ctx, interactionSelect+` WHERE username=? AND adapter_kind=? AND session_id=? AND vendor_request_id=?`, username, adapter, session, vendorID))
}

const responseSelect = `SELECT response_id,request_id,checkpoint_id,action,selected_option_ids,feedback,actor,device_id,auth_evidence,expected_row_version,idempotency_key,input_hash,created_at FROM responses`

func scanResponse(row scanner) (Response, error) {
	var response Response
	var checkpoint sql.NullString
	var selectedJSON, created string
	if err := row.Scan(&response.ResponseID, &response.RequestID, &checkpoint, &response.Action, &selectedJSON, &response.Feedback,
		&response.Actor, &response.DeviceID, &response.AuthEvidence, &response.ExpectedRowVersion, &response.IdempotencyKey, &response.InputHash, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Response{}, ErrNotFound
		}
		return Response{}, err
	}
	response.CheckpointID = checkpoint.String
	response.CreatedAt = parseTime(created)
	_ = json.Unmarshal([]byte(selectedJSON), &response.SelectedOptionIDs)
	return response, nil
}

func getResponse(ctx context.Context, tx *sql.Tx, username, responseID string) (Response, error) {
	return scanResponse(tx.QueryRowContext(ctx, responseSelect+` WHERE username=? AND response_id=?`, username, responseID))
}

func validateCreate(input CreateInteraction) error {
	required := []string{input.Username, input.SessionID, input.AdapterKind, input.VendorRequestID, input.RequestKind, input.RiskClass, input.Title, input.InputHash}
	for _, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: required interaction field is empty", ErrValidation)
		}
	}
	if len(input.Preview) > 8192 || len(input.Title) > 240 || len(input.Summary) > 2000 {
		return fmt.Errorf("%w: interaction display field is too long", ErrValidation)
	}
	if len(normalizedStrings(input.AllowedActions)) == 0 {
		return fmt.Errorf("%w: allowedActions is required", ErrValidation)
	}
	switch input.ResponseSchema.Type {
	case "action", "single_choice", "multiple_choice", "text":
	default:
		return fmt.Errorf("%w: unsupported response schema type", ErrValidation)
	}
	return nil
}

func validateResponse(interaction Interaction, input RespondInput) error {
	if !contains(interaction.AllowedActions, input.Action) {
		return fmt.Errorf("%w: action is not allowed", ErrValidation)
	}
	if interaction.ResponseSchema.MaxFeedbackLength > 0 && len([]rune(input.Feedback)) > interaction.ResponseSchema.MaxFeedbackLength {
		return fmt.Errorf("%w: feedback is too long", ErrValidation)
	}
	options := make(map[string]struct{}, len(interaction.ResponseSchema.Options))
	for _, option := range interaction.ResponseSchema.Options {
		options[option.ID] = struct{}{}
	}
	for _, selected := range input.SelectedOptionIDs {
		if _, ok := options[selected]; !ok {
			return fmt.Errorf("%w: selected option is not declared", ErrValidation)
		}
	}
	selectedCount := len(input.SelectedOptionIDs)
	switch interaction.ResponseSchema.Type {
	case "action":
		if selectedCount != 0 {
			return fmt.Errorf("%w: action response cannot select options", ErrValidation)
		}
	case "single_choice":
		if selectedCount != 1 {
			return fmt.Errorf("%w: exactly one option is required", ErrValidation)
		}
	case "multiple_choice":
		min := interaction.ResponseSchema.MinSelections
		max := interaction.ResponseSchema.MaxSelections
		if selectedCount < min || (max > 0 && selectedCount > max) {
			return fmt.Errorf("%w: selection count is outside schema bounds", ErrValidation)
		}
	case "text":
		if strings.TrimSpace(input.Feedback) == "" {
			return fmt.Errorf("%w: text answer is required", ErrValidation)
		}
	}
	return nil
}

func insertAudit(ctx context.Context, tx *sql.Tx, username, requestID, eventType, actor, detail string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(event_id,username,request_id,event_type,actor,detail,created_at) VALUES(?,?,?,?,?,?,?)`, newID("audit"), username, requestID, eventType, actor, detail, formatTime(now))
	return err
}

func insertOutbox(ctx context.Context, tx *sql.Tx, username, requestID, eventType string, payload interface{}, now time.Time) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox_events(event_id,username,aggregate_id,event_type,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, newID("outbox"), username, requestID, eventType, string(encoded), formatTime(now), formatTime(now))
	return err
}

func normalizedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hashResponseInput(requestID string, input RespondInput) string {
	copyInput := input
	copyInput.SelectedOptionIDs = append([]string(nil), input.SelectedOptionIDs...)
	sort.Strings(copyInput.SelectedOptionIDs)
	payload, _ := json.Marshal(struct {
		RequestID string
		Input     RespondInput
	}{requestID, copyInput})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func newID(prefix string) string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return prefix + "-" + hex.EncodeToString(buf)
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func nullableTime(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return formatTime(value)
}

func nullString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
