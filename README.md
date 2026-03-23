# ccecho

`ccecho` is a small local proxy and log viewer for Claude Code traffic.

It currently targets Anthropic / Claude request flow only. The project can already forward generic HTTP traffic, but the request diffing and response parsing logic are still Claude-specific.

## Build

```bash
go build -o ccecho ./cmd/ccecho
```

## Commands

```bash
./ccecho proxy [--proxy-addr 127.0.0.1:9999]
./ccecho run   [--proxy-addr 127.0.0.1:9999] [-- claude args]
./ccecho show  [--viewer-addr 127.0.0.1:18080]
./ccecho clean
```

## Typical Flow

### 1. Start the proxy

```bash
./ccecho proxy
```

This will:

- create a new session under `./.ccecho/logs/<timestamp>/`
- write current session metadata to `./.ccecho/current_session.json`
- start a local HTTP proxy on `127.0.0.1:9999`

The proxy reads `ANTHROPIC_BASE_URL` from `~/.claude/settings.json`, then forwards only requests under that configured Anthropic base path.

### 2. Run Claude through the proxy

You can let `ccecho` start Claude directly:

```bash
./ccecho run
```

Pass extra Claude arguments after `--`:

```bash
./ccecho run -- --continue
```

Internally this injects a temporary Claude settings override so Claude talks to the local proxy instead of the upstream Anthropic endpoint.

### 3. Open the viewer

```bash
./ccecho show
```

Then open:

```text
http://127.0.0.1:18080
```

## Viewer

The viewer reads sessions from `./.ccecho/logs/` and provides:

- session list and request list
- incremental request display based on diff vs the previous request
- parsed response blocks from `response*.stream`
- raw response fallback when block parsing fails
- request / session filtering
- request sorting
- dark mode
- manual refresh, optional auto refresh, and follow-latest toggle

## Runtime Files

Generated data is stored under `./.ccecho/`:

- `logs/<timestamp>/request*.json`
- `logs/<timestamp>/response*.stream`
- `logs/<timestamp>/response*.json`
- `logs/<timestamp>/ccecho.log`
- `current_session.json`

## Cleanup

Remove invalid log session folders that contain neither request nor response logs:

```bash
./ccecho clean
```

## Limitations

- Claude-only for now: request extraction and stream parsing assume Anthropic message / SSE shapes
- `run` currently launches `claude` only
- Codex / OpenAI Responses-style traffic is not yet parsed by the viewer
