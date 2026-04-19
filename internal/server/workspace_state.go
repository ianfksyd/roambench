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
	walPath := s.walPathFor(username)
	state, exists, err := workspaceStateReadFile(path)
	if err != nil {
		return workspaceStatePayload{}, false, err
	}
	walState, walExists, err := workspaceStateReadFile(walPath)
	if err != nil {
		return workspaceStatePayload{}, false, err
	}
	if !exists && !walExists {
		return workspaceStatePayload{Workspaces: []workspaceStateRecord{}}, false, nil
	}

	stateSourceIsWAL := false
	clearWAL := false
	switch {
	case walExists && (!exists || workspaceStatePreferState(walState, state)):
		state = walState
		exists = true
		stateSourceIsWAL = true
	case walExists:
		clearWAL = true
	}
	normalizeWorkspaceStatePayload(&state)
	if stateSourceIsWAL {
		if err := s.saveLocked(username, state); err != nil {
			return workspaceStatePayload{}, false, err
		}
	} else if clearWAL {
		_ = os.Remove(walPath)
	}
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

	return s.saveLocked(username, state)
}

func (s *workspaceStateStore) saveLocked(username string, state workspaceStatePayload) error {
	if strings.TrimSpace(username) == "" {
		return errors.New("missing username")
	}
	normalizeWorkspaceStatePayload(&state)
	if state.UpdatedAt == "" {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	if err := os.MkdirAll(s.rootDir, 0700); err != nil {
		return err
	}

	path := s.pathFor(username)
	walPath := s.walPathFor(username)
	backupPath := path + ".bak"
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := workspaceStateWriteFileAtomically(walPath, payload, 0600); err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if err := workspaceStateWriteFileAtomically(backupPath, existing, 0600); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := workspaceStateWriteFileAtomically(path, payload, 0600); err != nil {
		return err
	}
	_ = os.Remove(walPath)
	return nil
}

func (s *workspaceStateStore) pathFor(username string) string {
	return filepath.Join(s.rootDir, storagePathSegment(username)+".json")
}

func (s *workspaceStateStore) walPathFor(username string) string {
	return filepath.Join(s.rootDir, storagePathSegment(username)+".wal.json")
}

func workspaceStateReadFile(path string) (workspaceStatePayload, bool, error) {
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return workspaceStatePayload{}, false, nil
	}
	if err != nil {
		return workspaceStatePayload{}, false, err
	}
	var state workspaceStatePayload
	if err := json.Unmarshal(payload, &state); err != nil {
		return workspaceStatePayload{}, false, err
	}
	return state, true, nil
}

func workspaceStateParseUpdatedAt(state workspaceStatePayload) (time.Time, bool) {
	value := strings.TrimSpace(state.UpdatedAt)
	if value == "" {
		return time.Time{}, false
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts, true
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts, true
	}
	return time.Time{}, false
}

func workspaceStatePreferState(candidate, current workspaceStatePayload) bool {
	candidateTime, candidateOK := workspaceStateParseUpdatedAt(candidate)
	currentTime, currentOK := workspaceStateParseUpdatedAt(current)
	switch {
	case candidateOK && currentOK:
		if candidateTime.Equal(currentTime) {
			return true
		}
		return candidateTime.After(currentTime)
	case candidateOK && !currentOK:
		return true
	case !candidateOK && currentOK:
		return false
	default:
		return true
	}
}

func workspaceStateWriteFileAtomically(path string, payload []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
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
	value = strings.ReplaceAll(value, string(os.PathSeparator), "_")
	value = strings.ReplaceAll(value, "..", "_")
	return value
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
