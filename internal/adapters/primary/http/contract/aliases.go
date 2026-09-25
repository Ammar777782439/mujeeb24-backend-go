package contract

import (
        "net/http"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
)

type UUID = dto.UUID
type Timestamp = dto.Timestamp
type Page = dto.Page
type RequestMeta = dto.RequestMeta
type ErrorBody = dto.ErrorBody
type ErrorEnvelope = dto.ErrorEnvelope

type Principal = dto.Principal
type Business = dto.Business
type BusinessMembership = dto.BusinessMembership
type TeamMember = dto.TeamMember
type TeamInvitation = dto.TeamInvitation
type TeamInvitationCreated = dto.TeamInvitationCreated
type BusinessPolicy = dto.BusinessPolicy
type DashboardOverview = dto.DashboardOverview
type ChannelConnection = dto.ChannelConnection
type ChannelProvisioning = dto.ChannelProvisioning
type Capability = dto.Capability
type CustomerSummary = dto.CustomerSummary
type Conversation = dto.Conversation
type Message = dto.Message
type ConversationRead = dto.ConversationRead
type CannedReply = dto.CannedReply
type AutomationRule = dto.AutomationRule
type ContactPoint = dto.ContactPoint
type Customer = dto.Customer
type Catalog = dto.Catalog
type CatalogItem = dto.CatalogItem
type Offer = dto.Offer
type Variant = dto.Variant
type AttributeDefinition = dto.AttributeDefinition
type AttributeSchema = dto.AttributeSchema
type Lead = dto.Lead
type LeadAttribution = dto.LeadAttribution
type LeadScore = dto.LeadScore
type OrderLineSnapshot = dto.OrderLineSnapshot
type CommercialTransaction = dto.CommercialTransaction
type TransactionReview = dto.TransactionReview
type AIDecision = dto.AIDecision
type AuditEvent = dto.AuditEvent

type EmptyInput = dto.EmptyInput
type BusinessUpdateInput = dto.BusinessUpdateInput
type ConversationMessageListInput = dto.ConversationMessageListInput
type CustomerListInput = dto.CustomerListInput
type CreateCustomerInput = dto.CreateCustomerInput
type BusinessPath = dto.BusinessPath
type ConversationPath = dto.ConversationPath
type TeamMemberPath = dto.TeamMemberPath
type TeamMemberListInput = dto.TeamMemberListInput
type TeamInvitationCreateInput = dto.TeamInvitationCreateInput
type TeamInvitationAcceptInput = dto.TeamInvitationAcceptInput
type TeamMemberRoleUpdateInput = dto.TeamMemberRoleUpdateInput
type TeamMemberRevokeInput = dto.TeamMemberRevokeInput
type CustomerPath = dto.CustomerPath
type ListQuery = dto.ListQuery
type BusinessListInput = dto.BusinessListInput
type MeBusinessListInput = dto.MeBusinessListInput
type CatalogListInput = dto.CatalogListInput
type ConversationListInput = dto.ConversationListInput
type ConversationInput = dto.ConversationInput
type ConversationMessageInput = dto.ConversationMessageInput
type ConversationReadInput = dto.ConversationReadInput
type CannedReplyListInput = dto.CannedReplyListInput
type CannedReplyCreateInput = dto.CannedReplyCreateInput
type CannedReplyUpdateInput = dto.CannedReplyUpdateInput
type SendCannedReplyInput = dto.SendCannedReplyInput
type AutomationRuleListInput = dto.AutomationRuleListInput
type AutomationRuleCreateInput = dto.AutomationRuleCreateInput
type AutomationRuleUpdateInput = dto.AutomationRuleUpdateInput
type CustomerInput = dto.CustomerInput
type BusinessBodyInput = dto.BusinessBodyInput
type SendMessageRequest = dto.SendMessageRequest
type AssignConversationRequest = dto.AssignConversationRequest
type CreateTeamInvitationRequest = dto.CreateTeamInvitationRequest
type AcceptTeamInvitationRequest = dto.AcceptTeamInvitationRequest
type UpdateTeamMemberRoleRequest = dto.UpdateTeamMemberRoleRequest
type LabelsRequest = dto.LabelsRequest
type PrivateNoteRequest = dto.PrivateNoteRequest
type CreateCannedReplyRequest = dto.CreateCannedReplyRequest
type UpdateCannedReplyRequest = dto.UpdateCannedReplyRequest
type CreateAutomationRuleRequest = dto.CreateAutomationRuleRequest
type UpdateAutomationRuleRequest = dto.UpdateAutomationRuleRequest
type CreateCustomerRequest = dto.CreateCustomerRequest
type ContactPointInput = dto.ContactPointInput
type MergeCustomerRequest = dto.MergeCustomerRequest
type BusinessUpdateRequest = dto.BusinessUpdateRequest
type BusinessPolicyUpdateRequest = dto.BusinessPolicyUpdateRequest
type CatalogPath = dto.CatalogPath
type ItemPath = dto.ItemPath
type OfferPath = dto.OfferPath
type VariantPath = dto.VariantPath
type SchemaPath = dto.SchemaPath
type LeadPath = dto.LeadPath
type TransactionPath = dto.TransactionPath
type DecisionPath = dto.DecisionPath
type AuditPath = dto.AuditPath
type ConnectionPath = dto.ConnectionPath
type ItemOffersInput = dto.ItemOffersInput
type ItemVariantsInput = dto.ItemVariantsInput
type CatalogItemsInput = dto.CatalogItemsInput
type AttributeSchemasInput = dto.AttributeSchemasInput
type LeadListInput = dto.LeadListInput
type LeadAttributionsInput = dto.LeadAttributionsInput
type LeadScoresInput = dto.LeadScoresInput
type TransactionListInput = dto.TransactionListInput
type AIDecisionListInput = dto.AIDecisionListInput
type AuditListInput = dto.AuditListInput
type CommandHeaders = dto.CommandHeaders
type BeginChannelConnectionRequest = dto.BeginChannelConnectionRequest
type ConnectionActionRequest = dto.ConnectionActionRequest
type UpdateConversationRequest = dto.UpdateConversationRequest
type CreateCatalogRequest = dto.CreateCatalogRequest
type UpdateCatalogRequest = dto.UpdateCatalogRequest
type CreateAttributeSchemaRequest = dto.CreateAttributeSchemaRequest
type AttributeDefinitionInput = dto.AttributeDefinitionInput
type CreateCatalogItemRequest = dto.CreateCatalogItemRequest
type UpdateCatalogItemRequest = dto.UpdateCatalogItemRequest
type CreateOfferRequest = dto.CreateOfferRequest
type UpdateOfferRequest = dto.UpdateOfferRequest
type CreateVariantRequest = dto.CreateVariantRequest
type UpdateVariantRequest = dto.UpdateVariantRequest
type CreateLeadRequest = dto.CreateLeadRequest
type UpdateLeadRequest = dto.UpdateLeadRequest
type LeadDecisionRequest = dto.LeadDecisionRequest
type MarkLeadLostRequest = dto.MarkLeadLostRequest
type TransactionLineInput = dto.TransactionLineInput
type CreateTransactionDraftRequest = dto.CreateTransactionDraftRequest
type UpdateTransactionDraftRequest = dto.UpdateTransactionDraftRequest
type ConfirmTransactionRequest = dto.ConfirmTransactionRequest
type TransactionReasonRequest = dto.TransactionReasonRequest
type SubmitReviewRequest = dto.SubmitReviewRequest
type ReviewDecisionRequest = dto.ReviewDecisionRequest
type AIHumanRequest = dto.AIHumanRequest
type BusinessPolicyInput = dto.BusinessPolicyInput
type ConnectionCapabilityInput = dto.ConnectionCapabilityInput
type CatalogCreateInput = dto.CatalogCreateInput
type CatalogUpdateInput = dto.CatalogUpdateInput
type CatalogItemPath = dto.CatalogItemPath
type CatalogItemCreateInput = dto.CatalogItemCreateInput
type CatalogItemUpdateInput = dto.CatalogItemUpdateInput
type OfferCreateInput = dto.OfferCreateInput
type OfferUpdateInput = dto.OfferUpdateInput
type VariantCreateInput = dto.VariantCreateInput
type VariantUpdateInput = dto.VariantUpdateInput
type AttributeSchemaCreateInput = dto.AttributeSchemaCreateInput
type LeadCreateInput = dto.LeadCreateInput
type LeadUpdateInput = dto.LeadUpdateInput
type LeadQualifyInput = dto.LeadQualifyInput
type LeadLostInput = dto.LeadLostInput
type TransactionCreateInput = dto.TransactionCreateInput
type TransactionUpdateInput = dto.TransactionUpdateInput
type TransactionConfirmInput = dto.TransactionConfirmInput
type TransactionCancelInput = dto.TransactionCancelInput
type TransactionReviewSubmitInput = dto.TransactionReviewSubmitInput
type TransactionReviewDecisionInput = dto.TransactionReviewDecisionInput
type AIHumanInput = dto.AIHumanInput
type LoginInput = dto.LoginInput
type RefreshInput = dto.RefreshInput
type LogoutInput = dto.LogoutInput
type NoContentOutput = dto.NoContentOutput
type AuthResponse = dto.AuthResponse
type Health = dto.Health
type MetricsOutput = dto.MetricsOutput
type WebhookPath = dto.WebhookPath
type WebhookHeaders = dto.WebhookHeaders
type SocialWebhookInput = dto.SocialWebhookInput
type WebhookAccepted = dto.WebhookAccepted
type WebhookAcceptedOutput = dto.WebhookAcceptedOutput
type ConnectionCreateInput = dto.ConnectionCreateInput
type ConnectionActionInput = dto.ConnectionActionInput
type ConversationAssignInput = dto.ConversationAssignInput
type ConversationLabelsInput = dto.ConversationLabelsInput
type ConversationNoteInput = dto.ConversationNoteInput
type CustomerConversationsInput = dto.CustomerConversationsInput
type CustomerTransactionsInput = dto.CustomerTransactionsInput
type CustomerMergeInput = dto.CustomerMergeInput
type ConnectionListInput = dto.ConnectionListInput
type ConversationUpdateInput = dto.ConversationUpdateInput
type CustomerUpdateInput = dto.CustomerUpdateInput
type UpdateCustomerRequest = dto.UpdateCustomerRequest

type Single[T any] struct {
        Body struct {
                Data      T      `json:"data"`
                RequestID string `json:"request_id"`
        }
}

type AuthOutput struct {
        SetCookie *http.Cookie `header:"Set-Cookie"`
        Body      struct {
                Data      AuthResponse `json:"data"`
                RequestID string       `json:"request_id"`
        }
}

type List[T any] struct {
        Body struct {
                Data       []T    `json:"data"`
                Pagination Page   `json:"pagination"`
                RequestID  string `json:"request_id"`
        }
}

type ErrorResponse struct{ Body ErrorEnvelope }

// MerchantAIChatInput per contract 11 §6 — B2B Merchant Catalog AI turn.
type MerchantAIChatInput = dto.MerchantAIChatInput
type MerchantAIChatRequest = dto.MerchantAIChatRequest

// MerchantAIChatResponse is the B2B Merchant Catalog AI proposal projection.
// Per contract 11 §5, the agent returns a CatalogOperationProposal.
//
// Per ADR-044 layer 2: the structured Create/Update/Delete payloads are
// returned alongside the human-readable response_text. The frontend
// renders these as a card with approve/reject buttons. When the merchant
// approves, the frontend calls the catalog application service (separate
// endpoint) to execute the mutation.
//
// Per ADR-044 layer 3: MissingFields is populated when status=needs_more_data
// — the frontend can render these as a form for the merchant to fill in.
type MerchantAIChatResponse struct {
        Operation    string                          `json:"operation"`
        Status       string                          `json:"status"`
        ResponseText string                          `json:"response_text,omitempty"`
        SessionID    string                          `json:"session_id"`
        // Per ADR-044 layer 2: structured proposal payload. Present only when
        // Operation is create/update/delete (NOT ask_merchant/answer).
        // The frontend renders this as an approval card.
        Create       *MerchantAIProposalCreate       `json:"create,omitempty"`
        Update       *MerchantAIProposalUpdate       `json:"update,omitempty"`
        Delete       *MerchantAIProposalDelete       `json:"delete,omitempty"`
        // Per ADR-044 layer 3: fields the agent needs from the merchant
        // before the proposal can be executed. The frontend can render these
        // as a form instead of asking the merchant to type the answer.
        MissingFields []MerchantAIMissingField       `json:"missing_fields,omitempty"`
        // TargetCatalogID per ADR-041 — populated by CatalogResolutionService
        // when the operation is a mutation. The frontend displays "ستُضاف إلى:
        // <catalog_name>" using this ID.
        TargetCatalogID string                       `json:"target_catalog_id,omitempty"`
}

// MerchantAIProposalCreate mirrors services.CatalogCreatePayload for the
// HTTP response. Defined here (contract package) to avoid importing the
// services package into the dto layer (would create an import cycle).
type MerchantAIProposalCreate struct {
        TargetCatalogID string                          `json:"target_catalog_id,omitempty"`
        Item            *MerchantAIProposalItem         `json:"item,omitempty"`
        Variants        []MerchantAIProposalVariant      `json:"variants,omitempty"`
        Offers          []MerchantAIProposalOffer        `json:"offers,omitempty"`
}

// MerchantAIProposalUpdate mirrors services.CatalogUpdatePayload.
type MerchantAIProposalUpdate struct {
        ItemID      string                          `json:"item_id"`
        Changes     MerchantAIProposalItem          `json:"changes"`
        NewVariants []MerchantAIProposalVariant     `json:"new_variants,omitempty"`
        NewOffers   []MerchantAIProposalOffer      `json:"new_offers,omitempty"`
}

// MerchantAIProposalDelete mirrors services.CatalogDeletePayload.
type MerchantAIProposalDelete struct {
        ItemID      string `json:"item_id"`
        Confirmed   bool   `json:"confirmed"`
        ReasonGiven string `json:"reason_given,omitempty"`
}

// MerchantAIProposalItem is the HTTP projection of the AI-proposed item.
type MerchantAIProposalItem struct {
        Name                 string         `json:"name"`
        ItemType             string         `json:"item_type"`
        ShortDescription     string         `json:"short_description,omitempty"`
        LongDescription      string         `json:"long_description,omitempty"`
        PricingMode          string         `json:"pricing_mode"`
        AvailabilityMode     string         `json:"availability_mode"`
        FulfillmentMode      string         `json:"fulfillment_mode"`
        RequiresConfirmation bool           `json:"requires_confirmation"`
        Attributes           map[string]any `json:"attributes,omitempty"`
}

// MerchantAIProposalVariant is the HTTP projection of the AI-proposed variant.
type MerchantAIProposalVariant struct {
        Name       string         `json:"name"`
        Attributes map[string]any `json:"attributes,omitempty"`
}

// MerchantAIProposalOffer is the HTTP projection of the AI-proposed offer.
type MerchantAIProposalOffer struct {
        VariantNameRef     string  `json:"variant_name_ref,omitempty"`
        Name               string  `json:"name"`
        PricingMode        string  `json:"pricing_mode"`
        Amount             *string `json:"amount,omitempty"`
        Currency           *string `json:"currency,omitempty"`
        PricingUnit        *string `json:"pricing_unit,omitempty"`
        PriceSource        *string `json:"price_source,omitempty"`
        AvailabilityMode   *string `json:"availability_mode,omitempty"`
        AvailabilityStatus *string `json:"availability_status,omitempty"`
        FulfillmentMode    *string `json:"fulfillment_mode,omitempty"`
        ValidityFrom       *string `json:"validity_from,omitempty"`
        ValidityUntil      *string `json:"validity_until,omitempty"`
}

// MerchantAIMissingField is the HTTP projection of services.CatalogMissingField.
// Per ADR-044 layer 3, the frontend can render these as a form for the
// merchant to fill in (instead of asking them to type the answer in chat).
type MerchantAIMissingField struct {
        Path        string `json:"path"`
        DisplayName string `json:"display_name"`
        DataType    string `json:"data_type"`
        Reason      string `json:"reason"`
}
