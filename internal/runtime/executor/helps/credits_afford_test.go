package helps

import (
	"net/http"
	"testing"

	"github.com/tidwall/gjson"
)

func TestParseCreditsAffordLimit(t *testing.T) {
	body := `This request requires more credits, or fewer max_tokens. You requested up to 32000 tokens, but can only afford 12810.`
	limit, ok := ParseCreditsAffordLimit(body)
	if !ok || limit != 12810 {
		t.Fatalf("ParseCreditsAffordLimit() = (%d,%v), want (12810,true)", limit, ok)
	}
}

func TestClampOpenAIMaxTokens(t *testing.T) {
	body := []byte(`{"model":"x","max_tokens":32000,"messages":[]}`)
	out, ok := ClampOpenAIMaxTokens(body, 12810)
	if !ok {
		t.Fatal("expected clamp")
	}
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 12810 {
		t.Fatalf("max_tokens=%d, want 12810", got)
	}
}

func TestTryClampCreditsAffordBody(t *testing.T) {
	body := []byte(`{"max_tokens":32000}`)
	errBody := `{"error":{"message":"This request requires more credits, or fewer max_tokens. You requested up to 32000 tokens, but can only afford 931."}}`
	out, ok := TryClampCreditsAffordBody(body, http.StatusPaymentRequired, errBody)
	if !ok {
		t.Fatal("expected clamp retry body")
	}
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 931 {
		t.Fatalf("max_tokens=%d, want 931", got)
	}
}

func TestIsCreditsAffordError_RejectsUnrelated402(t *testing.T) {
	if IsCreditsAffordError(http.StatusPaymentRequired, `{"error":{"message":"Payment required"}}`) {
		t.Fatal("unrelated 402 should not match")
	}
}
