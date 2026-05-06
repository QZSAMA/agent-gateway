package langgraph

import (
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

type LangGraphProvider struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	logger     zerolog.Logger
	assistants []string
}

func New(logger zerolog.Logger) *LangGraphProvider {
	return &LangGraphProvider{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		logger:     logger,
	}
}

func (p *LangGraphProvider) Name() string {
	return "langgraph"
}

func (p *LangGraphProvider) Initialize(ctx context.Context, cfg provider.ProviderConfig) error {
	p.endpoint = strings.TrimRight(cfg.Endpoint, "/")
	p.apiKey = cfg.Auth.APIKey
	if p.apiKey == "" {
		p.apiKey = cfg.Auth.Token
	}

	if opts, ok := cfg.Options["default_assistant"].(string); ok && opts != "" {
		p.assistants = []string{opts}
	} else {
		p.assistants = []string{"agent"}
	}

	return nil
}

func (p *LangGraphProvider) Shutdown(ctx context.Context) error {
	return nil
}

func (p *LangGraphProvider) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", p.endpoint+"/ok", nil)
	if err != nil {
		return err
	}
	p.setAuth(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check status: %d", resp.StatusCode)
	}
	return nil
}

func (p *LangGraphProvider) ListAgents(ctx context.Context) ([]model.AgentDescriptor, error) {
	var agents []model.AgentDescriptor
	for _, name := range p.assistants {
		agents = append(agents, model.AgentDescriptor{
			ID:          "langgraph:" + name,
			Name:        "LangGraph Agent: " + name,
			Description: "LangGraph agent deployed as assistant: " + name,
			Provider:    "langgraph",
			Capabilities: model.AgentCapabilities{
				Streaming:        true,
				ToolUse:          true,
				HumanInTheLoop:   true,
				LongRunningTasks: true,
				Memory:           true,
			},
		})
	}
	return agents, nil
}

func (p *LangGraphProvider) GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error) {
	agents, err := p.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		if a.ID == agentID || "langgraph:"+a.ID == agentID {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("agent not found: %s", agentID)
}

func (p *LangGraphProvider) CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error) {
	body := map[string]any{"metadata": map[string]any{}}
	if opts != nil && opts.Metadata != nil {
		body["metadata"] = opts.Metadata
	}

	respBody, err := p.doRequest(ctx, "POST", "/threads", body)
	if err != nil {
		return nil, fmt.Errorf("create thread: %w", err)
	}

	var result map[string]any
	json.Unmarshal(respBody, &result)

	threadID, _ := result["thread_id"].(string)
	now := time.Now()

	return &model.Session{
		ID:        threadID,
		AgentID:   agentID,
		Provider:  "langgraph",
		Status:    model.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (p *LangGraphProvider) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	respBody, err := p.doRequest(ctx, "GET", "/threads/"+sessionID, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	json.Unmarshal(respBody, &result)

	threadID, _ := result["thread_id"].(string)
	now := time.Now()

	return &model.Session{
		ID:        threadID,
		AgentID:   p.assistants[0],
		Provider:  "langgraph",
		Status:    model.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (p *LangGraphProvider) ListSessions(ctx context.Context, agentID string) ([]model.Session, error) {
	respBody, err := p.doRequest(ctx, "GET", "/threads?limit=100", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Threads []map[string]any `json:"threads"`
	}
	json.Unmarshal(respBody, &result)

	var sessions []model.Session
	for _, t := range result.Threads {
		threadID, _ := t["thread_id"].(string)
		createdAt, _ := t["created_at"].(string)
		parsedTime := time.Now()
		if createdAt != "" {
			parsedTime, _ = time.Parse(time.RFC3339, createdAt)
		}

		sessions = append(sessions, model.Session{
			ID:        threadID,
			AgentID:   agentID,
			Provider:  "langgraph",
			Status:    model.SessionActive,
			CreatedAt: parsedTime,
			UpdatedAt: parsedTime,
		})
	}
	return sessions, nil
}

func (p *LangGraphProvider) CloseSession(ctx context.Context, sessionID string) error {
	_, err := p.doRequest(ctx, "DELETE", "/threads/"+sessionID, nil)
	return err
}

func (p *LangGraphProvider) SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (*model.Message, error) {
	assistantID := p.resolveAssistantID("")

	input := map[string]any{
		"messages": p.convertMessage(msg),
	}

	body := map[string]any{
		"assistant_id": assistantID,
		"input":        input,
		"stream_mode":  []string{"values"},
	}

	respBody, err := p.doRequest(ctx, "POST", "/threads/"+sessionID+"/runs/wait", body)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	return p.parseResponse(respBody, sessionID), nil
}

func (p *LangGraphProvider) StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (<-chan model.StreamEvent, error) {
	assistantID := p.resolveAssistantID("")
	ch := make(chan model.StreamEvent, 64)

	input := map[string]any{
		"messages": p.convertMessage(msg),
	}

	body := map[string]any{
		"assistant_id": assistantID,
		"input":        input,
		"stream_mode":  []string{"messages", "updates"},
	}

	bodyJSON, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/threads/"+sessionID+"/runs/stream", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	p.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stream request: %w", err)
	}

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		for decoder.More() {
			var event map[string]any
			if err := decoder.Decode(&event); err != nil {
				if err == io.EOF {
					break
				}
				ch <- model.StreamEvent{
					Type:         model.EventError,
					SessionID:    sessionID,
					ErrorMessage: err.Error(),
				}
				break
			}

			for _, evt := range p.parseStreamEvent(sessionID, event) {
				ch <- evt
			}
		}

		ch <- model.StreamEvent{
			Type:      model.EventTaskStatus,
			SessionID: sessionID,
			TaskStatus: model.TaskCompleted,
		}
	}()

	return ch, nil
}

func (p *LangGraphProvider) Cancel(ctx context.Context, sessionID string) error {
	return nil
}

func (p *LangGraphProvider) ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error) {
	return nil, nil
}

func (p *LangGraphProvider) InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error) {
	return nil, fmt.Errorf("direct tool invocation not supported for langgraph provider")
}

func (p *LangGraphProvider) RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error {
	return fmt.Errorf("not implemented for langgraph provider")
}

func (p *LangGraphProvider) ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error) {
	assistantID := p.resolveAssistantID("")

	body := map[string]any{
		"assistant_id": assistantID,
		"command": map[string]any{
			"resume": resumeData,
		},
		"stream_mode": []string{"values"},
	}

	respBody, err := p.doRequest(ctx, "POST", "/threads/"+sessionID+"/runs/wait", body)
	if err != nil {
		return nil, fmt.Errorf("resume task: %w", err)
	}

	return p.parseResponse(respBody, sessionID), nil
}

func (p *LangGraphProvider) GetHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	respBody, err := p.doRequest(ctx, "GET", "/threads/"+sessionID+"/state", nil)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	json.Unmarshal(respBody, &result)

	values, _ := result["values"].(map[string]any)
	messagesRaw, _ := values["messages"].([]any)

	var messages []model.Message
	for _, m := range messagesRaw {
		msgMap, _ := m.(map[string]any)
		msgType, _ := msgMap["type"].(string)
		content, _ := msgMap["content"].(string)

		role := model.RoleAgent
		if msgType == "human" {
			role = model.RoleUser
		} else if msgType == "system" {
			role = model.RoleSystem
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

func (p *LangGraphProvider) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
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
		return nil, fmt.Errorf("langgraph api error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (p *LangGraphProvider) setAuth(req *http.Request) {
	if p.apiKey != "" {
		req.Header.Set("x-api-key", p.apiKey)
	}
}

func (p *LangGraphProvider) resolveAssistantID(agentID string) string {
	if agentID == "" {
		if len(p.assistants) > 0 {
			return p.assistants[0]
		}
		return "agent"
	}
	parts := strings.SplitN(agentID, ":", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return agentID
}

func (p *LangGraphProvider) convertMessage(msg *model.Message) []map[string]any {
	var messages []map[string]any
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			msgType := "human"
			if msg.Role == model.RoleAgent {
				msgType = "ai"
			} else if msg.Role == model.RoleSystem {
				msgType = "system"
			}
			messages = append(messages, map[string]any{
				"type":    msgType,
				"content": block.Text,
			})
		}
	}
	return messages
}

func (p *LangGraphProvider) parseResponse(respBody []byte, sessionID string) *model.Message {
	var result map[string]any
	json.Unmarshal(respBody, &result)

	values, _ := result["values"].(map[string]any)
	messages, _ := values["messages"].([]any)

	var lastContent string
	var lastRole string
	if len(messages) > 0 {
		lastMsg, _ := messages[len(messages)-1].(map[string]any)
		lastContent, _ = lastMsg["content"].(string)
		lastType, _ := lastMsg["type"].(string)
		if lastType == "ai" {
			lastRole = "agent"
		} else {
			lastRole = lastType
		}
	}

	role := model.RoleAgent
	if lastRole == "human" {
		role = model.RoleUser
	} else if lastRole == "system" {
		role = model.RoleSystem
	}

	return &model.Message{
		ID:        util.NewMessageID(),
		SessionID: sessionID,
		Role:      role,
		Content:   []model.ContentBlock{{Type: model.ContentText, Text: lastContent}},
		Timestamp: time.Now(),
	}
}

func (p *LangGraphProvider) parseStreamEvent(sessionID string, raw map[string]any) []model.StreamEvent {
	event, _ := raw["event"].(string)
	data, _ := raw["data"].(map[string]any)

	switch event {
	case "messages/partial", "messages/complete":
		msgType, _ := data["type"].(string)
		content, _ := data["content"].(string)
		if msgType == "ai" && content != "" {
			return []model.StreamEvent{{
				Type:      model.EventMessageChunk,
				SessionID: sessionID,
				Role:      model.RoleAgent,
				Content:   &model.ContentBlock{Type: model.ContentText, Text: content},
			}}
		}

	case "updates":
		for _, v := range data {
			nodeUpdate, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if msgs, ok := nodeUpdate["messages"].([]any); ok && len(msgs) > 0 {
				lastMsg, _ := msgs[len(msgs)-1].(map[string]any)
				content, _ := lastMsg["content"].(string)
				toolCalls, _ := lastMsg["tool_calls"].([]any)

				var events []model.StreamEvent
				if content != "" {
					events = append(events, model.StreamEvent{
						Type:      model.EventMessageChunk,
						SessionID: sessionID,
						Role:      model.RoleAgent,
						Content:   &model.ContentBlock{Type: model.ContentText, Text: content},
					})
				}

				for _, tc := range toolCalls {
					tcMap, _ := tc.(map[string]any)
					tcID, _ := tcMap["id"].(string)
					tcName, _ := tcMap["name"].(string)
					tcArgs, _ := tcMap["args"].(map[string]any)
					events = append(events, model.StreamEvent{
						Type:      model.EventToolCall,
						SessionID: sessionID,
						ToolCallID: tcID,
						ToolName:  tcName,
						ToolKind:  model.ToolKindOther,
						ToolInput: tcArgs,
					})
				}

				return events
			}
		}
	}

	return nil
}
