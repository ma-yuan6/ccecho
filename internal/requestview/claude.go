package requestview

type ClaudeParser struct{}

func (ClaudeParser) Parse(raw []byte, payload map[string]any, previousPayload map[string]any) ParsedRequest {
	currentMessages := normalizeClaudeMessages(extractObjectArray(payload["messages"]))
	previousMessages := normalizeClaudeMessages(extractObjectArray(previousPayload["messages"]))

	return ParsedRequest{
		Model:        stringValue(payload["model"]),
		Raw:          string(raw),
		Payload:      payload,
		Messages:     currentMessages,
		NewMessages:  incrementalMessages(currentMessages, previousMessages),
		MessageCount: len(currentMessages),
		InputMode:    "messages",
	}
}

func normalizeClaudeMessages(messages []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		normalized := cloneObject(message)
		content := normalizeClaudeContent(extractObjectArray(message["content"]))
		normalized["content"] = content
		result = append(result, normalized)
	}
	return result
}

func normalizeClaudeContent(content []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(content))
	for _, block := range content {
		if isClaudeSystemReminderBlock(block) {
			continue
		}
		result = append(result, cloneObject(block))
	}
	return result
}

func isClaudeSystemReminderBlock(block map[string]any) bool {
	if stringValue(block["type"]) != "text" {
		return false
	}
	text := stringValue(block["text"])
	return len(text) > 0 && (contains(text, "<system-reminder>") || contains(text, "</system-reminder>"))
}
