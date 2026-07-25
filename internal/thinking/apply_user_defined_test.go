package thinking_test

import (
	"testing"

	"github.com/fxzer/cpa-core/v7/internal/registry"
	"github.com/fxzer/cpa-core/v7/internal/thinking"
	_ "github.com/fxzer/cpa-core/v7/internal/thinking/provider/claude"
	_ "github.com/fxzer/cpa-core/v7/internal/thinking/provider/openai"
	"github.com/tidwall/gjson"
)

func TestClampOpenAICompatLevel(t *testing.T) {
	cases := []struct {
		in   thinking.ThinkingLevel
		want thinking.ThinkingLevel
	}{
		{thinking.LevelXHigh, thinking.LevelHigh},
		{thinking.LevelMax, thinking.LevelHigh},
		{thinking.LevelHigh, thinking.LevelHigh},
		{thinking.LevelMedium, thinking.LevelMedium},
		{thinking.LevelLow, thinking.LevelLow},
		{thinking.ThinkingLevel("XHIGH"), thinking.LevelHigh},
	}
	for _, tc := range cases {
		if got := thinking.ClampOpenAICompatLevel(tc.in); got != tc.want {
			t.Fatalf("ClampOpenAICompatLevel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestApplyThinking_UserDefinedOpenAIClampsXHigh(t *testing.T) {
	reg := registry.GetGlobalRegistry()
	clientID := "test-user-defined-openai-" + t.Name()
	modelID := "gpt-5.5"
	reg.RegisterClient(clientID, "openai", []*registry.ModelInfo{{ID: modelID, UserDefined: true}})
	t.Cleanup(func() {
		reg.UnregisterClient(clientID)
	})

	tests := []struct {
		name  string
		model string
		body  []byte
	}{
		{
			name:  "body reasoning_effort xhigh",
			model: modelID,
			body:  []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"xhigh"}`),
		},
		{
			name:  "suffix xhigh",
			model: modelID + "(xhigh)",
			body:  []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}]}`),
		},
		{
			name:  "body reasoning_effort max",
			model: modelID,
			body:  []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"max"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := thinking.ApplyThinking(tt.body, tt.model, "openai", "openai", "openai")
			if err != nil {
				t.Fatalf("ApplyThinking() error = %v", err)
			}
			if got := gjson.GetBytes(out, "reasoning_effort").String(); got != "high" {
				t.Fatalf("reasoning_effort = %q, want %q, body=%s", got, "high", string(out))
			}
		})
	}
}

func TestApplyThinking_UserDefinedClaudePreservesAdaptiveLevel(t *testing.T) {
	reg := registry.GetGlobalRegistry()
	clientID := "test-user-defined-claude-" + t.Name()
	modelID := "custom-claude-4-6"
	reg.RegisterClient(clientID, "claude", []*registry.ModelInfo{{ID: modelID, UserDefined: true}})
	t.Cleanup(func() {
		reg.UnregisterClient(clientID)
	})

	tests := []struct {
		name  string
		model string
		body  []byte
	}{
		{
			name:  "claude adaptive effort body",
			model: modelID,
			body:  []byte(`{"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`),
		},
		{
			name:  "suffix level",
			model: modelID + "(high)",
			body:  []byte(`{}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := thinking.ApplyThinking(tt.body, tt.model, "openai", "claude", "claude")
			if err != nil {
				t.Fatalf("ApplyThinking() error = %v", err)
			}
			if got := gjson.GetBytes(out, "thinking.type").String(); got != "adaptive" {
				t.Fatalf("thinking.type = %q, want %q, body=%s", got, "adaptive", string(out))
			}
			if got := gjson.GetBytes(out, "output_config.effort").String(); got != "high" {
				t.Fatalf("output_config.effort = %q, want %q, body=%s", got, "high", string(out))
			}
			if gjson.GetBytes(out, "thinking.budget_tokens").Exists() {
				t.Fatalf("thinking.budget_tokens should be removed, body=%s", string(out))
			}
		})
	}
}
