package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
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
	fmt.Fprintln(os.Stderr, `Usage: roambench-agent <command> [flags]

Commands:
  status                          Show current task and phase
  artifact --kind <k> --outcome <o> [--label <l>] [--value <v>] [--file <f>]
                                  Submit an artifact for the current phase
  checkpoint --reason <r>         Request human review
  notify --title <t> [--body <b>] Send a terminal notification (OSC 777)
  version                         Print version

Environment:
  ROAMBENCH_URL    Base URL (e.g. http://localhost:3000)
  ROAMBENCH_TOKEN  Agent bearer token`)
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
	fmt.Fprintln(os.Stderr, "error: "+msg)
	os.Exit(1)
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
