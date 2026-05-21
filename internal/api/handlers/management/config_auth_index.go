package management

import (
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/watcher/synthesizer"
)

type providerAPIKeyEntryWithAuthIndex struct {
	config.ProviderAPIKeyEntry
	AuthIndex string `json:"auth-index,omitempty"`
}

type geminiKeyWithAuthIndex struct {
	Name           string                             `json:"name,omitempty"`
	Priority       int                                `json:"priority,omitempty"`
	Prefix         string                             `json:"prefix,omitempty"`
	BaseURL        string                             `json:"base-url,omitempty"`
	Headers        map[string]string                  `json:"headers,omitempty"`
	Models         []config.GeminiModel               `json:"models,omitempty"`
	ExcludedModels []string                           `json:"excluded-models,omitempty"`
	APIKeyEntries  []providerAPIKeyEntryWithAuthIndex `json:"api-key-entries"`
}

type claudeKeyWithAuthIndex struct {
	Name                     string                             `json:"name,omitempty"`
	Priority                 int                                `json:"priority,omitempty"`
	Prefix                   string                             `json:"prefix,omitempty"`
	BaseURL                  string                             `json:"base-url"`
	Headers                  map[string]string                  `json:"headers,omitempty"`
	Models                   []config.ClaudeModel               `json:"models,omitempty"`
	ExcludedModels           []string                           `json:"excluded-models,omitempty"`
	DisableCooling           bool                               `json:"disable-cooling,omitempty"`
	Cloak                    *config.CloakConfig                `json:"cloak,omitempty"`
	ExperimentalCCHSigning   bool                               `json:"experimental-cch-signing,omitempty"`
	APIKeyEntries            []providerAPIKeyEntryWithAuthIndex `json:"api-key-entries"`
}

type codexKeyWithAuthIndex struct {
	Name           string                             `json:"name,omitempty"`
	Priority       int                                `json:"priority,omitempty"`
	Prefix         string                             `json:"prefix,omitempty"`
	BaseURL        string                             `json:"base-url"`
	Websockets     bool                               `json:"websockets,omitempty"`
	Headers        map[string]string                  `json:"headers,omitempty"`
	Models         []config.CodexModel                `json:"models,omitempty"`
	ExcludedModels []string                           `json:"excluded-models,omitempty"`
	DisableCooling bool                               `json:"disable-cooling,omitempty"`
	APIKeyEntries  []providerAPIKeyEntryWithAuthIndex `json:"api-key-entries"`
}

type vertexCompatKeyWithAuthIndex struct {
	Name           string                             `json:"name,omitempty"`
	Priority       int                                `json:"priority,omitempty"`
	Prefix         string                             `json:"prefix,omitempty"`
	BaseURL        string                             `json:"base-url,omitempty"`
	Headers        map[string]string                  `json:"headers,omitempty"`
	Models         []config.VertexCompatModel         `json:"models,omitempty"`
	ExcludedModels []string                           `json:"excluded-models,omitempty"`
	APIKeyEntries  []providerAPIKeyEntryWithAuthIndex `json:"api-key-entries"`
}

type openAICompatibilityAPIKeyWithAuthIndex struct {
	config.OpenAICompatibilityAPIKey
	AuthIndex string `json:"auth-index,omitempty"`
}

type openAICompatibilityWithAuthIndex struct {
	Name          string                                   `json:"name"`
	Priority      int                                      `json:"priority,omitempty"`
	Disabled      bool                                     `json:"disabled"`
	Prefix        string                                   `json:"prefix,omitempty"`
	BaseURL       string                                   `json:"base-url"`
	APIKeyEntries []openAICompatibilityAPIKeyWithAuthIndex `json:"api-key-entries,omitempty"`
	Models        []config.OpenAICompatibilityModel        `json:"models,omitempty"`
	Headers       map[string]string                        `json:"headers,omitempty"`
	AuthIndex     string                                   `json:"auth-index,omitempty"`
}

func (h *Handler) liveAuthIndexByID() map[string]string {
	out := map[string]string{}
	if h == nil {
		return out
	}
	h.mu.Lock()
	manager := h.authManager
	h.mu.Unlock()
	if manager == nil {
		return out
	}
	for _, auth := range manager.List() {
		if auth == nil {
			continue
		}
		id := strings.TrimSpace(auth.ID)
		if id == "" {
			continue
		}
		idx := strings.TrimSpace(auth.Index)
		if idx == "" {
			idx = auth.EnsureIndex()
		}
		if idx == "" {
			continue
		}
		out[id] = idx
	}
	return out
}

func buildProviderAPIKeyEntriesWithAuthIndex(
	idKind string,
	baseURL string,
	entries []config.ProviderAPIKeyEntry,
	liveIndexByID map[string]string,
	idGen *synthesizer.StableIDGenerator,
) []providerAPIKeyEntryWithAuthIndex {
	if len(entries) == 0 {
		return nil
	}
	out := make([]providerAPIKeyEntryWithAuthIndex, len(entries))
	for j := range entries {
		apiKeyEntry := entries[j]
		id, _ := idGen.Next(idKind, apiKeyEntry.APIKey, baseURL, apiKeyEntry.ProxyURL)
		out[j] = providerAPIKeyEntryWithAuthIndex{
			ProviderAPIKeyEntry: apiKeyEntry,
			AuthIndex:           liveIndexByID[id],
		}
	}
	return out
}

func (h *Handler) geminiKeysWithAuthIndex() []geminiKeyWithAuthIndex {
	if h == nil {
		return nil
	}
	liveIndexByID := h.liveAuthIndexByID()

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil {
		return nil
	}

	idGen := synthesizer.NewStableIDGenerator()
	out := make([]geminiKeyWithAuthIndex, len(h.cfg.GeminiKey))
	for i := range h.cfg.GeminiKey {
		entry := h.cfg.GeminiKey[i]
		out[i] = geminiKeyWithAuthIndex{
			Name:           entry.Name,
			Priority:       entry.Priority,
			Prefix:         entry.Prefix,
			BaseURL:        entry.BaseURL,
			Headers:        entry.Headers,
			Models:         entry.Models,
			ExcludedModels: entry.ExcludedModels,
			APIKeyEntries: buildProviderAPIKeyEntriesWithAuthIndex(
				"gemini:apikey",
				entry.BaseURL,
				entry.APIKeyEntries,
				liveIndexByID,
				idGen,
			),
		}
	}
	return out
}

func (h *Handler) claudeKeysWithAuthIndex() []claudeKeyWithAuthIndex {
	if h == nil {
		return nil
	}
	liveIndexByID := h.liveAuthIndexByID()

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil {
		return nil
	}

	idGen := synthesizer.NewStableIDGenerator()
	out := make([]claudeKeyWithAuthIndex, len(h.cfg.ClaudeKey))
	for i := range h.cfg.ClaudeKey {
		entry := h.cfg.ClaudeKey[i]
		out[i] = claudeKeyWithAuthIndex{
			Name:                   entry.Name,
			Priority:               entry.Priority,
			Prefix:                 entry.Prefix,
			BaseURL:                entry.BaseURL,
			Headers:                entry.Headers,
			Models:                 entry.Models,
			ExcludedModels:         entry.ExcludedModels,
			DisableCooling:         entry.DisableCooling,
			Cloak:                  entry.Cloak,
			ExperimentalCCHSigning: entry.ExperimentalCCHSigning,
			APIKeyEntries: buildProviderAPIKeyEntriesWithAuthIndex(
				"claude:apikey",
				entry.BaseURL,
				entry.APIKeyEntries,
				liveIndexByID,
				idGen,
			),
		}
	}
	return out
}

func (h *Handler) codexKeysWithAuthIndex() []codexKeyWithAuthIndex {
	if h == nil {
		return nil
	}
	liveIndexByID := h.liveAuthIndexByID()

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil {
		return nil
	}

	idGen := synthesizer.NewStableIDGenerator()
	out := make([]codexKeyWithAuthIndex, len(h.cfg.CodexKey))
	for i := range h.cfg.CodexKey {
		entry := h.cfg.CodexKey[i]
		out[i] = codexKeyWithAuthIndex{
			Name:           entry.Name,
			Priority:       entry.Priority,
			Prefix:         entry.Prefix,
			BaseURL:        entry.BaseURL,
			Websockets:     entry.Websockets,
			Headers:        entry.Headers,
			Models:         entry.Models,
			ExcludedModels: entry.ExcludedModels,
			DisableCooling: entry.DisableCooling,
			APIKeyEntries: buildProviderAPIKeyEntriesWithAuthIndex(
				"codex:apikey",
				entry.BaseURL,
				entry.APIKeyEntries,
				liveIndexByID,
				idGen,
			),
		}
	}
	return out
}

func (h *Handler) vertexCompatKeysWithAuthIndex() []vertexCompatKeyWithAuthIndex {
	if h == nil {
		return nil
	}
	liveIndexByID := h.liveAuthIndexByID()

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil {
		return nil
	}

	idGen := synthesizer.NewStableIDGenerator()
	out := make([]vertexCompatKeyWithAuthIndex, len(h.cfg.VertexCompatAPIKey))
	for i := range h.cfg.VertexCompatAPIKey {
		entry := h.cfg.VertexCompatAPIKey[i]
		out[i] = vertexCompatKeyWithAuthIndex{
			Name:           entry.Name,
			Priority:       entry.Priority,
			Prefix:         entry.Prefix,
			BaseURL:        entry.BaseURL,
			Headers:        entry.Headers,
			Models:         entry.Models,
			ExcludedModels: entry.ExcludedModels,
			APIKeyEntries: buildProviderAPIKeyEntriesWithAuthIndex(
				"vertex:apikey",
				entry.BaseURL,
				entry.APIKeyEntries,
				liveIndexByID,
				idGen,
			),
		}
	}
	return out
}

func (h *Handler) openAICompatibilityWithAuthIndex() []openAICompatibilityWithAuthIndex {
	if h == nil {
		return nil
	}
	liveIndexByID := h.liveAuthIndexByID()

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil {
		return nil
	}

	normalized := normalizedOpenAICompatibilityEntries(h.cfg.OpenAICompatibility)
	out := make([]openAICompatibilityWithAuthIndex, len(normalized))
	idGen := synthesizer.NewStableIDGenerator()
	for i := range normalized {
		entry := normalized[i]
		providerName := strings.ToLower(strings.TrimSpace(entry.Name))
		if providerName == "" {
			providerName = "openai-compatibility"
		}
		idKind := fmt.Sprintf("openai-compatibility:%s", providerName)

		response := openAICompatibilityWithAuthIndex{
			Name:      entry.Name,
			Priority:  entry.Priority,
			Disabled:  entry.Disabled,
			Prefix:    entry.Prefix,
			BaseURL:   entry.BaseURL,
			Models:    entry.Models,
			Headers:   entry.Headers,
			AuthIndex: "",
		}
		if len(entry.APIKeyEntries) == 0 {
			id, _ := idGen.Next(idKind, entry.BaseURL)
			response.AuthIndex = liveIndexByID[id]
		} else {
			response.APIKeyEntries = make([]openAICompatibilityAPIKeyWithAuthIndex, len(entry.APIKeyEntries))
			for j := range entry.APIKeyEntries {
				apiKeyEntry := entry.APIKeyEntries[j]
				id, _ := idGen.Next(idKind, apiKeyEntry.APIKey, entry.BaseURL, apiKeyEntry.ProxyURL)
				response.APIKeyEntries[j] = openAICompatibilityAPIKeyWithAuthIndex{
					OpenAICompatibilityAPIKey: apiKeyEntry,
					AuthIndex:                 liveIndexByID[id],
				}
			}
		}
		out[i] = response
	}
	return out
}
