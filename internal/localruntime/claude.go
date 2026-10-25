// Package localruntime runs claimed tasks through a coding-agent executor.
//
// The Claude adapter drives `claude --print --output-format stream-json`:
// one JSON event per line on stdout, and process exit marks the end of the
// turn with a real exit code. That replaces the old "did the pane return to
// bash" heuristic, which could not tell completion from a crash.
//
// Readable output is mirrored into an observer sink (typically a tmux pane)
// so a person can watch the same execution the panel reports on; the
// structured events are kept separately for the task progress feed.
package localruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

// Event is one structured message produced by an execution.
type Event struct {
	// Kind is the event family: "assistant" (agent text), "tool_use",
	// "result" (final turn payload), "system", or "raw" (unparsed line).
	Kind string
	// Text is the human-readable content, when the event carries any.
	Text string
	// Raw is the original line for kinds the adapter does not model.
	Raw string
}

// Observer receives human-readable execution output. Implementations must be
// safe for concurrent use; the runner mirrors events from its own goroutine.
type Observer interface {
	WriteReadable(line string)
}

// ObserverFunc adapts a function to Observer.
type ObserverFunc func(line string)

// WriteReadable implements Observer.
func (f ObserverFunc) WriteReadable(line string) { f(line) }

// Handle is a started execution.
type Handle struct {
	// ExitCode is the executor's real exit code, valid after Wait returns.
	ExitCode int
	// Err is the wait error: nil on clean exit, exec.ExitError on non-zero,
	// or a start/wait failure.
	Err error

	cmd      *exec.Cmd
	cancel   context.CancelFunc
	events   []Event
	done     chan struct{}
	mu       sync.Mutex
	observer Observer
}

// Events returns the structured events collected so far (snapshot).
func (h *Handle) Events() []Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Event, len(h.events))
	copy(out, h.events)
	return out
}

func (h *Handle) addEvent(e Event) {
	h.mu.Lock()
	h.events = append(h.events, e)
	h.mu.Unlock()
	if h.observer != nil && strings.TrimSpace(e.Text) != "" {
		for _, line := range strings.Split(strings.TrimRight(e.Text, "\n"), "\n") {
			h.observer.WriteReadable(line)
		}
	}
}

// StartRequest describes one executor run.
type StartRequest struct {
	// Executor is the CLI binary, e.g. "claude".
	Executor string
	// Args are executor flags placed before the prompt.
	Args []string
	// Model is an optional model override (--model).
	Model string
	// PermissionMode maps to the executor's permission switch. Empty means
	// the executor default. The operator chooses this explicitly.
	PermissionMode string
	// Prompt is the task instruction. Passed as an argv element, never
	// through a shell, so task text cannot be interpreted as commands.
	Prompt string
	// Workspace is the working directory for the run.
	Workspace string
	// Observer receives readable output lines (may be nil).
	Observer Observer
}

// Start launches the executor in headless structured-output mode.
//
// The default adapter is Claude Code:
//
//	<executor> [-model m] [--permission-mode mode] --print
//	  --output-format stream-json --verbose -- <prompt>
//
// ctx bounds the whole run; cancelling it kills the process group.
func Start(ctx context.Context, req StartRequest) (*Handle, error) {
	executor := strings.TrimSpace(req.Executor)
	if executor == "" {
		executor = "claude"
	}
	if strings.TrimSpace(req.Workspace) == "" {
		return nil, fmt.Errorf("workspace is required")
	}
	args := make([]string, 0, len(req.Args)+8)
	args = append(args, req.Args...)
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.PermissionMode != "" {
		args = append(args, "--permission-mode", req.PermissionMode)
	}
	args = append(args,
		"--print", "--output-format", "stream-json", "--verbose",
		"--", req.Prompt,
	)

	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, executor, args...)
	cmd.Dir = req.Workspace
	// Own process group: Stop kills the whole tree, including child processes
	// spawned by the executor that would otherwise hold our stdout pipe open.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = nil // stderr is diagnostics only; events come from stdout
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start %s: %w", executor, err)
	}

	handle := &Handle{cmd: cmd, cancel: cancel, observer: req.Observer, done: make(chan struct{})}
	go func() {
		defer close(handle.done)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			handle.addEvent(parseClaudeLine(line))
		}
		waitErr := cmd.Wait()
		handle.mu.Lock()
		if cmd.ProcessState != nil {
			handle.ExitCode = cmd.ProcessState.ExitCode()
		}
		handle.Err = waitErr
		handle.mu.Unlock()
		if handle.observer != nil {
			handle.observer.WriteReadable(fmt.Sprintf("[exit %d]", handle.ExitCode))
		}
		cancel()
	}()
	return handle, nil
}

// Wait blocks until the executor process exits and fills ExitCode/Err, or
// until ctx is cancelled (the process keeps running; use Stop to kill it).
func (h *Handle) Wait(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-h.done:
	}
}

// Stop cancels the run context and kills the executor's whole process
// group, so children spawned by the executor cannot outlive the run.
func (h *Handle) Stop() {
	h.cancel()
	if h.cmd != nil && h.cmd.Process != nil {
		_ = syscall.Kill(-h.cmd.Process.Pid, syscall.SIGKILL)
	}
}

// parseClaudeLine maps one stream-json line to an Event.
func parseClaudeLine(line string) Event {
	var envelope struct {
		Type    string `json:"type"`
		Result  string `json:"result"`
		Error   string `json:"error"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
		Subtype string `json:"subtype"`
	}
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		return Event{Kind: "raw", Raw: line}
	}
	switch envelope.Type {
	case "assistant":
		var texts []string
		for _, block := range envelope.Message.Content {
			if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
				texts = append(texts, block.Text)
			}
		}
		return Event{Kind: "assistant", Text: strings.Join(texts, "\n\n"), Raw: line}
	case "user", "tool_use", "tool_result":
		return Event{Kind: "tool_use", Raw: line}
	case "result":
		text := envelope.Result
		if text == "" {
			text = envelope.Error
		}
		return Event{Kind: "result", Text: text, Raw: line}
	case "system":
		return Event{Kind: "system", Raw: line}
	default:
		return Event{Kind: "raw", Raw: line}
	}
}

// DefaultPermissionMode is used when the operator did not choose one. It
// allows file edits (the point of a coding task) while still denying
// anything that would prompt, so a headless run cannot hang or silently
// escalate. Full autonomy is an explicit operator setting.
const DefaultPermissionMode = "acceptEdits"

// Done closes after the executor exited AND its exit code and error have
// been recorded. Waiting on this (instead of process exit) guarantees the
// outcome fields are observable without a race.
func (h *Handle) Done() <-chan struct{} { return h.done }
