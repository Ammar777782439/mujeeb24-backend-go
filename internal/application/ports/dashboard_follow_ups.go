package ports

import (
	"context"
	"time"
)

// DashboardFollowUpRecord is a read projection of stored lead/conversation data.
// It does not infer intent or execute a sales capability.
type DashboardFollowUpRecord struct {
	LeadID                string
	CustomerID            string
	CustomerName          string
	LeadStatus            string
	ConversationID        *string
	Channel               *string
	AssigneeName          *string
	ProductName           *string
	InterestReason        *string
	LastCustomerMessage   *string
	LastCustomerMessageAt *time.Time
	NextActionAt          *time.Time
	WorkflowID            *string
	Attention             string
	UpdatedAt             time.Time
}

type DashboardFollowUpRepository interface {
	ListFollowUps(context.Context, string, ConversationAccess, time.Time, int) ([]DashboardFollowUpRecord, error)
}
