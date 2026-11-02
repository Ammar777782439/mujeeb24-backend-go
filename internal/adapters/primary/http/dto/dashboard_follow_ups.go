package dto

import "time"

// Only authorized, stored facts are projected. Missing context remains absent.
type DashboardFollowUp struct {
	LeadID                UUID       `json:"lead_id"`
	CustomerID            UUID       `json:"customer_id"`
	CustomerName          string     `json:"customer_name"`
	LeadStatus            string     `json:"lead_status" enum:"new,interested,qualified"`
	ConversationID        *UUID      `json:"conversation_id,omitempty"`
	Channel               *string    `json:"channel,omitempty"`
	AssigneeName          *string    `json:"assignee_name,omitempty"`
	ProductName           *string    `json:"product_name,omitempty"`
	InterestReason        *string    `json:"interest_reason,omitempty"`
	LastCustomerMessage   *string    `json:"last_customer_message,omitempty"`
	LastCustomerMessageAt *time.Time `json:"last_customer_message_at,omitempty"`
	NextActionAt          *time.Time `json:"next_action_at,omitempty"`
	WorkflowID            *UUID      `json:"workflow_id,omitempty"`
	Attention             string     `json:"attention" enum:"waiting_human,awaiting_review,follow_up_due,unassigned,new,interested,qualified"`
	UpdatedAt             time.Time  `json:"updated_at"`
}
type DashboardFollowUps struct {
	Items   []DashboardFollowUp `json:"items"`
	HasMore bool                `json:"has_more"`
	AsOf    time.Time           `json:"as_of"`
}
