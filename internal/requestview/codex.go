package requestview

type CodexParser struct{}

func (CodexParser) Parse(raw []byte, payload map[string]any, previousPayload map[string]any) ParsedRequest {
	currentItems := extractObjectArray(payload["input"])
	previousItems := extractObjectArray(previousPayload["input"])

	return ParsedRequest{
		Model:        stringValue(payload["model"]),
		Raw:          string(raw),
		Payload:      payload,
		Messages:     currentItems,
		NewMessages:  incrementalMessages(currentItems, previousItems),
		MessageCount: len(currentItems),
		InputMode:    "input",
	}
}
