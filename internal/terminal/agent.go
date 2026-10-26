package terminal

import (
	"fmt"
	"os/exec"
	"strings"
)

// Agent CLI process observation: the server-side fact that an interactive
// agent session (Claude, Codex, …) is running inside a terminal pane. The
// handoff flow treats "paste delivered into a pane where an agent CLI is
// running" as a validated submission boundary — the agent consumes every
// line it is sent, so the paste is its input by construction.
//
// Detection walks the pane's process tree and matches the first command
// whose name (basename of argv[0]) is a known agent CLI AND that owns the
// pane's foreground process group (ps stat "+"). A backgrounded
// lookalike — "(exec -a codex sleep 60) &" under a shell prompt — is not
// receiving the terminal's input and is never reported. npm-style
// wrappers that re-exec through node are not detected; that limitation is
// accepted rather than guessed around.

// knownAgentCLIs are matched case-insensitively against argv[0] basenames.
var knownAgentCLIs = map[string]bool{
	"claude":       true,
	"codex":        true,
	"opencode":     true,
	"gemini":       true,
	"aider":        true,
	"hermes":       true,
	"kiro-cli":     true,
	"cursor-agent": true,
}

// AgentProcess identifies one agent CLI instance in a pane.
type AgentProcess struct {
	Name string // e.g. "codex"
	PID  string
	// Start is the process start time reported by ps; together with the
	// PID it forms a generation signature that survives neither process
	// exit nor PID reuse.
	Start string
}

// Signature is the stable-per-process-generation identity used to tell
// successive agent runs in the same terminal apart.
func (p AgentProcess) Signature() string {
	return p.PID + ":" + p.Start
}

// FormatAgentSig is a helper for logs and records.
func FormatAgentSig(p AgentProcess) string {
	return fmt.Sprintf("%s@%s", p.Name, p.Signature())
}

// AgentProcessInSession returns the first known agent CLI found among the
// session pane's descendant processes, if any. Ownership is the caller's
// check (the server resolves the session through ListSessions first).
func (m *Manager) AgentProcessInSession(sessionID string) (AgentProcess, bool) {
	if m == nil || strings.TrimSpace(sessionID) == "" {
		return AgentProcess{}, false
	}
	out, err := exec.Command("tmux", "display-message", "-p", "-t", sessionID, "#{pane_pid}").Output()
	if err != nil {
		return AgentProcess{}, false
	}
	panePID := strings.TrimSpace(string(out))
	if panePID == "" {
		return AgentProcess{}, false
	}
	return agentProcessUnder(panePID)
}

// agentProcessUnder walks the descendants of root breadth-first and
// reports the first known agent CLI (closer to the pane shell wins).
func agentProcessUnder(root string) (AgentProcess, bool) {
	out, err := exec.Command("ps", "-eo", "pid=,ppid=,stat=,lstart=,args=").Output()
	if err != nil {
		return AgentProcess{}, false
	}
	type proc struct {
		pid, ppid, stat, lstart, args string
	}
	procs := make(map[string]proc)
	children := make(map[string][]string)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		// pid ppid stat lstart(5 tokens: wday mon day hh:mm:ss year) args…
		if len(fields) < 9 {
			continue
		}
		p := proc{
			pid:    fields[0],
			ppid:   fields[1],
			stat:   fields[2],
			lstart: strings.Join(fields[3:8], " "),
			args:   strings.Join(fields[8:], " "),
		}
		procs[p.pid] = p
		children[p.ppid] = append(children[p.ppid], p.pid)
	}
	if _, ok := procs[root]; !ok {
		return AgentProcess{}, false
	}
	// The agent may be the pane process itself (exec replaces the shell).
	queue := []string{root}
	visited := map[string]bool{}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if visited[pid] {
			continue
		}
		visited[pid] = true
		p, ok := procs[pid]
		if !ok {
			continue
		}
		// Foreground only: a background job is not reading the terminal.
		if !strings.Contains(p.stat, "+") {
			continue
		}
		if name := agentNameOfArgs(p.args); name != "" {
			return AgentProcess{Name: name, PID: p.pid, Start: p.lstart}, true
		}
		queue = append(queue, children[pid]...)
	}
	return AgentProcess{}, false
}

// agentNameOfArgs extracts argv[0]'s basename and matches it against the
// known agent CLI set.
func agentNameOfArgs(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return ""
	}
	argv0 := strings.Fields(args)[0]
	base := argv0
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(base)
	if knownAgentCLIs[base] {
		return base
	}
	return ""
}
