package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/ianf339/roambench/internal/config"
	"github.com/ianf339/roambench/internal/systemstats"
)

var (
	ErrMaxSessions            = errors.New("maximum number of sessions reached")
	ErrNotFound               = errors.New("session not found")
	ErrForbidden              = errors.New("session access denied")
	defaultTerminalNameRegexp = regexp.MustCompile(`(?i)^Term (\d+)$`)
)

const (
	terminalClientTerm  = "xterm-256color"
	terminalColorDepth  = "truecolor"
	terminalDefaultRows = 40

	terminalBashRCPath = "/tmp/.roambench-bashrc"
	terminalBashRC     = `# Default colored prompt (kept even when user shell files override it)
# Prefer a color-capable shell setup even when user defaults are plain.
force_color_prompt=yes

# Source system and user shell init
[ -f /etc/bash.bashrc ] && . /etc/bash.bashrc
[ -f ~/.bashrc ] && . ~/.bashrc

# Ensure color output for common commands
command -v dircolors >/dev/null 2>&1 && eval "$(dircolors -b)"
alias ls='ls --color=auto' 2>/dev/null
alias grep='grep --color=auto' 2>/dev/null
alias fgrep='fgrep --color=auto' 2>/dev/null
alias egrep='egrep --color=auto' 2>/dev/null

# Override prompt style after user startup files are loaded.
PS1='\[\033[01;32m\]\u@\h\[\033[00m\]:\[\033[01;34m\]\w\[\033[00m\]\$ '
`

	sessionCleanupInterval = 10 * time.Minute
	sessionFlushInterval   = time.Minute
)

type SessionInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type SessionCreateOptions struct {
	WorkDir string
	Name    string
}

type ScrollState struct {
	InMode         bool
	HistorySize    int
	ScrollPosition int
	PaneHeight     int
}

type persistedSession struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Username     string    `json:"username"`
	WorkDir      string    `json:"workDir,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActivity time.Time `json:"lastActivity"`
	UpdatedAt    time.Time `json:"updatedAt,omitempty"`
}

type tmuxSessionState struct {
	ID           string
	CreatedAt    time.Time
	LastActivity time.Time
}

type Session struct {
	Info            SessionInfo
	Username        string
	WorkDir         string
	LastActivity    time.Time
	cmd             *exec.Cmd
	ptyFile         *os.File
	replacedPTYs    map[*os.File]struct{}
	dirty           bool
	cgroupProcsPath string
}

type Manager struct {
	cfg               *config.TerminalConfig
	hasTmux           bool
	mu                sync.Mutex
	sessions          map[string]*Session
	sessionEnded      func(username, sessionID string)
	cancel            context.CancelFunc
	persistDir        string
	tmuxSessionExists func(string) bool
	readSystemStats   func() (systemstats.Snapshot, error)
	sessionCgroups    *sessionCgroupManager
	sessionCgroupErr  error
}

func (m *Manager) SetSessionEndedHandler(handler func(username, sessionID string)) {
	m.mu.Lock()
	m.sessionEnded = handler
	m.mu.Unlock()
}

func NewManager(cfg *config.TerminalConfig) *Manager {
	_, err := exec.LookPath("tmux")
	sessionCgroups, sessionCgroupErr := newSessionCgroupManager(cfg.Resources)
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		cfg:        cfg,
		hasTmux:    err == nil,
		sessions:   make(map[string]*Session),
		cancel:     cancel,
		persistDir: cfg.GetPersistDir(),
		tmuxSessionExists: func(sessionID string) bool {
			return exec.Command("tmux", "has-session", "-t", sessionID).Run() == nil
		},
		readSystemStats:  systemstats.Read,
		sessionCgroups:   sessionCgroups,
		sessionCgroupErr: sessionCgroupErr,
	}
	if m.hasTmux {
		// Set global tmux defaults BEFORE any session is created so the
		// first window of every session inherits the correct terminal type.
		exec.Command("tmux", "set-option", "-g", "default-terminal", terminalClientTerm).Run()
		exec.Command("tmux", "set-option", "-g", "mouse", "off").Run()
		m.ensureTmuxFeature(terminalClientTerm, "RGB")
		m.ensureTmuxFeature("screen."+terminalClientTerm, "RGB")
	}
	os.MkdirAll(m.persistDir, 0700)
	m.loadPersistedSessions()
	go m.cleanupLoop(ctx)
	os.WriteFile(terminalBashRCPath, []byte(terminalBashRC), 0644)
	return m
}

func (m *Manager) Stop() {
	m.flushDirtySessions()
	m.cancel()
}

func (m *Manager) HasTmux() bool {
	return m.hasTmux
}

func (m *Manager) ListSessions(username string) []SessionInfo {
	m.mu.Lock()
	var result []SessionInfo
	for _, s := range m.sessions {
		if s.Username == username {
			result = append(result, s.Info)
		}
	}
	m.mu.Unlock()

	if m.hasTmux {
		filtered := result[:0]
		for _, info := range result {
			if m.pruneMissingTmuxSession(info.ID) {
				continue
			}
			filtered = append(filtered, info)
		}
		result = filtered
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result
}

func (m *Manager) CreateSession(username string) (SessionInfo, error) {
	return m.CreateSessionWithOptions(username, SessionCreateOptions{})
}

func (m *Manager) CreateSessionWithOptions(username string, opts SessionCreateOptions) (SessionInfo, error) {
	if err := m.checkResourceAdmission(); err != nil {
		return SessionInfo{}, err
	}

	m.mu.Lock()

	count := 0
	for _, s := range m.sessions {
		if s.Username == username {
			count++
		}
	}
	if m.cfg.MaxSessions > 0 && count >= m.cfg.MaxSessions {
		m.mu.Unlock()
		return SessionInfo{}, ErrMaxSessions
	}

	id := m.generateID()
	if m.sessionCgroupErr != nil {
		m.mu.Unlock()
		return SessionInfo{}, m.sessionCgroupErr
	}
	cgroupProcsPath := ""
	if m.sessionCgroups != nil {
		var prepareErr error
		cgroupProcsPath, prepareErr = m.sessionCgroups.Prepare(id)
		if prepareErr != nil {
			m.mu.Unlock()
			return SessionInfo{}, prepareErr
		}
	}
	name, err := normalizeSessionName(opts.Name)
	if err != nil {
		m.sessionCgroups.Cleanup(id)
		m.mu.Unlock()
		return SessionInfo{}, err
	}
	if name == "" {
		name = m.nextDefaultNameLocked(username)
	}
	now := time.Now()
	homeDir := lookupHomeDir(username)
	workDir := resolveSessionWorkDir(opts.WorkDir, homeDir)
	session := &Session{
		Info: SessionInfo{
			ID:        id,
			Name:      name,
			CreatedAt: now,
		},
		Username:        username,
		WorkDir:         workDir,
		LastActivity:    now,
		dirty:           false,
		cgroupProcsPath: cgroupProcsPath,
	}
	m.sessions[id] = session
	m.mu.Unlock()

	if m.hasTmux {
		defaultCommand := tmuxShellCommandInCgroup(m.cfg.Shell, homeDir, cgroupProcsPath)
		cmdArgs := tmuxNewSessionArgs(id, defaultCommand, workDir)
		cmd := exec.Command("tmux", cmdArgs...)
		if workDir != "" {
			cmd.Dir = workDir
		}
		cmd.Env = terminalCommandEnv(os.Environ(), homeDir)
		if err := cmd.Run(); err != nil {
			m.mu.Lock()
			delete(m.sessions, id)
			m.mu.Unlock()
			m.sessionCgroups.Cleanup(id)
			return SessionInfo{}, fmt.Errorf("tmux new-session: %w", err)
		}
		m.configureTmuxTerminal(id, defaultCommand)
		if m.cfg.Scrollback > 0 {
			exec.Command("tmux", "set-option", "-t", id, "history-limit", strconv.Itoa(m.cfg.Scrollback)).Run()
		}
		// Disable status bar and mouse mode for web use — mouse mode
		// captures click/drag events which breaks xterm.js text selection.
		exec.Command("tmux", "set-option", "-t", id, "status", "off").Run()
		exec.Command("tmux", "set-option", "-t", id, "mouse", "off").Run()
	}

	if err := m.persistSession(id); err != nil {
		m.KillSession(id)
		return SessionInfo{}, fmt.Errorf("persist session: %w", err)
	}
	m.enforcePersistedStorageLimit()

	m.mu.Lock()
	info := session.Info
	m.mu.Unlock()
	return info, nil
}

func (m *Manager) checkResourceAdmission() error {
	resources := m.cfg.Resources
	if resources.MinSystemAvailableBytes <= 0 && resources.MaxSystemSwapUsedPercent <= 0 && resources.MaxMemoryPressureAvg10 <= 0 {
		return nil
	}

	snapshot, err := m.readSystemStats()
	if err != nil {
		return fmt.Errorf("%w: system statistics unavailable: %v", systemstats.ErrAdmissionDenied, err)
	}
	return snapshot.CheckAdmission(systemstats.AdmissionPolicy{
		MinSystemAvailableBytes:  uint64(resources.MinSystemAvailableBytes),
		MaxSystemSwapUsedPercent: resources.MaxSystemSwapUsedPercent,
		MaxMemoryPressureAvg10:   resources.MaxMemoryPressureAvg10,
	})
}

func (m *Manager) sessionForUser(username, sessionID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	if session.Username != username {
		return nil, ErrForbidden
	}
	return session, nil
}

// AttachSession returns a pty file descriptor for the session.
// The caller must close the returned file when done. The manager waits for the
// returned command when it exits so attachment processes are reaped.
func (m *Manager) AttachSessionForUser(username, sessionID string) (*os.File, *exec.Cmd, error) {
	session, err := m.sessionForUser(username, sessionID)
	if err != nil {
		return nil, nil, err
	}
	if m.pruneMissingTmuxSession(sessionID) {
		return nil, nil, ErrNotFound
	}

	m.mu.Lock()
	if session.ptyFile != nil {
		if session.replacedPTYs == nil {
			session.replacedPTYs = make(map[*os.File]struct{})
		}
		session.replacedPTYs[session.ptyFile] = struct{}{}
		session.ptyFile.Close()
		session.ptyFile = nil
	}
	if session.cmd != nil && session.cmd.Process != nil {
		session.cmd.Process.Kill()
	}
	session.cmd = nil
	m.mu.Unlock()

	var cmd *exec.Cmd
	homeDir := lookupHomeDir(session.Username)
	workDir := resolveSessionWorkDir(session.WorkDir, homeDir)
	if m.hasTmux {
		cmd = exec.Command("tmux", "attach-session", "-t", sessionID)
		cmd.Env = terminalCommandEnv(os.Environ(), homeDir)
		m.reconcileTmuxSession(sessionID, homeDir)
	} else {
		if shellUsesRcFile(m.cfg.Shell) {
			cmd = exec.Command(m.cfg.Shell, "-i", "--rcfile", terminalBashRCPath)
		} else {
			cmd = exec.Command(m.cfg.Shell, "-i")
		}
		if workDir != "" {
			cmd.Dir = workDir
		}
		cmd.Env = terminalCommandEnv(os.Environ(), homeDir)
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, nil, fmt.Errorf("pty start: %w", err)
	}
	if !m.hasTmux && session.cgroupProcsPath != "" {
		if err := writeCgroupValue(session.cgroupProcsPath, strconv.Itoa(cmd.Process.Pid)); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			_ = ptmx.Close()
			return nil, nil, fmt.Errorf("move terminal process into session cgroup: %w", err)
		}
	}

	pty.Setsize(ptmx, &pty.Winsize{Rows: 40, Cols: 120})

	m.mu.Lock()
	session.cmd = cmd
	session.ptyFile = ptmx
	session.LastActivity = time.Now()
	session.dirty = true
	m.mu.Unlock()

	m.reapAttachedCommand(sessionID, cmd, ptmx)

	return ptmx, cmd, nil
}

// ConsumeAttachReplacementForUser reports whether the given PTY was detached
// because a newer browser connection attached to the same terminal session.
// The replacement marker is consumed so callers can distinguish replacement
// from a normal shell/tmux exit without leaking stale PTY references.
func (m *Manager) ConsumeAttachReplacementForUser(username, sessionID string, ptyFile *os.File) bool {
	if ptyFile == nil {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok || session.Username != username || session.replacedPTYs == nil {
		return false
	}

	if _, replaced := session.replacedPTYs[ptyFile]; replaced {
		delete(session.replacedPTYs, ptyFile)
		return true
	}

	return false
}

func (m *Manager) pruneMissingTmuxSession(sessionID string) bool {
	if !m.hasTmux || sessionID == "" {
		return false
	}
	if m.tmuxSessionExists(sessionID) {
		return false
	}
	_ = m.KillSession(sessionID)
	return true
}

func (m *Manager) reapAttachedCommand(sessionID string, cmd *exec.Cmd, ptyFile *os.File) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if cmd == nil {
			return
		}

		_ = cmd.Wait()
		if ptyFile != nil {
			ptyFile.Close()
		}

		m.mu.Lock()
		defer m.mu.Unlock()
		session, ok := m.sessions[sessionID]
		if !ok {
			return
		}
		if session.cmd == cmd {
			session.cmd = nil
		}
		if session.ptyFile == ptyFile {
			session.ptyFile = nil
		}
	}()
	return done
}

func (m *Manager) ResizeSessionForUser(username, sessionID string, rows, cols uint16) error {
	session, err := m.sessionForUser(username, sessionID)
	if err != nil {
		return err
	}

	if session.ptyFile != nil {
		pty.Setsize(session.ptyFile, &pty.Winsize{Rows: rows, Cols: cols})
	}

	if m.hasTmux {
		exec.Command("tmux", "resize-window", "-t", sessionID,
			"-x", fmt.Sprintf("%d", cols), "-y", fmt.Sprintf("%d", rows)).Run()
	}

	m.TouchSessionForUser(username, sessionID)
	return nil
}

func (m *Manager) ScrollSessionForUser(username, sessionID string, lines int) error {
	if _, err := m.sessionForUser(username, sessionID); err != nil {
		return err
	}
	if !m.hasTmux || lines == 0 {
		return nil
	}

	amount := abs(lines)
	if amount > 100 {
		amount = 100
	}

	inCopyMode := m.tmuxPaneInCopyMode(sessionID)
	if lines < 0 {
		if !inCopyMode {
			exec.Command("tmux", "copy-mode", "-e", "-t", sessionID).Run()
			inCopyMode = true
		}
		if inCopyMode {
			exec.Command("tmux", "send-keys", "-X", "-N", strconv.Itoa(amount), "-t", sessionID, "scroll-up").Run()
		}
	} else if inCopyMode {
		exec.Command("tmux", "send-keys", "-X", "-N", strconv.Itoa(amount), "-t", sessionID, "scroll-down").Run()
	}

	m.TouchSessionForUser(username, sessionID)
	return nil
}

func (m *Manager) ResetScrollSessionForUser(username, sessionID string) error {
	if _, err := m.sessionForUser(username, sessionID); err != nil {
		return err
	}
	if !m.hasTmux || !m.tmuxPaneInCopyMode(sessionID) {
		return nil
	}

	exec.Command("tmux", "send-keys", "-X", "-t", sessionID, "cancel").Run()
	m.TouchSessionForUser(username, sessionID)
	return nil
}

func (m *Manager) ScrollStateForUser(username, sessionID string) (ScrollState, error) {
	if _, err := m.sessionForUser(username, sessionID); err != nil {
		return ScrollState{}, err
	}

	if !m.hasTmux {
		return ScrollState{
			InMode:         false,
			HistorySize:    0,
			ScrollPosition: 0,
			PaneHeight:     terminalDefaultRows,
		}, nil
	}

	output, err := exec.Command("tmux", "display-message", "-p", "-t", sessionID, "#{pane_in_mode}|#{history_size}|#{scroll_position}|#{pane_height}").Output()
	if err != nil {
		return ScrollState{}, err
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) != 4 {
		return ScrollState{}, fmt.Errorf("unexpected tmux scroll state output: %q", strings.TrimSpace(string(output)))
	}

	return ScrollState{
		InMode:         strings.TrimSpace(parts[0]) == "1",
		HistorySize:    parseTmuxInt(parts[1]),
		ScrollPosition: parseTmuxInt(parts[2]),
		PaneHeight:     max(parseTmuxInt(parts[3]), terminalDefaultRows),
	}, nil
}

func (m *Manager) TouchSessionForUser(username, sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok || session.Username != username {
		return
	}
	session.LastActivity = time.Now()
	session.dirty = true
}

func (m *Manager) tmuxPaneInCopyMode(sessionID string) bool {
	output, err := exec.Command("tmux", "display-message", "-p", "-t", sessionID, "#{pane_in_mode}").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "1"
}

func parseTmuxInt(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (m *Manager) RenameSession(username, sessionID, name string) error {
	normalized, err := normalizeSessionName(name)
	if err != nil {
		return err
	}

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	if session.Username != username {
		m.mu.Unlock()
		return ErrForbidden
	}

	previousName := session.Info.Name
	session.Info.Name = normalized
	session.dirty = true
	m.mu.Unlock()

	if err := m.persistSession(sessionID); err != nil {
		m.mu.Lock()
		if rollback, ok := m.sessions[sessionID]; ok && rollback.Username == username {
			rollback.Info.Name = previousName
			rollback.dirty = true
		}
		m.mu.Unlock()
		return err
	}

	return nil
}

func normalizeSessionName(name string) (string, error) {
	normalized := strings.Join(strings.Fields(name), " ")
	if normalized == "" {
		return "", nil
	}
	if len(normalized) > 80 {
		return "", errors.New("terminal name must be 80 characters or less")
	}
	return normalized, nil
}

func (m *Manager) KillSessionForUser(username, sessionID string) error {
	if _, err := m.sessionForUser(username, sessionID); err != nil {
		return err
	}
	return m.KillSession(sessionID)
}

func (m *Manager) KillSession(sessionID string) error {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	handler := m.sessionEnded
	if ok {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()

	if !ok {
		return ErrNotFound
	}

	if session.ptyFile != nil {
		session.ptyFile.Close()
	}
	if session.cmd != nil && session.cmd.Process != nil {
		session.cmd.Process.Kill()
	}

	if m.hasTmux {
		exec.Command("tmux", "kill-session", "-t", sessionID).Run()
	}
	m.sessionCgroups.Cleanup(sessionID)

	m.removePersistedSession(session.Username, sessionID)
	if handler != nil {
		handler(session.Username, sessionID)
	}
	return nil
}

func (m *Manager) generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "lt-" + hex.EncodeToString(b)
}

func (m *Manager) nextDefaultNameLocked(username string) string {
	maxNumber := 0
	for _, session := range m.sessions {
		if session.Username != username {
			continue
		}
		match := defaultTerminalNameRegexp.FindStringSubmatch(session.Info.Name)
		if len(match) != 2 {
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		if number > maxNumber {
			maxNumber = number
		}
	}
	return fmt.Sprintf("Term %d", maxNumber+1)
}

func (m *Manager) configureTmuxTerminal(sessionID, defaultCommand string) {
	exec.Command("tmux", "set-option", "-t", sessionID, "default-terminal", terminalClientTerm).Run()
	exec.Command("tmux", "set-environment", "-t", sessionID, "TERM", terminalClientTerm).Run()
	exec.Command("tmux", "set-environment", "-t", sessionID, "COLORTERM", terminalColorDepth).Run()
	exec.Command("tmux", "set-option", "-t", sessionID, "default-command", defaultCommand).Run()
	// Disable mouse mode so xterm.js handles selection/copy natively
	exec.Command("tmux", "set-option", "-t", sessionID, "mouse", "off").Run()
	exec.Command("tmux", "set-option", "-t", sessionID, "set-titles", "off").Run()
	m.ensureTmuxFeature("xterm-256color", "RGB")
	m.ensureTmuxFeature("screen.xterm-256color", "RGB")
}

func (m *Manager) reconcileTmuxSession(sessionID, homeDir string) {
	exec.Command("tmux", "set-option", "-t", sessionID, "default-terminal", terminalClientTerm).Run()
	exec.Command("tmux", "set-environment", "-t", sessionID, "TERM", terminalClientTerm).Run()
	exec.Command("tmux", "set-environment", "-t", sessionID, "COLORTERM", terminalColorDepth).Run()
	exec.Command("tmux", "set-option", "-t", sessionID, "mouse", "off").Run()
	exec.Command("tmux", "set-option", "-t", sessionID, "set-titles", "off").Run()
	exec.Command("tmux", "set-environment", "-t", sessionID, "HOME", homeDir).Run()
	m.ensureTmuxFeature(terminalClientTerm, "RGB")
	m.ensureTmuxFeature("screen."+terminalClientTerm, "RGB")
}

func (m *Manager) ensureTmuxFeature(term, feature string) {
	current, err := exec.Command("tmux", "show-options", "-g", "terminal-features").CombinedOutput()
	if err == nil && strings.Contains(string(current), term+":"+feature) {
		return
	}
	exec.Command("tmux", "set-option", "-as", "terminal-features", ","+term+":"+feature).Run()
}

func terminalCommandEnv(baseEnv []string, homeDir string) []string {
	replacedKeys := map[string]struct{}{
		"TERM":                 {},
		"COLORTERM":            {},
		"NO_COLOR":             {},
		"TERM_PROGRAM":         {},
		"TERM_PROGRAM_VERSION": {},
		"TMUX":                 {},
		"STY":                  {},
		"WINDOW":               {},
	}

	env := make([]string, 0, len(baseEnv)+5)
	for _, item := range baseEnv {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 0 {
			continue
		}
		if _, skip := replacedKeys[parts[0]]; skip {
			continue
		}
		env = append(env, item)
	}

	env = append(env,
		"TERM="+terminalClientTerm,
		"COLORTERM="+terminalColorDepth,
		fmt.Sprintf("HOME=%s", homeDir),
	)
	return env
}

func tmuxShellCommand(shellPath, homeDir string) string {
	return tmuxShellCommandInCgroup(shellPath, homeDir, "")
}

func tmuxShellCommandInCgroup(shellPath, homeDir, cgroupProcsPath string) string {
	command := ""
	if cgroupProcsPath != "" {
		command = fmt.Sprintf("printf '%%s\\n' \"$$\" > %s || exit 125; ", shQuote(cgroupProcsPath))
	}
	command += fmt.Sprintf(
		"exec env -u TERM_PROGRAM -u TERM_PROGRAM_VERSION -u NO_COLOR -u TMUX -u STY -u WINDOW TERM=%s COLORTERM=%s HOME=%s SHELL=%s ",
		shQuote(terminalClientTerm),
		shQuote(terminalColorDepth),
		shQuote(homeDir),
		shQuote(shellPath),
	)

	command += shQuote(shellPath)
	if shellUsesRcFile(shellPath) {
		command += " --rcfile " + shQuote(terminalBashRCPath) + " -i"
	} else {
		command += " -i"
	}

	return command
}

func tmuxNewSessionArgs(sessionID, defaultCommand, homeDir string) []string {
	args := []string{"new-session", "-d", "-s", sessionID}
	if homeDir != "" {
		args = append(args, "-c", homeDir)
	}
	args = append(args, "-x", "120", "-y", "40", defaultCommand)
	return args
}

func shQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func shellUsesRcFile(shellPath string) bool {
	return filepath.Base(shellPath) == "bash"
}

func lookupHomeDir(username string) string {
	homeDir := os.Getenv("HOME")
	if u, err := user.Lookup(username); err == nil && u.HomeDir != "" {
		homeDir = u.HomeDir
	}
	return homeDir
}

func resolveSessionWorkDir(requested, fallback string) string {
	candidates := []string{requested, fallback}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if !filepath.IsAbs(candidate) {
			absPath, err := filepath.Abs(candidate)
			if err != nil {
				continue
			}
			candidate = absPath
		}
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil && strings.TrimSpace(resolved) != "" {
			candidate = resolved
		}
		info, err := os.Stat(candidate)
		if err != nil || !info.IsDir() {
			continue
		}
		return candidate
	}
	return ""
}

func (m *Manager) cleanupLoop(ctx context.Context) {
	flushTicker := time.NewTicker(sessionFlushInterval)
	cleanupTicker := time.NewTicker(sessionCleanupInterval)
	defer flushTicker.Stop()
	defer cleanupTicker.Stop()

	idleTimeout := m.cfg.GetIdleTimeout()

	for {
		select {
		case <-ctx.Done():
			return
		case <-flushTicker.C:
			m.flushDirtySessions()
		case <-cleanupTicker.C:
			m.flushDirtySessions()

			m.mu.Lock()
			now := time.Now()
			var toDelete []string
			for id, s := range m.sessions {
				if now.Sub(s.LastActivity) > idleTimeout {
					toDelete = append(toDelete, id)
				}
			}
			m.mu.Unlock()

			for _, id := range toDelete {
				m.KillSession(id)
			}
		}
	}
}

func (m *Manager) flushDirtySessions() {
	var dirtyIDs []string

	m.mu.Lock()
	for id, session := range m.sessions {
		if session.dirty {
			dirtyIDs = append(dirtyIDs, id)
		}
	}
	m.mu.Unlock()

	if len(dirtyIDs) == 0 {
		return
	}

	for _, id := range dirtyIDs {
		if err := m.persistSession(id); err != nil {
			m.mu.Lock()
			if session, ok := m.sessions[id]; ok {
				session.dirty = true
			}
			m.mu.Unlock()
		}
	}

	m.enforcePersistedStorageLimit()
}

func (m *Manager) persistSession(sessionID string) error {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	data := persistedSession{
		ID:           session.Info.ID,
		Name:         session.Info.Name,
		Username:     session.Username,
		WorkDir:      session.WorkDir,
		CreatedAt:    session.Info.CreatedAt,
		LastActivity: session.LastActivity,
	}
	m.mu.Unlock()

	if err := m.writePersistedSession(data); err != nil {
		return err
	}

	m.mu.Lock()
	if session, ok := m.sessions[sessionID]; ok {
		session.dirty = session.Info.Name != data.Name ||
			session.WorkDir != data.WorkDir ||
			!session.Info.CreatedAt.Equal(data.CreatedAt) ||
			!session.LastActivity.Equal(data.LastActivity)
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) writePersistedSession(data persistedSession) error {
	if data.Username == "" || data.ID == "" {
		return errors.New("session metadata incomplete")
	}
	if data.LastActivity.IsZero() {
		data.LastActivity = data.CreatedAt
	}
	if data.LastActivity.IsZero() {
		data.LastActivity = time.Now()
	}
	if data.UpdatedAt.IsZero() {
		data.UpdatedAt = time.Now().UTC()
	}

	dir := filepath.Dir(m.persistedSessionPath(data.Username, data.ID))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	path := m.persistedSessionPath(data.Username, data.ID)
	walPath := m.persistedSessionWALPath(data.Username, data.ID)
	backupPath := m.persistedSessionBackupPath(data.Username, data.ID)
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if err := writePersistedSessionFileAtomically(walPath, payload, 0600); err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if err := writePersistedSessionFileAtomically(backupPath, existing, 0600); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := writePersistedSessionFileAtomically(path, payload, 0600); err != nil {
		return err
	}
	_ = os.Remove(walPath)
	return nil
}

func (m *Manager) removePersistedSession(username, sessionID string) {
	path := m.persistedSessionPath(username, sessionID)
	walPath := m.persistedSessionWALPath(username, sessionID)
	backupPath := m.persistedSessionBackupPath(username, sessionID)
	os.Remove(path)
	os.Remove(walPath)
	os.Remove(backupPath)
	parent := filepath.Dir(path)
	entries, err := os.ReadDir(parent)
	if err == nil && len(entries) == 0 {
		os.Remove(parent)
	}
}

func (m *Manager) persistedSessionPath(username, sessionID string) string {
	return filepath.Join(m.persistDir, storagePathSegment(username), storagePathSegment(sessionID)+".json")
}

func (m *Manager) persistedSessionWALPath(username, sessionID string) string {
	return filepath.Join(m.persistDir, storagePathSegment(username), storagePathSegment(sessionID)+".wal.json")
}

func (m *Manager) persistedSessionBackupPath(username, sessionID string) string {
	return m.persistedSessionPath(username, sessionID) + ".bak"
}

func storagePathSegment(value string) string {
	return strings.ReplaceAll(value, string(os.PathSeparator), "_")
}

func readPersistedSessionData(path string) (persistedSession, bool, error) {
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return persistedSession{}, false, nil
	}
	if err != nil {
		return persistedSession{}, false, err
	}

	var data persistedSession
	if err := json.Unmarshal(payload, &data); err != nil {
		return persistedSession{}, false, err
	}
	return data, true, nil
}

func persistedSessionRecency(data persistedSession) time.Time {
	if !data.UpdatedAt.IsZero() {
		return data.UpdatedAt
	}
	if !data.LastActivity.IsZero() {
		return data.LastActivity
	}
	return data.CreatedAt
}

func preferPersistedSession(candidate, current persistedSession) bool {
	candidateTime := persistedSessionRecency(candidate)
	currentTime := persistedSessionRecency(current)
	switch {
	case !candidateTime.IsZero() && !currentTime.IsZero():
		if candidateTime.Equal(currentTime) {
			return true
		}
		return candidateTime.After(currentTime)
	case !candidateTime.IsZero() && currentTime.IsZero():
		return true
	case candidateTime.IsZero() && !currentTime.IsZero():
		return false
	default:
		return true
	}
}

func writePersistedSessionFileAtomically(path string, payload []byte, mode os.FileMode) error {
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

func (m *Manager) loadPersistedSessions() {
	if !m.hasTmux {
		return
	}

	files, err := filepath.Glob(filepath.Join(m.persistDir, "*", "*.json"))
	if err != nil {
		return
	}

	idleTimeout := m.cfg.GetIdleTimeout()
	now := time.Now()
	seen := map[string]bool{}

	for _, path := range files {
		base := filepath.Base(path)
		dir := filepath.Dir(path)
		var key string
		switch {
		case strings.HasSuffix(base, ".wal.json"):
			key = filepath.Join(dir, strings.TrimSuffix(base, ".wal.json"))
		case strings.HasSuffix(base, ".json"):
			key = filepath.Join(dir, strings.TrimSuffix(base, ".json"))
		default:
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		mainPath := key + ".json"
		walPath := key + ".wal.json"
		data, exists, err := readPersistedSessionData(mainPath)
		if err != nil {
			_ = os.Remove(mainPath)
			data = persistedSession{}
			exists = false
		}
		walData, walExists, err := readPersistedSessionData(walPath)
		if err != nil {
			_ = os.Remove(walPath)
			walData = persistedSession{}
			walExists = false
		}
		if !exists && !walExists {
			continue
		}

		selectedFromWAL := false
		clearWAL := false
		switch {
		case walExists && (!exists || preferPersistedSession(walData, data)):
			data = walData
			exists = true
			selectedFromWAL = true
		case walExists:
			clearWAL = true
		}
		if !exists {
			continue
		}
		if data.ID == "" || data.Username == "" {
			_ = os.Remove(mainPath)
			_ = os.Remove(walPath)
			continue
		}
		if data.LastActivity.IsZero() {
			data.LastActivity = data.CreatedAt
		}
		if data.CreatedAt.IsZero() {
			data.CreatedAt = data.LastActivity
		}
		if data.CreatedAt.IsZero() {
			_ = os.Remove(mainPath)
			_ = os.Remove(walPath)
			continue
		}
		if now.Sub(data.LastActivity) > idleTimeout {
			exec.Command("tmux", "kill-session", "-t", data.ID).Run()
			m.removePersistedSession(data.Username, data.ID)
			continue
		}
		if !m.tmuxSessionExists(data.ID) {
			m.removePersistedSession(data.Username, data.ID)
			continue
		}
		if selectedFromWAL {
			if err := m.writePersistedSession(data); err != nil {
				continue
			}
		} else if clearWAL {
			_ = os.Remove(walPath)
		}

		m.mu.Lock()
		existing, exists := m.sessions[data.ID]
		if !exists || existing.LastActivity.Before(data.LastActivity) {
			m.sessions[data.ID] = &Session{
				Info: SessionInfo{
					ID:        data.ID,
					Name:      data.Name,
					CreatedAt: data.CreatedAt,
				},
				Username:     data.Username,
				WorkDir:      resolveSessionWorkDir(data.WorkDir, lookupHomeDir(data.Username)),
				LastActivity: data.LastActivity,
			}
		}
		m.mu.Unlock()
	}

	m.loadOrphanTmuxSessions()
	m.enforcePersistedStorageLimit()
}

func (m *Manager) enforcePersistedStorageLimit() {
	limit := m.cfg.GetPersistMaxBytes()
	if limit <= 0 {
		return
	}

	for {
		total := m.persistedStoreSize()
		if total <= limit {
			return
		}

		victimID := m.oldestSessionID()
		if victimID == "" {
			return
		}
		m.KillSession(victimID)
	}
}

func (m *Manager) persistedStoreSize() int64 {
	var total int64
	files, err := filepath.Glob(filepath.Join(m.persistDir, "*", "*.json"))
	if err != nil {
		return 0
	}
	for _, path := range files {
		if strings.HasSuffix(path, ".wal.json") {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		total += info.Size()
	}
	return total
}

func (m *Manager) oldestSessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	type candidate struct {
		id           string
		lastActivity time.Time
	}

	var candidates []candidate
	for id, session := range m.sessions {
		candidates = append(candidates, candidate{
			id:           id,
			lastActivity: session.LastActivity,
		})
	}
	if len(candidates) == 0 {
		return ""
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].lastActivity.Equal(candidates[j].lastActivity) {
			return candidates[i].id < candidates[j].id
		}
		return candidates[i].lastActivity.Before(candidates[j].lastActivity)
	})
	return candidates[0].id
}

func (m *Manager) loadOrphanTmuxSessions() {
	username := currentProcessUsername()
	if username == "" {
		return
	}

	idleTimeout := m.cfg.GetIdleTimeout()
	for _, state := range m.listTmuxSessions() {
		if !strings.HasPrefix(state.ID, "lt-") {
			continue
		}
		if time.Since(state.LastActivity) > idleTimeout {
			exec.Command("tmux", "kill-session", "-t", state.ID).Run()
			continue
		}

		m.mu.Lock()
		if _, exists := m.sessions[state.ID]; exists {
			m.mu.Unlock()
			continue
		}
		session := &Session{
			Info: SessionInfo{
				ID:        state.ID,
				Name:      m.nextDefaultNameLocked(username),
				CreatedAt: state.CreatedAt,
			},
			Username:     username,
			LastActivity: state.LastActivity,
		}
		m.sessions[state.ID] = session
		m.mu.Unlock()

		m.persistSession(state.ID)
	}
}

func (m *Manager) listTmuxSessions() []tmuxSessionState {
	output, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}\t#{session_created}\t#{session_activity}").Output()
	if err != nil {
		return nil
	}

	var result []tmuxSessionState
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}

		createdUnix, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		activityUnix, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			activityUnix = createdUnix
		}

		createdAt := time.Unix(createdUnix, 0).UTC()
		lastActivity := time.Unix(activityUnix, 0).UTC()
		if lastActivity.Before(createdAt) {
			lastActivity = createdAt
		}

		result = append(result, tmuxSessionState{
			ID:           fields[0],
			CreatedAt:    createdAt,
			LastActivity: lastActivity,
		})
	}

	return result
}

func currentProcessUsername() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}
