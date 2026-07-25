// Package openai provides request translation functionality for OpenAI to Gemini CLI API compatibility.
// It converts OpenAI Chat Completions requests into Gemini CLI compatible JSON using gjson/sjson only.
package chat_completions

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertOpenAIRequestToOpenAI converts an OpenAI Chat Completions request (raw JSON)
// into a complete Gemini CLI request JSON. All JSON construction uses sjson and lookups use gjson.
//
// Parameters:
//   - modelName: The name of the model to use for the request
//   - rawJSON: The raw JSON request data from the OpenAI API
//   - stream: A boolean indicating if the request is for a streaming response (unused in current implementation)
//
// Returns:
//   - []byte: The transformed request data in Gemini CLI API format
func ConvertOpenAIRequestToOpenAI(modelName string, inputRawJSON []byte, _ bool) []byte {
	updatedJSON, err := sjson.SetBytes(inputRawJSON, "model", modelName)
	if err != nil {
		return inputRawJSON
	}
	return normalizeOpenAIChatMessages(updatedJSON)
}

// normalizeOpenAIChatMessages makes OpenAI-compat upstreams more tolerant of Agent payloads:
//  1. map role "developer" → "system"
//  2. move system messages to the front (fixes "System message must be at the beginning")
func normalizeOpenAIChatMessages(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}
	arr := messages.Array()
	if len(arr) == 0 {
		return body
	}

	type msg struct {
		raw  string
		role string
	}
	items := make([]msg, 0, len(arr))
	changed := false
	for _, item := range arr {
		raw := item.Raw
		role := strings.ToLower(strings.TrimSpace(item.Get("role").String()))
		if role == "developer" {
			updated, err := sjson.Set(raw, "role", "system")
			if err == nil {
				raw = updated
				role = "system"
				changed = true
			}
		}
		items = append(items, msg{raw: raw, role: role})
	}

	system := make([]msg, 0, len(items))
	rest := make([]msg, 0, len(items))
	for _, item := range items {
		if item.role == "system" {
			system = append(system, item)
		} else {
			rest = append(rest, item)
		}
	}
	reordered := append(system, rest...)
	if !changed {
		sameOrder := true
		for i := range items {
			if items[i].raw != reordered[i].raw {
				sameOrder = false
				break
			}
		}
		if sameOrder {
			return body
		}
	}

	out := make([]any, 0, len(reordered))
	for _, item := range reordered {
		out = append(out, gjson.Parse(item.raw).Value())
	}
	updated, err := sjson.SetBytes(body, "messages", out)
	if err != nil {
		return body
	}
	return updated
}
