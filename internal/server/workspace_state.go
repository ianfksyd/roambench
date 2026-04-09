package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type workspaceStatePayload struct {
	ActiveWorkspaceID string                 `json:"activeWorkspaceId"`
	Workspaces        []workspaceStateRecord `json:"workspaces"`
	UpdatedAt         string                 `json:"updatedAt,omitempty"`
}

type workspaceStateRecord struct {
	ID          string   `json:"id"`
	Layout      string   `json:"layout"`
	TerminalIDs []string `json:"terminalIds"`
	Name        string   `json:"name"`
	LabelNumber int      `json:"labelNumber"`
}

type workspaceStateStore struct {
	rootDir string
	mu      sync.Mutex
}

func newWorkspaceStateStore(basePersistDir string) *workspaceStateStore {
	root := filepath.Join(basePersistDir, ".workspaces")
	_ = os.MkdirAll(root, 0700)
	return &workspaceStateStore{rootDir: root}
}

func (s *workspaceStateStore) Load(username string) (workspaceStatePayload, bool, error) {
	if strings.TrimSpace(username) == "" {
		return workspaceStatePayload{}, false, errors.New("missing username")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.pathFor(username)
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return workspaceStatePayload{Workspaces: []workspaceStateRecord{}}, false, nil
	}
	if err != nil {
		return workspaceStatePayload{}, false, err
	}

	var state workspaceStatePayload
	if err := json.Unmarshal(payload, &state); err != nil {
		return workspaceStatePayload{}, false, err
	}

	normalizeWorkspaceStatePayload(&state)
	return state, true, nil
}

func (s *workspaceStateStore) Save(username string, state workspaceStatePayload) error {
	if strings.TrimSpace(username) == "" {
		return errors.New("missing username")
	}

	normalizeWorkspaceStatePayload(&state)
	if state.UpdatedAt == "" {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.rootDir, 0700); err != nil {
		return err
	}

	path := s.pathFor(username)
	tmpPath := path + ".tmp"
	backupPath := path + ".bak"
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(backupPath, existing, 0600); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(tmpPath, payload, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *workspaceStateStore) pathFor(username string) string {
	return filepath.Join(s.rootDir, storagePathSegment(username)+".json")
}

func normalizeWorkspaceStatePayload(state *workspaceStatePayload) {
	if state == nil {
		return
	}

	if state.Workspaces == nil {
		state.Workspaces = []workspaceStateRecord{}
	}

	for index := range state.Workspaces {
		record := &state.Workspaces[index]
		record.ID = strings.TrimSpace(record.ID)
		record.Name = strings.TrimSpace(record.Name)
		record.Layout = normalizeWorkspaceLayout(record.Layout)
		record.TerminalIDs = normalizeWorkspaceTerminalIDs(record.TerminalIDs)
		if record.LabelNumber < 1 {
			record.LabelNumber = index + 1
		}
	}
}

func normalizeWorkspaceLayout(layout string) string {
	switch strings.TrimSpace(layout) {
	case "1", "2", "4", "4w":
		return strings.TrimSpace(layout)
	default:
		return "1"
	}
}

func normalizeWorkspaceTerminalIDs(ids []string) []string {
	normalized := make([]string, 4)

	for index := 0; index < len(normalized) && index < len(ids); index += 1 {
		normalized[index] = strings.TrimSpace(ids[index])
	}

	return normalized
}

func storagePathSegment(value string) string {
	return strings.ReplaceAll(value, string(os.PathSeparator), "_")
}

func (s *Server) handleWorkspaceState(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)

	switch r.Method {
	case http.MethodGet:
		state, _, err := s.workspaceState.Load(username)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load workspace state"})
			return
		}
		writeJSON(w, http.StatusOK, state)
	case http.MethodPut:
		var req workspaceStatePayload

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		req.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.workspaceState.Save(username, req); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save workspace state"})
			return
		}
		writeJSON(w, http.StatusOK, req)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
