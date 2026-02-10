# aagent

A Go-based autonomous AI coding agent that executes tasks in sessions.

## Features

- **TUI Interface**: Beautiful terminal UI with scrollable history, multi-line input, and real-time status
- **Agentic Loop**: Receive task → call LLM with tools → execute tool calls → return results → repeat until complete
- **Session Persistence**: SQLite-based session storage with resumption support
- **Tool System**: Modular, extensible tools (bash, read, write, edit, glob, grep)
- **Kimi K2.5**: Uses Kimi K2.5 by Moonshot AI as the LLM backend
- **Live Metrics**: Token usage tracking and context window percentage display

## Installation

```bash
# Build from source
make build

# Or install to GOPATH/bin
make install
```

## Usage

```bash
# Set your API key
export KIMI_API_KEY=sk-...

# Launch TUI (interactive mode)
./aagent

# Run with an initial task
./aagent "Create a hello world Go program"

# Resume a previous session
./aagent --continue <session-id>

# List sessions
./aagent session list
```

## TUI Interface

The TUI provides an interactive interface with:

- **Top Bar**: Task summary on the left, token usage and context window percentage on the right
- **Message History**: Scrollable view of all conversation messages with timestamps
- **Status Line**: Loading indicator when processing, human-readable timer showing time since last input
- **Input Area**: Multi-line text area for entering queries (Alt+Enter for new line, Enter to send)
- **Keyboard Shortcuts**:
  - `esc`: Quit
  - `enter`: Send message
  - `alt+enter`: Insert new line in input
  - `ctrl+c`: Force quit

## Configuration

Configuration is loaded from:
1. `.aagent/config.json` (project-level)
2. `~/.config/aagent/config.json` (user-level)

Environment variables:
- `KIMI_API_KEY` - Kimi API key (required)
- `AAGENT_MODEL` - Override default model
- `AAGENT_DATA_PATH` - Data storage directory

## Tools

| Tool | Description |
|------|-------------|
| `bash` | Execute shell commands |
| `read` | Read file contents with line range support |
| `write` | Create or overwrite files |
| `edit` | String replacement edits in files |
| `glob` | Find files by pattern |
| `grep` | Search file contents with regex |

## Project Structure

```
aagent/
├── cmd/aagent/         # CLI entry point
├── internal/
│   ├── agent/          # Agent orchestrator and loop
│   ├── config/         # Configuration management
│   ├── llm/            # LLM client interfaces
│   │   └── kimi/       # Kimi K2.5 implementation
│   ├── session/        # Session management
│   ├── storage/        # SQLite persistence
│   ├── tools/          # Tool implementations
│   └── tui/            # Terminal user interface (Bubble Tea)
├── go.mod
├── Makefile
└── README.md
```

## Development

```bash
# Build
make build

# Run tests
make test

# Format code
make fmt

# Lint
make lint
```

## License

MIT
