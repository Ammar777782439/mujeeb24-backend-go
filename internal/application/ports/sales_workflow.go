package ports

import (
	"context"
	"time"
)

// Workflow arguments are proposals. Identity, money, permissions and result are server-owned.
type SalesWorkflowProposal struct {
	WorkflowID         string              `json:"workflow_id,omitempty"`
	ExpectedVersion    int64               `json:"expected_version,omitempty"`
	TransactionType    string              `json:"transaction_type,omitempty"`
	Lines              []SalesWorkflowLine `json:"lines,omitempty"`
	TransactionData    map[string]any      `json:"transaction_data,omitempty"`
	EvidenceMessageIDs []string            `json:"evidence_message_ids"`
}
type SalesWorkflowLine struct {
	ItemID             string         `json:"item_id"`
	OfferID            *string        `json:"offer_id,omitempty"`
	VariantID          *string        `json:"variant_id,omitempty"`
	Quantity           string         `json:"quantity"`
	SelectedAttributes map[string]any `json:"selected_attributes,omitempty"`
}
type SalesWorkflowRecord struct {
	PendingAction   string                `json:"pending_action,omitempty"`
	SourceMessageID *string               `json:"source_message_id,omitempty"`
	ID              string                `json:"id"`
	CustomerID      string                `json:"customer_id"`
	ConversationID  string                `json:"conversation_id"`
	LeadID          *string               `json:"lead_id,omitempty"`
	TransactionID   *string               `json:"transaction_id,omitempty"`
	State           string                `json:"state"`
	Proposal        SalesWorkflowProposal `json:"proposal"`
	PolicyVersion   int64                 `json:"policy_version"`
	ResourceVersion int64                 `json:"resource_version"`
}
type SalesRequirement struct {
	TransactionType string                       `json:"transaction_type"`
	SchemaVersion   int                          `json:"schema_version"`
	Definitions     []SalesRequirementDefinition `json:"definitions"`
}
type SalesRequirementDefinition struct {
	Key      string         `json:"attribute_key"`
	Label    string         `json:"label"`
	DataType string         `json:"data_type"`
	Required bool           `json:"is_required"`
	Rules    map[string]any `json:"validation_rules"`
}
type SalesWorkflowContext struct {
	// Server-owned consent for the active sales workflow. Never model-authored.
	StartRequestID    string                `json:"start_request_id,omitempty"`
	Enabled           bool                  `json:"enabled"`
	AllowInteractions bool                  `json:"allow_interactions"`
	Active            []SalesWorkflowRecord `json:"active"`
	Requirements      []SalesRequirement    `json:"requirements"`
}
type SalesWorkflowInput struct {
	BusinessID, CustomerID, ConversationID, SourceMessageID, IdempotencyKey string
	DecisionID                                                              string
	SalesStartRequestID                                                     string
	Capability                                                              CustomerSalesCapability
	Proposal                                                                SalesWorkflowProposal
	HumanApproved                                                           bool
	ActorReference                                                          string
	RequiresApproval                                                        bool
	Now                                                                     time.Time
}
type SalesWorkflowResult struct {
	MissingFields       []string             `json:"missing_fields,omitempty"`
	NeedsMerchantReview bool                 `json:"needs_merchant_review,omitempty"`
	Lines               []SalesReceiptLine   `json:"lines,omitempty"`
	Details             []SalesReceiptDetail `json:"details,omitempty"`
	Workflow            SalesWorkflowRecord  `json:"workflow"`
	Duplicate           bool                 `json:"-"`
	ConfirmationID      string               `json:"confirmation_id,omitempty"`
	TotalAmount         *string              `json:"total_amount,omitempty"`
	Currency            *string              `json:"currency,omitempty"`
}
type SalesButtonAction struct {
	BusinessID, CustomerID, ConversationID, SourceMessageID, Payload string
	Now                                                              time.Time
}
type SalesWorkflowRepository interface {
	GetContext(context.Context, string, string, string) (SalesWorkflowContext, error)
	Apply(context.Context, SalesWorkflowInput) (SalesWorkflowResult, error)
	ApplyButton(context.Context, SalesButtonAction) (SalesWorkflowResult, error)
}

type InteractiveButton struct {
	Title   string `json:"title"`
	Payload string `json:"payload"`
}
type InteractiveMessage struct {
	Text    string              `json:"text"`
	Buttons []InteractiveButton `json:"buttons,omitempty"`
}

type SalesReceiptLine struct {
	Name     string  `json:"name"`
	Quantity string  `json:"quantity"`
	Total    *string `json:"total,omitempty"`
	Currency *string `json:"currency,omitempty"`
}
type SalesReceiptDetail struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

// SalesStartTTL is the initial validity window for starting a customer sales journey.
const SalesStartTTL = 24 * time.Hour

// The start decision is only permission to prepare a sale, never agreement confirmation.
type SalesStartRequest struct {
	ID, BusinessID, CustomerID, ConversationID string
	OutboundMessageID, SourceMessageID         string
	CreatedAt, ExpiresAt                       time.Time
}

type SalesStartDecision struct {
	ID, BusinessID, CustomerID, ConversationID, SourceMessageID string
	Accepted, Duplicate, RequiresApproval                       bool
}

type SalesStartRepository interface {
	Create(context.Context, SalesStartRequest) error
	Decide(context.Context, SalesButtonAction) (SalesStartDecision, error)
}
