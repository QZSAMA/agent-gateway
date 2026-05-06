package provider

import (
	"context"

	"github.com/agent-gateway/gateway/internal/model"
)

type AgentProviderAdapter interface {
	Name() string
	Initialize(ctx context.Context, config ProviderConfig) error
	Shutdown(ctx context.Context) error
	HealthCheck(ctx context.Context) error

	ListAgents(ctx context.Context) ([]model.AgentDescriptor, error)
	GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error)

	CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error)
	GetSession(ctx context.Context, sessionID string) (*model.Session, error)
	ListSessions(ctx context.Context, agentID string) ([]model.Session, error)
	CloseSession(ctx context.Context, sessionID string) error

	SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *SendOptions) (*model.Message, error)
	StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *SendOptions) (<-chan model.StreamEvent, error)

	Cancel(ctx context.Context, sessionID string) error

	ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error)
	InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error)

	RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error
	ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error)

	GetHistory(ctx context.Context, sessionID string) ([]model.Message, error)
}

type ProviderConfig struct {
	Endpoint string         `json:"endpoint"`
	Auth     AuthConfig     `json:"auth"`
	Options  map[string]any `json:"options"`
}

type AuthConfig struct {
	Token  string `json:"token,omitempty"`
	APIKey string `json:"apiKey,omitempty"`
}

type SendOptions struct {
	Stream   bool           `json:"stream,omitempty"`
	Timeout  int            `json:"timeout,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
