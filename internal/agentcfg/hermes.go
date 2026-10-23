package agentcfg

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Hermes keeps its model access in ~/.hermes/config.yaml:
//
//	model:
//	  default: <model id>
//	  provider: <provider key>
//	providers:
//	  <provider key>:
//	    base_url: https://...
//	    api_mode: chat_completions | anthropic_messages | codex_responses
//	    key_cmd: <command that prints the credential>
//
// The adapter adds one provider entry for the gateway and switches
// model.provider to it. The edit is surgical on a yaml.Node, so every other
// provider, model entry and comment survives untouched.

// hermesAPIMode maps the profile protocol onto hermes' api_mode values.
func hermesAPIMode(profile Profile) string {
	if profile.Protocol() == "codex" {
		return "codex_responses"
	}
	return "anthropic_messages"
}

func hermesProviderKey(profile Profile) string {
	key := profile.Name
	if key == "" {
		key = "roambench-gateway"
	}
	return key
}

// applyHermes writes the profile into the hermes config content.
func applyHermes(content string, profile Profile) (string, []Change, error) {
	root, doc := ensureYAMLMapping(content)

	changes := make([]Change, 0, 6)

	providers := yamlMappingChild(root, "providers")
	if providers == nil {
		providers = &yaml.Node{Kind: yaml.MappingNode}
		appendYAMLPair(root, "providers", providers)
	}
	key := hermesProviderKey(profile)
	provider := yamlMappingChild(providers, key)
	if provider == nil {
		provider = &yaml.Node{Kind: yaml.MappingNode}
		appendYAMLPair(providers, key, provider)
	}

	providerTargets := map[string]string{
		"base_url": profile.BaseURL,
		"api_mode": hermesAPIMode(profile),
	}
	if profile.AuthToken != "" {
		// The gateway credential is a placeholder; hermes runs key_cmd to
		// obtain it, which keeps the env plumbing out of the way.
		providerTargets["key_cmd"] = "echo " + profile.AuthToken
	}
	providerTargets["discover_models"] = "true"
	for _, field := range sortedYAMLFields(providerTargets) {
		want := providerTargets[field]
		changes = append(changes, setYAMLPair(provider, field, want, "providers."+key+"."+field))
	}

	model := yamlMappingChild(root, "model")
	if model == nil {
		model = &yaml.Node{Kind: yaml.MappingNode}
		appendYAMLPair(root, "model", model)
	}
	changes = append(changes, setYAMLPair(model, "provider", key, "model.provider"))
	if profile.Model != "" {
		changes = append(changes, setYAMLPair(model, "default", profile.Model, "model.default"))
	}

	sortChanges(changes)
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return "", nil, err
	}
	if err := encoder.Close(); err != nil {
		return "", nil, err
	}
	return out.String(), changes, nil
}

func currentHermes(content string) map[string]string {
	current := map[string]string{}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return current
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return current
	}
	root := doc.Content[0]
	if model := yamlMappingChild(root, "model"); model != nil {
		if node := yamlMappingChild(model, "provider"); node != nil {
			current["model.provider"] = node.Value
		}
		if node := yamlMappingChild(model, "default"); node != nil {
			current["model.default"] = node.Value
		}
	}
	if providers := yamlMappingChild(root, "providers"); providers != nil && providers.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(providers.Content); i += 2 {
			name := providers.Content[i].Value
			entry := providers.Content[i+1]
			if entry.Kind != yaml.MappingNode {
				continue
			}
			if node := yamlMappingChild(entry, "base_url"); node != nil {
				current["providers."+name+".base_url"] = node.Value
			}
		}
	}
	return current
}

// ensureYAMLMapping parses content into a document with a mapping root,
// creating an empty one when the file is absent or empty.
func ensureYAMLMapping(content string) (*yaml.Node, *yaml.Node) {
	doc := &yaml.Node{Kind: yaml.DocumentNode}
	if strings.TrimSpace(content) != "" {
		var parsed yaml.Node
		if err := yaml.Unmarshal([]byte(content), &parsed); err == nil && len(parsed.Content) > 0 {
			doc = &parsed
		}
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		root = &yaml.Node{Kind: yaml.MappingNode}
		doc.Content = []*yaml.Node{root}
	}
	return root, doc
}

// yamlMappingChild returns the value node of a mapping key, or nil.
func yamlMappingChild(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// appendYAMLPair appends a key/value pair to a mapping node.
func appendYAMLPair(mapping *yaml.Node, key string, value *yaml.Node) {
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}

// setYAMLPair sets a scalar pair on a mapping and reports the key-level
// change. Booleans are detected by their literal form.
func setYAMLPair(mapping *yaml.Node, key, value, reportKey string) Change {
	tag := "!!str"
	if value == "true" || value == "false" {
		tag = "!!bool"
	}
	existing := yamlMappingChild(mapping, key)
	if existing != nil && existing.Kind == yaml.ScalarNode && existing.Value == value {
		return Change{Key: reportKey, Before: value, After: value, Action: "none"}
	}
	before := ""
	if existing != nil && existing.Kind == yaml.ScalarNode {
		before = existing.Value
	}
	action := "update"
	if existing == nil {
		action = "add"
		appendYAMLPair(mapping, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value})
	} else {
		existing.Value = value
		existing.Tag = tag
		existing.Style = 0
	}
	return Change{Key: reportKey, Before: before, After: value, Action: action}
}

// sortedYAMLFields returns the map keys in a stable order.
func sortedYAMLFields(targets map[string]string) []string {
	fields := make([]string, 0, len(targets))
	for field := range targets {
		fields = append(fields, field)
	}
	for i := 1; i < len(fields); i++ {
		for j := i; j > 0 && fields[j] < fields[j-1]; j-- {
			fields[j], fields[j-1] = fields[j-1], fields[j]
		}
	}
	return fields
}

// GatewayUnsupported reports agents whose CLI has no configuration surface
// for a custom model endpoint, so no adapter can ever point them at a
// gateway. They still work as workers; their models come from their own
// login.
func GatewayUnsupported(label string) bool {
	return strings.TrimSpace(label) == "kiro-cli"
}

var _ = fmt.Sprintf
