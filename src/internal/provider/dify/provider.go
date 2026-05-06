package dify

import (
	"context"
	"fmt"

	"github.com/agent-gateway/gateway/internal/model"
	"github.com/agent-gateway/gateway/internal/provider"
)

type DifyProvider struct{}

func New() *DifyProvider {
	return &DifyProvider{}
}

func (p *DifyProvider) Name() string { return "dify" }

func (p *DifyProvider) Initialize(ctx context.Context, cfg provider.ProviderConfig) error {
	return nil
}

func (p *DifyProvider) Shutdown(ctx context.Context) error { return nil }

func (p *DifyProvider) HealthCheck(ctx context.Context) error {
	return fmt.Errorf("dify provider not yet implemented")
}

func (p *DifyProvider) ListAgents(ctx context.Context) ([]model.AgentDescriptor, error) {
	return nil, nil
}

func (p *DifyProvider) GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *DifyProvider) CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *DifyProvider) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *DifyProvider) ListSessions(ctx context.Context, agentID string) ([]model.Session, error) {
	return nil, nil
}

func (p *DifyProvider) CloseSession(ctx context.Context, sessionID string) error {
	return fmt.Errorf("not implemented")
}

func (p *DifyProvider) SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *DifyProvider) StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (<-chan model.StreamEvent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *DifyProvider) Cancel(ctx context.Context, sessionID string) error {
	return fmt.Errorf("not implemented")
}

func (p *DifyProvider) ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error) {
	return nil, nil
}

func (p *DifyProvider) InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *DifyProvider) RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error {
	return fmt.Errorf("not implemented")
}

func (p *DifyProvider) ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *DifyProvider) GetHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	return nil, nil
}
