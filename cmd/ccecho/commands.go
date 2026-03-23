package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"claude-proxy-go/internal/state"
	"claude-proxy-go/internal/viewer"
)

// 命令

func runProxy(args []string) {
	fs := flag.NewFlagSet("proxy", flag.ExitOnError)
	proxyAddr := fs.String("proxy-addr", "127.0.0.1:9999", "proxy listen address")
	_ = fs.Parse(args)

	app := mustApp()
	server, session, err := startProxy(app, *proxyAddr)
	if err != nil {
		log.Fatal(err)
	}

	logProxySession(session)

	waitForShutdown(func(ctx context.Context) {
		_ = server.Shutdown(ctx)
	})
}

func runClaude(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	proxyAddr := fs.String("proxy-addr", "127.0.0.1:9999", "proxy listen address")
	_ = fs.Parse(args)

	app := mustApp()
	server, session, err := startProxy(app, *proxyAddr)
	if err != nil {
		log.Fatal(err)
	}
	runtimeLogFile, restoreLogs, err := redirectLogs(filepath.Join(session.SessionPath, "ccecho.log"))
	if err != nil {
		_ = server.Shutdown(context.Background())
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

	claudeArgs := append([]string{"--settings", settingsArg}, fs.Args()...)
	cmd := exec.Command("claude", claudeArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("starting: claude %s", strings.Join(claudeArgs, " "))

	if err := cmd.Start(); err != nil {
		_ = server.Shutdown(context.Background())
		log.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Wait()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

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
	_ = server.Shutdown(ctx)

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		log.Fatal(err)
	}
}

func runShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	viewerAddr := fs.String("viewer-addr", "127.0.0.1:18080", "viewer listen address")
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
