package protocolfixture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type fixture struct {
	FixtureVersion          int              `json:"fixtureVersion"`
	Vendor                  string           `json:"vendor"`
	Protocol                string           `json:"protocol"`
	MinimumSupportedVersion string           `json:"minimumSupportedVersion"`
	CapturedVersion         string           `json:"capturedVersion"`
	SourceURL               string           `json:"sourceUrl"`
	ValidationLevel         string           `json:"validationLevel"`
	Messages                []fixtureMessage `json:"messages"`
}

type fixtureMessage struct {
	Direction     string          `json:"direction"`
	Kind          string          `json:"kind"`
	Transport     string          `json:"transport"`
	CorrelationID string          `json:"correlationId"`
	Payload       json.RawMessage `json:"payload"`
}

func TestApprovalFixturesAreReplayable(t *testing.T) {
	tests := []struct {
		vendor   string
		validate func(*testing.T, fixture)
	}{
		{vendor: "codex", validate: validateCodex},
		{vendor: "claude", validate: validateClaude},
		{vendor: "opencode", validate: validateOpenCode},
		{vendor: "kimi", validate: validateKimi},
	}

	for _, test := range tests {
		t.Run(test.vendor, func(t *testing.T) {
			path := filepath.Join("testdata", test.vendor, "approval.json")
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture %s: %v", path, err)
			}

			var got fixture
			if err := json.Unmarshal(payload, &got); err != nil {
				t.Fatalf("decode fixture %s: %v", path, err)
			}
			if got.FixtureVersion != 1 {
				t.Fatalf("fixtureVersion = %d, want 1", got.FixtureVersion)
			}
			if got.Vendor != test.vendor {
				t.Fatalf("vendor = %q, want %q", got.Vendor, test.vendor)
			}
			if got.Protocol == "" || got.MinimumSupportedVersion == "" || got.CapturedVersion == "" || got.SourceURL == "" || got.ValidationLevel == "" {
				t.Fatalf("fixture metadata is incomplete: %#v", got)
			}
			if len(got.Messages) < 2 {
				t.Fatalf("messages = %d, want at least request and response", len(got.Messages))
			}
			request, response := got.Messages[0], got.Messages[1]
			if request.Direction != "vendor_to_adapter" || request.Kind != "request" {
				t.Fatalf("first message = %#v, want vendor request", request)
			}
			if response.Direction != "adapter_to_vendor" || response.Kind != "response" {
				t.Fatalf("second message = %#v, want adapter response", response)
			}
			if request.CorrelationID == "" || request.CorrelationID != response.CorrelationID {
				t.Fatalf("request/response correlation IDs = %q/%q", request.CorrelationID, response.CorrelationID)
			}
			if request.Transport == "" || response.Transport == "" || !json.Valid(request.Payload) || !json.Valid(response.Payload) {
				t.Fatal("fixture messages must have transports and valid JSON payloads")
			}

			test.validate(t, got)
		})
	}
}

func decodeObject(t *testing.T, payload json.RawMessage) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return object
}

func nestedObject(t *testing.T, object map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := object[key].(map[string]any)
	if !ok {
		t.Fatalf("%q = %#v, want object", key, object[key])
	}
	return value
}

func validateCodex(t *testing.T, got fixture) {
	request := decodeObject(t, got.Messages[0].Payload)
	response := decodeObject(t, got.Messages[1].Payload)
	if request["method"] != "item/commandExecution/requestApproval" {
		t.Fatalf("Codex method = %#v", request["method"])
	}
	if nestedObject(t, response, "result")["decision"] != "accept" {
		t.Fatalf("Codex response = %#v", response)
	}
}

func validateClaude(t *testing.T, got fixture) {
	request := decodeObject(t, got.Messages[0].Payload)
	response := decodeObject(t, got.Messages[1].Payload)
	if request["hook_event_name"] != "PermissionRequest" {
		t.Fatalf("Claude hook event = %#v", request["hook_event_name"])
	}
	hookOutput := nestedObject(t, response, "hookSpecificOutput")
	if hookOutput["hookEventName"] != "PermissionRequest" || nestedObject(t, hookOutput, "decision")["behavior"] != "allow" {
		t.Fatalf("Claude response = %#v", response)
	}
}

func validateOpenCode(t *testing.T, got fixture) {
	request := decodeObject(t, got.Messages[0].Payload)
	response := decodeObject(t, got.Messages[1].Payload)
	if request["type"] != "permission.asked" {
		t.Fatalf("OpenCode event type = %#v", request["type"])
	}
	if response["method"] != "POST" || nestedObject(t, response, "body")["reply"] != "once" {
		t.Fatalf("OpenCode response = %#v", response)
	}
}

func validateKimi(t *testing.T, got fixture) {
	request := decodeObject(t, got.Messages[0].Payload)
	response := decodeObject(t, got.Messages[1].Payload)
	if request["method"] != "request" || nestedObject(t, request, "params")["type"] != "ApprovalRequest" {
		t.Fatalf("Kimi request = %#v", request)
	}
	if nestedObject(t, response, "result")["response"] != "approve" {
		t.Fatalf("Kimi response = %#v", response)
	}
}
