package management

import (
	"testing"

	"github.com/fxzer/cpa-core/v7/internal/config"
	"github.com/fxzer/cpa-core/v7/internal/registry"
	coreauth "github.com/fxzer/cpa-core/v7/sdk/cliproxy/auth"
)

func TestResolveUpstreamAuthFileModels_CodexAliases(t *testing.T) {
	cfg := &config.Config{
		OAuthModelAlias: map[string][]config.OAuthModelAlias{
			"codex": {
				{Name: "gpt-5.2", Alias: "pro"},
				{Name: "gpt-5.3-codex", Alias: "lite", Fork: true},
				{Name: "gpt-5.4", Alias: "abc", Fork: true},
			},
		},
	}
	auth := &coreauth.Auth{
		Provider: "codex",
		Attributes: map[string]string{
			"auth_kind": "oauth",
		},
	}
	models := []*registry.ModelInfo{
		{ID: "pro", DisplayName: "GPT 5.2", Type: "openai"},
		{ID: "gpt-5.3-codex", DisplayName: "GPT 5.3 Codex", Type: "openai"},
		{ID: "lite", DisplayName: "GPT 5.3 Codex", Type: "openai"},
		{ID: "gpt-5.4", DisplayName: "GPT 5.4", Type: "openai"},
		{ID: "abc", DisplayName: "GPT 5.4", Type: "openai"},
		{ID: "gpt-5.5", DisplayName: "GPT 5.5", Type: "openai"},
	}

	got := resolveUpstreamAuthFileModels(cfg, auth, models)
	ids := make([]string, 0, len(got))
	for _, m := range got {
		ids = append(ids, m.ID)
	}

	want := []string{"gpt-5.2", "gpt-5.3-codex", "gpt-5.4", "gpt-5.5"}
	if len(ids) != len(want) {
		t.Fatalf("model count = %d (%v), want %d (%v)", len(ids), ids, len(want), want)
	}
	for i, id := range want {
		if ids[i] != id {
			t.Fatalf("ids[%d] = %q, want %q (all: %v)", i, ids[i], id, ids)
		}
	}
	if got[0].DisplayName != "GPT 5.2" {
		t.Fatalf("gpt-5.2 display_name = %q, want %q", got[0].DisplayName, "GPT 5.2")
	}
}

func TestResolveUpstreamAuthFileModels_NoAliasConfig(t *testing.T) {
	cfg := &config.Config{}
	auth := &coreauth.Auth{Provider: "codex", Attributes: map[string]string{"auth_kind": "oauth"}}
	models := []*registry.ModelInfo{{ID: "pro", DisplayName: "GPT 5.2"}}

	got := resolveUpstreamAuthFileModels(cfg, auth, models)
	if len(got) != 1 || got[0].ID != "pro" {
		t.Fatalf("expected unchanged model list, got %#v", got)
	}
}

func TestParseUpstreamQuery(t *testing.T) {
	cases := map[string]bool{
		"":      true,
		"1":     true,
		"true":  true,
		"0":     false,
		"false": false,
		"off":   false,
	}
	for input, want := range cases {
		if got := parseUpstreamQuery(input); got != want {
			t.Fatalf("parseUpstreamQuery(%q) = %v, want %v", input, got, want)
		}
	}
}
