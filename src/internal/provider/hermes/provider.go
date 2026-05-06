package hermes

import (
	"bufio"
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

type HermesProvider struct {
	endpoint   string
	token      string
	httpClient *http.Client
	logger     zerolog.Logger
	agents     []model.AgentDescriptor
	mu         sync.RWMutex
	sessions   map[string]*model.Session
	messages   map[string][]model.Message
}

func New(logger zerolog.Logger) *HermesProvider {
	return &HermesProvider{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		logger:     logger,
		sessions:   make(map[string]*model.Session),
		messages:   make(map[string][]model.Message),
	}
}

func (p *HermesProvider) Name() string { return "hermes" }

func (p *HermesProvider) Initialize(ctx context.Context, cfg provider.ProviderConfig) error {
	p.endpoint = strings.TrimRight(cfg.Endpoint, "/")
	p.token = cfg.Auth.Token
	if p.token == "" {
		p.token = cfg.Auth.APIKey
	}

	p.agents = []model.AgentDescriptor{
		{
			ID:          "hermes:default",
			Name:        "Hermes Default Agent",
			Description: "Hermes AI agent with 47+ built-in tools",
			Provider:    "hermes",
			Capabilities: model.AgentCapabilities{
				Streaming:      true,
				ToolUse:       true,
				HumanInTheLoop: true,
				MultiModal:     true,
				LongRunningTasks: true,
				Memory:         true,
			},
		},
		{
			ID:          "hermes:code",
			Name:        "Hermes Code Agent",
			Description: "Hermes agent specialized for coding tasks",
			Provider:    "hermes",
			Capabilities: model.AgentCapabilities{
				Streaming:      true,
				ToolUse:       true,
				HumanInTheLoop: true,
				Memory:         true,
			},
		},
	}

	return nil
}

func (p *HermesProvider) Shutdown(ctx context.Context) error { return nil }

func (p *HermesProvider) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", p.endpoint+"/health", nil)
	if err != nil {
		return err
	}
	p.setAuth(req)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("hermes health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hermes health check status: %d", resp.StatusCode)
	}
	return nil
}

func (p *HermesProvider) ListAgents(ctx context.Context) ([]model.AgentDescriptor, error) {
	return p.agents, nil
}

func (p *HermesProvider) GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error) {
	for _, a := range p.agents {
		if a.ID == agentID {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("agent not found: %s", agentID)
}

func (p *HermesProvider) CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error) {
	now := time.Now()
	sess := &model.Session{
		ID:        util.NewSessionID(),
		AgentID:   agentID,
		Provider:  "hermes",
		Status:    model.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	p.mu.Lock()
	p.sessions[sess.ID] = sess
	p.mu.Unlock()
	return sess, nil
}

func (p *HermesProvider) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	sess, ok := p.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return sess, nil
}

func (p *HermesProvider) ListSessions(ctx context.Context, agentID string) ([]model.Session, error) {
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

func (p *HermesProvider) CloseSession(ctx context.Context, sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessions, sessionID)
	return nil
}

func (p *HermesProvider) SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (*model.Message, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	body := map[string]any{
		"session_id": sessionID,
		"message":    text,
		"stream":     false,
	}

	respBody, err := p.doRequest(ctx, "POST", "/api/chat", body)
	if err != nil {
		return nil, fmt.Errorf("hermes send message: %w", err)
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

func (p *HermesProvider) StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (<-chan model.StreamEvent, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	body := map[string]any{
		"session_id": sessionID,
		"message":    text,
		"stream":     true,
	}

	bodyJSON, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/api/chat", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	p.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hermes stream: %w", err)
	}

	ch := make(chan model.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			var event map[string]any
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}

			evt := p.parseStreamEvent(sessionID, event)
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

func (p *HermesProvider) Cancel(ctx context.Context, sessionID string) error {
	return nil
}

func (p *HermesProvider) ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error) {
	return []model.ToolDefinition{
		{Name: "file_read", Description: "Read file contents", InputSchema: map[string]any{"type": "object"}},
		{Name: "file_edit", Description: "Edit file contents", InputSchema: map[string]any{"type": "object"}},
		{Name: "terminal", Description: "Execute terminal commands", InputSchema: map[string]any{"type": "object"}},
		{Name: "web_search", Description: "Search the web", InputSchema: map[string]any{"type": "object"}},
	}, nil
}

func (p *HermesProvider) InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error) {
	return nil, fmt.Errorf("direct tool invocation not supported for hermes provider")
}

func (p *HermesProvider) RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error {
	return nil
}

func (p *HermesProvider) ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented for hermes provider")
}

func (p *HermesProvider) GetHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.messages[sessionID], nil
}

func (p *HermesProvider) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
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
		return nil, fmt.Errorf("hermes api error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (p *HermesProvider) setAuth(req *http.Request) {
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
}

func (p *HermesProvider) parseStreamEvent(sessionID string, event map[string]any) *model.StreamEvent {
	eventType, _ := event["type"].(string)

	switch eventType {
	case "content":
		text, _ := event["text"].(string)
		if text != "" {
			return &model.StreamEvent{
				Type:      model.EventMessageChunk,
				SessionID: sessionID,
				Role:      model.RoleAgent,
				Content:   &model.ContentBlock{Type: model.ContentText, Text: text},
			}
		}

	case "tool_use":
		toolName, _ := event["name"].(string)
		toolInput, _ := event["input"].(map[string]any)
		return &model.StreamEvent{
			Type:       model.EventToolCall,
			SessionID:  sessionID,
			ToolCallID: util.NewID("tc"),
			ToolName:   toolName,
			ToolKind:   model.ToolKindOther,
			ToolInput:  toolInput,
		}

	case "permission_request":
		action, _ := event["action"].(string)
		return &model.StreamEvent{
			Type:        model.EventApprovalRequest,
			SessionID:   sessionID,
			ApprovalID:  util.NewID("appr"),
			ActionType:  action,
			Description: action,
			RiskLevel:   "medium",
		}

	case "error":
		errMsg, _ := event["message"].(string)
		return &model.StreamEvent{
			Type:         model.EventError,
			SessionID:    sessionID,
			ErrorMessage: errMsg,
		}
	}

	return nil
}
