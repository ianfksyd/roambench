package controlplane

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestHandoffDraftMigrationPreservesLegacyPending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE pending_handoffs (
 id TEXT PRIMARY KEY, username TEXT NOT NULL, source_task_id TEXT NOT NULL DEFAULT '',
 source_terminal TEXT NOT NULL DEFAULT '', target_terminal TEXT NOT NULL DEFAULT '',
 fingerprint TEXT NOT NULL, snapshot TEXT NOT NULL DEFAULT '', copy_id TEXT NOT NULL DEFAULT '',
 promoted_task_id TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
 CREATE UNIQUE INDEX pending_handoffs_active_uniq ON pending_handoffs(username,target_terminal,copy_id) WHERE promoted_task_id='';
 INSERT INTO pending_handoffs VALUES('legacy','ian','source','src','dst','fp','legacy snapshot','copy','','2026-10-26');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	repo, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rows, err := repo.ListPendingHandoffs(ctx, "ian", "dst")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Snapshot != "legacy snapshot" || rows[0].DraftID != "" {
		t.Fatalf("legacy row lost: %+v", rows)
	}
	// The same copy may belong to two independently cancellable drafts.
	for _, draft := range []string{"a", "b", "a"} {
		if _, err := repo.RecordPendingHandoff(ctx, "ian", PendingHandoff{SourceTaskID: "source", SourceTerminal: "src", TargetTerminal: "dst", Fingerprint: "fp", CopyID: "copy", DraftID: draft}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	repo, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	rows, err = repo.ListPendingHandoffs(ctx, "ian", "dst")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("draft identities did not survive reopen: %+v", rows)
	}
	if err := repo.CancelHandoffDraft(ctx, "ian", "dst", "a"); err != nil {
		t.Fatal(err)
	}
	rows, err = repo.ListPendingHandoffs(ctx, "ian", "dst")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("cancel affected another draft: %+v", rows)
	}
}
