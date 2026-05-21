package management

import "testing"

func TestInferOAuthChannelFromAuthFileName_Antigravity(t *testing.T) {
	channel := inferOAuthChannelFromAuthFileName("antigravity-fxzer8888@gmail.com.json")
	if channel != "antigravity" {
		t.Fatalf("channel = %q, want antigravity", channel)
	}
}

func TestInferOAuthChannelFromAuthFileName_Codex(t *testing.T) {
	channel := inferOAuthChannelFromAuthFileName("codex-user@example.com.json")
	if channel != "codex" {
		t.Fatalf("channel = %q, want codex", channel)
	}
}
