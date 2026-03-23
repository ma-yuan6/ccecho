package stream

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Block struct {
	Index   int    `json:"index"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type Tokens struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ParsedResponse struct {
	Source string  `json:"source"`
	Blocks []Block `json:"blocks"`
	Tokens Tokens  `json:"tokens"`
}

type eventEnvelope struct {
	Type         string          `json:"type"`
	Index        int             `json:"index"`
	Message      json.RawMessage `json:"message"`
	ContentBlock json.RawMessage `json:"content_block"`
	Delta        json.RawMessage `json:"delta"`
	Usage        Tokens          `json:"usage"`
}

type contentBlockStart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
}

type contentDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	PartialJSON string `json:"partial_json"`
	Signature   string `json:"signature"`
}

func ParseFile(path string) (ParsedResponse, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ParsedResponse{}, nil, err
	}
	parsed, err := ParseBytes(filepath.Base(path), raw)
	return parsed, raw, err
}

func ParseBytes(source string, raw []byte) (ParsedResponse, error) {
	events, err := parseEvents(raw)
	if err != nil {
		return ParsedResponse{}, err
	}
	return parseResponseFromEvents(source, events), nil
}

func ParseMessageJSON(source string, raw []byte) (ParsedResponse, error) {
	var payload struct {
		Content []map[string]any `json:"content"`
		Usage   Tokens           `json:"usage"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ParsedResponse{}, err
	}

	result := ParsedResponse{
		Source: source,
		Tokens: payload.Usage,
	}
	for idx, item := range payload.Content {
		blockType, _ := item["type"].(string)
		content := renderContentBlock(item)
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		result.Blocks = append(result.Blocks, Block{
			Index:   idx,
			Type:    blockType,
			Content: content,
		})
	}
	return result, nil
}

func FinalMessageFromStream(raw []byte) ([]byte, bool, error) {
	events, err := parseEvents(raw)
	if err != nil {
		return nil, false, err
	}
	return finalMessageFromEvents(events)
}

func parseEvents(raw []byte) ([]eventEnvelope, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	events := make([]eventEnvelope, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event eventEnvelope
		if err := json.Unmarshal([]byte(line[6:]), &event); err != nil {
			continue
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

func parseResponseFromEvents(source string, events []eventEnvelope) ParsedResponse {
	blocks := make(map[int]*Block)
	order := make([]int, 0)
	result := ParsedResponse{
		Source: source,
		Tokens: Tokens{},
	}

	for _, event := range events {
		switch event.Type {
		case "content_block_start":
			var start contentBlockStart
			_ = json.Unmarshal(event.ContentBlock, &start)
			block := ensureBlock(blocks, &order, event.Index, start.Type)
			if start.Text != "" {
				block.Content += start.Text
			}
			if start.Thinking != "" {
				block.Content += start.Thinking
			}
		case "content_block_delta":
			var delta contentDelta
			_ = json.Unmarshal(event.Delta, &delta)
			blockType := fallbackBlockType(delta.Type)
			block := ensureBlock(blocks, &order, event.Index, blockType)
			switch delta.Type {
			case "thinking_delta":
				block.Content += delta.Thinking
			case "text_delta":
				block.Content += delta.Text
			case "input_json_delta":
				block.Content += delta.PartialJSON
			}
		case "message_delta":
			if event.Usage.InputTokens != 0 {
				result.Tokens.InputTokens = event.Usage.InputTokens
			}
			if event.Usage.OutputTokens != 0 {
				result.Tokens.OutputTokens = event.Usage.OutputTokens
			}
		}
	}

	for _, idx := range order {
		block := blocks[idx]
		block.Content = strings.TrimSpace(block.Content)
		if block.Content == "" {
			continue
		}
		result.Blocks = append(result.Blocks, *block)
	}
	return result
}

func ensureBlock(blocks map[int]*Block, order *[]int, index int, blockType string) *Block {
	if block, ok := blocks[index]; ok {
		if block.Type == "" {
			block.Type = blockType
		}
		return block
	}
	block := &Block{Index: index, Type: blockType}
	blocks[index] = block
	*order = append(*order, index)
	return block
}

func fallbackBlockType(deltaType string) string {
	switch deltaType {
	case "thinking_delta":
		return "thinking"
	case "text_delta":
		return "text"
	case "input_json_delta":
		return "input_json"
	default:
		return "unknown"
	}
}

func finalMessageFromEvents(events []eventEnvelope) ([]byte, bool, error) {
	var message map[string]any
	contentBlocks := make(map[int]map[string]any)
	inputJSON := make(map[int]string)
	complete := false

	for _, event := range events {
		switch event.Type {
		case "message_start":
			if len(event.Message) == 0 {
				continue
			}
			_ = json.Unmarshal(event.Message, &message)
			if message == nil {
				message = map[string]any{}
			}
		case "content_block_start":
			var block map[string]any
			if err := json.Unmarshal(event.ContentBlock, &block); err != nil {
				continue
			}
			contentBlocks[event.Index] = cloneMap(block)
		case "content_block_delta":
			block := contentBlocks[event.Index]
			if block == nil {
				block = map[string]any{
					"type": fallbackBlockTypeFromIndex(event),
				}
				contentBlocks[event.Index] = block
			}
			var delta contentDelta
			if err := json.Unmarshal(event.Delta, &delta); err != nil {
				continue
			}
			switch delta.Type {
			case "text_delta":
				block["type"] = "text"
				block["text"] = stringValue(block["text"]) + delta.Text
			case "thinking_delta":
				block["type"] = "thinking"
				block["thinking"] = stringValue(block["thinking"]) + delta.Thinking
			case "input_json_delta":
				block["type"] = "tool_use"
				inputJSON[event.Index] += delta.PartialJSON
			case "signature_delta":
				block["signature"] = stringValue(block["signature"]) + delta.Signature
			}
		case "message_delta":
			if message == nil {
				message = map[string]any{}
			}
			if len(event.Delta) > 0 {
				var delta map[string]any
				if err := json.Unmarshal(event.Delta, &delta); err == nil {
					for key, value := range delta {
						message[key] = value
					}
				}
			}
			if event.Usage != (Tokens{}) {
				message["usage"] = event.Usage
			}
		case "message_stop":
			complete = true
		}
	}

	if !complete || message == nil {
		return nil, false, nil
	}

	indexes := make([]int, 0, len(contentBlocks))
	for idx := range contentBlocks {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	content := make([]map[string]any, 0, len(indexes))
	for _, idx := range indexes {
		block := cloneMap(contentBlocks[idx])
		if raw := inputJSON[idx]; raw != "" {
			var parsed any
			if json.Unmarshal([]byte(raw), &parsed) == nil {
				block["input"] = parsed
			} else {
				block["input"] = raw
			}
		}
		content = append(content, block)
	}
	message["content"] = content

	payload, err := json.Marshal(message)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func renderContentBlock(block map[string]any) string {
	switch stringValue(block["type"]) {
	case "text":
		return stringValue(block["text"])
	case "thinking":
		return stringValue(block["thinking"])
	case "tool_use":
		delete(block, "type")
		if raw, err := json.Marshal(block); err == nil {
			return string(raw)
		}
	}
	raw, _ := json.Marshal(block)
	return string(raw)
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func fallbackBlockTypeFromIndex(event eventEnvelope) string {
	var delta contentDelta
	if err := json.Unmarshal(event.Delta, &delta); err != nil {
		return "unknown"
	}
	switch delta.Type {
	case "text_delta":
		return "text"
	case "thinking_delta":
		return "thinking"
	case "input_json_delta":
		return "tool_use"
	default:
		return "unknown"
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
