package auth

import (
	"strings"

	internalconfig "github.com/fxzer/cpa-core/v7/internal/config"
	"github.com/fxzer/cpa-core/v7/internal/thinking"
)

type modelAliasEntry interface {
	GetName() string
	GetAlias() string
}

type oauthModelAliasTable struct {
	// reverse maps channel -> alias (lower) -> upstream model names in config order.
	// Multiple upstream names may share the same alias to form an alias pool.
	reverse map[string]map[string][]string
}

func compileOAuthModelAliasTable(aliases map[string][]internalconfig.OAuthModelAlias) *oauthModelAliasTable {
	if len(aliases) == 0 {
		return &oauthModelAliasTable{}
	}
	out := &oauthModelAliasTable{
		reverse: make(map[string]map[string][]string, len(aliases)),
	}
	for rawChannel, entries := range aliases {
		channel := strings.ToLower(strings.TrimSpace(rawChannel))
		if channel == "" || len(entries) == 0 {
			continue
		}
		rev := make(map[string][]string, len(entries))
		for _, entry := range entries {
			name := strings.TrimSpace(entry.Name)
			alias := strings.TrimSpace(entry.Alias)
			if name == "" || alias == "" {
				continue
			}
			if strings.EqualFold(name, alias) {
				continue
			}
			aliasKey := strings.ToLower(alias)
			names := rev[aliasKey]
			dup := false
			for _, existing := range names {
				if strings.EqualFold(existing, name) {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			rev[aliasKey] = append(names, name)
		}
		if len(rev) > 0 {
			out.reverse[channel] = rev
		}
	}
	if len(out.reverse) == 0 {
		out.reverse = nil
	}
	return out
}

// SetOAuthModelAlias updates the OAuth model name alias table used during execution.
// The alias is applied per-auth channel to resolve the upstream model name while keeping the
// client-visible model name unchanged for translation/response formatting.
func (m *Manager) SetOAuthModelAlias(aliases map[string][]internalconfig.OAuthModelAlias) {
	if m == nil {
		return
	}
	table := compileOAuthModelAliasTable(aliases)
	// atomic.Value requires non-nil store values.
	if table == nil {
		table = &oauthModelAliasTable{}
	}
	m.oauthModelAlias.Store(table)
}

// applyOAuthModelAlias resolves the upstream model from OAuth model alias.
// If an alias exists, the returned model is the upstream model.
func (m *Manager) applyOAuthModelAlias(auth *Auth, requestedModel string) string {
	upstreamModel := m.resolveOAuthUpstreamModel(auth, requestedModel)
	if upstreamModel == "" {
		return requestedModel
	}
	return upstreamModel
}

func modelAliasLookupCandidates(requestedModel string) (thinking.SuffixResult, []string) {
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return thinking.SuffixResult{}, nil
	}
	requestResult := thinking.ParseSuffix(requestedModel)
	base := requestResult.ModelName
	if base == "" {
		base = requestedModel
	}
	candidates := []string{base}
	if base != requestedModel {
		candidates = append(candidates, requestedModel)
	}
	return requestResult, candidates
}

func preserveResolvedModelSuffix(resolved string, requestResult thinking.SuffixResult) string {
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		return ""
	}
	if thinking.ParseSuffix(resolved).HasSuffix {
		return resolved
	}
	if requestResult.HasSuffix && requestResult.RawSuffix != "" {
		return resolved + "(" + requestResult.RawSuffix + ")"
	}
	return resolved
}

func resolveModelAliasPoolFromConfigModels(requestedModel string, models []modelAliasEntry) []string {
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return nil
	}
	if len(models) == 0 {
		return nil
	}

	requestResult, candidates := modelAliasLookupCandidates(requestedModel)
	if len(candidates) == 0 {
		return nil
	}

	out := make([]string, 0)
	seen := make(map[string]struct{})
	for i := range models {
		name := strings.TrimSpace(models[i].GetName())
		alias := strings.TrimSpace(models[i].GetAlias())
		for _, candidate := range candidates {
			if candidate == "" || alias == "" || !strings.EqualFold(alias, candidate) {
				continue
			}
			resolved := candidate
			if name != "" {
				resolved = name
			}
			resolved = preserveResolvedModelSuffix(resolved, requestResult)
			key := strings.ToLower(strings.TrimSpace(resolved))
			if key == "" {
				break
			}
			if _, exists := seen[key]; exists {
				break
			}
			seen[key] = struct{}{}
			out = append(out, resolved)
			break
		}
	}
	if len(out) > 0 {
		return out
	}

	for i := range models {
		name := strings.TrimSpace(models[i].GetName())
		for _, candidate := range candidates {
			if candidate == "" || name == "" || !strings.EqualFold(name, candidate) {
				continue
			}
			return []string{preserveResolvedModelSuffix(name, requestResult)}
		}
	}
	return nil
}

func resolveModelAliasFromConfigModels(requestedModel string, models []modelAliasEntry) string {
	resolved := resolveModelAliasPoolFromConfigModels(requestedModel, models)
	if len(resolved) > 0 {
		return resolved[0]
	}
	return ""
}

// resolveOAuthUpstreamModel resolves the upstream model name from OAuth model alias.
// When multiple upstream names share the same alias, the first configured entry is returned.
func (m *Manager) resolveOAuthUpstreamModel(auth *Auth, requestedModel string) string {
	pool := m.resolveOAuthUpstreamModelPool(auth, requestedModel)
	if len(pool) > 0 {
		return pool[0]
	}
	return ""
}

// resolveOAuthUpstreamModelPool resolves all upstream model names for an OAuth alias pool.
func (m *Manager) resolveOAuthUpstreamModelPool(auth *Auth, requestedModel string) []string {
	return resolveOAuthUpstreamModelPoolFromTable(m, auth, requestedModel, modelAliasChannel(auth))
}

func resolveOAuthUpstreamModelPoolFromTable(m *Manager, auth *Auth, requestedModel, channel string) []string {
	if m == nil || auth == nil || channel == "" {
		return nil
	}

	requestResult, candidates := modelAliasLookupCandidates(requestedModel)
	if len(candidates) == 0 {
		return nil
	}
	baseModel := requestResult.ModelName
	if baseModel == "" {
		baseModel = strings.TrimSpace(requestedModel)
	}

	raw := m.oauthModelAlias.Load()
	table, _ := raw.(*oauthModelAliasTable)
	if table == nil || table.reverse == nil {
		return nil
	}
	rev := table.reverse[channel]
	if rev == nil {
		return nil
	}

	for _, candidate := range candidates {
		key := strings.ToLower(strings.TrimSpace(candidate))
		if key == "" {
			continue
		}
		originals := rev[key]
		if len(originals) == 0 {
			continue
		}

		out := make([]string, 0, len(originals))
		seen := make(map[string]struct{}, len(originals))
		for _, original := range originals {
			original = strings.TrimSpace(original)
			if original == "" {
				continue
			}
			if strings.EqualFold(original, baseModel) {
				return nil
			}

			resolved := original
			if thinking.ParseSuffix(original).HasSuffix {
				resolved = original
			} else if requestResult.HasSuffix && requestResult.RawSuffix != "" {
				resolved = original + "(" + requestResult.RawSuffix + ")"
			}

			resolvedKey := strings.ToLower(strings.TrimSpace(resolved))
			if resolvedKey == "" {
				continue
			}
			if _, exists := seen[resolvedKey]; exists {
				continue
			}
			seen[resolvedKey] = struct{}{}
			out = append(out, resolved)
		}
		if len(out) > 0 {
			return out
		}
	}

	return nil
}

func oauthModelPoolKey(auth *Auth, requestedModel string) string {
	if auth == nil {
		return ""
	}
	requestResult := thinking.ParseSuffix(requestedModel)
	base := strings.ToLower(strings.TrimSpace(requestResult.ModelName))
	if base == "" {
		base = strings.ToLower(strings.TrimSpace(requestedModel))
	}
	return strings.ToLower(strings.TrimSpace(auth.ID)) + "|" + modelAliasChannel(auth) + "|" + base
}

// modelAliasChannel extracts the OAuth model alias channel from an Auth object.
// It determines the provider and auth kind from the Auth's attributes and delegates
// to OAuthModelAliasChannel for the actual channel resolution.
func modelAliasChannel(auth *Auth) string {
	if auth == nil {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(auth.Provider))
	authKind := ""
	if auth.Attributes != nil {
		authKind = strings.ToLower(strings.TrimSpace(auth.Attributes["auth_kind"]))
	}
	if authKind == "" {
		if kind, _ := auth.AccountInfo(); strings.EqualFold(kind, "api_key") {
			authKind = "apikey"
		}
	}
	return OAuthModelAliasChannel(provider, authKind)
}

// OAuthModelAliasChannel returns the OAuth model alias channel name for a given provider
// and auth kind. Returns empty string if the provider/authKind combination doesn't support
// OAuth model alias (e.g., API key authentication).
//
// Supported channels: gemini-cli, vertex, aistudio, antigravity, claude, codex, kimi, xai.
func OAuthModelAliasChannel(provider, authKind string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	authKind = strings.ToLower(strings.TrimSpace(authKind))
	switch provider {
	case "gemini":
		// gemini provider uses gemini-api-key config, not oauth-model-alias.
		// OAuth-based gemini auth is converted to "gemini-cli" by the synthesizer.
		return ""
	case "vertex":
		if authKind == "apikey" {
			return ""
		}
		return "vertex"
	case "claude":
		if authKind == "apikey" {
			return ""
		}
		return "claude"
	case "codex":
		if authKind == "apikey" {
			return ""
		}
		return "codex"
	case "gemini-cli", "aistudio", "antigravity", "kimi", "xai":
		return provider
	default:
		return ""
	}
}
