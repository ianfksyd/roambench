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
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ianf339/roambench/internal/localruntime"
)

var version = "dev"

type options struct {
	server         string
	token          string
	runtimeID      string
	agentLabel     string
	executor       string
	execArgs       []string
	model          string
	permissionMode string
	poll           time.Duration
	maxRuntime     time.Duration
	maxConcurrent  int
}

func main() {
	fmt.Println("roambench-worker " + version)
	o := options{}
	flag.StringVar(&o.server, "server", env("ROAMBENCH_URL", "http://127.0.0.1:3000"), "RoamBench base URL")
	flag.StringVar(&o.token, "token", env("ROAMBENCH_TOKEN", env("ROAMBENCH_AGENT_TOKEN", "")), "Agent bearer token")
	flag.StringVar(&o.runtimeID, "runtime-id", "worker-"+hostname(), "Runtime id used for claims")
	flag.StringVar(&o.agentLabel, "agent-label", "claude", "Agent label reported with claims")
	flag.StringVar(&o.executor, "executor", env("ROAMBENCH_WORKER_EXECUTOR", "claude"), "Executor CLI that runs claimed tasks")
	flag.StringVar(&o.permissionMode, "permission-mode", env("ROAMBENCH_WORKER_PERMISSION_MODE", localruntime.DefaultPermissionMode), "Executor permission mode (e.g. acceptEdits; bypass flags are the operator's explicit choice)")
	flag.StringVar(&o.model, "model", env("ROAMBENCH_WORKER_MODEL", ""), "Optional model override passed as --model")
	poll := flag.Duration("poll", 15*time.Second, "Poll interval between claims")
	maxRuntime := flag.Duration("max-runtime", 2*time.Hour, "Per-task runtime bound (0 = unlimited)")
	maxConcurrent := flag.Int("max-concurrent", envInt("ROAMBENCH_WORKER_MAX_CONCURRENT", 1), "Max tasks executed simultaneously (worker claims serially until this many are running)")
	var execArgsRaw string
	flag.StringVar(&execArgsRaw, "executor-args", env("ROAMBENCH_WORKER_EXECUTOR_ARGS", ""), "Extra executor flags, space separated (e.g. permission flags — operator's choice)")
	flag.Parse()

	o.executor = strings.TrimSpace(o.executor)
	o.execArgs = splitArgs(execArgsRaw)
	o.poll = *poll
	o.maxRuntime = *maxRuntime
	o.maxConcurrent = *maxConcurrent
	if o.maxConcurrent < 1 {
		o.maxConcurrent = 1
	}
	if o.token == "" {
		fatal("no agent token: set ROAMBENCH_TOKEN or pass -token")
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("worker %s on %s (executor: %s, poll: %s, concurrent: %d)\n", o.runtimeID, o.server, o.executor, o.poll, o.maxConcurrent)
	slots := make(chan struct{}, o.maxConcurrent)
	var wg sync.WaitGroup
	shutdown := false
	for !shutdown {
		select {
		case <-stop:
			shutdown = true
			continue
		default:
		}
		// Full capacity: wait for a slot or shutdown instead of hot-polling.
		if len(slots) == cap(slots) {
			sleepUntil(o.poll, stop)
			continue
		}
		claimed := tryClaim(o)
		if claimed == nil {
			sleepUntil(o.poll, stop)
			continue
		}
		slots <- struct{}{}
		wg.Add(1)
		go func(task *claimedTask) {
			defer wg.Done()
			defer func() { <-slots }()
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("runTask panic: %v\n", r)
				}
			}()
			runTask(o, task)
		}(claimed)
	}
	wg.Wait()
	fmt.Println("all executions settled")
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return fallback
	}
	return v
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

// runTask executes one claimed task through the local runtime and reports
// back. The executor runs headless with structured output; readable lines
// are mirrored into the task terminal so the web UI shows the same run.
func runTask(o options, task *claimedTask) {
	fmt.Printf("claimed %s (%s)\n", task.TaskID, firstLine(task.Title))

	lostLease := make(chan struct{})
	// Heartbeat loop keeps the lease alive while the executor works; losing
	// the lease (expired/revoked) closes this execution's write access.
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-lostLease:
				return
			case <-ticker.C:
				out := cli("heartbeat", "--lease-id", task.LeaseID)
				if strings.Contains(out, "404") || strings.Contains(out, "not found") {
					fmt.Println("lease lost; stopping executor")
					close(lostLease)
					return
				}
			}
		}
	}()

	trace("creating terminal")
	terminalID := createTaskTerminal(o, task)
	trace(fmt.Sprintf("terminal created: %q", terminalID))
	if terminalID == "" {
		fmt.Println("no terminal session available; refusing to run headless (worker requires tmux)")
		_ = cli("release", "--lease-id", task.LeaseID, "--outcome", "failed")
		close(lostLease)
		return
	}

	// Progress reporting: executor events go to the server's task progress
	// feed (best-effort, batched, never blocks the executor reader).
	progress := newProgressReporter(task.TaskID, task.LeaseID)

	// Observation window: model output is written to a 0600 log file and the
	// terminal runs a FIXED tail command on it (composed by the worker, no
	// model text passes through send-keys). This removes the injection path
	// where displayed text was executed as commands in the pane shell.
	logPath := filepath.Join(task.Workspace, ".roambench-execution.log")
	_ = os.WriteFile(logPath, []byte(""), 0600)
	// -l takes ONLY the literal text; Enter must be a separate send-keys
	// call, otherwise "Enter" is typed as literal text and the tail command
	// never starts.
	tmuxLiteral(terminalID, "tail -n 40 -f "+shellQuote(logPath))
	tmuxEnter(terminalID)
	time.Sleep(300 * time.Millisecond)
	observer := localruntime.ObserverFunc(func(line string) {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(line + "\n")
	})

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	// Losing the lease stops the executor.
	go func() {
		select {
		case <-lostLease:
			cancelRun()
		case <-runCtx.Done():
		}
	}()

	if o.maxRuntime > 0 {
		runCtx, cancelRun = context.WithTimeout(runCtx, o.maxRuntime)
	}
	_ = cancelRun

	fmt.Fprintf(os.Stderr, "[worker] terminal=%q executor=%s mode=%s\n", terminalID, o.executor, o.permissionMode)
	handle, err := localruntime.Start(runCtx, localruntime.StartRequest{
		Executor:       o.executor,
		Args:           o.execArgs,
		Model:          o.model,
		PermissionMode: o.permissionMode,
		Prompt:         buildPrompt(task),
		Workspace:      task.Workspace,
		Observer: localruntime.ObserverFunc(func(line string) {
			observer(line)
			progress.add(line)
		}),
	})
	if err != nil {
		fmt.Printf("executor start failed: %v\n", err)
		_ = cli("release", "--lease-id", task.LeaseID, "--outcome", "failed")
		close(lostLease)
		return
	}

	// Wait for executor exit, runtime bound, or lease loss. The executor
	// handle's own Done channel closes only AFTER its exit code and error
	// have been recorded, so the outcome below never races the wait.
	var timedOut bool
	execDone := make(chan struct{})
	go func() {
		handle.Wait(runCtx)
		close(execDone)
	}()
	select {
	case <-execDone:
	case <-lostLease:
		// Lease gone: kill the whole process group, wait for the exit to be
		// recorded, then report failure.
		handle.Stop()
		<-handle.Done()
		timedOut = true
	case <-runCtx.Done():
		// Runtime bound hit: kill the process group (children of the
		// executor would otherwise hold the pipe and stall the wait), then
		// record the real exit before reporting.
		timedOut = true
		handle.Stop()
		<-handle.Done()
	}

	outcome := "done"
	failed := timedOut || handle.Err != nil || handle.ExitCode != 0
	if failed {
		outcome = "failed"
	}
	summary := summarizeEvents(handle.Events())
	_ = cli("artifact", "--kind", "completion_check",
		"--outcome", map[bool]string{true: "fail", false: "pass"}[failed],
		"--label", "worker execution",
		"--task", task.TaskID,
		"--lease-id", task.LeaseID,
		"--file", writeFileTemp(summary))
	rel := cli("release", "--lease-id", task.LeaseID, "--outcome", outcome)
	if strings.Contains(rel, "error:") {
		fmt.Println("release failed:", rel)
	} else {
		fmt.Printf("released %s (%s)\n", task.TaskID, outcome)
	}
}

// progressReporter batches executor events and reports them to the task
// progress feed. Reports are best-effort: failures are counted, never
// propagated, and the reporter never blocks the executor reader.
type progressReporter struct {
	taskID  string
	leaseID string
	seq     int
	mu      sync.Mutex
	flush   func(seq int, kind, text, leaseID string)
}

func newProgressReporter(taskID, leaseID string) *progressReporter {
	p := &progressReporter{taskID: taskID, leaseID: leaseID}
	p.flush = func(seq int, kind, text, leaseID string) {
		_ = cli("progress", "--task", p.taskID, "--seq", fmt.Sprint(seq),
			"--kind", kind, "--text", text, "--lease-id", leaseID)
	}
	return p
}

// add records one readable line, flushing on blank lines (paragraph ends)
// so reports arrive as complete thoughts.
func (p *progressReporter) add(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	p.mu.Lock()
	p.seq++
	seq := p.seq
	text := line
	p.mu.Unlock()
	go p.flush(seq, "assistant", text, p.leaseID)
}

// summarizeEvents renders the last assistant/result text as the artifact value.
func summarizeEvents(events []localruntime.Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if (e.Kind == "assistant" || e.Kind == "result") && strings.TrimSpace(e.Text) != "" {
			return e.Text
		}
	}
	return "executor produced no structured output"
}

// writeFileTemp writes text to a temp file and returns its path, for
// roambench-agent artifact --file.
func writeFileTemp(text string) string {
	f, err := os.CreateTemp("", "roambench-artifact-*.txt")
	if err != nil {
		return ""
	}
	defer f.Close()
	_, _ = f.WriteString(text)
	return f.Name()
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

// tmuxLiteral types literal text into a tmux session.
func tmuxLiteral(terminalID, text string) {
	_ = exec.Command("tmux", "send-keys", "-t", terminalID, "-l", text).Run()
}

// tmuxEnter presses Enter in a tmux session.
func tmuxEnter(terminalID string) {
	_ = exec.Command("tmux", "send-keys", "-t", terminalID, "Enter").Run()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
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
