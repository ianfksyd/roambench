package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/ianf339/roambench/internal/controlplane"
)

func (s *Server) migrateLegacyApprovals(username string) error {
	if s.controlPlane == nil || strings.TrimSpace(username) == "" {
		return nil
	}
	store := s.projectControl
	store.mu.Lock()
	defer store.mu.Unlock()
	state, exists, err := store.loadLocked(username)
	if err != nil || !exists {
		return err
	}
	if len(state.Checkpoints) == 0 && len(state.Decisions) == 0 {
		return nil
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	backupPath := store.pathFor(username) + ".pre-control-plane-v1.bak"
	if _, statErr := os.Stat(backupPath); errors.Is(statErr, os.ErrNotExist) {
		if err := projectControlWriteFileAtomically(backupPath, payload, 0400); err != nil {
			return err
		}
	}
	snapshot := legacyApprovalSnapshot(username, state, hex.EncodeToString(sum[:]))
	if err := s.controlPlane.ImportLegacyApprovals(context.Background(), snapshot); err != nil {
		return err
	}
	state.Checkpoints = []projectControlCheckpoint{}
	state.Decisions = []projectControlDecision{}
	return store.saveLocked(username, state)
}

func legacyApprovalSnapshot(username string, state projectControlState, sourceHash string) controlplane.LegacyApprovalSnapshot {
	snapshot := controlplane.LegacyApprovalSnapshot{
		Username: username, SourceHash: sourceHash, SourceUpdatedAt: state.UpdatedAt,
	}
	for _, checkpoint := range state.Checkpoints {
		inputPayload, _ := json.Marshal([]string{checkpoint.TaskID, checkpoint.Kind, checkpoint.Title, checkpoint.Reason})
		inputSum := sha256.Sum256(inputPayload)
		snapshot.Checkpoints = append(snapshot.Checkpoints, controlplane.LegacyCheckpoint{
			ID: checkpoint.ID, TaskID: checkpoint.TaskID, Kind: checkpoint.Kind, Title: checkpoint.Title,
			Reason: checkpoint.Reason, Status: checkpoint.Status, RequestedAt: checkpoint.RequestedAt,
			AllowedActions: checkpoint.AllowedActions, DecisionSummary: checkpoint.DecisionSummary,
			RowVersion: checkpoint.RowVersion, InputHash: "sha256:" + hex.EncodeToString(inputSum[:]),
		})
	}
	for _, decision := range state.Decisions {
		snapshot.Decisions = append(snapshot.Decisions, controlplane.LegacyDecision{
			ID: decision.ID, DecisionType: decision.DecisionType, Actor: decision.Actor, Timestamp: decision.Timestamp,
			Summary: decision.Summary, TaskID: decision.TaskID, CheckpointID: decision.CheckpointID,
		})
	}
	for _, event := range state.Events {
		if event.CheckpointID != "" {
			snapshot.AuditEvents = append(snapshot.AuditEvents, controlplane.LegacyAuditEvent{
				ID: event.ID, Timestamp: event.Timestamp, Actor: event.Actor, Action: event.Action,
				Detail: event.Detail, CheckpointID: event.CheckpointID,
			})
		}
	}
	return snapshot
}
