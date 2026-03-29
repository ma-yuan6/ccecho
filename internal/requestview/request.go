package requestview

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"claude-proxy-go/internal/config"
	jsonutil "claude-proxy-go/internal/utils/json"
)

type ParsedRequest struct {
	Model        string           `json:"model"`
	Raw          string           `json:"raw"`
	Payload      map[string]any   `json:"payload"`
	Messages     []map[string]any `json:"messages"`
	NewMessages  []map[string]any `json:"new_messages"`
	MessageCount int              `json:"message_count"`
	InputMode    string           `json:"input_mode"`
}

type Parser interface {
	Parse(raw []byte, payload map[string]any, previousPayload map[string]any) ParsedRequest
}

func ParseRequestFile(path string, previousPath string, provider string) (ParsedRequest, error) {
	parser, err := ParserForProvider(provider)
	if err != nil {
		return ParsedRequest{}, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return ParsedRequest{}, err
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ParsedRequest{Raw: string(raw)}, nil
	}

	var previousPayload map[string]any
	if previousPath != "" {
		if prevRaw, err := os.ReadFile(previousPath); err == nil {
			_ = json.Unmarshal(prevRaw, &previousPayload)
		}
	}

	return parser.Parse(raw, payload, previousPayload), nil
}

func ParserForProvider(provider string) (Parser, error) {
	switch provider {
	case config.ProviderClaude, config.ProviderCodex:
		if provider == config.ProviderClaude {
			return ClaudeParser{}, nil
		}
		return CodexParser{}, nil
	default:
		return nil, fmt.Errorf("[error] unsupported request provider: %q", provider)
	}
}

func incrementalMessages(current, previous []map[string]any) []map[string]any {
	prefix := 0
	for prefix < len(current) && prefix < len(previous) {
		left, _ := json.Marshal(current[prefix])
		right, _ := json.Marshal(previous[prefix])
		if string(left) != string(right) {
			break
		}
		prefix++
	}
	return current[prefix:]
}

func extractObjectArray(value any) []map[string]any {
	return jsonutil.ExtractObjectArray(value)
}

func stringValue(value any) string {
	return jsonutil.StringValue(value)
}

func cloneObject(in map[string]any) map[string]any {
	return jsonutil.CloneObject(in)
}

func contains(s string, substr string) bool {
	return strings.Contains(s, substr)
}
