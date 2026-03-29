package viewer

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"claude-proxy-go/internal/requestview"
	"claude-proxy-go/internal/sessionmeta"
)

// Service 提供日志会话查看页及对应的 JSON API。
type Service struct {
	logDir string
}

// NewService 创建一个基于日志目录的 viewer 服务。
func NewService(logDir string) *Service {
	return &Service{logDir: logDir}
}

// Register 将 viewer 页面和 API 注册到传入的 ServeMux。
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleIndex)
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(mustAssetSubtree()))))
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/session/", s.handleSessionItems)
	mux.HandleFunc("/api/detail/", s.handleItemDetail)
}

// handleIndex 返回内嵌的 viewer 首页资源。
func (s *Service) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data, err := assetFS.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, "viewer index not found", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(data)
}

// handleStatus 返回最近一次会话的简要状态。
func (s *Service) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.status())
}

// handleSessions 返回所有可用会话的摘要列表。
func (s *Service) handleSessions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.sessions())
}

// handleSessionItems 返回指定会话中的请求条目列表。
func (s *Service) handleSessionItems(w http.ResponseWriter, r *http.Request) {
	name := stringsTrimPrefix(r.URL.Path, "/api/session/")
	writeJSON(w, s.sessionItems(name))
}

// handleItemDetail 返回指定会话中单条请求的完整详情。
func (s *Service) handleItemDetail(w http.ResponseWriter, r *http.Request) {
	session, idx, ok := parseDetailPath(stringsTrimPrefix(r.URL.Path, "/api/detail/"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	item, ok := s.itemDetail(session, idx)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, item)
}

// sessions 扫描日志目录并返回所有可展示的会话，按会话名倒序排列。
func (s *Service) sessions() []SessionSummary {
	entries, err := os.ReadDir(s.logDir)
	if err != nil {
		return nil
	}
	result := make([]SessionSummary, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(s.logDir, entry.Name())
		meta, err := sessionmeta.Read(dir)
		if err != nil {
			continue
		}
		count := len(globNames(dir, "request*.json"))
		result = append(result, SessionSummary{Name: entry.Name(), Count: count, Provider: meta.Provider})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name > result[j].Name })
	return result
}

// status 返回最新会话的状态摘要，供首页快速轮询使用。
func (s *Service) status() ViewerStatus {
	sessions := s.sessions()
	if len(sessions) == 0 {
		return ViewerStatus{}
	}
	name := sessions[0].Name
	dir := filepath.Join(s.logDir, name)
	return ViewerStatus{
		LatestSession:      name,
		LatestRequestCount: len(globNames(dir, "request*.json")),
		LatestUpdatedAt:    latestUpdatedAt(dir),
	}
}

// sessionItems 读取指定会话下的请求文件，并构造成 viewer 列表项。
func (s *Service) sessionItems(session string) []LogItemSummary {
	dir, meta, ok := s.loadSessionMeta(session)
	if !ok {
		return nil
	}
	files := globNames(dir, "request*.json")
	items := make([]LogItemSummary, 0, len(files))
	for _, name := range files {
		idx := extractIdx(name)
		if idx == 0 {
			continue
		}
		req, err := requestview.ParseRequestFile(filepath.Join(dir, name), "", meta.Provider)
		if err != nil {
			continue
		}
		items = append(items, LogItemSummary{Idx: idx, Model: req.Model, Provider: meta.Provider})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Idx < items[j].Idx })
	return items
}

// itemDetail 组合单条请求的请求体、增量消息和响应内容。
func (s *Service) itemDetail(session string, idx int) (LogItemDetail, bool) {
	dir, meta, ok := s.loadSessionMeta(session)
	if !ok {
		return LogItemDetail{}, false
	}
	requestPath := requestFilePath(dir, idx)
	if _, err := os.Stat(requestPath); err != nil {
		return LogItemDetail{}, false
	}

	previousPath := previousRequestFilePath(dir, idx)

	req, err := requestview.ParseRequestFile(requestPath, previousPath, meta.Provider)
	if err != nil {
		return LogItemDetail{}, false
	}

	resp, raw := s.readResponse(dir, idx, meta.Provider)
	return LogItemDetail{
		Idx:                    idx,
		Model:                  req.Model,
		Provider:               meta.Provider,
		RequestJSON:            req.Raw,
		RequestMessageCount:    req.MessageCount,
		RequestNewMessageCount: len(req.NewMessages),
		RequestNewMessages:     req.NewMessages,
		ResponseBlocks:         resp.Blocks,
		ResponseTokens:         resp.Tokens,
		ResponseRaw:            string(raw),
	}, true
}

// readResponse 优先读取流式响应文件；如果不存在或解析失败，再回退到最终 JSON 响应文件。
func (s *Service) readResponse(dir string, idx int, provider string) (ParsedResponse, []byte) {
	streamPath := responseStreamFilePath(dir, idx)
	if parsed, raw, err := ParseFileForProvider(streamPath, provider); err == nil {
		return parsed, raw
	}

	jsonPath := responseJSONFilePath(dir, idx)
	if raw, err := os.ReadFile(jsonPath); err == nil {
		if parsed, parseErr := ParseMessageJSONForProvider(filepath.Base(jsonPath), raw, provider); parseErr == nil {
			return parsed, raw
		}
		var parsed ParsedResponse
		if json.Unmarshal(raw, &parsed) == nil {
			return parsed, raw
		}
	}

	return ParsedResponse{}, nil
}

// loadSessionMeta 解析指定会话目录的元信息；不存在或损坏时返回 false。
func (s *Service) loadSessionMeta(session string) (string, sessionmeta.Meta, bool) {
	dir := filepath.Join(s.logDir, session)
	meta, err := sessionmeta.Read(dir)
	if err != nil {
		return "", sessionmeta.Meta{}, false
	}
	return dir, meta, true
}
