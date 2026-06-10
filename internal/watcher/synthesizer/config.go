package synthesizer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fxzer/cpa-core/v7/internal/config"
	"github.com/fxzer/cpa-core/v7/internal/watcher/diff"
	coreauth "github.com/fxzer/cpa-core/v7/sdk/cliproxy/auth"
)

func providerKeyLabel(prefix, name, fallback string) string {
	if label := config.ProviderDisplayName(prefix, name); label != "" {
		return label
	}
	return fallback
}

func providerNameAttr(name string) map[string]string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	return map[string]string{"provider_name": trimmed}
}

// ConfigSynthesizer generates Auth entries from configuration API keys.
// It handles Gemini, Claude, Codex, OpenAI-compat, and Vertex-compat providers.
type ConfigSynthesizer struct{}

// NewConfigSynthesizer creates a new ConfigSynthesizer instance.
func NewConfigSynthesizer() *ConfigSynthesizer {
	return &ConfigSynthesizer{}
}

// Synthesize generates Auth entries from config API keys.
func (s *ConfigSynthesizer) Synthesize(ctx *SynthesisContext) ([]*coreauth.Auth, error) {
	out := make([]*coreauth.Auth, 0, 32)
	if ctx == nil || ctx.Config == nil {
		return out, nil
	}

	// Gemini API Keys
	out = append(out, s.synthesizeGeminiKeys(ctx)...)
	// Claude API Keys
	out = append(out, s.synthesizeClaudeKeys(ctx)...)
	// Codex API Keys
	out = append(out, s.synthesizeCodexKeys(ctx)...)
	// OpenAI-compat
	out = append(out, s.synthesizeOpenAICompat(ctx)...)
	// Vertex-compat
	out = append(out, s.synthesizeVertexCompat(ctx)...)

	return out, nil
}

// synthesizeGeminiKeys creates Auth entries for Gemini API keys.
func (s *ConfigSynthesizer) synthesizeGeminiKeys(ctx *SynthesisContext) []*coreauth.Auth {
	cfg := ctx.Config
	out := make([]*coreauth.Auth, 0, len(cfg.GeminiKey))
	for i := range cfg.GeminiKey {
		entry := cfg.GeminiKey[i]
		out = append(out, synthesizeProviderKeyAuths(ctx, entry.APIKeyEntries, providerKeyAuthParams{
			Provider:       "gemini",
			Label:          providerKeyLabel(entry.Prefix, entry.Name, "gemini-apikey"),
			IDKind:         "gemini:apikey",
			SourcePrefix:   "config:gemini",
			Prefix:         strings.TrimSpace(entry.Prefix),
			BaseURL:        strings.TrimSpace(entry.BaseURL),
			Priority:       entry.Priority,
			DisableCooling: entry.DisableCooling,
			Headers:        entry.Headers,
			ExcludedModels: entry.ExcludedModels,
			ModelsHash:     diff.ComputeGeminiModelsHash(entry.Models),
			ExtraAttrs:     providerNameAttr(entry.Name),
		})...)
	}
	return out
}

// synthesizeClaudeKeys creates Auth entries for Claude API keys.
func (s *ConfigSynthesizer) synthesizeClaudeKeys(ctx *SynthesisContext) []*coreauth.Auth {
	cfg := ctx.Config
	out := make([]*coreauth.Auth, 0, len(cfg.ClaudeKey))
	for i := range cfg.ClaudeKey {
		ck := cfg.ClaudeKey[i]
		out = append(out, synthesizeProviderKeyAuths(ctx, ck.APIKeyEntries, providerKeyAuthParams{
			Provider:       "claude",
			Label:          providerKeyLabel(ck.Prefix, ck.Name, "claude-apikey"),
			IDKind:         "claude:apikey",
			SourcePrefix:   "config:claude",
			Prefix:         strings.TrimSpace(ck.Prefix),
			BaseURL:        strings.TrimSpace(ck.BaseURL),
			Priority:       ck.Priority,
			DisableCooling: ck.DisableCooling,
			Headers:        ck.Headers,
			ExcludedModels: ck.ExcludedModels,
			ModelsHash:     diff.ComputeClaudeModelsHash(ck.Models),
			ExtraAttrs:     providerNameAttr(ck.Name),
		})...)
	}
	return out
}

// synthesizeCodexKeys creates Auth entries for Codex API keys.
func (s *ConfigSynthesizer) synthesizeCodexKeys(ctx *SynthesisContext) []*coreauth.Auth {
	cfg := ctx.Config
	out := make([]*coreauth.Auth, 0, len(cfg.CodexKey))
	for i := range cfg.CodexKey {
		ck := cfg.CodexKey[i]
		extra := map[string]string{}
		if ck.Websockets {
			extra["websockets"] = "true"
		}
		for k, v := range providerNameAttr(ck.Name) {
			extra[k] = v
		}
		out = append(out, synthesizeProviderKeyAuths(ctx, ck.APIKeyEntries, providerKeyAuthParams{
			Provider:       "codex",
			Label:          providerKeyLabel(ck.Prefix, ck.Name, "codex-apikey"),
			IDKind:         "codex:apikey",
			SourcePrefix:   "config:codex",
			Prefix:         strings.TrimSpace(ck.Prefix),
			BaseURL:        strings.TrimSpace(ck.BaseURL),
			Priority:       ck.Priority,
			DisableCooling: ck.DisableCooling,
			Headers:        ck.Headers,
			ExcludedModels: ck.ExcludedModels,
			ModelsHash:     diff.ComputeCodexModelsHash(ck.Models),
			ExtraAttrs:     extra,
		})...)
	}
	return out
}

// synthesizeOpenAICompat creates Auth entries for OpenAI-compatible providers.
func (s *ConfigSynthesizer) synthesizeOpenAICompat(ctx *SynthesisContext) []*coreauth.Auth {
	cfg := ctx.Config
	now := ctx.Now
	idGen := ctx.IDGenerator

	out := make([]*coreauth.Auth, 0)
	for i := range cfg.OpenAICompatibility {
		compat := &cfg.OpenAICompatibility[i]
		if compat.Disabled {
			continue
		}
		prefix := strings.TrimSpace(compat.Prefix)
		providerName := strings.ToLower(strings.TrimSpace(compat.Name))
		if providerName == "" {
			providerName = "openai-compatibility"
		}
		base := strings.TrimSpace(compat.BaseURL)
		disableCooling := compat.DisableCooling

		// Handle new APIKeyEntries format (preferred)
		createdEntries := 0
		for j := range compat.APIKeyEntries {
			entry := &compat.APIKeyEntries[j]
			key := strings.TrimSpace(entry.APIKey)
			proxyURL := strings.TrimSpace(entry.ProxyURL)
			idKind := fmt.Sprintf("openai-compatibility:%s", providerName)
			id, token := idGen.Next(idKind, key, base, proxyURL)
			attrs := map[string]string{
				"source":       fmt.Sprintf("config:%s[%s]", providerName, token),
				"base_url":     base,
				"compat_name":  compat.Name,
				"provider_key": providerName,
			}
			metadata := map[string]any{}
			if disableCooling {
				metadata["disable_cooling"] = true
			}
			if compat.Priority != 0 {
				attrs["priority"] = strconv.Itoa(compat.Priority)
			}
			if key != "" {
				attrs["api_key"] = key
			}
			if hash := diff.ComputeOpenAICompatModelsHash(compat.Models); hash != "" {
				attrs["models_hash"] = hash
			}
			addConfigHeadersToAttrs(compat.Headers, attrs)
			a := &coreauth.Auth{
				ID:         id,
				Provider:   providerName,
				Label:      compat.Name,
				Prefix:     prefix,
				Status:     coreauth.StatusActive,
				ProxyURL:   proxyURL,
				Attributes: attrs,
				Metadata:   metadata,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if len(a.Metadata) == 0 {
				a.Metadata = nil
			}
			out = append(out, a)
			createdEntries++
		}
		// Fallback: create entry without API key if no APIKeyEntries
		if createdEntries == 0 {
			idKind := fmt.Sprintf("openai-compatibility:%s", providerName)
			id, token := idGen.Next(idKind, base)
			attrs := map[string]string{
				"source":       fmt.Sprintf("config:%s[%s]", providerName, token),
				"base_url":     base,
				"compat_name":  compat.Name,
				"provider_key": providerName,
			}
			metadata := map[string]any{}
			if disableCooling {
				metadata["disable_cooling"] = true
			}
			if compat.Priority != 0 {
				attrs["priority"] = strconv.Itoa(compat.Priority)
			}
			if hash := diff.ComputeOpenAICompatModelsHash(compat.Models); hash != "" {
				attrs["models_hash"] = hash
			}
			addConfigHeadersToAttrs(compat.Headers, attrs)
			a := &coreauth.Auth{
				ID:         id,
				Provider:   providerName,
				Label:      compat.Name,
				Prefix:     prefix,
				Status:     coreauth.StatusActive,
				Attributes: attrs,
				Metadata:   metadata,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if len(a.Metadata) == 0 {
				a.Metadata = nil
			}
			out = append(out, a)
		}
	}
	return out
}

// synthesizeVertexCompat creates Auth entries for Vertex-compatible providers.
func (s *ConfigSynthesizer) synthesizeVertexCompat(ctx *SynthesisContext) []*coreauth.Auth {
	cfg := ctx.Config
	out := make([]*coreauth.Auth, 0, len(cfg.VertexCompatAPIKey))
	for i := range cfg.VertexCompatAPIKey {
		compat := &cfg.VertexCompatAPIKey[i]
		extra := map[string]string{
			"provider_key": "vertex",
		}
		for k, v := range providerNameAttr(compat.Name) {
			extra[k] = v
		}
		out = append(out, synthesizeProviderKeyAuths(ctx, compat.APIKeyEntries, providerKeyAuthParams{
			Provider:       "vertex",
			Label:          providerKeyLabel(compat.Prefix, compat.Name, "vertex-apikey"),
			IDKind:         "vertex:apikey",
			SourcePrefix:   "config:vertex-apikey",
			Prefix:         strings.TrimSpace(compat.Prefix),
			BaseURL:        strings.TrimSpace(compat.BaseURL),
			Priority:       compat.Priority,
			Headers:        compat.Headers,
			ExcludedModels: compat.ExcludedModels,
			ModelsHash:     diff.ComputeVertexCompatModelsHash(compat.Models),
			ExtraAttrs:     extra,
		})...)
	}
	return out
}
