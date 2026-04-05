# ccecho

[中文](./README.md) | [English](./README.en.md)

  A lightweight CLI that adds transparent logging, call visualization, and local replay for Claude Code / Codex CLI.

## Overview

`ccecho` is a lightweight CLI tool for `Claude Code` and `Codex CLI`. It sits between your agent and the upstream model service, automatically recording each request, response, and session metadata, and provides a local web viewer for replay, debugging, and review.

It is quick to adopt, small in size, and easy to integrate. With a single local CLI, you can add recording, visualization, and replay capabilities to your agent API calls at low cost.

## Quick Start

1. Download the binary for your platform.

2. Add `ccecho` or `ccecho.exe` to your `PATH`.

3. Verify that the command is available:

```bash
ccecho --help
```

4. Run your agent with the proxy wired in automatically:

```bash
ccecho run [claude|codex]
```

5. Start the local viewer:

```bash
ccecho view
```

Default address: http://127.0.0.1:18888

## Build From Source

If you want to compile the binary for your current platform:

```bash
go build -o ccecho ./cmd/ccecho
```

## What Problems ccecho Solves

- You want to review a high-quality conversation, but do not have the full context.
- You want to compare how Claude and Codex actually behave, but lack a unified observation point.
- You want your team to discuss agent behavior based on factual records instead of personal impressions.
- You suspect the issue comes from the prompt, context, or tool usage, but the terminal only shows fragmented output.
- You want to share a successful agent workflow with teammates, but have no complete record they can replay.
- You want to accumulate real AI development samples over time instead of letting every call disappear after it ends.

`ccecho` turns those short-lived call traces into locally traceable records, so debugging, replay, comparison, and collaboration can be grounded in real requests and responses.

## Product Highlights

- CLI tool with zero-intrusion integration into existing workflows
- Lightweight executable with no need to deploy an extra service
- Runs locally and stores data locally, so setup and removal stay simple
- Does not replace your existing agent, only adds recording and replay capabilities

## Core Capabilities

- Proxy requests from Claude Code / Codex CLI locally
- Save request bodies, response bodies, streamed responses, and metadata by session
- Parse Claude / Codex SSE chunks and reconstruct text, thinking, tool use, and related information as much as possible
- Replay full call flows, inspect token usage, and view raw JSON
- Clean up empty log directories that contain no valid request or response data

## Use Cases

- Troubleshoot unstable model quality, context drift, or abnormal tool invocation
- Study how agents solve real tasks by inspecting request and response payloads
- Observe tool usage and command execution to catch potentially dangerous operations early
- Review context organization and token consumption patterns to evaluate cost changes
- Compare the behavior of different agents on real engineering tasks
- Retain local AI development sessions as searchable assets over time

## Who It Is For

- Individual developers who use Claude Code / Codex CLI frequently
- Tooling or platform engineers who need to inspect real request/response issues
- Learners who want to understand how agents work through real request and response data
- Users who need visibility into whether key commands and tool calls match expectations
- Users who want to analyze calling patterns and token usage to evaluate cost
- Users who want persistent local session records instead of only terminal scrollback

## Commands

### `ccecho run`

Start the local proxy and launch the target CLI.

```bash
ccecho run claude
ccecho run codex
```

### `ccecho view`

Start the local viewer and inspect saved sessions and request details.

```bash
ccecho view
```

### `ccecho clean`

Remove empty log directories that do not contain valid request or response content.

```bash
ccecho clean
```

## How It Works

The `ccecho` workflow is straightforward:

1. Start a local proxy.
2. Override the request endpoint used by Claude Code or Codex CLI with the proxy address.
3. Agent requests first pass through `ccecho`, then get forwarded to the real model service.
4. `ccecho` writes requests, streamed responses, final responses, and session metadata into `.ccecho/`.
5. Start the local viewer with `ccecho view` and inspect the full process in the browser.

## Implementation Overview

The project is organized into three main layers:

- `cmd/ccecho`
  CLI entry point responsible for command parsing, starting the proxy, launching Claude / Codex, and starting the viewer
- `internal/proxy`
  Local reverse proxy that forwards requests upstream and persists logs during forwarding
- `internal/viewer`
  Local web viewer that serves pages and JSON APIs for browsing sessions, requests, and response details

Other important modules:

- `internal/logstore`
  Manages request/response files by session directory and persists data through an async write queue
- `internal/requestview`
  Parses Claude / Codex request formats and presents them in a unified view
- `internal/stream`
  Handles streamed responses and reconstructs them into final message JSON as much as possible
- `internal/sessionmeta`, `internal/state`
  Stores current session metadata and recent state
