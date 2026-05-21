package config

import "strings"

// ProviderKeyGroupMatchesAPIKey reports whether a credential group matches the auth API key attribute.
func ProviderKeyGroupMatchesAPIKey(entries []ProviderAPIKeyEntry, baseURL, attrKey, attrBase string) bool {
	attrKey = strings.TrimSpace(attrKey)
	attrBase = strings.TrimSpace(attrBase)
	baseURL = strings.TrimSpace(baseURL)
	if attrKey == "" {
		if attrBase == "" {
			return false
		}
		return strings.EqualFold(baseURL, attrBase)
	}
	if !ProviderAPIKeyEntriesContain(entries, attrKey) {
		return false
	}
	if attrBase == "" || baseURL == "" {
		return true
	}
	return strings.EqualFold(baseURL, attrBase)
}
