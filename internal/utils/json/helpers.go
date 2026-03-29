package jsonutil

// ExtractObjectArray 将任意切片值中过滤为 []map[string]any
func ExtractObjectArray(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result
}

// CloneObject 做一层 map 复制，避免调用方修改原始 JSON 结构
func CloneObject(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// StringValue 在宽松 JSON 结构上安全提取字符串字段
func StringValue(value any) string {
	text, _ := value.(string)
	return text
}
