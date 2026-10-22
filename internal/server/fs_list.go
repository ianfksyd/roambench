package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ianf339/roambench/internal/roots"
)

// GET /api/fs/list?path=/absolute/path returns the child directory names of
// that directory plus the browse roots the user can jump to.
//
// Browsing is not confined to the home directory: the roots are quick-access
// entries, so any absolute path on the machine may be listed. An empty path
// opens the home directory.
func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := GetUsername(r)
	home := roots.HomeDir(username)
	if home == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "home unavailable"})
		return
	}
	expanded, err := roots.ExpandRaw(strings.TrimSpace(r.URL.Query().Get("path")), home)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// Resolve symlinks before collapsing "..", so "link/.." keeps its POSIX
	// meaning.
	resolved, err := filepath.EvalSymlinks(expanded)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "directory does not exist"})
		return
	}
	info, err := os.Stat(resolved)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "directory does not exist"})
		return
	}
	if !info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not a directory"})
		return
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "directory is not readable"})
		return
	}
	directories := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			directories = append(directories, entry.Name())
			continue
		}
		// Follow symlinks so linked directories stay browsable.
		if target, statErr := os.Stat(filepath.Join(resolved, entry.Name())); statErr == nil && target.IsDir() {
			directories = append(directories, entry.Name())
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":        resolved,
		"directories": directories,
		"roots":       s.roots.List(username),
	})
}
