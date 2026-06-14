package terminal

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/config"
)

func newTestManager(t *testing.T, cfg *config.TerminalConfig) *Manager {
	t.Helper()
	if cfg.PersistDir == "" {
		cfg.PersistDir = t.TempDir()
	}
	mgr := NewManager(cfg)
	mgr.hasTmux = false
	return mgr
}

func readPersistedSessionFile(t *testing.T, mgr *Manager, username, sessionID string) persistedSession {
	t.Helper()
	path := mgr.persistedSessionPath(username, sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error: %v", path, err)
	}

	var persisted persistedSession
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal(%s) error: %v", path, err)
	}
	return persisted
}

func writePersistedSessionWALFile(t *testing.T, mgr *Manager, data persistedSession) {
	t.Helper()
	payload, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal persistedSession error: %v", err)
	}
	if err := writePersistedSessionFileAtomically(mgr.persistedSessionWALPath(data.Username, data.ID), payload, 0600); err != nil {
		t.Fatalf("writePersistedSessionWALFile error: %v", err)
	}
}

func TestManagerSessionOwnership(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 10,
		IdleTimeout: "1h",
	})

	session, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	id := session.ID

	if strings.Contains(id, "ian") {
		t.Fatalf("session ID %q should not include username", id)
	}

	if err := mgr.RenameSession("other", id, "other shell"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("RenameSession wrong user error = %v, want %v", err, ErrForbidden)
	}

	if err := mgr.KillSessionForUser("other", id); !errors.Is(err, ErrForbidden) {
		t.Fatalf("KillSessionForUser wrong user error = %v, want %v", err, ErrForbidden)
	}

	if err := mgr.RenameSession("ian", id, "build shell"); err != nil {
		t.Fatalf("RenameSession owner error: %v", err)
	}

	sessions := mgr.ListSessions("ian")
	if len(sessions) != 1 || sessions[0].Name != "build shell" {
		t.Fatalf("ListSessions = %#v, want one renamed session", sessions)
	}

	persisted := readPersistedSessionFile(t, mgr, "ian", id)
	if persisted.Name != "build shell" {
		t.Fatalf("persisted name = %q, want %q", persisted.Name, "build shell")
	}

	if err := mgr.KillSessionForUser("ian", id); err != nil {
		t.Fatalf("KillSessionForUser owner error: %v", err)
	}
	if _, err := os.Stat(mgr.persistedSessionPath("ian", id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("persisted session file still exists, stat err = %v", err)
	}
}

func TestManagerConsumesAttachReplacement(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 10,
		IdleTimeout: "1h",
	})
	defer mgr.Stop()

	session, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	defer mgr.KillSessionForUser("ian", session.ID)

	firstPTY, firstCmd, err := mgr.AttachSessionForUser("ian", session.ID)
	if err != nil {
		t.Fatalf("first AttachSessionForUser error: %v", err)
	}
	if mgr.ConsumeAttachReplacementForUser("ian", session.ID, firstPTY) {
		t.Fatal("first attach was marked replaced before a second attach")
	}

	secondPTY, secondCmd, err := mgr.AttachSessionForUser("ian", session.ID)
	if err != nil {
		t.Fatalf("second AttachSessionForUser error: %v", err)
	}

	if !mgr.ConsumeAttachReplacementForUser("ian", session.ID, firstPTY) {
		t.Fatal("first attach was not marked replaced after second attach")
	}
	if mgr.ConsumeAttachReplacementForUser("ian", session.ID, firstPTY) {
		t.Fatal("first attach replacement marker was not consumed")
	}
	if mgr.ConsumeAttachReplacementForUser("ian", session.ID, secondPTY) {
		t.Fatal("current attach should not be marked replaced")
	}

	if firstCmd != nil && firstCmd.Process != nil {
		_ = firstCmd.Process.Kill()
	}
	if secondCmd != nil && secondCmd.Process != nil {
		_ = secondCmd.Process.Kill()
	}
}

func TestManagerCreateSessionWithOptionsPersistsWorkDir(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 10,
		IdleTimeout: "1h",
	})
	workDir := t.TempDir()

	session, err := mgr.CreateSessionWithOptions("ian", SessionCreateOptions{WorkDir: workDir})
	if err != nil {
		t.Fatalf("CreateSessionWithOptions error: %v", err)
	}

	mgr.mu.Lock()
	recordedWorkDir := ""
	if stored, ok := mgr.sessions[session.ID]; ok {
		recordedWorkDir = stored.WorkDir
	}
	mgr.mu.Unlock()
	if recordedWorkDir != workDir {
		t.Fatalf("session workdir = %q, want %q", recordedWorkDir, workDir)
	}

	persisted := readPersistedSessionFile(t, mgr, "ian", session.ID)
	if persisted.WorkDir != workDir {
		t.Fatalf("persisted workdir = %q, want %q", persisted.WorkDir, workDir)
	}
}

func TestManagerCreateSessionWithOptionsUsesProvidedName(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 10,
		IdleTimeout: "1h",
	})

	session, err := mgr.CreateSessionWithOptions("ian", SessionCreateOptions{Name: "  Implement   panel shell  "})
	if err != nil {
		t.Fatalf("CreateSessionWithOptions error: %v", err)
	}
	if session.Name != "Implement panel shell" {
		t.Fatalf("session name = %q, want %q", session.Name, "Implement panel shell")
	}

	persisted := readPersistedSessionFile(t, mgr, "ian", session.ID)
	if persisted.Name != "Implement panel shell" {
		t.Fatalf("persisted name = %q, want %q", persisted.Name, "Implement panel shell")
	}
}

func TestManagerDefaultNamesAndOrdering(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 10,
		Scrollback:  1000,
		IdleTimeout: "1h",
	})

	first, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession first error: %v", err)
	}
	second, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession second error: %v", err)
	}
	third, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession third error: %v", err)
	}

	if first.Name != "Term 1" || second.Name != "Term 2" || third.Name != "Term 3" {
		t.Fatalf("default names = %q, %q, %q", first.Name, second.Name, third.Name)
	}

	mgr.mu.Lock()
	mgr.sessions[first.ID].Info.CreatedAt = time.Unix(30, 0)
	mgr.sessions[second.ID].Info.CreatedAt = time.Unix(10, 0)
	mgr.sessions[third.ID].Info.CreatedAt = time.Unix(20, 0)
	mgr.mu.Unlock()

	sessions := mgr.ListSessions("ian")
	if len(sessions) != 3 {
		t.Fatalf("ListSessions length = %d, want 3", len(sessions))
	}
	if sessions[0].ID != second.ID || sessions[1].ID != third.ID || sessions[2].ID != first.ID {
		t.Fatalf("ListSessions order = %#v, want %q, %q, %q", sessions, second.ID, third.ID, first.ID)
	}
}

func TestManagerTouchSessionUpdatesLastActivity(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 10,
		Scrollback:  1000,
		IdleTimeout: "1h",
	})

	session, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	oldActivity := time.Now().Add(-2 * time.Hour)
	mgr.mu.Lock()
	mgr.sessions[session.ID].LastActivity = oldActivity
	mgr.sessions[session.ID].dirty = false
	mgr.mu.Unlock()

	mgr.TouchSessionForUser("ian", session.ID)

	mgr.mu.Lock()
	newActivity := mgr.sessions[session.ID].LastActivity
	dirty := mgr.sessions[session.ID].dirty
	mgr.mu.Unlock()

	if !newActivity.After(oldActivity) {
		t.Fatalf("LastActivity = %v, want after %v", newActivity, oldActivity)
	}
	if !dirty {
		t.Fatal("session should be marked dirty after touch")
	}

	mgr.flushDirtySessions()
	persisted := readPersistedSessionFile(t, mgr, "ian", session.ID)
	if !persisted.LastActivity.After(oldActivity) {
		t.Fatalf("persisted LastActivity = %v, want after %v", persisted.LastActivity, oldActivity)
	}
}

func TestManagerMaxSessionsZeroAllowsMoreThanTenSessions(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 0,
		Scrollback:  1000,
		IdleTimeout: "1h",
	})

	for i := 0; i < 12; i++ {
		if _, err := mgr.CreateSession("ian"); err != nil {
			t.Fatalf("CreateSession #%d error: %v", i+1, err)
		}
	}

	sessions := mgr.ListSessions("ian")
	if len(sessions) != 12 {
		t.Fatalf("ListSessions length = %d, want 12", len(sessions))
	}
}

func TestManagerLoadPersistedSessionsRestoresExistingTmuxSessions(t *testing.T) {
	dir := t.TempDir()
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:           "/bin/sh",
		MaxSessions:     0,
		Scrollback:      1000,
		IdleTimeout:     "72h",
		PersistDir:      dir,
		PersistMaxBytes: 64 << 20,
	})

	now := time.Now().UTC().Truncate(time.Second)
	persisted := persistedSession{
		ID:           "lt-restore-me",
		Name:         "Recovered shell",
		Username:     "ian",
		CreatedAt:    now.Add(-10 * time.Minute),
		LastActivity: now.Add(-2 * time.Minute),
	}
	if err := mgr.writePersistedSession(persisted); err != nil {
		t.Fatalf("writePersistedSession error: %v", err)
	}

	mgr.mu.Lock()
	mgr.sessions = make(map[string]*Session)
	mgr.mu.Unlock()
	mgr.hasTmux = true
	mgr.tmuxSessionExists = func(sessionID string) bool {
		return sessionID == persisted.ID
	}

	mgr.loadPersistedSessions()

	sessions := mgr.ListSessions("ian")
	if len(sessions) != 1 {
		t.Fatalf("ListSessions length = %d, want 1", len(sessions))
	}
	if sessions[0].ID != persisted.ID || sessions[0].Name != persisted.Name {
		t.Fatalf("restored session = %#v, want id %q name %q", sessions[0], persisted.ID, persisted.Name)
	}
}

func TestManagerLoadPersistedSessionsPrefersNewerWAL(t *testing.T) {
	dir := t.TempDir()
	baseWorkDir := t.TempDir()
	recoveredWorkDir := t.TempDir()
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:           "/bin/sh",
		MaxSessions:     0,
		Scrollback:      1000,
		IdleTimeout:     "72h",
		PersistDir:      dir,
		PersistMaxBytes: 64 << 20,
	})

	now := time.Now().UTC().Truncate(time.Second)
	base := persistedSession{
		ID:           "lt-wal-newer",
		Name:         "Base shell",
		Username:     "ian",
		WorkDir:      baseWorkDir,
		CreatedAt:    now.Add(-10 * time.Minute),
		LastActivity: now.Add(-2 * time.Minute),
		UpdatedAt:    now.Add(-2 * time.Minute),
	}
	if err := mgr.writePersistedSession(base); err != nil {
		t.Fatalf("writePersistedSession(base) error: %v", err)
	}

	recovered := base
	recovered.Name = "Recovered shell"
	recovered.WorkDir = recoveredWorkDir
	recovered.UpdatedAt = now.Add(-time.Minute)
	writePersistedSessionWALFile(t, mgr, recovered)

	mgr.mu.Lock()
	mgr.sessions = make(map[string]*Session)
	mgr.mu.Unlock()
	mgr.hasTmux = true
	mgr.tmuxSessionExists = func(sessionID string) bool {
		return sessionID == recovered.ID
	}

	mgr.loadPersistedSessions()

	sessions := mgr.ListSessions("ian")
	if len(sessions) != 1 {
		t.Fatalf("ListSessions length = %d, want 1", len(sessions))
	}
	if sessions[0].Name != recovered.Name {
		t.Fatalf("restored session name = %q, want %q", sessions[0].Name, recovered.Name)
	}
	mgr.mu.Lock()
	workDir := mgr.sessions[recovered.ID].WorkDir
	mgr.mu.Unlock()
	if workDir != recovered.WorkDir {
		t.Fatalf("restored workdir = %q, want %q", workDir, recovered.WorkDir)
	}
	if _, err := os.Stat(mgr.persistedSessionWALPath("ian", recovered.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("persisted WAL still exists, stat err = %v", err)
	}
	persisted := readPersistedSessionFile(t, mgr, "ian", recovered.ID)
	if persisted.Name != recovered.Name || persisted.WorkDir != recovered.WorkDir {
		t.Fatalf("persisted session = %#v, want name %q workdir %q", persisted, recovered.Name, recovered.WorkDir)
	}
}

func TestManagerLoadPersistedSessionsRestoresWALWithoutMainFile(t *testing.T) {
	dir := t.TempDir()
	recoveredWorkDir := t.TempDir()
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:           "/bin/sh",
		MaxSessions:     0,
		Scrollback:      1000,
		IdleTimeout:     "72h",
		PersistDir:      dir,
		PersistMaxBytes: 64 << 20,
	})

	now := time.Now().UTC().Truncate(time.Second)
	recovered := persistedSession{
		ID:           "lt-wal-only",
		Name:         "Recovered from WAL",
		Username:     "ian",
		WorkDir:      recoveredWorkDir,
		CreatedAt:    now.Add(-8 * time.Minute),
		LastActivity: now.Add(-time.Minute),
		UpdatedAt:    now.Add(-time.Minute),
	}
	writePersistedSessionWALFile(t, mgr, recovered)

	mgr.mu.Lock()
	mgr.sessions = make(map[string]*Session)
	mgr.mu.Unlock()
	mgr.hasTmux = true
	mgr.tmuxSessionExists = func(sessionID string) bool {
		return sessionID == recovered.ID
	}

	mgr.loadPersistedSessions()

	sessions := mgr.ListSessions("ian")
	if len(sessions) != 1 {
		t.Fatalf("ListSessions length = %d, want 1", len(sessions))
	}
	if sessions[0].ID != recovered.ID || sessions[0].Name != recovered.Name {
		t.Fatalf("restored session = %#v, want id %q name %q", sessions[0], recovered.ID, recovered.Name)
	}
	if _, err := os.Stat(mgr.persistedSessionPath("ian", recovered.ID)); err != nil {
		t.Fatalf("persisted session file missing after WAL recovery: %v", err)
	}
	if _, err := os.Stat(mgr.persistedSessionWALPath("ian", recovered.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("persisted WAL still exists, stat err = %v", err)
	}
}

func TestManagerListSessionsPrunesMissingTmuxSessions(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 10,
		IdleTimeout: "1h",
	})

	session, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	mgr.hasTmux = true
	mgr.tmuxSessionExists = func(string) bool {
		return false
	}

	sessions := mgr.ListSessions("ian")
	if len(sessions) != 0 {
		t.Fatalf("ListSessions length = %d, want 0 after pruning missing tmux session", len(sessions))
	}
	if _, err := os.Stat(mgr.persistedSessionPath("ian", session.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("persisted session file still exists, stat err = %v", err)
	}
}

func TestManagerAttachSessionForUserPrunesMissingTmuxSessions(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 10,
		IdleTimeout: "1h",
	})

	session, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	mgr.hasTmux = true
	mgr.tmuxSessionExists = func(string) bool {
		return false
	}

	if _, _, err := mgr.AttachSessionForUser("ian", session.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AttachSessionForUser error = %v, want %v", err, ErrNotFound)
	}

	mgr.mu.Lock()
	_, ok := mgr.sessions[session.ID]
	mgr.mu.Unlock()
	if ok {
		t.Fatal("session should be removed after missing tmux session is pruned")
	}
	if _, err := os.Stat(mgr.persistedSessionPath("ian", session.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("persisted session file still exists, stat err = %v", err)
	}
}

func TestManagerReapsAttachedCommandAndClearsSessionReference(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 10,
		IdleTimeout: "1h",
	})

	session, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	cmd := startSleepCommand(t)
	defer cmd.Process.Kill()
	done := mgr.reapAttachedCommand(session.ID, cmd, nil)

	mgr.mu.Lock()
	mgr.sessions[session.ID].cmd = cmd
	mgr.mu.Unlock()

	_ = cmd.Process.Kill()
	waitForReaper(t, done)

	mgr.mu.Lock()
	got := mgr.sessions[session.ID].cmd
	mgr.mu.Unlock()
	if got != nil {
		t.Fatalf("session cmd = %#v, want nil after attached command is reaped", got)
	}
}

func TestManagerReapDoesNotClearReplacementAttachment(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 10,
		IdleTimeout: "1h",
	})

	session, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	stale := startSleepCommand(t)
	defer stale.Process.Kill()
	staleDone := mgr.reapAttachedCommand(session.ID, stale, nil)
	replacement := startSleepCommand(t)
	defer replacement.Process.Kill()
	replacementDone := mgr.reapAttachedCommand(session.ID, replacement, nil)

	mgr.mu.Lock()
	mgr.sessions[session.ID].cmd = replacement
	mgr.mu.Unlock()

	_ = stale.Process.Kill()
	waitForReaper(t, staleDone)

	mgr.mu.Lock()
	got := mgr.sessions[session.ID].cmd
	mgr.mu.Unlock()
	if got != replacement {
		t.Fatalf("session cmd = %#v, want replacement command %#v", got, replacement)
	}

	_ = replacement.Process.Kill()
	waitForReaper(t, replacementDone)
}

func TestManagerEnforcePersistedStorageLimitPrunesOldestSessions(t *testing.T) {
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:           "/bin/sh",
		MaxSessions:     0,
		Scrollback:      1000,
		IdleTimeout:     "72h",
		PersistMaxBytes: 4096,
	})

	first, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession first error: %v", err)
	}
	second, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession second error: %v", err)
	}

	mgr.mu.Lock()
	mgr.sessions[first.ID].LastActivity = time.Unix(10, 0)
	mgr.sessions[second.ID].LastActivity = time.Unix(20, 0)
	mgr.mu.Unlock()
	mgr.flushDirtySessions()

	mgr.cfg.PersistMaxBytes = 1
	mgr.enforcePersistedStorageLimit()

	sessions := mgr.ListSessions("ian")
	if len(sessions) != 0 {
		t.Fatalf("ListSessions length = %d, want 0 after aggressive pruning", len(sessions))
	}
}

func TestTerminalCommandEnvSanitizesColorFlags(t *testing.T) {
	env := terminalCommandEnv([]string{
		"TERM=screen.xterm-256color",
		"COLORTERM=",
		"NO_COLOR=1",
		"TERM_PROGRAM=tmux",
		"TERM_PROGRAM_VERSION=3.4",
		"TMUX=/tmp/tmux.sock",
		"STY=123.session",
		"WINDOW=1",
		"PATH=/usr/bin",
	}, "/home/ian")

	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, unwanted := range []string{
		"\nTERM=screen.xterm-256color\n",
		"\nNO_COLOR=1\n",
		"\nTERM_PROGRAM=tmux\n",
		"\nTERM_PROGRAM_VERSION=3.4\n",
		"\nTMUX=/tmp/tmux.sock\n",
		"\nSTY=123.session\n",
		"\nWINDOW=1\n",
	} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("terminalCommandEnv preserved unwanted entry %q in %q", unwanted, joined)
		}
	}

	for _, required := range []string{
		"\nTERM=xterm-256color\n",
		"\nCOLORTERM=truecolor\n",
		"\nHOME=/home/ian\n",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("terminalCommandEnv missing required entry %q in %q", required, joined)
		}
	}
}

func TestTmuxShellCommandForcesColorFriendlyEnv(t *testing.T) {
	command := tmuxShellCommand("/bin/bash", "/home/ian")

	for _, fragment := range []string{
		"env -u TERM_PROGRAM -u TERM_PROGRAM_VERSION -u NO_COLOR -u TMUX -u STY -u WINDOW",
		"TERM='xterm-256color'",
		"COLORTERM='truecolor'",
		"HOME='/home/ian'",
		"SHELL='/bin/bash'",
		" --rcfile '/tmp/.roambench-bashrc' -i",
	} {
		if !strings.Contains(command, fragment) {
			t.Fatalf("tmuxShellCommand missing %q in %q", fragment, command)
		}
	}

	if !shellUsesRcFile("/bin/bash") {
		t.Fatal("shellUsesRcFile failed for /bin/bash")
	}
	if shellUsesRcFile("/bin/sh") {
		t.Fatal("shellUsesRcFile should be false for /bin/sh")
	}
}

func TestTmuxNewSessionArgsStartInHomeDir(t *testing.T) {
	args := tmuxNewSessionArgs("lt-123", "exec /bin/bash", "/home/ian")
	joined := "\n" + strings.Join(args, "\n") + "\n"

	for _, required := range []string{
		"\nnew-session\n",
		"\n-d\n",
		"\n-s\n",
		"\nlt-123\n",
		"\n-c\n",
		"\n/home/ian\n",
		"\n-x\n",
		"\n120\n",
		"\n-y\n",
		"\n40\n",
		"\nexec /bin/bash\n",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("tmuxNewSessionArgs missing %q in %q", required, joined)
		}
	}
}

func TestPersistedSessionPathUsesConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 0,
		IdleTimeout: "1h",
		PersistDir:  dir,
	})

	path := mgr.persistedSessionPath("ian", "lt-123")
	want := filepath.Join(dir, "ian", "lt-123.json")
	if path != want {
		t.Fatalf("persistedSessionPath = %q, want %q", path, want)
	}
}

func TestPersistedStoreSizeIgnoresNonSessionFiles(t *testing.T) {
	dir := t.TempDir()
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 0,
		IdleTimeout: "1h",
		PersistDir:  dir,
	})

	session, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	sessionPath := mgr.persistedSessionPath("ian", session.ID)
	info, err := os.Stat(sessionPath)
	if err != nil {
		t.Fatalf("Stat(%s) error: %v", sessionPath, err)
	}
	before := mgr.persistedStoreSize()
	if before < info.Size() {
		t.Fatalf("persistedStoreSize before unrelated file = %d, want at least %d", before, info.Size())
	}

	unrelatedPath := filepath.Join(dir, ".project-control", "workspaces", "ian", "snapshot.bin")
	if err := os.MkdirAll(filepath.Dir(unrelatedPath), 0700); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	if err := os.WriteFile(unrelatedPath, []byte(strings.Repeat("x", 4096)), 0600); err != nil {
		t.Fatalf("WriteFile(%s) error: %v", unrelatedPath, err)
	}

	if got := mgr.persistedStoreSize(); got != before {
		t.Fatalf("persistedStoreSize = %d, want unchanged size %d", got, before)
	}
}

func TestPersistedStoreSizeIgnoresSessionWALFiles(t *testing.T) {
	dir := t.TempDir()
	mgr := newTestManager(t, &config.TerminalConfig{
		Shell:       "/bin/sh",
		MaxSessions: 0,
		IdleTimeout: "1h",
		PersistDir:  dir,
	})

	session, err := mgr.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	before := mgr.persistedStoreSize()
	if before == 0 {
		t.Fatal("persistedStoreSize = 0, want persisted session size")
	}

	persisted := readPersistedSessionFile(t, mgr, "ian", session.ID)
	persisted.Name = "Recovered later"
	persisted.UpdatedAt = time.Now().UTC().Add(time.Minute)
	writePersistedSessionWALFile(t, mgr, persisted)

	if got := mgr.persistedStoreSize(); got != before {
		t.Fatalf("persistedStoreSize = %d, want unchanged size %d", got, before)
	}
}

func startSleepCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start error: %v", err)
	}
	return cmd
}

func waitForReaper(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("attached command was not reaped before timeout")
	}
}
