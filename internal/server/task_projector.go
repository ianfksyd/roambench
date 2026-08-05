package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ianf339/roambench/internal/controlplane"
)

func (s *Server) runPendingTaskProjections(ctx context.Context, username string) error {
	if s.controlPlane == nil {
		return errors.New("control plane unavailable")
	}
	projections, err := s.controlPlane.ListPendingTaskProjections(ctx, username, 100)
	if err != nil {
		return err
	}
	var firstErr error
	for _, projection := range projections {
		if err := s.applyTaskProjection(ctx, username, projection); err != nil {
			_ = s.controlPlane.MarkTaskProjectionFailed(ctx, username, projection.DecisionID, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := s.controlPlane.MarkTaskProjectionApplied(ctx, username, projection.DecisionID); err != nil && !errors.Is(err, controlplane.ErrConflict) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *Server) applyTaskProjection(ctx context.Context, username string, projection controlplane.TaskProjection) error {
	interaction, err := s.controlPlane.GetInteraction(ctx, username, projection.RequestID)
	if err != nil {
		return err
	}
	response, err := s.controlPlane.GetResponseByID(ctx, username, projection.ResponseID)
	if err != nil {
		return err
	}
	_, err = s.projectControl.withStateLocked(username, func(state *projectControlState) error {
		eventID := "event-projection-" + projection.DecisionID
		projectionApplied := false
		for _, event := range state.Events {
			if event.ID == eventID {
				projectionApplied = true
				break
			}
		}
		for index := range state.Tasks {
			task := &state.Tasks[index]
			if task.ID != interaction.TaskID {
				continue
			}
			if !projectionApplied {
				if err := applyProjectedTaskDecision(task, interaction.RequestKind, response.Action, response.ResponseID); err != nil {
					return err
				}
				task.RowVersion++
				projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
					ID: eventID, Timestamp: time.Now().UTC().Format(time.RFC3339), Actor: response.Actor,
					Action: "project_task_projection_applied", Detail: "Applied decision " + response.ResponseID + " to task " + task.ID + ".",
					ProjectID: task.ProjectID, WorkstreamID: task.WorkstreamID, TaskID: task.ID,
				})
			}
			appendProjectedDecisionEvents(state, *task, interaction, response, projection.DecisionID)
			return nil
		}
		return fmt.Errorf("task %q not found for projection", interaction.TaskID)
	})
	return err
}

func appendProjectedDecisionEvents(state *projectControlState, task projectControlTask, interaction controlplane.Interaction, response controlplane.Response, decisionID string) {
	now := response.CreatedAt.UTC().Format(time.RFC3339)
	checkpointStatus := compatibilityCheckpointStatus(interaction)
	decisionAction := compatibilityDecisionType(interaction.RequestKind, response.Action)
	summary := projectedDecisionSummary(interaction.RequestKind, response.Action)
	events := []projectControlRecordedEvent{
		{
			ID: "event-projection-checkpoint-resolved-" + decisionID, Timestamp: now, Actor: response.Actor,
			Action: "checkpoint_resolved", Detail: "Resolved checkpoint " + interaction.RequestID + " as " + checkpointStatus + ".",
		},
		{
			ID: "event-projection-decision-made-" + decisionID, Timestamp: now, Actor: response.Actor,
			Action: "decision_made", Detail: "Recorded decision " + decisionAction + " for checkpoint " + interaction.RequestID + ".",
		},
		{
			ID: "event-projection-z-decision-action-" + decisionID, Timestamp: now, Actor: response.Actor,
			Action: decisionAction, Detail: summary,
		},
	}
	for index := range events {
		events[index].ProjectID = task.ProjectID
		events[index].WorkstreamID = task.WorkstreamID
		events[index].TaskID = task.ID
		events[index].CheckpointID = interaction.RequestID
		if !projectControlEventExists(state.Events, events[index].ID) {
			projectControlAppendRecordedEvent(state, events[index])
		}
	}
}

func projectControlEventExists(events []projectControlRecordedEvent, eventID string) bool {
	for _, event := range events {
		if event.ID == eventID {
			return true
		}
	}
	return false
}

func projectedDecisionSummary(requestKind, action string) string {
	approved := action == "approve" || action == "approve_once" || action == "approve_session"
	rejected := action == "reject" || action == "reject_with_feedback"
	switch {
	case action == "expire":
		return "Approval request expired"
	case action == "cancel":
		return "Approval request cancelled"
	case requestKind == "archive_override" && approved:
		return "Archive override approved by human operator"
	case requestKind == "archive_override" && rejected:
		return "Archive override rejected by human operator"
	case approved:
		return "Accepted by human operator"
	case rejected:
		return "Rejected by human operator"
	default:
		return "Rerouted by human operator"
	}
}

func applyProjectedTaskDecision(task *projectControlTask, requestKind, action, decisionID string) error {
	switch requestKind {
	case "final_acceptance":
		switch action {
		case "approve", "approve_once", "approve_session":
			task.AcceptanceStatus = "accepted"
			task.AcceptanceDecisionID = decisionID
			task.RecentSummary = "Task accepted by human operator; execution remains visible for audit."
			task.NextStep = "Archive or hide accepted work with board filters."
		case "reject", "reject_with_feedback":
			task.State = "running"
			task.AcceptanceStatus = "rejected"
			task.AcceptanceDecisionID = decisionID
			task.RecentSummary = "Task rejected during final acceptance and sent back for revision."
			task.NextStep = "Revise the work, regenerate evidence, and resubmit for acceptance."
		case "reroute", "request_changes":
			task.State = "blocked"
			task.AcceptanceStatus = "not_ready"
			task.AcceptanceDecisionID = ""
		default:
			return fmt.Errorf("unsupported final acceptance action %q", action)
		}
	case "archive_override":
		switch action {
		case "approve", "approve_once", "approve_session":
			task.State = "archived"
			task.AcceptanceStatus = "not_ready"
			task.AcceptanceDecisionID = ""
			task.ArchiveDecisionID = decisionID
		case "reject", "reject_with_feedback":
			task.ArchiveDecisionID = decisionID
		default:
			return fmt.Errorf("unsupported archive override action %q", action)
		}
	}
	return nil
}
