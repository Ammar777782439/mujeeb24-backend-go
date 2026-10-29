package dto

import "encoding/json"

// Dashboard AI Assist proposes explanations. The operation is read-only and
// never accepts SQL, tool names, tenant identity or arbitrary JSONB.
type MerchantAssistRequest struct {
	Question     string `json:"question" minLength:"1" maxLength:"1600"`
	Task         string `json:"task" enum:"explain,summarize"`
	ProjectionID string `json:"projection_id" minLength:"1" maxLength:"80"`
	RecordID     string `json:"record_id,omitempty" maxLength:"128"`
	Search       string `json:"search,omitempty" maxLength:"160"`
}
type MerchantAssistInput struct {
	BusinessPath
	Body MerchantAssistRequest
}
type MerchantAssistResponse struct {
	Answer         string          `json:"answer"`
	ProjectionID   string          `json:"projection_id"`
	Evidence       json.RawMessage `json:"evidence"`
	ReviewRequired bool            `json:"review_required"`
}
