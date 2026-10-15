package server

import (
	"net/http"
)

// Dispatch pipeline & board stubs (WS-20261015-001, C1 contract桩).
// K2/K3/K5 routes are registered in server.go; handlers live here so that
// C2 (board backend), C4 (agent probe) and C5 (claims) replace the bodies
// without touching route registration again.

// tokenRequired wraps an agent-API handler with the same bearer-token check
// the existing /api/agent/v1 handlers use (s.agentUsername).
func (s *Server) tokenRequired(next func(w http.ResponseWriter, r *http.Request, username string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, ok := s.agentUsername(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid agent token"})
			return
		}
		next(w, r, username)
	}
}

// handleAgentClaim / handleAgentHeartbeat / handleAgentRelease live in
// agent_claims.go (C5). handleProjectControlAgents is in agent_probe.go (C4).
// handleProjectControlBoard body is served by serveProjectControlBoard in
// project_board.go (C2, K2).

func (s *Server) handleProjectControlBoard(w http.ResponseWriter, r *http.Request) {
	s.serveProjectControlBoard(w, r)
}

// handleProjectControlAgents is implemented in agent_probe.go (C4, K3).
// The 501 stub body was removed here to avoid a duplicate declaration; the
// route registration in server.go is unchanged.

func (s *Server) writeStub(w http.ResponseWriter, route string) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "not implemented yet (WS-20261015-001 contract stub)",
		"route": route,
	})
}
