package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// buildUserPrompt encodes the customer-facing context and message for Gemini.
// Prompt construction belongs to the customer-sales capability, not the provider client.
func buildUserPrompt(input ports.CustomerSalesDecisionRequest) string {
	prompt := fmt.Sprintf("Business ID: %s\nConversation ID: %s\nChannel: %s\nPolicy version: %s\nSource message reference: %s\nCustomer message:\n%s",
		input.BusinessID, input.ConversationID, input.Channel, input.PolicyVersion,
		input.SourceMessageReference, strings.TrimSpace(input.Text))
	if input.Context == nil {
		return prompt
	}
	encoded, err := json.Marshal(customerSalesPromptContextFrom(input.Context))
	if err != nil {
		return prompt + "\nVerified Mujeeb context: unavailable"
	}
	return prompt + "\nVerified Mujeeb context (evidence only; do not infer missing facts):\n" + string(encoded)
}

type customerSalesPromptContext struct {
	SchemaVersion          int                                            `json:"schema_version"`
	Freshness              string                                         `json:"freshness"`
	Business               ports.CustomerSalesContextBusiness             `json:"business"`
	Conversation           ports.CustomerSalesContextConversation         `json:"conversation"`
	Customer               customerSalesPromptCustomer                    `json:"customer"`
	CatalogEvidence        []ports.CustomerSalesCatalogEvidence            `json:"catalog_evidence"`
	CatalogSummary         []ports.CustomerSalesCatalogSummaryEntry        `json:"catalog_summary,omitempty"`
	OfferEvidence          []ports.CustomerSalesOfferEvidence              `json:"offer_evidence"`
	VariantEvidence        []ports.CustomerSalesVariantEvidence            `json:"variant_evidence"`
	KnowledgeEvidence      []ports.CustomerSalesKnowledgeEvidence          `json:"knowledge_evidence"`
	BusinessPolicyEvidence []ports.CustomerSalesBusinessPolicyEvidence     `json:"business_policy_evidence"`
	RecentMessages         []ports.CustomerSalesRecentMessageEvidence      `json:"recent_messages"`
	PolicyEvidence         ports.CustomerSalesPolicyEvidence              `json:"policy_evidence"`
	KnowledgeState         string                                         `json:"knowledge_state"`
	ConversationState      *ports.ConversationStateRecord                 `json:"conversation_state,omitempty"`
	ConversationSummary    string                                         `json:"conversation_summary,omitempty"`
	CatalogNames           []string                                       `json:"catalog_names,omitempty"`
	GeneratedAt            time.Time                                      `json:"generated_at"`
	ExpiresAt              time.Time                                      `json:"expires_at"`
}

type customerSalesPromptCustomer struct {
	Reference        string `json:"reference"`
	LocalePreference string `json:"locale_preference"`
	Status           string `json:"status"`
}

func customerSalesPromptContextFrom(value *ports.CustomerSalesContext) customerSalesPromptContext {
	return customerSalesPromptContext{
		SchemaVersion:   value.SchemaVersion,
		Freshness:       value.Freshness,
		Business:        value.Business,
		Conversation:    value.Conversation,
		Customer: customerSalesPromptCustomer{
			Reference:        value.Customer.Reference,
			LocalePreference: value.Customer.LocalePreference,
			Status:           value.Customer.Status,
		},
		CatalogEvidence:        value.CatalogEvidence,
		CatalogSummary:         value.CatalogSummary,
		OfferEvidence:          value.OfferEvidence,
		VariantEvidence:        value.VariantEvidence,
		KnowledgeEvidence:      value.KnowledgeEvidence,
		BusinessPolicyEvidence: value.BusinessPolicyEvidence,
		RecentMessages:         value.RecentMessages,
		PolicyEvidence:         value.PolicyEvidence,
		KnowledgeState:         value.KnowledgeState,
		ConversationState:      value.ConversationState,
		ConversationSummary:    value.ConversationSummary,
		CatalogNames:           value.CatalogNames,
		GeneratedAt:            value.GeneratedAt,
		ExpiresAt:              value.ExpiresAt,
	}
}
