package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ianf339/roambench/internal/config"
)

const (
	StatusActive   = "active"
	StatusIdle     = "idle"
	StatusExited   = "exited"
	StatusDetached = "detached"
)

type tmuxObservation struct {
	created  time.Time
	activity time.Time
	detached *time.Time
	alive    bool
}

func classifySession(info *SessionInfo, now time.Time, alive bool) {
	if info.LastActivity.IsZero() {
		info.LastActivity = info.CreatedAt
	}
	info.IdleSeconds = int64(now.Sub(info.LastActivity) / time.Second)
	if info.IdleSeconds < 0 {
		info.IdleSeconds = 0
	}
	switch {
	case !alive:
		info.Status = StatusExited
	case info.DetachedAt != nil && now.Sub(*info.DetachedAt) > 24*time.Hour:
		info.Status = StatusDetached
	case now.Sub(info.LastActivity) > 2*time.Hour:
		info.Status = StatusIdle
	default:
		info.Status = StatusActive
	}
}

func applyTmuxObservation(info *SessionInfo, observed tmuxObservation, now time.Time) {
	info.Backend = "tmux"
	if info.CreatedAt.IsZero() {
		info.CreatedAt = observed.created
	}
	if observed.activity.After(info.LastActivity) {
		info.LastActivity = observed.activity
	}
	info.DetachedAt = observed.detached
	classifySession(info, now, observed.alive)
}

// Track the actual last-client detach, including clients outside the web bridge.
// Use a private hook index so existing user hooks are preserved. Older sessions
// with no timestamp start tracking now; last-attached is not a detach timestamp.
func trackTmuxDetachment(id string) {
	exec.Command("tmux", "set-option", "-t", id, "@roambench_epoch_format", "%s").Run()
	exec.Command("tmux", "set-hook", "-t", id, "client-detached[900]", `if-shell -F '#{==:#{session_attached},0}' 'set-option -F @roambench_detached_at "#{T:@roambench_epoch_format}"'`).Run()
	exec.Command("tmux", "set-hook", "-t", id, "client-attached[900]", `set-option -u @roambench_detached_at`).Run()
	exec.Command("tmux", "if-shell", "-t", id, "-F", "#{&&:#{==:#{session_attached},0},#{==:#{@roambench_detached_at},}}", `set-option -F @roambench_detached_at '#{T:@roambench_epoch_format}'`).Run()
}

// Observe all panes so a dead pane does not hide a live sibling and background
// window output counts as activity. No tmux options or metadata are mutated.
func observeTmuxSessions() (map[string]tmuxObservation, error) {
	result := make(map[string]tmuxObservation)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	format := "#{session_name}\t#{session_created}\t#{session_activity}\t#{window_activity}\t#{session_attached}\t#{@roambench_detached_at}\t#{pane_dead}"
	output, err := exec.CommandContext(ctx, "tmux", "list-panes", "-a", "-F", format).CombinedOutput()
	if err != nil {
		// A stopped server (or absent tmux binary) means no live tmux sessions.
		message := string(output)
		if errors.Is(err, exec.ErrNotFound) || strings.Contains(message, "no server running") || strings.Contains(message, "No such file or directory") {
			return result, nil
		}
		return nil, fmt.Errorf("observe tmux: %w: %s", err, strings.TrimSpace(message))
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(output), "\n"), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 7 {
			return nil, fmt.Errorf("unexpected tmux observation: %q", line)
		}
		created, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid tmux creation time: %w", err)
		}
		observed := result[fields[0]]
		observed.created = time.Unix(created, 0).UTC()
		for _, raw := range fields[1:4] {
			seconds, _ := strconv.ParseInt(raw, 10, 64)
			stamp := time.Unix(seconds, 0).UTC()
			if stamp.After(observed.activity) {
				observed.activity = stamp
			}
		}
		observed.alive = observed.alive || fields[6] == "0"
		if fields[4] == "0" {
			seconds, _ := strconv.ParseInt(fields[5], 10, 64)
			if seconds > 0 {
				stamp := time.Unix(seconds, 0).UTC()
				observed.detached = &stamp
			}
		} else {
			observed.detached = nil
		}
		result[fields[0]] = observed
	}
	return result, nil
}

// ReadSessionStatuses is the read-only CLI inventory. Unlike NewManager it never
// restores, prunes, flushes or kills sessions. Exited entries remain visible as
// long as their metadata is retained by the server's existing cleanup policy.
func ReadSessionStatuses(cfg *config.TerminalConfig, username string) ([]SessionInfo, error) {
	return readSessionStatuses(cfg, username, time.Now(), observeTmuxSessions)
}

func readSessionStatuses(cfg *config.TerminalConfig, username string, now time.Time, observe func() (map[string]tmuxObservation, error)) ([]SessionInfo, error) {
	dir := filepath.Join(cfg.GetPersistDir(), storagePathSegment(username))
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	records := make(map[string]persistedSession)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, exists, err := readPersistedSessionData(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read session %s: %w", entry.Name(), err)
		}
		if !exists || data.ID == "" || data.Username != username {
			continue
		}
		old, ok := records[data.ID]
		if !ok || preferPersistedSession(data, old) {
			records[data.ID] = data
		}
	}
	observed, err := observe()
	if err != nil {
		return nil, err
	}
	result := make([]SessionInfo, 0, len(records))
	for id, data := range records {
		info := SessionInfo{ID: id, Name: data.Name, TaskID: data.TaskID, ProjectID: data.ProjectID, CreatedAt: data.CreatedAt, LastActivity: data.LastActivity, Backend: data.Backend}
		if info.Backend == "" {
			info.Backend = "tmux"
		} // legacy persistent sessions
		if state, ok := observed[id]; ok && info.Backend == "tmux" {
			applyTmuxObservation(&info, state, now)
		} else {
			alive := info.Backend == "pty" && data.PID > 0 && syscall.Kill(data.PID, 0) == nil
			classifySession(&info, now, alive)
		}
		result = append(result, info)
	}
	for id, state := range observed {
		if _, ok := records[id]; ok || !strings.HasPrefix(id, "lt-") {
			continue
		}
		info := SessionInfo{ID: id, Name: id}
		applyTmuxObservation(&info, state, now)
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}
