package generic

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

type Protocol string

const (
	ProtocolA2A    Protocol = "a2a"
	ProtocolMCP    Protocol = "mcp"
	ProtocolOpenAI Protocol = "openai"
	ProtocolAuto   Protocol = "auto"
)

type AutoDiscoveryProvider struct {
	endpoint   string
	apiKey     string
	protocol   Protocol
	httpClient *http.Client
	logger     zerolog.Logger

	discoveredProtocol Protocol
	agentCard          map[string]any
	agentID            string

	mu       sync.RWMutex
	sessions map[string]*model.Session
	messages map[string][]model.Message
}

func New(logger zerolog.Logger) *AutoDiscoveryProvider {
	return &AutoDiscoveryProvider{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		protocol:   ProtocolAuto,
		logger:     logger,
		sessions:   make(map[string]*model.Session),
		messages:   make(map[string][]model.Message),
	}
}

func (p *AutoDiscoveryProvider) Name() string {
	if p.discoveredProtocol != "" {
		return "generic:" + string(p.discoveredProtocol)
	}
	return "generic"
}

func (p *AutoDiscoveryProvider) Initialize(ctx context.Context, cfg provider.ProviderConfig) error {
	p.endpoint = strings.TrimRight(cfg.Endpoint, "/")
	p.apiKey = cfg.Auth.APIKey
	if p.apiKey == "" {
		p.apiKey = cfg.Auth.Token
	}

	if proto, ok := cfg.Options["protocol"].(string); ok && proto != "" {
		p.protocol = Protocol(proto)
	}

	if p.protocol == ProtocolAuto {
		if err := p.discover(ctx); err != nil {
			return fmt.Errorf("auto-discovery failed for %s: %w", p.endpoint, err)
		}
	} else {
		p.discoveredProtocol = p.protocol
		if err := p.loadAgentInfo(ctx); err != nil {
			p.logger.Warn().Err(err).Msg("failed to load agent info, using defaults")
		}
	}

	p.logger.Info().
		Str("endpoint", p.endpoint).
		Str("protocol", string(p.discoveredProtocol)).
		Str("agent_id", p.agentID).
		Msg("auto-discovery provider initialized")

	return nil
}

func (p *AutoDiscoveryProvider) discover(ctx context.Context) error {
	probes := []struct {
		protocol Protocol
		path     string
		method   string
		validate func(resp *http.Response) bool
	}{
		{
			protocol: ProtocolA2A,
			path:     "/.well-known/agent.json",
			method:   "GET",
			validate: func(resp *http.Response) bool {
				return resp.StatusCode == 200
			},
		},
		{
			protocol: ProtocolA2A,
			path:     "/.well-known/a2a.json",
			method:   "GET",
			validate: func(resp *http.Response) bool {
				return resp.StatusCode == 200
			},
		},
		{
			protocol: ProtocolMCP,
			path:     "/mcp",
			method:   "POST",
			validate: func(resp *http.Response) bool {
				ct := resp.Header.Get("Content-Type")
				return strings.Contains(ct, "application/json") || strings.Contains(ct, "text/event-stream")
			},
		},
		{
			protocol: ProtocolOpenAI,
			path:     "/v1/models",
			method:   "GET",
			validate: func(resp *http.Response) bool {
				return resp.StatusCode == 200
			},
		},
		{
			protocol: ProtocolOpenAI,
			path:     "/v1/chat/completions",
			method:   "POST",
			validate: func(resp *http.Response) bool {
				return resp.StatusCode == 200 || resp.StatusCode == 400
			},
		},
	}

	for _, probe := range probes {
		req, err := http.NewRequestWithContext(ctx, probe.method, p.endpoint+probe.path, nil)
		if err != nil {
			continue
		}
		p.setAuth(req)

		if probe.protocol == ProtocolMCP {
			mcpInit := map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "initialize",
				"params": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
					"clientInfo":      map[string]any{"name": "agent-gateway", "version": "1.0.0"},
				},
			}
			body, _ := json.Marshal(mcpInit)
			req, _ = http.NewRequestWithContext(ctx, "POST", p.endpoint+probe.path, bytes.NewReader(body))
			p.setAuth(req)
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := p.httpClient.Do(req)
		if err != nil {
			p.logger.Debug().Str("protocol", string(probe.protocol)).Str("path", probe.path).Err(err).Msg("probe failed")
			continue
		}

		valid := probe.validate(resp)

		if probe.protocol == ProtocolA2A && resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var card map[string]any
			if json.Unmarshal(body, &card) == nil {
				p.agentCard = card
				if name, ok := card["name"].(string); ok {
					p.agentID = "generic:" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
				}
			}
		} else {
			resp.Body.Close()
		}

		if valid {
			p.discoveredProtocol = probe.protocol
			p.logger.Info().
				Str("protocol", string(probe.protocol)).
				Str("path", probe.path).
				Msg("protocol discovered")
			return nil
		}
	}

	return fmt.Errorf("could not detect protocol at %s, tried A2A/MCP/OpenAI", p.endpoint)
}

func (p *AutoDiscoveryProvider) loadAgentInfo(ctx context.Context) error {
	if p.discoveredProtocol == ProtocolA2A {
		paths := []string{"/.well-known/agent.json", "/.well-known/a2a.json"}
		for _, path := range paths {
			req, err := http.NewRequestWithContext(ctx, "GET", p.endpoint+path, nil)
			if err != nil {
				continue
			}
			p.setAuth(req)
			resp, err := p.httpClient.Do(req)
			if err != nil {
				continue
			}
			if resp.StatusCode == 200 {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				var card map[string]any
				if json.Unmarshal(body, &card) == nil {
					p.agentCard = card
					if name, ok := card["name"].(string); ok {
						p.agentID = "generic:" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
					}
				}
				return nil
			}
			resp.Body.Close()
		}
	}
	return nil
}

func (p *AutoDiscoveryProvider) Shutdown(ctx context.Context) error { return nil }

func (p *AutoDiscoveryProvider) HealthCheck(ctx context.Context) error {
	var path string
	switch p.discoveredProtocol {
	case ProtocolA2A:
		path = "/.well-known/agent.json"
	case ProtocolMCP:
		path = "/mcp"
	case ProtocolOpenAI:
		path = "/v1/models"
	default:
		path = "/"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", p.endpoint+path, nil)
	if err != nil {
		return err
	}
	p.setAuth(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("health check status: %d", resp.StatusCode)
	}
	return nil
}

func (p *AutoDiscoveryProvider) ListAgents(ctx context.Context) ([]model.AgentDescriptor, error) {
	agentID := p.agentID
	if agentID == "" {
		host := strings.TrimPrefix(p.endpoint, "http://")
		host = strings.TrimPrefix(host, "https://")
		agentID = "generic:" + strings.ReplaceAll(host, ":", "-")
	}

	name := "Unknown Agent"
	desc := fmt.Sprintf("Auto-discovered agent at %s (%s)", p.endpoint, p.discoveredProtocol)

	capabilities := model.AgentCapabilities{
		Streaming:      true,
		ToolUse:        p.discoveredProtocol == ProtocolMCP,
		HumanInTheLoop: p.discoveredProtocol == ProtocolA2A,
		Memory:         true,
	}

	if p.agentCard != nil {
		if n, ok := p.agentCard["name"].(string); ok {
			name = n
		}
		if d, ok := p.agentCard["description"].(string); ok {
			desc = d
		}
		if caps, ok := p.agentCard["capabilities"].(map[string]any); ok {
			if s, ok := caps["streaming"].(bool); ok {
				capabilities.Streaming = s
			}
			if p, ok := caps["pushNotifications"].(bool); ok {
				capabilities.PushNotifications = p
			}
		}
		if skills, ok := p.agentCard["skills"].([]any); ok {
			var agentSkills []model.AgentSkill
			for _, s := range skills {
				if skillMap, ok := s.(map[string]any); ok {
					skill := model.AgentSkill{}
					if id, ok := skillMap["id"].(string); ok {
						skill.ID = id
					}
					if n, ok := skillMap["name"].(string); ok {
						skill.Name = n
					}
					if d, ok := skillMap["description"].(string); ok {
						skill.Description = d
					}
					agentSkills = append(agentSkills, skill)
				}
			}
			if len(agentSkills) > 0 {
				_ = agentSkills
			}
		}
	}

	return []model.AgentDescriptor{
		{
			ID:           agentID,
			Name:         name,
			Description:  desc,
			Provider:     "generic",
			Capabilities: capabilities,
		},
	}, nil
}

func (p *AutoDiscoveryProvider) GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error) {
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

func (p *AutoDiscoveryProvider) CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error) {
	now := time.Now()
	sess := &model.Session{
		ID:        util.NewSessionID(),
		AgentID:   agentID,
		Provider:  "generic",
		Status:    model.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	p.mu.Lock()
	p.sessions[sess.ID] = sess
	p.mu.Unlock()
	return sess, nil
}

func (p *AutoDiscoveryProvider) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	sess, ok := p.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return sess, nil
}

func (p *AutoDiscoveryProvider) ListSessions(ctx context.Context, agentID string) ([]model.Session, error) {
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

func (p *AutoDiscoveryProvider) CloseSession(ctx context.Context, sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessions, sessionID)
	return nil
}

func (p *AutoDiscoveryProvider) SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (*model.Message, error) {
	switch p.discoveredProtocol {
	case ProtocolA2A:
		return p.sendA2A(ctx, sessionID, msg)
	case ProtocolMCP:
		return p.sendMCP(ctx, sessionID, msg)
	case ProtocolOpenAI:
		return p.sendOpenAI(ctx, sessionID, msg)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", p.discoveredProtocol)
	}
}

func (p *AutoDiscoveryProvider) StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (<-chan model.StreamEvent, error) {
	switch p.discoveredProtocol {
	case ProtocolA2A:
		return p.streamA2A(ctx, sessionID, msg)
	case ProtocolOpenAI:
		return p.streamOpenAI(ctx, sessionID, msg)
	case ProtocolMCP:
		return p.streamMCP(ctx, sessionID, msg)
	default:
		return nil, fmt.Errorf("unsupported protocol for streaming: %s", p.discoveredProtocol)
	}
}

func (p *AutoDiscoveryProvider) Cancel(ctx context.Context, sessionID string) error {
	if p.discoveredProtocol == ProtocolA2A {
		return p.cancelA2A(ctx, sessionID)
	}
	return nil
}

func (p *AutoDiscoveryProvider) ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error) {
	if p.discoveredProtocol == ProtocolMCP {
		return p.listMCPTools(ctx)
	}
	return nil, nil
}

func (p *AutoDiscoveryProvider) InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error) {
	if p.discoveredProtocol == ProtocolMCP {
		return p.invokeMCPTool(ctx, toolName, input)
	}
	return nil, fmt.Errorf("tool invocation not supported for protocol: %s", p.discoveredProtocol)
}

func (p *AutoDiscoveryProvider) RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error {
	return nil
}

func (p *AutoDiscoveryProvider) ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *AutoDiscoveryProvider) GetHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.messages[sessionID], nil
}

// ===== A2A Protocol Implementation =====

func (p *AutoDiscoveryProvider) sendA2A(ctx context.Context, sessionID string, msg *model.Message) (*model.Message, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	a2aReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tasks/send",
		"params": map[string]any{
			"id": sessionID,
			"message": map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{"type": "text", "text": text},
				},
			},
		},
	}

	respBody, err := p.doJSONRequest(ctx, "POST", "/", a2aReq)
	if err != nil {
		return nil, fmt.Errorf("a2a send: %w", err)
	}

	return p.parseA2AResponse(respBody, sessionID), nil
}

func (p *AutoDiscoveryProvider) streamA2A(ctx context.Context, sessionID string, msg *model.Message) (<-chan model.StreamEvent, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	a2aReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tasks/sendSubscribe",
		"params": map[string]any{
			"id": sessionID,
			"message": map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{"type": "text", "text": text},
				},
			},
		},
	}

	bodyJSON, _ := json.Marshal(a2aReq)
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	p.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("a2a stream: %w", err)
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
			if json.Unmarshal([]byte(data), &event) != nil {
				continue
			}

			for _, evt := range p.parseA2ASSEEvent(sessionID, event) {
				ch <- evt
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

func (p *AutoDiscoveryProvider) cancelA2A(ctx context.Context, sessionID string) error {
	a2aReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tasks/cancel",
		"params": map[string]any{
			"id": sessionID,
		},
	}
	_, err := p.doJSONRequest(ctx, "POST", "/", a2aReq)
	return err
}

// ===== MCP Protocol Implementation =====

func (p *AutoDiscoveryProvider) sendMCP(ctx context.Context, sessionID string, msg *model.Message) (*model.Message, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	mcpReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "chat",
			"arguments": map[string]any{
				"message":     text,
				"session_id":  sessionID,
			},
		},
	}

	respBody, err := p.doJSONRequest(ctx, "POST", "/mcp", mcpReq)
	if err != nil {
		return nil, fmt.Errorf("mcp send: %w", err)
	}

	return p.parseMCPResponse(respBody, sessionID), nil
}

func (p *AutoDiscoveryProvider) streamMCP(ctx context.Context, sessionID string, msg *model.Message) (<-chan model.StreamEvent, error) {
	resp, err := p.SendMessage(ctx, sessionID, msg, nil)
	if err != nil {
		return nil, err
	}

	ch := make(chan model.StreamEvent, 2)
	go func() {
		defer close(ch)
		ch <- model.StreamEvent{
			Type:      model.EventMessageChunk,
			SessionID: sessionID,
			Role:      model.RoleAgent,
			Content:   &resp.Content[0],
		}
		ch <- model.StreamEvent{
			Type:       model.EventTaskStatus,
			SessionID:  sessionID,
			TaskStatus: model.TaskCompleted,
		}
	}()

	return ch, nil
}

func (p *AutoDiscoveryProvider) listMCPTools(ctx context.Context) ([]model.ToolDefinition, error) {
	mcpReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]any{},
	}

	respBody, err := p.doJSONRequest(ctx, "POST", "/mcp", mcpReq)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	json.Unmarshal(respBody, &result)

	res, _ := result["result"].(map[string]any)
	toolsRaw, _ := res["tools"].([]any)

	var tools []model.ToolDefinition
	for _, t := range toolsRaw {
		if toolMap, ok := t.(map[string]any); ok {
			tool := model.ToolDefinition{}
			if n, ok := toolMap["name"].(string); ok {
				tool.Name = n
			}
			if d, ok := toolMap["description"].(string); ok {
				tool.Description = d
			}
			if s, ok := toolMap["inputSchema"].(map[string]any); ok {
				tool.InputSchema = s
			}
			tools = append(tools, tool)
		}
	}
	return tools, nil
}

func (p *AutoDiscoveryProvider) invokeMCPTool(ctx context.Context, toolName string, input map[string]any) (any, error) {
	mcpReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": input,
		},
	}

	respBody, err := p.doJSONRequest(ctx, "POST", "/mcp", mcpReq)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	json.Unmarshal(respBody, &result)
	return result["result"], nil
}

// ===== OpenAI Compatible Implementation =====

func (p *AutoDiscoveryProvider) sendOpenAI(ctx context.Context, sessionID string, msg *model.Message) (*model.Message, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	chatReq := map[string]any{
		"model": "default",
		"messages": []map[string]any{
			{"role": "user", "content": text},
		},
	}

	respBody, err := p.doJSONRequest(ctx, "POST", "/v1/chat/completions", chatReq)
	if err != nil {
		return nil, fmt.Errorf("openai send: %w", err)
	}

	return p.parseOpenAIResponse(respBody, sessionID), nil
}

func (p *AutoDiscoveryProvider) streamOpenAI(ctx context.Context, sessionID string, msg *model.Message) (<-chan model.StreamEvent, error) {
	var text string
	for _, block := range msg.Content {
		if block.Type == model.ContentText {
			text = block.Text
		}
	}

	chatReq := map[string]any{
		"model":  "default",
		"stream": true,
		"messages": []map[string]any{
			{"role": "user", "content": text},
		},
	}

	bodyJSON, _ := json.Marshal(chatReq)
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/v1/chat/completions", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	p.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai stream: %w", err)
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
			if data == "[DONE]" {
				break
			}

			var event map[string]any
			if json.Unmarshal([]byte(data), &event) != nil {
				continue
			}

			evt := p.parseOpenAISSEEvent(sessionID, event)
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

// ===== HTTP Helpers =====

func (p *AutoDiscoveryProvider) doJSONRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
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
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (p *AutoDiscoveryProvider) setAuth(req *http.Request) {
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
}

// ===== Response Parsers =====

func (p *AutoDiscoveryProvider) parseA2AResponse(respBody []byte, sessionID string) *model.Message {
	var result map[string]any
	json.Unmarshal(respBody, &result)

	resultData, _ := result["result"].(map[string]any)
	status, _ := resultData["status"].(string)
	_ = status

	artifacts, _ := resultData["artifacts"].([]any)
	if len(artifacts) > 0 {
		if artifact, ok := artifacts[0].(map[string]any); ok {
			if parts, ok := artifact["parts"].([]any); ok && len(parts) > 0 {
				if part, ok := parts[0].(map[string]any); ok {
					if text, ok := part["text"].(string); ok {
						return &model.Message{
							ID:        util.NewMessageID(),
							SessionID: sessionID,
							Role:      model.RoleAgent,
							Content:   []model.ContentBlock{{Type: model.ContentText, Text: text}},
							Timestamp: time.Now(),
						}
					}
				}
			}
		}
	}

	return &model.Message{
		ID:        util.NewMessageID(),
		SessionID: sessionID,
		Role:      model.RoleAgent,
		Content:   []model.ContentBlock{{Type: model.ContentText, Text: ""}},
		Timestamp: time.Now(),
	}
}

func (p *AutoDiscoveryProvider) parseA2ASSEEvent(sessionID string, event map[string]any) []model.StreamEvent {
	var events []model.StreamEvent

	result, _ := event["result"].(map[string]any)
	if result == nil {
		return nil
	}

	artifacts, _ := result["artifacts"].([]any)
	for _, a := range artifacts {
		artifact, _ := a.(map[string]any)
		parts, _ := artifact["parts"].([]any)
		for _, part := range parts {
			p, _ := part.(map[string]any)
			if text, ok := p["text"].(string); ok {
				events = append(events, model.StreamEvent{
					Type:      model.EventMessageChunk,
					SessionID: sessionID,
					Role:      model.RoleAgent,
					Content:   &model.ContentBlock{Type: model.ContentText, Text: text},
				})
			}
		}
	}

	if status, ok := result["status"].(string); ok {
		taskStatus := model.TaskWorking
		switch status {
		case "completed":
			taskStatus = model.TaskCompleted
		case "failed":
			taskStatus = model.TaskFailed
		case "canceled":
			taskStatus = model.TaskCancelled
		}
		events = append(events, model.StreamEvent{
			Type:       model.EventTaskStatus,
			SessionID:  sessionID,
			TaskStatus: taskStatus,
		})
	}

	return events
}

func (p *AutoDiscoveryProvider) parseMCPResponse(respBody []byte, sessionID string) *model.Message {
	var result map[string]any
	json.Unmarshal(respBody, &result)

	res, _ := result["result"].(map[string]any)
	content, _ := res["content"].([]any)

	var text string
	for _, c := range content {
		if contentMap, ok := c.(map[string]any); ok {
			if t, ok := contentMap["text"].(string); ok {
				text = t
				break
			}
		}
	}

	return &model.Message{
		ID:        util.NewMessageID(),
		SessionID: sessionID,
		Role:      model.RoleAgent,
		Content:   []model.ContentBlock{{Type: model.ContentText, Text: text}},
		Timestamp: time.Now(),
	}
}

func (p *AutoDiscoveryProvider) parseOpenAIResponse(respBody []byte, sessionID string) *model.Message {
	var result map[string]any
	json.Unmarshal(respBody, &result)

	choices, _ := result["choices"].([]any)
	if len(choices) == 0 {
		return &model.Message{
			ID:        util.NewMessageID(),
			SessionID: sessionID,
			Role:      model.RoleAgent,
			Content:   []model.ContentBlock{{Type: model.ContentText, Text: ""}},
			Timestamp: time.Now(),
		}
	}

	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	content, _ := message["content"].(string)

	return &model.Message{
		ID:        util.NewMessageID(),
		SessionID: sessionID,
		Role:      model.RoleAgent,
		Content:   []model.ContentBlock{{Type: model.ContentText, Text: content}},
		Timestamp: time.Now(),
	}
}

func (p *AutoDiscoveryProvider) parseOpenAISSEEvent(sessionID string, event map[string]any) *model.StreamEvent {
	choices, _ := event["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}

	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	content, _ := delta["content"].(string)

	if content == "" {
		return nil
	}

	return &model.StreamEvent{
		Type:      model.EventMessageChunk,
		SessionID: sessionID,
		Role:      model.RoleAgent,
		Content:   &model.ContentBlock{Type: model.ContentText, Text: content},
	}
}
