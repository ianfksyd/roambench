package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/controlplane"
)

func TestRuntimeCheckpointCreationUsesSQLiteWithoutJSONOwnership(t *testing.T) {
	srv, sessionToken, sessions := newInteractionAPIServer(t, t.TempDir())
	defer sessions.Stop()
	agentToken := issueAgentToken(t, srv, sessionToken)

	reason := "Need a human decision about the deployment target"
	body := `{"taskId":"task-runtime-checkpoint","reason":"` + reason + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/checkpoint", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST checkpoint status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	interactions, err := srv.controlPlane.ListInteractions(context.Background(), "ian", 500)
	if err != nil {
		t.Fatalf("ListInteractions: %v", err)
	}
	var requestID string
	for _, interaction := range interactions {
		if interaction.TaskID == "task-runtime-checkpoint" && interaction.RequestKind == "agent_request" && interaction.Summary == reason {
			requestID = interaction.RequestID
			if interaction.Status != "pending" {
				t.Fatalf("interaction status = %q, want pending", interaction.Status)
			}
			break
		}
	}
	if requestID == "" {
		t.Fatalf("runtime checkpoint was not written to SQLite: %#v", interactions)
	}

	state := readPersistedProjectControlState(t, srv.projectControl.pathFor("ian"))
	if len(state.Checkpoints) != 0 || len(state.Decisions) != 0 {
		t.Fatalf("JSON still owns approvals: checkpoints=%d decisions=%d", len(state.Checkpoints), len(state.Decisions))
	}

	outbox, err := srv.controlPlane.ListOutbox(context.Background(), "ian", 500)
	if err != nil {
		t.Fatalf("ListOutbox: %v", err)
	}
	if !hasRuntimeCheckpointOutbox(outbox, requestID, "interaction.created") {
		t.Fatalf("missing interaction.created outbox event for %q: %#v", requestID, outbox)
	}
}

func TestRuntimeCheckpointExpirationHydratesFromSQLiteAndPersistsUpdate(t *testing.T) {
	srv, _, sessions := newInteractionAPIServer(t, t.TempDir())
	defer sessions.Stop()

	const checkpointID = "checkpoint-runtime-expire"
	requestedAt := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Checkpoints = append(state.Checkpoints, projectControlCheckpoint{
			ID: checkpointID, TaskID: "task-runtime-expire", Kind: "permission",
			Title: "Expired runtime approval", Reason: "The approval window elapsed",
			Status: "pending", RequestedAt: requestedAt, AllowedActions: []string{"approve", "reject"}, RowVersion: 1,
		})
		return nil
	}); err != nil {
		t.Fatalf("create runtime checkpoint: %v", err)
	}

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for index := range state.Checkpoints {
			checkpoint := &state.Checkpoints[index]
			if checkpoint.ID != checkpointID {
				continue
			}
			checkpoint.Status = "expired"
			checkpoint.DecisionSummary = "Approval request expired"
			checkpoint.AllowedActions = nil
			checkpoint.RowVersion++
			return nil
		}
		return context.Canceled // proves the checkpoint was not hydrated
	}); err != nil {
		t.Fatalf("expire hydrated checkpoint: %v", err)
	}

	interaction, err := srv.controlPlane.GetInteraction(context.Background(), "ian", checkpointID)
	if err != nil {
		t.Fatalf("GetInteraction: %v", err)
	}
	if interaction.Status != "expired" || interaction.RowVersion != 2 {
		t.Fatalf("expired interaction = status %q rowVersion %d, want expired/2", interaction.Status, interaction.RowVersion)
	}
	state := readPersistedProjectControlState(t, srv.projectControl.pathFor("ian"))
	if len(state.Checkpoints) != 0 || len(state.Decisions) != 0 {
		t.Fatalf("JSON still owns approvals: checkpoints=%d decisions=%d", len(state.Checkpoints), len(state.Decisions))
	}
	outbox, err := srv.controlPlane.ListOutbox(context.Background(), "ian", 500)
	if err != nil {
		t.Fatalf("ListOutbox: %v", err)
	}
	if !hasRuntimeCheckpointOutbox(outbox, checkpointID, "interaction.expired") {
		t.Fatalf("missing interaction.expired outbox event for %q: %#v", checkpointID, outbox)
	}
}

func readPersistedProjectControlState(t *testing.T, path string) projectControlState {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read project control state: %v", err)
	}
	var state projectControlState
	if err := json.Unmarshal(payload, &state); err != nil {
		t.Fatalf("decode project control state: %v", err)
	}
	return state
}

func hasRuntimeCheckpointOutbox(events []controlplane.OutboxEvent, aggregateID, eventType string) bool {
	for _, event := range events {
		if event.AggregateID == aggregateID && event.EventType == eventType {
			return true
		}
	}
	return false
}
