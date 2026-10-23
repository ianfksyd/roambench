package agentprobe

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stubScript writes an executable shell stub into a fake PATH dir and returns
// the dir. The stub either echoes a fixed version, sleeps past the timeout, or
// emits binary garbage, depending on the script body.
func stubScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", name, err)
	}
	return dir
}

// fakeProber builds a Prober whose lookPath resolves binaries inside dir and
// whose versionRunner invokes them for real (via exec), so timeout and binary
// output scenarios exercise the actual subprocess path.
func fakeProber(dir string) *Prober {
	return &Prober{
		versionRunner: defaultVersionRunner,
		lookPath: func(binary string) (string, error) {
			p := filepath.Join(dir, binary)
			if _, err := os.Stat(p); err != nil {
				return "", errors.New("not found")
			}
			return p, nil
		},
	}
}

func agentByLabel(snap *Snapshot, label string) Agent {
	for _, a := range snap.Agents {
		if a.AgentLabel == label {
			return a
		}
	}
	return Agent{AgentLabel: label}
}

func TestProbeNormalVersion(t *testing.T) {
	dir := t.TempDir()
	stubScript(t, dir, "kiro-cli", `echo "kiro-cli 9.9.9 (stub)"`)
	stubScript(t, dir, "claude", `echo "2.1.300 (Claude Code)"`)
	stubScript(t, dir, "codex", `echo "codex-cli 0.162.1"`)
	stubScript(t, dir, "opencode", `echo "opencode 2.0.27"`)
	stubScript(t, dir, "hermes", `echo "hermes 0.1.0"`) // present in fake PATH

	p := fakeProber(dir)
	snap := p.Probe(false)

	if len(snap.Agents) != 5 {
		t.Fatalf("got %d agents, want 5", len(snap.Agents))
	}
	if _, err := time.Parse(time.RFC3339, snap.DetectedAt); err != nil {
		t.Errorf("detectedAt not RFC3339: %v", snap.DetectedAt)
	}
	for _, want := range []struct{ label, version string }{
		{"kiro-cli", "kiro-cli 9.9.9 (stub)"},
		{"claude", "2.1.300 (Claude Code)"},
		{"codex", "codex-cli 0.162.1"},
		{"opencode", "opencode 2.0.27"},
		{"hermes", "hermes 0.1.0"},
	} {
		a := agentByLabel(snap, want.label)
		if !a.Installed {
			t.Errorf("%s: installed=false, want true", want.label)
		}
		if a.Version != want.version {
			t.Errorf("%s version = %q, want %q", want.label, a.Version, want.version)
		}
		if a.Models == nil {
			t.Errorf("%s: models must be non-nil empty (GATE-1 #3)", want.label)
		}
		if a.Binary != want.label {
			t.Errorf("%s: binary = %q", want.label, a.Binary)
		}
	}
}

func TestProbeMissingBinary(t *testing.T) {
	dir := t.TempDir()
	stubScript(t, dir, "kiro-cli", `echo "kiro-cli 1.0.0"`)
	stubScript(t, dir, "claude", `echo "claude 1.0.0"`)
	stubScript(t, dir, "codex", `echo "codex 1.0.0"`)
	stubScript(t, dir, "opencode", `echo "opencode 1.0.0"`)
	// hermes intentionally absent from the fake PATH -> installed=false.

	p := fakeProber(dir)
	snap := p.Probe(false)

	a := agentByLabel(snap, "hermes")
	if a.Installed {
		t.Error("hermes: installed=true, want false (not on fake PATH)")
	}
	if a.Version != "" {
		t.Errorf("hermes: version = %q, want empty", a.Version)
	}
	if a.Models == nil {
		t.Error("hermes: models must still be non-nil empty")
	}
	if got := len(snap.Recommendations); got != 4 {
		t.Errorf("recommendations = %d entries, want 4 (all T1-T4)", got)
	}
}

func TestProbeTimeout(t *testing.T) {
	dir := t.TempDir()
	stubScript(t, dir, "kiro-cli", `sleep 6; echo "too late"`)
	stubScript(t, dir, "claude", `echo "claude 1.0.0"`)
	stubScript(t, dir, "codex", `echo "codex 1.0.0"`)
	stubScript(t, dir, "opencode", `echo "opencode 1.0.0"`)

	start := time.Now()
	p := fakeProber(dir)
	snap := p.Probe(false)
	elapsed := time.Since(start)

	if elapsed >= 6*time.Second {
		t.Errorf("probe took %v, timeout of 5s should have bounded it", elapsed)
	}
	a := agentByLabel(snap, "kiro-cli")
	if a.Version != "" {
		t.Errorf("kiro-cli: version = %q after timeout, want empty", a.Version)
	}
	if a.Path == "" {
		t.Error("kiro-cli: path should still be set (binary exists)")
	}
}

func TestProbeGarbageOutput(t *testing.T) {
	dir := t.TempDir()
	stubScript(t, dir, "kiro-cli", `printf '\000\377\376\001binary-garbage-line\nsecond line'`)
	stubScript(t, dir, "claude", `echo "claude 1.0.0"`)
	stubScript(t, dir, "codex", `echo "codex 1.0.0"`)
	stubScript(t, dir, "opencode", `echo "opencode 1.0.0"`)

	p := fakeProber(dir)
	snap := p.Probe(false)

	a := agentByLabel(snap, "kiro-cli")
	if !a.Installed {
		t.Error("kiro-cli: installed=false, want true (binary ran, output was just garbled)")
	}
	want := "\x00\xff\xfe\x01binary-garbage-line"
	if a.Version != want {
		t.Errorf("kiro-cli version = %q, want first line %q", a.Version, want)
	}
}

func TestProbeVersionTruncatedTo80(t *testing.T) {
	dir := t.TempDir()
	// 100 chars of 'x' on a single line -> truncated to 80.
	stubScript(t, dir, "kiro-cli", `printf 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'`)
	stubScript(t, dir, "claude", `echo "claude 1.0.0"`)
	stubScript(t, dir, "codex", `echo "codex 1.0.0"`)
	stubScript(t, dir, "opencode", `echo "opencode 1.0.0"`)

	p := fakeProber(dir)
	snap := p.Probe(false)

	a := agentByLabel(snap, "kiro-cli")
	if len(a.Version) != 80 {
		t.Errorf("version length = %d, want 80 (truncated)", len(a.Version))
	}
}

func TestProbeCacheAndRefresh(t *testing.T) {
	dir := t.TempDir()
	stubScript(t, dir, "kiro-cli", `echo "v-cached"`)
	stubScript(t, dir, "claude", `echo "v-cached"`)
	stubScript(t, dir, "codex", `echo "v-cached"`)
	stubScript(t, dir, "opencode", `echo "v-cached"`)

	p := fakeProber(dir)
	first := p.Probe(false)
	second := p.Probe(false) // cached: same snapshot pointer
	if first != second {
		t.Error("second Probe(false) should return the cached snapshot")
	}
	// Rewrite the stub with a new version; refresh must pick it up.
	if err := os.WriteFile(filepath.Join(dir, "kiro-cli"), []byte("#!/bin/sh\necho v-refreshed\n"), 0o755); err != nil {
		t.Fatalf("rewrite stub: %v", err)
	}
	third := p.Probe(true) // forced re-probe
	if got := agentByLabel(third, "kiro-cli").Version; got != "v-refreshed" {
		t.Errorf("after refresh kiro-cli version = %q, want v-refreshed", got)
	}
}

func TestRecommendationsDeterministic(t *testing.T) {
	mk := func(installed map[string]bool) []Agent {
		out := make([]Agent, 0, len(agentLabels))
		for _, label := range agentLabels {
			a := Agent{AgentLabel: label, Binary: label, Models: []string{}}
			if installed[label] {
				a.Installed = true
			}
			out = append(out, a)
		}
		return out
	}

	cases := []struct {
		name      string
		installed map[string]bool
		wantT12   [2]string // worker, reviewer
		wantT34   [2]string
		wantLen   int
	}{
		{
			name:      "all four installed",
			installed: map[string]bool{"kiro-cli": true, "claude": true, "codex": true, "opencode": true},
			wantT12:   [2]string{"opencode", "kiro-cli"}, // cheapest first
			wantT34:   [2]string{"kiro-cli", "codex"},    // kiro worker, codex reviewer
			wantLen:   4,
		},
		{
			name:      "only claude",
			installed: map[string]bool{"claude": true},
			wantT12:   [2]string{"claude", ""}, // no distinct reviewer
			wantLen:   0,
		},
		{
			name:      "kiro and claude",
			installed: map[string]bool{"kiro-cli": true, "claude": true},
			wantT12:   [2]string{"kiro-cli", "claude"},
			wantT34:   [2]string{"kiro-cli", ""}, // codex missing, no reviewer family
			wantLen:   2,
		},
		{
			name:      "only codex",
			installed: map[string]bool{"codex": true},
			wantT12:   [2]string{"codex", ""},
			wantT34:   [2]string{"codex", ""},
			wantLen:   0,
		},
		{
			name:      "codex and claude",
			installed: map[string]bool{"codex": true, "claude": true},
			wantT12:   [2]string{"codex", "claude"},
			wantT34:   [2]string{"codex", ""}, // kiro missing
			wantLen:   2,
		},
		{
			name:      "none installed",
			installed: map[string]bool{},
			wantLen:   0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := mk(tc.installed)
			got := Recommendations(in)
			if len(got) != tc.wantLen {
				t.Fatalf("got %d recommendations, want %d: %+v", len(got), tc.wantLen, got)
			}
			if tc.wantLen == 0 {
				if got == nil {
					t.Error("recommendations must be empty slice, not nil (JSON contract)")
				}
				return
			}
			byTier := map[string]Recommendation{}
			for _, r := range got {
				byTier[r.Tier] = r
			}
			for _, tier := range []string{"T1", "T2"} {
				r, ok := byTier[tier]
				if !ok {
					t.Fatalf("missing %s", tier)
				}
				if r.WorkerLabel != tc.wantT12[0] || r.ReviewerLabel != tc.wantT12[1] {
					t.Errorf("%s = worker %q reviewer %q, want %q/%q", tier, r.WorkerLabel, r.ReviewerLabel, tc.wantT12[0], tc.wantT12[1])
				}
			}
			if rec, ok := byTier["T3"]; ok {
				if rec.WorkerLabel != tc.wantT34[0] || rec.ReviewerLabel != tc.wantT34[1] {
					t.Errorf("T3 = worker %q reviewer %q, want %q/%q", rec.WorkerLabel, rec.ReviewerLabel, tc.wantT34[0], tc.wantT34[1])
				}
			}
			if rec, ok := byTier["T4"]; ok {
				if rec.WorkerLabel != tc.wantT34[0] || rec.ReviewerLabel != tc.wantT34[1] {
					t.Errorf("T4 = worker %q reviewer %q, want %q/%q", rec.WorkerLabel, rec.ReviewerLabel, tc.wantT34[0], tc.wantT34[1])
				}
			}
			// Determinism: same input -> identical output.
			again := Recommendations(mk(tc.installed))
			if len(again) != len(got) {
				t.Fatalf("non-deterministic length: %d vs %d", len(again), len(got))
			}
			for i := range got {
				if got[i] != again[i] {
					t.Errorf("non-deterministic at %d: %+v vs %+v", i, got[i], again[i])
				}
			}
		})
	}
}

func TestFirstLineAndTruncation(t *testing.T) {
	if got := firstLine("1.2.3\r\nother\nlines"); got != "1.2.3" {
		t.Errorf("firstLine CRLF = %q", got)
	}
	if got := firstLine("  spaced  "); got != "spaced" {
		t.Errorf("firstLine trim = %q", got)
	}
	if got := truncateVersion(string(make([]byte, 100)) + "abcdef"); len(got) != 80 {
		t.Errorf("truncateVersion len = %d, want 80", len(got))
	}
}

func TestDefaultVersionRunnerContextCancel(t *testing.T) {
	dir := t.TempDir()
	// Slow stub; context deadline should fire first.
	p := filepath.Join(dir, "slow")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nsleep 30; echo late\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := defaultVersionRunner(ctx, p); err == nil {
		t.Error("expected error from cancelled context")
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("runner did not respect context deadline")
	}
}

// lookupAgentBinary must find binaries that live in the common install
// directories of agent managers even when PATH does not include them.
func TestLookupAgentBinaryFallsBackToInstallDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// The resolver reads $HOME through os.UserHomeDir; on macOS the temp dir
	// may be a symlink, which does not matter for the fallback logic.
	dir := filepath.Join(home, ".opencode", "bin")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "opencode")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho v1\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Force the PATH miss: a PATH of one read-only directory finds nothing.
	t.Setenv("PATH", "/nonexistent-roambench-probe")

	path, err := lookupAgentBinary("opencode")
	if err != nil {
		t.Fatalf("lookupAgentBinary error: %v", err)
	}
	if path != binary {
		t.Fatalf("path = %q, want %q", path, binary)
	}
	if _, err := lookupAgentBinary("definitely-not-installed-xyz"); err == nil {
		t.Fatal("expected an error for a missing binary")
	}
}

// A candidate without the exec bit is not a usable binary.
func TestLookupAgentBinaryIgnoresNonExecutables(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".opencode", "bin")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte("not executable"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/nonexistent-roambench-probe")

	if _, err := lookupAgentBinary("opencode"); err == nil {
		t.Fatal("a non-executable file must not count as installed")
	}
}
