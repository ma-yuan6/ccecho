package requestview

// 请求文件解析

import (
	"encoding/json"
	"os"
)

type ParsedRequest struct {
	Model        string           `json:"model"`
	Raw          string           `json:"raw"`
	Payload      map[string]any   `json:"payload"`
	Messages     []map[string]any `json:"messages"`
	NewMessages  []map[string]any `json:"new_messages"`
	MessageCount int              `json:"message_count"`
}

// ParseRequestFile 解析请求文件
func ParseRequestFile(path string, previousPath string) (ParsedRequest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ParsedRequest{}, err
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ParsedRequest{Raw: string(raw)}, nil
	}

	currentMessages := extractMessages(payload["messages"])
	previousMessages := []map[string]any(nil)
	if previousPath != "" {
		// 只用上一条 request 做前缀对比：Claude 每次请求会带完整 messages，
		// 所以“当前轮新增内容” = current - previous 的公共前缀。
		if prevRaw, err := os.ReadFile(previousPath); err == nil {
			var prevPayload map[string]any
			if json.Unmarshal(prevRaw, &prevPayload) == nil {
				previousMessages = extractMessages(prevPayload["messages"])
			}
		}
	}

	return ParsedRequest{
		Model:        stringValue(payload["model"]),
		Raw:          string(raw),
		Payload:      payload,
		Messages:     currentMessages,
		NewMessages:  incrementalMessages(currentMessages, previousMessages),
		MessageCount: len(currentMessages),
	}, nil
}

// 提取 messages 字段
func extractMessages(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	messages := make([]map[string]any, 0, len(items))
	for _, item := range items {
		msg, ok := item.(map[string]any)
		if ok {
			messages = append(messages, msg)
		}
	}
	return messages
}

func incrementalMessages(current, previous []map[string]any) []map[string]any {
	prefix := 0
	for prefix < len(current) && prefix < len(previous) {
		// 逐条比较消息内容，找到 first diff index。
		// 这里比较 JSON 序列化结果，是为了做深比较并避免手写字段递归。
		left, _ := json.Marshal(current[prefix])
		right, _ := json.Marshal(previous[prefix])
		if string(left) != string(right) {
			break
		}
		prefix++
	}
	// 返回“当前请求相对上一请求”的新增切片，viewer 只渲染这段。
	return current[prefix:]
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
