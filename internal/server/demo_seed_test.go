package server

import (
	"testing"
	"time"
)

// Test-only fixture: the historical demo/prototype seed data. RoamBench ships
// with an empty default state (no demo data); tests that need a populated
// project/workstream/task set call demoProjectControlState() explicitly so the
// production seed stays clean. Kept out of non-test code on purpose.

func demoProjectControlState() projectControlState {
	now := time.Now().UTC()
	stampA := now.Add(-2 * time.Hour).Format(time.RFC3339)
	stampB := now.Add(-90 * time.Minute).Format(time.RFC3339)
	stampC := now.Add(-85 * time.Minute).Format(time.RFC3339)
	stampD := now.Add(-70 * time.Minute).Format(time.RFC3339)
	stampE := now.Add(-50 * time.Minute).Format(time.RFC3339)
	stampF := now.Add(-45 * time.Minute).Format(time.RFC3339)

	state := projectControlState{
		ActiveProjectID: projectControlProjectID,
		Projects: []projectControlProject{{
			ID:          projectControlProjectID,
			Key:         "roambench",
			Name:        "RoamBench Control Plane",
			Description: "Incremental project-control prototype layered on top of the existing terminal workspace.",
			Status:      "active",
			CurrentGoal: "Validate the dual-mode Terminal / Project Panel information architecture without breaking the current terminal UX.",
			RowVersion:  1,
		}},
		Workstreams: []projectControlWorkstream{
			{ID: projectControlWorkstreamUXID, ProjectID: projectControlProjectID, Title: "UI & Information Architecture", Description: "Project dashboard, workstream board, task detail, and session detail navigation.", Priority: "high", Status: "running", ScopeSummary: "Keep Terminal as the default mode and add Project Panel as a progressive drill-down control surface.", RowVersion: 1},
			{ID: projectControlWorkstreamRuntimeID, ProjectID: projectControlProjectID, Title: "Runtime Integration", Description: "Capability-gated terminal attach, approvals inbox, and runtime health visibility.", Priority: "high", Status: "running", ScopeSummary: "Reuse existing tmux/local shell infrastructure as the first runtime adapter.", RowVersion: 1},
		},
		Tasks: []projectControlTask{
			{
				ID:               projectControlTaskIAID,
				ProjectID:        projectControlProjectID,
				WorkstreamID:     projectControlWorkstreamUXID,
				Title:            "Review information architecture proposal",
				Goal:             "Confirm that the Project Panel drill-down matches the docs and is ready for explicit human acceptance.",
				State:            "execution_complete",
				AcceptanceStatus: "under_human_review",
				RiskLevel:        "medium",
				Priority:         "high",
				AgentLabel:       "reviewer",
				RuntimeID:        projectControlRuntimeID,
				RecentSummary:    "IA doc updated; approval still pending because business acceptance is tracked separately from execution completion.",
				NextStep:         "Approve, reject, or reroute the pending final acceptance checkpoint.",
				FilesChanged:     []string{"docs/project-control-discussions/information-architecture.md"},
				DiffSummary:      "Refined the top-level dual-mode navigation and clarified the Project → Workstream → Task → Session drill-down.",
				SessionIDs:       []string{"session-review-ia"},
				Timeline:         []projectControlEvent{{Timestamp: stampA, Actor: "human", Action: "submitted_doc", Detail: "Updated the information architecture proposal."}, {Timestamp: stampB, Actor: "system", Action: "completion_check_passed", Detail: "Execution marked complete; final acceptance kept separate."}},
				Evidence:         []projectControlEvidence{{Label: "Submitted file", Value: "docs/project-control-discussions/information-architecture.md"}, {Label: "Committed change", Value: "f62e6a6 docs: refine information architecture"}, {Label: "Acceptance note", Value: "Execution complete does not imply business acceptance."}},
				Audit:            []projectControlAuditItem{{Timestamp: stampC, Actor: "policy_engine", Action: "raised_checkpoint", Detail: "Generated final_acceptance checkpoint for explicit human sign-off."}},
				RowVersion:       1,
			},
			{
				ID:               projectControlTaskPanelID,
				ProjectID:        projectControlProjectID,
				WorkstreamID:     projectControlWorkstreamUXID,
				Title:            "Build Project Panel shell",
				Goal:             "Add a second top-level mode for project control while preserving Terminal as the default workspace.",
				State:            "planned",
				AcceptanceStatus: "not_ready",
				RiskLevel:        "high",
				Priority:         "high",
				AgentLabel:       "worker",
				RuntimeID:        projectControlRuntimeID,
				RecentSummary:    "Waiting for an active execution session to attach real UI work to this task.",
				NextStep:         "Start or attach a terminal session, then wire the Project Panel into the existing header and workspace shell.",
				FilesChanged:     []string{"web/index.html", "web/css/style.css", "web/js/app.js", "web/js/project-panel.js"},
				DiffSummary:      "Introduce the new Project Panel mode without regressing terminal layouts, files, or editor interactions.",
				SessionIDs:       []string{},
				Timeline:         []projectControlEvent{{Timestamp: stampD, Actor: "orchestrator", Action: "queued", Detail: "Reserved for the live implementation session."}},
				Evidence:         []projectControlEvidence{{Label: "Constraint", Value: "Terminal remains the default landing experience."}, {Label: "Primary bridge", Value: "Session Detail → Attach Terminal (capability gated)"}},
				Audit:            []projectControlAuditItem{},
				RowVersion:       1,
			},
			{
				ID:               projectControlTaskAttachID,
				ProjectID:        projectControlProjectID,
				WorkstreamID:     projectControlWorkstreamRuntimeID,
				Title:            "Capability-gate terminal attach",
				Goal:             "Only show Attach Terminal when the runtime supports interactive re-attachment.",
				State:            "planned",
				AcceptanceStatus: "not_ready",
				RiskLevel:        "medium",
				Priority:         "medium",
				AgentLabel:       "worker",
				RuntimeID:        projectControlRuntimeID,
				RecentSummary:    "Waiting for a runtime-backed session; current terminal infrastructure is the intended execution bridge.",
				NextStep:         "Expose live terminal sessions as Project Panel sessions and switch back into Terminal mode on attach.",
				FilesChanged:     []string{"internal/server/project_control.go", "web/js/app.js", "web/js/project-panel.js"},
				DiffSummary:      "Map live terminal sessions into Project Panel session cards and attach using the existing websocket path.",
				SessionIDs:       []string{},
				Timeline:         []projectControlEvent{{Timestamp: stampE, Actor: "system", Action: "runtime_scanned", Detail: "Interactive attach available via the existing terminal workspace"}},
				Evidence:         []projectControlEvidence{{Label: "Runtime health", Value: "Interactive attach available via the existing terminal workspace"}},
				Audit:            []projectControlAuditItem{},
				RowVersion:       1,
			},
			{
				ID:               projectControlTaskApprovalsID,
				ProjectID:        projectControlProjectID,
				WorkstreamID:     projectControlWorkstreamRuntimeID,
				Title:            "Wire approvals inbox",
				Goal:             "Use pending checkpoints as the single canonical record source for approval decisions.",
				State:            "blocked",
				AcceptanceStatus: "not_ready",
				RiskLevel:        "medium",
				Priority:         "medium",
				AgentLabel:       "policy_engine",
				RuntimeID:        projectControlRuntimeID,
				RecentSummary:    "Waiting for the first checkpoint action flow to be connected end-to-end.",
				NextStep:         "Submit an approve / reject / reroute decision and reflect it in task acceptance state plus audit history.",
				FilesChanged:     []string{"internal/server/project_control.go", "web/js/project-panel.js"},
				DiffSummary:      "Keep approvals as a filtered view over checkpoint records instead of a separate source of truth.",
				SessionIDs:       []string{},
				Timeline:         []projectControlEvent{{Timestamp: stampF, Actor: "system", Action: "blocked", Detail: "Waiting for decision handling API."}},
				Evidence:         []projectControlEvidence{{Label: "Canonical source", Value: "pending checkpoints"}},
				Audit:            []projectControlAuditItem{},
				RowVersion:       1,
			},
		},
		Artifacts: []projectControlArtifact{
			{ID: "artifact-seed-ia-plan", TaskID: projectControlTaskIAID, Kind: "plan", Outcome: "recorded", Label: "Plan", Value: "Information architecture review plan recorded.", CreatedAt: stampA},
			{ID: "artifact-seed-ia-diff", TaskID: projectControlTaskIAID, Kind: "diff_summary", Outcome: "recorded", Label: "Diff summary", Value: "Refined Project Panel drill-down documentation and acceptance split.", CreatedAt: stampB},
			{ID: "artifact-seed-ia-test", TaskID: projectControlTaskIAID, Kind: "test_result", Outcome: "pass", Label: "Test result", Value: "Documentation review completed without blocking validation findings.", CreatedAt: stampB},
			{ID: "artifact-seed-ia-review", TaskID: projectControlTaskIAID, Kind: "review_result", Outcome: "pass", Label: "Review result", Value: "No blocking objections before final human acceptance.", CreatedAt: stampC},
			{ID: "artifact-seed-ia-completion", TaskID: projectControlTaskIAID, Kind: "completion_check", Outcome: "pass", Label: "Completion check", Value: "Seeded IA task has all completion evidence required for final acceptance review.", CreatedAt: stampC},
		},
		Checkpoints: []projectControlCheckpoint{{
			ID:              projectControlCheckpointAcceptanceID,
			TaskID:          projectControlTaskIAID,
			Kind:            "final_acceptance",
			Title:           "Final acceptance required",
			Reason:          "The IA redesign is execution-complete, but business acceptance must remain an explicit human decision.",
			Status:          "pending",
			RequestedAt:     stampC,
			AllowedActions:  []string{"approve", "reject", "reroute"},
			DecisionSummary: "",
			RowVersion:      1,
		}},
		Events: []projectControlRecordedEvent{
			{ID: "event-seed-ia-submitted", Timestamp: stampA, Actor: "human", Action: "submitted_doc", Detail: "Updated the information architecture proposal.", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamUXID, TaskID: projectControlTaskIAID},
			{ID: "event-seed-ia-complete", Timestamp: stampB, Actor: "system", Action: "completion_check_passed", Detail: "Execution marked complete; final acceptance kept separate.", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamUXID, TaskID: projectControlTaskIAID},
			{ID: "event-seed-acceptance-checkpoint", Timestamp: stampC, Actor: "policy_engine", Action: "checkpoint_raised", Detail: "Generated final_acceptance checkpoint for explicit human sign-off.", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamUXID, TaskID: projectControlTaskIAID, CheckpointID: projectControlCheckpointAcceptanceID},
			{ID: "event-seed-panel-queued", Timestamp: stampD, Actor: "orchestrator", Action: "queued", Detail: "Reserved for the live implementation session.", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamUXID, TaskID: projectControlTaskPanelID},
			{ID: "event-seed-runtime-scanned", Timestamp: stampE, Actor: "system", Action: "runtime_scanned", Detail: "Interactive attach available via the existing terminal workspace", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamRuntimeID, TaskID: projectControlTaskAttachID},
			{ID: "event-seed-approvals-blocked", Timestamp: stampF, Actor: "system", Action: "blocked", Detail: "Waiting for decision handling API.", ProjectID: projectControlProjectID, WorkstreamID: projectControlWorkstreamRuntimeID, TaskID: projectControlTaskApprovalsID},
		},
		UpdatedAt: now.Format(time.RFC3339Nano),
	}
	projectControlNormalizeState(&state)
	return state
}

// seedDemoControl writes the demo fixture into the store so tests that drive the
// HTTP API (which loads state via loadOrSeed) see the historical seeded IDs.
func seedDemoControl(t *testing.T, srv *Server) {
	t.Helper()
	if err := srv.projectControl.saveLocked("ian", demoProjectControlState()); err != nil {
		t.Fatalf("seedDemoControl saveLocked error: %v", err)
	}
}
