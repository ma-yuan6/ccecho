package viewer

import (
	"encoding/json"

	jsonutil "ccecho/internal/utils/json"
)

// renderResponseContentBlock 把不同 provider 的内容块尽量转换成可读文本。
func renderResponseContentBlock(block map[string]any) string {
	switch jsonutil.StringValue(block["type"]) {
	case "text", "input_text", "output_text":
		return jsonutil.StringValue(block["text"])
	case "thinking":
		return jsonutil.StringValue(block["thinking"])
	case "reasoning":
		if summary := jsonutil.StringValue(block["summary_text"]); summary != "" {
			return summary
		}
		if raw, err := json.Marshal(block); err == nil {
			return string(raw)
		}
	case "tool_use":
		block = jsonutil.CloneObject(block)
		delete(block, "type")
		if raw, err := json.Marshal(block); err == nil {
			return string(raw)
		}
	}
	raw, _ := json.Marshal(block)
	return string(raw)
}

// ensureClaudeBlock 获取指定索引的 Claude block，不存在时按顺序创建。
func ensureClaudeBlock(blocks map[int]*Block, order *[]int, index int, blockType string) *Block {
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

// fallbackClaudeBlockType 为仅携带 delta 类型的事件推断 viewer block 类型。
func fallbackClaudeBlockType(deltaType string) string {
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
