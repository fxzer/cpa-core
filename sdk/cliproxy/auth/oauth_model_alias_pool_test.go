package auth

import (
	"context"
	"net/http"
	"sync"
	"testing"

	internalconfig "github.com/fxzer/cpa-core/v7/internal/config"
	"github.com/fxzer/cpa-core/v7/internal/registry"
	cliproxyexecutor "github.com/fxzer/cpa-core/v7/sdk/cliproxy/executor"
)

type oauthPoolExecutor struct {
	id string

	mu            sync.Mutex
	executeModels []string
}

func (e *oauthPoolExecutor) Identifier() string { return e.id }

func (e *oauthPoolExecutor) Execute(_ context.Context, _ *Auth, req cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	e.mu.Lock()
	e.executeModels = append(e.executeModels, req.Model)
	e.mu.Unlock()
	return cliproxyexecutor.Response{Payload: []byte(req.Model)}, nil
}

func (e *oauthPoolExecutor) ExecuteStream(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, &Error{Message: "ExecuteStream not implemented"}
}

func (e *oauthPoolExecutor) Refresh(_ context.Context, auth *Auth) (*Auth, error) { return auth, nil }

func (e *oauthPoolExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, &Error{Message: "CountTokens not implemented"}
}

func (e *oauthPoolExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, &Error{Message: "HttpRequest not implemented"}
}

func (e *oauthPoolExecutor) ExecuteModels() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.executeModels))
	copy(out, e.executeModels)
	return out
}

func newOAuthPoolTestManager(t *testing.T, channel, alias string, aliases []internalconfig.OAuthModelAlias, executor *oauthPoolExecutor) *Manager {
	t.Helper()
	m := NewManager(nil, nil, nil)
	m.SetConfig(&internalconfig.Config{})
	m.SetOAuthModelAlias(map[string][]internalconfig.OAuthModelAlias{channel: aliases})
	if executor == nil {
		executor = &oauthPoolExecutor{id: channel}
	}
	m.RegisterExecutor(executor)

	auth := &Auth{
		ID:       "oauth-pool-auth-" + t.Name(),
		Provider: channel,
		Status:   StatusActive,
		Attributes: map[string]string{
			"auth_kind": "oauth",
		},
	}
	if _, err := m.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	reg := registry.GetGlobalRegistry()
	reg.RegisterClient(auth.ID, channel, []*registry.ModelInfo{{ID: alias}})
	t.Cleanup(func() {
		reg.UnregisterClient(auth.ID)
	})
	return m
}

func TestResolveOAuthUpstreamModelPool_MultipleNames(t *testing.T) {
	t.Parallel()

	mgr := NewManager(nil, nil, nil)
	mgr.SetOAuthModelAlias(map[string][]internalconfig.OAuthModelAlias{
		"codex": {
			{Name: "gpt-5.4-mini", Alias: "mini"},
			{Name: "gpt-5.4", Alias: "mini"},
		},
	})
	auth := createAuthForChannel("codex")

	got := mgr.resolveOAuthUpstreamModelPool(auth, "mini(8192)")
	want := []string{"gpt-5.4-mini(8192)", "gpt-5.4(8192)"}
	if len(got) != len(want) {
		t.Fatalf("pool len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestManagerExecute_OAuthAliasPoolRotatesWithinAuth(t *testing.T) {
	alias := "mini"
	channel := "codex"
	executor := &oauthPoolExecutor{id: channel}
	m := newOAuthPoolTestManager(t, channel, alias, []internalconfig.OAuthModelAlias{
		{Name: "gpt-5.4-mini", Alias: alias},
		{Name: "gpt-5.4", Alias: alias},
	}, executor)

	for i := 0; i < 3; i++ {
		resp, err := m.Execute(context.Background(), []string{channel}, cliproxyexecutor.Request{Model: alias}, cliproxyexecutor.Options{})
		if err != nil {
			t.Fatalf("execute %d: %v", i, err)
		}
		if len(resp.Payload) == 0 {
			t.Fatalf("execute %d returned empty payload", i)
		}
	}

	got := executor.ExecuteModels()
	want := []string{"gpt-5.4-mini", "gpt-5.4", "gpt-5.4-mini"}
	if len(got) != len(want) {
		t.Fatalf("execute calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("execute call %d model = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExecutionModelCandidates_OAuthAliasPoolDoesNotBreakDirectUpstreamName(t *testing.T) {
	t.Parallel()

	mgr := NewManager(nil, nil, nil)
	mgr.SetOAuthModelAlias(map[string][]internalconfig.OAuthModelAlias{
		"codex": {
			{Name: "gpt-5.4-mini", Alias: "mini"},
		},
	})
	auth := createAuthForChannel("codex")

	got := mgr.executionModelCandidates(auth, "gpt-5.4-mini")
	if len(got) != 1 || got[0] != "gpt-5.4-mini" {
		t.Fatalf("executionModelCandidates() = %v, want [gpt-5.4-mini]", got)
	}
}
