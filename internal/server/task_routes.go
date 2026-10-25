package server

import (
	"net/http"
	"strconv"
	"strings"
)

// handleProjectControlTaskRoutes dispatches the sub-resource routes under
// /api/project-control/tasks/{id}/: replay (existing) and progress (P1).
func (s *Server) handleProjectControlTaskRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/project-control/tasks/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(id) > 128 || !isValidProjectControlID(id) {
		http.NotFound(w, r)
		return
	}
	// No sub-path (e.g. PATCH /tasks/{id}): handled by the legacy handler.
	if len(parts) == 1 {
		s.handleProjectControlTaskReplay(w, r)
		return
	}
	if parts[1] == "progress" && r.Method == http.MethodGet {
		username := GetUsername(r)
		s.handleTaskProgressGET(w, r, username, id)
		return
	}
	if parts[1] == "source-links" && r.Method == http.MethodGet {
		username := GetUsername(r)
		refs, err := s.controlPlane.ListSourceRefs(r.Context(), username, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"refs": refs})
		return
	}
	if parts[1] == "branches" && r.Method == http.MethodPost {
		username := GetUsername(r)
		s.handleTaskBranches(w, r, username, id)
		return
	}
	if parts[1] == "compose" && r.Method == http.MethodPost {
		username := GetUsername(r)
		s.handleTaskCompose(w, r, username, id)
		return
	}
	// replay / PATCH / anything else: preserve the legacy handler's exact
	// behaviour (including its 405 for unmatched methods).
	s.handleProjectControlTaskReplay(w, r)
}

// queryInt parses an integer query parameter; errors yield 0.
func queryInt(r *http.Request, key string) int {
	v, _ := strconv.Atoi(r.URL.Query().Get(key))
	return v
}
