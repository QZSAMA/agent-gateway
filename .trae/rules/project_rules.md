# Agent Gateway - Project Rules

## Project Overview
Agent Gateway is a universal AI agent gateway that provides interoperability between different agent platforms including OpenClaw, Hermes, LangGraph, Dify, and any A2A/MCP/ACP-compatible agent.

## Directory Structure
- `doc/` - Design documents, proposals, and project progress tracking
- `src/` - All Go source code (the project root for Go module)
- `test/` - Test code and test fixtures

## Tech Stack
- Language: Go 1.23+
- HTTP Framework: chi (lightweight, stdlib-compatible)
- WebSocket: gorilla/websocket
- Config: Viper + YAML
- Logging: zerolog
- Storage: SQLite (default) + PostgreSQL (optional)
- Auth: JWT + API Key

## Build & Run Commands
- Build: `cd src && go build -o ../bin/gateway ./cmd/gateway/`
- Run: `cd src && go run ./cmd/gateway/`
- Test: `cd src && go test ./...`
- Test (verbose): `cd src && go test -v ./...`
- Lint: `cd src && go vet ./...`
- Format: `cd src && gofmt -w .`

## Code Style
- Follow standard Go conventions (effective Go)
- No comments unless explicitly requested
- Use zerolog for all logging (structured JSON)
- Error handling: always wrap errors with fmt.Errorf("context: %w", err)
- Interface segregation: keep interfaces small and focused
- All public types and functions should have clear, minimal signatures

## Architecture
- Provider Pattern: Each agent platform implements the AgentProviderAdapter interface
- Protocol Layer: A2A, ACP, MCP, OpenAI-compatible endpoints
- Unified Model: All platforms map to internal unified data models (Agent, Session, Message, Task, StreamEvent)
- Config-driven: Providers are enabled/disabled via gateway.yaml
