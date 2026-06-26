package auth

import (
	"reflect"
	"testing"
	"time"

	internalconfig "github.com/fxzer/cpa-core/v7/internal/config"
)

func TestPlanExecutionModelsAt_DecisionTable(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name       string
		routeModel string
		auth       *Auth
		setup      func(*Manager)
		wantModels []string
		wantPooled bool
	}{
		{
			name:       "direct model uses requested route model",
			routeModel: "gemini-2.5-pro",
			auth:       &Auth{ID: "direct", Provider: "gemini-cli"},
			wantModels: []string{"gemini-2.5-pro"},
			wantPooled: false,
		},
		{
			name:       "oauth alias pool keeps pooled status and filters cooled upstream",
			routeModel: "mini",
			auth: &Auth{
				ID:       "codex-pool",
				Provider: "codex",
				Attributes: map[string]string{
					"auth_kind": "oauth",
				},
				ModelStates: map[string]*ModelState{
					"gpt-5.4-mini": {
						Unavailable:    true,
						Status:         StatusError,
						NextRetryAfter: now.Add(time.Minute),
						Quota: QuotaState{
							Exceeded:      true,
							NextRecoverAt: now.Add(time.Minute),
						},
					},
				},
			},
			setup: func(m *Manager) {
				m.SetOAuthModelAlias(map[string][]internalconfig.OAuthModelAlias{
					"codex": {
						{Name: "gpt-5.4-mini", Alias: "mini"},
						{Name: "gpt-5.4", Alias: "mini"},
					},
				})
			},
			wantModels: []string{"gpt-5.4"},
			wantPooled: true,
		},
		{
			name:       "api key alias resolves to upstream without pooled state",
			routeModel: "fast",
			auth: &Auth{
				ID:       "api-key",
				Provider: "openai-compatibility",
				Attributes: map[string]string{
					"auth_kind": "api-key",
					"api_key":   "secret",
				},
			},
			setup: func(m *Manager) {
				m.apiKeyModelAlias.Store(apiKeyModelAliasTable{
					"api-key": {
						"fast": "gpt-fast",
					},
				})
			},
			wantModels: []string{"gpt-fast"},
			wantPooled: false,
		},
		{
			name:       "home upstream model overrides configured aliases",
			routeModel: "mini",
			auth: &Auth{
				ID:       "home-auth",
				Provider: "codex",
				Attributes: map[string]string{
					"auth_kind":                   "oauth",
					homeUpstreamModelAttributeKey: "home-model",
				},
			},
			setup: func(m *Manager) {
				m.SetOAuthModelAlias(map[string][]internalconfig.OAuthModelAlias{
					"codex": {
						{Name: "gpt-5.4-mini", Alias: "mini"},
						{Name: "gpt-5.4", Alias: "mini"},
					},
				})
			},
			wantModels: []string{"home-model"},
			wantPooled: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := NewManager(nil, nil, nil)
			m.SetConfig(&internalconfig.Config{})
			if tt.setup != nil {
				tt.setup(m)
			}

			got := m.planExecutionModelsAt(tt.auth, tt.routeModel, now)
			if !reflect.DeepEqual(got.models, tt.wantModels) {
				t.Fatalf("models = %v, want %v", got.models, tt.wantModels)
			}
			if got.pooled != tt.wantPooled {
				t.Fatalf("pooled = %v, want %v", got.pooled, tt.wantPooled)
			}
		})
	}
}
