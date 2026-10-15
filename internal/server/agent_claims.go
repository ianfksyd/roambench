package server

// K5 agent claim/heartbeat/release handlers (WS-20261015-001, task card C5).
//
// Wiring notes for the integrator (Hermes):
//   - No init call is required: the claim store is built lazily from
//     s.projectControl.controlPlane (claimStoreFor). If controlPlane is nil
//     the handlers answer 503.
//   - claim and heartbeat run expireStaleClaimsForUser first, so stale leases
//     recycle their tasks back to ready even without the startup sweep; call
//     expireStaleClaimsForUser(username) once at server start for parity.
//   - Approval hook (checkpoint Kind=="dispatch", see K4/GATE-1): on approve,
//     set task.DispatchState="ready"; on reject, set it to "none" (or leave
//     awaiting_approval). Until that hook is wired in recordCheckpointDecision,
//     tests set DispatchState directly.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ianf339/roambench/internal/controlplane"
)

// projectControlClaimNow is the injectable clock for the claim store.
// Tests override it; production uses UTC wall clock.
var projectControlClaimNow = func() time.Time { return time.Now().UTC() }

// claimStoreFor lazily wraps the control-plane repository. ClaimStore is a
// stateless facade (repository + clock), so rebuilding it per call is cheap
// and avoids adding fields to projectControlStore (owned by C1).
func (s *projectControlStore) claimStoreFor() *controlplane.ClaimStore {
	if s == nil || s.controlPlane == nil {
		return nil
	}
	return controlplane.NewClaimStore(s.controlPlane, projectControlClaimNow)
}

type agentClaimRequest struct {
	RuntimeID  string `json:"runtimeId"`
	AgentLabel string `json:"agentLabel"`
}

type agentHeartbeatRequest struct {
	LeaseID string `json:"leaseId"`
}

type agentReleaseRequest struct {
	LeaseID string `json:"leaseId"`
	Outcome string `json:"outcome"`
}

// agentClaimTask implements the claim half of K5. New function on
// projectControlStore (no existing function bodies changed).
func (s *projectControlStore) agentClaimTask(username string, req agentClaimRequest) (int, map[string]interface{}, error) {
	claims := s.claimStoreFor()
	if claims == nil {
		return http.StatusServiceUnavailable, nil, errors.New("claim lease store unavailable")
	}
	req.RuntimeID = strings.TrimSpace(req.RuntimeID)
	req.AgentLabel = strings.TrimSpace(req.AgentLabel)
	if req.RuntimeID == "" || req.AgentLabel == "" {
		return http.StatusBadRequest, nil, errors.New("runtimeId and agentLabel are required")
	}
	if _, err := s.expireStaleClaimsForUser(username); err != nil {
		return http.StatusInternalServerError, nil, fmt.Errorf("lease expiry sweep: %w", err)
	}
	candidates := s.readyTaskIDsForUser(username)
	claim, err := claims.ClaimReadyTask(context.Background(), username, candidates, req.RuntimeID, req.AgentLabel)
	if errors.Is(err, controlplane.ErrNotFound) {
		return http.StatusNoContent, nil, nil
	}
	if err != nil {
		return http.StatusInternalServerError, nil, err
	}
	// TOCTOU re-check (K5): the candidate snapshot was taken before the lease
	// INSERT. Re-verify inside withStateLocked that the task is still
	// dispatch-ready before overwriting its dispatch state; a checkpoint
	// rejection that raced in between must win over the claim.
	if err := s.applyClaimToTaskRechecked(username, claim.TaskID, controlplane.DispatchStateClaimed, claim.RuntimeID, claim.ClaimID, func(current string) error {
		if current != controlplane.DispatchStateReady {
			return controlplane.ErrConflict
		}
		return nil
	}); err != nil {
		// Roll the losing lease back so the lease table stays consistent
		// with the task projection.
		_, _ = claims.ReleaseClaim(context.Background(), username, claim.ClaimID, controlplane.ReleaseOutcomeReleased)
		if errors.Is(err, controlplane.ErrConflict) {
			return http.StatusConflict, nil, fmt.Errorf("task %s is no longer dispatch-ready", claim.TaskID)
		}
		return http.StatusInternalServerError, nil, err
	}
	s.recordClaimTimeline(username, claim.TaskID, claim.AgentLabel, "task_claimed",
		fmt.Sprintf("Task claimed by runtime %s (lease %s, agent %s) until %s.",
			claim.RuntimeID, claim.ClaimID, claim.AgentLabel, claim.LeaseExpiresAt.UTC().Format(time.RFC3339)))
	response := map[string]interface{}{
		"taskId":          claim.TaskID,
		"leaseId":         claim.ClaimID,
		"leaseExpiresAt":  claim.LeaseExpiresAt.UTC().Format(time.RFC3339),
		"runtimeId":       claim.RuntimeID,
		"agentLabel":      claim.AgentLabel,
		"claimStatus":     claim.Status,
		"leaseTtlSeconds": int(controlplane.LeaseTTL / time.Second),
	}
	for key, value := range s.agentTaskPayloadForTaskID(username, claim.TaskID) {
		if _, exists := response[key]; !exists {
			response[key] = value
		}
	}
	return http.StatusOK, response, nil
}

func (s *projectControlStore) agentHeartbeatTask(username string, req agentHeartbeatRequest) (int, map[string]interface{}, error) {
	claims := s.claimStoreFor()
	if claims == nil {
		return http.StatusServiceUnavailable, nil, errors.New("claim lease store unavailable")
	}
	if strings.TrimSpace(req.LeaseID) == "" {
		return http.StatusBadRequest, nil, errors.New("leaseId is required")
	}
	if _, err := s.expireStaleClaimsForUser(username); err != nil {
		return http.StatusInternalServerError, nil, fmt.Errorf("lease expiry sweep: %w", err)
	}
	result, err := claims.HeartbeatClaim(context.Background(), username, req.LeaseID)
	if errors.Is(err, controlplane.ErrNotFound) {
		return http.StatusNotFound, nil, errors.New("lease not found, released or expired")
	}
	if err != nil {
		return http.StatusInternalServerError, nil, err
	}
	s.recordClaimTimeline(username, result.Claim.TaskID, result.Claim.AgentLabel, "task_lease_heartbeat",
		fmt.Sprintf("Lease %s renewed by runtime %s until %s.",
			result.Claim.ClaimID, result.Claim.RuntimeID, result.LeaseExpiresAt.UTC().Format(time.RFC3339)))
	return http.StatusOK, map[string]interface{}{
		"leaseId":        result.Claim.ClaimID,
		"taskId":         result.Claim.TaskID,
		"leaseExpiresAt": result.LeaseExpiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *projectControlStore) agentReleaseTask(username string, req agentReleaseRequest) (int, map[string]interface{}, error) {
	claims := s.claimStoreFor()
	if claims == nil {
		return http.StatusServiceUnavailable, nil, errors.New("claim lease store unavailable")
	}
	if strings.TrimSpace(req.LeaseID) == "" {
		return http.StatusBadRequest, nil, errors.New("leaseId is required")
	}
	outcome := strings.TrimSpace(strings.ToLower(req.Outcome))
	switch outcome {
	case controlplane.ReleaseOutcomeDone, controlplane.ReleaseOutcomeFailed, controlplane.ReleaseOutcomeReleased:
	default:
		return http.StatusBadRequest, nil, errors.New("outcome must be done, failed or released")
	}
	result, err := claims.ReleaseClaim(context.Background(), username, req.LeaseID, outcome)
	if errors.Is(err, controlplane.ErrNotFound) {
		return http.StatusNotFound, nil, errors.New("lease not found, released or expired")
	}
	if err != nil {
		return http.StatusInternalServerError, nil, err
	}
	nextDispatchState := result.NextDispatchState
	claimedBy := ""
	if nextDispatchState == controlplane.DispatchStateClaimed {
		claimedBy = result.Claim.RuntimeID
	}
	if err := s.applyClaimToTask(username, result.Claim.TaskID, nextDispatchState, claimedBy, result.Claim.ClaimID); err != nil {
		return http.StatusInternalServerError, nil, err
	}
	s.recordClaimTimeline(username, result.Claim.TaskID, result.Claim.AgentLabel, "task_lease_released",
		fmt.Sprintf("Lease %s released by runtime %s with outcome %s; task dispatch state -> %s.",
			result.Claim.ClaimID, result.Claim.RuntimeID, outcome, nextDispatchState))
	return http.StatusOK, map[string]interface{}{
		"leaseId":        result.Claim.ClaimID,
		"taskId":         result.Claim.TaskID,
		"outcome":        outcome,
		"dispatchState":  nextDispatchState,
		"leaseExpiresAt": result.LeaseExpiresAt.UTC().Format(time.RFC3339),
	}, nil
}

// expireStaleClaimsForUser recycles stale active leases for username: the
// lease rows go to status='expired' and their tasks back to dispatchState=ready.
func (s *projectControlStore) expireStaleClaimsForUser(username string) ([]controlplane.ExpiredClaim, error) {
	claims := s.claimStoreFor()
	if claims == nil {
		return nil, nil
	}
	expired, err := claims.ExpireStaleClaims(context.Background())
	if err != nil {
		return nil, err
	}
	for _, item := range expired {
		if item.Claim.Username != username {
			continue
		}
		if err := s.applyClaimToTask(username, item.Claim.TaskID, controlplane.DispatchStateReady, "", item.Claim.ClaimID); err != nil {
			return expired, err
		}
		s.recordClaimTimeline(username, item.Claim.TaskID, item.Claim.AgentLabel, "task_lease_expired",
			fmt.Sprintf("Lease %s for runtime %s expired; task returned to ready.", item.Claim.ClaimID, item.Claim.RuntimeID))
	}
	return expired, nil
}

// readyTaskIDsForUser reads the project-control task list (which owns
// DispatchState) and returns claimable task IDs in stored order.
func (s *projectControlStore) readyTaskIDsForUser(username string) []string {
	state, err := s.loadOrSeed(username)
	if err != nil {
		return nil
	}
	var ids []string
	for _, task := range state.Tasks {
		dispatchState := strings.TrimSpace(task.DispatchState)
		if dispatchState == "" {
			dispatchState = controlplane.DispatchStateNone
		}
		if dispatchState == controlplane.DispatchStateReady {
			ids = append(ids, task.ID)
		}
	}
	return ids
}

// applyClaimToTask writes DispatchState / ClaimedByRuntimeID back to the
// project-control task and appends a Timeline entry. Task fields live in the
// project-control state; the lease row stays authoritative for leases.
//
// The recheckDispatchState callback (when non-nil) runs inside the same
// withStateLocked critical section just before the write, so a claim that
// raced with a checkpoint rejection (dispatchState left ready) fails with
// ErrConflict instead of overwriting the human decision (K5 TOCTOU fix).
func (s *projectControlStore) applyClaimToTask(username, taskID, dispatchState, claimedByRuntimeID, claimID string) error {
	return s.applyClaimToTaskRechecked(username, taskID, dispatchState, claimedByRuntimeID, claimID, nil)
}

func (s *projectControlStore) applyClaimToTaskRechecked(username, taskID, dispatchState, claimedByRuntimeID, claimID string, recheckDispatchState func(current string) error) error {
	if strings.TrimSpace(taskID) == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.withStateLocked(username, func(state *projectControlState) error {
		found := false
		for i := range state.Tasks {
			if state.Tasks[i].ID != taskID {
				continue
			}
			found = true
			if recheckDispatchState != nil {
				current := strings.TrimSpace(state.Tasks[i].DispatchState)
				if current == "" {
					current = controlplane.DispatchStateNone
				}
				if err := recheckDispatchState(current); err != nil {
					return err
				}
			}
			state.Tasks[i].DispatchState = dispatchState
			state.Tasks[i].ClaimedByRuntimeID = claimedByRuntimeID
			state.Tasks[i].RowVersion += 1
			projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
				ID:           projectControlID("event", "task-dispatch-"+dispatchState),
				Timestamp:    now,
				Actor:        "dispatch",
				Action:       "task_dispatch_state_changed",
				Detail:       fmt.Sprintf("Task dispatch state set to %s (lease %s, runtime %s).", dispatchState, claimID, claimedByRuntimeID),
				ProjectID:    state.Tasks[i].ProjectID,
				WorkstreamID: state.Tasks[i].WorkstreamID,
				TaskID:       taskID,
			})
			break
		}
		if !found {
			return fmt.Errorf("task not found: %s", taskID)
		}
		return nil
	})
	return err
}

// recordClaimTimeline appends a claim Timeline entry to the owning task.
// The action is prefixed with checkpoint_ so the existing Audit projection
// (applyRecordedEventsToTasks) also surfaces it in the task audit list.
func (s *projectControlStore) recordClaimTimeline(username, taskID, agentLabel, action, detail string) {
	if taskID == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.withStateLocked(username, func(state *projectControlState) error {
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
			ID:        projectControlID("event", action),
			Timestamp: now,
			Actor:     "agent:" + agentLabel,
			Action:    "checkpoint_" + action,
			Detail:    detail,
			TaskID:    taskID,
		})
		return nil
	})
}

// agentTaskPayloadForTaskID returns the K5 task fields for a claimed task
// (same shape as the legacy agentGetTask payload, keyed by exact task ID).
func (s *projectControlStore) agentTaskPayloadForTaskID(username, taskID string) map[string]interface{} {
	state, err := s.loadOrSeed(username)
	if err != nil {
		return nil
	}
	for _, task := range state.Tasks {
		if task.ID != taskID {
			continue
		}
		runbook := projectControlRunbookForTask(task)
		var phase projectControlRunbookPhase
		if p, ok := findProjectControlRunbookPhase(runbook, task.CurrentPhase); ok {
			phase = p
		}
		return map[string]interface{}{
			"title":             task.Title,
			"goal":              task.Goal,
			"state":             task.State,
			"currentPhase":      task.CurrentPhase,
			"runbookState":      task.RunbookState,
			"requiredArtifacts": phase.RequiredArtifacts,
			"missingEvidence":   task.MissingEvidence,
			"nextStep":          task.NextStep,
		}
	}
	return nil
}

// ---- HTTP handlers (bodies replace the C1 stubs; route registration unchanged) ----

func (s *Server) handleAgentClaim(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req agentClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	status, payload, err := s.projectControl.agentClaimTask(username, req)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	if status == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, status, payload)
}

func (s *Server) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req agentHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	status, payload, err := s.projectControl.agentHeartbeatTask(username, req)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, status, payload)
}

func (s *Server) handleAgentRelease(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req agentReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	status, payload, err := s.projectControl.agentReleaseTask(username, req)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, status, payload)
}
