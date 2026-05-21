package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOAuthExcludedModelsConfig_UnmarshalLegacyArray(t *testing.T) {
	var cfg OAuthExcludedModelsConfig
	if err := yaml.Unmarshal([]byte(`
codex:
  - gpt-image-2
  - gpt-5-codex-mini
`), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !cfg.IsModelDisabled("codex", "gpt-image-2") {
		t.Fatalf("expected gpt-image-2 disabled")
	}
	if cfg["codex"]["gpt-image-2"] != true {
		t.Fatalf("expected bool true storage")
	}
}

func TestOAuthExcludedModelsConfig_UnmarshalBoolMap(t *testing.T) {
	var cfg OAuthExcludedModelsConfig
	if err := yaml.Unmarshal([]byte(`
antigravity:
  gemini-3-pro-preview: true
`), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !cfg.IsModelDisabled("antigravity", "gemini-3-pro-preview") {
		t.Fatalf("expected model disabled")
	}
}

func TestOAuthExcludedModelsConfig_MarshalDoesNotRecurse(t *testing.T) {
	cfg := OAuthExcludedModelsConfig{
		"codex": {"gpt-5": true},
	}
	if _, err := yaml.Marshal(cfg); err != nil {
		t.Fatalf("yaml marshal failed: %v", err)
	}
	if _, err := cfg.MarshalJSON(); err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
}
