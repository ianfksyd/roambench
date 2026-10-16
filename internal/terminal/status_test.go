package terminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/config"
)

func TestSessionIdleClassification(t *testing.T) {
	now := time.Unix(1800000000, 0)
	for _, tc := range []struct {
		name           string
		idle, detached time.Duration
		alive          bool
		want           string
	}{
		{"active", time.Minute, 0, true, StatusActive},
		{"two hour boundary", 2 * time.Hour, 0, true, StatusActive},
		{"idle", 2*time.Hour + time.Second, 0, true, StatusIdle},
		{"recent detach", 3 * time.Hour, time.Hour, true, StatusIdle},
		{"day boundary", time.Minute, 24 * time.Hour, true, StatusActive},
		{"detached despite recent output", time.Minute, 24*time.Hour + time.Second, true, StatusDetached},
		{"exit wins", 3 * time.Hour, 25 * time.Hour, false, StatusExited},
		{"future activity", -time.Hour, 0, true, StatusActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := SessionInfo{LastActivity: now.Add(-tc.idle)}
			if tc.detached > 0 {
				stamp := now.Add(-tc.detached)
				info.DetachedAt = &stamp
			}
			classifySession(&info, now, tc.alive)
			if info.Status != tc.want || info.IdleSeconds < 0 {
				t.Fatalf("classification: %+v", info)
			}
		})
	}
}

func TestSessionAssociationsPersistAndRestore(t *testing.T) {
	cfg := config.Defaults().Terminal
	cfg.PersistDir = t.TempDir()
	mgr := newTestManager(t, &cfg)
	info, err := mgr.CreateSessionWithOptions("ian", SessionCreateOptions{Name: " rb-schema-review ", TaskID: "task-1", ProjectID: "project-1"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "rb-schema-review" || info.TaskID != "task-1" || info.ProjectID != "project-1" {
		t.Fatalf("created: %+v", info)
	}
	data := readPersistedSessionFile(t, mgr, "ian", info.ID)
	if data.TaskID != info.TaskID || data.ProjectID != info.ProjectID {
		t.Fatalf("persisted: %+v", data)
	}
	restored := &Manager{cfg: &cfg, hasTmux: true, persistDir: cfg.PersistDir, sessions: map[string]*Session{}, tmuxSessionExists: func(string) bool { return true }}
	restored.loadPersistedSessions()
	if got := restored.sessions[info.ID].Info; got.TaskID != info.TaskID || got.ProjectID != info.ProjectID || got.Name != info.Name {
		t.Fatalf("restored: %+v", got)
	}
}

func TestReadSessionStatusesReadOnly(t *testing.T) {
	now := time.Now().UTC()
	cfg := config.Defaults().Terminal
	cfg.PersistDir = t.TempDir()
	mgr := newTestManager(t, &cfg)
	for _, data := range []persistedSession{
		{ID: "lt-live", Name: "build", Username: "ian", ProjectID: "p1", TaskID: "t1", CreatedAt: now.Add(-80 * time.Hour), LastActivity: now.Add(-75 * time.Hour)},
		{ID: "lt-dead", Name: "old", Username: "ian", CreatedAt: now.Add(-80 * time.Hour), LastActivity: now.Add(-75 * time.Hour)},
		{ID: "plain", Name: "plain", Username: "ian", Backend: "pty", PID: os.Getpid(), CreatedAt: now, LastActivity: now},
	} {
		if err := mgr.writePersistedSession(data); err != nil {
			t.Fatal(err)
		}
	}
	wal := persistedSession{ID: "lt-live", Name: "build-renamed", Username: "ian", ProjectID: "p1", TaskID: "t1", CreatedAt: now.Add(-80 * time.Hour), LastActivity: now.Add(-74 * time.Hour), UpdatedAt: now.Add(time.Minute)}
	writePersistedSessionWALFile(t, mgr, wal)
	dir := filepath.Join(cfg.PersistDir, "ian")
	snapshot := func() map[string]string {
		files, _ := os.ReadDir(dir)
		result := map[string]string{}
		for _, f := range files {
			b, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				t.Fatal(err)
			}
			result[f.Name()] = string(b)
		}
		return result
	}
	before := snapshot()
	result, err := readSessionStatuses(&cfg, "ian", now, func() (map[string]tmuxObservation, error) {
		return map[string]tmuxObservation{
			"lt-live":   {alive: true, activity: now.Add(-time.Minute)},
			"lt-orphan": {alive: true, created: now, activity: now},
			"unrelated": {alive: true, created: now, activity: now},
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 4 {
		t.Fatalf("inventory: %+v", result)
	}
	if result[0].Name != "build-renamed" || result[0].Status != StatusActive || result[0].IdleSeconds != 60 || result[0].TaskID != "t1" || result[0].ProjectID != "p1" {
		t.Fatalf("live: %+v", result[0])
	}
	if result[2].Status != StatusExited || result[3].Status != StatusActive {
		t.Fatalf("inventory: %+v", result)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("read-only listing mutated persisted sessions")
	}
}

func TestTmuxObservationAndDetachHooks(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux unavailable")
	}
	cfg := config.Defaults().Terminal
	cfg.PersistDir = t.TempDir()
	mgr := newManager(&cfg, true)
	defer mgr.Stop()
	info, err := mgr.CreateSessionWithOptions(currentProcessUsername(), SessionCreateOptions{Name: "i1-observer"})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.KillSession(info.ID)
	observed, err := observeTmuxSessions()
	if err != nil {
		t.Fatal(err)
	}
	state, ok := observed[info.ID]
	if !ok || !state.alive || state.detached == nil || time.Since(*state.detached) > time.Minute {
		t.Fatalf("initial observation: %+v", state)
	}
	oldDetach := time.Now().Add(-25 * time.Hour).Unix()
	if out, err := exec.Command("tmux", "set-option", "-t", info.ID, "@roambench_detached_at", strconv.FormatInt(oldDetach, 10)).CombinedOutput(); err != nil {
		t.Fatalf("set old detach: %v %s", err, out)
	}
	// A real control client exercises attached/detached hooks without a terminal.
	cmd := exec.Command("tmux", "-C", "attach-session", "-t", info.ID)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { stdin.Close(); cmd.Wait() }()
	waitFor := func(detached bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			states, err := observeTmuxSessions()
			if err == nil && (states[info.ID].detached != nil) == detached {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("detach state did not become %v", detached)
	}
	waitFor(false)
	if out, err := exec.Command("tmux", "show-options", "-qv", "-t", info.ID, "@roambench_detached_at").Output(); err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("attach hook did not clear timestamp: %q %v", out, err)
	}
	if _, err := stdin.Write([]byte("detach-client\n")); err != nil {
		t.Fatal(err)
	}
	waitFor(true)
	observed, err = observeTmuxSessions()
	if err != nil || observed[info.ID].detached.Unix() <= oldDetach {
		t.Fatalf("detach hook did not refresh timestamp: %+v %v", observed[info.ID], err)
	}
	statuses, err := ReadSessionStatuses(&cfg, currentProcessUsername())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, status := range statuses {
		if status.ID == info.ID {
			found = true
			if status.Name != "i1-observer" || status.Status != StatusActive {
				t.Fatalf("status: %+v", status)
			}
		}
	}
	if !found {
		t.Fatal("missing live session")
	}
}

func TestTmuxObservationCountsDetachedOutput(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux unavailable")
	}
	const id = "lt-i1-background-output"
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", id, "sleep 1.2; printf background-output; sleep 30").CombinedOutput(); err != nil {
		t.Fatalf("create: %v %s", err, out)
	}
	defer exec.Command("tmux", "kill-session", "-t", id).Run()
	initial, err := observeTmuxSessions()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		observed, err := observeTmuxSessions()
		if err != nil {
			t.Fatal(err)
		}
		if observed[id].activity.After(initial[id].activity) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("detached pane output did not advance last activity")
}
