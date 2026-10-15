package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/agentprobe"
	"github.com/ianf339/roambench/internal/auth"
)

// contract stub tests for C1 expect the agents route to return 501 until C4
// lands; these tests pin the real handler's JSON contract (K3).

func agentProbeHandlerRequest(t *testing.T, srv *Server, token string, path string) (*httptest.ResponseRecorder, agentprobe.Snapshot) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var snap agentprobe.Snapshot
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(&snap); err != nil {
			t.Fatalf("decode agents JSON: %v", err)
		}
	}
	return rec, snap
}

func TestAgentsEndpointJSONContract(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	rec, snap := agentProbeHandlerRequest(t, srv, token, "/api/project-control/agents")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/project-control/agents = %d, want 200", rec.Code)
	}
	if snap.DetectedAt == "" {
		t.Error("detectedAt must be set")
	}
	if _, err := json.Marshal(snap); err != nil {
		t.Fatalf("re-marshal: %v", err)
	}

	wantLabels := map[string]bool{"kiro-cli": false, "claude": false, "codex": false, "opencode": false, "hermes": false}
	if len(snap.Agents) != len(wantLabels) {
		t.Fatalf("got %d agents, want %d", len(snap.Agents), len(wantLabels))
	}
	for _, a := range snap.Agents {
		if _, known := wantLabels[a.AgentLabel]; !known {
			t.Errorf("unexpected agentLabel %q", a.AgentLabel)
		}
		wantLabels[a.AgentLabel] = true
		if a.Binary == "" {
			t.Errorf("%s: binary must be set", a.AgentLabel)
		}
		if a.Models == nil {
			t.Errorf("%s: models must be non-nil (GATE-1 #3: empty allowed, not null)", a.AgentLabel)
		}
		if a.Installed && a.Version == "" {
			// acceptable (version probe may fail), but Path must be set
			if a.Path == "" {
				t.Errorf("%s: installed with empty path", a.AgentLabel)
			}
		}
		if !a.Installed && a.Version != "" {
			t.Errorf("%s: version set but installed=false", a.AgentLabel)
		}
	}
	for label, seen := range wantLabels {
		if !seen {
			t.Errorf("missing agentLabel %q", label)
		}
	}

	// recommendations: tier/worker/reviewer validation
	seenTiers := map[string]bool{}
	for _, r := range snap.Recommendations {
		switch r.Tier {
		case "T1", "T2", "T3", "T4":
			if seenTiers[r.Tier] {
				t.Errorf("duplicate tier %s", r.Tier)
			}
			seenTiers[r.Tier] = true
		default:
			t.Errorf("invalid tier %q", r.Tier)
		}
		if r.WorkerLabel == "" {
			t.Errorf("%s: empty workerLabel", r.Tier)
		}
		if r.ReviewerLabel == r.WorkerLabel {
			t.Errorf("%s: reviewer must differ from worker", r.Tier)
		}
		if r.Reason == "" {
			t.Errorf("%s: empty reason", r.Tier)
		}
	}
}

func TestAgentsEndpointRefreshForcesReprobe(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	_, first := agentProbeHandlerRequest(t, srv, token, "/api/project-control/agents")
	_, cached := agentProbeHandlerRequest(t, srv, token, "/api/project-control/agents")
	if cached.DetectedAt != first.DetectedAt {
		t.Errorf("cached probe detectedAt changed: %q vs %q", first.DetectedAt, cached.DetectedAt)
	}
	_, refreshed := agentProbeHandlerRequest(t, srv, token, "/api/project-control/agents?refresh=1")
	if refreshed.DetectedAt == first.DetectedAt {
		// RFC3339 seconds resolution can legitimately coincide on a fast
		// machine; treat an unchanged timestamp as a re-probe failure only
		// when the probe is provably not cached — force one more refresh with
		// a tiny sleep to cross the second boundary.
		time.Sleep(1100 * time.Millisecond)
		_, refreshed = agentProbeHandlerRequest(t, srv, token, "/api/project-control/agents?refresh=1")
		if refreshed.DetectedAt == first.DetectedAt {
			t.Error("refresh=1 did not force re-probe (same detectedAt)")
		}
	}
}

func TestAgentsEndpointRequiresAuth(t *testing.T) {
	srv, _, sessions := testProjectControlServer(t)
	defer sessions.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/project-control/agents", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated agents request = %d, want 401", rec.Code)
	}
}
