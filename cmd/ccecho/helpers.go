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

func ifClaudeArgConflicts(settings string) bool {
	settings = strings.TrimSpace(settings)
	if settings == "" {
		return false
	}

	// 可能配置文件地址
	raw := []byte(settings)
	if !strings.HasPrefix(settings, "{") {
		fileRaw, err := os.ReadFile(settings)
		if err != nil {
			return false
		}
		raw = fileRaw
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		log.Println("[warn] unmarshal claude code settings json error")
		return false
	}

	// 代理URL配置在 env.ANTHROPIC_BASE_URL
	env, exists := payload["env"].(map[string]any)
	if !exists {
		return false
	}

	value, exists := env["ANTHROPIC_BASE_URL"]
	if !exists {
		return false
	}

	_, ok := value.(string)
	return ok
}

// 创建codex代理参数
func codexBaseURLOverrideArgs(providerName string, localBaseURL string) []string {
	return []string{
		"-c", fmt.Sprintf("model_providers.%s.base_url=%q", providerName, localBaseURL),
	}
}

// claude 代理地址被重写
func warnOnClaudeArgConflicts(args []string) {
	for idx := 0; idx < len(args); idx++ {
		argItem := args[idx]
		if argItem == "--settings" && idx+1 < len(args) {
			if ifClaudeArgConflicts(args[idx+1]) {
				printArgConflictWarnings("claude", fmt.Sprintf("[warn] claude argument “%q” overrides env.ANTHROPIC_BASE_URL, which may bypass the local ccecho proxy", args[idx+1]))
				return
			}
		}
	}
}

// codex 代理地址被重写
func warnOnCodexArgConflicts(args []string, providerName string) {
	baseURLKey := fmt.Sprintf("model_providers.%s.base_url", providerName)
	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]
		if arg == "-c" && idx+1 < len(args) {
			overrideArg := args[idx+1]
			key, _, ok := strings.Cut(overrideArg, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			if key == baseURLKey {
				printArgConflictWarnings("codex", fmt.Sprintf("[warn] codex argument “-c %q” overrides %s, which would bypass the local ccecho proxy", overrideArg, baseURLKey))
				return
			}
		}
	}
}

// 打印代理配置被重写警告
func printArgConflictWarnings(agent string, warning string) {
	fmt.Printf("[warn] detected user arguments that may override ccecho proxy settings for %s\n", agent)
	fmt.Println(warning)
	fmt.Println("[warn] ccecho will continue, but requests may no longer go through the local proxy")
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
	fmt.Println("ccecho proxy [--proxy-addr 127.0.0.1:19999] [--proxy-code code] [--provider claude|codex] [--codex-provider name]")
	fmt.Println("ccecho run   [claude|codex] [--proxy-addr 127.0.0.1:19999] [--proxy-code code] [provider options] [-- tool args]")
	fmt.Println("ccecho codex [--proxy-addr 127.0.0.1:19999] [--proxy-code code] [--provider name] [-- codex args]")
	fmt.Println("ccecho view  [--viewer-addr 127.0.0.1:18888]")
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
