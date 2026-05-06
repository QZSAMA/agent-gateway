package gateway

import (
	"sync"

	"github.com/agent-gateway/gateway/internal/model"
)

type AgentRegistry struct {
	agents map[string]*model.AgentDescriptor
	mu     sync.RWMutex
}

func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*model.AgentDescriptor),
	}
}

func (r *AgentRegistry) Register(agent *model.AgentDescriptor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[agent.ID] = agent
}

func (r *AgentRegistry) Unregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, agentID)
}

func (r *AgentRegistry) Get(agentID string) *model.AgentDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agents[agentID]
}

func (r *AgentRegistry) ListAll() []model.AgentDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]model.AgentDescriptor, 0, len(r.agents))
	for _, a := range r.agents {
		result = append(result, *a)
	}
	return result
}

func (r *AgentRegistry) ListByProvider(provider string) []model.AgentDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []model.AgentDescriptor
	for _, a := range r.agents {
		if a.Provider == provider {
			result = append(result, *a)
		}
	}
	return result
}

func (r *AgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}
