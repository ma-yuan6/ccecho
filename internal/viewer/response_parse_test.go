package viewer

import "testing"

func TestParseMessageJSONClaude(t *testing.T) {
	raw := []byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"echo","input":{"value":1}}],"usage":{"input_tokens":3,"output_tokens":5}}`)

	parsed, err := ParseMessageJSONForProvider("response1.json", raw, "claude")
	if err != nil {
		t.Fatalf("ParseMessageJSONForProvider returned error: %v", err)
	}
	if len(parsed.Blocks) != 2 {
		t.Fatalf("unexpected block count: %d", len(parsed.Blocks))
	}
	if parsed.Blocks[0].Content != "ok" {
		t.Fatalf("unexpected first block content: %#v", parsed.Blocks[0].Content)
	}
	if parsed.Tokens.OutputTokens != 5 {
		t.Fatalf("unexpected output tokens: %d", parsed.Tokens.OutputTokens)
	}
}

func TestParseMessageJSONCodex(t *testing.T) {
	raw := []byte(`{"id":"resp_1","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]},{"id":"fc_1","type":"function_call","name":"ls","arguments":"{\"path\":\".\"}"}],"usage":{"input_tokens":7,"output_tokens":9}}`)

	parsed, err := ParseMessageJSONForProvider("response1.json", raw, "codex")
	if err != nil {
		t.Fatalf("ParseMessageJSONForProvider returned error: %v", err)
	}
	if len(parsed.Blocks) != 2 {
		t.Fatalf("unexpected block count: %d", len(parsed.Blocks))
	}
	if parsed.Blocks[0].Type != "output_text" || parsed.Blocks[0].Content != "ok" {
		t.Fatalf("unexpected first block: %#v", parsed.Blocks[0])
	}
	if parsed.Blocks[1].Type != "function_call" {
		t.Fatalf("unexpected second block type: %#v", parsed.Blocks[1].Type)
	}
	if parsed.Tokens.OutputTokens != 9 {
		t.Fatalf("unexpected output tokens: %d", parsed.Tokens.OutputTokens)
	}
}
