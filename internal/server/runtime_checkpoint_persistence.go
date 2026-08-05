package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/ianf339/roambench/internal/controlplane"
)

// hydrateApprovalsLocked restores the compatibility projection used by legacy
// Project Control mutations. SQLite remains the owner; callers may mutate the
// hydrated projection and saveLocked will commit it back before writing JSON.
func (s *projectControlStore) hydrateApprovalsLocked(username string, state *projectControlState) error {
	if s.controlPlane == nil || state == nil {
		return nil
	}
	ctx := context.Background()
	interactions, err := s.controlPlane.ListAllInteractions(ctx, username)
	if err != nil {
		return err
	}
	checkpointIndexes := make(map[string]int, len(state.Checkpoints))
	for index, checkpoint := range state.Checkpoints {
		checkpointIndexes[checkpoint.ID] = index
	}
	decisionIDs := make(map[string]bool, len(state.Decisions))
	for _, decision := range state.Decisions {
		decisionIDs[decision.ID] = true
	}
	for _, interaction := range interactions {
		checkpoint := projectControlCheckpointFromInteraction(interaction)
		responses, responseErr := s.controlPlane.ListResponses(ctx, username, interaction.RequestID)
		if responseErr != nil {
			return responseErr
		}
		if len(responses) == 1 {
			response := responses[0]
			checkpoint.ResolvedByDecisionID = response.ResponseID
			if !decisionIDs[response.ResponseID] {
				decision := projectControlDecisionFromResponse(interaction, response)
				decision.ProjectID = checkpointProjectID(state, interaction.TaskID)
				decision.WorkstreamID = checkpointWorkstreamID(state, interaction.TaskID)
				state.Decisions = append(state.Decisions, decision)
				decisionIDs[response.ResponseID] = true
			}
		}
		if index, exists := checkpointIndexes[checkpoint.ID]; exists {
			state.Checkpoints[index] = checkpoint
		} else {
			checkpointIndexes[checkpoint.ID] = len(state.Checkpoints)
			state.Checkpoints = append(state.Checkpoints, checkpoint)
		}
	}
	return nil
}

func (s *projectControlStore) syncApprovalsLocked(username string, state projectControlState) error {
	if s.controlPlane == nil || (len(state.Checkpoints) == 0 && len(state.Decisions) == 0) {
		return nil
	}
	payload, err := json.Marshal(struct {
		Checkpoints []projectControlCheckpoint
		Decisions   []projectControlDecision
		Events      []projectControlRecordedEvent
	}{state.Checkpoints, state.Decisions, state.Events})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	snapshot := legacyApprovalSnapshot(username, state, hex.EncodeToString(sum[:]))
	return s.controlPlane.ImportLegacyApprovals(context.Background(), snapshot)
}

func projectControlCheckpointFromInteraction(interaction controlplane.Interaction) projectControlCheckpoint {
	checkpoint := projectControlCheckpoint{
		ID: interaction.RequestID, TaskID: interaction.TaskID, Kind: interaction.RequestKind,
		Title: interaction.Title, Reason: interaction.Summary, Status: compatibilityCheckpointStatus(interaction),
		RequestedAt: interaction.CreatedAt.Format(time.RFC3339Nano), AllowedActions: compatibilityAllowedActions(interaction.AllowedActions),
		DecisionSummary: interaction.FinalAction, RowVersion: interaction.RowVersion,
	}
	if checkpoint.Status != "pending" {
		checkpoint.AllowedActions = []string{}
		checkpoint.DecisionSummary = projectedDecisionSummary(interaction.RequestKind, interaction.FinalAction)
	}
	return checkpoint
}

func projectControlDecisionFromResponse(interaction controlplane.Interaction, response controlplane.Response) projectControlDecision {
	decision := projectControlDecision{
		ID: response.ResponseID, DecisionType: compatibilityDecisionType(interaction.RequestKind, response.Action), Actor: response.Actor,
		Timestamp: response.CreatedAt.Format(time.RFC3339Nano), Summary: response.Feedback,
		TaskID: interaction.TaskID, CheckpointID: interaction.RequestID,
	}
	if decision.Summary == "" {
		decision.Summary = projectedDecisionSummary(interaction.RequestKind, response.Action)
	}
	return decision
}
