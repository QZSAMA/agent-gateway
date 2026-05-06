package openclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/agent-gateway/gateway/internal/model"
	"github.com/agent-gateway/gateway/internal/provider"
	"github.com/agent-gateway/gateway/internal/util"
)

type OpenClawProvider struct {
	endpoint   string
	token      string
	httpClient *http.Client
	logger     zerolog.Logger
	agents     []model.AgentDescriptor
	mu         sync.RWMutex
	sessions   map[string]*model.Session
	messages   map[string][]model.Message
	approvals  map[string]*model.ApprovalRequest
}

func New(logger zerolog.Logger) *OpenClawProvider {
	return &OpenClawProvider{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		logger:     logger,
		sessions:   make(map[string]*model.Session),
		messages:   make(map[string][]model.Message),
		approvals:  make(map[string]*model.ApprovalRequest),
	}
}

func (p *OpenClawProvider) Name() string { return "openclaw" }

func (p *OpenClawProvider) Initialize(ctx context.Context, cfg provider.ProviderConfig) error {
	p.endpoint = strings.TrimRight(cfg.Endpoint, "/")
	p.token = cfg.Auth.Token
	if p.token == "" {
		p.token = cfg.Auth.APIKey
	}

	p.discoverAgents(ctx)
	return nil
}

func (p *OpenClawProvider) discoverAgents(ctx context.Context) {
	p.agents = []model.AgentDescriptor{
		{
			ID:          "openclaw:main",
			Name:        "OpenClaw Main Agent",
			Description: "Primary OpenClaw coding agent",
			Provider:    "openclaw",
			Capabilities: model.AgentCapabilities{
				Streaming:      true,
				ToolUse:       true,
				HumanInTheLoop: true,
				MultiModal:     true,
				LongRunningTasks: true,
				Memory:         true,
			},
			Skills: []model.AgentSkill{
				{ID: "code", Name: "Code Generation", Description: "Write and modify code"},
				{ID: "review", Name: "Code Review", Description: "Review code changes"},
				{ID: "deploy", Name: "Deploy", Description: "Deploy applications"},
			},
		},
		{
			ID:          "openclaw:reviewer",
			Name:        "OpenClaw Code Reviewer",
			Description: "OpenClaw code review specialist",
			Provider:    "openclaw",
			Capabilities: model.AgentCapabilities{
				Streaming:      true,
				ToolUse:       true,
				HumanInTheLoop: true,
				Memory:         true,
			},
		},
	}
}

func (p *OpenClawProvider) Shutdown(ctx context.Context) error { return nil }

func (p *OpenClawProvider) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", p.endpoint+"/api/health", nil)
	if err != nil {
		return err
	}
	p.setAuth(req)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("openclaw health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openclaw health check status: %d", resp.StatusCode)
	}
	return nil
}

func (p *OpenClawProvider) ListAgents(ctx context.Context) ([]model.AgentDescriptor, error) {
	return p.agents, nil
}

func (p *OpenClawProvider) GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error) {
	for _, a := range p.agents {
		if a.ID == agentID {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("agent not found: %s", agentID)
}

func (p *OpenClawProvider) CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error) {
	now := time.Now()
	sess := &model.Session{
		ID:        util.NewSessionID(),
		AgentID:   agentID,
		Provider:  "openclaw",
		Status:    model.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	p.mu.Lock()
	p.sessions[sess.ID] = sess
	p.mu.Unlock()
	return sess, nil
}

func (p *OpenClawProvider) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	sess, ok := p.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return sess, nil
}

func (p *OpenClawProvider) ListSessions(ctx context.Context, agentID string) ([]model.Session, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var sessions []model.Session
	for _, s := range p.sessions {
		if agentID == "" || s.AgentID == agentID {
			sessions = append(sessions, *s)
		}
	}
	return sessions, nil
}

func (p *OpenClawProvider) CloseSession(ctx context.Context, sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessions, sessionID)
	return nil
}

func (p *OpenClawProvider) SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (*model.Message, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	body := map[string]any{
		"message": text,
		"stream":  false,
	}

	respBody, err := p.doRequest(ctx, "POST", "/api/agents/main/chat", body)
	if err != nil {
		return nil, fmt.Errorf("openclaw send message: %w", err)
	}

	var result map[string]any
	json.Unmarshal(respBody, &result)

	response, _ := result["response"].(string)

	resp := &model.Message{
		ID:        util.NewMessageID(),
		SessionID: sessionID,
		Role:      model.RoleAgent,
		Content:   []model.ContentBlock{{Type: model.ContentText, Text: response}},
		Timestamp: time.Now(),
	}

	p.mu.Lock()
	p.messages[sessionID] = append(p.messages[sessionID], *msg, *resp)
	p.mu.Unlock()

	return resp, nil
}

func (p *OpenClawProvider) StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (<-chan model.StreamEvent, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	body := map[string]any{
		"message": text,
		"stream":  true,
	}

	bodyJSON, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/api/agents/main/chat", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	p.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openclaw stream: %w", err)
	}

	ch := make(chan model.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		for decoder.More() {
			var event map[string]any
			if err := decoder.Decode(&event); err != nil {
				break
			}

			evt := p.parseWSEvent(sessionID, event)
			if evt != nil {
				ch <- *evt
			}
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

func (p *OpenClawProvider) Cancel(ctx context.Context, sessionID string) error {
	return nil
}

func (p *OpenClawProvider) ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error) {
	return []model.ToolDefinition{
		{Name: "code_editor", Description: "Edit code files", InputSchema: map[string]any{"type": "object"}},
		{Name: "terminal", Description: "Run terminal commands", InputSchema: map[string]any{"type": "object"}},
	}, nil
}

func (p *OpenClawProvider) InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error) {
	return nil, fmt.Errorf("direct tool invocation not supported for openclaw provider")
}

func (p *OpenClawProvider) RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.approvals, approvalID)
	return nil
}

func (p *OpenClawProvider) ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented for openclaw provider")
}

func (p *OpenClawProvider) GetHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.messages[sessionID], nil
}

func (p *OpenClawProvider) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyJSON, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyJSON)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.endpoint+path, bodyReader)
	if err != nil {
		return nil, err
	}
	p.setAuth(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openclaw api error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (p *OpenClawProvider) setAuth(req *http.Request) {
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
}

func (p *OpenClawProvider) parseWSEvent(sessionID string, event map[string]any) *model.StreamEvent {
	eventType, _ := event["type"].(string)

	switch eventType {
	case "agent_update":
		content, _ := event["content"].(string)
		if content != "" {
			return &model.StreamEvent{
				Type:      model.EventMessageChunk,
				SessionID: sessionID,
				Role:      model.RoleAgent,
				Content:   &model.ContentBlock{Type: model.ContentText, Text: content},
			}
		}

	case "task_step":
		toolName, _ := event["tool_name"].(string)
		if toolName != "" {
			toolInput, _ := event["input"].(map[string]any)
			return &model.StreamEvent{
				Type:       model.EventToolCall,
				SessionID:  sessionID,
				ToolCallID: util.NewID("tc"),
				ToolName:   toolName,
				ToolKind:   model.ToolKindOther,
				ToolInput:  toolInput,
			}
		}

	case "approval_request":
		approvalID, _ := event["approval_id"].(string)
		action, _ := event["action"].(string)
		riskLevel, _ := event["risk_level"].(string)
		return &model.StreamEvent{
			Type:        model.EventApprovalRequest,
			SessionID:   sessionID,
			ApprovalID:  approvalID,
			ActionType:  action,
			Description: action,
			RiskLevel:   riskLevel,
		}
	}

	return nil
}
