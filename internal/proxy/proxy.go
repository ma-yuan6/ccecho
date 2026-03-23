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

	"claude-proxy-go/internal/logstore"
	"claude-proxy-go/internal/stream"
)

type Server struct {
	baseURL *url.URL
	store   *logstore.Store
	client  *http.Client
}

func New(baseURL *url.URL, store *logstore.Store) *Server {
	return &Server{
		baseURL: baseURL,
		store:   store,
		client:  &http.Client{},
	}
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.handle)
}

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
			data = trimToCompleteSSEEvents(data)
			finalJSON, complete, err := stream.FinalMessageFromStream(data)
			if err != nil {
				log.Printf("[error] convert response %d: %v", idx, err)
			}
			if complete {
				if err := s.store.WriteResponseJSON(idx, finalJSON); err != nil {
					log.Printf("[error] write response %d: %v", idx, err)
				}
				return
			}
			if err := s.store.WriteResponseStream(idx, data); err != nil {
				log.Printf("[error] write response %d: %v", idx, err)
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
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Printf("[error] read upstream response %d: %v", idx, err)
		return
	}
}

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

func (s *Server) matches(path string) bool {
	basePath := strings.TrimRight(s.baseURL.Path, "/")
	requestPath := strings.TrimRight(path, "/")
	if basePath == "" {
		return strings.HasPrefix(requestPath, "/")
	}
	return requestPath == basePath || strings.HasPrefix(requestPath, basePath+"/")
}

func (s *Server) upstreamURL(incoming *url.URL) *url.URL {
	upstream := *incoming
	upstream.Scheme = s.baseURL.Scheme
	upstream.Host = s.baseURL.Host
	return &upstream
}

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

type captureWriter struct {
	idx             int
	req             *http.Request
	captured        *bytes.Buffer
	downstream      io.Writer
	flushDownstream http.Flusher
	closed          bool
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
			log.Printf("[error] write downstream response %d: %v", w.idx, err)
		}
		w.closed = true
		return len(p), nil
	}
	if w.flushDownstream != nil {
		w.flushDownstream.Flush()
	}
	return len(p), nil
}

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

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
