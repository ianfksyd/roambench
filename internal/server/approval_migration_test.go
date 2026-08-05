package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
)

func TestLegacyApprovalsMigrateToSQLiteAndLeaveReadOnlyBackup(t *testing.T) {
	persistDir := t.TempDir()
	legacyStore := newProjectControlStore(persistDir)
	state := defaultProjectControlState()
	state.Checkpoints = append(state.Checkpoints, projectControlCheckpoint{
		ID: "checkpoint-resolved-migration", TaskID: projectControlTaskPanelID,
		Kind: "agent_request", Title: "Resolved request", Reason: "Choose a path",
		Status: "approved", RequestedAt: "2026-08-05T01:00:00Z",
		ResolvedByDecisionID: "decision-resolved-migration", AllowedActions: []string{},
		DecisionSummary: "Accepted by human operator", RowVersion: 2,
	})
	state.Decisions = append(state.Decisions, projectControlDecision{
		ID: "decision-resolved-migration", DecisionType: "checkpoint_approved", Actor: "human",
		Timestamp: "2026-08-05T01:01:00Z", Summary: "Accepted by human operator",
		TaskID: projectControlTaskPanelID, CheckpointID: "checkpoint-resolved-migration",
	})
	if err := legacyStore.save("ian", state); err != nil {
		t.Fatalf("save legacy state: %v", err)
	}
	legacyPath := legacyStore.pathFor("ian")

	srv, sessionToken, sessions := newInteractionAPIServer(t, persistDir)
	defer sessions.Stop()

	cleaned, exists, err := projectControlReadStateFile(legacyPath)
	if err != nil || !exists {
		t.Fatalf("read cleaned state: exists=%v err=%v", exists, err)
	}
	if len(cleaned.Checkpoints) != 0 || len(cleaned.Decisions) != 0 {
		t.Fatalf("JSON still owns approvals after migration: checkpoints=%d decisions=%d", len(cleaned.Checkpoints), len(cleaned.Decisions))
	}

	backupPath := legacyPath + ".pre-control-plane-v1.bak"
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("stat migration backup: %v", err)
	}
	if backupInfo.Mode().Perm()&0222 != 0 {
		t.Fatalf("migration backup mode = %o, want read-only", backupInfo.Mode().Perm())
	}
	backup, exists, err := projectControlReadStateFile(backupPath)
	if err != nil || !exists || len(backup.Checkpoints) != 2 || len(backup.Decisions) != 1 {
		t.Fatalf("backup approvals = checkpoints %d decisions %d, exists=%v err=%v", len(backup.Checkpoints), len(backup.Decisions), exists, err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET snapshot status = %d: %s", rec.Code, rec.Body.String())
	}
	var snapshot projectControlSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	seen := map[string]string{}
	for _, checkpoint := range snapshot.Checkpoints {
		seen[checkpoint.ID] = checkpoint.Status
	}
	if seen[projectControlCheckpointAcceptanceID] != "pending" || seen["checkpoint-resolved-migration"] != "approved" {
		t.Fatalf("migrated checkpoints in compatibility snapshot = %#v", seen)
	}
	if len(snapshot.Decisions) != 1 || snapshot.Decisions[0].ID != "decision-resolved-migration" {
		t.Fatalf("migrated decisions = %#v", snapshot.Decisions)
	}
}

func TestLegacyApprovalMigrationRecoversAfterDatabaseCommitBeforeJSONCleanup(t *testing.T) {
	persistDir := t.TempDir()
	legacyStore := newProjectControlStore(persistDir)
	state := defaultProjectControlState()
	if err := legacyStore.save("ian", state); err != nil {
		t.Fatalf("save legacy state: %v", err)
	}
	legacyPath := legacyStore.pathFor("ian")

	first, _, firstSessions := newInteractionAPIServer(t, persistDir)
	if err := first.controlPlane.Close(); err != nil {
		t.Fatalf("close first control plane: %v", err)
	}
	firstSessions.Stop()

	backupPath := legacyPath + ".pre-control-plane-v1.bak"
	backupPayload, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if err := os.WriteFile(legacyPath, backupPayload, 0600); err != nil {
		t.Fatalf("restore interrupted legacy JSON: %v", err)
	}

	restarted, _, restartedSessions := newInteractionAPIServer(t, persistDir)
	defer restartedSessions.Stop()
	cleaned, exists, err := projectControlReadStateFile(legacyPath)
	if err != nil || !exists || len(cleaned.Checkpoints) != 0 || len(cleaned.Decisions) != 0 {
		t.Fatalf("recovery did not clean JSON: exists=%v checkpoints=%d decisions=%d err=%v", exists, len(cleaned.Checkpoints), len(cleaned.Decisions), err)
	}
	interactions, err := restarted.controlPlane.ListInteractions(context.Background(), "ian", 100)
	if err != nil {
		t.Fatalf("ListInteractions: %v", err)
	}
	if len(interactions) != 1 || interactions[0].RequestID != projectControlCheckpointAcceptanceID {
		t.Fatalf("interactions after recovery = %#v", interactions)
	}
	events, err := restarted.controlPlane.ListOutbox(context.Background(), "ian", 100)
	if err != nil {
		t.Fatalf("ListOutbox: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("outbox events after idempotent recovery = %d, want 1", len(events))
	}
}
