package dto

type MerchantAIChatRequest struct {
	Message   string `json:"message" minLength:"1" maxLength:"10000" doc:"Merchant natural language input message"`
	SessionID *UUID  `json:"session_id,omitempty" format:"uuid" doc:"Optional assistant session ID to continue multi-turn interaction"`
}

type MerchantAIChatInput struct {
	BusinessPath
	CommandHeaders
	Body MerchantAIChatRequest
}

type MerchantAIChatResponse struct {
	Message   string `json:"message" doc:"Assistant response or clarification text in Arabic"`
	Action    string `json:"action" enum:"answer,ask_clarification,catalog_updated,no_action" doc:"Assistant conversational outcome"`
	SessionID *UUID  `json:"session_id,omitempty" format:"uuid" doc:"Session ID for continuing multi-turn interaction"`
}
