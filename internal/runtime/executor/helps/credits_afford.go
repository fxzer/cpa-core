package helps

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var creditsAffordLimitPattern = regexp.MustCompile(`(?i)can only afford\s+(\d+)`)

// IsCreditsAffordError reports whether status/body look like OpenRouter-style
// "requires more credits, or fewer max_tokens / can only afford N" errors.
func IsCreditsAffordError(status int, body string) bool {
	// OpenRouter uses 402; some compat mirrors may echo the same text on 400.
	if status != http.StatusPaymentRequired && status != http.StatusBadRequest {
		return false
	}
	lower := strings.ToLower(body)
	if !strings.Contains(lower, "can only afford") {
		return false
	}
	return strings.Contains(lower, "max_tokens") ||
		strings.Contains(lower, "more credits") ||
		strings.Contains(lower, "fewer max_tokens")
}

// ParseCreditsAffordLimit extracts the affordable token budget from an upstream
// error body. Returns ok=false when the limit cannot be parsed or is non-positive.
func ParseCreditsAffordLimit(body string) (limit int, ok bool) {
	match := creditsAffordLimitPattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(match[1])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// ClampOpenAIMaxTokens lowers max_tokens / max_output_tokens / max_completion_tokens
// in an OpenAI-compatible JSON body to at most limit. Returns ok=false when no
// clamp was applied (missing fields, already <= limit, or invalid body).
func ClampOpenAIMaxTokens(body []byte, limit int) (clamped []byte, ok bool) {
	if limit <= 0 || len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false
	}
	out := body
	changed := false
	for _, path := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
		node := gjson.GetBytes(out, path)
		if !node.Exists() || node.Type != gjson.Number {
			continue
		}
		current := int(node.Int())
		if current <= 0 || current <= limit {
			continue
		}
		updated, err := sjson.SetBytes(out, path, limit)
		if err != nil {
			continue
		}
		out = updated
		changed = true
	}
	return out, changed
}

// TryClampCreditsAffordBody combines afford-error detection, limit parsing, and
// body clamping. Used by executors to retry once before surfacing the error.
func TryClampCreditsAffordBody(body []byte, status int, errBody string) (clamped []byte, ok bool) {
	if !IsCreditsAffordError(status, errBody) {
		return body, false
	}
	limit, parsed := ParseCreditsAffordLimit(errBody)
	if !parsed {
		return body, false
	}
	return ClampOpenAIMaxTokens(body, limit)
}
