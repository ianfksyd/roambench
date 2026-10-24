// roambench-worker is the local task runtime: it claims dispatchable tasks
// from the board, runs each one in a visible terminal session (created
// through the server, so the web UI can attach to it), and reports the
// outcome back.
//
// The executor — which coding agent CLI runs the task — comes from the
// configuration ([agent.worker] executor, default claude) or the --executor
// flag. Executor flags are NOT preset: running an agent without its
// permission prompts is the operator's explicit choice, configured through
// executor_args. The loop is intentionally simple: claim, run, heartbeat,
// release.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var version = "dev"

type options struct {
	server     string
	token      string
	runtimeID  string
	agentLabel string
	executor   string
	execArgs   []string
	model      string
	poll       time.Duration
	maxRuntime time.Duration
}

func main() {
	fmt.Println("roambench-worker " + version)
	o := options{}
	flag.StringVar(&o.server, "server", env("ROAMBENCH_URL", "http://127.0.0.1:3000"), "RoamBench base URL")
	flag.StringVar(&o.token, "token", env("ROAMBENCH_TOKEN", env("ROAMBENCH_AGENT_TOKEN", "")), "Agent bearer token")
	flag.StringVar(&o.runtimeID, "runtime-id", "worker-"+hostname(), "Runtime id used for claims")
	flag.StringVar(&o.agentLabel, "agent-label", "claude", "Agent label reported with claims")
	flag.StringVar(&o.executor, "executor", env("ROAMBENCH_WORKER_EXECUTOR", "claude"), "Executor CLI that runs claimed tasks")
	flag.StringVar(&o.model, "model", env("ROAMBENCH_WORKER_MODEL", ""), "Optional model override passed as --model")
	poll := flag.Duration("poll", 15*time.Second, "Poll interval between claims")
	maxRuntime := flag.Duration("max-runtime", 2*time.Hour, "Per-task runtime bound (0 = unlimited)")
	var execArgsRaw string
	flag.StringVar(&execArgsRaw, "executor-args", env("ROAMBENCH_WORKER_EXECUTOR_ARGS", ""), "Extra executor flags, space separated (e.g. permission flags — operator's choice)")
	flag.Parse()

	o.executor = strings.TrimSpace(o.executor)
	o.execArgs = splitArgs(execArgsRaw)
	o.poll = *poll
	o.maxRuntime = *maxRuntime
	if o.token == "" {
		fatal("no agent token: set ROAMBENCH_TOKEN or pass -token")
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("worker %s on %s (executor: %s, poll: %s)\n", o.runtimeID, o.server, o.executor, o.poll)
	for {
		select {
		case <-stop:
			fmt.Println("shutting down")
			return
		default:
		}
		claimed := tryClaim(o)
		if claimed == nil {
			sleepUntil(o.poll, stop)
			continue
		}
		runTask(o, claimed)
	}
}

// claimedTask is the subset of the claim response the worker acts on.
type claimedTask struct {
	TaskID    string `json:"taskId"`
	LeaseID   string `json:"leaseId"`
	Goal      string `json:"goal"`
	Title     string `json:"title"`
	Workspace string `json:"workspace"`
	Phase     string `json:"currentPhase"`
}

func tryClaim(o options) *claimedTask {
	out := cliJSON("claim", "--runtime-id", o.runtimeID, "--agent-label", o.agentLabel)
	if out == "" {
		return nil // 204: nothing to do
	}
	var task claimedTask
	if err := json.Unmarshal([]byte(out), &task); err != nil {
		fmt.Printf("claim parse error: %v (%s)\n", err, truncate(out, 200))
		return nil
	}
	if task.TaskID == "" || task.LeaseID == "" {
		return nil
	}
	return &task
}

// runTask executes one claimed task in a visible terminal and reports back.
func runTask(o options, task *claimedTask) {
	fmt.Printf("claimed %s (%s)\n", task.TaskID, firstLine(task.Title))

	// Heartbeat loop keeps the lease alive while the executor works.
	hbStop := make(chan struct{})
	go func() {
		for {
			select {
			case <-hbStop:
				return
			case <-time.After(60 * time.Second):
				_ = cli("heartbeat", "--lease-id", task.LeaseID)
			}
		}
	}()
	defer close(hbStop)

	// The execution terminal is created through the server, so the web UI
	// can attach to the very same session and watch the executor live.
	trace("creating terminal")
	terminalID := createTaskTerminal(o, task)
	trace(fmt.Sprintf("terminal created: %q", terminalID))
	prompt := buildPrompt(task)
	args := append([]string{}, o.execArgs...)
	if o.model != "" {
		args = append(args, "--model", o.model)
	}
	args = append(args, prompt)

	fmt.Fprintf(os.Stderr, "[worker] terminal=%q executor=%s args=%d\n", terminalID, o.executor, len(args))
	trace(fmt.Sprintf("runTask enter terminal=%q", terminalID))
	if terminalID != "" {
		// Type the command into the task terminal so the user watches it live.
		sendKeys(terminalID, o.executor+" "+shellQuoteAll(args))
		trace("send-keys done, waiting for idle")
		fmt.Fprintln(os.Stderr, "[worker] send-keys done, waiting for idle")
	} else {
		fmt.Println("no terminal session available; refusing to run headless (worker requires tmux)")
		_ = cli("release", "--lease-id", task.LeaseID, "--outcome", "failed")
		return
	}

	// Wait for the executor to return to a shell, bounded by max-runtime.
	done := make(chan struct{})
	var timedOut bool
	go func() {
		select {
		case <-done:
		case <-time.After(orZero(o.maxRuntime)):
			timedOut = true
			sendKeys(terminalID, "C-c")
			select {
			case <-done:
			default:
				close(done)
			}
		}
	}()
	waitTerminalIdle(terminalID, done)
	if !timedOut {
		select {
		case <-done:
		default:
			close(done)
		}
	}

	trace("waitTerminalIdle returned; releasing")
	outcome := "done"
	if timedOut {
		outcome = "failed"
		fmt.Println("task timed out")
	}
	_ = cli("release", "--lease-id", task.LeaseID, "--outcome", outcome)
	fmt.Printf("released %s (%s)\n", task.TaskID, outcome)
}

// buildPrompt renders the executor prompt from the claim payload.
func buildPrompt(task *claimedTask) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(firstLine(task.Title)))
	if goal := strings.TrimSpace(task.Goal); goal != "" && goal != firstLine(task.Title) {
		b.WriteString("\n\n")
		b.WriteString(goal)
	}
	return b.String()
}

// createTaskTerminal asks the server for an execution terminal bound to the
// task; an empty result means the attach path is unavailable.
func createTaskTerminal(o options, task *claimedTask) string {
	payload, _ := json.Marshal(map[string]string{
		"taskId":  task.TaskID,
		"workDir": task.Workspace,
		"name":    "roambench-task",
	})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(o.server, "/")+"/api/agent/v1/terminal", bytes.NewReader(payload))
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+o.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		fmt.Printf("terminal create failed: %v\n", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		fmt.Printf("terminal create: HTTP %d\n", resp.StatusCode)
		return ""
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return ""
	}
	return session.ID
}

// waitTerminalIdle blocks until the terminal's running command returns to a
// shell prompt, polled through tmux. An unlimited runtime (0) waits forever.
func waitTerminalIdle(terminalID string, done chan struct{}) {
	if terminalID == "" {
		return
	}
	for {
		select {
		case <-done:
			return
		case <-time.After(5 * time.Second):
			cmd := exec.Command("tmux", "list-panes", "-t", terminalID, "-F", "#{pane_current_command}")
			out, err := cmd.Output()
			if err != nil {
				return // session gone
			}
			switch strings.TrimSpace(string(out)) {
			case "bash", "zsh", "sh", "fish":
				return
			}
		}
	}
}

// sendKeys types a command line into a tmux session and presses Enter.
func sendKeys(terminalID, command string) {
	_ = exec.Command("tmux", "send-keys", "-t", terminalID, command, "Enter").Run()
}

func cli(args ...string) string {
	cmd := exec.Command(roambenchAgentBin(), args...)
	cmd.Env = append(os.Environ(),
		"ROAMBENCH_URL="+o_server(),
		"ROAMBENCH_TOKEN="+o_token(),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("roambench-agent %s: %v (%s)\n", args[0], err, truncate(string(out), 200))
	}
	return strings.TrimSpace(string(out))
}

// cliJSON runs a subcommand with --json and returns stdout when it produced
// a JSON object (a 204 / "no-task" yields an empty string).
func cliJSON(args ...string) string {
	// --json goes after the subcommand: the CLI parses the first argument
	// as the command name.
	fixed := append([]string{args[0], "--json"}, args[1:]...)
	out := cli(fixed...)
	if out == "" || strings.Contains(out, "no-task") || strings.Contains(out, `"claimed":false`) || !strings.HasPrefix(out, "{") {
		return ""
	}
	return out
}

func roambenchAgentBin() string {
	if path := env("ROAMBENCH_AGENT_BIN", ""); path != "" {
		return path
	}
	return "roambench-agent"
}

// o_server / o_token read the ambient environment for the agent CLI; the
// worker already validated a token exists at startup.
func o_server() string { return env("ROAMBENCH_URL", "http://127.0.0.1:3000") }
func o_token() string  { return env("ROAMBENCH_TOKEN", env("ROAMBENCH_AGENT_TOKEN", "")) }

func sleepUntil(d time.Duration, stop chan os.Signal) {
	select {
	case <-stop:
	case <-time.After(d):
	}
}

func splitArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.Fields(raw)
}

func shellQuoteAll(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		quoted = append(quoted, "'"+strings.ReplaceAll(a, "'", "'\\''")+"'")
	}
	return strings.Join(quoted, " ")
}

func orZero(d time.Duration) time.Duration {
	if d <= 0 {
		return 365 * 24 * time.Hour
	}
	return d
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "local"
	}
	return strings.ReplaceAll(strings.ToLower(h), ".", "-")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func trace(msg string) {
	f, err := os.OpenFile("/tmp/rbw/worker-trace.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05.000"), msg)
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
