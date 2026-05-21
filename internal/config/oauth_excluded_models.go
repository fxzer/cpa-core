package config

import (
	"encoding/json"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// OAuthExcludedProviderModels stores disabled models for one OAuth provider channel.
// Only explicit true values are persisted; absent keys mean not disabled.
type OAuthExcludedProviderModels map[string]bool

// OAuthExcludedModelsConfig stores global OAuth model disablement per provider channel.
type OAuthExcludedModelsConfig map[string]OAuthExcludedProviderModels

func parseOAuthExcludedProviderModels(raw any) OAuthExcludedProviderModels {
	if raw == nil {
		return nil
	}

	switch value := raw.(type) {
	case []string:
		return oauthExcludedProviderModelsFromList(value)
	case []any:
		list := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok {
				list = append(list, s)
			}
		}
		return oauthExcludedProviderModelsFromList(list)
	case map[string]bool:
		return normalizeOAuthExcludedProviderModels(value)
	case map[string]any:
		out := make(OAuthExcludedProviderModels, len(value))
		for key, item := range value {
			if disabled, ok := item.(bool); ok && disabled {
				if trimmed := strings.TrimSpace(key); trimmed != "" {
					out[trimmed] = true
				}
			}
		}
		return normalizeOAuthExcludedProviderModels(out)
	case map[any]any:
		out := make(OAuthExcludedProviderModels, len(value))
		for key, item := range value {
			keyStr, _ := key.(string)
			disabled, _ := item.(bool)
			if disabled {
				if trimmed := strings.TrimSpace(keyStr); trimmed != "" {
					out[trimmed] = true
				}
			}
		}
		return normalizeOAuthExcludedProviderModels(out)
	default:
		return nil
	}
}

func oauthExcludedProviderModelsFromList(list []string) OAuthExcludedProviderModels {
	if len(list) == 0 {
		return nil
	}
	out := make(OAuthExcludedProviderModels, len(list))
	for _, entry := range list {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			out[trimmed] = true
		}
	}
	return normalizeOAuthExcludedProviderModels(out)
}

func normalizeOAuthExcludedProviderModels(models OAuthExcludedProviderModels) OAuthExcludedProviderModels {
	if len(models) == 0 {
		return nil
	}
	out := make(OAuthExcludedProviderModels, len(models))
	for name, disabled := range models {
		if !disabled {
			continue
		}
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		out[trimmed] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NormalizeOAuthExcludedModelsConfig normalizes provider keys and keeps only disabled=true entries.
func NormalizeOAuthExcludedModelsConfig(entries OAuthExcludedModelsConfig) OAuthExcludedModelsConfig {
	if len(entries) == 0 {
		return nil
	}
	out := make(OAuthExcludedModelsConfig, len(entries))
	for provider, models := range entries {
		key := strings.ToLower(strings.TrimSpace(provider))
		if key == "" {
			continue
		}
		normalized := normalizeOAuthExcludedProviderModels(models)
		if len(normalized) == 0 {
			continue
		}
		out[key] = normalized
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// OAuthExcludedModelsConfigFromLegacy converts legacy provider -> []string config to bool maps.
func OAuthExcludedModelsConfigFromLegacy(entries map[string][]string) OAuthExcludedModelsConfig {
	if len(entries) == 0 {
		return nil
	}
	out := make(OAuthExcludedModelsConfig, len(entries))
	for provider, models := range entries {
		key := strings.ToLower(strings.TrimSpace(provider))
		if key == "" {
			continue
		}
		normalized := NormalizeExcludedModels(models)
		providerModels := oauthExcludedProviderModelsFromList(normalized)
		if len(providerModels) == 0 {
			continue
		}
		out[key] = providerModels
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Patterns returns disabled model IDs/patterns for wildcard matching.
func (entries OAuthExcludedModelsConfig) Patterns(provider string) []string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || len(entries) == 0 {
		return nil
	}
	models := entries[provider]
	if len(models) == 0 {
		return nil
	}
	out := make([]string, 0, len(models))
	for name, disabled := range models {
		if !disabled {
			continue
		}
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	sort.Strings(out)
	return out
}

// IsModelDisabled reports whether a model ID matches a disabled entry or wildcard pattern.
func (entries OAuthExcludedModelsConfig) IsModelDisabled(provider, modelID string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	if provider == "" || modelID == "" || len(entries) == 0 {
		return false
	}
	models := entries[provider]
	if len(models) == 0 {
		return false
	}
	for pattern, disabled := range models {
		if !disabled {
			continue
		}
		if matchOAuthExcludedPattern(strings.ToLower(strings.TrimSpace(pattern)), modelID) {
			return true
		}
	}
	return false
}

func matchOAuthExcludedPattern(pattern, modelID string) bool {
	if pattern == "" || modelID == "" {
		return false
	}
	if !strings.Contains(pattern, "*") {
		return pattern == modelID
	}
	if pattern == "*" {
		return true
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(modelID, parts[0]) {
		return false
	}
	offset := len(parts[0])
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		idx := strings.Index(modelID[offset:], part)
		if idx < 0 {
			return false
		}
		offset += idx + len(part)
	}
	return true
}

func (entries *OAuthExcludedModelsConfig) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		*entries = nil
		return nil
	}
	var raw map[string]any
	if err := value.Decode(&raw); err != nil {
		return err
	}
	out := make(OAuthExcludedModelsConfig, len(raw))
	for provider, models := range raw {
		key := strings.ToLower(strings.TrimSpace(provider))
		if key == "" {
			continue
		}
		parsed := parseOAuthExcludedProviderModels(models)
		if len(parsed) == 0 {
			continue
		}
		out[key] = parsed
	}
	*entries = NormalizeOAuthExcludedModelsConfig(out)
	return nil
}

func (entries OAuthExcludedModelsConfig) MarshalYAML() (any, error) {
	normalized := NormalizeOAuthExcludedModelsConfig(entries)
	if len(normalized) == 0 {
		return nil, nil
	}
	// Avoid re-entering MarshalYAML when the encoder walks the returned value.
	type plain OAuthExcludedModelsConfig
	return plain(normalized), nil
}

func (entries *OAuthExcludedModelsConfig) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*entries = nil
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(OAuthExcludedModelsConfig, len(raw))
	for provider, modelsRaw := range raw {
		key := strings.ToLower(strings.TrimSpace(provider))
		if key == "" {
			continue
		}
		var asList []string
		if err := json.Unmarshal(modelsRaw, &asList); err == nil {
			if parsed := oauthExcludedProviderModelsFromList(asList); len(parsed) > 0 {
				out[key] = parsed
			}
			continue
		}
		var asMap map[string]bool
		if err := json.Unmarshal(modelsRaw, &asMap); err == nil {
			if parsed := normalizeOAuthExcludedProviderModels(asMap); len(parsed) > 0 {
				out[key] = parsed
			}
		}
	}
	*entries = NormalizeOAuthExcludedModelsConfig(out)
	return nil
}

func (entries OAuthExcludedModelsConfig) MarshalJSON() ([]byte, error) {
	normalized := NormalizeOAuthExcludedModelsConfig(entries)
	if len(normalized) == 0 {
		return []byte("null"), nil
	}
	// Avoid re-entering MarshalJSON when encoding the normalized map.
	type plain OAuthExcludedModelsConfig
	return json.Marshal(plain(normalized))
}
