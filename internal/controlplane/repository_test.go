package controlplane

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testPermissionRequest() CreateInteraction {
	return CreateInteraction{
		Username:        "ian",
		TaskID:          "task-1",
		RuntimeID:       "runtime-1",
		SessionID:       "session-1",
		AdapterKind:     "generic",
		VendorRequestID: "vendor-1",
		RequestKind:     "permission",
		RiskClass:       "R1",
		Title:           "Allow command?",
		Summary:         "Run the requested command",
		Preview:         "go test ./...",
		AllowedActions:  []string{"approve_once", "reject", "reject_with_feedback"},
		ResponseSchema: ResponseSchema{
			Type:              "action",
			MaxFeedbackLength: 500,
		},
		InputHash: "sha256:abc",
	}
}

func TestCreateInteractionPersistsRequestAndOutboxAtomically(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "control-plane.sqlite")
	repo, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	created, err := repo.CreateInteraction(context.Background(), testPermissionRequest())
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}
	if created.Status != "pending" || created.RowVersion != 1 {
		t.Fatalf("created status/version = %q/%d, want pending/1", created.Status, created.RowVersion)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	got, err := reopened.GetInteraction(context.Background(), "ian", created.RequestID)
	if err != nil {
		t.Fatalf("GetInteraction after reopen: %v", err)
	}
	if got.InputHash != "sha256:abc" || got.Preview != "go test ./..." {
		t.Fatalf("reopened interaction = %#v", got)
	}
	events, err := reopened.ListOutbox(context.Background(), "ian", 10)
	if err != nil {
		t.Fatalf("ListOutbox: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "interaction.created" || events[0].AggregateID != created.RequestID {
		t.Fatalf("outbox events = %#v", events)
	}
}

func TestLegacyCheckpointRuntimeUpdatesAreIdempotentAndCannotReopenTerminalState(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "control-plane.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer repo.Close()
	ctx := context.Background()
	checkpoint := LegacyCheckpoint{
		ID: "checkpoint-runtime", TaskID: "task-1", Kind: "permission", Title: "Approve?", Reason: "Runtime request",
		Status: "pending", RequestedAt: "2026-08-05T01:00:00Z", AllowedActions: []string{"approve", "reject"}, RowVersion: 1,
		InputHash: "sha256:runtime",
	}
	importCheckpoint := func(sourceHash string, value LegacyCheckpoint) {
		t.Helper()
		if err := repo.ImportLegacyApprovals(ctx, LegacyApprovalSnapshot{
			Username: "ian", SourceHash: sourceHash, SourceUpdatedAt: "2026-08-05T01:00:00Z", Checkpoints: []LegacyCheckpoint{value},
		}); err != nil {
			t.Fatalf("ImportLegacyApprovals(%s): %v", sourceHash, err)
		}
	}
	importCheckpoint("pending", checkpoint)
	checkpoint.Status = "expired"
	checkpoint.AllowedActions = nil
	checkpoint.RowVersion = 2
	importCheckpoint("expired", checkpoint)
	importCheckpoint("expired-replay", checkpoint)

	stale := checkpoint
	stale.Status = "pending"
	stale.AllowedActions = []string{"approve", "reject"}
	stale.RowVersion = 3
	importCheckpoint("stale-pending", stale)

	interaction, err := repo.GetInteraction(ctx, "ian", checkpoint.ID)
	if err != nil {
		t.Fatalf("GetInteraction: %v", err)
	}
	if interaction.Status != "expired" || interaction.RowVersion != 2 {
		t.Fatalf("interaction after replay/stale write = status %q rowVersion %d, want expired/2", interaction.Status, interaction.RowVersion)
	}
	events, err := repo.ListOutbox(ctx, "ian", 10)
	if err != nil {
		t.Fatalf("ListOutbox: %v", err)
	}
	if len(events) != 2 || events[0].EventType != "interaction.created" || events[1].EventType != "interaction.expired" {
		t.Fatalf("outbox events = %#v, want one created and one expired", events)
	}
}

func TestConcurrentResponsesProduceExactlyOneFinalDecision(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "control-plane.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer repo.Close()
	created, err := repo.CreateInteraction(context.Background(), testPermissionRequest())
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}

	inputs := []RespondInput{
		{Action: "approve_once", Actor: "ian", DeviceID: "phone", ExpectedRowVersion: 1, IdempotencyKey: "phone-1", InputHash: "sha256:abc"},
		{Action: "reject", Actor: "ian", DeviceID: "desktop", ExpectedRowVersion: 1, IdempotencyKey: "desktop-1", InputHash: "sha256:abc"},
	}
	var wg sync.WaitGroup
	results := make(chan error, len(inputs))
	for _, input := range inputs {
		input := input
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, respondErr := repo.Respond(context.Background(), "ian", created.RequestID, input)
			results <- respondErr
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for respondErr := range results {
		switch {
		case respondErr == nil:
			successes++
		case errors.Is(respondErr, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected response error: %v", respondErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes/conflicts = %d/%d, want 1/1", successes, conflicts)
	}
	responses, err := repo.ListResponses(context.Background(), "ian", created.RequestID)
	if err != nil {
		t.Fatalf("ListResponses: %v", err)
	}
	if len(responses) != 1 {
		t.Fatalf("responses = %d, want 1", len(responses))
	}
}

func TestResponseRetryWithSameIdempotencyKeyReturnsOriginalResult(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "control-plane.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer repo.Close()
	created, err := repo.CreateInteraction(context.Background(), testPermissionRequest())
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}
	input := RespondInput{Action: "approve_once", Actor: "ian", DeviceID: "phone", ExpectedRowVersion: 1, IdempotencyKey: "retry-key", InputHash: "sha256:abc"}

	first, replayed, err := repo.Respond(context.Background(), "ian", created.RequestID, input)
	if err != nil || replayed {
		t.Fatalf("first Respond = replayed %v, err %v", replayed, err)
	}
	second, replayed, err := repo.Respond(context.Background(), "ian", created.RequestID, input)
	if err != nil || !replayed {
		t.Fatalf("second Respond = replayed %v, err %v", replayed, err)
	}
	if second.ResponseID != first.ResponseID || second.Action != first.Action {
		t.Fatalf("replayed response = %#v, want original %#v", second, first)
	}
}

func TestResponseTransactionQueuesTaskProjection(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "control-plane.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer repo.Close()
	request := testPermissionRequest()
	request.RequestKind = "final_acceptance"
	created, err := repo.CreateInteraction(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}
	response, _, err := repo.Respond(context.Background(), "ian", created.RequestID, RespondInput{
		Action: "approve_once", Actor: "ian", ExpectedRowVersion: 1,
		IdempotencyKey: "projection-decision", InputHash: "sha256:abc",
	})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	projections, err := repo.ListPendingTaskProjections(context.Background(), "ian", 10)
	if err != nil {
		t.Fatalf("ListPendingTaskProjections: %v", err)
	}
	if len(projections) != 1 || projections[0].DecisionID != response.ResponseID || projections[0].RequestID != created.RequestID {
		t.Fatalf("pending projections = %#v", projections)
	}
	events, err := repo.ListOutbox(context.Background(), "ian", 20)
	if err != nil {
		t.Fatalf("ListOutbox: %v", err)
	}
	found := false
	for _, event := range events {
		if event.EventType == "project_task_projection.requested" && event.AggregateID == created.RequestID {
			found = true
		}
	}
	if !found {
		t.Fatalf("project_task_projection.requested missing from outbox: %#v", events)
	}
}

func TestQuestionSchemaRejectsUnknownOptionAndAcceptsDeclaredChoice(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "control-plane.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer repo.Close()
	request := testPermissionRequest()
	request.RequestKind = "question_single"
	request.VendorRequestID = "question-1"
	request.AllowedActions = []string{"answer"}
	request.ResponseSchema = ResponseSchema{
		Type:    "single_choice",
		Options: []ResponseOption{{ID: "safe", Label: "Safe"}, {ID: "fast", Label: "Fast"}},
	}
	created, err := repo.CreateInteraction(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}

	_, _, err = repo.Respond(context.Background(), "ian", created.RequestID, RespondInput{
		Action: "answer", SelectedOptionIDs: []string{"unknown"}, Actor: "ian", ExpectedRowVersion: 1, IdempotencyKey: "bad-choice", InputHash: "sha256:abc",
	})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("unknown option error = %v, want ErrValidation", err)
	}
	response, _, err := repo.Respond(context.Background(), "ian", created.RequestID, RespondInput{
		Action: "answer", SelectedOptionIDs: []string{"safe"}, Actor: "ian", ExpectedRowVersion: 1, IdempotencyKey: "good-choice", InputHash: "sha256:abc",
	})
	if err != nil {
		t.Fatalf("declared choice Respond: %v", err)
	}
	if len(response.SelectedOptionIDs) != 1 || response.SelectedOptionIDs[0] != "safe" {
		t.Fatalf("selected options = %#v", response.SelectedOptionIDs)
	}
}

func TestWaitReturnsResolutionAndCancelSurvivesReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "control-plane.sqlite")
	repo, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	created, err := repo.CreateInteraction(context.Background(), testPermissionRequest())
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}

	go func() {
		time.Sleep(25 * time.Millisecond)
		_, _, _ = repo.Respond(context.Background(), "ian", created.RequestID, RespondInput{
			Action: "reject", Actor: "ian", ExpectedRowVersion: 1, IdempotencyKey: "wait-response", InputHash: "sha256:abc",
		})
	}()
	resolved, err := repo.WaitInteraction(context.Background(), "ian", created.RequestID, time.Second)
	if err != nil {
		t.Fatalf("WaitInteraction: %v", err)
	}
	if resolved.Status != "resolved" || resolved.FinalAction != "reject" {
		t.Fatalf("resolved interaction = %#v", resolved)
	}

	request := testPermissionRequest()
	request.VendorRequestID = "cancel-1"
	cancellable, err := repo.CreateInteraction(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateInteraction cancellable: %v", err)
	}
	cancelled, err := repo.CancelInteraction(context.Background(), "ian", cancellable.RequestID, 1, "adapter stopped")
	if err != nil {
		t.Fatalf("CancelInteraction: %v", err)
	}
	if cancelled.Status != "cancelled" || cancelled.RowVersion != 2 {
		t.Fatalf("cancelled = %#v", cancelled)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	persisted, err := reopened.GetInteraction(context.Background(), "ian", cancellable.RequestID)
	if err != nil {
		t.Fatalf("GetInteraction: %v", err)
	}
	if persisted.Status != "cancelled" {
		t.Fatalf("persisted status = %q, want cancelled", persisted.Status)
	}
}
