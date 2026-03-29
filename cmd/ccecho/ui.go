package main

import (
	"fmt"
	"os"
	"strings"
)

// printRunBanner 以固定宽度输出 run 阶段的会话信息面板
func printRunBanner(w *os.File, session proxySession, runtimeLogPath string) {
	const (
		reset = "\x1b[0m"
		bold  = "\x1b[1m"
		cyan  = "\x1b[36m"
		gray  = "\x1b[90m"
		width = 120
	)

	fmt.Fprintf(w, "%s┌%s┐%s\n", gray, strings.Repeat("─", width-2), reset)
	printBannerTitleLine(w, width, bold+cyan+"ccecho"+reset, gray)
	fmt.Fprintf(w, "%s├%s┤%s\n", gray, strings.Repeat("─", width-2), reset)
	printRunBannerLine(w, "Session", session.SessionName, width)
	if session.ProxyCode != "" {
		printRunBannerLine(w, "Code", session.ProxyCode, width)
	}
	printRunBannerLine(w, "Target", session.Target, width)
	printRunBannerLine(w, "Proxy", session.LocalBaseURL, width)
	printRunBannerLine(w, "Logs", session.SessionPath, width)
	printRunBannerLine(w, "Runtime", runtimeLogPath, width)
	printRunBannerLine(w, "Status", sessionStartStatus(session.Provider), width)
	fmt.Fprintf(w, "%s└%s┘%s\n", gray, strings.Repeat("─", width-2), reset)
}

// sessionStartStatus 根据 provider 返回启动状态文案
func sessionStartStatus(provider string) string {
	switch provider {
	case "codex":
		return "Starting Codex"
	default:
		return "Starting Claude Code"
	}
}

// printBannerTitleLine 输出面板标题行，并按可见宽度补齐右侧空白
func printBannerTitleLine(w *os.File, width int, text string, borderColor string) {
	contentWidth := width - 4
	padding := contentWidth - visibleWidth(text)
	if padding < 0 {
		padding = 0
	}
	fmt.Fprintf(w, "%s│ %s%s %s│\x1b[0m\n", borderColor, text, strings.Repeat(" ", padding), borderColor)
}

// printRunBannerLine 输出一行键值信息，超长值会在中间截断
func printRunBannerLine(w *os.File, label string, value string, width int) {
	const (
		reset = "\x1b[0m"
		blue  = "\x1b[94m"
		gray  = "\x1b[90m"
	)

	labelWidth := 8
	contentWidth := width - 4
	valueWidth := contentWidth - labelWidth - 2
	clipped := clipMiddle(value, valueWidth)
	padding := valueWidth - visibleWidth(clipped)
	if padding < 0 {
		padding = 0
	}

	fmt.Fprintf(
		w,
		"%s│ %s%-8s%s  %s %s│%s\n",
		gray,
		blue,
		label,
		reset,
		clipped+strings.Repeat(" ", padding),
		gray,
		reset,
	)
}

// clipMiddle 在字符串超出最大可见宽度时，保留首尾并以省略号连接
func clipMiddle(s string, max int) string {
	if visibleWidth(s) <= max || max < 4 {
		return s
	}
	runes := []rune(s)
	left := (max - 1) / 2
	right := max - 1 - left
	return string(runes[:left]) + "…" + string(runes[len(runes)-right:])
}

// visibleWidth 计算字符串的可见宽度，忽略 ANSI 转义序列
func visibleWidth(s string) int {
	width := 0
	inEscape := false
	for _, r := range s {
		switch {
		case inEscape && r == 'm':
			inEscape = false
		case inEscape:
		case r == '\x1b':
			inEscape = true
		default:
			width++
		}
	}
	return width
}
