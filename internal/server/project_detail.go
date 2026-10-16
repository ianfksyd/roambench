package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/ianf339/roambench/internal/controlplane"
	"github.com/ianf339/roambench/internal/gitstatus"
)

type projectActivityEvent struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Kind      string `json:"kind"`
}

type projectIssue struct {
	Text     string `json:"text"`
	Severity string `json:"severity"`
	Session  string `json:"session,omitempty"`
	Task     string `json:"task,omitempty"`
}

type projectDetail struct {
	Lifecycle  projectLifecycle   `json:"lifecycle"`
	Milestones []projectMilestone `json:"milestones"`
	NextAction projectNextAction  `json:"nextAction"`
	Brief      projectBrief       `json:"brief"`
	Stats      projectDetailStats `json:"stats"`
	Issues     []projectIssue     `json:"issues"`
}

type projectDetailStats struct {
	ArtifactsByKind   map[string]int        `json:"artifactsByKind"`
	CheckpointsByKind map[string]int        `json:"checkpointsByKind"`
	CodeChanges       gitstatus.CodeChanges `json:"codeChanges"`
}

// Activity is a compact, persistent ledger independent of retained audit logs.
// Only canonical decision/artifact events enter it, keyed by source object ID.
const projectActivityLimit = 4096

func trimProjectActivity(state *projectControlState) {
	if len(state.Activity) > projectActivityLimit {
		state.Activity = append([]projectActivityEvent(nil), state.Activity[len(state.Activity)-projectActivityLimit:]...)
	}
}

func recordProjectActivity(state *projectControlState, event projectControlRecordedEvent) {
	trimProjectActivity(state)
	if event.Action != "checkpoint_resolved" && event.Action != "artifact_recorded" {
		return
	}
	if event.SourceID == "" {
		event.SourceID = event.CheckpointID
	}
	if event.SourceID == "" {
		event.SourceID = event.ID
	}
	id := event.Action + ":" + event.SourceID
	for _, e := range state.Activity {
		if e.ID == id {
			return
		}
	}
	projectID := event.ProjectID
	if projectID == "" {
		projectID = checkpointProjectID(state, event.TaskID)
	}
	kind := event.Kind
	if kind == "" {
		kind = "uncategorized"
	}
	state.Activity = append(state.Activity, projectActivityEvent{ID: id, ProjectID: projectID, Kind: kind})
	trimProjectActivity(state)
}

func aggregateProjectDetail(state projectControlState, p projectControlProject, claims []controlplane.TaskClaim) projectDetail {
	result := projectDetail{Lifecycle: p.Lifecycle, Milestones: append([]projectMilestone{}, p.Milestones...), Brief: p.Brief, NextAction: deriveNextAction(p, state.Tasks, claims), Issues: []projectIssue{}}
	result.Stats.CheckpointsByKind = map[string]int{"design": 0, "test": 0, "coding": 0, "unclassified": 0}
	result.Stats.ArtifactsByKind = map[string]int{"design": 0, "test": 0, "coding": 0, "unclassified": 0}
	seen := map[string]bool{}
	for _, e := range state.Activity {
		if e.ProjectID != p.ID || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		kind := e.Kind
		if kind != "design" && kind != "test" && kind != "coding" {
			kind = "unclassified"
		}
		switch {
		case strings.HasPrefix(e.ID, "checkpoint_resolved:"):
			result.Stats.CheckpointsByKind[kind]++
		case strings.HasPrefix(e.ID, "artifact_recorded:"):
			result.Stats.ArtifactsByKind[kind]++
		}
	}
	if p.Lifecycle.State == "blocked" {
		result.Issues = append(result.Issues, projectIssue{Text: p.Lifecycle.Reason, Severity: "error"})
	}
	for _, t := range state.Tasks {
		if t.ProjectID != p.ID {
			continue
		}
		if t.PauseReason != "" {
			result.Issues = append(result.Issues, projectIssue{Text: acceptanceIssueText(t.PauseReason), Severity: "error", Task: t.ID})
		} else if t.DispatchState == "ready" && len(t.AcceptanceCommand) == 0 {
			result.Issues = append(result.Issues, projectIssue{Text: "已批准但缺验收命令，无法领取", Severity: "warning", Task: t.ID})
		}
	}
	for _, a := range state.PhaseAttempts {
		t, ok := controlTask(state, a.TaskID)
		if ok && t.ProjectID == p.ID && a.Status == "failed" && !a.Released {
			result.Issues = append(result.Issues, projectIssue{Text: a.FailureReason, Severity: "warning", Session: projectControlPhaseAttemptSessionID(a)})
		}
	}
	return result
}

func (s *Server) serveProjectDetail(w http.ResponseWriter, r *http.Request, id string) {
	username := GetUsername(r)
	if _, err := s.projectControl.expireStaleClaimsForUser(username); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	p, ok := controlProject(state, id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	claims := []controlplane.TaskClaim{}
	if store := s.projectControl.claimStoreFor(); store != nil {
		for _, t := range state.Tasks {
			if t.ProjectID == id {
				if c, err := store.ActiveClaimForTask(context.Background(), username, t.ID); err == nil {
					claims = append(claims, c)
				}
			}
		}
	}
	detail := aggregateProjectDetail(state, p, claims)
	dir := p.RepoPath
	if dir == "" {
		dir = s.projectControl.runtimeRootDir
	}
	real, err := validateProjectControlRepoPath(dir)
	if err != nil {
		detail.Stats.CodeChanges.Error = err.Error()
	} else {
		detail.Stats.CodeChanges = gitstatus.FetchCodeChanges(real)
	}
	writeJSON(w, 200, detail)
}

// Keep machine-readable pause reasons in task state and actionable guidance in detail.
func acceptanceIssueText(reason string) string {
	if reason == "acceptance_inputs_changed" || reason == "acceptance_inputs_unavailable" {
		return "验收相关文件已变更或无法核验，请人工核对 diff 后显式重启。"
	}
	return reason
}
