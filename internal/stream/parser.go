// Package stream 负责把 Claude / Codex 的原始响应流收敛成最终 JSON
// 它不参与代理转发，也不负责 viewer 展示模型的构造

package stream

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"claude-proxy-go/internal/config"
	jsonutil "claude-proxy-go/internal/utils/json"
)

type usageTokens struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Parser 定义单个 provider 的最终 JSON 收敛能力
type Parser interface {
	FinalMessageFromEvents(events []eventEnvelope) ([]byte, bool, error)
}

type eventEnvelope struct {
	Type         string          `json:"type"`
	Index        int             `json:"index"`
	Message      json.RawMessage `json:"message"`
	ContentBlock json.RawMessage `json:"content_block"`
	Delta        json.RawMessage `json:"delta"`
	Item         json.RawMessage `json:"item"`
	Response     json.RawMessage `json:"response"`
	Usage        usageTokens     `json:"usage"`
	OutputIndex  int             `json:"output_index"`
}

type contentDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	PartialJSON string `json:"partial_json"`
	Signature   string `json:"signature"`
}

// FinalMessageFromStreamForProvider 尝试从一段完整事件流中还原最终消息 JSON
//
// 处理路径:
//
//	raw stream
//	    |
//	    v
//	parseEvents(raw)
//	    |
//	    v
//	[]eventEnvelope
//	    |
//	    v
//	providerParser.FinalMessageFromEvents(events)
//	    |
//	    +--> complete=true  -> 返回最终 JSON
//	    |
//	    +--> complete=false -> 返回 nil，表示流还不完整
//
// 当流尚未结束时，complete=false，返回值为 nil
func FinalMessageFromStreamForProvider(raw []byte, provider string) ([]byte, bool, error) {
	parser, err := ParserForProvider(provider) // 通过代理的工具 codex/calude 获取不同的解析器
	if err != nil {
		return nil, false, err
	}
	events, err := parseEvents(raw) // 解析sse 结构的数据（这里是通用的）
	if err != nil {
		return nil, false, err
	}
	return parser.FinalMessageFromEvents(events) // 使用解析器解析处理过的sse数据
}

// parseEvents 从 SSE 文本中提取每个 "data: ..." 事件的 JSON 包体
func parseEvents(raw []byte) ([]eventEnvelope, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 8*1024*8), 8*1024*1024)
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

// cloneMap 复制 map，避免后续解析过程中意外修改原始对象
func cloneMap(in map[string]any) map[string]any {
	return jsonutil.CloneObject(in)
}

// stringValue 安全提取任意值中的字符串内容
func stringValue(value any) string {
	return jsonutil.StringValue(value)
}

// ParserForProvider 返回对应 provider 的流解析器实现
func ParserForProvider(provider string) (Parser, error) {
	switch provider {
	case config.ProviderClaude:
		return ClaudeParser{}, nil
	case config.ProviderCodex:
		return CodexParser{}, nil
	default:
		return nil, fmt.Errorf("[error] unsupported stream provider: %q", provider)
	}
}
