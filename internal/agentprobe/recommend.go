package agentprobe

import "fmt"

// Recommendations derives the T1-T4 tier assignments from a probe result.
//
// Deterministic by construction: the input slice fully determines the output,
// no wall-clock or randomness involved. Tiers are emitted in order T1..T4.
//
// T1/T2 (light tasks): worker is the cheapest available agent by the fixed
// cost priority opencode > kiro-cli > codex > claude (hermes is the runtime
// itself and never a worker/reviewer); reviewer is the next available agent
// that differs from the worker.
//
// T3/T4 (heavy tasks): worker is kiro-cli when installed, otherwise codex;
// reviewer is the other family of the pair. A tier is only emitted when both
// a worker and a distinct reviewer are installed.
func Recommendations(agents []Agent) []Recommendation {
	installed := map[string]bool{}
	for _, a := range agents {
		if a.Installed {
			installed[a.AgentLabel] = true
		}
	}

	lightPriority := []string{"opencode", "kiro-cli", "codex", "claude"}
	out := make([]Recommendation, 0, 4)

	worker, ok := firstInstalled(lightPriority, installed)
	if ok {
		reviewer, ok := firstInstalledExcluding(lightPriority, installed, worker)
		if ok {
			reason := fmt.Sprintf("T1/T2: cheapest available agent %s as worker, distinct reviewer %s", worker, reviewer)
			out = append(out,
				Recommendation{Tier: "T1", WorkerLabel: worker, ReviewerLabel: reviewer, Reason: reason},
				Recommendation{Tier: "T2", WorkerLabel: worker, ReviewerLabel: reviewer, Reason: reason},
			)
		}
	}

	// T3/T4: kiro-cli preferred, codex as the alternate family.
	heavyPair := []string{"kiro-cli", "codex"}
	heavyWorker, ok := firstInstalled(heavyPair, installed)
	if ok {
		reviewer, ok := firstInstalledExcluding(heavyPair, installed, heavyWorker)
		if ok {
			reason := fmt.Sprintf("T3/T4: worker %s, reviewer from different family %s", heavyWorker, reviewer)
			out = append(out,
				Recommendation{Tier: "T3", WorkerLabel: heavyWorker, ReviewerLabel: reviewer, Reason: reason},
				Recommendation{Tier: "T4", WorkerLabel: heavyWorker, ReviewerLabel: reviewer, Reason: reason},
			)
		}
	}

	return out
}

// firstInstalled returns the first label in priority order that is installed.
func firstInstalled(priority []string, installed map[string]bool) (string, bool) {
	for _, label := range priority {
		if installed[label] {
			return label, true
		}
	}
	return "", false
}

// firstInstalledExcluding is firstInstalled but skips the given label, so the
// reviewer is always a different agent from the worker.
func firstInstalledExcluding(priority []string, installed map[string]bool, exclude string) (string, bool) {
	for _, label := range priority {
		if label != exclude && installed[label] {
			return label, true
		}
	}
	return "", false
}
