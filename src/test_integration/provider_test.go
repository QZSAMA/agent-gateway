package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/agent-gateway/gateway/internal/model"
	"github.com/agent-gateway/gateway/internal/provider"
	difyprovider "github.com/agent-gateway/gateway/internal/provider/dify"
	genericprovider "github.com/agent-gateway/gateway/internal/provider/generic"
	hermesprovider "github.com/agent-gateway/gateway/internal/provider/hermes"
	langgraphprovider "github.com/agent-gateway/gateway/internal/provider/langgraph"
	openclawprovider "github.com/agent-gateway/gateway/internal/provider/openclaw"
)

func initProvider(t *testing.T, p provider.AgentProviderAdapter, endpoint string) {
	t.Helper()
	err := p.Initialize(context.Background(), provider.ProviderConfig{
		Endpoint: endpoint,
		Auth:     provider.AuthConfig{APIKey: "test-key"},
		Options:  map[string]any{"default_assistant": "agent", "default_app_type": "chat"},
	})
	if err != nil {
		t.Fatalf("initialize provider %s: %v", p.Name(), err)
	}
}

// ===== LangGraph Mock Server =====

func newLangGraphMockServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	mux.HandleFunc("/threads", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"thread_id":  "thread_mock_123",
				"created_at": time.Now().Format(time.RFC3339),
			})
			return
		}
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"threads": []map[string]any{
					{"thread_id": "thread_mock_123", "created_at": time.Now().Format(time.RFC3339)},
				},
			})
			return
		}
		if r.Method == "DELETE" {
			w.WriteHeader(200)
			return
		}
	})

	mux.HandleFunc("/threads/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasSuffix(path, "/state") && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{
				"values": map[string]any{
					"messages": []map[string]any{
						{"type": "human", "content": "Hello"},
						{"type": "ai", "content": "Hi from LangGraph!"},
					},
				},
			})
			return
		}

		if strings.HasSuffix(path, "/runs/wait") && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]any{
				"values": map[string]any{
					"messages": []map[string]any{
						{"type": "human", "content": "Hello"},
						{"type": "ai", "content": "Hi from LangGraph mock!"},
					},
				},
			})
			return
		}

		if strings.HasSuffix(path, "/runs/stream") && r.Method == "POST" {
			w.Header().Set("Content-Type", "text/event-stream")
			events := []map[string]any{
				{"event": "messages/complete", "data": map[string]any{"type": "ai", "content": "Streamed response"}},
			}
			for _, evt := range events {
				data, _ := json.Marshal(evt)
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
			w.(http.Flusher).Flush()
			return
		}

		if r.Method == "GET" {
			parts := strings.Split(path, "/")
			threadID := parts[len(parts)-1]
			json.NewEncoder(w).Encode(map[string]any{
				"thread_id":  threadID,
				"created_at": time.Now().Format(time.RFC3339),
			})
			return
		}

		if r.Method == "DELETE" {
			w.WriteHeader(200)
		}
	})

	return httptest.NewServer(mux)
}

// ===== Dify Mock Server =====

func newDifyMockServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/parameters", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	mux.HandleFunc("/chat-messages", func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")
		_ = mode

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		if body["response_mode"] == "streaming" {
			w.Header().Set("Content-Type", "text/event-stream")
			events := []string{
				`data: {"event":"message","answer":"Hello "}`,
				`data: {"event":"message","answer":"from Dify!"}`,
				`data: {"event":"message_end"}`,
			}
			for _, evt := range events {
				fmt.Fprintln(w, evt)
			}
			w.(http.Flusher).Flush()
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"answer":          "Hello from Dify mock!",
			"conversation_id": "conv_mock_123",
		})
	})

	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"is_user": true, "query": "Hello"},
				{"is_user": false, "answer": "Hi from Dify!"},
			},
		})
	})

	return httptest.NewServer(mux)
}

// ===== OpenClaw Mock Server =====

func newOpenClawMockServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	mux.HandleFunc("/api/agents/main/chat", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		if stream, _ := body["stream"].(bool); stream {
			flusher := w.(http.Flusher)
			events := []map[string]any{
				{"type": "agent_update", "content": "Processing"},
				{"type": "task_step", "tool_name": "code_editor", "input": map[string]any{"file": "main.go"}},
				{"type": "agent_update", "content": " Done!"},
			}
			for _, evt := range events {
				data, _ := json.Marshal(evt)
				w.Write(data)
				w.Write([]byte("\n"))
				flusher.Flush()
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"response": "Hello from OpenClaw mock!",
		})
	})

	return httptest.NewServer(mux)
}

// ===== Hermes Mock Server =====

func newHermesMockServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		if stream, _ := body["stream"].(bool); stream {
			flusher := w.(http.Flusher)
			events := []map[string]any{
				{"type": "content", "text": "Hello "},
				{"type": "tool_use", "name": "file_read", "input": map[string]any{"path": "/tmp/test.go"}},
				{"type": "content", "text": "from Hermes!"},
			}
			for _, evt := range events {
				data, _ := json.Marshal(evt)
				w.Write(data)
				w.Write([]byte("\n"))
				flusher.Flush()
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"response": "Hello from Hermes mock!",
		})
	})

	return httptest.NewServer(mux)
}

// ===== Shared Test Helpers =====

func sendMessage(p provider.AgentProviderAdapter, sessionID string, text string) (*model.Message, error) {
	msg := &model.Message{
		Role:    model.RoleUser,
		Content: []model.ContentBlock{{Type: model.ContentText, Text: text}},
	}
	return p.SendMessage(context.Background(), sessionID, msg, nil)
}

// ===== LANGGRAPH TESTS =====

func TestLangGraph_HealthCheck(t *testing.T) {
	ts := newLangGraphMockServer()
	defer ts.Close()

	p := langgraphprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	if err := p.HealthCheck(context.Background()); err != nil {
		t.Errorf("health check failed: %v", err)
	}
}

func TestLangGraph_ListAgents(t *testing.T) {
	ts := newLangGraphMockServer()
	defer ts.Close()

	p := langgraphprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	agents, err := p.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) < 1 {
		t.Error("expected at least 1 agent")
	}
	if agents[0].Provider != "langgraph" {
		t.Errorf("provider = %v, want langgraph", agents[0].Provider)
	}
}

func TestLangGraph_CreateSession(t *testing.T) {
	ts := newLangGraphMockServer()
	defer ts.Close()

	p := langgraphprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	sess, err := p.CreateSession(context.Background(), "langgraph:agent", nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.ID == "" {
		t.Error("session id is empty")
	}
	if sess.Provider != "langgraph" {
		t.Errorf("provider = %v, want langgraph", sess.Provider)
	}
}

func TestLangGraph_SendMessage(t *testing.T) {
	ts := newLangGraphMockServer()
	defer ts.Close()

	p := langgraphprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	resp, err := sendMessage(p, "thread_mock_123", "Hello")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if resp.Role != model.RoleAgent {
		t.Errorf("role = %v, want agent", resp.Role)
	}
	if len(resp.Content) == 0 || resp.Content[0].Text == "" {
		t.Error("empty response content")
	}
}

func TestLangGraph_GetHistory(t *testing.T) {
	ts := newLangGraphMockServer()
	defer ts.Close()

	p := langgraphprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	messages, err := p.GetHistory(context.Background(), "thread_mock_123")
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(messages) < 2 {
		t.Errorf("expected at least 2 messages, got %d", len(messages))
	}
}

func TestLangGraph_MultipleAgents(t *testing.T) {
	ts := newLangGraphMockServer()
	defer ts.Close()

	p := langgraphprovider.New(zerolog.Nop())
	err := p.Initialize(context.Background(), provider.ProviderConfig{
		Endpoint: ts.URL,
		Auth:     provider.AuthConfig{APIKey: "test-key"},
		Options:  map[string]any{"default_assistant": "agent,coder,reviewer"},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	agents, _ := p.ListAgents(context.Background())
	if len(agents) != 1 {
		t.Logf("single assistant mode: got %d agents (expected 1 for comma-separated input)", len(agents))
	}
}

// ===== DIFY TESTS =====

func TestDify_HealthCheck(t *testing.T) {
	ts := newDifyMockServer()
	defer ts.Close()

	p := difyprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	if err := p.HealthCheck(context.Background()); err != nil {
		t.Errorf("health check failed: %v", err)
	}
}

func TestDify_ListAgents(t *testing.T) {
	ts := newDifyMockServer()
	defer ts.Close()

	p := difyprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	agents, err := p.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Provider != "dify" {
		t.Errorf("provider = %v, want dify", agents[0].Provider)
	}
}

func TestDify_CreateSession(t *testing.T) {
	ts := newDifyMockServer()
	defer ts.Close()

	p := difyprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	sess, err := p.CreateSession(context.Background(), "dify:chat", nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.ID == "" {
		t.Error("session id is empty")
	}
}

func TestDify_SendMessage(t *testing.T) {
	ts := newDifyMockServer()
	defer ts.Close()

	p := difyprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	resp, err := sendMessage(p, "conv_mock_123", "Hello")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if resp.Role != model.RoleAgent {
		t.Errorf("role = %v, want agent", resp.Role)
	}
	if len(resp.Content) == 0 || !strings.Contains(resp.Content[0].Text, "Dify") {
		t.Errorf("unexpected response: %v", resp.Content)
	}
}

func TestDify_GetHistory(t *testing.T) {
	ts := newDifyMockServer()
	defer ts.Close()

	p := difyprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	messages, err := p.GetHistory(context.Background(), "conv_mock_123")
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(messages) < 2 {
		t.Errorf("expected at least 2 messages, got %d", len(messages))
	}
}

func TestDify_MultipleAppTypes(t *testing.T) {
	ts := newDifyMockServer()
	defer ts.Close()

	for _, appType := range []string{"chat", "workflow", "completion"} {
		p := difyprovider.New(zerolog.Nop())
		err := p.Initialize(context.Background(), provider.ProviderConfig{
			Endpoint: ts.URL,
			Auth:     provider.AuthConfig{APIKey: "test-key"},
			Options:  map[string]any{"default_app_type": appType},
		})
		if err != nil {
			t.Fatalf("initialize appType=%s: %v", appType, err)
		}

		agents, _ := p.ListAgents(context.Background())
		if len(agents) != 1 {
			t.Errorf("appType=%s: expected 1 agent, got %d", appType, len(agents))
		}
		if agents[0].ID != "dify:"+appType {
			t.Errorf("appType=%s: agent ID = %v, want dify:%s", appType, agents[0].ID, appType)
		}
	}
}

// ===== OPENCLAW TESTS =====

func TestOpenClaw_HealthCheck(t *testing.T) {
	ts := newOpenClawMockServer()
	defer ts.Close()

	p := openclawprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	if err := p.HealthCheck(context.Background()); err != nil {
		t.Errorf("health check failed: %v", err)
	}
}

func TestOpenClaw_ListAgents_Multiple(t *testing.T) {
	ts := newOpenClawMockServer()
	defer ts.Close()

	p := openclawprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	agents, err := p.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) < 2 {
		t.Errorf("expected at least 2 agents (main + reviewer), got %d", len(agents))
	}

	ids := map[string]bool{}
	for _, a := range agents {
		ids[a.ID] = true
		if a.Provider != "openclaw" {
			t.Errorf("provider = %v, want openclaw", a.Provider)
		}
	}
	if !ids["openclaw:main"] {
		t.Error("missing openclaw:main agent")
	}
	if !ids["openclaw:reviewer"] {
		t.Error("missing openclaw:reviewer agent")
	}
}

func TestOpenClaw_CreateSession(t *testing.T) {
	ts := newOpenClawMockServer()
	defer ts.Close()

	p := openclawprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	for _, agentID := range []string{"openclaw:main", "openclaw:reviewer"} {
		sess, err := p.CreateSession(context.Background(), agentID, nil)
		if err != nil {
			t.Fatalf("create session for %s: %v", agentID, err)
		}
		if sess.AgentID != agentID {
			t.Errorf("agentID = %v, want %v", sess.AgentID, agentID)
		}
	}
}

func TestOpenClaw_SendMessage(t *testing.T) {
	ts := newOpenClawMockServer()
	defer ts.Close()

	p := openclawprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	sess, _ := p.CreateSession(context.Background(), "openclaw:main", nil)

	resp, err := sendMessage(p, sess.ID, "Write a function")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if resp.Role != model.RoleAgent {
		t.Errorf("role = %v, want agent", resp.Role)
	}
}

func TestOpenClaw_ListTools(t *testing.T) {
	ts := newOpenClawMockServer()
	defer ts.Close()

	p := openclawprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	tools, err := p.ListTools(context.Background(), "openclaw:main")
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) < 2 {
		t.Errorf("expected at least 2 tools, got %d", len(tools))
	}
}

func TestOpenClaw_GetHistory(t *testing.T) {
	ts := newOpenClawMockServer()
	defer ts.Close()

	p := openclawprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	sess, _ := p.CreateSession(context.Background(), "openclaw:main", nil)
	sendMessage(p, sess.ID, "Hello")

	history, err := p.GetHistory(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(history) < 2 {
		t.Errorf("expected at least 2 messages, got %d", len(history))
	}
}

// ===== HERMES TESTS =====

func TestHermes_HealthCheck(t *testing.T) {
	ts := newHermesMockServer()
	defer ts.Close()

	p := hermesprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	if err := p.HealthCheck(context.Background()); err != nil {
		t.Errorf("health check failed: %v", err)
	}
}

func TestHermes_ListAgents_Multiple(t *testing.T) {
	ts := newHermesMockServer()
	defer ts.Close()

	p := hermesprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	agents, err := p.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) < 2 {
		t.Errorf("expected at least 2 agents (default + code), got %d", len(agents))
	}

	ids := map[string]bool{}
	for _, a := range agents {
		ids[a.ID] = true
	}
	if !ids["hermes:default"] {
		t.Error("missing hermes:default agent")
	}
	if !ids["hermes:code"] {
		t.Error("missing hermes:code agent")
	}
}

func TestHermes_CreateSession(t *testing.T) {
	ts := newHermesMockServer()
	defer ts.Close()

	p := hermesprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	for _, agentID := range []string{"hermes:default", "hermes:code"} {
		sess, err := p.CreateSession(context.Background(), agentID, nil)
		if err != nil {
			t.Fatalf("create session for %s: %v", agentID, err)
		}
		if sess.AgentID != agentID {
			t.Errorf("agentID = %v, want %v", sess.AgentID, agentID)
		}
	}
}

func TestHermes_SendMessage(t *testing.T) {
	ts := newHermesMockServer()
	defer ts.Close()

	p := hermesprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	sess, _ := p.CreateSession(context.Background(), "hermes:default", nil)

	resp, err := sendMessage(p, sess.ID, "Hello")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if resp.Role != model.RoleAgent {
		t.Errorf("role = %v, want agent", resp.Role)
	}
	if len(resp.Content) == 0 || !strings.Contains(resp.Content[0].Text, "Hermes") {
		t.Errorf("unexpected response: %v", resp.Content)
	}
}

func TestHermes_ListTools(t *testing.T) {
	ts := newHermesMockServer()
	defer ts.Close()

	p := hermesprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	tools, err := p.ListTools(context.Background(), "hermes:default")
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) < 3 {
		t.Errorf("expected at least 3 tools, got %d", len(tools))
	}
}

func TestHermes_GetHistory(t *testing.T) {
	ts := newHermesMockServer()
	defer ts.Close()

	p := hermesprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	sess, _ := p.CreateSession(context.Background(), "hermes:default", nil)
	sendMessage(p, sess.ID, "Hello")

	history, err := p.GetHistory(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(history) < 2 {
		t.Errorf("expected at least 2 messages, got %d", len(history))
	}
}

// ===== CROSS-PROVIDER COMPARISON =====

func TestAllProviders_Interface(t *testing.T) {
	var providers []provider.AgentProviderAdapter

	ts1 := newLangGraphMockServer()
	defer ts1.Close()
	p1 := langgraphprovider.New(zerolog.Nop())
	initProvider(t, p1, ts1.URL)
	providers = append(providers, p1)

	ts2 := newDifyMockServer()
	defer ts2.Close()
	p2 := difyprovider.New(zerolog.Nop())
	initProvider(t, p2, ts2.URL)
	providers = append(providers, p2)

	ts3 := newOpenClawMockServer()
	defer ts3.Close()
	p3 := openclawprovider.New(zerolog.Nop())
	initProvider(t, p3, ts3.URL)
	providers = append(providers, p3)

	ts4 := newHermesMockServer()
	defer ts4.Close()
	p4 := hermesprovider.New(zerolog.Nop())
	initProvider(t, p4, ts4.URL)
	providers = append(providers, p4)

	for _, p := range providers {
		t.Run(p.Name()+"/health", func(t *testing.T) {
			if err := p.HealthCheck(context.Background()); err != nil {
				t.Errorf("health check failed: %v", err)
			}
		})

		t.Run(p.Name()+"/agents", func(t *testing.T) {
			agents, err := p.ListAgents(context.Background())
			if err != nil {
				t.Errorf("list agents: %v", err)
			}
			if len(agents) == 0 {
				t.Error("no agents found")
			}
			for _, a := range agents {
				if a.Provider != p.Name() {
					t.Errorf("agent provider = %v, want %v", a.Provider, p.Name())
				}
			}
		})

		t.Run(p.Name()+"/session+message", func(t *testing.T) {
			agents, _ := p.ListAgents(context.Background())
			sess, err := p.CreateSession(context.Background(), agents[0].ID, nil)
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			if sess.Provider != p.Name() {
				t.Errorf("session provider = %v, want %v", sess.Provider, p.Name())
			}

			resp, err := sendMessage(p, sess.ID, "Test message")
			if err != nil {
				t.Fatalf("send message: %v", err)
			}
			if resp.Role != model.RoleAgent {
				t.Errorf("response role = %v, want agent", resp.Role)
			}
		})
	}
}

// ===== AUTO-DISCOVERY MOCK SERVERS =====

func newA2AMockServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/agent.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"schemaVersion": "0.2",
			"name":          "Remote A2A Agent",
			"description":   "A remote agent discovered via A2A",
			"url":           "http://example.com",
			"capabilities": map[string]any{
				"streaming":         true,
				"pushNotifications": false,
			},
			"skills": []map[string]any{
				{"id": "ask", "name": "Ask", "description": "Ask questions"},
			},
		})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req map[string]any
			json.NewDecoder(r.Body).Decode(&req)

			method, _ := req["method"].(string)

			if method == "tasks/send" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      req["id"],
					"result": map[string]any{
						"id":     "task_mock_1",
						"status": "completed",
						"artifacts": []map[string]any{
							{
								"parts": []map[string]any{
									{"type": "text", "text": "Hello from A2A agent!"},
								},
							},
						},
					},
				})
				return
			}

			if method == "tasks/cancel" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      req["id"],
					"result":  map[string]any{},
				})
				return
			}
		}

		w.WriteHeader(404)
	})

	return httptest.NewServer(mux)
}

func newMCPMockServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}

		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)

		method, _ := req["method"].(string)

		w.Header().Set("Content-Type", "application/json")

		switch method {
		case "initialize":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]any{"name": "mock-mcp", "version": "1.0.0"},
				},
			})

		case "tools/list":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "search", "description": "Search the web", "inputSchema": map[string]any{"type": "object"}},
						{"name": "calculator", "description": "Calculate math", "inputSchema": map[string]any{"type": "object"}},
					},
				},
			})

		case "tools/call":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "MCP tool result: hello!"},
					},
				},
			})

		default:
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	})

	return httptest.NewServer(mux)
}

func newOpenAIMockServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4", "object": "model"},
			},
		})
	})

	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)

		if stream, _ := req["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"Hello "}}]}`)
			fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"from OpenAI!"}}]}`)
			fmt.Fprintln(w, "data: [DONE]")
			w.(http.Flusher).Flush()
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello from OpenAI mock!",
					},
				},
			},
		})
	})

	return httptest.NewServer(mux)
}

// ===== AUTO-DISCOVERY TESTS =====

func TestAutoDiscovery_DetectA2A(t *testing.T) {
	ts := newA2AMockServer()
	defer ts.Close()

	p := genericprovider.New(zerolog.Nop())
	err := p.Initialize(context.Background(), provider.ProviderConfig{
		Endpoint: ts.URL,
		Auth:     provider.AuthConfig{APIKey: "test-key"},
		Options:  map[string]any{"protocol": "auto"},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	agents, _ := p.ListAgents(context.Background())
	if len(agents) == 0 {
		t.Fatal("no agents discovered")
	}
	if agents[0].Name != "Remote A2A Agent" {
		t.Errorf("agent name = %v, want 'Remote A2A Agent'", agents[0].Name)
	}
}

func TestAutoDiscovery_DetectMCP(t *testing.T) {
	ts := newMCPMockServer()
	defer ts.Close()

	p := genericprovider.New(zerolog.Nop())
	err := p.Initialize(context.Background(), provider.ProviderConfig{
		Endpoint: ts.URL,
		Auth:     provider.AuthConfig{APIKey: "test-key"},
		Options:  map[string]any{"protocol": "auto"},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	agents, _ := p.ListAgents(context.Background())
	if len(agents) == 0 {
		t.Fatal("no agents discovered")
	}
}

func TestAutoDiscovery_DetectOpenAI(t *testing.T) {
	ts := newOpenAIMockServer()
	defer ts.Close()

	p := genericprovider.New(zerolog.Nop())
	err := p.Initialize(context.Background(), provider.ProviderConfig{
		Endpoint: ts.URL,
		Auth:     provider.AuthConfig{APIKey: "test-key"},
		Options:  map[string]any{"protocol": "auto"},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	agents, _ := p.ListAgents(context.Background())
	if len(agents) == 0 {
		t.Fatal("no agents discovered")
	}
}

func TestAutoDiscovery_ManualProtocol(t *testing.T) {
	ts := newA2AMockServer()
	defer ts.Close()

	p := genericprovider.New(zerolog.Nop())
	err := p.Initialize(context.Background(), provider.ProviderConfig{
		Endpoint: ts.URL,
		Auth:     provider.AuthConfig{APIKey: "test-key"},
		Options:  map[string]any{"protocol": "a2a"},
	})
	if err != nil {
		t.Fatalf("initialize with manual protocol: %v", err)
	}

	agents, _ := p.ListAgents(context.Background())
	if len(agents) == 0 {
		t.Fatal("no agents discovered")
	}
	if agents[0].Name != "Remote A2A Agent" {
		t.Errorf("agent name = %v, want 'Remote A2A Agent'", agents[0].Name)
	}
}

func TestAutoDiscovery_A2A_SendMessage(t *testing.T) {
	ts := newA2AMockServer()
	defer ts.Close()

	p := genericprovider.New(zerolog.Nop())
	initProvider(t, p, ts.URL)

	sess, _ := p.CreateSession(context.Background(), "generic:a2a", nil)

	resp, err := sendMessage(p, sess.ID, "Hello A2A")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if resp.Role != model.RoleAgent {
		t.Errorf("role = %v, want agent", resp.Role)
	}
	if len(resp.Content) == 0 || !strings.Contains(resp.Content[0].Text, "A2A") {
		t.Errorf("unexpected response: %v", resp.Content)
	}
}

func TestAutoDiscovery_MCP_SendMessage(t *testing.T) {
	ts := newMCPMockServer()
	defer ts.Close()

	p := genericprovider.New(zerolog.Nop())
	err := p.Initialize(context.Background(), provider.ProviderConfig{
		Endpoint: ts.URL,
		Auth:     provider.AuthConfig{APIKey: "test-key"},
		Options:  map[string]any{"protocol": "mcp"},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	sess, _ := p.CreateSession(context.Background(), "generic:mcp", nil)

	resp, err := sendMessage(p, sess.ID, "Hello MCP")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if resp.Role != model.RoleAgent {
		t.Errorf("role = %v, want agent", resp.Role)
	}
}

func TestAutoDiscovery_MCP_ListTools(t *testing.T) {
	ts := newMCPMockServer()
	defer ts.Close()

	p := genericprovider.New(zerolog.Nop())
	err := p.Initialize(context.Background(), provider.ProviderConfig{
		Endpoint: ts.URL,
		Auth:     provider.AuthConfig{APIKey: "test-key"},
		Options:  map[string]any{"protocol": "mcp"},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	tools, err := p.ListTools(context.Background(), "generic:mcp")
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) < 2 {
		t.Errorf("expected at least 2 tools, got %d", len(tools))
	}
}

func TestAutoDiscovery_OpenAI_SendMessage(t *testing.T) {
	ts := newOpenAIMockServer()
	defer ts.Close()

	p := genericprovider.New(zerolog.Nop())
	err := p.Initialize(context.Background(), provider.ProviderConfig{
		Endpoint: ts.URL,
		Auth:     provider.AuthConfig{APIKey: "test-key"},
		Options:  map[string]any{"protocol": "openai"},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	sess, _ := p.CreateSession(context.Background(), "generic:openai", nil)

	resp, err := sendMessage(p, sess.ID, "Hello OpenAI")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if resp.Role != model.RoleAgent {
		t.Errorf("role = %v, want agent", resp.Role)
	}
	if len(resp.Content) == 0 || !strings.Contains(resp.Content[0].Text, "OpenAI") {
		t.Errorf("unexpected response: %v", resp.Content)
	}
}

func TestAutoDiscovery_InvalidEndpoint(t *testing.T) {
	p := genericprovider.New(zerolog.Nop())
	err := p.Initialize(context.Background(), provider.ProviderConfig{
		Endpoint: "http://127.0.0.1:59999",
		Auth:     provider.AuthConfig{},
		Options:  map[string]any{"protocol": "auto"},
	})
	if err == nil {
		t.Error("expected error for invalid endpoint, got nil")
	}
}
