package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

// GET    /api/fs/roots  lists the browse roots of the signed-in user.
// POST   /api/fs/roots  adds a personal root ({"path": "/data"}).
// DELETE /api/fs/roots  removes a personal root ({"path": "/data"}).
//
// Instance roots come from the configuration and are read-only here; the
// personal ones live in ~/.config/roambench/roots.toml.
func (s *Server) handleFSRoots(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	switch r.Method {
	case http.MethodGet:
		s.writeFSRoots(w, username)
	case http.MethodPost:
		if !s.cfg.Filesystem.AllowUserRoots {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "personal roots are disabled"})
			return
		}
		path, ok := s.readFSRootsPath(w, r)
		if !ok {
			return
		}
		if err := s.roots.Add(username, path); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.writeFSRoots(w, username)
	case http.MethodDelete:
		if !s.cfg.Filesystem.AllowUserRoots {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "personal roots are disabled"})
			return
		}
		path, ok := s.readFSRootsPath(w, r)
		if !ok {
			return
		}
		if err := s.roots.Remove(username, path); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.writeFSRoots(w, username)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// readFSRootsPath takes the root path from the JSON body or the query string.
func (s *Server) readFSRootsPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" && r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			path = strings.TrimSpace(req.Path)
		}
	}
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return "", false
	}
	return path, true
}

func (s *Server) writeFSRoots(w http.ResponseWriter, username string) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"roots":            s.roots.List(username),
		"allow_user_roots": s.cfg.Filesystem.AllowUserRoots,
	})
}
