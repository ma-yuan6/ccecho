package viewer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"claude-proxy-go/internal/config"
	jsonutil "claude-proxy-go/internal/utils/json"
)

// responseParser 定义了 provider 解析器统一实现的接口。
type responseParser interface {
	ParseEvents(source string, events []responseEventEnvelope) ParsedResponse
	ParseMessageJSON(source string, raw []byte) (ParsedResponse, error)
}

// responseEventEnvelope 是对流式响应事件的最小解包结构，兼容 Claude 与 Codex 的 viewer 解析需求。
type responseEventEnvelope struct {
	Type         string          `json:"type"`
	Index        int             `json:"index"`
	Message      json.RawMessage `json:"message"`
	ContentBlock json.RawMessage `json:"content_block"`
	Delta        json.RawMessage `json:"delta"`
	Item         json.RawMessage `json:"item"`
	Response     json.RawMessage `json:"response"`
	Usage        Tokens          `json:"usage"`
	OutputIndex  int             `json:"output_index"`
}

// responseContentBlockStart 对应 Claude `content_block_start` 事件结构。
type responseContentBlockStart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
}

// responseContentDelta 对应 Claude `content_block_delta` 事件结构。
type responseContentDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	PartialJSON string `json:"partial_json"`
	Signature   string `json:"signature"`
}

// ParseFileForProvider 读取响应文件，并按 provider 对其做结构化解析。
func ParseFileForProvider(path string, provider string) (ParsedResponse, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ParsedResponse{}, nil, err
	}
	parsed, err := ParseBytesForProvider(path, raw, provider)
	return parsed, raw, err
}

// ParseBytesForProvider 将 SSE 风格的原始响应字节解析为 viewer 统一结构。
func ParseBytesForProvider(source string, raw []byte, provider string) (ParsedResponse, error) {
	parser, err := parserForProvider(provider)
	if err != nil {
		return ParsedResponse{}, err
	}
	events, err := parseResponseEvents(raw)
	if err != nil {
		return ParsedResponse{}, err
	}
	return parser.ParseEvents(source, events), nil
}

// ParseMessageJSONForProvider 将最终 JSON 响应解析为 viewer 统一结构。
func ParseMessageJSONForProvider(source string, raw []byte, provider string) (ParsedResponse, error) {
	parser, err := parserForProvider(provider)
	if err != nil {
		return ParsedResponse{}, err
	}
	return parser.ParseMessageJSON(source, raw)
}

// parseResponseEvents 只提取 SSE 中的 `data:` 行，并忽略无法反序列化的事件。
func parseResponseEvents(raw []byte) ([]responseEventEnvelope, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	events := make([]responseEventEnvelope, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event responseEventEnvelope
		if err := json.Unmarshal([]byte(line[6:]), &event); err != nil {
			continue
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

// parserForProvider 选择与 provider 对应的响应解析器。
func parserForProvider(provider string) (responseParser, error) {
	switch provider {
	case config.ProviderClaude:
		return claudeResponseParser{}, nil
	case config.ProviderCodex:
		return codexResponseParser{}, nil
	default:
		return nil, fmt.Errorf("[error] unsupported viewer response provider: %q", provider)
	}
}

// claudeResponseParser 负责解析 Claude 响应事件与最终 JSON。
type claudeResponseParser struct{}

// ParseMessageJSON 解析 Claude 非流式 message JSON 响应。
func (claudeResponseParser) ParseMessageJSON(source string, raw []byte) (ParsedResponse, error) {
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
		content := strings.TrimSpace(renderResponseContentBlock(item))
		if content == "" {
			continue
		}
		result.Blocks = append(result.Blocks, Block{
			Index:   idx,
			Type:    jsonutil.StringValue(item["type"]),
			Content: content,
		})
	}
	return result, nil
}

// ParseEvents 解析 Claude SSE 事件流，并按 content block 聚合文本内容。
func (claudeResponseParser) ParseEvents(source string, events []responseEventEnvelope) ParsedResponse {
	blocks := make(map[int]*Block)
	order := make([]int, 0)
	result := ParsedResponse{
		Source: source,
		Tokens: Tokens{},
	}

	for _, event := range events {
		switch event.Type {
		case "content_block_start":
			var start responseContentBlockStart
			_ = json.Unmarshal(event.ContentBlock, &start)
			block := ensureClaudeBlock(blocks, &order, event.Index, start.Type)
			block.Content += start.Text + start.Thinking
		case "content_block_delta":
			var delta responseContentDelta
			_ = json.Unmarshal(event.Delta, &delta)
			block := ensureClaudeBlock(blocks, &order, event.Index, fallbackClaudeBlockType(delta.Type))
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

// codexResponseParser 负责解析 Codex/OpenAI Responses 风格响应。
type codexResponseParser struct{}

// ParseMessageJSON 解析 Codex/OpenAI Responses 风格的最终 JSON 响应。
func (codexResponseParser) ParseMessageJSON(source string, raw []byte) (ParsedResponse, error) {
	var payload struct {
		Output []map[string]any `json:"output"`
		Usage  Tokens           `json:"usage"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ParsedResponse{}, err
	}
	return parsedResponseFromCodexOutput(source, payload.Output, payload.Usage), nil
}

// ParseEvents 从 Codex/OpenAI Responses 事件流中恢复 output items 和 usage。
func (codexResponseParser) ParseEvents(source string, events []responseEventEnvelope) ParsedResponse {
	items, usage := collectCodexOutputItems(events)
	return parsedResponseFromCodexOutput(source, items, usage)
}

// parsedResponseFromCodexOutput 将 output items 展平成 viewer 可消费的 block 列表。
func parsedResponseFromCodexOutput(source string, output []map[string]any, usage Tokens) ParsedResponse {
	result := ParsedResponse{
		Source: source,
		Tokens: usage,
	}
	blockIndex := 0
	for _, item := range output {
		for _, block := range codexBlocksFromItem(item, &blockIndex) {
			result.Blocks = append(result.Blocks, block)
		}
	}
	return result
}

// collectCodexOutputItems 从增量事件中收集 output item；若拿到 completed 事件则优先采用完整快照。
func collectCodexOutputItems(events []responseEventEnvelope) ([]map[string]any, Tokens) {
	byIndex := make(map[int]map[string]any)
	order := make([]int, 0)
	usage := Tokens{}

	for _, event := range events {
		switch event.Type {
		case "response.output_item.added", "response.output_item.done":
			if len(event.Item) == 0 {
				continue
			}
			var item map[string]any
			if err := json.Unmarshal(event.Item, &item); err != nil {
				continue
			}
			if _, ok := byIndex[event.OutputIndex]; !ok {
				order = append(order, event.OutputIndex)
			}
			byIndex[event.OutputIndex] = jsonutil.CloneObject(item)
		case "response.completed":
			var payload struct {
				Output []map[string]any `json:"output"`
				Usage  Tokens           `json:"usage"`
			}
			if len(event.Response) == 0 {
				continue
			}
			if err := json.Unmarshal(event.Response, &payload); err != nil {
				continue
			}
			if payload.Usage != (Tokens{}) {
				usage = payload.Usage
			}
			if len(payload.Output) == 0 {
				continue
			}
			items := make([]map[string]any, 0, len(payload.Output))
			for _, item := range payload.Output {
				items = append(items, jsonutil.CloneObject(item))
			}
			return items, usage
		}
	}

	sort.Ints(order)
	items := make([]map[string]any, 0, len(order))
	for _, idx := range order {
		item := byIndex[idx]
		if item == nil {
			continue
		}
		items = append(items, jsonutil.CloneObject(item))
	}
	return items, usage
}

// codexBlocksFromItem 将单个 output item 拆成一个或多个 viewer block。
func codexBlocksFromItem(item map[string]any, nextIndex *int) []Block {
	if item == nil {
		return nil
	}
	contentItems := jsonutil.ExtractObjectArray(item["content"])
	if len(contentItems) == 0 {
		text := strings.TrimSpace(renderResponseContentBlock(item))
		if text == "" {
			return nil
		}
		block := Block{
			Index:   *nextIndex,
			Type:    jsonutil.StringValue(item["type"]),
			Content: text,
		}
		*nextIndex++
		return []Block{block}
	}

	blocks := make([]Block, 0, len(contentItems))
	for _, part := range contentItems {
		text := strings.TrimSpace(renderResponseContentBlock(part))
		if text == "" {
			continue
		}
		block := Block{
			Index:   *nextIndex,
			Type:    jsonutil.StringValue(part["type"]),
			Content: text,
		}
		*nextIndex++
		blocks = append(blocks, block)
	}
	return blocks
}
