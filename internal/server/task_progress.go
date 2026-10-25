package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"strings"

	"github.com/ianf339/roambench/internal/controlplane"
)

// Task progress feed.
//
// The local worker reports structured executor events while a task runs;
// the panel reads them as the "latest progress" card — no terminal parsing
// and no model calls. When a task has no reported events (legacy runs), the
// read path falls back to a tail of the task's live terminal output, clearly
// labelled as a terminal fragment.

// agentProgressRequest is the write payload from the worker.
type agentProgressRequest struct {
	TaskID  string `json:"taskId"`
	LeaseID string `json:"leaseId,omitempty"`
	Seq     int    `json:"seq"`
	Kind    string `json:"kind"`
	Text    string `json:"text"`
}

// handleAgentTaskProgress receives worker progress events.
//
// POST /api/agent/v1/progress {"taskId":"...","seq":1,"kind":"assistant","text":"..."}
func (s *Server) handleAgentTaskProgress(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req agentProgressRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if s.controlPlane == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "progress store unavailable"})
		return
	}
	// The task must exist: the worker only reports for tasks it claimed, so
	// an unknown taskId is a client bug, not telemetry to record anyway.
	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if _, ok := controlTask(state, req.TaskID); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	if err := s.controlPlane.AddTaskProgress(r.Context(), username, req.TaskID, req.Seq, req.Kind, req.Text); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, controlplane.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// taskProgressResponse is the read payload for the panel.
type taskProgressResponse struct {
	Events    []taskProgressEntry `json:"events"`
	Source    string              `json:"source"` // "reported" | "terminal_fragment" | "none"
	Truncated bool                `json:"truncated,omitempty"`
	Terminal  string              `json:"terminal,omitempty"`
}

type taskProgressEntry struct {
	Seq       int    `json:"seq"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// progressLineLimit caps the rendered card content.
const progressLineLimit = 8

// handleTaskProgressGET serves the panel's progress card.
func (s *Server) handleTaskProgressGET(w http.ResponseWriter, r *http.Request, username, taskID string) {
	after := 0
	if raw := queryInt(r, "after"); raw > 0 {
		after = raw
	}
	var events []taskProgressEntry
	source := "none"
	if s.controlPlane != nil {
		stored, err := s.controlPlane.ListTaskProgress(r.Context(), username, taskID, after)
		if err == nil {
			for _, p := range stored {
				events = append(events, taskProgressEntry{Seq: p.Seq, Kind: p.Kind, Text: p.Text, CreatedAt: p.CreatedAt})
			}
			if len(events) > 0 {
				source = "reported"
			}
		}
	}
	// Compatibility: no reported events (or the caller wants the live tail)
	// → show a fragment of the task's live terminal output.
	if source == "none" {
		if fragment, terminalID, ok := s.taskTerminalFragment(username, taskID, progressLineLimit); ok {
			source = "terminal_fragment"
			events = []taskProgressEntry{{Seq: 0, Kind: "terminal_fragment", Text: fragment}}
			if terminalID != "" {
				events[0].CreatedAt = terminalID
			}
		}
	}
	if len(events) > progressLineLimit {
		events = events[:progressLineLimit]
		source = "reported"
	}
	writeJSON(w, http.StatusOK, taskProgressResponse{Events: events, Source: source})
}

// taskTerminalFragment tails the newest live terminal of a task.
func (s *Server) taskTerminalFragment(username, taskID string, lines int) (string, string, bool) {
	if s.terminals == nil {
		return "", "", false
	}
	state, err := s.projectControl.loadOrSeed(username)
	if err != nil {
		return "", "", false
	}
	task, ok := controlTask(state, taskID)
	if !ok {
		return "", "", false
	}
	for i := len(task.SessionIDs) - 1; i >= 0; i-- {
		terminalID := strings.TrimPrefix(task.SessionIDs[i], "session-live-")
		if terminalID == task.SessionIDs[i] {
			continue // not a live session reference
		}
		out, err := exec.Command("tmux", "capture-pane", "-t", terminalID, "-p",
			"-S", "-"+itoa(lines)).Output()
		if err != nil {
			continue
		}
		text := strings.TrimRight(string(out), "\n")
		if strings.TrimSpace(text) == "" {
			continue
		}
		return text, terminalID, true
	}
	return "", "", false
}

// itoa is a tiny helper for the capture-pane argument.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
