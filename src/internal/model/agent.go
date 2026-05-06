package model

type AgentDescriptor struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Provider     string              `json:"provider"`
	Capabilities AgentCapabilities   `json:"capabilities"`
	Skills       []AgentSkill        `json:"skills,omitempty"`
	AuthSchemes  []map[string]any    `json:"authSchemes,omitempty"`
	Metadata     map[string]any      `json:"metadata,omitempty"`
}

type AgentCapabilities struct {
	Streaming         bool `json:"streaming"`
	ToolUse           bool `json:"toolUse"`
	HumanInTheLoop    bool `json:"humanInTheLoop"`
	MultiModal        bool `json:"multiModal"`
	LongRunningTasks  bool `json:"longRunningTasks"`
	PushNotifications bool `json:"pushNotifications"`
	Memory            bool `json:"memory"`
}

type AgentSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type AgentCard struct {
	SchemaVersion     string               `json:"schemaVersion"`
	Name              string               `json:"name"`
	URL               string               `json:"url"`
	Description       string               `json:"description"`
	Provider          AgentProviderCard    `json:"provider"`
	Version           string               `json:"version"`
	Capabilities      AgentCardCapabilities `json:"capabilities"`
	AuthenticationSchemes []map[string]any `json:"authenticationSchemes,omitempty"`
	Skills            []AgentCardSkill     `json:"skills"`
}

type AgentProviderCard struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

type AgentCardCapabilities struct {
	Streaming         bool `json:"streaming"`
	PushNotifications bool `json:"pushNotifications"`
	StateTransition   bool `json:"stateTransition"`
}

type AgentCardSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
}
