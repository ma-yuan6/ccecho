package viewer

// Block 是 viewer 展示响应内容时的最小文本块。
type Block struct {
	Index   int    `json:"index"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

// Tokens 保存一次响应相关的输入/输出 token 统计。
type Tokens struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ParsedResponse 是不同 provider 响应在 viewer 内部的统一表示。
type ParsedResponse struct {
	Source string  `json:"source"`
	Blocks []Block `json:"blocks"`
	Tokens Tokens  `json:"tokens"`
}

// SessionSummary 描述一个已捕获会话的基础信息。
type SessionSummary struct {
	Name     string `json:"name"`
	Count    int    `json:"count"`
	Provider string `json:"provider"`
}

// ViewerStatus 提供首页轮询所需的最新会话状态。
type ViewerStatus struct {
	LatestSession      string `json:"latest_session"`
	LatestRequestCount int    `json:"latest_request_count"`
	LatestUpdatedAt    int64  `json:"latest_updated_at"`
}

// LogItemSummary 是会话列表中单条请求的摘要信息。
type LogItemSummary struct {
	Idx      int    `json:"idx"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
}

// LogItemDetail 是 viewer 详情页需要的完整请求/响应数据。
type LogItemDetail struct {
	Idx                    int              `json:"idx"`
	Model                  string           `json:"model"`
	Provider               string           `json:"provider"`
	RequestJSON            string           `json:"request_json"`
	RequestMessageCount    int              `json:"request_message_count"`
	RequestNewMessageCount int              `json:"request_new_message_count"`
	RequestNewMessages     []map[string]any `json:"request_new_messages"`
	ResponseBlocks         []Block          `json:"response_blocks"`
	ResponseTokens         Tokens           `json:"response_tokens"`
	ResponseRaw            string           `json:"response_raw"`
}
