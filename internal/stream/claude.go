package stream

import (
	"encoding/json"
	"sort"
)

// ClaudeParser 实现 Claude Messages SSE / message JSON 的解析逻辑
type ClaudeParser struct{}

// FinalMessageFromEvents 尝试把 Claude 事件流还原成完整 message JSON。
//
// Claude 的收敛过程大致是:
//
//	message_start
//	    |
//	    v
//	message 基础骨架
//	    |
//	    +--> content_block_start(index=i)
//	    |       |
//	    |       v
//	    |   contentBlocks[i] = 初始 block
//	    |
//	    +--> content_block_delta(index=i)
//	    |       |
//	    |       +--> text_delta      -> 追加到 block["text"]
//	    |       +--> thinking_delta  -> 追加到 block["thinking"]
//	    |       +--> input_json_delta-> 先累积到 inputJSON[i]
//	    |
//	    +--> message_delta
//	    |       |
//	    |       +--> 更新 stop_reason / usage 等 message 级字段
//	    |
//	    +--> message_stop
//	            |
//	            v
//	        complete = true
//
// 最后再按 index 排序 contentBlocks，组装回:
// message["content"] = [...]
// 只有观察到 message_stop 时才认为流已完整结束
func (ClaudeParser) FinalMessageFromEvents(events []eventEnvelope) ([]byte, bool, error) {
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
				block = map[string]any{"type": fallbackClaudeBlockTypeFromEvent(event)}
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
			if event.Usage != (usageTokens{}) {
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

// fallbackClaudeBlockTypeFromEvent 从原始 Claude delta 事件中推断 block 类型
func fallbackClaudeBlockTypeFromEvent(event eventEnvelope) string {
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
