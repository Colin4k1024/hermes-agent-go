# Hermes Agent (Go)

A complete Go rewrite of [hermes-agent](https://github.com/NousResearch/hermes-agent) — the self-improving AI agent by Nous Research.

## Quick Start

```bash
# Build
go build -o hermes ./cmd/hermes/

# Or use Make
make build

# Run interactive CLI
./hermes

# Single query
./hermes chat -q "Hello, what can you do?"

# Setup wizard
./hermes setup

# Check health
./hermes doctor
```

## Configuration

Uses the same config as the Python version:
- `~/.hermes/config.yaml` — settings
- `~/.hermes/.env` — API keys
- `~/.hermes/skills/` — skill files
- `~/.hermes/memories/` — persistent memory

## Features

- **40+ tools**: terminal, file operations, web search, browser, vision, TTS, etc.
- **18 platform adapters**: Telegram, Discord, Slack, WhatsApp, Signal, etc.
- **Skill system**: procedural memory with YAML/Markdown skill files
- **Session persistence**: SQLite with FTS5 full-text search
- **Context compression**: automatic summarization when approaching token limits
- **Subagent delegation**: parallel task execution via goroutines
- **Cron scheduling**: scheduled tasks with platform delivery
- **MCP integration**: Model Context Protocol client

## Building

```bash
# Dependencies
go mod tidy

# Build
make build

# Cross-compile
make build-all

# Docker
docker build -t hermes-agent .
```

## Testing

```bash
make test           # Full test suite
make test-short     # Quick tests
make test-race      # Race condition detection
```

## Architecture

```
cmd/hermes/          Entry point (Cobra CLI)
internal/
  agent/             Core agent loop (AIAgent)
  cli/               Interactive TUI (Bubble Tea)
  config/            Configuration management
  gateway/           Messaging platform gateway
    platforms/       Platform adapters
  tools/             Tool implementations
    environments/    Terminal backends
  state/             SQLite session database
  skills/            Skill loading and parsing
  toolsets/          Tool grouping and resolution
  llm/               LLM client (OpenAI-compatible)
  cron/              Scheduled tasks
  utils/             Shared utilities
```

## License

MIT — same as the original Python version.
