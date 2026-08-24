package contract

import "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"

type UUID = dto.UUID
type Timestamp = dto.Timestamp
type Page = dto.Page
type RequestMeta = dto.RequestMeta
type ErrorBody = dto.ErrorBody
type ErrorEnvelope = dto.ErrorEnvelope

type Principal = dto.Principal
type Business = dto.Business
type BusinessMembership = dto.BusinessMembership
type BusinessPolicy = dto.BusinessPolicy
type DashboardOverview = dto.DashboardOverview
type ChannelConnection = dto.ChannelConnection
type Capability = dto.Capability
type CustomerSummary = dto.CustomerSummary
type Conversation = dto.Conversation
type Message = dto.Message
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
type CustomerPath = dto.CustomerPath
type ListQuery = dto.ListQuery
type BusinessListInput = dto.BusinessListInput
type CatalogListInput = dto.CatalogListInput
type ConversationListInput = dto.ConversationListInput
type ConversationInput = dto.ConversationInput
type ConversationMessageInput = dto.ConversationMessageInput
type CustomerInput = dto.CustomerInput
type BusinessBodyInput = dto.BusinessBodyInput
type SendMessageRequest = dto.SendMessageRequest
type AssignConversationRequest = dto.AssignConversationRequest
type LabelsRequest = dto.LabelsRequest
type PrivateNoteRequest = dto.PrivateNoteRequest
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
type ChatwootWebhookInput = dto.ChatwootWebhookInput
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

type List[T any] struct {
	Body struct {
		Data       []T    `json:"data"`
		Pagination Page   `json:"pagination"`
		RequestID  string `json:"request_id"`
	}
}

type ErrorResponse struct{ Body ErrorEnvelope }
