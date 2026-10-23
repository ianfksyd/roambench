package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ianf339/roambench/internal/agentcfg"
	"github.com/ianf339/roambench/internal/agentmem"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	switch cmd {
	case "status":
		cmdStatus()
	case "artifact":
		cmdArtifact()
	case "checkpoint":
		cmdCheckpoint()
	case "notify":
		cmdNotify()
	case "claim":
		cmdClaim(os.Args[2:])
	case "heartbeat":
		cmdHeartbeat(os.Args[2:])
	case "release":
		cmdRelease(os.Args[2:])
	case "memory":
		cmdMemory(os.Args[2:])
	case "config":
		cmdConfig(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("roambench-agent " + version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(stderrTarget, `Usage: roambench-agent <command> [flags]

Commands:
  status                          Show current task and phase
  claim --runtime-id <id> --agent-label <l>
                                  Claim the next dispatch-ready task (204 = no task)
  heartbeat --lease-id <id> [--interval <d>]
                                  Renew a claim lease once, or loop every <interval>
  release --lease-id <id> --outcome done|failed|released
                                  Close a claim lease and report the outcome
  memory get [--project <id>] [--task <id>]
                                  Print the canonical project memory
  memory append --project <id> --text <text> [--file <f>]
                                  Append to the canonical project memory
  memory set --project <id> --text <text> [--file <f>]
                                  Replace the canonical project memory
  memory pointer --project <id> --file <path> [--file <path> ...]
                                  Write the shared-memory pointer into files
  config show --agent <label>
                                  Show where an agent currently points
  config apply --agent <label> --base-url <url> [--auth-token <t>]
                   [--model <id>] [--name <n>] [--codex-path <p>]
                                  Point an agent at a gateway (keeps a backup)
  artifact --kind <k> --outcome <o> [--label <l>] [--value <v>] [--file <f>]
                                  Submit an artifact for the current phase
  checkpoint --reason <r>         Request human review
  notify --title <t> [--body <b>] Send a terminal notification (OSC 777)
  version                         Print version

Claim flags (all accept --json for machine-readable output):
  claim     --server <url> --token <t> --runtime-id <id> --agent-label <l> [--json]
  heartbeat --server <url> --token <t> --lease-id <id> [--interval 60s] [--json]
  release   --server <url> --token <t> --lease-id <id> --outcome <o> [--json]

Environment:
  ROAMBENCH_URL    Base URL (e.g. http://localhost:3000); --server overrides
  ROAMBENCH_TOKEN  Agent bearer token; --token overrides and ROAMBENCH_AGENT_TOKEN works too`)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func baseURL() string {
	u := env("ROAMBENCH_URL", "")
	if u == "" {
		fatal("ROAMBENCH_URL is not set")
	}
	return strings.TrimRight(u, "/")
}

func token() string {
	t := env("ROAMBENCH_TOKEN", "")
	if t == "" {
		fatal("ROAMBENCH_TOKEN is not set")
	}
	return t
}

func fatal(msg string) {
	fatalCode(1, msg)
}

func flag(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, name+"=") {
			return strings.TrimPrefix(a, name+"=")
		}
	}
	return ""
}

// flagsAll returns every value given for a repeated flag.
func flagsAll(args []string, name string) []string {
	var values []string
	for i, a := range args {
		if a == name && i+1 < len(args) {
			values = append(values, args[i+1])
			continue
		}
		if strings.HasPrefix(a, name+"=") {
			values = append(values, strings.TrimPrefix(a, name+"="))
		}
	}
	return values
}

func apiGet(path string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", baseURL()+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token())
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func apiPost(path string, payload interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", baseURL()+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	return result, nil
}

func cmdStatus() {
	result, err := apiGet("/api/agent/v1/task")
	if err != nil {
		fatal(err.Error())
	}
	taskID, _ := result["taskId"].(string)
	if taskID == "" {
		fmt.Println("No active task.")
		return
	}
	title, _ := result["title"].(string)
	phase, _ := result["currentPhase"].(string)
	state, _ := result["state"].(string)
	nextStep, _ := result["nextStep"].(string)
	fmt.Printf("Task:    %s\n", title)
	fmt.Printf("ID:      %s\n", taskID)
	fmt.Printf("State:   %s\n", state)
	fmt.Printf("Phase:   %s\n", phase)
	if nextStep != "" {
		fmt.Printf("Next:    %s\n", nextStep)
	}
}

// cmdConfig shows and applies the model access point of a coding agent.
//
// Applying is deterministic: the CLI edits the agent configuration file
// itself and keeps a backup, so no language model touches user configuration.
func cmdConfig(args []string) {
	action := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		action = args[0]
		args = args[1:]
	}
	if action == "" {
		action = "show"
	}
	label := flag(args, "--agent")
	if label == "" {
		fatal("--agent is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fatal(err.Error())
	}

	switch action {
	case "show":
		path, ok := agentcfg.ConfigPath(label, home)
		if !ok {
			fatal("unsupported agent: " + label)
		}
		fmt.Printf("%s: %s\n", label, path)
		current := agentcfg.Current(label, home)
		if len(current) == 0 {
			fmt.Println("(no configuration yet)")
			return
		}
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Printf("  %s = %s\n", key, current[key])
		}
	case "apply":
		profile := agentcfg.Profile{
			Name:      flag(args, "--name"),
			BaseURL:   flag(args, "--base-url"),
			AuthToken: flag(args, "--auth-token"),
			Model:     flag(args, "--model"),
			CodexPath: flag(args, "--codex-path"),
		}
		if profile.BaseURL == "" {
			fatal("--base-url is required")
		}
		result, err := agentcfg.Apply(label, home, profile)
		if err != nil {
			fatal(err.Error())
		}
		if !result.Changed {
			fmt.Printf("%s already points at this profile\n", label)
			return
		}
		fmt.Printf("%s updated: %s\n", label, result.Plan.Path)
		if result.BackupPath != "" {
			fmt.Printf("backup: %s\n", result.BackupPath)
		}
		for _, change := range result.Plan.Changes {
			if change.Action == "none" {
				continue
			}
			before := change.Before
			if before == "" {
				before = "(unset)"
			}
			fmt.Printf("  %s: %s -> %s\n", change.Key, before, change.After)
		}
	default:
		fatal("unknown config action: " + action)
	}
}

// cmdMemory reads and writes the canonical project memory: the shared
// document every agent of a project uses instead of keeping a private copy.
func cmdMemory(args []string) {
	action := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		action = args[0]
		args = args[1:]
	}
	if action == "" {
		action = "get"
	}
	projectID := flag(args, "--project")
	taskID := flag(args, "--task")
	text := flag(args, "--text")
	file := flag(args, "--file")
	actor := flag(args, "--actor")

	// The pointer action writes files; --file there names the target, not
	// the body of the memory text.
	if action != "pointer" && file != "" && text == "" {
		data, err := os.ReadFile(file)
		if err != nil {
			fatal(err.Error())
		}
		text = string(data)
	}

	switch action {
	case "get":
		query := ""
		if projectID != "" {
			query = "?project=" + url.QueryEscape(projectID)
		} else if taskID != "" {
			query = "?task=" + url.QueryEscape(taskID)
		}
		result, err := apiGet("/api/agent/v1/memory" + query)
		if err != nil {
			fatal(err.Error())
		}
		content, _ := result["content"].(string)
		if strings.TrimSpace(content) == "" {
			fmt.Println("(project memory is empty)")
			return
		}
		fmt.Print(content)
		if !strings.HasSuffix(content, "\n") {
			fmt.Println()
		}
	case "pointer":
		if projectID == "" {
			fatal("--project is required")
		}
		files := flagsAll(args, "--file")
		if len(files) == 0 {
			fatal("--file is required")
		}
		for _, path := range files {
			result, err := agentmem.ApplyPointer(path, projectID)
			if err != nil {
				fatal(err.Error())
			}
			switch {
			case result.Created:
				fmt.Printf("wrote pointer: %s\n", result.Path)
			case result.Changed:
				fmt.Printf("updated pointer: %s (backup: %s)\n", result.Path, result.BackupPath)
			default:
				fmt.Printf("pointer already current: %s\n", result.Path)
			}
		}
	case "append", "set":
		if projectID == "" {
			fatal("--project is required")
		}
		if strings.TrimSpace(text) == "" {
			fatal("--text or --file is required")
		}
		mode := "append"
		if action == "set" {
			mode = "replace"
		}
		payload := map[string]interface{}{
			"projectId": projectID,
			"content":   strings.TrimRight(text, "\n"),
			"mode":      mode,
		}
		if actor != "" {
			payload["actor"] = actor
		}
		result, err := apiPost("/api/agent/v1/memory", payload)
		if err != nil {
			fatal(err.Error())
		}
		revision := 0
		switch v := result["revision"].(type) {
		case float64:
			revision = int(v)
		case int:
			revision = v
		}
		fmt.Printf("project memory updated (revision %d)\n", revision)
	default:
		fatal("unknown memory action: " + action)
	}
}

func cmdArtifact() {
	args := os.Args[2:]
	kind := flag(args, "--kind")
	outcome := flag(args, "--outcome")
	label := flag(args, "--label")
	value := flag(args, "--value")
	file := flag(args, "--file")
	taskID := flag(args, "--task")
	phaseID := flag(args, "--phase")

	if kind == "" {
		fatal("--kind is required")
	}
	if outcome == "" {
		outcome = "recorded"
	}
	if file != "" && value == "" {
		data, err := os.ReadFile(file)
		if err != nil {
			fatal("read file: " + err.Error())
		}
		value = string(data)
	}
	if label == "" {
		label = kind
	}

	// If no task specified, get the active one
	if taskID == "" {
		result, err := apiGet("/api/agent/v1/task")
		if err != nil {
			fatal(err.Error())
		}
		taskID, _ = result["taskId"].(string)
		if taskID == "" {
			fatal("no active task")
		}
		if phaseID == "" {
			phaseID, _ = result["currentPhase"].(string)
		}
	}

	payload := map[string]string{
		"taskId":       taskID,
		"phaseId":      phaseID,
		"artifactKind": kind,
		"outcome":      outcome,
		"label":        label,
		"value":        value,
	}
	_, err := apiPost("/api/agent/v1/artifact", payload)
	if err != nil {
		fatal(err.Error())
	}
	fmt.Printf("Artifact submitted: %s (%s)\n", kind, outcome)
}

func cmdCheckpoint() {
	args := os.Args[2:]
	reason := flag(args, "--reason")
	taskID := flag(args, "--task")

	if reason == "" {
		fatal("--reason is required")
	}
	if taskID == "" {
		result, err := apiGet("/api/agent/v1/task")
		if err != nil {
			fatal(err.Error())
		}
		taskID, _ = result["taskId"].(string)
		if taskID == "" {
			fatal("no active task")
		}
	}

	payload := map[string]string{
		"taskId": taskID,
		"reason": reason,
	}
	_, err := apiPost("/api/agent/v1/checkpoint", payload)
	if err != nil {
		fatal(err.Error())
	}
	fmt.Println("Checkpoint requested.")
}

func cmdNotify() {
	args := os.Args[2:]
	title := flag(args, "--title")
	body := flag(args, "--body")

	if title == "" {
		fatal("--title is required")
	}
	// Output OSC 777 sequence directly to terminal
	if body != "" {
		fmt.Printf("\033]777;notify;%s;%s\007", title, body)
	} else {
		fmt.Printf("\033]777;notify;%s;\007", title)
	}
}

// ---- Claim / heartbeat / release (K5, WS-20261015-001 task card C6) ----

// claimOverrides carries the per-invocation flags that differ from the
// legacy status/artifact commands: --server and --token override the
// environment, and --json switches to machine-readable output.
type claimOverrides struct {
	server string
	token  string
	json   bool
}

func parseOverrides(args []string) claimOverrides {
	return claimOverrides{
		server: flag(args, "--server"),
		token:  flag(args, "--token"),
		json:   hasFlag(args, "--json"),
	}
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name || strings.HasPrefix(a, name+"=") {
			return true
		}
	}
	return false
}

// claimBaseURL resolves the server base URL for claim commands.
func claimBaseURL(o claimOverrides) string {
	u := o.server
	if u == "" {
		u = env("ROAMBENCH_URL", "")
	}
	if u == "" {
		fatal("server URL is required: pass --server or set ROAMBENCH_URL")
	}
	return strings.TrimRight(u, "/")
}

// claimToken resolves the agent bearer token for claim commands.
// ROAMBENCH_AGENT_TOKEN is accepted as a fallback so a Hermes-side
// environment variable does not have to be renamed.
func claimToken(o claimOverrides) string {
	t := o.token
	if t == "" {
		t = env("ROAMBENCH_TOKEN", "")
	}
	if t == "" {
		t = env("ROAMBENCH_AGENT_TOKEN", "")
	}
	if t == "" {
		fatal("agent token is required: pass --token or set ROAMBENCH_TOKEN / ROAMBENCH_AGENT_TOKEN")
	}
	return t
}

// claimAPIPost issues a POST against the claim API with retries on
// transport errors. Status codes are returned to the caller for handling:
// 200/201 = ok, 204 = no task, 401 = bad token, 404 = lease gone,
// 5xx = server-side problem. Only network/transport failures are retried;
// HTTP error statuses are answered once by the server and never retried.
func claimAPIPost(o claimOverrides, path string, payload interface{}) (int, []byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	const attempts = 3
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			// Exponential backoff: 1s, 2s before the 2nd/3rd try.
			time.Sleep(time.Duration(1<<(i-1)) * time.Second)
		}
		req, err := http.NewRequest(http.MethodPost, claimBaseURL(o)+path, bytes.NewReader(data))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+claimToken(o))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp.StatusCode, body, nil
	}
	return 0, nil, lastErr
}

// printClaimResult prints a claim response either as JSON or human-readable.
func printClaimResult(asJSON bool, body []byte) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		fatal("invalid JSON response from server: " + err.Error())
	}
	if asJSON {
		out, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(out))
		return
	}
	fmt.Printf("Task:    %s\n", strOr(payload["title"], payload["taskId"]))
	fmt.Printf("ID:      %s\n", payload["taskId"])
	fmt.Printf("Lease:   %s (expires %s, ttl %vs)\n", payload["leaseId"], payload["leaseExpiresAt"], payload["leaseTtlSeconds"])
	if v, _ := payload["state"].(string); v != "" {
		fmt.Printf("State:   %s\n", v)
	}
	if v, _ := payload["currentPhase"].(string); v != "" {
		fmt.Printf("Phase:   %s\n", v)
	}
	if v, _ := payload["nextStep"].(string); v != "" {
		fmt.Printf("Next:    %s\n", v)
	}
	if missing, ok := payload["missingEvidence"].([]interface{}); ok && len(missing) > 0 {
		fmt.Printf("Missing: %v\n", missing)
	}
}

func strOr(values ...interface{}) string {
	for _, v := range values {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return ""
}

// exitCodeFor maps non-retryable HTTP error statuses onto exit codes so a
// calling script can branch (401/404 are distinct failure modes).
const (
	exitUnauthorized = 3 // 401: token invalid
	exitLeaseGone    = 4 // 404: lease expired, released or unknown
)

func fatalCode(exitCode int, msg string) {
	fmt.Fprintln(stderrTarget, "error: "+msg)
	osExit(exitCode)
}

// Indirection points so tests can capture exit codes and stderr without
// ending the test process.
var osExit = os.Exit

var stderrTarget io.Writer = os.Stderr

// cmdClaim claims the next dispatch-ready task.
func cmdClaim(args []string) {
	o := parseOverrides(args)
	runtimeID := flag(args, "--runtime-id")
	agentLabel := flag(args, "--agent-label")
	if runtimeID == "" {
		fatal("--runtime-id is required")
	}
	if agentLabel == "" {
		fatal("--agent-label is required")
	}

	status, body, err := claimAPIPost(o, "/api/agent/v1/claim", map[string]string{
		"runtimeId":  runtimeID,
		"agentLabel": agentLabel,
	})
	if err != nil {
		fatal("network error after 3 attempts: " + err.Error())
	}
	switch status {
	case http.StatusOK:
		printClaimResult(o.json, body)
	case http.StatusNoContent:
		// Silent in both modes except a stable machine-readable marker.
		if o.json {
			fmt.Println(`{"claimed":false}`)
		} else {
			fmt.Println("no-task")
		}
		return
	case http.StatusUnauthorized:
		fatalCode(exitUnauthorized, "agent token rejected (HTTP 401): the token is invalid or was regenerated; get a fresh one via POST /api/project-control/agent-token and update ROAMBENCH_AGENT_TOKEN")
	default:
		fatal(fmt.Sprintf("HTTP %d: %s", status, strings.TrimSpace(string(body))))
	}
}

// cmdHeartbeat renews a lease once, or loops every --interval until the
// process is interrupted.
func cmdHeartbeat(args []string) {
	o := parseOverrides(args)
	leaseID := flag(args, "--lease-id")
	interval := flag(args, "--interval")
	if leaseID == "" {
		fatal("--lease-id is required")
	}

	once := func() (int, []byte, error) {
		return claimAPIPost(o, "/api/agent/v1/heartbeat", map[string]string{"leaseId": leaseID})
	}
	printHeartbeat := func(status int, body []byte) {
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			fatal("invalid JSON response from server: " + err.Error())
		}
		if o.json {
			out, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(out))
			return
		}
		fmt.Printf("Lease %s renewed until %s\n", payload["leaseId"], payload["leaseExpiresAt"])
	}

	if interval == "" {
		status, body, err := once()
		if err != nil {
			fatal("network error after 3 attempts: " + err.Error())
		}
		handleHeartbeatStatus(status, body, printHeartbeat)
		return
	}

	d, err := time.ParseDuration(interval)
	if err != nil || d <= 0 {
		fatal("--interval must be a positive duration (e.g. 60s)")
	}
	// Loop mode: heartbeat immediately, then every interval. The lease TTL
	// is 120s server-side, so the interval must stay well below that.
	fmt.Fprintf(os.Stderr, "heartbeat loop started (interval %s, lease ttl 120s); stop with Ctrl+C\n", d)
	for {
		status, body, hbErr := once()
		if hbErr != nil {
			fmt.Fprintf(os.Stderr, "warning: network error after 3 attempts: %v\n", hbErr)
		} else {
			handleHeartbeatStatus(status, body, printHeartbeat)
		}
		time.Sleep(d)
	}
}

// handleHeartbeatStatus interprets one heartbeat response. 200 prints the
// renewal; 401/404 exit with a distinct message and exit code (in loop mode
// the lease is unrecoverable: expired leases cannot be renewed).
func handleHeartbeatStatus(status int, body []byte, printOK func(int, []byte)) {
	switch status {
	case http.StatusOK:
		printOK(status, body)
	case http.StatusUnauthorized:
		fatalCode(exitUnauthorized, "agent token rejected (HTTP 401): the token is invalid or was regenerated")
	case http.StatusNotFound:
		fatalCode(exitLeaseGone, "lease expired or no longer active (HTTP 404): the lease is gone; re-claim the task with `roambench-agent claim` and continue with the new leaseId")
	default:
		fatal(fmt.Sprintf("HTTP %d: %s", status, strings.TrimSpace(string(body))))
	}
}

// cmdRelease closes a lease with an explicit outcome.
func cmdRelease(args []string) {
	o := parseOverrides(args)
	leaseID := flag(args, "--lease-id")
	outcome := flag(args, "--outcome")
	if leaseID == "" {
		fatal("--lease-id is required")
	}
	switch outcome {
	case "done", "failed", "released":
	default:
		fatal("--outcome must be one of done|failed|released")
	}

	status, body, err := claimAPIPost(o, "/api/agent/v1/release", map[string]string{
		"leaseId": leaseID,
		"outcome": outcome,
	})
	if err != nil {
		fatal("network error after 3 attempts: " + err.Error())
	}
	switch status {
	case http.StatusOK:
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			fatal("invalid JSON response from server: " + err.Error())
		}
		if o.json {
			out, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(out))
			return
		}
		fmt.Printf("Released: task %s outcome %s (dispatchState -> %s)\n",
			payload["taskId"], payload["outcome"], payload["dispatchState"])
	case http.StatusUnauthorized:
		fatalCode(exitUnauthorized, "agent token rejected (HTTP 401): the token is invalid or was regenerated")
	case http.StatusNotFound:
		fatalCode(exitLeaseGone, "lease expired or no longer active (HTTP 404): nothing to release; re-claim with `roambench-agent claim` if the work is still needed")
	default:
		fatal(fmt.Sprintf("HTTP %d: %s", status, strings.TrimSpace(string(body))))
	}
}
