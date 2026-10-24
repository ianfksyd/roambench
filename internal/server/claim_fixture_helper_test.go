package server

import (
	"testing"
)

// muteSeedTasks hides the demo-seeded tasks from the claim loop: tests that
// exercise claiming create their own task and must not race with the seeds.
// Needed since human-raised tasks without an acceptance command are now
// claimable (the seeds are exactly that shape).
func muteSeedTasks(t *testing.T, srv *Server) {
	t.Helper()
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Tasks {
			if state.Tasks[i].DispatchState == "ready" {
				state.Tasks[i].DispatchState = "none"
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
