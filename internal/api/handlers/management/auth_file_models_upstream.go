package management

import (
	"sort"
	"strings"

	"github.com/fxzer/cpa-core/v7/internal/config"
	"github.com/fxzer/cpa-core/v7/internal/registry"
	coreauth "github.com/fxzer/cpa-core/v7/sdk/cliproxy/auth"
)

func parseUpstreamQuery(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	switch strings.ToLower(raw) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func buildOAuthAliasReverseLookup(aliases []config.OAuthModelAlias) map[string]string {
	reverse := make(map[string]string)
	for i := range aliases {
		name := strings.TrimSpace(aliases[i].Name)
		alias := strings.TrimSpace(aliases[i].Alias)
		if name == "" || alias == "" || strings.EqualFold(name, alias) {
			continue
		}
		nameKey := strings.ToLower(name)
		aliasKey := strings.ToLower(alias)
		reverse[nameKey] = name
		reverse[aliasKey] = name
	}
	return reverse
}

func resolveUpstreamAuthFileModels(cfg *config.Config, auth *coreauth.Auth, models []*registry.ModelInfo) []*registry.ModelInfo {
	if cfg == nil || auth == nil || len(models) == 0 {
		return models
	}
	channel := coreauth.OAuthModelAliasChannel(auth.Provider, authKindFromAuth(auth))
	if channel == "" || len(cfg.OAuthModelAlias) == 0 {
		return models
	}
	aliases := cfg.OAuthModelAlias[channel]
	if len(aliases) == 0 {
		return models
	}
	reverse := buildOAuthAliasReverseLookup(aliases)
	if len(reverse) == 0 {
		return models
	}

	byUpstream := make(map[string]*registry.ModelInfo, len(models))
	for _, model := range models {
		if model == nil {
			continue
		}
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		upstreamID := reverse[strings.ToLower(id)]
		if upstreamID == "" {
			upstreamID = id
		}
		key := strings.ToLower(upstreamID)

		existing := byUpstream[key]
		if existing == nil {
			clone := *model
			clone.ID = upstreamID
			byUpstream[key] = &clone
			continue
		}
		if strings.EqualFold(id, upstreamID) && !strings.EqualFold(existing.ID, upstreamID) {
			clone := *model
			clone.ID = upstreamID
			byUpstream[key] = &clone
		}
	}

	out := make([]*registry.ModelInfo, 0, len(byUpstream))
	for _, model := range byUpstream {
		out = append(out, model)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].ID) < strings.ToLower(out[j].ID)
	})
	return out
}

func authKindFromAuth(auth *coreauth.Auth) string {
	if auth == nil || auth.Attributes == nil {
		return ""
	}
	if kind := strings.ToLower(strings.TrimSpace(auth.Attributes["auth_kind"])); kind != "" {
		return kind
	}
	if kind, _ := auth.AccountInfo(); strings.EqualFold(kind, "api_key") {
		return "apikey"
	}
	return ""
}
