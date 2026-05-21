package management

import (
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

type authFileModelConfigRow struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	Type        string `json:"type,omitempty"`
	OwnedBy     string `json:"owned_by,omitempty"`
	Available   bool   `json:"available"`
	Alias       string `json:"alias"`
	Fork        bool   `json:"fork"`
	Disabled    bool   `json:"disabled"`
}

type authFileModelsConfigSummary struct {
	Total       int `json:"total"`
	Aliased     int `json:"aliased"`
	Passthrough int `json:"passthrough"`
	Disabled    int `json:"disabled"`
}

func (h *Handler) resolveAuthFileRecord(name string) (authID string, authRecord *coreauth.Auth) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	if h.authManager != nil {
		if auth, ok := h.authManager.GetByID(name); ok && auth != nil {
			return auth.ID, auth
		}
		baseName := filepath.Base(name)
		for _, auth := range h.authManager.List() {
			if auth == nil {
				continue
			}
			fileName := strings.TrimSpace(auth.FileName)
			if fileName != "" {
				if fileName == name || strings.EqualFold(fileName, name) || filepath.Base(fileName) == baseName {
					return auth.ID, auth
				}
			}
			id := strings.TrimSpace(auth.ID)
			if id != "" && (id == name || strings.EqualFold(id, name)) {
				return auth.ID, auth
			}
			if path := strings.TrimSpace(authAttribute(auth, "path")); path != "" && filepath.Base(path) == baseName {
				return auth.ID, auth
			}
		}
	}
	return name, nil
}

func inferOAuthChannelFromAuthFileName(name string) string {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(name)))
	base = strings.TrimSuffix(base, ".json")
	if base == "" {
		return ""
	}
	if idx := strings.Index(base, "-"); idx > 0 {
		return coreauth.OAuthModelAliasChannel(base[:idx], "oauth")
	}
	return coreauth.OAuthModelAliasChannel(base, "oauth")
}

func (h *Handler) resolveAuthFileModelsContext(name string) (authID string, authRecord *coreauth.Auth, channel string) {
	authID, authRecord = h.resolveAuthFileRecord(name)
	if authID == "" {
		authID = name
	}
	if authRecord != nil {
		channel = resolveOAuthChannel(authRecord)
	}
	if channel == "" {
		channel = inferOAuthChannelFromAuthFileName(name)
	}
	return authID, authRecord, channel
}

func resolveOAuthChannel(authRecord *coreauth.Auth) string {
	if authRecord == nil {
		return ""
	}
	return coreauth.OAuthModelAliasChannel(authRecord.Provider, authKindFromAuth(authRecord))
}

func buildAuthFileModelsConfig(
	cfg *config.Config,
	authRecord *coreauth.Auth,
	authID string,
	channel string,
) ([]authFileModelConfigRow, authFileModelsConfigSummary) {
	catalog := registry.GetStaticModelDefinitionsByChannel(channel)
	availableSet := make(map[string]struct{})
	if authID != "" {
		reg := registry.GetGlobalRegistry()
		modelsForAuth := reg.GetModelsForClient(authID)
		if authRecord != nil {
			modelsForAuth = resolveUpstreamAuthFileModels(cfg, authRecord, modelsForAuth)
		}
		for _, model := range modelsForAuth {
			if model == nil {
				continue
			}
			id := strings.TrimSpace(model.ID)
			if id != "" {
				availableSet[strings.ToLower(id)] = struct{}{}
			}
		}
	}

	byID := make(map[string]*registry.ModelInfo)
	addCatalog := func(model *registry.ModelInfo) {
		if model == nil {
			return
		}
		id := strings.TrimSpace(model.ID)
		if id == "" {
			return
		}
		key := strings.ToLower(id)
		if _, exists := byID[key]; !exists {
			byID[key] = model
		}
	}
	for _, model := range catalog {
		addCatalog(model)
	}
	if cfg != nil && channel != "" {
		if aliases := cfg.OAuthModelAlias[channel]; len(aliases) > 0 {
			for i := range aliases {
				name := strings.TrimSpace(aliases[i].Name)
				if name != "" {
					addCatalog(&registry.ModelInfo{ID: name})
				}
			}
		}
		if excluded := cfg.OAuthExcludedModels[channel]; len(excluded) > 0 {
			for name := range excluded {
				if trimmed := strings.TrimSpace(name); trimmed != "" {
					addCatalog(&registry.ModelInfo{ID: trimmed})
				}
			}
		}
	}

	aliasByName := make(map[string]config.OAuthModelAlias)
	if cfg != nil && channel != "" {
		for _, entry := range cfg.OAuthModelAlias[channel] {
			name := strings.TrimSpace(entry.Name)
			if name == "" {
				continue
			}
			aliasByName[strings.ToLower(name)] = entry
		}
	}

	rows := make([]authFileModelConfigRow, 0, len(byID))
	for _, model := range byID {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		aliasEntry, hasAlias := aliasByName[strings.ToLower(id)]
		row := authFileModelConfigRow{
			ID:        id,
			Available: false,
			Alias:     "",
			Fork:      true,
			Disabled:  cfg != nil && cfg.OAuthExcludedModels.IsModelDisabled(channel, id),
		}
		if model.DisplayName != "" {
			row.DisplayName = model.DisplayName
		}
		if model.Type != "" {
			row.Type = model.Type
		}
		if model.OwnedBy != "" {
			row.OwnedBy = model.OwnedBy
		}
		if _, ok := availableSet[strings.ToLower(id)]; ok {
			row.Available = true
		}
		if hasAlias {
			row.Alias = strings.TrimSpace(aliasEntry.Alias)
			row.Fork = aliasEntry.Fork
		}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].ID) < strings.ToLower(rows[j].ID)
	})

	summary := authFileModelsConfigSummary{Total: len(rows)}
	aliasedNames := make(map[string]struct{})
	for _, row := range rows {
		if row.Disabled {
			summary.Disabled++
		}
		if strings.TrimSpace(row.Alias) != "" {
			aliasedNames[strings.ToLower(row.ID)] = struct{}{}
		}
	}
	summary.Aliased = len(aliasedNames)
	summary.Passthrough = summary.Total - summary.Aliased
	return rows, summary
}

// GetAuthFileModelsConfig returns modal-shaped model alias/disable config for one auth file.
func (h *Handler) GetAuthFileModelsConfig(c *gin.Context) {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	authID, authRecord, channel := h.resolveAuthFileModelsContext(name)
	if channel == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth file not found"})
		return
	}

	rows, summary := buildAuthFileModelsConfig(h.cfg, authRecord, authID, channel)
	c.JSON(http.StatusOK, gin.H{
		"provider": channel,
		"rows":     rows,
		"summary":  summary,
	})
}

// PatchAuthFileModelsConfig updates modal-shaped alias/disable config for one auth file provider channel.
func (h *Handler) PatchAuthFileModelsConfig(c *gin.Context) {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	var body struct {
		Rows []authFileModelConfigRow `json:"rows"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	_, _, channel := h.resolveAuthFileModelsContext(name)
	if channel == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth file not found"})
		return
	}

	aliases := make([]config.OAuthModelAlias, 0)
	excluded := make(config.OAuthExcludedProviderModels)
	for _, row := range body.Rows {
		modelID := strings.TrimSpace(row.ID)
		if modelID == "" {
			continue
		}
		if row.Disabled {
			excluded[modelID] = true
		}
		alias := strings.TrimSpace(row.Alias)
		if alias == "" || strings.EqualFold(alias, modelID) {
			continue
		}
		entry := config.OAuthModelAlias{Name: modelID, Alias: alias}
		if row.Fork {
			entry.Fork = true
		}
		aliases = append(aliases, entry)
	}

	normalizedAliasesMap := sanitizedOAuthModelAlias(map[string][]config.OAuthModelAlias{channel: aliases})
	normalizedAliases := normalizedAliasesMap[channel]
	if len(normalizedAliases) > 0 {
		if h.cfg.OAuthModelAlias == nil {
			h.cfg.OAuthModelAlias = make(map[string][]config.OAuthModelAlias)
		}
		h.cfg.OAuthModelAlias[channel] = normalizedAliases
	} else if h.cfg.OAuthModelAlias != nil {
		delete(h.cfg.OAuthModelAlias, channel)
		if len(h.cfg.OAuthModelAlias) == 0 {
			h.cfg.OAuthModelAlias = nil
		}
	}

	normalizedExcluded := normalizeOAuthExcludedProviderModels(excluded)
	if len(normalizedExcluded) > 0 {
		if h.cfg.OAuthExcludedModels == nil {
			h.cfg.OAuthExcludedModels = make(config.OAuthExcludedModelsConfig)
		}
		h.cfg.OAuthExcludedModels[channel] = normalizedExcluded
	} else if h.cfg.OAuthExcludedModels != nil {
		delete(h.cfg.OAuthExcludedModels, channel)
		if len(h.cfg.OAuthExcludedModels) == 0 {
			h.cfg.OAuthExcludedModels = nil
		}
	}

	h.persist(c)
}

func normalizeOAuthExcludedProviderModels(models config.OAuthExcludedProviderModels) config.OAuthExcludedProviderModels {
	if len(models) == 0 {
		return nil
	}
	out := make(config.OAuthExcludedProviderModels, len(models))
	for name, disabled := range models {
		if !disabled {
			continue
		}
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			out[trimmed] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
