package viewer

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"claude-proxy-go/internal/requestview"
	"claude-proxy-go/internal/stream"
)

var idxPattern = regexp.MustCompile(`(\d+)`)

type Service struct {
	logDir string
}

func NewService(logDir string) *Service {
	return &Service{logDir: logDir}
}

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleIndex)
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(mustAssetSubtree()))))
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/session/", s.handleSessionItems)
	mux.HandleFunc("/api/detail/", s.handleItemDetail)
}

func (s *Service) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data, err := assetFS.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, "viewer index not found", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(data)
}

func (s *Service) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.status())
}

func (s *Service) handleSessions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.sessions())
}

func (s *Service) handleSessionItems(w http.ResponseWriter, r *http.Request) {
	name := stringsTrimPrefix(r.URL.Path, "/api/session/")
	writeJSON(w, s.sessionItems(name))
}

func (s *Service) handleItemDetail(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(stringsTrimPrefix(r.URL.Path, "/api/detail/"))
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	idx, err := strconv.Atoi(parts[1])
	if err != nil {
		http.NotFound(w, r)
		return
	}

	item, ok := s.itemDetail(parts[0], idx)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, item)
}

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
		count := len(globNames(dir, "request*.json"))
		result = append(result, SessionSummary{Name: entry.Name(), Count: count})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name > result[j].Name })
	return result
}

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

func (s *Service) sessionItems(session string) []LogItemSummary {
	dir := filepath.Join(s.logDir, session)
	files := globNames(dir, "request*.json")
	items := make([]LogItemSummary, 0, len(files))
	for _, name := range files {
		idx := extractIdx(name)
		if idx == 0 {
			continue
		}
		req, err := requestview.ParseRequestFile(filepath.Join(dir, name), "")
		if err != nil {
			continue
		}
		items = append(items, LogItemSummary{Idx: idx, Model: req.Model})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Idx < items[j].Idx })
	return items
}

func (s *Service) itemDetail(session string, idx int) (LogItemDetail, bool) {
	dir := filepath.Join(s.logDir, session)
	requestPath := filepath.Join(dir, fmt.Sprintf("request%d.json", idx))
	if _, err := os.Stat(requestPath); err != nil {
		return LogItemDetail{}, false
	}

	previousPath := ""
	if idx > 1 {
		path := filepath.Join(dir, fmt.Sprintf("request%d.json", idx-1))
		if _, err := os.Stat(path); err == nil {
			previousPath = path
		}
	}

	req, err := requestview.ParseRequestFile(requestPath, previousPath)
	if err != nil {
		return LogItemDetail{}, false
	}

	resp, raw := s.readResponse(dir, idx)
	return LogItemDetail{
		Idx:                    idx,
		Model:                  req.Model,
		RequestJSON:            req.Raw,
		RequestMessageCount:    req.MessageCount,
		RequestNewMessageCount: len(req.NewMessages),
		RequestNewMessages:     req.NewMessages,
		ResponseBlocks:         resp.Blocks,
		ResponseTokens:         resp.Tokens,
		ResponseRaw:            string(raw),
	}, true
}

func (s *Service) readResponse(dir string, idx int) (stream.ParsedResponse, []byte) {
	streamPath := filepath.Join(dir, fmt.Sprintf("response%d.stream", idx))
	if parsed, raw, err := stream.ParseFile(streamPath); err == nil {
		return parsed, raw
	}

	jsonPath := filepath.Join(dir, fmt.Sprintf("response%d.json", idx))
	if raw, err := os.ReadFile(jsonPath); err == nil {
		if parsed, parseErr := stream.ParseMessageJSON(filepath.Base(jsonPath), raw); parseErr == nil {
			return parsed, raw
		}
		var parsed stream.ParsedResponse
		if json.Unmarshal(raw, &parsed) == nil {
			return parsed, raw
		}
	}

	return stream.ParsedResponse{}, nil
}

func globNames(dir string, pattern string) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, filepath.Base(match))
	}
	sort.Strings(names)
	return names
}

func extractIdx(name string) int {
	match := idxPattern.FindStringSubmatch(name)
	if len(match) != 2 {
		return 0
	}
	idx, _ := strconv.Atoi(match[1])
	return idx
}

func latestUpdatedAt(dir string) int64 {
	info, err := os.Stat(dir)
	if err != nil {
		return 0
	}
	latest := info.ModTime().Unix()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return latest
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if ts := info.ModTime().Unix(); ts > latest {
			latest = ts
		}
	}
	return latest
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func stringsTrimPrefix(value string, prefix string) string {
	if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
		return value[len(prefix):]
	}
	return value
}

func splitPath(path string) []string {
	raw := filepath.Clean(path)
	if raw == "." || raw == "/" {
		return nil
	}
	parts := make([]string, 0)
	for _, item := range strings.Split(raw, string(filepath.Separator)) {
		if item != "" && item != "." {
			parts = append(parts, item)
		}
	}
	return parts
}

func mustAssetSubtree() fs.FS {
	subtree, err := fs.Sub(assetFS, "assets")
	if err != nil {
		panic(err)
	}
	return subtree
}
