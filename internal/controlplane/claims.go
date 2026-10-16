package controlplane

// Task claim leases (K5/K6, WS-20261015-001, task card C5).
//
// A claim lease is an exclusive, time-bounded hold on a task for one agent
// runtime. Uniqueness of the active holder is enforced by the partial unique
// index task_claims_active_idx(task_id) WHERE status='active': concurrent
// claimants race on INSERT and exactly one wins. Leases are persisted in
// SQLite so they survive server restarts.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// LeaseTTL is the contract-fixed lease duration (K5).
const LeaseTTL = 120 * time.Second

// Claim statuses.
const (
	ClaimStatusActive   = "active"
	ClaimStatusReleased = "released"
	ClaimStatusExpired  = "expired"
)

// Release outcomes accepted by ReleaseClaim (K5).
const (
	ReleaseOutcomeDone     = "done"
	ReleaseOutcomeFailed   = "failed"
	ReleaseOutcomeReleased = "released"
)

// Task claim dispatch states (mirror of the projectControlTask field values;
// defined here so the control-plane package has no import cycle).
const (
	DispatchStateNone             = "none"
	DispatchStateAwaitingApproval = "awaiting_approval"
	DispatchStateReady            = "ready"
	DispatchStateClaimed          = "claimed"
	DispatchStateReleased         = "released"
	DispatchStateDone             = "done"
)

var ErrLeaseConflict = errors.New("claim lease conflict")

// TaskClaim is a persisted lease row.
type TaskClaim struct {
	ClaimID        string    `json:"claimId"`
	Username       string    `json:"username"`
	TaskID         string    `json:"taskId"`
	RuntimeID      string    `json:"runtimeId"`
	AgentLabel     string    `json:"agentLabel"`
	Status         string    `json:"status"`
	LeaseExpiresAt time.Time `json:"leaseExpiresAt"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// ExpiredClaim is the result of recycling one stale lease.
type ExpiredClaim struct {
	Claim    TaskClaim
	Timeline ClaimTimelineEvent
}

// ClaimTimelineEvent is the Timeline/Audit trail entry produced for a claim
// transition. The server-side store persists these into the project-control
// state (Timeline + Audit) — see server agent_claims hook wiring.
type ClaimTimelineEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	TaskID    string    `json:"taskId"`
	ClaimID   string    `json:"claimId"`
}

// ClaimStore implements the K5 lease operations on top of a *Repository.
// now is injectable for clock-controlled tests.
type ClaimStore struct {
	repo *Repository
	now  func() time.Time
}

// NewClaimStore wraps a Repository. A nil now falls back to time.Now.
func NewClaimStore(repo *Repository, now func() time.Time) *ClaimStore {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &ClaimStore{repo: repo, now: now}
}

// HeartbeatResult is the outcome of HeartbeatClaim.
type HeartbeatResult struct {
	Claim          TaskClaim
	LeaseExpiresAt time.Time
	Timeline       ClaimTimelineEvent
}

// ReleaseResult is the outcome of ReleaseClaim.
type ReleaseResult struct {
	Claim             TaskClaim
	LeaseExpiresAt    time.Time // expiry at the moment of release
	NextDispatchState string    // dispatchState the task should move to
	Timeline          ClaimTimelineEvent
}

// ClaimReadyTask atomically claims the first ready task for username.
// readyTaskIDs lists the candidate tasks in priority order (the caller reads
// the project-control task list, which owns dispatch state). The SQLite
// partial unique index guarantees that when several ClaimStore instances
// race on the same task, exactly one INSERT succeeds; the loser moves on to
// the next candidate. Returns ErrNotFound when no candidate can be claimed.
func (c *ClaimStore) ClaimReadyTask(ctx context.Context, username string, readyTaskIDs []string, runtimeID, agentLabel string) (TaskClaim, error) {
	username = strings.TrimSpace(username)
	runtimeID = strings.TrimSpace(runtimeID)
	agentLabel = strings.TrimSpace(agentLabel)
	if username == "" {
		return TaskClaim{}, fmt.Errorf("%w: username is required", ErrValidation)
	}
	if runtimeID == "" || agentLabel == "" {
		return TaskClaim{}, fmt.Errorf("%w: runtimeId and agentLabel are required", ErrValidation)
	}
	for _, taskID := range readyTaskIDs {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		claim, err := c.tryClaimTask(ctx, username, taskID, runtimeID, agentLabel)
		if errors.Is(err, ErrLeaseConflict) {
			continue // another runtime holds the active lease; next candidate
		}
		if err != nil {
			return TaskClaim{}, err
		}
		return claim, nil
	}
	return TaskClaim{}, ErrNotFound
}

func (c *ClaimStore) tryClaimTask(ctx context.Context, username, taskID, runtimeID, agentLabel string) (TaskClaim, error) {
	now := c.now().UTC()
	expiresAt := now.Add(LeaseTTL)
	claim := TaskClaim{
		ClaimID: newID("claim"), Username: username, TaskID: taskID,
		RuntimeID: runtimeID, AgentLabel: agentLabel, Status: ClaimStatusActive,
		LeaseExpiresAt: expiresAt, CreatedAt: now, UpdatedAt: now,
	}
	tx, err := c.repo.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskClaim{}, err
	}
	defer tx.Rollback()
	// The partial unique index turns a double claim into an integrity error.
	_, err = tx.ExecContext(ctx, `INSERT INTO task_claims(
		claim_id,username,task_id,runtime_id,agent_label,status,lease_expires_at,created_at,updated_at
	) VALUES(?,?,?,?,?,?,?,?,?)`,
		claim.ClaimID, claim.Username, claim.TaskID, claim.RuntimeID, claim.AgentLabel,
		claim.Status, formatTime(claim.LeaseExpiresAt), formatTime(claim.CreatedAt), formatTime(claim.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return TaskClaim{}, ErrLeaseConflict
		}
		return TaskClaim{}, err
	}
	if err := tx.Commit(); err != nil {
		return TaskClaim{}, err
	}
	return claim, nil
}

// HeartbeatClaim renews an active, unexpired lease to now+LeaseTTL.
// Returns ErrNotFound when the lease does not exist, was released, or is
// expired (expired leases must be re-claimed, not heartbeated).
func (c *ClaimStore) HeartbeatClaim(ctx context.Context, username, claimID string) (HeartbeatResult, error) {
	claimID = strings.TrimSpace(claimID)
	if username == "" || claimID == "" {
		return HeartbeatResult{}, fmt.Errorf("%w: leaseId is required", ErrValidation)
	}
	tx, err := c.repo.db.BeginTx(ctx, nil)
	if err != nil {
		return HeartbeatResult{}, err
	}
	defer tx.Rollback()
	claim, err := getClaimTx(ctx, tx, username, claimID)
	if err != nil {
		return HeartbeatResult{}, err
	}
	now := c.now().UTC()
	if claim.Status != ClaimStatusActive {
		return HeartbeatResult{}, ErrNotFound
	}
	if !claim.LeaseExpiresAt.After(now) {
		return HeartbeatResult{}, ErrNotFound
	}
	leaseExpiresAt := now.Add(LeaseTTL)
	result, err := tx.ExecContext(ctx, `UPDATE task_claims SET lease_expires_at=?, updated_at=? WHERE claim_id=? AND username=? AND status='active'`,
		formatTime(leaseExpiresAt), formatTime(now), claimID, username)
	if err != nil {
		return HeartbeatResult{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return HeartbeatResult{}, ErrNotFound
	}
	claim.LeaseExpiresAt = leaseExpiresAt
	claim.UpdatedAt = now
	event := ClaimTimelineEvent{
		Timestamp: now, Actor: "agent:" + claim.AgentLabel, Action: "task_lease_heartbeat",
		Detail: fmt.Sprintf("Lease %s renewed for runtime %s until %s.", claim.ClaimID, claim.RuntimeID, formatTime(leaseExpiresAt)),
		TaskID: claim.TaskID, ClaimID: claim.ClaimID,
	}
	if err := tx.Commit(); err != nil {
		return HeartbeatResult{}, err
	}
	return HeartbeatResult{Claim: claim, LeaseExpiresAt: leaseExpiresAt, Timeline: event}, nil
}

// ReleaseClaim closes an active lease with the given outcome. The returned
// NextDispatchState tells the caller what the task's DispatchState should
// become: done for outcome "done", ready (re-claimable) otherwise.
func (c *ClaimStore) ReleaseClaim(ctx context.Context, username, claimID, outcome string) (ReleaseResult, error) {
	claimID = strings.TrimSpace(claimID)
	outcome = strings.TrimSpace(strings.ToLower(outcome))
	switch outcome {
	case ReleaseOutcomeDone, ReleaseOutcomeFailed, ReleaseOutcomeReleased:
	default:
		return ReleaseResult{}, fmt.Errorf("%w: outcome must be done, failed or released", ErrValidation)
	}
	tx, err := c.repo.db.BeginTx(ctx, nil)
	if err != nil {
		return ReleaseResult{}, err
	}
	defer tx.Rollback()
	claim, err := getClaimTx(ctx, tx, username, claimID)
	if err != nil {
		return ReleaseResult{}, err
	}
	now := c.now().UTC()
	if claim.Status != ClaimStatusActive || !claim.LeaseExpiresAt.After(now) {
		return ReleaseResult{}, ErrNotFound
	}
	result, err := tx.ExecContext(ctx, `UPDATE task_claims SET status='released', updated_at=? WHERE claim_id=? AND username=? AND status='active'`,
		formatTime(now), claimID, username)
	if err != nil {
		return ReleaseResult{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ReleaseResult{}, ErrNotFound
	}
	claim.Status = ClaimStatusReleased
	claim.UpdatedAt = now
	nextDispatchState := DispatchStateReady
	if outcome == ReleaseOutcomeDone {
		nextDispatchState = DispatchStateDone
	}
	event := ClaimTimelineEvent{
		Timestamp: now, Actor: "agent:" + claim.AgentLabel, Action: "task_lease_released",
		Detail: fmt.Sprintf("Lease %s released by runtime %s with outcome %s; task dispatch state -> %s.",
			claim.ClaimID, claim.RuntimeID, outcome, nextDispatchState),
		TaskID: claim.TaskID, ClaimID: claim.ClaimID,
	}
	if err := tx.Commit(); err != nil {
		return ReleaseResult{}, err
	}
	return ReleaseResult{Claim: claim, LeaseExpiresAt: claim.LeaseExpiresAt, NextDispatchState: nextDispatchState, Timeline: event}, nil
}

// ExpireStaleClaims moves every active lease whose expiry has passed to
// status='expired'. Callers must transition the affected tasks back to the
// ready dispatch state (task state lives in the project-control store, not
// SQLite). Returns the recycled leases so the caller can emit Timeline and
// Audit entries. Safe to call on server start and before claim/heartbeat.
func (c *ClaimStore) ExpireStaleClaims(ctx context.Context) ([]ExpiredClaim, error) {
	return c.expireStaleClaims(ctx, "")
}

func (c *ClaimStore) expireStaleClaims(ctx context.Context, username string) ([]ExpiredClaim, error) {
	now := c.now().UTC()
	tx, err := c.repo.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, claimSelect+` WHERE status='active' AND (?='' OR username=?)`, username, username)
	if err != nil {
		return nil, err
	}
	var stale []TaskClaim
	for rows.Next() {
		claim, scanErr := scanClaim(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		if !claim.LeaseExpiresAt.After(now) {
			stale = append(stale, claim)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var expired []ExpiredClaim
	for _, claim := range stale {
		result, err := tx.ExecContext(ctx, `UPDATE task_claims SET status='expired', updated_at=? WHERE claim_id=? AND status='active'`,
			formatTime(now), claim.ClaimID)
		if err != nil {
			return nil, err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			continue
		}
		claim.Status = ClaimStatusExpired
		claim.UpdatedAt = now
		event := ClaimTimelineEvent{
			Timestamp: now, Actor: "system", Action: "task_lease_expired",
			Detail: fmt.Sprintf("Lease %s for runtime %s expired at %s; task returned to ready.",
				claim.ClaimID, claim.RuntimeID, formatTime(claim.LeaseExpiresAt)),
			TaskID: claim.TaskID, ClaimID: claim.ClaimID,
		}
		expired = append(expired, ExpiredClaim{Claim: claim, Timeline: event})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return expired, nil
}

// GetClaim returns one lease by ID (any status).
func (c *ClaimStore) GetClaim(ctx context.Context, username, claimID string) (TaskClaim, error) {
	return getClaimTx(ctx, c.repo.db, username, claimID)
}

// ActiveClaimForTask returns the active lease for a task, or ErrNotFound.
func (c *ClaimStore) ActiveClaimForTask(ctx context.Context, username, taskID string) (TaskClaim, error) {
	if username == "" || taskID == "" {
		return TaskClaim{}, fmt.Errorf("%w: username and taskId are required", ErrValidation)
	}
	row := c.repo.db.QueryRowContext(ctx, claimSelect+` WHERE username=? AND task_id=? AND status='active'`, username, taskID)
	return scanClaim(row)
}

const claimSelect = `SELECT claim_id,username,task_id,runtime_id,agent_label,status,lease_expires_at,created_at,updated_at FROM task_claims`

func scanClaim(row scanner) (TaskClaim, error) {
	var claim TaskClaim
	var expires, created, updated string
	if err := row.Scan(&claim.ClaimID, &claim.Username, &claim.TaskID, &claim.RuntimeID, &claim.AgentLabel,
		&claim.Status, &expires, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TaskClaim{}, ErrNotFound
		}
		return TaskClaim{}, err
	}
	claim.LeaseExpiresAt = parseTime(expires)
	claim.CreatedAt = parseTime(created)
	claim.UpdatedAt = parseTime(updated)
	return claim, nil
}

func getClaimTx(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, username, claimID string) (TaskClaim, error) {
	row := q.QueryRowContext(ctx, claimSelect+` WHERE username=? AND claim_id=?`, username, claimID)
	return scanClaim(row)
}

// isUniqueViolation reports whether err is a SQLite/SQL constraint violation
// (modernc.org/sqlite surfaces UNIQUE conflicts as constraint errors with
// numeric codes; database/sql may also wrap them).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "constraint") ||
		strings.Contains(msg, "UNIQUE") ||
		strings.Contains(msg, "unique") ||
		strings.Contains(msg, "SQLITE_CONSTRAINT") ||
		strings.Contains(msg, "2067")
}

// RevokeTaskClaims closes even overdue active leases, scoped to one user's task.
// The project-control store closes its execution gate before calling this.
func (c *ClaimStore) RevokeTaskClaims(ctx context.Context, username, taskID string) error {
	_, err := c.repo.db.ExecContext(ctx, `UPDATE task_claims SET status='released', updated_at=? WHERE username=? AND task_id=? AND status='active'`, formatTime(c.now().UTC()), username, taskID)
	return err
}

// ExpireStaleClaimsForUser avoids consuming another user's expiry events.
func (c *ClaimStore) ExpireStaleClaimsForUser(ctx context.Context, username string) ([]ExpiredClaim, error) {
	return c.expireStaleClaims(ctx, username)
}
