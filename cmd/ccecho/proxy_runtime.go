package main

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"claude-proxy-go/internal/config"
	"claude-proxy-go/internal/logstore"
	"claude-proxy-go/internal/proxy"
	"claude-proxy-go/internal/state"
)

type proxySession struct {
	SessionName  string
	SessionPath  string
	Target       string
	LocalBaseURL string
	ProxyAddr    string
}

// 起代理服务
func startProxy(app config.App, proxyAddr string) (*http.Server, proxySession, error) {
	baseURL := mustBaseURL(app)
	target := mustTarget(app)
	localBaseURL := localBaseURL(baseURL, proxyAddr)
	sessionName := time.Now().Format("20060102_150405")
	store, err := logstore.New(app.LogDir, sessionName)
	if err != nil {
		return nil, proxySession{}, err
	}

	if err := os.MkdirAll(app.StateDir, 0o755); err != nil {
		return nil, proxySession{}, err
	}

	if err := state.Write(app.StateDir, state.Current{
		SessionName: store.SessionName(),
		SessionPath: store.CurrentPath(),
		ProxyAddr:   proxyAddr,
	}); err != nil {
		return nil, proxySession{}, err
	}

	server := &http.Server{
		Addr:    proxyAddr,
		Handler: proxy.New(baseURL, store).Handler(),
	}

	ln, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		return nil, proxySession{}, err
	}

	go func() {
		log.Printf("proxy listening on http://%s", proxyAddr)
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[error] proxy server: %v", err)
		}
	}()

	return server, proxySession{
		SessionName:  store.SessionName(),
		SessionPath:  store.CurrentPath(),
		Target:       target,
		LocalBaseURL: localBaseURL,
		ProxyAddr:    proxyAddr,
	}, nil
}

func logProxySession(session proxySession) {
	log.Printf("session: %s", session.SessionName)
	log.Printf("logs: %s", session.SessionPath)
	log.Printf("target: %s", session.Target)
	log.Printf("local base url: %s", session.LocalBaseURL)
}

func localBaseURL(baseURL *url.URL, proxyAddr string) string {
	return (&url.URL{
		Scheme: "http",
		Host:   proxyAddr,
		Path:   strings.TrimRight(baseURL.Path, "/"),
	}).String()
}
