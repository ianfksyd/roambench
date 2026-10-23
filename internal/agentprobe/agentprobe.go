// Package agentprobe implements F2 agent detection (WS-20261015-001, K3).
//
// It probes a fixed set of CLI coding agents (kiro-cli, claude, codex,
// opencode, hermes) by resolving each binary via PATH and running a
// side-effect-free --version command with a hard timeout. Detection results
// are cached; ?refresh=1 forces a re-probe. Recommendations map probe
// outcomes to deterministic T1-T4 worker/reviewer assignments.
package agentprobe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// agentLabels is the fixed, ordered set of agents defined by the K3 contract.
// Order matters: it is the display order of the agents array.
var agentLabels = []string{"kiro-cli", "claude", "codex", "opencode", "hermes"}

// versionTimeout is the per-agent hard timeout for the --version command.
const versionTimeout = 5 * time.Second

// versionMaxLen caps the reported version string. Output beyond this limit is
// truncated so that a chatty or broken CLI cannot flood the response.
const versionMaxLen = 80

// binaryNames maps agentLabel to the executable name resolved via PATH.
// hermes 不在 PATH 是正常场景（installed=false），不是错误。
var binaryNames = map[string]string{
	"kiro-cli": "kiro-cli",
	"claude":   "claude",
	"codex":    "codex",
	"opencode": "opencode",
	"hermes":   "hermes",
}

// Agent is one probed agent (K3 contract fields).
type Agent struct {
	AgentLabel     string   `json:"agentLabel"`
	Binary         string   `json:"binary"`
	Path           string   `json:"path,omitempty"`
	Installed      bool     `json:"installed"`
	Version        string   `json:"version,omitempty"`
	NonInteractive bool     `json:"nonInteractive"`
	Models         []string `json:"models"`
}

// Recommendation is one tier assignment (K3 contract fields).
type Recommendation struct {
	Tier          string `json:"tier"`
	WorkerLabel   string `json:"workerLabel"`
	ReviewerLabel string `json:"reviewerLabel"`
	Reason        string `json:"reason"`
}

// Snapshot is the full probe result (K3 contract shape, minus HTTP envelope).
type Snapshot struct {
	DetectedAt      string           `json:"detectedAt"`
	Agents          []Agent          `json:"agents"`
	Recommendations []Recommendation `json:"recommendations"`
}

// Prober detects agents and caches the result until Refresh is called.
// It is safe for concurrent use.
type Prober struct {
	versionRunner func(ctx context.Context, path string) (string, error)
	lookPath      func(binary string) (string, error)

	mu   sync.Mutex
	snap *Snapshot
}

// NewProber returns a Prober using the real process executor.
func NewProber() *Prober {
	return &Prober{
		versionRunner: defaultVersionRunner,
		lookPath:      lookupAgentBinary,
	}
}

// Probe returns the cached snapshot, re-probing when refresh is true or no
// snapshot exists yet.
func (p *Prober) Probe(refresh bool) *Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.snap == nil || refresh {
		p.snap = p.probeAll()
	}
	return p.snap
}

func (p *Prober) probeAll() *Snapshot {
	agents := make([]Agent, 0, len(agentLabels))
	for _, label := range agentLabels {
		agents = append(agents, p.probeOne(label))
	}
	return &Snapshot{
		DetectedAt:      time.Now().UTC().Format(time.RFC3339),
		Agents:          agents,
		Recommendations: Recommendations(agents),
	}
}

func (p *Prober) probeOne(label string) Agent {
	binary := binaryNames[label]
	a := Agent{
		AgentLabel: label,
		Binary:     binary,
		Models:     []string{}, // GATE-1 #3: no model enumeration, always non-nil.
	}
	path, err := p.lookPath(binary)
	if err != nil {
		a.Installed = false
		return a
	}
	a.Path = path
	a.Installed = true
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()
	version, err := p.versionRunner(ctx, path)
	if err != nil {
		// Binary found but version command failed/timed out: still installed,
		// no version reported (probe is best-effort, never fails the endpoint).
		return a
	}
	a.Version = truncateVersion(version)
	return a
}

// lookupAgentBinary resolves an agent binary via PATH first, then through
// the common install locations of the agent ecosystems. Agent CLIs are often
// installed by their own managers (opencode, volta, nvm, openclaw-managed
// node) into directories a service process does not inherit in its PATH.
func lookupAgentBinary(binary string) (string, error) {
	if path, err := exec.LookPath(binary); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("%s not found", binary)
	}
	for _, dir := range agentCandidateDirs(home) {
		candidate := filepath.Join(dir, binary)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			// A candidate outside PATH still needs the exec bit to be useful.
			if info.Mode().Perm()&0111 != 0 {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("%s not found", binary)
}

// agentCandidateDirs lists the extra directories probed for agent binaries.
// Patterns are expanded once per call, so freshly installed toolchains are
// found after a refresh without a restart.
func agentCandidateDirs(home string) []string {
	dirs := []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".opencode", "bin"),
		filepath.Join(home, ".volta", "bin"),
		filepath.Join(home, "go", "bin"),
	}
	for _, pattern := range []string{
		filepath.Join(home, ".openclaw", "tools", "*", "bin"),
		filepath.Join(home, ".nvm", "versions", "node", "*", "bin"),
	} {
		matches, _ := filepath.Glob(pattern)
		dirs = append(dirs, matches...)
	}
	return dirs
}

// defaultVersionRunner runs `<path> --version` with the caller's context
// (5s hard timeout) and returns the first output line.
func defaultVersionRunner(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, path, "--version")
	configureProcessGroup(cmd)
	cmd.Env = sanitizedEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Some CLIs exit non-zero for --version; accept any output as version.
		if len(strings.TrimSpace(string(out))) == 0 {
			return "", err
		}
	}
	return firstLine(string(out)), nil
}

// sanitizedEnv minimizes inherited environment for the version command.
func sanitizedEnv() []string {
	return []string{"PATH=" + os.Getenv("PATH")}
}

func firstLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// truncateVersion caps the version string at versionMaxLen bytes (K3: <=80).
func truncateVersion(s string) string {
	if len(s) > versionMaxLen {
		return s[:versionMaxLen]
	}
	return s
}
