package stream

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestFinalMessageFromStreamCompletesOnMessageStop(t *testing.T) {
	raw := []byte("" +
		"event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-test\",\"content\":[],\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":12,\"output_tokens\":1}}}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n" +
		"event: message_delta\n" +
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"input_tokens\":12,\"output_tokens\":34}}\n\n" +
		"event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n")

	finalJSON, complete, err := FinalMessageFromStream(raw)
	if err != nil {
		t.Fatalf("FinalMessageFromStream returned error: %v", err)
	}
	if !complete {
		t.Fatal("expected stream to be complete after message_stop")
	}

	var payload map[string]any
	if err := json.Unmarshal(finalJSON, &payload); err != nil {
		t.Fatalf("final json is invalid: %v", err)
	}
	if payload["id"] != "msg_1" {
		t.Fatalf("unexpected id: %#v", payload["id"])
	}
	if payload["stop_reason"] != "end_turn" {
		t.Fatalf("unexpected stop_reason: %#v", payload["stop_reason"])
	}
	content, _ := payload["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("unexpected content length: %d", len(content))
	}
	block, _ := content[0].(map[string]any)
	if block["text"] != "hello world" {
		t.Fatalf("unexpected text block: %#v", block["text"])
	}
}

func TestFinalMessageFromStreamRequiresMessageStop(t *testing.T) {
	raw := []byte("" +
		"event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-test\",\"content\":[]}}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"partial\"}}\n\n")

	finalJSON, complete, err := FinalMessageFromStream(raw)
	if err != nil {
		t.Fatalf("FinalMessageFromStream returned error: %v", err)
	}
	if complete {
		t.Fatal("expected incomplete stream without message_stop")
	}
	if finalJSON != nil {
		t.Fatal("expected no final json for incomplete stream")
	}
}

func TestParseMessageJSON(t *testing.T) {
	raw := []byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"echo","input":{"value":1}}],"usage":{"input_tokens":3,"output_tokens":5}}`)

	parsed, err := ParseMessageJSON("response1.json", raw)
	if err != nil {
		t.Fatalf("ParseMessageJSON returned error: %v", err)
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

func TestFinalMessageFromStreamWithLargeEventLine(t *testing.T) {
	large := string(bytes.Repeat([]byte("x"), 200*1024))
	raw := []byte("" +
		"event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-test\",\"content\":[]}}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"" + large + "\"}}\n\n" +
		"event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n")

	finalJSON, complete, err := FinalMessageFromStream(raw)
	if err != nil {
		t.Fatalf("FinalMessageFromStream returned error: %v", err)
	}
	if !complete {
		t.Fatal("expected complete stream")
	}

	var payload map[string]any
	if err := json.Unmarshal(finalJSON, &payload); err != nil {
		t.Fatalf("final json is invalid: %v", err)
	}
	content, _ := payload["content"].([]any)
	block, _ := content[0].(map[string]any)
	if got, _ := block["text"].(string); got != large {
		t.Fatalf("large text was truncated: got %d want %d", len(got), len(large))
	}
}
