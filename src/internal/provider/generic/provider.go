package generic

import (
	"context"
	"fmt"

	"github.com/agent-gateway/gateway/internal/model"
	"github.com/agent-gateway/gateway/internal/provider"
)

type GenericA2AProvider struct{}

func New() *GenericA2AProvider {
	return &GenericA2AProvider{}
}

func (p *GenericA2AProvider) Name() string { return "generic-a2a" }

func (p *GenericA2AProvider) Initialize(ctx context.Context, cfg provider.ProviderConfig) error {
	return nil
}

func (p *GenericA2AProvider) Shutdown(ctx context.Context) error { return nil }

func (p *GenericA2AProvider) HealthCheck(ctx context.Context) error {
	return fmt.Errorf("generic a2a provider not yet implemented")
}

func (p *GenericA2AProvider) ListAgents(ctx context.Context) ([]model.AgentDescriptor, error) {
	return nil, nil
}

func (p *GenericA2AProvider) GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *GenericA2AProvider) CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *GenericA2AProvider) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *GenericA2AProvider) ListSessions(ctx context.Context, agentID string) ([]model.Session, error) {
	return nil, nil
}

func (p *GenericA2AProvider) CloseSession(ctx context.Context, sessionID string) error {
	return fmt.Errorf("not implemented")
}

func (p *GenericA2AProvider) SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *GenericA2AProvider) StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (<-chan model.StreamEvent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *GenericA2AProvider) Cancel(ctx context.Context, sessionID string) error {
	return fmt.Errorf("not implemented")
}

func (p *GenericA2AProvider) ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error) {
	return nil, nil
}

func (p *GenericA2AProvider) InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *GenericA2AProvider) RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error {
	return fmt.Errorf("not implemented")
}

func (p *GenericA2AProvider) ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *GenericA2AProvider) GetHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	return nil, nil
}
