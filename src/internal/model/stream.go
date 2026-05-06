package model

type StreamEventType string

const (
	EventMessageChunk   StreamEventType = "message_chunk"
	EventThoughtChunk   StreamEventType = "thought_chunk"
	EventToolCall       StreamEventType = "tool_call"
	EventToolCallUpdate StreamEventType = "tool_call_update"
	EventTaskStatus     StreamEventType = "task_status"
	EventArtifact       StreamEventType = "artifact"
	EventApprovalRequest StreamEventType = "approval_request"
	EventError          StreamEventType = "error"
)

type ToolKind string

const (
	ToolKindRead    ToolKind = "read"
	ToolKindEdit    ToolKind = "edit"
	ToolKindDelete  ToolKind = "delete"
	ToolKindMove    ToolKind = "move"
	ToolKindSearch  ToolKind = "search"
	ToolKindExecute ToolKind = "execute"
	ToolKindThink   ToolKind = "think"
	ToolKindFetch   ToolKind = "fetch"
	ToolKindOther   ToolKind = "other"
)

type StreamEvent struct {
	Type      StreamEventType `json:"type"`
	SessionID string          `json:"sessionId"`

	Role    MessageRole `json:"role,omitempty"`
	Content *ContentBlock `json:"content,omitempty"`

	ToolCallID   string         `json:"toolCallId,omitempty"`
	ToolName     string         `json:"toolName,omitempty"`
	ToolKind     ToolKind       `json:"toolKind,omitempty"`
	ToolInput    map[string]any `json:"toolInput,omitempty"`
	ToolStatus   string         `json:"toolStatus,omitempty"`

	TaskStatus TaskStatus `json:"taskStatus,omitempty"`

	Artifact *Artifact `json:"artifact,omitempty"`
	Append   bool      `json:"append,omitempty"`

	ApprovalID   string `json:"approvalId,omitempty"`
	ActionType   string `json:"actionType,omitempty"`
	Description  string `json:"description,omitempty"`
	RiskLevel    string `json:"riskLevel,omitempty"`

	ErrorMessage string `json:"errorMessage,omitempty"`
}

type DiffContent struct {
	OldPath string `json:"oldPath,omitempty"`
	NewPath string `json:"newPath,omitempty"`
	Patch   string `json:"patch"`
}
