package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/agent-gateway/gateway/internal/config"
	"github.com/agent-gateway/gateway/internal/gateway"
	"github.com/agent-gateway/gateway/internal/model"
	"github.com/agent-gateway/gateway/internal/provider"
)

type Server struct {
	cfg     *config.Config
	gw      *gateway.Gateway
	logger  zerolog.Logger
	upgrader websocket.Upgrader
}

func New(cfg *config.Config, gw *gateway.Gateway, logger zerolog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		gw:     gw,
		logger: logger,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	if len(s.cfg.Server.CORS.AllowedOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins: s.cfg.Server.CORS.AllowedOrigins,
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
			MaxAge:         300,
		}))
	}

	r.Get("/v1/health", s.handleHealth)

	r.Route("/v1", func(r chi.Router) {
		r.Route("/agents", func(r chi.Router) {
			r.Get("/", s.handleListAgents)
			r.Get("/{agentId}", s.handleGetAgent)
			r.Get("/{agentId}/card", s.handleGetAgentCard)
			r.Get("/{agentId}/tools", s.handleListTools)
			r.Post("/{agentId}/tools/invoke", s.handleInvokeTool)
		})

		r.Route("/sessions", func(r chi.Router) {
			r.Post("/", s.handleCreateSession)
			r.Get("/", s.handleListSessions)
			r.Get("/{sessionId}", s.handleGetSession)
			r.Delete("/{sessionId}", s.handleCloseSession)
			r.Post("/{sessionId}/messages", s.handleSendMessage)
			r.Post("/{sessionId}/stream", s.handleStreamMessage)
			r.Get("/{sessionId}/history", s.handleGetHistory)
			r.Post("/{sessionId}/cancel", s.handleCancel)
			r.Post("/{sessionId}/resume", s.handleResumeTask)
		})

		r.Route("/tasks", func(r chi.Router) {
			r.Post("/send", s.handleTaskSend)
			r.Post("/sendSubscribe", s.handleTaskSendSubscribe)
			r.Get("/{taskId}", s.handleGetTask)
			r.Post("/{taskId}/cancel", s.handleCancelTask)
		})

		r.Route("/approvals", func(r chi.Router) {
			r.Get("/pending", s.handleListPendingApprovals)
			r.Post("/{approvalId}/respond", s.handleRespondApproval)
		})
	})

	if s.cfg.Server.WebSocket.Enabled {
		r.Get(s.cfg.Server.WebSocket.Path, s.handleWebSocket)
	}

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	results := s.gw.HealthCheck(r.Context())
	healthy := true
	for _, err := range results {
		if err != nil {
			healthy = false
			break
		}
	}

	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, map[string]any{
		"status":    "ok",
		"healthy":   healthy,
		"providers": results,
		"timestamp": time.Now(),
	})
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents := s.gw.ListAgents(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"agents": agents,
		"count":  len(agents),
	})
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	agent, err := s.gw.GetAgent(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (s *Server) handleGetAgentCard(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	card, err := s.gw.GetAgentCard(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, card)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string                `json:"agentId"`
		Options *model.SessionOptions `json:"options,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sess, err := s.gw.CreateSession(r.Context(), req.AgentID, req.Options)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, sess)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agentId")
	sessions, err := s.gw.ListSessions(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
	})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	sess, err := s.gw.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleCloseSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	if err := s.gw.CloseSession(r.Context(), sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "closed"})
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	var msg model.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := s.gw.SendMessage(r.Context(), sessionID, &msg, &provider.SendOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleStreamMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	var msg model.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	events, err := s.gw.StreamMessage(r.Context(), sessionID, &msg, &provider.SendOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	for evt := range events {
		data, _ := json.Marshal(evt)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
		flusher.Flush()
	}

	fmt.Fprintf(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
}

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	messages, err := s.gw.GetHistory(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"messages": messages,
	})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	if err := s.gw.Cancel(r.Context(), sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

func (s *Server) handleResumeTask(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	var req struct {
		ResumeData any `json:"resumeData"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := s.gw.ResumeTask(r.Context(), sessionID, req.ResumeData)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	tools, err := s.gw.ListTools(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tools": tools,
	})
}

func (s *Server) handleInvokeTool(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	var req struct {
		ToolName string         `json:"toolName"`
		Input    map[string]any `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := s.gw.InvokeTool(r.Context(), agentID, req.ToolName, req.Input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (s *Server) handleTaskSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID   string           `json:"agentId"`
		Message   *model.Message   `json:"message"`
		SessionID string           `json:"sessionId,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	var sess *model.Session
	var err error

	if req.SessionID != "" {
		sess, err = s.gw.GetSession(ctx, req.SessionID)
	} else {
		sess, err = s.gw.CreateSession(ctx, req.AgentID, nil)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp, err := s.gw.SendMessage(ctx, sess.ID, req.Message, &provider.SendOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	task := &model.Task{
		ID:        fmt.Sprintf("task_%d", time.Now().UnixMilli()),
		SessionID: sess.ID,
		AgentID:   req.AgentID,
		Status:    model.TaskCompleted,
		Input:     req.Message,
		Output:    resp,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"jsonrpc": "2.0",
		"result":  task,
	})
}

func (s *Server) handleTaskSendSubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID   string         `json:"agentId"`
		Message   *model.Message `json:"message"`
		SessionID string         `json:"sessionId,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	var sess *model.Session
	var err error

	if req.SessionID != "" {
		sess, err = s.gw.GetSession(ctx, req.SessionID)
	} else {
		sess, err = s.gw.CreateSession(ctx, req.AgentID, nil)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	events, err := s.gw.StreamMessage(ctx, sess.ID, req.Message, &provider.SendOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	for evt := range events {
		data, _ := json.Marshal(evt)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
		flusher.Flush()
	}

	fmt.Fprintf(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "task status polling not yet implemented")
}

func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskId")
	if err := s.gw.Cancel(r.Context(), taskID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

func (s *Server) handleListPendingApprovals(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"approvals": []any{}})
}

func (s *Server) handleRespondApproval(w http.ResponseWriter, r *http.Request) {
	approvalID := chi.URLParam(r, "approvalId")
	var resp model.ApprovalResponse
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp.ApprovalID = approvalID

	if err := s.gw.RespondApproval(r.Context(), approvalID, &resp); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "responded"})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error().Err(err).Msg("websocket upgrade failed")
		return
	}
	defer conn.Close()

	s.logger.Info().Str("remote", conn.RemoteAddr().String()).Msg("websocket client connected")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.logger.Error().Err(err).Msg("websocket read error")
			}
			break
		}

		var req struct {
			Type   string         `json:"type"`
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(message, &req); err != nil {
			s.writeWSResponse(conn, req.ID, nil, fmt.Errorf("invalid message format"))
			continue
		}

		s.handleWSMessage(conn, req.ID, req.Method, req.Params)
	}
}

func (s *Server) handleWSMessage(conn *websocket.Conn, id, method string, params map[string]any) {
	ctx := context.Background()

	switch method {
	case "session.create":
		agentID, _ := params["agentId"].(string)
		sess, err := s.gw.CreateSession(ctx, agentID, nil)
		if err != nil {
			s.writeWSResponse(conn, id, nil, err)
			return
		}
		s.writeWSResponse(conn, id, sess, nil)

	case "session.send":
		sessionID, _ := params["sessionId"].(string)
		msgData, _ := params["message"].(map[string]any)
		msgBytes, _ := json.Marshal(msgData)
		var msg model.Message
		json.Unmarshal(msgBytes, &msg)

		events, err := s.gw.StreamMessage(ctx, sessionID, &msg, &provider.SendOptions{})
		if err != nil {
			s.writeWSResponse(conn, id, nil, err)
			return
		}

		for evt := range events {
			s.writeWSEvent(conn, "stream."+string(evt.Type), evt)
		}

		s.writeWSResponse(conn, id, map[string]any{"status": "completed"}, nil)

	case "session.list":
		agentID, _ := params["agentId"].(string)
		sessions, err := s.gw.ListSessions(ctx, agentID)
		if err != nil {
			s.writeWSResponse(conn, id, nil, err)
			return
		}
		s.writeWSResponse(conn, id, sessions, nil)

	case "session.close":
		sessionID, _ := params["sessionId"].(string)
		err := s.gw.CloseSession(ctx, sessionID)
		s.writeWSResponse(conn, id, map[string]any{"status": "closed"}, err)

	case "approval.respond":
		approvalID, _ := params["approvalId"].(string)
		decision, _ := params["decision"].(string)
		resp := &model.ApprovalResponse{ApprovalID: approvalID, Decision: decision}
		err := s.gw.RespondApproval(ctx, approvalID, resp)
		s.writeWSResponse(conn, id, map[string]any{"status": "responded"}, err)

	default:
		s.writeWSResponse(conn, id, nil, fmt.Errorf("unknown method: %s", method))
	}
}

func (s *Server) writeWSResponse(conn *websocket.Conn, id string, result any, err error) {
	resp := map[string]any{
		"type": "res",
		"id":   id,
	}
	if err != nil {
		resp["ok"] = false
		resp["error"] = map[string]any{"message": err.Error()}
	} else {
		resp["ok"] = true
		resp["payload"] = result
	}
	conn.WriteJSON(resp)
}

func (s *Server) writeWSEvent(conn *websocket.Conn, event string, payload any) {
	conn.WriteJSON(map[string]any{
		"type":      "event",
		"event":     event,
		"payload":   payload,
		"timestamp": time.Now(),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": message,
		},
	})
}
