package model

import "time"

type Session struct {
	ID        string         `json:"id"`
	AgentID   string         `json:"agentId"`
	Provider  string         `json:"provider"`
	Status    SessionStatus  `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type SessionStatus string

const (
	SessionActive  SessionStatus = "active"
	SessionIdle    SessionStatus = "idle"
	SessionExpired SessionStatus = "expired"
	SessionError   SessionStatus = "error"
)

type SessionOptions struct {
	CWD         string         `json:"cwd,omitempty"`
	Model       string         `json:"model,omitempty"`
	SystemPrompt string        `json:"systemPrompt,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	MCPServers  []MCPServerConfig `json:"mcpServers,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type MCPServerConfig struct {
	Name    string         `json:"name"`
	URL     string         `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}
