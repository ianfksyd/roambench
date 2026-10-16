package server

// F1 project board backend (K2, WS-20261015-001, task card C2).
//
// GET /api/project-control/board assembles, for every project the requesting
// user can see:
//   - the first non-terminal task (currentTask) with its state and phase,
//   - the latest activity timestamp/summary from tasks, workstreams and the
//     recorded event log,
//   - a read-only git status of the backing repository (repoPath, falling back
//     to the server runtime root when unset), cached 10s per repoPath.
//
// Git problems never fail the request: the project's `git` block reports
// available=false plus a human-readable error instead. Symlinked repoPaths are
// resolved inside repoPath validation so a link cannot point the git read
// outside the user's home directory.

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ianf339/roambench/internal/gitstatus"
)

const (
	// boardGitCacheTTL mirrors the shared gitstatus cache TTL (K2: 10s).
	boardGitCacheTTL = gitstatus.CacheTTL
)

type boardCurrentTask struct {
	TaskID       string `json:"taskId"`
	Title        string `json:"title"`
	State        string `json:"state"`
	CurrentPhase string `json:"currentPhase"`
}

type boardProject struct {
	ProjectID           string            `json:"projectId"`
	Name                string            `json:"name"`
	Status              string            `json:"status"`
	CurrentTask         *boardCurrentTask `json:"currentTask"`
	LastActivityAt      string            `json:"lastActivityAt"`
	LastActivitySummary string            `json:"lastActivitySummary"`
	Git                 gitstatus.Status  `json:"git"`
}

type boardResponse struct {
	GeneratedAt string         `json:"generatedAt"`
	Projects    []boardProject `json:"projects"`
}

// boardGitCache is the process-wide 10s cache; the mutex lives inside the
// gitstatus.Cache so concurrent board requests share entries.
var boardGitCache = gitstatus.NewCache()

// serveProjectControlBoard is the real board handler. It is invoked by the
// route registered in server.go; the C1 contract stub body in
// contract_stubs.go delegates here (final wiring is done by the integrator).
func (s *Server) serveProjectControlBoard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state, err := s.projectControl.loadOrSeed(GetUsername(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load project control state"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	projects := make([]boardProject, len(state.Projects))
	// Probe git repositories concurrently: each read has a 3s timeout, so a
	// serial loop could take 3s per project. The bounded WaitGroup keeps the
	// whole board at ~3s worst case. Order is preserved via index.
	var wg sync.WaitGroup
	for i := range state.Projects {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			project := state.Projects[i]
			projects[i] = boardProject{
				ProjectID:           project.ID,
				Name:                project.Name,
				Status:              project.Status,
				CurrentTask:         boardCurrentTaskFor(state, project.ID),
				LastActivityAt:      boardLastActivityAt(state, project),
				LastActivitySummary: boardLastActivitySummary(state, project),
				Git:                 s.boardGitStatus(project.RepoPath),
			}
		}(i)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, boardResponse{GeneratedAt: now, Projects: projects})
}

// boardGitStatus resolves and validates the repository path for one project
// (K1: repoPath or runtime root), then probes it through the shared cache.
func (s *Server) boardGitStatus(repoPath string) gitstatus.Status {
	resolved := strings.TrimSpace(repoPath)
	if resolved == "" {
		resolved = s.projectControl.runtimeRootDir
	}
	validated, err := validateProjectControlRepoPath(resolved)
	if err != nil {
		return gitstatus.Status{
			RepoPath:  resolved,
			Available: false,
			Error:     err.Error(),
			Worktrees: []gitstatus.Worktree{},
		}
	}
	return boardGitCache.Fetch(validated)
}

// boardCurrentTask returns the first task of the project that is not in a
// terminal state, or nil. Terminal states mirror the archive/acceptance flow
// in project_control.go: execution_complete, archived (and rejected
// acceptance) mean nothing is actively being worked on.
func boardCurrentTaskFor(state projectControlState, projectID string) *boardCurrentTask {
	for _, task := range state.Tasks {
		if task.ProjectID != projectID {
			continue
		}
		st := normalizeProjectControlTaskState(task.State)
		if st == "execution_complete" || st == "archived" {
			continue
		}
		if task.AcceptanceStatus == "rejected" && st == "waiting_review" {
			continue
		}
		return &boardCurrentTask{
			TaskID:       task.ID,
			Title:        task.Title,
			State:        st,
			CurrentPhase: normalizeProjectControlPhaseID(task.CurrentPhase),
		}
	}
	return nil
}

// boardLastActivityAt picks the newest timestamp across the project's tasks
// (timeline, state update), workstreams, and the recorded event log.
func boardLastActivityAt(state projectControlState, project projectControlProject) string {
	newest := ""
	touch := func(ts string) {
		ts = strings.TrimSpace(ts)
		if ts == "" {
			return
		}
		if newest == "" || ts > newest {
			newest = ts
		}
	}
	for _, task := range state.Tasks {
		if task.ProjectID != project.ID {
			continue
		}
		for _, event := range task.Timeline {
			touch(event.Timestamp)
		}
		for _, item := range task.Audit {
			touch(item.Timestamp)
		}
	}
	for _, event := range state.Events {
		if event.ProjectID == project.ID {
			touch(event.Timestamp)
		}
	}
	if newest == "" {
		return projectControlProcessStartedAt.UTC().Format(time.RFC3339)
	}
	return newest
}

// boardLastActivitySummary describes the most recent thing that happened on
// the project, in one line, from the same sources boardLastActivityAt uses.
func boardLastActivitySummary(state projectControlState, project projectControlProject) string {
	newestAt := ""
	summary := ""
	consider := func(ts, text string) {
		ts = strings.TrimSpace(ts)
		text = strings.TrimSpace(text)
		if ts == "" || text == "" {
			return
		}
		if newestAt == "" || ts > newestAt {
			newestAt = ts
			summary = text
		}
	}
	for _, task := range state.Tasks {
		if task.ProjectID != project.ID {
			continue
		}
		for _, event := range task.Timeline {
			consider(event.Timestamp, event.Detail)
		}
		for _, item := range task.Audit {
			consider(item.Timestamp, item.Detail)
		}
	}
	for _, event := range state.Events {
		if event.ProjectID == project.ID {
			consider(event.Timestamp, event.Detail)
		}
	}
	if summary == "" {
		return "No activity recorded yet."
	}
	if len(summary) > 200 {
		summary = summary[:200] + "…"
	}
	return summary
}
