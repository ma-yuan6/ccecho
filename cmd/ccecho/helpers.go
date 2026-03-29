package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"claude-proxy-go/internal/config"
	"claude-proxy-go/internal/logstore"
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

// waitCommandAndShutdown 等待外部命令结束，收到退出信号时转发给子进程，并优雅关闭 server
// 主动关闭 code/claude
func waitCommandAndShutdown(cmd *exec.Cmd, stop func(ctx context.Context)) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Wait()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	var err error
	select {
	case sig := <-sigCh:
		if cmd.Process != nil {
			_ = cmd.Process.Signal(sig)
		}
		err = <-errCh
	case err = <-errCh:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stop(ctx)

	return err
}

// shutdownProxy 先停止接收新请求，再等待日志写盘队列清空
func shutdownProxy(ctx context.Context, server *http.Server, store *logstore.Store) {
	_ = server.Shutdown(ctx)
	if store != nil {
		_ = store.Close()
	}
}

// exitOnCommandError 按子进程退出码退出当前进程，非退出码错误按 fatal 处理
func exitOnCommandError(err error) {
	if err == nil {
		return
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	log.Fatal(err)
}

func mustApp() config.App {
	baseDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	return config.New(baseDir)
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

// 创建codex代理参数
func codexBaseURLOverrideArgs(providerName string, localBaseURL string, provider config.CodexProviderConfig) []string {
	if provider.Name == "" {
		provider.Name = providerName
	}
	if provider.WireAPI == "" {
		provider.WireAPI = "responses"
	}
	return []string{
		"-c", fmt.Sprintf("model_provider=%q", providerName),
		"-c", fmt.Sprintf("model_providers.%s.name=%q", providerName, provider.Name),
		"-c", fmt.Sprintf("model_providers.%s.base_url=%q", providerName, localBaseURL),
		"-c", fmt.Sprintf("model_providers.%s.wire_api=%q", providerName, provider.WireAPI),
		"-c", fmt.Sprintf("model_providers.%s.requires_openai_auth=%t", providerName, provider.RequiresOpenAIAuth),
	}
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
	fmt.Println("ccecho --version")
	fmt.Println("ccecho proxy [--proxy-addr 127.0.0.1:9999] [--proxy-code code] [--provider claude|codex] [--codex-provider name]")
	fmt.Println("ccecho run   [claude|codex] [--proxy-addr 127.0.0.1:9999] [--proxy-code code] [provider options] [-- tool args]")
	fmt.Println("ccecho codex [--proxy-addr 127.0.0.1:9999] [--proxy-code code] [--provider name] [-- codex args]")
	fmt.Println("ccecho view  [--viewer-addr 127.0.0.1:18080]")
	fmt.Println("ccecho clean")
	fmt.Println("ccecho version")
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
