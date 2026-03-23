package viewer

import (
	"claude-proxy-go/internal/stream"
)

type SessionSummary struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type ViewerStatus struct {
	LatestSession      string `json:"latest_session"`
	LatestRequestCount int    `json:"latest_request_count"`
	LatestUpdatedAt    int64  `json:"latest_updated_at"`
}

type LogItemSummary struct {
	Idx   int    `json:"idx"`
	Model string `json:"model"`
}

type LogItemDetail struct {
	Idx                    int              `json:"idx"`
	Model                  string           `json:"model"`
	RequestJSON            string           `json:"request_json"`
	RequestMessageCount    int              `json:"request_message_count"`
	RequestNewMessageCount int              `json:"request_new_message_count"`
	RequestNewMessages     []map[string]any `json:"request_new_messages"`
	ResponseBlocks         []stream.Block   `json:"response_blocks"`
	ResponseTokens         stream.Tokens    `json:"response_tokens"`
	ResponseRaw            string           `json:"response_raw"`
}
