package model

import "time"

type Task struct {
	ID        string      `json:"id"`
	SessionID string      `json:"sessionId"`
	AgentID   string      `json:"agentId"`
	Status    TaskStatus  `json:"status"`
	Input     *Message    `json:"input,omitempty"`
	Output    *Message    `json:"output,omitempty"`
	Artifacts []Artifact  `json:"artifacts,omitempty"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

type TaskStatus string

const (
	TaskSubmitted  TaskStatus = "submitted"
	TaskWorking    TaskStatus = "working"
	TaskCompleted  TaskStatus = "completed"
	TaskFailed     TaskStatus = "failed"
	TaskCancelled  TaskStatus = "cancelled"
	TaskInterrupted TaskStatus = "interrupted"
)

type Artifact struct {
	ID       string         `json:"id"`
	Name     string         `json:"name,omitempty"`
	Content  []ContentBlock `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
