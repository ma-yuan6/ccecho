package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"ccecho/internal/config"
	"ccecho/internal/logstore"
	"ccecho/internal/stream"
)

type Server struct {
	target      config.Target
	baseURL     *url.URL
	store       *logstore.Store
	client      *http.Client
	routePrefix string
}

// New 创建一个反向代理服务，用于将请求转发到配置的上游地址，
// 并将请求与响应内容落盘到 logstore 中。
func New(target config.Target, store *logstore.Store, routePrefix string) *Server {
	return &Server{
		target:      target,
		baseURL:     target.BaseURL,
		store:       store,
		client:      &http.Client{},
		routePrefix: strings.TrimRight(routePrefix, "/"),
	}
}

// Handler 将代理入口暴露为标准的 http.Handler。
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.handle)
}

// w 当前这次 HTTP 请求对应的响应写入器，往 w 里写什么 codex/claude 就收到什么
// req 当前客户端发给代理服务的请求对象 (codex/claude 的请求)

func (s *Server) handle(w http.ResponseWriter, req *http.Request) {
	if !s.matches(req.URL.Path) {
		http.NotFound(w, req)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "[error] read request body failed", http.StatusBadRequest)
		return
	}
	_ = req.Body.Close()

	idx := s.store.NextIndex()
	// 先记录原始请求，再发往上游；这样即使上游调用失败，也能回溯对应输入
	if err := s.store.WriteRequest(idx, body); err != nil {
		log.Printf("[error] write request %d: %v", idx, err)
	}
	//else {
	//	log.Printf("saved request %d", idx)
	//}

	upstreamURL := s.upstreamURL(req.URL)
	upstreamReq, err := http.NewRequestWithContext(context.Background(), req.Method, upstreamURL.String(), bytes.NewReader(body))
	if err != nil {
		s.writeErrorResponse(idx, "build_upstream_request", err)
		http.Error(w, "[error] build upstream request failed", http.StatusInternalServerError)
		return
	}
	copyHeaders(upstreamReq.Header, req.Header)
	upstreamReq.Host = s.baseURL.Host
	upstreamReq.ContentLength = int64(len(body))

	resp, err := s.client.Do(upstreamReq)
	if err != nil {
		s.writeErrorResponse(idx, "upstream_request", err)
		http.Error(w, fmt.Sprintf("[error] upstream request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	isEventStream := strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
	var captured bytes.Buffer
	defer func() {
		data := captured.Bytes()
		if isEventStream {
			// 如果客户端在流式响应中途中断，只保留完整的 SSE 事件，
			// 避免后续解析时读到截断的尾部数据。
			data = trimToCompleteSSEEvents(data)
			if err := s.store.WriteResponseStream(idx, data); err != nil {
				log.Printf("[error] write response %d stream: %v", idx, err)
			}
			// 当 provider 对应的流解析器能够还原出完整消息时，
			// 额外保存一份聚合后的最终 JSON 响应。
			finalJSON, complete, err := stream.FinalMessageFromStreamForProvider(data, s.target.Provider)
			if err != nil {
				log.Printf("[error] convert response %d: %v", idx, err)
			}
			if complete {
				if err := s.store.WriteResponseJSON(idx, finalJSON); err != nil {
					log.Printf("[error] write response %d json: %v", idx, err)
				}
			}
			return
		}
		if err := s.store.WriteResponseJSON(idx, data); err != nil {
			log.Printf("[error] write response %d: %v", idx, err)
		}
	}()

	writer := &captureWriter{
		idx:             idx,
		req:             req,
		captured:        &captured,
		downstream:      w,
		flushDownstream: flusher,
	}
	if _, err := io.CopyBuffer(writer, resp.Body, make([]byte, 32*1024)); err != nil {
		// 请求被取消通常表示下游客户端已经断开；上面的 defer 仍会把当前已捕获的数据写入存储
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Printf("[error] read upstream response %d: %v", idx, err)
		return
	}
}

// trimToCompleteSSEEvents 会裁掉结尾不完整的 SSE 事件，保证落盘内容总是结束在事件边界上
func trimToCompleteSSEEvents(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}

	if idx := bytes.LastIndex(raw, []byte("\r\n\r\n")); idx >= 0 {
		return raw[:idx+4]
	}
	if idx := bytes.LastIndex(raw, []byte("\n\n")); idx >= 0 {
		return raw[:idx+2]
	}
	return nil
}

// writeErrorResponse 将代理侧错误写入同一个响应槽位，便于回放请求或排查问题时看到失败原因
func (s *Server) writeErrorResponse(idx int, stage string, err error) {
	payload, marshalErr := json.Marshal(map[string]string{
		"type":  "ccecho_error",
		"stage": stage,
		"error": err.Error(),
	})
	if marshalErr != nil {
		log.Printf("[error] marshal error response %d: %v", idx, marshalErr)
		return
	}
	if writeErr := s.store.WriteResponseJSON(idx, payload); writeErr != nil {
		log.Printf("[error] write response %d: %v", idx, writeErr)
	}
}

// matches 用于判断请求路径在去掉本地路由前缀后，是否命中了当前配置的上游路径
func (s *Server) matches(path string) bool {
	path = s.stripRoutePrefix(path)
	if path == "" {
		return false
	}
	basePath := strings.TrimRight(s.baseURL.Path, "/")
	requestPath := strings.TrimRight(path, "/")
	if basePath == "" {
		return strings.HasPrefix(requestPath, "/")
	}
	return requestPath == basePath || strings.HasPrefix(requestPath, basePath+"/")
}

// upstreamURL 将传入请求的 URL 改写到配置的上游地址，同时保留查询参数和 routePrefix 之后的路径
func (s *Server) upstreamURL(incoming *url.URL) *url.URL {
	upstream := *incoming
	upstream.Scheme = s.baseURL.Scheme
	upstream.Host = s.baseURL.Host
	upstream.Path = s.stripRoutePrefix(incoming.Path)
	upstream.RawPath = s.stripRoutePrefix(incoming.RawPath)
	return &upstream
}

// stripRoutePrefix 会去掉对外暴露的路由挂载前缀，供后续匹配和转发上游时使用
func (s *Server) stripRoutePrefix(requestPath string) string {
	if s.routePrefix == "" {
		return requestPath
	}
	if requestPath == s.routePrefix {
		return "/"
	}
	if strings.HasPrefix(requestPath, s.routePrefix+"/") {
		return requestPath[len(s.routePrefix):]
	}
	return ""
}

// copyHeaders 用来转发可透传的请求头或响应头，像 Connection、Keep-Alive 这类只对当前连接生效的头不会继续传递
func copyHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) {
			continue
		}
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// isHopByHopHeader 判断给定请求头是否属于代理不应继续转发的 hop-by-hop 头
func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

// captureWriter 在把上游响应写给客户端的同时保留一份内存副本用于落盘
// 如果下游写失败，会停止继续向客户端写，但不会中断上游读取，以便尽可能保留已收到的数据
type captureWriter struct {
	// 当前响应在 logstore 里的序号
	// 只用于在写下游失败时输出带编号的错误日志，方便定位是哪一次代理响应出错
	idx int

	// 原始下游请求
	// 这里只借它的 Context 判断客户端是否已经断开，避免在客户端主动取消时打印误导性的写失败日志
	req *http.Request

	// 已从上游读取到的响应数据副本
	// 无论是否成功写给客户端，都会先写入这里，后续 defer 会基于这份内容落盘
	// 对 SSE 场景，还会基于它截断不完整事件并尝试还原最终消息
	captured *bytes.Buffer

	// 面向客户端的实际输出
	// captureWriter 在捕获响应内容的同时，把相同的数据转发到这里
	downstream io.Writer

	// 下游支持流式刷新时使用的 flush 能力
	// 每次成功转发一段数据后都会立即 Flush，避免 SSE/流式响应被缓冲住
	flushDownstream http.Flusher

	// 标记下游是否已经不可继续写入
	// 一旦向客户端写失败，就置为 true；之后仍继续捕获上游数据，但不再尝试写下游
	closed bool
}

func (w *captureWriter) Write(p []byte) (int, error) {
	if _, err := w.captured.Write(p); err != nil {
		return 0, err
	}
	if w.closed || w.downstream == nil {
		return len(p), nil
	}
	if err := writeFull(w.downstream, p); err != nil {
		if w.req.Context().Err() == nil {
			log.Printf("[error] write downs tream response %d: %v", w.idx, err)
		}
		w.closed = true
		return len(p), nil
	}
	if w.flushDownstream != nil {
		w.flushDownstream.Flush()
	}
	return len(p), nil
}

// writeFull 会处理短写，避免流式转发时静默丢失部分数据
func writeFull(dst io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := dst.Write(p)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}
