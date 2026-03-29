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

	finalJSON, complete, err := FinalMessageFromStreamForProvider(raw, "claude")
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

	finalJSON, complete, err := FinalMessageFromStreamForProvider(raw, "claude")
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

func TestFinalMessageFromStreamOpenAIResponses(t *testing.T) {
	raw := []byte("" +
		"event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"status\":\"in_progress\"}}\n\n" +
		"event: response.output_item.added\n" +
		"data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[]}}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"item_id\":\"msg_1\",\"delta\":\"hel\"}\n\n" +
		"event: response.output_text.done\n" +
		"data: {\"type\":\"response.output_text.done\",\"output_index\":0,\"content_index\":0,\"item_id\":\"msg_1\",\"text\":\"hello\"}\n\n" +
		"event: response.output_item.done\n" +
		"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n")

	finalJSON, complete, err := FinalMessageFromStreamForProvider(raw, "codex")
	if err != nil {
		t.Fatalf("FinalMessageFromStream returned error: %v", err)
	}
	if !complete {
		t.Fatal("expected complete stream after response.completed")
	}

	var payload map[string]any
	if err := json.Unmarshal(finalJSON, &payload); err != nil {
		t.Fatalf("final json is invalid: %v", err)
	}
	if payload["id"] != "resp_1" {
		t.Fatalf("unexpected id: %#v", payload["id"])
	}
	output, _ := payload["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("unexpected output length: %d", len(output))
	}
	usage, _ := payload["usage"].(map[string]any)
	if usage["output_tokens"] != float64(2) {
		t.Fatalf("unexpected usage: %#v", usage)
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

	finalJSON, complete, err := FinalMessageFromStreamForProvider(raw, "claude")
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
