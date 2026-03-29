package stream

import (
	"encoding/json"
)

// CodexParser 实现 Codex / OpenAI Responses 事件流与最终 JSON 的解析逻辑。
type CodexParser struct{}

// FinalMessageFromEvents 从 Codex 事件流中提取 response.completed 携带的最终响应 JSON。
//
// Codex / OpenAI Responses 的收敛过程比 Claude 更直接:
//
//	response.created
//	response.output_item.added
//	response.output_text.delta
//	response.output_item.done
//	...
//	response.completed
//	    |
//	    v
//	event.Response
//	    |
//	    v
//	最终完整 response JSON
//
// 这里不需要像 Claude 那样手动逐块重建文本；
// 只要拿到 response.completed，就能直接取出最终 response。
func (CodexParser) FinalMessageFromEvents(events []eventEnvelope) ([]byte, bool, error) {
	for _, event := range events {
		if event.Type != "response.completed" || len(event.Response) == 0 {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(event.Response, &payload); err != nil {
			return nil, false, err
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, false, err
		}
		return raw, true, nil
	}
	return nil, false, nil
}
