package dify

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/agent-gateway/gateway/internal/model"
	"github.com/agent-gateway/gateway/internal/provider"
	"github.com/agent-gateway/gateway/internal/util"
)

type DifyProvider struct {
	endpoint   string
	apiKey     string
	appType    string
	httpClient *http.Client
	logger     zerolog.Logger
}

func New(logger zerolog.Logger) *DifyProvider {
	return &DifyProvider{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		appType:    "chat",
		logger:     logger,
	}
}

func (p *DifyProvider) Name() string { return "dify" }

func (p *DifyProvider) Initialize(ctx context.Context, cfg provider.ProviderConfig) error {
	p.endpoint = strings.TrimRight(cfg.Endpoint, "/")
	p.apiKey = cfg.Auth.APIKey
	if p.apiKey == "" {
		p.apiKey = cfg.Auth.Token
	}
	if appType, ok := cfg.Options["default_app_type"].(string); ok && appType != "" {
		p.appType = appType
	}
	return nil
}

func (p *DifyProvider) Shutdown(ctx context.Context) error { return nil }

func (p *DifyProvider) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", p.endpoint+"/parameters", nil)
	if err != nil {
		return err
	}
	p.setAuth(req)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("dify health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("dify health check status: %d", resp.StatusCode)
	}
	return nil
}

func (p *DifyProvider) ListAgents(ctx context.Context) ([]model.AgentDescriptor, error) {
	return []model.AgentDescriptor{
		{
			ID:          "dify:" + p.appType,
			Name:        "Dify " + p.appType + " App",
			Description: "Dify " + p.appType + " application",
			Provider:    "dify",
			Capabilities: model.AgentCapabilities{
				Streaming:      true,
				ToolUse:        true,
				HumanInTheLoop: p.appType == "workflow",
				Memory:         p.appType == "chat",
			},
		},
	}, nil
}

func (p *DifyProvider) GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error) {
	agents, err := p.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		if a.ID == agentID {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("agent not found: %s", agentID)
}

func (p *DifyProvider) CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error) {
	now := time.Now()
	return &model.Session{
		ID:        util.NewSessionID(),
		AgentID:   agentID,
		Provider:  "dify",
		Status:    model.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (p *DifyProvider) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	return &model.Session{
		ID:       sessionID,
		Provider: "dify",
		Status:   model.SessionActive,
	}, nil
}

func (p *DifyProvider) ListSessions(ctx context.Context, agentID string) ([]model.Session, error) {
	return nil, nil
}

func (p *DifyProvider) CloseSession(ctx context.Context, sessionID string) error {
	return nil
}

func (p *DifyProvider) SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (*model.Message, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	body := map[string]any{
		"inputs":        map[string]any{},
		"query":         text,
		"response_mode": "blocking",
		"user":          "agent-gateway",
	}
	if sessionID != "" {
		body["conversation_id"] = sessionID
	}

	respBody, err := p.doRequest(ctx, "POST", "/chat-messages", body)
	if err != nil {
		return nil, fmt.Errorf("dify send message: %w", err)
	}

	return p.parseBlockingResponse(respBody, sessionID), nil
}

func (p *DifyProvider) StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (<-chan model.StreamEvent, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	body := map[string]any{
		"inputs":        map[string]any{},
		"query":         text,
		"response_mode": "streaming",
		"user":          "agent-gateway",
	}
	if sessionID != "" {
		body["conversation_id"] = sessionID
	}

	bodyJSON, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat-messages", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	p.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dify stream request: %w", err)
	}

	ch := make(chan model.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "" {
				continue
			}

			var event map[string]any
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			evt := p.parseSSEEvent(sessionID, event)
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

	return ch, nil
}

func (p *DifyProvider) Cancel(ctx context.Context, sessionID string) error {
	return nil
}

func (p *DifyProvider) ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error) {
	return nil, nil
}

func (p *DifyProvider) InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error) {
	return nil, fmt.Errorf("direct tool invocation not supported for dify provider")
}

func (p *DifyProvider) RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error {
	return nil
}

func (p *DifyProvider) ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented for dify provider")
}

func (p *DifyProvider) GetHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	respBody, err := p.doRequest(ctx, "GET", "/messages?conversation_id="+sessionID+"&limit=100", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []map[string]any `json:"data"`
	}
	json.Unmarshal(respBody, &result)

	var messages []model.Message
	for _, m := range result.Data {
		role := model.RoleAgent
		if isUser, _ := m["is_user"].(bool); isUser {
			role = model.RoleUser
		}
		content, _ := m["answer"].(string)
		if query, _ := m["query"].(string); query != "" {
			content = query
		}

		messages = append(messages, model.Message{
			ID:        util.NewMessageID(),
			SessionID: sessionID,
			Role:      role,
			Content:   []model.ContentBlock{{Type: model.ContentText, Text: content}},
			Timestamp: time.Now(),
		})
	}

	return messages, nil
}

func (p *DifyProvider) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
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
		return nil, fmt.Errorf("dify api error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (p *DifyProvider) setAuth(req *http.Request) {
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
}

func (p *DifyProvider) parseBlockingResponse(respBody []byte, sessionID string) *model.Message {
	var result map[string]any
	json.Unmarshal(respBody, &result)

	answer, _ := result["answer"].(string)
	convID, _ := result["conversation_id"].(string)
	_ = convID

	return &model.Message{
		ID:        util.NewMessageID(),
		SessionID: sessionID,
		Role:      model.RoleAgent,
		Content:   []model.ContentBlock{{Type: model.ContentText, Text: answer}},
		Timestamp: time.Now(),
	}
}

func (p *DifyProvider) parseSSEEvent(sessionID string, event map[string]any) *model.StreamEvent {
	eventType, _ := event["event"].(string)

	switch eventType {
	case "message", "agent_message":
		answer, _ := event["answer"].(string)
		if answer != "" {
			return &model.StreamEvent{
				Type:      model.EventMessageChunk,
				SessionID: sessionID,
				Role:      model.RoleAgent,
				Content:   &model.ContentBlock{Type: model.ContentText, Text: answer},
			}
		}

	case "agent_thought":
		thought, _ := event["thought"].(string)
		if thought != "" {
			return &model.StreamEvent{
				Type:      model.EventThoughtChunk,
				SessionID: sessionID,
				Content:   &model.ContentBlock{Type: model.ContentText, Text: thought},
			}
		}

	case "tool_used":
		toolName, _ := event["tool_name"].(string)
		toolInput, _ := event["tool_input"].(string)
		return &model.StreamEvent{
			Type:       model.EventToolCall,
			SessionID:  sessionID,
			ToolCallID: util.NewID("tc"),
			ToolName:   toolName,
			ToolKind:   model.ToolKindOther,
			ToolInput:  map[string]any{"input": toolInput},
		}

	case "message_end":
		return &model.StreamEvent{
			Type:       model.EventTaskStatus,
			SessionID:  sessionID,
			TaskStatus: model.TaskCompleted,
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
