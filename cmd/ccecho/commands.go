package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"ccecho/internal/config"
	"ccecho/internal/state"
	"ccecho/internal/viewer"
)

const version = "1.2.0"

func runVersion() {
	fmt.Printf("ccecho %s\n", version)
}

// 命令

func runProxy(args []string) {
	fs := flag.NewFlagSet("proxy", flag.ExitOnError)
	proxyAddr := fs.String("proxy-addr", "127.0.0.1:19999", "proxy listen address")
	proxyCode := fs.String("proxy-code", "", "route code prefix for the proxy base URL")
	provider := fs.String("provider", config.ProviderClaude, "proxy provider: claude or codex")
	codexProvider := fs.String("codex-provider", "", "codex provider name from ~/.codex/config.toml")
	_ = fs.Parse(args)

	app := mustApp()
	target, err := app.LoadTarget(*provider, *codexProvider)
	if err != nil {
		log.Fatal(err)
	}
	server, store, session, err := startProxy(app, *proxyAddr, target, *proxyCode)
	if err != nil {
		log.Fatal(err)
	}

	logProxySession(session)

	// 阻塞
	waitForShutdown(func(ctx context.Context) {
		shutdownProxy(ctx, server, store)
	})
}

func runRun(args []string) {
	provider := config.ProviderClaude
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		provider = args[0]
		args = args[1:]
	}

	switch provider {
	case config.ProviderClaude:
		runClaude(args)
	case config.ProviderCodex:
		runCodex(args)
	default:
		log.Fatalf("unsupported run provider: %s", provider)
	}
}

// run claude
func runClaude(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	proxyAddr := fs.String("proxy-addr", "127.0.0.1:19999", "proxy listen address")
	proxyCode := fs.String("proxy-code", "", "route code prefix for the proxy base URL; empty means auto-generate")
	_ = fs.Parse(args)

	app := mustApp()
	target, err := app.LoadTarget(config.ProviderClaude, "")
	if err != nil {
		log.Fatal(err)
	}
	resolvedProxyCode, err := ensureProxyCode(*proxyCode, true)
	if err != nil {
		log.Fatal(err)
	}
	server, store, session, err := startProxy(app, *proxyAddr, target, resolvedProxyCode)
	if err != nil {
		log.Fatal(err)
	}
	runtimeLogFile, restoreLogs, err := redirectLogs(filepath.Join(session.SessionPath, "ccecho.log"))
	if err != nil {
		shutdownProxy(context.Background(), server, store)
		log.Fatal(err)
	}
	defer restoreLogs()
	defer func() {
		_ = runtimeLogFile.Close()
	}()

	printRunBanner(os.Stderr, session, runtimeLogFile.Name())

	settingsArg, err := claudeSettingsArg(session.LocalBaseURL)
	if err != nil {
		log.Fatal(err)
	}

	warnOnClaudeArgConflicts(fs.Args())
	claudeArgs := append([]string{"--settings", settingsArg}, fs.Args()...)
	cmd := exec.Command("claude", claudeArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		shutdownProxy(context.Background(), server, store)
		log.Fatal(err)
	}

	// 阻塞
	err = waitCommandAndShutdown(cmd, func(ctx context.Context) {
		shutdownProxy(ctx, server, store)
	})
	exitOnCommandError(err)
}

// run codex
func runCodex(args []string) {
	fs := flag.NewFlagSet("run codex", flag.ExitOnError)
	proxyAddr := fs.String("proxy-addr", "127.0.0.1:19999", "proxy listen address")
	proxyCode := fs.String("proxy-code", "", "route code prefix for the proxy base URL; empty means auto-generate")
	providerName := fs.String("provider", "", "codex provider name from ~/.codex/config.toml")
	_ = fs.Parse(args)

	app := mustApp()
	target, err := app.LoadTarget(config.ProviderCodex, *providerName)
	if err != nil {
		log.Fatal(err)
	}
	//codexConfig, err := app.LoadCodexConfig()
	//if err != nil {
	//	log.Fatal(err)
	//}
	// 将 codex 实际的provider取出
	//providerCfg, ok := codexConfig.ModelProviders[target.Name]
	//if !ok {
	//	log.Fatalf("codex provider %q not found in %s", target.Name, app.CodexConfigPath())
	//}
	resolvedProxyCode, err := ensureProxyCode(*proxyCode, true)
	if err != nil {
		log.Fatal(err)
	}
	server, store, session, err := startProxy(app, *proxyAddr, target, resolvedProxyCode)
	if err != nil {
		log.Fatal(err)
	}
	runtimeLogFile, restoreLogs, err := redirectLogs(filepath.Join(session.SessionPath, "ccecho.log"))
	if err != nil {
		shutdownProxy(context.Background(), server, store)
		log.Fatal(err)
	}
	defer restoreLogs()
	defer func() {
		_ = runtimeLogFile.Close()
	}()

	printRunBanner(os.Stderr, session, runtimeLogFile.Name())

	warnOnCodexArgConflicts(fs.Args(), target.Name)

	codexArgs := append(codexBaseURLOverrideArgs(target.Name, session.LocalBaseURL), fs.Args()...)
	cmd := exec.Command("codex", codexArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		shutdownProxy(context.Background(), server, store)
		log.Fatal(err)
	}

	// 阻塞
	err = waitCommandAndShutdown(cmd, func(ctx context.Context) {
		shutdownProxy(ctx, server, store)
	})
	exitOnCommandError(err)
}

func runView(args []string) {
	fs := flag.NewFlagSet("view", flag.ExitOnError)
	viewerAddr := fs.String("viewer-addr", "127.0.0.1:18888", "viewer listen address")
	_ = fs.Parse(args)

	app := mustApp()
	if current, err := state.Read(app.StateDir); err == nil {
		log.Printf("current session: %s", current.SessionName)
		log.Printf("current logs: %s", current.SessionPath)
	}

	mux := http.NewServeMux()
	viewer.NewService(app.LogDir).Register(mux)
	server := &http.Server{
		Addr:    *viewerAddr,
		Handler: mux,
	}
	ln, err := net.Listen("tcp", *viewerAddr)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		log.Printf("viewer listening on http://%s", *viewerAddr)
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[error] viewer server: %v", err)
		}
	}()

	// 阻塞
	waitForShutdown(func(ctx context.Context) {
		_ = server.Shutdown(ctx)
	})
}

func runClean(args []string) {
	fs := flag.NewFlagSet("clean", flag.ExitOnError)
	_ = fs.Parse(args)

	app := mustApp()
	entries, err := os.ReadDir(app.LogDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("[error] logs directory not found: %s", app.LogDir)
			return
		}
		log.Fatal(err)
	}

	var removed []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionDir := filepath.Join(app.LogDir, entry.Name())
		empty, err := hasNoRequestOrResponseLogs(sessionDir)
		if err != nil {
			log.Printf("[error] skip %s: %v", sessionDir, err)
			continue
		}
		if !empty {
			continue
		}

		if err := os.RemoveAll(sessionDir); err != nil {
			log.Printf("[error] failed to remove %s: %v", sessionDir, err)
			continue
		}
		removed = append(removed, sessionDir)
	}

	sort.Strings(removed)
	for _, dir := range removed {
		log.Printf("removed: %s", dir)
	}
	log.Printf("clean done, removed %d directories", len(removed))
}
