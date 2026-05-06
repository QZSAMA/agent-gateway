package model

import "time"

type Message struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId"`
	Role      MessageRole    `json:"role"`
	Content   []ContentBlock `json:"content"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type MessageRole string

const (
	RoleUser   MessageRole = "user"
	RoleAgent  MessageRole = "agent"
	RoleSystem MessageRole = "system"
)

type ContentBlockType string

const (
	ContentText  ContentBlockType = "text"
	ContentImage ContentBlockType = "image"
	ContentAudio ContentBlockType = "audio"
	ContentFile  ContentBlockType = "file"
	ContentData  ContentBlockType = "data"
)

type ContentBlock struct {
	Type     ContentBlockType `json:"type"`
	Text     string           `json:"text,omitempty"`
	Data     string           `json:"data,omitempty"`
	URL      string           `json:"url,omitempty"`
	MimeType string           `json:"mimeType,omitempty"`
	Name     string           `json:"name,omitempty"`
	JSONData map[string]any   `json:"jsonData,omitempty"`
}
