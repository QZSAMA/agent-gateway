package gateway

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/agent-gateway/gateway/internal/config"
	"github.com/agent-gateway/gateway/internal/model"
	"github.com/agent-gateway/gateway/internal/provider"
	"github.com/agent-gateway/gateway/internal/store"
)

type Gateway struct {
	cfg       *config.Config
	store     *store.Store
	providers map[string]provider.AgentProviderAdapter
	registry  *AgentRegistry
	logger    zerolog.Logger
	mu        sync.RWMutex
}

func New(cfg *config.Config, s *store.Store, logger zerolog.Logger) *Gateway {
	return &Gateway{
		cfg:       cfg,
		store:     s,
		providers: make(map[string]provider.AgentProviderAdapter),
		registry:  NewAgentRegistry(),
		logger:    logger,
	}
}

func (g *Gateway) RegisterProvider(p provider.AgentProviderAdapter, cfg provider.ProviderConfig) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	ctx := context.Background()
	if err := p.Initialize(ctx, cfg); err != nil {
		return fmt.Errorf("initialize provider %s: %w", p.Name(), err)
	}

	g.providers[p.Name()] = p

	agents, err := p.ListAgents(ctx)
	if err != nil {
		return fmt.Errorf("list agents from %s: %w", p.Name(), err)
	}

	for i := range agents {
		g.registry.Register(&agents[i])
		g.logger.Info().
			Str("agent_id", agents[i].ID).
			Str("agent_name", agents[i].Name).
			Str("provider", agents[i].Provider).
			Msg("registered agent")
	}

	g.logger.Info().Str("provider", p.Name()).Int("agents", len(agents)).Msg("provider registered")
	return nil
}

func (g *Gateway) Shutdown(ctx context.Context) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var firstErr error
	for name, p := range g.providers {
		if err := p.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("shutdown provider %s: %w", name, err)
		}
		g.logger.Info().Str("provider", name).Msg("provider shut down")
	}

	if g.store != nil {
		g.store.Close()
	}

	return firstErr
}

func (g *Gateway) ListAgents(ctx context.Context) []model.AgentDescriptor {
	return g.registry.ListAll()
}

func (g *Gateway) GetAgent(ctx context.Context, agentID string) (*model.AgentDescriptor, error) {
	a := g.registry.Get(agentID)
	if a == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	return a, nil
}

func (g *Gateway) GetAgentCard(ctx context.Context, agentID string) (*model.AgentCard, error) {
	a := g.registry.Get(agentID)
	if a == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	card := &model.AgentCard{
		SchemaVersion: "0.2",
		Name:          a.Name,
		URL:           fmt.Sprintf("http://%s/v1/agents/%s", g.cfg.Addr(), a.ID),
		Description:   a.Description,
		Provider: model.AgentProviderCard{
			Organization: a.Provider,
		},
		Version: "1.0.0",
		Capabilities: model.AgentCardCapabilities{
			Streaming:         a.Capabilities.Streaming,
			PushNotifications: a.Capabilities.PushNotifications,
			StateTransition:   true,
		},
	}
	for _, s := range a.Skills {
		card.Skills = append(card.Skills, model.AgentCardSkill{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
		})
	}
	return card, nil
}

func (g *Gateway) CreateSession(ctx context.Context, agentID string, opts *model.SessionOptions) (*model.Session, error) {
	p, agentIDWithoutProvider, err := g.resolveProvider(agentID)
	if err != nil {
		return nil, err
	}

	sess, err := p.CreateSession(ctx, agentIDWithoutProvider, opts)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	sess.AgentID = agentID
	sess.Provider = p.Name()

	if err := g.store.SaveSession(sess); err != nil {
		g.logger.Error().Err(err).Str("session_id", sess.ID).Msg("failed to save session")
	}

	return sess, nil
}

func (g *Gateway) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	sess, err := g.store.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if sess == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return sess, nil
}

func (g *Gateway) ListSessions(ctx context.Context, agentID string) ([]model.Session, error) {
	return g.store.ListSessions(agentID)
}

func (g *Gateway) CloseSession(ctx context.Context, sessionID string) error {
	sess, err := g.store.GetSession(sessionID)
	if err != nil {
		return err
	}
	if sess == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	p, _, err := g.resolveProvider(sess.AgentID)
	if err != nil {
		return err
	}

	if err := p.CloseSession(ctx, sessionID); err != nil {
		return fmt.Errorf("close session: %w", err)
	}

	return g.store.UpdateSessionStatus(sessionID, model.SessionExpired)
}

func (g *Gateway) SendMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (*model.Message, error) {
	sess, err := g.store.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	p, _, err := g.resolveProvider(sess.AgentID)
	if err != nil {
		return nil, err
	}

	msg.SessionID = sessionID
	resp, err := p.SendMessage(ctx, sessionID, msg, opts)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	g.store.SaveMessage(msg)
	g.store.SaveMessage(resp)

	return resp, nil
}

func (g *Gateway) StreamMessage(ctx context.Context, sessionID string, msg *model.Message, opts *provider.SendOptions) (<-chan model.StreamEvent, error) {
	sess, err := g.store.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	p, _, err := g.resolveProvider(sess.AgentID)
	if err != nil {
		return nil, err
	}

	msg.SessionID = sessionID
	g.store.SaveMessage(msg)

	return p.StreamMessage(ctx, sessionID, msg, opts)
}

func (g *Gateway) Cancel(ctx context.Context, sessionID string) error {
	sess, err := g.store.GetSession(sessionID)
	if err != nil {
		return err
	}
	if sess == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	p, _, err := g.resolveProvider(sess.AgentID)
	if err != nil {
		return err
	}

	return p.Cancel(ctx, sessionID)
}

func (g *Gateway) ListTools(ctx context.Context, agentID string) ([]model.ToolDefinition, error) {
	p, agentIDWithoutProvider, err := g.resolveProvider(agentID)
	if err != nil {
		return nil, err
	}
	return p.ListTools(ctx, agentIDWithoutProvider)
}

func (g *Gateway) InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error) {
	p, agentIDWithoutProvider, err := g.resolveProvider(agentID)
	if err != nil {
		return nil, err
	}
	return p.InvokeTool(ctx, agentIDWithoutProvider, toolName, input)
}

func (g *Gateway) RespondApproval(ctx context.Context, approvalID string, resp *model.ApprovalResponse) error {
	for _, p := range g.providers {
		if err := p.RespondApproval(ctx, approvalID, resp); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no provider handled approval: %s", approvalID)
}

func (g *Gateway) ResumeTask(ctx context.Context, sessionID string, resumeData any) (*model.Message, error) {
	sess, err := g.store.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	p, _, err := g.resolveProvider(sess.AgentID)
	if err != nil {
		return nil, err
	}

	return p.ResumeTask(ctx, sessionID, resumeData)
}

func (g *Gateway) GetHistory(ctx context.Context, sessionID string) ([]model.Message, error) {
	return g.store.GetHistory(sessionID)
}

func (g *Gateway) HealthCheck(ctx context.Context) map[string]error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	results := make(map[string]error)
	for name, p := range g.providers {
		results[name] = p.HealthCheck(ctx)
	}
	return results
}

func (g *Gateway) StartHealthChecks(ctx context.Context) {
	interval := g.cfg.Health.CheckInterval
	if interval == 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				results := g.HealthCheck(ctx)
				for name, err := range results {
					if err != nil {
						g.logger.Warn().Str("provider", name).Err(err).Msg("health check failed")
					} else {
						g.logger.Debug().Str("provider", name).Msg("health check passed")
					}
				}
			}
		}
	}()
}

func (g *Gateway) resolveProvider(agentID string) (provider.AgentProviderAdapter, string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	providerName, localID := parseAgentID(agentID)
	if providerName != "" {
		p, ok := g.providers[providerName]
		if !ok {
			return nil, "", fmt.Errorf("provider not found: %s", providerName)
		}
		return p, localID, nil
	}

	for _, p := range g.providers {
		if _, err := p.GetAgent(context.Background(), agentID); err == nil {
			return p, agentID, nil
		}
	}

	return nil, "", fmt.Errorf("no provider found for agent: %s", agentID)
}

func parseAgentID(agentID string) (providerName, localID string) {
	for i := 0; i < len(agentID); i++ {
		if agentID[i] == ':' {
			return agentID[:i], agentID[i+1:]
		}
	}
	return "", agentID
}
