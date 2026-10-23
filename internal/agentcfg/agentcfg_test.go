package agentcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func testProfile() Profile {
	return Profile{
		Name:      "magpie",
		BaseURL:   "http://127.0.0.1:3425",
		AuthToken: "magpie",
		Model:     "auto/glm-5",
		CodexPath: "/backend-api/codex",
	}
}

func TestPlanClaudeAddsSettings(t *testing.T) {
	home := t.TempDir()
	plan, err := PlanFor("claude", home, testProfile())
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if plan.Path != filepath.Join(home, ".claude", "settings.json") {
		t.Errorf("path = %q", plan.Path)
	}
	if !plan.Changed() {
		t.Fatal("expected changes for an empty config")
	}
	found := false
	for _, change := range plan.Changes {
		if change.Key == "env.ANTHROPIC_BASE_URL" && change.After == "http://127.0.0.1:3425" && change.Action == "add" {
			found = true
		}
	}
	if !found {
		t.Fatalf("base url change missing: %+v", plan.Changes)
	}
}

func TestApplyClaudeWritesAndBacksUp(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	original := `{"env":{"ANTHROPIC_BASE_URL":"https://api.anthropic.com"},"other":1}`
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Apply("claude", home, testProfile())
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !result.Changed || result.BackupPath == "" {
		t.Fatalf("result = %+v, want a change with a backup", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "http://127.0.0.1:3425") {
		t.Fatalf("base url not written:\n%s", content)
	}
	if !strings.Contains(content, `"other": 1`) {
		t.Fatalf("unrelated settings dropped:\n%s", content)
	}
	backup, err := os.ReadFile(result.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != original {
		t.Fatalf("backup = %q, want %q", backup, original)
	}

	// Applying again is a no-op.
	second, err := Apply("claude", home, testProfile())
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed {
		t.Fatal("second apply should not change anything")
	}
}

func TestApplyCodexKeepsCommentsAndOtherKeys(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	original := `# 我的 codex 配置
model = "gpt-6-astra"
openai_base_url = "http://127.0.0.1:3425/backend-api/codex"

[tui]
screen_reader_detection_done = true
`
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Apply("codex", home, Profile{BaseURL: "http://127.0.0.1:3425", CodexPath: "/backend-api/codex", Model: "auto/glm-5"})
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected a change")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "# 我的 codex 配置") {
		t.Fatalf("comment lost:\n%s", content)
	}
	if !strings.Contains(content, "[tui]") || !strings.Contains(content, "screen_reader_detection_done = true") {
		t.Fatalf("other sections lost:\n%s", content)
	}
	if !strings.Contains(content, `openai_base_url = "http://127.0.0.1:3425/backend-api/codex"`) {
		t.Fatalf("base url not written:\n%s", content)
	}
	if !strings.Contains(content, `model = "auto/glm-5"`) {
		t.Fatalf("model not written:\n%s", content)
	}
}

func TestApplyCodexAddsMissingKeys(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[tui]\nfoo = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply("codex", home, Profile{BaseURL: "http://127.0.0.1:3425", CodexPath: "/backend-api/codex"}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "openai_base_url") {
		t.Fatalf("key not added:\n%s", data)
	}
}

func TestApplyOpenCodeWritesProvider(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	result, err := Apply("opencode", home, Profile{Name: "magpie", BaseURL: "http://127.0.0.1:3425", Model: "auto/glm-5"})
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected a change")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "http://127.0.0.1:3425") {
		t.Fatalf("base url missing:\n%s", data)
	}
}

func TestPlanReportsNoChangeWhenAlreadyConfigured(t *testing.T) {
	home := t.TempDir()
	if _, err := Apply("claude", home, testProfile()); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanFor("claude", home, testProfile())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changed() {
		t.Fatalf("plan should be empty: %+v", plan.Changes)
	}
	if plan.Note == "" {
		t.Error("expected a note explaining there is nothing to do")
	}
}

func TestUnsupportedAgent(t *testing.T) {
	if _, err := PlanFor("kiro-cli", t.TempDir(), testProfile()); err == nil {
		t.Fatal("expected an error for an unsupported agent")
	}
	if Supports("kiro-cli") {
		t.Error("kiro-cli should not be supported")
	}
	if !Supports("claude") || !Supports("codex") || !Supports("opencode") {
		t.Error("expected claude, codex and opencode to be supported")
	}
}

func TestGuideIsDeterministic(t *testing.T) {
	steps := Guide("claude", testProfile())
	if len(steps) == 0 {
		t.Fatal("expected steps")
	}
	found := false
	for _, step := range steps {
		if step.Command == "claude" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the login step: %+v", steps)
	}
	if len(Guide("unknown-agent", testProfile())) == 0 {
		t.Fatal("expected a fallback step for an unknown agent")
	}
}

func TestCurrentForShowsOnlyRelevantKeys(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := `{"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:3425","CLAUDE_CODE_MAX_CONTEXT_TOKENS":"1048576"},"theme":"dark"}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	current := CurrentFor("claude", home, Profile{BaseURL: "http://127.0.0.1:3425", AuthToken: "magpie", Model: "auto/glm-5"})
	if _, ok := current["env.ANTHROPIC_BASE_URL"]; !ok {
		t.Fatalf("base url missing: %+v", current)
	}
	if _, ok := current["env.CLAUDE_CODE_MAX_CONTEXT_TOKENS"]; ok {
		t.Fatalf("unrelated key leaked into the wizard view: %+v", current)
	}
	if len(current) != 1 {
		t.Fatalf("expected only the base url, got %+v", current)
	}
}

// The hermes fixture mirrors the shape of a real ~/.hermes/config.yaml.
const hermesFixture = `model:
  default: glm-5.3-flash
  provider: opencode-console
providers:
  opencode-console:
    name: OpenCode Console
    base_url: https://opencode.ai/inference/openai/v1
    api_mode: chat_completions
    key_cmd: opencode auth token
  bigmodel-anthropic:
    base_url: https://open.bigmodel.cn/api/anthropic
    api_mode: anthropic_messages
    key_env: BIGMODEL_KEY
`

func TestApplyHermesAddsGatewayProviderAndSwitches(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".hermes", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(hermesFixture), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Apply("hermes", home, testProfile())
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected a change")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("result is not valid YAML: %v\n%s", err, content)
	}
	model := cfg["model"].(map[string]interface{})
	if model["provider"] != "magpie" {
		t.Errorf("model.provider = %v, want magpie", model["provider"])
	}
	if model["default"] != "auto/glm-5" {
		t.Errorf("model.default = %v, want auto/glm-5", model["default"])
	}
	providers := cfg["providers"].(map[string]interface{})
	gateway := providers["magpie"].(map[string]interface{})
	if gateway["base_url"] != "http://127.0.0.1:3425" {
		t.Errorf("base_url = %v", gateway["base_url"])
	}
	if gateway["api_mode"] != "anthropic_messages" {
		t.Errorf("api_mode = %v", gateway["api_mode"])
	}
	if gateway["key_cmd"] != "echo magpie" {
		t.Errorf("key_cmd = %v", gateway["key_cmd"])
	}
	if gateway["discover_models"] != true {
		t.Errorf("discover_models = %v, want true", gateway["discover_models"])
	}
	// The untouched provider survives.
	if _, ok := providers["opencode-console"]; !ok {
		t.Errorf("existing provider lost:\n%s", content)
	}
	if !strings.Contains(content, "https://opencode.ai/inference/openai/v1") {
		t.Errorf("existing provider base_url lost:\n%s", content)
	}

	// Second apply is a no-op.
	second, err := Apply("hermes", home, testProfile())
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed {
		t.Fatal("second apply should not change anything")
	}
}

func TestApplyHermesCreatesConfigFromNothing(t *testing.T) {
	home := t.TempDir()
	if _, err := Apply("hermes", home, Profile{Name: "magpie", BaseURL: "http://127.0.0.1:3425"}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".hermes", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid YAML: %v", err)
	}
	if cfg["model"].(map[string]interface{})["provider"] != "magpie" {
		t.Errorf("model.provider not set:\n%s", data)
	}
}

func TestApplyHermesSwitchesBackForthWithoutLosingProviders(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".hermes", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(hermesFixture), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply("hermes", home, testProfile()); err != nil {
		t.Fatal(err)
	}
	// A different gateway profile replaces the first provider entry.
	other := testProfile()
	other.Name = "other-gw"
	if _, err := Apply("hermes", home, other); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	providers := cfg["providers"].(map[string]interface{})
	for _, want := range []string{"opencode-console", "bigmodel-anthropic", "magpie", "other-gw"} {
		if _, ok := providers[want]; !ok {
			t.Errorf("provider %q lost", want)
		}
	}
	if cfg["model"].(map[string]interface{})["provider"] != "other-gw" {
		t.Errorf("model.provider not switched to other-gw")
	}
}

func TestGatewayUnsupportedKiroOnly(t *testing.T) {
	if !GatewayUnsupported("kiro-cli") {
		t.Error("kiro-cli should be gateway-unsupported")
	}
	for _, label := range Adapters() {
		if GatewayUnsupported(label) {
			t.Errorf("adapter %s must not be gateway-unsupported", label)
		}
	}
}
