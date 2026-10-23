// Package agentcfg applies a model-access profile to the configuration files
// of the coding agents installed on a machine.
//
// The point is to unify every agent onto one gateway without an LLM editing
// user configuration: each agent CLI has its own file format, so this package
// holds one small deterministic adapter per CLI. Every write is preceded by
// a backup and reported as a list of key-level changes, so the caller can
// show a preview before touching anything.
package agentcfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Profile describes one model access point, typically a local gateway.
type Profile struct {
	// Name labels the profile in the UI.
	Name string `json:"name"`
	// BaseURL is the gateway base URL.
	BaseURL string `json:"baseUrl"`
	// AuthToken is the credential value written into the agent config. For a
	// local gateway this is usually a placeholder rather than a real secret.
	AuthToken string `json:"authToken,omitempty"`
	// Model is the model id the agent should use.
	Model string `json:"model,omitempty"`
	// CodexPath is the backend path suffix Codex needs; most gateways expose
	// the OpenAI-compatible API on a different path than the Anthropic one.
	CodexPath string `json:"codexPath,omitempty"`
}

// Protocol returns the wire protocol of the profile.
func (p Profile) Protocol() string {
	if strings.HasSuffix(strings.TrimRight(p.BaseURL, "/"), "/backend-api/codex") {
		return "codex"
	}
	return "anthropic"
}

// codexBaseURL returns the base URL Codex should use.
func (p Profile) codexBaseURL() string {
	base := strings.TrimRight(p.BaseURL, "/")
	if p.CodexPath == "" {
		return base
	}
	if strings.HasSuffix(base, p.CodexPath) {
		return base
	}
	return base + "/" + strings.TrimLeft(p.CodexPath, "/")
}

// Change is one key-level configuration change.
type Change struct {
	// Key identifies the setting, e.g. "env.ANTHROPIC_BASE_URL".
	Key string `json:"key"`
	// Before and After are the values; Before is empty when the key is new.
	Before string `json:"before"`
	After  string `json:"after"`
	// Action is "update", "add" or "none".
	Action string `json:"action"`
}

// Plan is the result of comparing an agent configuration with a profile.
type Plan struct {
	// Agent is the agent label, e.g. "claude".
	Agent string `json:"agent"`
	// Path is the configuration file that would change.
	Path string `json:"path"`
	// Changes lists the key-level changes.
	Changes []Change `json:"changes"`
	// Note explains why nothing changes, when nothing does.
	Note string `json:"note,omitempty"`
}

// Changed reports whether the plan would modify the file.
func (p Plan) Changed() bool {
	for _, change := range p.Changes {
		if change.Action != "none" {
			return true
		}
	}
	return false
}

// ApplyResult reports what Apply did.
type ApplyResult struct {
	Plan       Plan   `json:"plan"`
	Changed    bool   `json:"changed"`
	BackupPath string `json:"backupPath,omitempty"`
}

// Adapters describes where an agent keeps its configuration and how a
// profile maps onto it.
type Adapter struct {
	// Label is the agent label.
	Label string
	// ConfigPath is the configuration file, relative to the home directory.
	ConfigPath string
	// Apply writes the profile into the file content and returns the changes.
	Apply func(content string, profile Profile) (string, []Change, error)
	// Current reads the values of the settings the profile touches.
	Current func(content string) map[string]string
}

// adapters is the fixed set of supported agents.
var adapters = map[string]Adapter{
	"claude": {
		Label:      "claude",
		ConfigPath: filepath.Join(".claude", "settings.json"),
		Apply:      applyClaude,
		Current:    currentClaude,
	},
	"codex": {
		Label:      "codex",
		ConfigPath: filepath.Join(".codex", "config.toml"),
		Apply:      applyCodex,
		Current:    currentCodex,
	},
	"opencode": {
		Label:      "opencode",
		ConfigPath: filepath.Join(".config", "opencode", "opencode.json"),
		Apply:      applyOpenCode,
		Current:    currentOpenCode,
	},
}

// targetsFor returns the configuration keys a profile touches per agent.
func targetsFor(label string, profile Profile) []string {
	switch strings.TrimSpace(label) {
	case "claude":
		keys := make([]string, 0, len(claudeTargets(profile)))
		for key := range claudeTargets(profile) {
			keys = append(keys, "env."+key)
		}
		sort.Strings(keys)
		return keys
	case "codex":
		keys := make([]string, 0, 2)
		for _, key := range []string{"openai_base_url", "model"} {
			if key == "model" && profile.Model == "" {
				continue
			}
			keys = append(keys, key)
		}
		return keys
	case "opencode":
		keys := make([]string, 0, len(opencodeTargets(profile)))
		for key := range opencodeTargets(profile) {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return keys
	default:
		return nil
	}
}

// Adapters returns the supported agent labels, sorted.
func Adapters() []string {
	labels := make([]string, 0, len(adapters))
	for label := range adapters {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return labels
}

// Supports reports whether an agent label has an adapter.
func Supports(label string) bool {
	_, ok := adapters[strings.TrimSpace(label)]
	return ok
}

// Current returns the settings of an agent that a profile would touch.
func Current(label, home string) map[string]string {
	adapter, ok := adapters[strings.TrimSpace(label)]
	if !ok {
		return nil
	}
	path := filepath.Join(home, adapter.ConfigPath)
	content, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	return adapter.Current(string(content))
}

// CurrentFor returns only the settings a profile touches. The wizard shows
// these, so the row stays readable even when the agent configuration holds
// dozens of unrelated keys.
func CurrentFor(label, home string, profile Profile) map[string]string {
	adapter, ok := adapters[strings.TrimSpace(label)]
	if !ok {
		return nil
	}
	path := filepath.Join(home, adapter.ConfigPath)
	content, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	all := adapter.Current(string(content))
	current := map[string]string{}
	for _, key := range targetsFor(adapter.Label, profile) {
		if value, ok := all[key]; ok {
			current[key] = value
		}
	}
	return current
}

// ConfigPath returns the configuration file of an agent for a home directory.
func ConfigPath(label, home string) (string, bool) {
	adapter, ok := adapters[strings.TrimSpace(label)]
	if !ok {
		return "", false
	}
	return filepath.Join(home, adapter.ConfigPath), true
}

// PlanFor compares the current configuration of an agent with a profile.
func PlanFor(label, home string, profile Profile) (Plan, error) {
	adapter, ok := adapters[strings.TrimSpace(label)]
	if !ok {
		return Plan{}, fmt.Errorf("unsupported agent: %s", label)
	}
	path := filepath.Join(home, adapter.ConfigPath)
	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return Plan{}, err
		}
		content = []byte{}
	}
	updated, changes, err := adapter.Apply(string(content), profile)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Agent: adapter.Label, Path: path, Changes: changes}
	if !plan.Changed() {
		plan.Note = "already points at this profile"
	}
	_ = updated
	return plan, nil
}

// Apply writes the profile into the agent configuration, keeping a backup of
// the previous file.
func Apply(label, home string, profile Profile) (ApplyResult, error) {
	plan, err := PlanFor(label, home, profile)
	if err != nil {
		return ApplyResult{}, err
	}
	result := ApplyResult{Plan: plan}
	if !plan.Changed() {
		return result, nil
	}
	adapter := adapters[strings.TrimSpace(label)]
	content, err := os.ReadFile(plan.Path)
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	updated, _, err := adapter.Apply(string(content), profile)
	if err != nil {
		return result, err
	}
	if len(content) > 0 {
		backup := plan.Path + ".roambench-bak"
		if err := os.WriteFile(backup, content, 0644); err != nil {
			return result, fmt.Errorf("cannot back up %s: %w", plan.Path, err)
		}
		result.BackupPath = backup
	}
	if err := os.MkdirAll(filepath.Dir(plan.Path), 0755); err != nil {
		return result, err
	}
	if err := os.WriteFile(plan.Path, []byte(updated), 0644); err != nil {
		return result, err
	}
	result.Changed = true
	return result, nil
}

// ---- Claude Code ----

// claudeEnvKeys are the settings a profile touches in ~/.claude/settings.json.
func claudeTargets(profile Profile) map[string]string {
	targets := map[string]string{
		"ANTHROPIC_BASE_URL": profile.BaseURL,
	}
	if profile.AuthToken != "" {
		targets["ANTHROPIC_AUTH_TOKEN"] = profile.AuthToken
	}
	if profile.Model != "" {
		targets["ANTHROPIC_DEFAULT_SONNET_MODEL"] = profile.Model
		targets["ANTHROPIC_DEFAULT_OPUS_MODEL"] = profile.Model
		targets["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = profile.Model
	}
	return targets
}

// decodeJSONObject parses a JSON object into raw members, so unrelated keys
// survive a rewrite untouched.
func decodeJSONObject(content string) (map[string]json.RawMessage, error) {
	object := map[string]json.RawMessage{}
	if strings.TrimSpace(content) == "" {
		return object, nil
	}
	if err := json.Unmarshal([]byte(content), &object); err != nil {
		return nil, err
	}
	return object, nil
}

// rawString reads a raw member as a string.
func rawString(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

// encodeJSONObject renders a raw member map as indented JSON.
func encodeJSONObject(object map[string]json.RawMessage) (string, error) {
	data, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func applyClaude(content string, profile Profile) (string, []Change, error) {
	root, err := decodeJSONObject(content)
	if err != nil {
		return "", nil, fmt.Errorf("cannot parse claude settings: %w", err)
	}
	env := map[string]json.RawMessage{}
	if raw, ok := root["env"]; ok {
		if err := json.Unmarshal(raw, &env); err != nil {
			return "", nil, fmt.Errorf("cannot parse claude env: %w", err)
		}
	}
	targets := claudeTargets(profile)
	changes := make([]Change, 0, len(targets))
	for key, want := range targets {
		got, ok := "", false
		if raw, present := env[key]; present {
			got, ok = rawString(raw)
		}
		switch {
		case ok && got == want:
			changes = append(changes, Change{Key: "env." + key, Before: got, After: want, Action: "none"})
		case ok:
			changes = append(changes, Change{Key: "env." + key, Before: got, After: want, Action: "update"})
		default:
			changes = append(changes, Change{Key: "env." + key, Before: "", After: want, Action: "add"})
		}
	}
	sortChanges(changes)
	for key, want := range targets {
		encoded, err := json.Marshal(want)
		if err != nil {
			return "", nil, err
		}
		env[key] = encoded
	}
	envData, err := json.Marshal(env)
	if err != nil {
		return "", nil, err
	}
	root["env"] = envData
	updated, err := encodeJSONObject(root)
	if err != nil {
		return "", nil, err
	}
	return updated, changes, nil
}

func currentClaude(content string) map[string]string {
	current := map[string]string{}
	root, err := decodeJSONObject(content)
	if err != nil {
		return current
	}
	raw, ok := root["env"]
	if !ok {
		return current
	}
	env := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &env); err != nil {
		return current
	}
	for key, value := range env {
		if text, ok := rawString(value); ok {
			current["env."+key] = text
		}
	}
	return current
}

// ---- Codex ----

// codexKeyPattern matches a top-level TOML assignment.
func codexKeyPattern(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*(.*)$`)
}

func applyCodex(content string, profile Profile) (string, []Change, error) {
	targets := map[string]string{
		"openai_base_url": profile.codexBaseURL(),
	}
	if profile.Model != "" {
		targets["model"] = profile.Model
	}
	changes := make([]Change, 0, len(targets))
	updated := content
	if strings.TrimSpace(content) == "" {
		updated = ""
	}
	for key, want := range targets {
		matches := codexKeyPattern(key).FindStringSubmatch(updated)
		if matches == nil {
			changes = append(changes, Change{Key: key, Before: "", After: want, Action: "add"})
			continue
		}
		got := strings.Trim(strings.TrimSpace(matches[1]), `"`)
		if got == want {
			changes = append(changes, Change{Key: key, Before: got, After: want, Action: "none"})
			continue
		}
		changes = append(changes, Change{Key: key, Before: got, After: want, Action: "update"})
		updated = codexKeyPattern(key).ReplaceAllString(updated, key+" = "+quoteTOML(want))
	}
	sortChanges(changes)
	// Keys that are new are appended at the end of the file, which keeps the
	// rest of the TOML (and its comments) untouched.
	for key, want := range targets {
		if codexKeyPattern(key).MatchString(updated) {
			continue
		}
		if updated != "" && !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		updated += key + " = " + quoteTOML(want) + "\n"
	}
	return updated, changes, nil
}

func currentCodex(content string) map[string]string {
	current := map[string]string{}
	for _, key := range []string{"openai_base_url", "model"} {
		matches := codexKeyPattern(key).FindStringSubmatch(content)
		if matches == nil {
			continue
		}
		current[key] = strings.Trim(strings.TrimSpace(matches[1]), `"`)
	}
	return current
}

func quoteTOML(value string) string {
	if strings.ContainsAny(value, `"'`) {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return `"` + value + `"`
}

// ---- OpenCode ----

func opencodeTargets(profile Profile) map[string]string {
	name := profile.Name
	if name == "" {
		name = "roambench-gateway"
	}
	targets := map[string]string{
		"provider." + name + ".baseURL": profile.BaseURL,
	}
	if profile.Model != "" {
		targets["provider."+name+".model"] = profile.Model
	}
	return targets
}

func applyOpenCode(content string, profile Profile) (string, []Change, error) {
	root, err := decodeJSONObject(content)
	if err != nil {
		return "", nil, fmt.Errorf("cannot parse opencode config: %w", err)
	}
	providers := map[string]json.RawMessage{}
	if raw, ok := root["provider"]; ok {
		if err := json.Unmarshal(raw, &providers); err != nil {
			return "", nil, fmt.Errorf("cannot parse opencode providers: %w", err)
		}
	}
	targets := opencodeTargets(profile)
	changes := make([]Change, 0, len(targets))
	for key, want := range targets {
		got, ok := lookupOpencode(providers, key)
		switch {
		case ok && got == want:
			changes = append(changes, Change{Key: key, Before: got, After: want, Action: "none"})
		case ok:
			changes = append(changes, Change{Key: key, Before: got, After: want, Action: "update"})
		default:
			changes = append(changes, Change{Key: key, Before: "", After: want, Action: "add"})
		}
	}
	sortChanges(changes)
	for key, want := range targets {
		if err := setOpencode(providers, key, want); err != nil {
			return "", nil, err
		}
	}
	providerData, err := json.Marshal(providers)
	if err != nil {
		return "", nil, err
	}
	root["provider"] = providerData
	updated, err := encodeJSONObject(root)
	if err != nil {
		return "", nil, err
	}
	return updated, changes, nil
}

func currentOpenCode(content string) map[string]string {
	current := map[string]string{}
	root, err := decodeJSONObject(content)
	if err != nil {
		return current
	}
	raw, ok := root["provider"]
	if !ok {
		return current
	}
	providers := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &providers); err != nil {
		return current
	}
	for name, rawProvider := range providers {
		provider := map[string]json.RawMessage{}
		if err := json.Unmarshal(rawProvider, &provider); err != nil {
			continue
		}
		for key, value := range provider {
			if text, ok := rawString(value); ok {
				current["provider."+name+"."+key] = text
			}
		}
	}
	return current
}

// lookupOpencode reads a dotted key out of the opencode providers.
func lookupOpencode(providers map[string]json.RawMessage, key string) (string, bool) {
	parts := strings.Split(key, ".")
	if len(parts) != 3 || parts[0] != "provider" {
		return "", false
	}
	raw, ok := providers[parts[1]]
	if !ok {
		return "", false
	}
	provider := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &provider); err != nil {
		return "", false
	}
	value, ok := provider[parts[2]]
	if !ok {
		return "", false
	}
	return rawString(value)
}

// setOpencode writes a dotted key into the opencode providers.
func setOpencode(providers map[string]json.RawMessage, key, value string) error {
	parts := strings.Split(key, ".")
	if len(parts) != 3 || parts[0] != "provider" {
		return fmt.Errorf("unsupported opencode key: %s", key)
	}
	provider := map[string]json.RawMessage{}
	if raw, ok := providers[parts[1]]; ok {
		if err := json.Unmarshal(raw, &provider); err != nil {
			return err
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	provider[parts[2]] = encoded
	data, err := json.Marshal(provider)
	if err != nil {
		return err
	}
	providers[parts[1]] = data
	return nil
}

// sortChanges keeps the change list stable and readable.
func sortChanges(changes []Change) {
	sort.Slice(changes, func(i, j int) bool { return changes[i].Key < changes[j].Key })
}
