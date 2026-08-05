package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/controlplane"
)

func TestMobileDecisionProjectsTaskBeforeResponseReturns(t *testing.T) {
	persistDir := t.TempDir()
	store := newProjectControlStore(persistDir)
	state := defaultProjectControlState()
	for index := range state.Tasks {
		if state.Tasks[index].ID == projectControlTaskPanelID {
			state.Tasks[index].State = "execution_complete"
			state.Tasks[index].AcceptanceStatus = "awaiting_acceptance"
		}
	}
	if err := store.save("ian", state); err != nil {
		t.Fatalf("save project state: %v", err)
	}
	srv, sessionToken, sessions := newInteractionAPIServer(t, persistDir)
	defer sessions.Stop()
	interaction, err := srv.controlPlane.CreateInteraction(context.Background(), controlplane.CreateInteraction{
		Username: "ian", TaskID: projectControlTaskPanelID, RuntimeID: "runtime-1", SessionID: "session-1",
		AdapterKind: "generic", VendorRequestID: "immediate-projection", RequestKind: "final_acceptance",
		RiskClass: "R1", Title: "Final acceptance", Summary: "Review completed work", Preview: "diff summary",
		AllowedActions: []string{"approve_once", "reject"}, ResponseSchema: controlplane.ResponseSchema{Type: "action"}, InputHash: "sha256:immediate",
	})
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}
	body := `{"action":"approve_once","expectedRowVersion":1,"idempotencyKey":"immediate-decision","inputHash":"sha256:immediate"}`
	req := httptest.NewRequest(http.MethodPost, "/api/mobile/interactions/"+interaction.RequestID+"/respond", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST respond status = %d: %s", rec.Code, rec.Body.String())
	}
	projected, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatalf("load projected state: %v", err)
	}
	task := projectControlTaskFromStateByID(t, projected, projectControlTaskPanelID)
	if task.AcceptanceStatus != "accepted" {
		t.Fatalf("acceptance status after response = %q, want accepted", task.AcceptanceStatus)
	}
}

func TestPendingFinalAcceptanceProjectionResumesAfterServerRestart(t *testing.T) {
	persistDir := t.TempDir()
	store := newProjectControlStore(persistDir)
	state := defaultProjectControlState()
	for index := range state.Tasks {
		if state.Tasks[index].ID == projectControlTaskPanelID {
			state.Tasks[index].State = "execution_complete"
			state.Tasks[index].AcceptanceStatus = "awaiting_acceptance"
			state.Tasks[index].RowVersion = 7
		}
	}
	if err := store.save("ian", state); err != nil {
		t.Fatalf("save project state: %v", err)
	}

	first, _, firstSessions := newInteractionAPIServer(t, persistDir)
	interaction, err := first.controlPlane.CreateInteraction(context.Background(), controlplane.CreateInteraction{
		Username: "ian", TaskID: projectControlTaskPanelID, RuntimeID: "runtime-1", SessionID: "session-1",
		AdapterKind: "generic", VendorRequestID: "restart-projection", RequestKind: "final_acceptance",
		RiskClass: "R1", Title: "Final acceptance", Summary: "Review completed work", Preview: "diff summary",
		AllowedActions: []string{"approve_once", "reject"}, ResponseSchema: controlplane.ResponseSchema{Type: "action"},
		InputHash: "sha256:projection",
	})
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}
	response, _, err := first.controlPlane.Respond(context.Background(), "ian", interaction.RequestID, controlplane.RespondInput{
		Action: "approve_once", Actor: "ian", ExpectedRowVersion: 1,
		IdempotencyKey: "restart-projection-decision", InputHash: "sha256:projection",
	})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if err := first.controlPlane.Close(); err != nil {
		t.Fatalf("close first control plane: %v", err)
	}
	firstSessions.Stop()

	restarted, _, restartedSessions := newInteractionAPIServer(t, persistDir)
	defer restartedSessions.Stop()
	projected, err := restarted.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatalf("load projected state: %v", err)
	}
	task := projectControlTaskFromStateByID(t, projected, projectControlTaskPanelID)
	if task.AcceptanceStatus != "accepted" || task.AcceptanceDecisionID != response.ResponseID {
		t.Fatalf("projected task = %#v", task)
	}
	rowVersion := task.RowVersion
	if err := restarted.runPendingTaskProjections(context.Background(), "ian"); err != nil {
		t.Fatalf("rerun projector: %v", err)
	}
	afterReplay, err := restarted.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatalf("reload projected state: %v", err)
	}
	replayedTask := projectControlTaskFromStateByID(t, afterReplay, projectControlTaskPanelID)
	if replayedTask.RowVersion != rowVersion {
		t.Fatalf("row version after idempotent replay = %d, want %d", replayedTask.RowVersion, rowVersion)
	}
	projections, err := restarted.controlPlane.ListPendingTaskProjections(context.Background(), "ian", 10)
	if err != nil {
		t.Fatalf("ListPendingTaskProjections: %v", err)
	}
	if len(projections) != 0 {
		t.Fatalf("pending projections after restart = %#v", projections)
	}
}

func TestFailedTaskProjectionRetriesAfterMissingTaskIsRestored(t *testing.T) {
	persistDir := t.TempDir()
	srv, _, sessions := newInteractionAPIServer(t, persistDir)
	defer sessions.Stop()
	const taskID = "task-restored-after-projection-failure"
	interaction, err := srv.controlPlane.CreateInteraction(context.Background(), controlplane.CreateInteraction{
		Username: "ian", TaskID: taskID, RuntimeID: "runtime-1", SessionID: "session-1",
		AdapterKind: "generic", VendorRequestID: "missing-task-projection", RequestKind: "final_acceptance",
		RiskClass: "R1", Title: "Final acceptance", Summary: "Review completed work", Preview: "diff summary",
		AllowedActions: []string{"approve_once", "reject"}, ResponseSchema: controlplane.ResponseSchema{Type: "action"},
		InputHash: "sha256:missing-task-projection",
	})
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}
	response, _, err := srv.controlPlane.Respond(context.Background(), "ian", interaction.RequestID, controlplane.RespondInput{
		Action: "approve_once", Actor: "ian", ExpectedRowVersion: 1,
		IdempotencyKey: "missing-task-projection-decision", InputHash: interaction.InputHash,
	})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}

	if err := srv.runPendingTaskProjections(context.Background(), "ian"); err == nil || !strings.Contains(err.Error(), "not found for projection") {
		t.Fatalf("first projector error = %v, want missing task", err)
	}
	projections, err := srv.controlPlane.ListPendingTaskProjections(context.Background(), "ian", 10)
	if err != nil {
		t.Fatalf("ListPendingTaskProjections after failure: %v", err)
	}
	if len(projections) != 1 || projections[0].State != "failed" || projections[0].AttemptCount != 1 {
		t.Fatalf("failed projections = %#v", projections)
	}
	failedState, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatalf("load state after failed projection: %v", err)
	}
	projectionEventID := "event-projection-" + response.ResponseID
	for _, event := range failedState.Events {
		if event.ID == projectionEventID {
			t.Fatalf("failed projection left event %q in project state", projectionEventID)
		}
	}

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: taskID, ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamRuntimeID,
			Title: "Restored task", Goal: "Verify projection retry", State: "execution_complete",
			AcceptanceStatus: "awaiting_acceptance", RiskLevel: "medium", Priority: "high", RowVersion: 1,
		})
		return nil
	}); err != nil {
		t.Fatalf("restore missing task: %v", err)
	}
	if err := srv.runPendingTaskProjections(context.Background(), "ian"); err != nil {
		t.Fatalf("retry projector: %v", err)
	}
	recovered, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatalf("load recovered state: %v", err)
	}
	task := projectControlTaskFromStateByID(t, recovered, taskID)
	if task.AcceptanceStatus != "accepted" || task.AcceptanceDecisionID != response.ResponseID {
		t.Fatalf("recovered task = %#v", task)
	}
	rowVersion := task.RowVersion
	projections, err = srv.controlPlane.ListPendingTaskProjections(context.Background(), "ian", 10)
	if err != nil {
		t.Fatalf("ListPendingTaskProjections after recovery: %v", err)
	}
	if len(projections) != 0 {
		t.Fatalf("pending projections after recovery = %#v", projections)
	}
	if err := srv.runPendingTaskProjections(context.Background(), "ian"); err != nil {
		t.Fatalf("idempotent projector rerun: %v", err)
	}
	afterReplay, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatalf("load state after idempotent rerun: %v", err)
	}
	if got := projectControlTaskFromStateByID(t, afterReplay, taskID).RowVersion; got != rowVersion {
		t.Fatalf("row version after idempotent rerun = %d, want %d", got, rowVersion)
	}
}

func projectControlTaskFromStateByID(t *testing.T, state projectControlState, taskID string) projectControlTask {
	t.Helper()
	for _, task := range state.Tasks {
		if task.ID == taskID {
			return task
		}
	}
	t.Fatalf("task %q not found", taskID)
	return projectControlTask{}
}
