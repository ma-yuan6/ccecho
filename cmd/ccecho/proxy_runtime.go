package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"claude-proxy-go/internal/config"
	"claude-proxy-go/internal/logstore"
	"claude-proxy-go/internal/proxy"
	"claude-proxy-go/internal/sessionmeta"
	"claude-proxy-go/internal/state"
)

// 一次会话 运行一次codex/claude
type proxySession struct {
	SessionName  string
	SessionPath  string
	Provider     string
	TargetName   string
	Target       string
	LocalBaseURL string
	ProxyAddr    string
	ProxyCode    string
	RoutePrefix  string
}

// startProxy 初始化会话元信息与状态文件，并启动本地代理 HTTP 服务
func startProxy(app config.App, proxyAddr string, target config.Target, proxyCode string) (*http.Server, *logstore.Store, proxySession, error) {
	routePrefix := routePrefix(proxyCode)
	localBaseURL := localBaseURL(target.BaseURL, proxyAddr, routePrefix)
	sessionName := time.Now().Format("20060102_150405")
	store, err := logstore.New(app.LogDir, sessionName)
	if err != nil {
		return nil, nil, proxySession{}, err
	}
	cleanupStore := true
	defer func() {
		if cleanupStore {
			_ = store.Close()
		}
	}()

	if err := os.MkdirAll(app.StateDir, 0o755); err != nil {
		return nil, nil, proxySession{}, err
	}

	session := proxySession{
		SessionName:  store.SessionName(),
		SessionPath:  store.CurrentPath(),
		Provider:     target.Provider,
		TargetName:   target.Name,
		Target:       target.BaseURL.String(),
		LocalBaseURL: localBaseURL,
		ProxyAddr:    proxyAddr,
		ProxyCode:    proxyCode,
		RoutePrefix:  routePrefix,
	}

	if err := sessionmeta.Write(session.SessionPath, sessionmeta.Meta{
		SessionName:  session.SessionName,
		SessionPath:  session.SessionPath,
		Provider:     session.Provider,
		TargetName:   session.TargetName,
		Target:       session.Target,
		ProxyAddr:    session.ProxyAddr,
		ProxyCode:    session.ProxyCode,
		RoutePrefix:  session.RoutePrefix,
		LocalBaseURL: session.LocalBaseURL,
		CreatedAt:    time.Now().Format(time.RFC3339),
	}); err != nil {
		return nil, nil, proxySession{}, err
	}

	if err := state.Write(app.StateDir, state.Current{
		SessionName: session.SessionName,
		SessionPath: session.SessionPath,
		ProxyAddr:   proxyAddr,
		ProxyCode:   proxyCode,
	}); err != nil {
		return nil, nil, proxySession{}, err
	}

	server := &http.Server{
		Addr:    proxyAddr,
		Handler: proxy.New(target, store, routePrefix).Handler(),
	}

	ln, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		return nil, nil, proxySession{}, err
	}

	go func() {
		log.Printf("[info] proxy listening on http://%s", proxyAddr)
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[error] proxy server: %v", err)
		}
	}()

	cleanupStore = false
	return server, store, session, nil
}

// logProxySession 将当前代理会话的关键字段输出到日志
func logProxySession(session proxySession) {
	log.Printf("session: %s", session.SessionName)
	log.Printf("logs: %s", session.SessionPath)
	log.Printf("provider: %s", session.Provider)
	if session.TargetName != "" {
		log.Printf("target name: %s", session.TargetName)
	}
	log.Printf("target: %s", session.Target)
	if session.ProxyCode != "" {
		log.Printf("proxy code: %s", session.ProxyCode)
	}
	log.Printf("local base url: %s", session.LocalBaseURL)
}

// localBaseURL 基于本地监听地址和目标路径生成对外访问的代理基地址
func localBaseURL(baseURL *url.URL, proxyAddr string, routePrefix string) string {
	return (&url.URL{
		Scheme: "http",
		Host:   proxyAddr,
		Path:   joinURLPath(routePrefix, strings.TrimRight(baseURL.Path, "/")),
	}).String()
}

// routePrefix 根据 proxyCode 生成路由前缀；空 code 时返回空前缀
func routePrefix(proxyCode string) string {
	if proxyCode == "" {
		return ""
	}
	return "/ccecho/" + proxyCode
}

// joinURLPath 按 URL 语义拼接多段路径，并确保结果以 "/" 开头
func joinURLPath(parts ...string) string {
	joined := ""
	for _, part := range parts {
		if part == "" || part == "/" {
			continue
		}
		if joined == "" {
			joined = part
			continue
		}
		joined = path.Join(joined, part)
	}
	if joined == "" {
		return ""
	}
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	return joined
}

// ensureProxyCode 在开启自动生成时为缺失的代理码生成随机十六进制字符串
func ensureProxyCode(proxyCode string, auto bool) (string, error) {
	if proxyCode != "" || !auto {
		return proxyCode, nil
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
