# Agent Gateway - Project Rules

## Directory Structure
- `doc/` - Design docs and progress tracking
- `src/` - Go source code (project root for Go module)
- `test/` - Test code and fixtures

## Tech Stack
Go 1.23+, chi, gorilla/websocket, Viper, zerolog, SQLite/PostgreSQL, JWT

## Build & Run (run in `src/` directory)
- Build: `go build -o ../bin/gateway ./cmd/gateway/`
- Run: `go run ./cmd/gateway/`
- Test: `go test ./...`
- Lint: `go vet ./...`
- Format: `gofmt -w .`
- Tidy: `go mod tidy`

## Code Style
- Follow effective Go conventions
- No comments unless explicitly requested
- Use zerolog for structured logging
- Wrap errors: `fmt.Errorf("context: %w", err)`
- Keep interfaces small and focused

## Architecture
- Provider Pattern: Each agent platform implements `AgentProviderAdapter`
- Protocol Layer: A2A, ACP, MCP, OpenAI-compatible
- Unified Model: Agent, Session, Message, Task, StreamEvent
- Config-driven: Providers enabled/disabled via `gateway.yaml`
