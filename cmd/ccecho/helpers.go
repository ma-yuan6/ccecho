package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"claude-proxy-go/internal/config"
)

func waitForShutdown(stop func(ctx context.Context)) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	<-sigCh
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stop(ctx)
}

func mustApp() config.App {
	baseDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	return config.New(baseDir)
}

func mustTarget(app config.App) string {
	target, err := app.LoadTargetURL()
	if err != nil {
		log.Fatal(err)
	}
	return target
}

func mustBaseURL(app config.App) *url.URL {
	baseURL, err := app.LoadBaseURL()
	if err != nil {
		log.Fatal(err)
	}
	return baseURL
}

// claudeSettingsArg 生成claude配置(因为需要代理 claude 所以需要将 claude 请求目标地址改为被代理地址)
func claudeSettingsArg(localBaseURL string) (string, error) {
	raw, err := json.Marshal(map[string]any{
		"env": map[string]string{
			"ANTHROPIC_BASE_URL": localBaseURL,
		},
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// 重定向 ccecho 日志
func redirectLogs(path string) (*os.File, func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	previous := log.Writer()
	log.SetOutput(file)
	return file, func() {
		log.SetOutput(previous)
	}, nil
}

// 命令帮助文档
func usage() {
	fmt.Println("ccecho proxy [--proxy-addr 127.0.0.1:9999]")
	fmt.Println("ccecho run   [--proxy-addr 127.0.0.1:9999] [-- claude args]")
	fmt.Println("ccecho show  [--viewer-addr 127.0.0.1:18080]")
	fmt.Println("ccecho clean")
}

// 检测目录下是否有请求或响应日志文件
func hasNoRequestOrResponseLogs(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "request") && strings.HasSuffix(name, ".json") {
			return false, nil
		}
		if strings.HasPrefix(name, "response") && strings.HasSuffix(name, ".stream") {
			return false, nil
		}
	}
	return true, nil
}
