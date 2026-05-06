package hermes

import (
	"context"
	"fmt"

	"github.com/agent-gateway/gateway/internal/model"
	"github.com/agent-gateway/gateway/internal/provider"
)

type HermesProvider struct{}

func New() *HermesProvider {
	return &HermesProvider{}
}

func (p *HermesProvider) Name() string { return "hermes" }

func (p *HermesProvider) Initialize(ctx context.Context, cfg provider.ProviderConfig) error {
	return nil
}

func (p *HermesProvider) Shutdown(ctx context.Context) error { return nil }

func (p *HermesProvider) HealthCheck(ctx context.Context) error {
	return fmt.Errorf("hermes provider not yet implemented")
}

func (p *HermesProvider) ListAgents(ctx context.Context) ([]model.AgentDescriptor, error) {
	return nil, nil
}

func (p *HermesProvider) GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *HermesProvider) CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *HermesProvider) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *HermesProvider) ListSessions(ctx context.Context, agentID string) ([]model.Session, error) {
	return nil, nil
}

func (p *HermesProvider) CloseSession(ctx context.Context, sessionID string) error {
	return fmt.Errorf("not implemented")
}

func (p *HermesProvider) SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *HermesProvider) StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (<-chan model.StreamEvent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *HermesProvider) Cancel(ctx context.Context, sessionID string) error {
	return fmt.Errorf("not implemented")
}

func (p *HermesProvider) ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error) {
	return nil, nil
}

func (p *HermesProvider) InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *HermesProvider) RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error {
	return fmt.Errorf("not implemented")
}

func (p *HermesProvider) ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *HermesProvider) GetHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	return nil, nil
}
