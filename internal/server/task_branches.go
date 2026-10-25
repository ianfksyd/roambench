package server

import (
	"encoding/json"
	"net/http"
)

// handleTaskBranches derives child tasks from a parent, atomically.
func (s *Server) handleTaskBranches(w http.ResponseWriter, r *http.Request, username, parentID string) {
	var req taskBranchesRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<18)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	snapshot, err := s.projectControl.createTaskBranches(username, parentID, req.Branches, s.terminals)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

// handleTaskCompose creates a synthesis task depending on all sources.
func (s *Server) handleTaskCompose(w http.ResponseWriter, r *http.Request, username, parentID string) {
	var req taskComposeRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<18)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	snapshot, err := s.projectControl.createComposedTask(username, req.SourceTaskIDs, req.Title, req.Goal, s.terminals)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}
