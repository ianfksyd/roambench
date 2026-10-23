package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ianf339/roambench/internal/agentmem"
)

// Memory pointers.
//
// The pointer tells an agent where the canonical project memory lives. It is
// written into the memory files the agents actually read: the workspace of
// the project and, when enabled, the global agent memory files. Without it
// the agents keep their own copies and the token cost comes back.
//
// The write is idempotent, backed up, and reversible.

// pointerReport is one touched file.
type pointerReport struct {
	Path    string `json:"path"`
	Action  string `json:"action"` // created | updated | unchanged
	Backup  string `json:"backupPath,omitempty"`
	Managed bool   `json:"managed"` // false = the file was left alone
}

// pointerRequest asks for the pointers of a project.
type pointerRequest struct {
	ProjectID string `json:"projectId"`
	// Remove drops the pointers instead of writing them.
	Remove bool `json:"remove,omitempty"`
}

// projectPointerFiles returns the files that should carry the pointer for a
// project: the memory files that already exist in the workspace, and — only
// when explicitly enabled for a registered project — the files of the agents
// installed on this machine.
func projectPointerFiles(workspace string, createMissing bool) []string {
	var files []string
	seen := map[string]bool{}
	for _, path := range agentmem.Candidates(workspace) {
		if seen[path] {
			continue
		}
		seen[path] = true
		files = append(files, path)
	}
	if !createMissing {
		return files
	}
	// An agent that is installed on the machine reads its instruction file
	// even when the project has none yet, so create it.
	installed := map[string]bool{}
	for _, agent := range agentProbeDefault.Probe(false).Agents {
		installed[agent.AgentLabel] = agent.Installed
	}
	if installed["claude"] {
		path := filepath.Join(workspace, "CLAUDE.md")
		if !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	if installed["codex"] || installed["opencode"] {
		path := filepath.Join(workspace, "AGENTS.md")
		if !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	return files
}

// globalPointerFiles returns the global agent memory files of the user. Only
// files that already exist are touched: a global file is the user's own
// instruction sheet.
func globalPointerFiles(username string) []string {
	home := agentConfigHome(username)
	if home == "" {
		return nil
	}
	var files []string
	for _, candidate := range []string{
		filepath.Join(home, ".claude", "CLAUDE.md"),
		filepath.Join(home, ".codex", "AGENTS.md"),
		filepath.Join(home, ".config", "opencode", "AGENTS.md"),
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			files = append(files, candidate)
		}
	}
	return files
}

// applyProjectPointers writes the memory pointers of a project and reports
// what changed.
func (s *Server) applyProjectPointers(username, projectID string, remove bool) ([]pointerReport, error) {
	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		return nil, err
	}
	project, ok := controlProject(state, projectID)
	if !ok {
		return nil, errors.New("project not found")
	}
	// Creating memory files is only ever done in a project the user
	// registered with its own path; the runtime fallback is detection-only.
	registered := strings.TrimSpace(project.RepoPath) != ""
	workspace := strings.TrimSpace(project.RepoPath)
	if workspace == "" {
		workspace = s.projectControl.runtimeRootDir
	}
	if workspace == "" {
		return nil, errors.New("project workspace is unavailable")
	}
	if info, err := os.Stat(workspace); err != nil || !info.IsDir() {
		return nil, errors.New("project workspace is not a directory")
	}

	reports := make([]pointerReport, 0, 4)
	for _, path := range projectPointerFiles(workspace, s.cfg.Agent.CreateMissingMemoryFiles && registered) {
		report := pointerReport{Path: path, Action: "unchanged"}
		if remove {
			removed, err := agentmem.RemovePointer(path, "project="+projectID)
			if err != nil {
				return nil, err
			}
			if removed {
				report.Action = "updated"
			}
			reports = append(reports, report)
			continue
		}
		result, err := agentmem.ApplyPointer(path, projectID)
		if err != nil {
			return nil, err
		}
		switch {
		case result.Created:
			report.Action = "created"
		case result.Changed:
			report.Action = "updated"
		}
		report.Backup = result.BackupPath
		reports = append(reports, report)
	}

	if s.cfg.Agent.GlobalMemoryPointer {
		for _, path := range globalPointerFiles(username) {
			report := pointerReport{Path: path, Action: "unchanged"}
			if remove {
				removed, err := agentmem.RemovePointer(path, agentmem.GenericScope)
				if err != nil {
					return nil, err
				}
				if removed {
					report.Action = "updated"
				}
				reports = append(reports, report)
				continue
			}
			result, err := agentmem.ApplyGenericPointer(path)
			if err != nil {
				return nil, err
			}
			switch {
			case result.Created:
				report.Action = "created"
			case result.Changed:
				report.Action = "updated"
			}
			report.Backup = result.BackupPath
			reports = append(reports, report)
		}
	}
	return reports, nil
}

// handleProjectPointers writes or removes the memory pointers of a project.
//
// POST /api/project-control/memory/pointers {"projectId":"...","remove":false}
func (s *Server) handleProjectPointers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := GetUsername(r)
	var req pointerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.ProjectID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "projectId is required"})
		return
	}
	reports, err := s.applyProjectPointers(username, req.ProjectID, req.Remove)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"pointers": reports})
}
