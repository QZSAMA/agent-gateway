package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/agent-gateway/gateway/internal/config"
	"github.com/agent-gateway/gateway/internal/gateway"
	"github.com/agent-gateway/gateway/internal/provider"
	"github.com/agent-gateway/gateway/internal/server"
	"github.com/agent-gateway/gateway/internal/store"
)

func testConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
			WebSocket: config.WebSocketConfig{
				Enabled: true,
				Path:    "/ws",
			},
			CORS: config.CORSConfig{
				AllowedOrigins: []string{"*"},
			},
		},
		Store: config.StoreConfig{
			Type: "sqlite",
			SQLite: config.SQLiteConfig{
				Path: "./test_gateway.db",
			},
		},
		Providers: map[string]config.ProviderEntry{},
		Logging: config.LoggingConfig{
			Level:  "warn",
			Format: "json",
		},
		Health: config.HealthConfig{
			CheckInterval:      30 * time.Second,
			UnhealthyThreshold: 3,
		},
	}
}

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	cfg := testConfig()
	logger := zerolog.New(zerolog.TestWriter{T: t}).With().Timestamp().Logger()

	s, err := store.New(cfg.Store.SQLite.Path)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	t.Cleanup(func() {
		s.Close()
		os.Remove(cfg.Store.SQLite.Path)
	})

	gw := gateway.New(cfg, s, logger)

	mock := NewMockProvider()
	if err := gw.RegisterProvider(mock, provider.ProviderConfig{
		Endpoint: "mock://test",
	}); err != nil {
		t.Fatalf("failed to register mock provider: %v", err)
	}

	srv := server.New(cfg, gw, logger)
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(func() {
		ts.Close()
	})

	return ts
}

func getJSON(t *testing.T, ts *httptest.Server, path string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return resp, result
}

func postJSON(t *testing.T, ts *httptest.Server, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return resp, result
}

func doDelete(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("DELETE", ts.URL+path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

func TestHealth(t *testing.T) {
	ts := setupTestServer(t)

	resp, result := getJSON(t, ts, "/v1/health")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if healthy, ok := result["healthy"].(bool); !ok || !healthy {
		t.Errorf("healthy = %v, want true", result["healthy"])
	}
}

func TestListAgents_WithMock(t *testing.T) {
	ts := setupTestServer(t)

	resp, result := getJSON(t, ts, "/v1/agents")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	count, _ := result["count"].(float64)
	if count != 1 {
		t.Errorf("count = %v, want 1", count)
	}
}

func TestGetAgent_Found(t *testing.T) {
	ts := setupTestServer(t)

	resp, result := getJSON(t, ts, "/v1/agents/test:echo")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if name, _ := result["name"].(string); name != "Echo Agent" {
		t.Errorf("name = %v, want Echo Agent", name)
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	ts := setupTestServer(t)

	resp, _ := http.Get(ts.URL + "/v1/agents/nonexistent")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestAgentCard(t *testing.T) {
	ts := setupTestServer(t)

	resp, result := getJSON(t, ts, "/v1/agents/test:echo/card")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if name, _ := result["name"].(string); name != "Echo Agent" {
		t.Errorf("card name = %v, want Echo Agent", name)
	}
}

func TestSessions_CRUD(t *testing.T) {
	ts := setupTestServer(t)

	resp, result := postJSON(t, ts, "/v1/sessions", map[string]any{
		"agentId": "test:echo",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := json.Marshal(result)
		t.Fatalf("create session status = %d, body = %s", resp.StatusCode, string(body))
	}

	sessionID, _ := result["id"].(string)
	if sessionID == "" {
		t.Fatal("session id is empty")
	}
	if result["provider"] != "test" {
		t.Errorf("provider = %v, want test", result["provider"])
	}

	resp2, _ := getJSON(t, ts, "/v1/sessions/"+sessionID)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("get session status = %d, want %d", resp2.StatusCode, http.StatusOK)
	}

	resp3, result3 := getJSON(t, ts, "/v1/sessions")
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK {
		t.Errorf("list sessions status = %d, want %d", resp3.StatusCode, http.StatusOK)
	}
	sessions, _ := result3["sessions"].([]any)
	if len(sessions) < 1 {
		t.Error("expected at least 1 session")
	}

	resp4 := doDelete(t, ts, "/v1/sessions/"+sessionID)
	defer resp4.Body.Close()

	if resp4.StatusCode != http.StatusOK {
		t.Errorf("close session status = %d, want %d", resp4.StatusCode, http.StatusOK)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	ts := setupTestServer(t)

	resp, _ := http.Get(ts.URL + "/v1/sessions/nonexistent")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestCreateSession_InvalidBody(t *testing.T) {
	ts := setupTestServer(t)

	resp, _ := http.Post(ts.URL+"/v1/sessions", "text/plain", bytes.NewReader([]byte("invalid")))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestSendMessage(t *testing.T) {
	ts := setupTestServer(t)

	_, sess := postJSON(t, ts, "/v1/sessions", map[string]any{
		"agentId": "test:echo",
	})
	sessionID, _ := sess["id"].(string)

	resp, result := postJSON(t, ts, "/v1/sessions/"+sessionID+"/messages", map[string]any{
		"role":    "user",
		"content": []map[string]any{{"type": "text", "text": "Hello"}},
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := json.Marshal(result)
		t.Fatalf("send message status = %d, body = %s", resp.StatusCode, string(body))
	}
	if role, _ := result["role"].(string); role != "agent" {
		t.Errorf("response role = %v, want agent", role)
	}
}

func TestStreamMessage(t *testing.T) {
	ts := setupTestServer(t)

	_, sess := postJSON(t, ts, "/v1/sessions", map[string]any{
		"agentId": "test:echo",
	})
	sessionID, _ := sess["id"].(string)

	b, _ := json.Marshal(map[string]any{
		"role":    "user",
		"content": []map[string]any{{"type": "text", "text": "Hello"}},
	})
	resp, err := http.Post(ts.URL+"/v1/sessions/"+sessionID+"/stream", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("stream status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %v, want text/event-stream", ct)
	}
}

func TestGetHistory(t *testing.T) {
	ts := setupTestServer(t)

	_, sess := postJSON(t, ts, "/v1/sessions", map[string]any{
		"agentId": "test:echo",
	})
	sessionID, _ := sess["id"].(string)

	postJSON(t, ts, "/v1/sessions/"+sessionID+"/messages", map[string]any{
		"role":    "user",
		"content": []map[string]any{{"type": "text", "text": "Hello"}},
	})

	resp, result := getJSON(t, ts, "/v1/sessions/"+sessionID+"/history")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("history status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	messages, _ := result["messages"].([]any)
	if len(messages) < 1 {
		t.Error("expected at least 1 message in history")
	}
}

func TestListTools(t *testing.T) {
	ts := setupTestServer(t)

	resp, _ := getJSON(t, ts, "/v1/agents/test:echo/tools")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("tools status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestApprovals_Empty(t *testing.T) {
	ts := setupTestServer(t)

	resp, _ := getJSON(t, ts, "/v1/approvals/pending")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestTaskSend(t *testing.T) {
	ts := setupTestServer(t)

	resp, result := postJSON(t, ts, "/v1/tasks/send", map[string]any{
		"agentId": "test:echo",
		"message": map[string]any{
			"role":    "user",
			"content": []map[string]any{{"type": "text", "text": "hi"}},
		},
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := json.Marshal(result)
		t.Fatalf("task send status = %d, body = %s", resp.StatusCode, string(body))
	}
}

func TestTaskSend_NoProvider(t *testing.T) {
	ts := setupTestServer(t)

	resp, _ := postJSON(t, ts, "/v1/tasks/send", map[string]any{
		"agentId": "nonexistent",
		"message": map[string]any{
			"role":    "user",
			"content": []map[string]any{{"type": "text", "text": "hi"}},
		},
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("task send status = %d, want %d (mock provider accepts any agent)", resp.StatusCode, http.StatusOK)
	}
}

func TestCancel(t *testing.T) {
	ts := setupTestServer(t)

	_, sess := postJSON(t, ts, "/v1/sessions", map[string]any{
		"agentId": "test:echo",
	})
	sessionID, _ := sess["id"].(string)

	resp, _ := postJSON(t, ts, "/v1/sessions/"+sessionID+"/cancel", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("cancel status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
