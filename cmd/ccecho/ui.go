package main

import (
	"fmt"
	"os"
	"strings"
)

func printRunBanner(w *os.File, session proxySession, runtimeLogPath string) {
	const (
		reset = "\x1b[0m"
		bold  = "\x1b[1m"
		cyan  = "\x1b[36m"
		gray  = "\x1b[90m"
		green = "\x1b[32m"
		width = 78
	)

	fmt.Fprintf(w, "%s┌%s┐%s\n", gray, strings.Repeat("─", width-2), reset)
	printBannerTextLine(w, width, bold+cyan+"Ccecho"+reset, gray)
	printBannerTextLine(w, width, bold+green+"Run"+reset, gray)
	fmt.Fprintf(w, "%s├%s┤%s\n", gray, strings.Repeat("─", width-2), reset)
	printRunBannerLine(w, "Session", session.SessionName, width)
	printRunBannerLine(w, "Target", session.Target, width)
	printRunBannerLine(w, "Logs", session.SessionPath, width)
	printRunBannerLine(w, "Runtime", runtimeLogPath, width)
	printRunBannerLine(w, "Status", "Starting Claude Code", width)
	fmt.Fprintf(w, "%s└%s┘%s\n", gray, strings.Repeat("─", width-2), reset)
}

func printBannerTextLine(w *os.File, width int, text string, borderColor string) {
	contentWidth := width - 4
	padding := contentWidth - visibleWidth(text)
	if padding < 0 {
		padding = 0
	}
	fmt.Fprintf(w, "%s│ %s%s %s│\x1b[0m\n", borderColor, text, strings.Repeat(" ", padding), borderColor)
}

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

func clipMiddle(s string, max int) string {
	if visibleWidth(s) <= max || max < 4 {
		return s
	}
	runes := []rune(s)
	left := (max - 1) / 2
	right := max - 1 - left
	return string(runes[:left]) + "…" + string(runes[len(runes)-right:])
}

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
