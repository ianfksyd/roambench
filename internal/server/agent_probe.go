package server

import (
	"net/http"
	"time"

	"github.com/ianf339/roambench/internal/agentprobe"
)

// F2 agent probe handler (WS-20261015-001, K3 contract).
//
// GET /api/project-control/agents[?refresh=1]
//
// Returns the cached probe snapshot, or re-probes when refresh=1:
//
//	{detectedAt, agents:[{agentLabel,binary,path,installed,version,
//	  nonInteractive,models[]}], recommendations:[{tier,workerLabel,
//	  reviewerLabel,reason}]}
//
// The handler wires a package-level Prober onto the Server; the probe itself
// runs side-effect-free --version commands with a 5s timeout and never fails
// the request — detection problems surface as installed=false / missing
// version instead of a 5xx.

var agentProbeDefault = agentprobe.NewProber()

// agentProbeResponse is the K3 JSON envelope for the agents endpoint.
type agentProbeResponse struct {
	DetectedAt      string                      `json:"detectedAt"`
	Agents          []agentprobe.Agent          `json:"agents"`
	Recommendations []agentprobe.Recommendation `json:"recommendations"`
}

func (s *Server) handleProjectControlAgents(w http.ResponseWriter, r *http.Request) {
	refresh := r.URL.Query().Get("refresh") == "1"
	snap := agentProbeDefault.Probe(refresh)
	writeJSON(w, http.StatusOK, agentProbeResponse{
		DetectedAt:      snap.DetectedAt,
		Agents:          snap.Agents,
		Recommendations: snap.Recommendations,
	})
}

// agentProbeStaleness reports how long ago the current snapshot was detected.
// Used by tests (and future UI) to reason about cache freshness.
func agentProbeStaleness(snap *agentprobe.Snapshot) time.Duration {
	detected, err := time.Parse(time.RFC3339, snap.DetectedAt)
	if err != nil {
		return time.Since(time.Time{})
	}
	return time.Since(detected)
}
