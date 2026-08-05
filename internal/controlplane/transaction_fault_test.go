package controlplane

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateInteractionRollsBackWhenOutboxInsertFails(t *testing.T) {
	repo := openFaultTestRepository(t)
	installFaultTrigger(t, repo, `CREATE TRIGGER inject_outbox_failure BEFORE INSERT ON outbox_events
		WHEN NEW.event_type = 'interaction.created'
		BEGIN SELECT RAISE(ABORT, 'injected create failure'); END`)

	request := testPermissionRequest()
	request.ActorScope = "adapter:generic"
	request.IdempotencyKey = "create-fault-retry"
	if _, err := repo.CreateInteraction(context.Background(), request); err == nil {
		t.Fatal("CreateInteraction succeeded with injected outbox failure")
	}
	assertTableCount(t, repo, "interactions", 0)
	assertTableCount(t, repo, "audit_events", 0)
	assertTableCount(t, repo, "outbox_events", 0)
	assertTableCount(t, repo, "post_idempotency_keys", 0)

	dropFaultTrigger(t, repo)
	created, err := repo.CreateInteraction(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateInteraction retry: %v", err)
	}
	if created.Status != "pending" || created.RowVersion != 1 {
		t.Fatalf("retry interaction = status %q rowVersion %d", created.Status, created.RowVersion)
	}
	assertTableCount(t, repo, "interactions", 1)
	assertTableCount(t, repo, "audit_events", 1)
	assertTableCount(t, repo, "outbox_events", 1)
	assertTableCount(t, repo, "post_idempotency_keys", 1)
}

func TestRespondRollsBackWhenFinalOutboxInsertFails(t *testing.T) {
	repo := openFaultTestRepository(t)
	created, err := repo.CreateInteraction(context.Background(), testPermissionRequest())
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}
	installFaultTrigger(t, repo, `CREATE TRIGGER inject_outbox_failure BEFORE INSERT ON outbox_events
		WHEN NEW.event_type = 'project_task_projection.requested'
		BEGIN SELECT RAISE(ABORT, 'injected response failure'); END`)
	input := RespondInput{
		Action: "approve_once", Actor: "ian", DeviceID: "phone", ExpectedRowVersion: 1,
		IdempotencyKey: "response-fault-retry", InputHash: created.InputHash,
	}
	if _, _, err := repo.Respond(context.Background(), "ian", created.RequestID, input); err == nil {
		t.Fatal("Respond succeeded with injected final outbox failure")
	}
	assertInteractionState(t, repo, created.RequestID, "pending", 1)
	assertTableCount(t, repo, "responses", 0)
	assertTableCount(t, repo, "decisions", 0)
	assertTableCount(t, repo, "task_projections", 0)
	assertTableCount(t, repo, "idempotency_keys", 0)
	assertTableCount(t, repo, "audit_events", 1)
	assertTableCount(t, repo, "outbox_events", 1)

	dropFaultTrigger(t, repo)
	response, replayed, err := repo.Respond(context.Background(), "ian", created.RequestID, input)
	if err != nil || replayed {
		t.Fatalf("Respond retry = replayed %v, err %v", replayed, err)
	}
	if response.Action != "approve_once" {
		t.Fatalf("retry response action = %q", response.Action)
	}
	assertInteractionState(t, repo, created.RequestID, "resolved", 2)
	assertTableCount(t, repo, "responses", 1)
	assertTableCount(t, repo, "decisions", 1)
	assertTableCount(t, repo, "task_projections", 1)
	assertTableCount(t, repo, "idempotency_keys", 1)
}

func TestExpiryRollsBackWhenOutboxInsertFails(t *testing.T) {
	repo := openFaultTestRepository(t)
	request := testPermissionRequest()
	request.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	created, err := repo.CreateInteraction(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}
	installFaultTrigger(t, repo, `CREATE TRIGGER inject_outbox_failure BEFORE INSERT ON outbox_events
		WHEN NEW.event_type = 'interaction.expired'
		BEGIN SELECT RAISE(ABORT, 'injected expiry failure'); END`)
	if _, err := repo.ExpireDueInteractions(context.Background(), "ian", time.Now().UTC()); err == nil {
		t.Fatal("ExpireDueInteractions succeeded with injected outbox failure")
	}
	assertInteractionState(t, repo, created.RequestID, "pending", 1)
	assertTableCount(t, repo, "audit_events", 1)
	assertTableCount(t, repo, "outbox_events", 1)

	dropFaultTrigger(t, repo)
	expired, err := repo.ExpireDueInteractions(context.Background(), "ian", time.Now().UTC())
	if err != nil {
		t.Fatalf("ExpireDueInteractions retry: %v", err)
	}
	if len(expired) != 1 || expired[0].RequestID != created.RequestID {
		t.Fatalf("expired retry result = %#v", expired)
	}
	assertInteractionState(t, repo, created.RequestID, "expired", 2)
	assertTableCount(t, repo, "audit_events", 2)
	assertTableCount(t, repo, "outbox_events", 2)
}

func TestSessionCancellationBatchRollsBackWhenSecondOutboxInsertFails(t *testing.T) {
	repo := openFaultTestRepository(t)
	firstRequest := testPermissionRequest()
	firstRequest.VendorRequestID = "session-cancel-first"
	first, err := repo.CreateInteraction(context.Background(), firstRequest)
	if err != nil {
		t.Fatalf("CreateInteraction first: %v", err)
	}
	secondRequest := testPermissionRequest()
	secondRequest.VendorRequestID = "session-cancel-second"
	second, err := repo.CreateInteraction(context.Background(), secondRequest)
	if err != nil {
		t.Fatalf("CreateInteraction second: %v", err)
	}
	installFaultTrigger(t, repo, `CREATE TRIGGER inject_outbox_failure BEFORE INSERT ON outbox_events
		WHEN NEW.event_type = 'interaction.cancelled'
		AND (SELECT COUNT(*) FROM outbox_events WHERE event_type = 'interaction.cancelled') = 1
		BEGIN SELECT RAISE(ABORT, 'injected second cancellation failure'); END`)
	if _, err := repo.CancelPendingInteractionsForSession(context.Background(), "ian", "session-1", "session ended"); err == nil {
		t.Fatal("CancelPendingInteractionsForSession succeeded with injected second outbox failure")
	}
	assertInteractionState(t, repo, first.RequestID, "pending", 1)
	assertInteractionState(t, repo, second.RequestID, "pending", 1)
	assertEventTypeCount(t, repo, "interaction.cancelled", 0)
	assertTableCount(t, repo, "audit_events", 2)

	dropFaultTrigger(t, repo)
	cancelled, err := repo.CancelPendingInteractionsForSession(context.Background(), "ian", "session-1", "session ended")
	if err != nil {
		t.Fatalf("CancelPendingInteractionsForSession retry: %v", err)
	}
	if len(cancelled) != 2 {
		t.Fatalf("cancelled retry count = %d, want 2", len(cancelled))
	}
	assertInteractionState(t, repo, first.RequestID, "cancelled", 2)
	assertInteractionState(t, repo, second.RequestID, "cancelled", 2)
	assertEventTypeCount(t, repo, "interaction.cancelled", 2)
	assertTableCount(t, repo, "audit_events", 4)
}

func TestLegacyImportRollsBackWhenOutboxInsertFails(t *testing.T) {
	repo := openFaultTestRepository(t)
	installFaultTrigger(t, repo, `CREATE TRIGGER inject_outbox_failure BEFORE INSERT ON outbox_events
		WHEN NEW.event_type = 'interaction.created'
		BEGIN SELECT RAISE(ABORT, 'injected migration failure'); END`)
	snapshot := LegacyApprovalSnapshot{
		Username: "ian", SourceHash: "sha256:legacy-fault", SourceUpdatedAt: "2026-08-05T01:00:00Z",
		Checkpoints: []LegacyCheckpoint{{
			ID: "checkpoint-legacy-fault", TaskID: "task-1", Kind: "permission", Title: "Approve?",
			Reason: "Legacy request", Status: "pending", RequestedAt: "2026-08-05T01:00:00Z",
			AllowedActions: []string{"approve", "reject"}, RowVersion: 1, InputHash: "sha256:legacy",
		}},
	}
	if err := repo.ImportLegacyApprovals(context.Background(), snapshot); err == nil {
		t.Fatal("ImportLegacyApprovals succeeded with injected outbox failure")
	}
	assertTableCount(t, repo, "interactions", 0)
	assertTableCount(t, repo, "outbox_events", 0)
	assertSQLiteObjectCount(t, repo, "table", "legacy_approval_migrations", 0)

	dropFaultTrigger(t, repo)
	if err := repo.ImportLegacyApprovals(context.Background(), snapshot); err != nil {
		t.Fatalf("ImportLegacyApprovals retry: %v", err)
	}
	assertTableCount(t, repo, "interactions", 1)
	assertTableCount(t, repo, "outbox_events", 1)
	assertTableCount(t, repo, "legacy_approval_migrations", 1)
}

func openFaultTestRepository(t *testing.T) *Repository {
	t.Helper()
	repo, err := Open(filepath.Join(t.TempDir(), "control-plane.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return repo
}

func installFaultTrigger(t *testing.T, repo *Repository, statement string) {
	t.Helper()
	if _, err := repo.db.Exec(statement); err != nil {
		t.Fatalf("install fault trigger: %v", err)
	}
}

func dropFaultTrigger(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := repo.db.Exec(`DROP TRIGGER inject_outbox_failure`); err != nil {
		t.Fatalf("drop fault trigger: %v", err)
	}
}

func assertTableCount(t *testing.T, repo *Repository, table string, want int) {
	t.Helper()
	allowed := map[string]bool{
		"interactions": true, "responses": true, "decisions": true, "audit_events": true,
		"outbox_events": true, "idempotency_keys": true, "post_idempotency_keys": true,
		"task_projections": true, "legacy_approval_migrations": true,
	}
	if !allowed[table] {
		t.Fatalf("unsupported count table %q", table)
	}
	var got int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}

func assertEventTypeCount(t *testing.T, repo *Repository, eventType string, want int) {
	t.Helper()
	var got int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE event_type=?`, eventType).Scan(&got); err != nil {
		t.Fatalf("count outbox event %q: %v", eventType, err)
	}
	if got != want {
		t.Fatalf("outbox event %q count = %d, want %d", eventType, got, want)
	}
}

func assertSQLiteObjectCount(t *testing.T, repo *Repository, objectType, name string, want int) {
	t.Helper()
	var got int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type=? AND name=?`, objectType, name).Scan(&got); err != nil {
		t.Fatalf("count sqlite object %s %q: %v", objectType, name, err)
	}
	if got != want {
		t.Fatalf("sqlite object %s %q count = %d, want %d", objectType, name, got, want)
	}
}

func assertInteractionState(t *testing.T, repo *Repository, requestID, wantStatus string, wantRowVersion int) {
	t.Helper()
	interaction, err := repo.GetInteraction(context.Background(), "ian", requestID)
	if err != nil {
		t.Fatalf("GetInteraction(%s): %v", requestID, err)
	}
	if interaction.Status != wantStatus || interaction.RowVersion != wantRowVersion {
		t.Fatalf("interaction %s = status %q rowVersion %d, want %q/%d", requestID, interaction.Status, interaction.RowVersion, wantStatus, wantRowVersion)
	}
}
