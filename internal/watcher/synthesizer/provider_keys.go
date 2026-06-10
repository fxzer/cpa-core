package synthesizer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fxzer/cpa-core/v7/internal/config"
	coreauth "github.com/fxzer/cpa-core/v7/sdk/cliproxy/auth"
)

type providerKeyAuthParams struct {
	Provider       string
	Label          string
	IDKind         string
	SourcePrefix   string
	Prefix         string
	BaseURL        string
	Priority       int
	DisableCooling bool
	Headers        map[string]string
	ExcludedModels []string
	ModelsHash     string
	ExtraAttrs     map[string]string
}

func synthesizeProviderKeyAuths(
	ctx *SynthesisContext,
	entries []config.ProviderAPIKeyEntry,
	params providerKeyAuthParams,
) []*coreauth.Auth {
	if ctx == nil || ctx.Config == nil || len(entries) == 0 {
		return nil
	}
	cfg := ctx.Config
	now := ctx.Now
	idGen := ctx.IDGenerator

	out := make([]*coreauth.Auth, 0, len(entries))
	for i := range entries {
		entry := entries[i]
		key := strings.TrimSpace(entry.APIKey)
		if key == "" {
			continue
		}
		proxyURL := strings.TrimSpace(entry.ProxyURL)
		id, token := idGen.Next(params.IDKind, key, params.BaseURL, proxyURL)
		attrs := map[string]string{
			"source": fmt.Sprintf("%s[%s]", params.SourcePrefix, token),
		}
		if params.BaseURL != "" {
			attrs["base_url"] = params.BaseURL
		}
		if key != "" {
			attrs["api_key"] = key
		}
		if params.Priority != 0 {
			attrs["priority"] = strconv.Itoa(params.Priority)
		}
		if params.ModelsHash != "" {
			attrs["models_hash"] = params.ModelsHash
		}
		for k, v := range params.ExtraAttrs {
			if strings.TrimSpace(v) != "" {
				attrs[k] = v
			}
		}
		addConfigHeadersToAttrs(params.Headers, attrs)

		metadata := map[string]any{}
		if params.DisableCooling {
			metadata["disable_cooling"] = true
		}

		a := &coreauth.Auth{
			ID:         id,
			Provider:   params.Provider,
			Label:      params.Label,
			Prefix:     params.Prefix,
			Status:     coreauth.StatusActive,
			ProxyURL:   proxyURL,
			Attributes: attrs,
			Metadata:   metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		ApplyAuthExcludedModelsMeta(a, cfg, params.ExcludedModels, "apikey")
		if len(a.Metadata) == 0 {
			a.Metadata = nil
		}
		out = append(out, a)
	}
	return out
}
