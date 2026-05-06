package openclaw

import (
	"context"
	"fmt"

	"github.com/agent-gateway/gateway/internal/model"
	"github.com/agent-gateway/gateway/internal/provider"
)

type OpenClawProvider struct{}

func New() *OpenClawProvider {
	return &OpenClawProvider{}
}

func (p *OpenClawProvider) Name() string { return "openclaw" }

func (p *OpenClawProvider) Initialize(ctx context.Context, cfg provider.ProviderConfig) error {
	return nil
}

func (p *OpenClawProvider) Shutdown(ctx context.Context) error { return nil }

func (p *OpenClawProvider) HealthCheck(ctx context.Context) error {
	return fmt.Errorf("openclaw provider not yet implemented")
}

func (p *OpenClawProvider) ListAgents(ctx context.Context) ([]model.AgentDescriptor, error) {
	return nil, nil
}

func (p *OpenClawProvider) GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *OpenClawProvider) CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *OpenClawProvider) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *OpenClawProvider) ListSessions(ctx context.Context, agentID string) ([]model.Session, error) {
	return nil, nil
}

func (p *OpenClawProvider) CloseSession(ctx context.Context, sessionID string) error {
	return fmt.Errorf("not implemented")
}

func (p *OpenClawProvider) SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *OpenClawProvider) StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (<-chan model.StreamEvent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *OpenClawProvider) Cancel(ctx context.Context, sessionID string) error {
	return fmt.Errorf("not implemented")
}

func (p *OpenClawProvider) ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error) {
	return nil, nil
}

func (p *OpenClawProvider) InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *OpenClawProvider) RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error {
	return fmt.Errorf("not implemented")
}

func (p *OpenClawProvider) ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *OpenClawProvider) GetHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	return nil, nil
}
