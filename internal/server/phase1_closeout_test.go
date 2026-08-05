package server

import (
	"context"
	"os"
	"testing"
)

func TestEmptyProjectControlFixtureStartsWithHealthyEmptyGateway(t *testing.T) {
	payload, err := os.ReadFile("testdata/empty-project-control.json")
	if err != nil {
		t.Fatalf("read empty fixture: %v", err)
	}
	persistDir := t.TempDir()
	store := newProjectControlStore(persistDir)
	if err := projectControlWriteFileAtomically(store.pathFor("ian"), payload, 0600); err != nil {
		t.Fatalf("write empty fixture: %v", err)
	}

	srv, _, sessions := newInteractionAPIServer(t, persistDir)
	defer sessions.Stop()
	if srv.controlPlaneErr != nil {
		t.Fatalf("control plane startup error: %v", srv.controlPlaneErr)
	}
	interactions, err := srv.controlPlane.ListAllInteractions(context.Background(), "ian")
	if err != nil {
		t.Fatalf("ListAllInteractions: %v", err)
	}
	if len(interactions) != 0 {
		t.Fatalf("interactions from empty fixture = %d, want 0", len(interactions))
	}
	state := readPersistedProjectControlState(t, store.pathFor("ian"))
	if len(state.Checkpoints) != 0 || len(state.Decisions) != 0 {
		t.Fatalf("empty fixture gained JSON approvals: checkpoints=%d decisions=%d", len(state.Checkpoints), len(state.Decisions))
	}
}
