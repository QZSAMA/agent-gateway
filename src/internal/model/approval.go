package model

import "time"

type ApprovalRequest struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"sessionId"`
	AgentID     string    `json:"agentId"`
	ActionType  string    `json:"actionType"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	RiskLevel   string    `json:"riskLevel"`
	CreatedAt   time.Time `json:"createdAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

type ApprovalResponse struct {
	ApprovalID string `json:"approvalId"`
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
}

type ApprovalDecision string

const (
	ApprovalApproved ApprovalDecision = "approved"
	ApprovalDenied   ApprovalDecision = "denied"
)
