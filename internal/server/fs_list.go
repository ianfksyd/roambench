package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// resolveHomeDirectory checks both lexical and resolved boundaries. A sibling
// sharing the home's prefix and a symlink pointing outside are both forbidden.
func resolveHomeDirectory(home, path string) (string, bool) {
	if !filepath.IsAbs(path) {
		return "", false
	}
	within := func(root, candidate string) bool {
		rel, err := filepath.Rel(root, candidate)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	if !within(home, filepath.Clean(path)) {
		return "", false
	}
	root, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !within(root, resolved) {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return resolved, true
}

// GET /api/fs/list?path=/absolute/home/path returns sorted child directory names.
// An empty path opens home. Files and links escaping home are omitted.
func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "home unavailable"})
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = home
	}
	resolved, ok := resolveHomeDirectory(home, path)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "directory must exist within home"})
		return
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "directory is not readable"})
		return
	}
	// Map the resolved location back under the lexical home (which may itself
	// be a symlink), so subsequent navigation and child checks use this directory.
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "home unavailable"})
		return
	}
	rel, err := filepath.Rel(realHome, resolved)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "directory unavailable"})
		return
	}
	path = filepath.Join(home, rel)
	directories := make([]string, 0)
	for _, entry := range entries {
		if _, ok := resolveHomeDirectory(home, filepath.Join(path, entry.Name())); ok {
			directories = append(directories, entry.Name())
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"path": filepath.Clean(path), "directories": directories})
}
