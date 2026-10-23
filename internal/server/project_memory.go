package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Canonical project memory.
//
// Every agent that works on a project reads the same memory instead of
// keeping its own copy, which is where the repeated re-reading used to cost
// tokens. The memory lives in the project control state, so it is shared by
// all agents of the owning user and survives agent restarts.
//
// The per-task memory in projectControlTaskMemory is a different thing: it is
// a derived window over the event stream, not a writable knowledge store.

// projectMemoryMaxBytes caps the stored memory so a runaway agent cannot
// grow it without bound.
const projectMemoryMaxBytes = 64 << 10

// projectControlProjectMemory is the canonical memory of one project.
type projectControlProjectMemory struct {
	ProjectID string `json:"projectId"`
	Content   string `json:"content"`
	Revision  int    `json:"revision"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	UpdatedBy string `json:"updatedBy,omitempty"`
}

// projectControlProjectMemoryRequest is the write payload.
type projectControlProjectMemoryRequest struct {
	ProjectID string `json:"projectId"`
	Content   string `json:"content"`
	// Mode is "append" (default) or "replace".
	Mode string `json:"mode,omitempty"`
	// Actor names who wrote the memory; defaults to the agent label.
	Actor string `json:"actor,omitempty"`
}

// projectMemoryFor returns the canonical memory of a project, if any.
func projectMemoryFor(state projectControlState, projectID string) (projectControlProjectMemory, bool) {
	for _, memory := range state.ProjectMemories {
		if memory.ProjectID == projectID {
			return memory, true
		}
	}
	return projectControlProjectMemory{}, false
}

// projectMemoryPayload renders the canonical memory for an agent payload.
// An empty project still gets the shape, so agents can rely on the keys.
func projectMemoryPayload(state projectControlState, projectID string) map[string]interface{} {
	memory, found := projectMemoryFor(state, projectID)
	if !found {
		return map[string]interface{}{
			"projectId": projectID,
			"content":   "",
			"revision":  0,
			"readWith":  "roambench-agent memory get --project " + projectID,
		}
	}
	return map[string]interface{}{
		"projectId": memory.ProjectID,
		"content":   memory.Content,
		"revision":  memory.Revision,
		"updatedAt": memory.UpdatedAt,
		"readWith":  "roambench-agent memory get --project " + memory.ProjectID,
	}
}

// projectMemoryUpsert writes the canonical memory of a project. Append keeps
// the existing content and adds a dated entry; replace overwrites it.
func (s *projectControlStore) projectMemoryUpsert(username, projectID, content, mode, actor string) (projectControlProjectMemory, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return projectControlProjectMemory{}, errors.New("projectId is required")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return projectControlProjectMemory{}, errors.New("content is required")
	}
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		mode = "append"
	}
	if mode != "append" && mode != "replace" {
		return projectControlProjectMemory{}, fmt.Errorf("mode must be append or replace")
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "agent"
	}

	var memory projectControlProjectMemory
	_, err := s.withStateLocked(username, func(state *projectControlState) error {
		if !projectControlProjectExists(*state, projectID) {
			return errors.New("project not found")
		}
		existing, found := projectMemoryFor(*state, projectID)
		content := content
		if mode == "append" && found && strings.TrimSpace(existing.Content) != "" {
			content = strings.TrimRight(existing.Content, "\n") + "\n\n" + content
		}
		if len(content) > projectMemoryMaxBytes {
			return fmt.Errorf("project memory exceeds %d bytes", projectMemoryMaxBytes)
		}
		memory = projectControlProjectMemory{
			ProjectID: projectID,
			Content:   content,
			Revision:  existing.Revision + 1,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			UpdatedBy: actor,
		}
		replaced := false
		for i := range state.ProjectMemories {
			if state.ProjectMemories[i].ProjectID != projectID {
				continue
			}
			state.ProjectMemories[i] = memory
			replaced = true
			break
		}
		if !replaced {
			state.ProjectMemories = append(state.ProjectMemories, memory)
		}
		projectControlAppendRecordedEvent(state, projectControlRecordedEvent{
			ID:        projectControlID("event", "project-memory-updated"),
			Timestamp: memory.UpdatedAt,
			Actor:     actor,
			Action:    "project_memory_updated",
			Detail:    fmt.Sprintf("Project memory updated (revision %d).", memory.Revision),
			ProjectID: projectID,
		})
		return nil
	})
	if err != nil {
		return projectControlProjectMemory{}, err
	}
	return memory, nil
}

// projectMemoryRead returns the canonical memory of a project.
func (s *projectControlStore) projectMemoryRead(username, projectID string) (projectControlProjectMemory, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return projectControlProjectMemory{}, errors.New("projectId is required")
	}
	state, err := s.loadOrSeed(username)
	if err != nil {
		return projectControlProjectMemory{}, err
	}
	if !projectControlProjectExists(state, projectID) {
		return projectControlProjectMemory{}, errors.New("project not found")
	}
	memory, found := projectMemoryFor(state, projectID)
	if !found {
		return projectControlProjectMemory{ProjectID: projectID}, nil
	}
	return memory, nil
}

// seedProjectMemory initialises the canonical memory from the project brief,
// so the shared memory is useful from the first task instead of empty.
func seedProjectMemory(state *projectControlState, project projectControlProject) {
	if _, found := projectMemoryFor(*state, project.ID); found {
		return
	}
	var lines []string
	if goal := strings.TrimSpace(project.CurrentGoal); goal != "" {
		lines = append(lines, "## 当前目标\n\n"+goal)
	}
	brief := project.Brief
	if understanding := strings.TrimSpace(brief.Understanding); understanding != "" {
		lines = append(lines, "## 项目理解\n\n"+understanding)
	}
	if deliverables := strings.TrimSpace(brief.Deliverables); deliverables != "" {
		lines = append(lines, "## 交付物\n\n"+deliverables)
	}
	if outOfScope := strings.TrimSpace(brief.OutOfScope); outOfScope != "" {
		lines = append(lines, "## 不在范围内\n\n"+outOfScope)
	}
	if assumptions := strings.TrimSpace(brief.Assumptions); assumptions != "" {
		lines = append(lines, "## 假设\n\n"+assumptions)
	}
	if len(lines) == 0 {
		return
	}
	state.ProjectMemories = append(state.ProjectMemories, projectControlProjectMemory{
		ProjectID: project.ID,
		Content:   strings.Join(lines, "\n\n"),
		Revision:  1,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatedBy: "system",
	})
}

// handleAgentProjectMemory serves the canonical project memory to agents.
//
// GET  /api/agent/v1/memory?project=<id>
// POST /api/agent/v1/memory {"projectId":"...","content":"...","mode":"append"}
func (s *Server) handleAgentProjectMemory(w http.ResponseWriter, r *http.Request, username string) {
	switch r.Method {
	case http.MethodGet:
		projectID := strings.TrimSpace(r.URL.Query().Get("project"))
		if projectID == "" {
			projectID = s.projectIDForAgentTask(username, strings.TrimSpace(r.URL.Query().Get("task")))
		}
		memory, err := s.projectControl.projectMemoryRead(username, projectID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, memory)
	case http.MethodPost:
		var req projectControlProjectMemoryRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		memory, err := s.projectControl.projectMemoryUpsert(username, req.ProjectID, req.Content, req.Mode, req.Actor)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, memory)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProjectControlMemory serves the same memory to the web UI.
//
// GET  /api/project-control/memory?project=<id>
// POST /api/project-control/memory {"projectId":"...","content":"...","mode":"append"}
func (s *Server) handleProjectControlMemory(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	switch r.Method {
	case http.MethodGet:
		projectID := strings.TrimSpace(r.URL.Query().Get("project"))
		memory, err := s.projectControl.projectMemoryRead(username, projectID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, memory)
	case http.MethodPost:
		var req projectControlProjectMemoryRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if actor := strings.TrimSpace(req.Actor); actor == "" {
			req.Actor = "human"
		}
		memory, err := s.projectControl.projectMemoryUpsert(username, req.ProjectID, req.Content, req.Mode, req.Actor)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, memory)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// projectIDForAgentTask resolves the project of a task, so an agent that
// only knows its task id can still read the project memory.
func (s *Server) projectIDForAgentTask(username, taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ""
	}
	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		return ""
	}
	for _, task := range state.Tasks {
		if task.ID == taskID {
			return task.ProjectID
		}
	}
	return ""
}
