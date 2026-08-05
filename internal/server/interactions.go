package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ianf339/roambench/internal/controlplane"
)

type createInteractionRequest struct {
	CheckpointID    string                      `json:"checkpointId"`
	TaskID          string                      `json:"taskId"`
	RuntimeID       string                      `json:"runtimeId"`
	SessionID       string                      `json:"sessionId"`
	AdapterKind     string                      `json:"adapterKind"`
	VendorRequestID string                      `json:"vendorRequestId"`
	RequestKind     string                      `json:"requestKind"`
	RiskClass       string                      `json:"riskClass"`
	Title           string                      `json:"title"`
	Summary         string                      `json:"summary"`
	Preview         string                      `json:"preview"`
	ArtifactRefs    []string                    `json:"artifactRefs"`
	AllowedActions  []string                    `json:"allowedActions"`
	ResponseSchema  controlplane.ResponseSchema `json:"responseSchema"`
	UIHints         map[string]interface{}      `json:"uiHints"`
	ExpiresAt       time.Time                   `json:"expiresAt"`
	InputHash       string                      `json:"inputHash"`
}

type cancelInteractionRequest struct {
	ExpectedRowVersion int    `json:"expectedRowVersion"`
	Reason             string `json:"reason"`
}

func (s *Server) controlPlaneReady(w http.ResponseWriter) bool {
	if s.controlPlane != nil && s.controlPlaneErr == nil {
		return true
	}
	message := "interaction control plane unavailable"
	if s.controlPlaneErr != nil {
		message += ": " + s.controlPlaneErr.Error()
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": message})
	return false
}

func (s *Server) handleAgentInteractions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.agentUsername(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid agent token"})
		return
	}
	if !s.controlPlaneReady(w) {
		return
	}
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Idempotency-Key header is required"})
		return
	}
	var req createInteractionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	interaction, err := s.controlPlane.CreateInteraction(r.Context(), controlplane.CreateInteraction{
		Username: username, CheckpointID: req.CheckpointID, TaskID: req.TaskID, RuntimeID: req.RuntimeID,
		SessionID: req.SessionID, AdapterKind: req.AdapterKind, VendorRequestID: req.VendorRequestID,
		RequestKind: req.RequestKind, RiskClass: req.RiskClass, Title: req.Title, Summary: req.Summary,
		Preview: req.Preview, ArtifactRefs: req.ArtifactRefs, AllowedActions: req.AllowedActions,
		ResponseSchema: req.ResponseSchema, UIHints: req.UIHints, ExpiresAt: req.ExpiresAt, InputHash: req.InputHash,
		ActorScope: "agent", IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, interaction)
}

func (s *Server) handleAgentInteraction(w http.ResponseWriter, r *http.Request) {
	username, ok := s.agentUsername(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid agent token"})
		return
	}
	if !s.controlPlaneReady(w) {
		return
	}
	requestID, action, ok := splitInteractionPath(r.URL.Path, "/api/agent/v1/interactions/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch {
	case r.Method == http.MethodGet && action == "":
		interaction, err := s.controlPlane.GetInteraction(r.Context(), username, requestID)
		if err != nil {
			writeControlPlaneError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, interaction)
	case r.Method == http.MethodGet && action == "wait":
		timeout := 30 * time.Second
		if raw := strings.TrimSpace(r.URL.Query().Get("timeout")); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil || parsed <= 0 || parsed > 30*time.Second {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timeout must be between 0 and 30s"})
				return
			}
			timeout = parsed
		}
		interaction, err := s.controlPlane.WaitInteraction(r.Context(), username, requestID, timeout)
		if err != nil {
			writeControlPlaneError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, interaction)
	case r.Method == http.MethodPost && action == "cancel":
		if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Idempotency-Key header is required"})
			return
		}
		var req cancelInteractionRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		interaction, err := s.controlPlane.CancelInteractionIdempotent(r.Context(), username, requestID, controlplane.CancelInteractionInput{
			ExpectedRowVersion: req.ExpectedRowVersion, Reason: req.Reason,
			ActorScope: "agent", IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		})
		if err != nil {
			writeControlPlaneError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, interaction)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMobileInteractions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.controlPlaneReady(w) {
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	items, err := s.controlPlane.ListPendingInteractions(r.Context(), GetUsername(r), limit)
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"interactions": items})
}

func (s *Server) handleMobileInteraction(w http.ResponseWriter, r *http.Request) {
	if !s.controlPlaneReady(w) {
		return
	}
	username := GetUsername(r)
	requestID, action, ok := splitInteractionPath(r.URL.Path, "/api/mobile/interactions/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet && action == "" {
		interaction, err := s.controlPlane.GetInteraction(r.Context(), username, requestID)
		if err != nil {
			writeControlPlaneError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, interaction)
		return
	}
	if r.Method != http.MethodPost || action != "respond" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req controlplane.RespondInput
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Actor = username
	response, replayed, err := s.controlPlane.Respond(r.Context(), username, requestID, req)
	if err != nil {
		if errors.Is(err, controlplane.ErrConflict) {
			current, getErr := s.controlPlane.GetInteraction(r.Context(), username, requestID)
			if getErr == nil {
				writeJSON(w, http.StatusConflict, map[string]interface{}{"error": err.Error(), "interaction": current})
				return
			}
		}
		writeControlPlaneError(w, err)
		return
	}
	_ = s.runPendingTaskProjections(r.Context(), username)
	writeJSON(w, http.StatusOK, map[string]interface{}{"response": response, "replayed": replayed})
}

func splitInteractionPath(path, prefix string) (string, string, bool) {
	remainder := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if remainder == "" || remainder == path {
		return "", "", false
	}
	parts := strings.Split(remainder, "/")
	if len(parts) > 2 || strings.TrimSpace(parts[0]) == "" || len(parts[0]) > 128 {
		return "", "", false
	}
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	return parts[0], action, true
}

func writeControlPlaneError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, controlplane.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, controlplane.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, controlplane.ErrValidation):
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (s *Server) mergeControlPlaneSnapshot(r *http.Request, username string, snapshot projectControlSnapshot) projectControlSnapshot {
	if s.controlPlane == nil || s.controlPlaneErr != nil {
		return snapshot
	}
	interactions, err := s.controlPlane.ListInteractions(r.Context(), username, 500)
	if err != nil {
		return snapshot
	}
	checkpointIDs := make(map[string]bool, len(snapshot.Checkpoints))
	for _, checkpoint := range snapshot.Checkpoints {
		checkpointIDs[checkpoint.ID] = true
	}
	decisionIDs := make(map[string]bool, len(snapshot.Decisions))
	for _, decision := range snapshot.Decisions {
		decisionIDs[decision.ID] = true
	}
	for _, interaction := range interactions {
		checkpoint := projectControlCheckpointFromInteraction(interaction)
		responses, responseErr := s.controlPlane.ListResponses(r.Context(), username, interaction.RequestID)
		if responseErr == nil && len(responses) == 1 {
			response := responses[0]
			checkpoint.ResolvedByDecisionID = response.ResponseID
			if !decisionIDs[response.ResponseID] {
				snapshot.Decisions = append(snapshot.Decisions, projectControlDecisionFromResponse(interaction, response))
				decisionIDs[response.ResponseID] = true
			}
		}
		if checkpointIDs[checkpoint.ID] {
			continue
		}
		checkpointIDs[checkpoint.ID] = true
		snapshot.Checkpoints = append(snapshot.Checkpoints, checkpoint)
		if checkpoint.Status == "pending" {
			snapshot.ApprovalsCount++
			snapshot.Dashboard.PendingApprovals++
		}
	}
	return snapshot
}

func compatibilityDecisionType(requestKind, action string) string {
	verb := action
	switch action {
	case "approve", "approve_once", "approve_session":
		verb = "approved"
	case "reject", "reject_with_feedback":
		verb = "rejected"
	case "reroute", "request_changes":
		verb = "rerouted"
	}
	switch requestKind {
	case "final_acceptance":
		return "final_acceptance_" + verb
	case "archive_override":
		return "archive_override_" + verb
	default:
		return action
	}
}

func compatibilityCheckpointStatus(interaction controlplane.Interaction) string {
	if interaction.Status != "resolved" {
		return interaction.Status
	}
	switch interaction.FinalAction {
	case "approve", "approve_once", "approve_session":
		return "approved"
	case "reject", "reject_with_feedback":
		return "rejected"
	case "reroute", "request_changes":
		return "rerouted"
	default:
		return "resolved"
	}
}

func compatibilityAllowedActions(actions []string) []string {
	result := make([]string, 0, len(actions))
	seen := map[string]bool{}
	for _, action := range actions {
		mapped := action
		switch action {
		case "approve_once", "approve_session":
			mapped = "approve"
		case "reject_with_feedback":
			mapped = "reject"
		case "request_changes":
			mapped = "reroute"
		}
		if (mapped == "approve" || mapped == "reject" || mapped == "reroute") && !seen[mapped] {
			seen[mapped] = true
			result = append(result, mapped)
		}
	}
	return result
}

func (s *Server) resolveControlPlaneCheckpointDecision(w http.ResponseWriter, r *http.Request, requestID, legacyAction string) bool {
	if s.controlPlane == nil || s.controlPlaneErr != nil {
		return false
	}
	username := GetUsername(r)
	interaction, err := s.controlPlane.GetInteraction(r.Context(), username, requestID)
	if errors.Is(err, controlplane.ErrNotFound) {
		return false
	}
	if err != nil {
		writeControlPlaneError(w, err)
		return true
	}
	if interaction.Status != "pending" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "checkpoint is not pending"})
		return true
	}
	action := mapLegacyDecisionAction(interaction.AllowedActions, strings.TrimSpace(strings.ToLower(legacyAction)))
	if action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action not allowed for interaction"})
		return true
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		idempotencyKey = "legacy-web-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	_, _, err = s.controlPlane.Respond(r.Context(), username, requestID, controlplane.RespondInput{
		Action: action, Actor: username, DeviceID: "web", ExpectedRowVersion: interaction.RowVersion,
		IdempotencyKey: idempotencyKey, InputHash: interaction.InputHash,
	})
	if err != nil {
		if errors.Is(err, controlplane.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		} else {
			writeControlPlaneError(w, err)
		}
		return true
	}
	_ = s.runPendingTaskProjections(r.Context(), username)
	snapshot, err := s.projectControl.snapshotForUser(username, s.terminals)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return true
	}
	writeJSON(w, http.StatusOK, s.mergeControlPlaneSnapshot(r, username, snapshot))
	return true
}

func mapLegacyDecisionAction(allowed []string, legacy string) string {
	candidates := map[string][]string{
		"approve": {"approve_once", "approve"},
		"reject":  {"reject", "reject_with_feedback"},
		"reroute": {"reroute", "request_changes"},
	}[legacy]
	for _, candidate := range candidates {
		for _, action := range allowed {
			if action == candidate {
				return candidate
			}
		}
	}
	return ""
}
