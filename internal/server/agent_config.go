package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/ianf339/roambench/internal/agentcfg"
	"github.com/ianf339/roambench/internal/agentprobe"
	"github.com/ianf339/roambench/internal/roots"
)

// Agent configuration wizard.
//
// The wizard shows which coding agents are installed, where they currently
// point, and what would change to move them onto one gateway. Applying the
// plan is deterministic: the server edits the agent configuration files
// itself, keeping a backup, and never asks a language model to do it.

// agentConfigProfileRequest carries the profile to apply. An empty profile
// falls back to the instance configuration.
type agentConfigProfileRequest struct {
	Profile agentcfg.Profile `json:"profile"`
}

// agentConfigReport is one agent row of the wizard.
type agentConfigReport struct {
	Label      string `json:"label"`
	Installed  bool   `json:"installed"`
	Version    string `json:"version,omitempty"`
	ConfigPath string `json:"configPath,omitempty"`
	Supported  bool   `json:"supported"`
	// Reason explains an unsupported agent: "worker_only" = the CLI has no
	// configuration surface for a custom model endpoint.
	Reason  string            `json:"reason,omitempty"`
	Current map[string]string `json:"current,omitempty"`
	Plan    *agentcfg.Plan    `json:"plan,omitempty"`
}

// agentConfigOverview is the whole wizard state.
type agentConfigOverview struct {
	Profile agentcfg.Profile    `json:"profile"`
	Agents  []agentConfigReport `json:"agents"`
	Steps   []agentcfg.Step     `json:"steps,omitempty"`
}

// agentConfigProfile returns the profile the instance offers by default.
func (s *Server) agentConfigProfile() agentcfg.Profile {
	gateway := s.cfg.Agent.Gateway
	return agentcfg.Profile{
		Name:      gateway.Name,
		BaseURL:   gateway.BaseURL,
		AuthToken: gateway.AuthToken,
		Model:     gateway.Model,
		CodexPath: gateway.CodexPath,
	}
}

// agentConfigHome returns the home directory the agent configuration belongs
// to: the home of the signed-in user, not the process home.
func agentConfigHome(username string) string {
	if home := roots.HomeDir(username); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return home
}

// agentConfigOverview builds the wizard state for a user.
func (s *Server) agentConfigOverview(username string, profile agentcfg.Profile) agentConfigOverview {
	home := agentConfigHome(username)
	snapshot := agentProbeDefault.Probe(false)
	installed := map[string]agentprobe.Agent{}
	for _, agent := range snapshot.Agents {
		installed[agent.AgentLabel] = agent
	}
	overview := agentConfigOverview{Profile: profile}
	for _, label := range agentcfg.Adapters() {
		report := agentConfigReport{Label: label, Supported: true}
		if agent, ok := installed[label]; ok {
			report.Installed = agent.Installed
			report.Version = agent.Version
		}
		path, ok := agentcfg.ConfigPath(label, home)
		if !ok {
			continue
		}
		report.ConfigPath = path
		report.Current = agentcfg.CurrentFor(label, home, profile)
		if plan, err := agentcfg.PlanFor(label, home, profile); err == nil {
			report.Plan = &plan
		}
		overview.Agents = append(overview.Agents, report)
	}
	// Agents that are installed but have no adapter still show up, so the
	// user sees that they will not be moved automatically. kiro-cli has no
	// gateway configuration surface at all: it is a worker, not a candidate.
	for _, agent := range snapshot.Agents {
		if agentcfg.Supports(agent.AgentLabel) || !agent.Installed {
			continue
		}
		report := agentConfigReport{Label: agent.AgentLabel, Installed: true, Version: agent.Version, Supported: false}
		if agentcfg.GatewayUnsupported(agent.AgentLabel) {
			report.Reason = "worker_only"
		}
		overview.Agents = append(overview.Agents, report)
	}
	return overview
}

// handleAgentConfig serves the wizard: overview, plan, apply and guide.
//
// GET  /api/project-control/agent-config
// POST /api/project-control/agent-config/plan
// POST /api/project-control/agent-config/apply
// GET  /api/project-control/agent-config/guide?agent=<label>
func (s *Server) handleAgentConfig(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	switch r.Method {
	case http.MethodGet:
		if agent := strings.TrimSpace(r.URL.Query().Get("agent")); agent != "" {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"steps": agentcfg.Guide(agent, s.agentConfigProfile()),
			})
			return
		}
		writeJSON(w, http.StatusOK, s.agentConfigOverview(username, s.agentConfigProfile()))
	case http.MethodPost:
		var req agentConfigProfileRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		profile := req.Profile
		if strings.TrimSpace(profile.BaseURL) == "" {
			profile = s.agentConfigProfile()
		}
		agent := strings.TrimSpace(r.URL.Query().Get("agent"))
		if agent == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent is required"})
			return
		}
		home := agentConfigHome(username)
		switch strings.TrimSpace(r.URL.Query().Get("action")) {
		case "apply":
			result, err := agentcfg.Apply(agent, home, profile)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, result)
		default:
			plan, err := agentcfg.PlanFor(agent, home, profile)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, plan)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
