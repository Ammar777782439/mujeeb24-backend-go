package dto

type SalesWorkflow struct {
	ID              UUID           `json:"id"`
	CustomerID      UUID           `json:"customer_id"`
	ConversationID  *UUID          `json:"conversation_id,omitempty"`
	LeadID          *UUID          `json:"lead_id,omitempty"`
	TransactionID   *UUID          `json:"transaction_id,omitempty"`
	State           string         `json:"state"`
	PendingAction   string         `json:"pending_action,omitempty"`
	Proposal        map[string]any `json:"proposal"`
	ResourceVersion string         `json:"resource_version"`
}
type SalesRequirementDefinition struct {
	Key      string         `json:"attribute_key"`
	Label    string         `json:"label"`
	DataType string         `json:"data_type" enum:"text,number,boolean,date,datetime,select,multi_select,location,money"`
	Required bool           `json:"is_required"`
	Rules    map[string]any `json:"validation_rules,omitempty"`
}
type SalesRequirement struct {
	TransactionType string                       `json:"transaction_type" enum:"order,booking,appointment,service_request,reservation,quote,subscription"`
	SchemaVersion   int                          `json:"schema_version" minimum:"1"`
	Definitions     []SalesRequirementDefinition `json:"definitions"`
}
type SalesWorkflowPath struct {
	BusinessID UUID `path:"business_id" format:"uuid"`
	WorkflowID UUID `path:"workflow_id" format:"uuid"`
}
type SalesWorkflowListInput struct {
	BusinessPath
	ListQuery
	CustomerID string `query:"customer_id"`
	State      string `query:"state"`
}
type SalesWorkflowReviewInput struct {
	SalesWorkflowPath
	CommandHeaders
	Body struct {
		Approved bool `json:"approved"`
	}
}
type SalesRequirementCreateInput struct {
	BusinessPath
	Body SalesRequirement
}
