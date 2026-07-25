package chat_completions

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertOpenAIRequestToOpenAI_NormalizesMessages(t *testing.T) {
	in := []byte(`{
		"model":"ignored",
		"messages":[
			{"role":"user","content":"hi"},
			{"role":"developer","content":"be brief"},
			{"role":"assistant","content":"ok"}
		]
	}`)
	out := ConvertOpenAIRequestToOpenAI("gpt-5.5", in, false)
	if got := gjson.GetBytes(out, "model").String(); got != "gpt-5.5" {
		t.Fatalf("model=%q", got)
	}
	msgs := gjson.GetBytes(out, "messages").Array()
	if len(msgs) != 3 {
		t.Fatalf("messages len=%d", len(msgs))
	}
	if got := msgs[0].Get("role").String(); got != "system" {
		t.Fatalf("messages[0].role=%q, want system (developer moved to front)", got)
	}
	if got := msgs[0].Get("content").String(); got != "be brief" {
		t.Fatalf("messages[0].content=%q", got)
	}
	if got := msgs[1].Get("role").String(); got != "user" {
		t.Fatalf("messages[1].role=%q", got)
	}
}

func TestNormalizeOpenAIChatMessages_SystemAlreadyFirst(t *testing.T) {
	in := []byte(`{"messages":[{"role":"system","content":"a"},{"role":"user","content":"b"}]}`)
	out := normalizeOpenAIChatMessages(in)
	if string(out) != string(in) {
		t.Fatalf("expected unchanged body, got %s", string(out))
	}
}
