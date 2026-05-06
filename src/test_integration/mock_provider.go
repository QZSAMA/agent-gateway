package integration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agent-gateway/gateway/internal/model"
	"github.com/agent-gateway/gateway/internal/provider"
	"github.com/agent-gateway/gateway/internal/util"
)

type MockProvider struct {
	mu       sync.RWMutex
	sessions map[string]*model.Session
	messages map[string][]model.Message
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		sessions: make(map[string]*model.Session),
		messages: make(map[string][]model.Message),
	}
}

func (p *MockProvider) Name() string { return "test" }

func (p *MockProvider) Initialize(ctx context.Context, cfg provider.ProviderConfig) error {
	return nil
}

func (p *MockProvider) Shutdown(ctx context.Context) error { return nil }

func (p *MockProvider) HealthCheck(ctx context.Context) error { return nil }

func (p *MockProvider) ListAgents(ctx context.Context) ([]model.AgentDescriptor, error) {
	return []model.AgentDescriptor{
		{
			ID:          "test:echo",
			Name:        "Echo Agent",
			Description: "A test echo agent",
			Provider:    "test",
			Capabilities: model.AgentCapabilities{
				Streaming: true,
				ToolUse:   false,
				Memory:    true,
			},
		},
	}, nil
}

func (p *MockProvider) GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error) {
	return &model.AgentDescriptor{
		ID:          "test:echo",
		Name:        "Echo Agent",
		Description: "A test echo agent",
		Provider:    "test",
	}, nil
}

func (p *MockProvider) CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error) {
	sess := &model.Session{
		ID:        util.NewSessionID(),
		AgentID:   agentID,
		Provider:  "test",
		Status:    model.SessionActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	p.mu.Lock()
	p.sessions[sess.ID] = sess
	p.mu.Unlock()
	return sess, nil
}

func (p *MockProvider) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	sess, ok := p.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return sess, nil
}

func (p *MockProvider) ListSessions(ctx context.Context, agentID string) ([]model.Session, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var sessions []model.Session
	for _, s := range p.sessions {
		sessions = append(sessions, *s)
	}
	return sessions, nil
}

func (p *MockProvider) CloseSession(ctx context.Context, sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessions, sessionID)
	return nil
}

func (p *MockProvider) SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (*model.Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	resp := &model.Message{
		ID:        util.NewMessageID(),
		SessionID: sessionID,
		Role:      model.RoleAgent,
		Content:   []model.ContentBlock{{Type: model.ContentText, Text: "Echo: " + text}},
		Timestamp: time.Now(),
	}
	p.messages[sessionID] = append(p.messages[sessionID], *msg, *resp)
	return resp, nil
}

func (p *MockProvider) StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (<-chan model.StreamEvent, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	ch := make(chan model.StreamEvent, 4)
	go func() {
		defer close(ch)
		ch <- model.StreamEvent{
			Type:      model.EventMessageChunk,
			SessionID: sessionID,
			Role:      model.RoleAgent,
			Content:   &model.ContentBlock{Type: model.ContentText, Text: "Echo: "},
		}
		ch <- model.StreamEvent{
			Type:      model.EventMessageChunk,
			SessionID: sessionID,
			Role:      model.RoleAgent,
			Content:   &model.ContentBlock{Type: model.ContentText, Text: text},
		}
		ch <- model.StreamEvent{
			Type:       model.EventTaskStatus,
			SessionID:  sessionID,
			TaskStatus: model.TaskCompleted,
		}
	}()

	p.mu.Lock()
	p.messages[sessionID] = append(p.messages[sessionID], *msg)
	p.mu.Unlock()

	return ch, nil
}

func (p *MockProvider) Cancel(ctx context.Context, sessionID string) error { return nil }

func (p *MockProvider) ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error) {
	return nil, nil
}

func (p *MockProvider) InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *MockProvider) RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error {
	return nil
}

func (p *MockProvider) ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *MockProvider) GetHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.messages[sessionID], nil
}
