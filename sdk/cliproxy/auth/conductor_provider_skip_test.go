package auth

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNoteProviderSkipForRequest(t *testing.T) {
	skipped := map[string]struct{}{}
	streak := map[string]int{}

	noteProviderSkipForRequest(skipped, streak, "OpenRouter", &Error{
		HTTPStatus: http.StatusPaymentRequired,
		Message:    `can only afford 100`,
	})
	if _, ok := skipped["openrouter"]; !ok {
		t.Fatal("expected openrouter skipped after first 402")
	}

	skipped = map[string]struct{}{}
	streak = map[string]int{}
	err401 := &Error{HTTPStatus: http.StatusUnauthorized, Message: "Invalid token"}
	noteProviderSkipForRequest(skipped, streak, "tokengen", err401)
	if _, ok := skipped["tokengen"]; ok {
		t.Fatal("did not expect skip after single 401")
	}
	noteProviderSkipForRequest(skipped, streak, "tokengen", err401)
	if _, ok := skipped["tokengen"]; !ok {
		t.Fatal("expected tokengen skipped after two 401s")
	}
}

func TestNoteProviderSkipForRequest_AccountPermanent(t *testing.T) {
	skipped := map[string]struct{}{}
	streak := map[string]int{}
	noteProviderSkipForRequest(skipped, streak, "stepfun", &Error{
		HTTPStatus: http.StatusBadRequest,
		Message:    "you have no active step plan subscription",
	})
	if _, ok := skipped["stepfun"]; !ok {
		t.Fatal("expected stepfun skipped after account-permanent 400")
	}
}

func TestManager_MarkResult_AccountPermanentLongCooldown(t *testing.T) {
	m := NewManager(nil, nil, nil)
	auth := &Auth{ID: "auth-1", Provider: "stepfun"}
	if _, err := m.Register(context.Background(), auth); err != nil {
		t.Fatalf("register: %v", err)
	}
	model := "step-3.7-flash"
	before := time.Now()
	m.MarkResult(context.Background(), Result{
		AuthID:   auth.ID,
		Provider: auth.Provider,
		Model:    model,
		Success:  false,
		Error: &Error{
			HTTPStatus: http.StatusBadRequest,
			Message:    "you have no active step plan subscription",
		},
	})
	updated, ok := m.GetByID(auth.ID)
	if !ok || updated == nil {
		t.Fatal("auth missing")
	}
	state := updated.ModelStates[model]
	if state == nil {
		t.Fatal("expected model state")
	}
	if state.NextRetryAfter.Before(before.Add(11 * time.Hour)) {
		t.Fatalf("expected ~12h cooldown, got %v", state.NextRetryAfter)
	}
}
