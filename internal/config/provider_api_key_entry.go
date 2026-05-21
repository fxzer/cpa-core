package config

import "strings"

// ProviderAPIKeyEntry is an API key with optional per-key proxy override.
type ProviderAPIKeyEntry struct {
	APIKey   string `yaml:"api-key" json:"api-key"`
	ProxyURL string `yaml:"proxy-url,omitempty" json:"proxy-url,omitempty"`
}

// NormalizeProviderAPIKeyEntries trims and removes empty API key entries.
func NormalizeProviderAPIKeyEntries(entries []ProviderAPIKeyEntry) []ProviderAPIKeyEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]ProviderAPIKeyEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for i := range entries {
		key := strings.TrimSpace(entries[i].APIKey)
		if key == "" {
			continue
		}
		proxyURL := strings.TrimSpace(entries[i].ProxyURL)
		unique := key + "|" + proxyURL
		if _, exists := seen[unique]; exists {
			continue
		}
		seen[unique] = struct{}{}
		out = append(out, ProviderAPIKeyEntry{
			APIKey:   key,
			ProxyURL: proxyURL,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ProviderAPIKeyEntriesContain reports whether any entry matches the given API key.
func ProviderAPIKeyEntriesContain(entries []ProviderAPIKeyEntry, apiKey string) bool {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return false
	}
	for i := range entries {
		if strings.EqualFold(strings.TrimSpace(entries[i].APIKey), apiKey) {
			return true
		}
	}
	return false
}

// ProviderAPIKeyEntriesEqual reports whether two entry lists are equivalent.
func ProviderAPIKeyEntriesEqual(left, right []ProviderAPIKeyEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if strings.TrimSpace(left[i].APIKey) != strings.TrimSpace(right[i].APIKey) {
			return false
		}
		if strings.TrimSpace(left[i].ProxyURL) != strings.TrimSpace(right[i].ProxyURL) {
			return false
		}
	}
	return true
}

// ProviderAPIKeyEntryProxyURL returns the per-key proxy URL for the matching API key.
func ProviderAPIKeyEntryProxyURL(entries []ProviderAPIKeyEntry, apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	for i := range entries {
		if apiKey != "" && strings.EqualFold(strings.TrimSpace(entries[i].APIKey), apiKey) {
			return strings.TrimSpace(entries[i].ProxyURL)
		}
	}
	if len(entries) > 0 {
		return strings.TrimSpace(entries[0].ProxyURL)
	}
	return ""
}

// FirstProviderAPIKey returns the first non-empty API key in entries.
func FirstProviderAPIKey(entries []ProviderAPIKeyEntry) string {
	for i := range entries {
		if key := strings.TrimSpace(entries[i].APIKey); key != "" {
			return key
		}
	}
	return ""
}
