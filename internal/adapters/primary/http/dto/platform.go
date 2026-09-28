package dto

// ============================================================================
// Platform Administration Contract V1 — HTTP DTOs
//
// Per Contract §56: every platform API lives under /api/v1/platform/* and
// is SEPARATE from merchant APIs (/api/v1/businesses/*).
//
// All inputs/outputs are platform-owned records (plans, subscriptions,
// platform_audit_events, support tickets). They NEVER expose merchant
// content (customer records, conversation messages, catalog, AI context).
//
// The DTOs mirror the JSON shapes documented in:
//   - docs/architecture/PlatformAdministrationContractV1.md
//   - docs/architecture/AIUsageTokenTelemetry.md
// ============================================================================

// ----------------------------------------------------------------------------
// Shared path/header inputs
// ----------------------------------------------------------------------------

type PlatformBusinessPath struct {
        BusinessID UUID `path:"business_id" format:"uuid"`
}

type PlatformPlanPath struct {
        PlanID UUID `path:"plan_id" format:"uuid"`
}

type PlatformSubscriptionPath struct {
        SubscriptionID UUID `path:"subscription_id" format:"uuid"`
}

type PlatformSupportTicketPath struct {
        TicketID UUID `path:"ticket_id" format:"uuid"`
}

type PlatformProviderPath struct {
        ProviderID string `path:"provider_id"`
}

type PlatformChannelPath struct {
        ConnectionID UUID `path:"connection_id" format:"uuid"`
}

// CommandHeaders carry Idempotency-Key, If-Match, X-Request-ID, X-Correlation-ID.
// Reused from the existing merchant-side DTO shape.
type PlatformCommandHeaders struct {
        IdempotencyKey  string `header:"Idempotency-Key,omitempty"`
        IfMatch         string `header:"If-Match,omitempty"`
        XRequestID      string `header:"X-Request-ID,omitempty"`
        XCorrelationID  string `header:"X-Correlation-ID,omitempty"`
}

// ----------------------------------------------------------------------------
// Plan DTOs (Contract §15-19)
// ----------------------------------------------------------------------------

type CreatePlanRequest struct {
        Code                     string `json:"code" minLength:"1" maxLength:"64"`
        DisplayName              string `json:"display_name" minLength:"1" maxLength:"128"`
        PriceYER                 int    `json:"price_yer" minimum:"1"`
        BillingInterval          string `json:"billing_interval" default:"MONTH"`
        AIReplyLimit             int    `json:"ai_reply_limit" minimum:"1"`
        AICatalogLimit           int    `json:"ai_catalog_limit" minimum:"0"`
        ChannelLimit             int    `json:"channel_limit" minimum:"1"`
        InternalAICostBudgetYER  int    `json:"internal_ai_cost_budget_yer" minimum:"1"`
}

type CreatePlanInput struct {
        PlatformCommandHeaders
        Body CreatePlanRequest
}

type PlanView struct {
        ID                       string  `json:"id"`
        Code                     string  `json:"code"`
        Version                  int     `json:"version"`
        DisplayName              string  `json:"display_name"`
        PriceYER                 int     `json:"price_yer"`
        BillingInterval          string  `json:"billing_interval"`
        AIReplyLimit             int     `json:"ai_reply_limit"`
        AICatalogLimit           int     `json:"ai_catalog_limit"`
        ChannelLimit             int     `json:"channel_limit"`
        InternalAICostBudgetYER  int     `json:"internal_ai_cost_budget_yer"`
        Status                   string  `json:"status"`
        CreatedAt                string  `json:"created_at"`
        UpdatedAt                string  `json:"updated_at"`
        RetiredAt                *string `json:"retired_at,omitempty"`
}

type PlanListInput struct {
        Status string `query:"status,omitempty"`
        Code   string `query:"code,omitempty"`
        Limit  int    `query:"limit,omitempty" minimum:"1" maximum:"1000"`
        Cursor string `query:"cursor,omitempty"`
}

type PlanActivateInput struct {
        PlatformPlanPath
        PlatformCommandHeaders
}

type PlanRetireInput struct {
        PlatformPlanPath
        PlatformCommandHeaders
}

// CreatePlanVersionRequest is the body for POST /platform/plans/{plan_id}/versions.
// Per Contract §17 + AIUsageTokenTelemetry.md §22-23: changing any field
// creates a new plan version; the original plan row is never edited.
type CreatePlanVersionRequest struct {
        DisplayName              string `json:"display_name" minLength:"1" maxLength:"128"`
        PriceYER                 int    `json:"price_yer" minimum:"1"`
        BillingInterval          string `json:"billing_interval" default:"MONTH"`
        AIReplyLimit             int    `json:"ai_reply_limit" minimum:"1"`
        AICatalogLimit           int    `json:"ai_catalog_limit" minimum:"0"`
        ChannelLimit             int    `json:"channel_limit" minimum:"1"`
        InternalAICostBudgetYER  int    `json:"internal_ai_cost_budget_yer" minimum:"1"`
        Reason                   string `json:"reason" minLength:"1" maxLength:"500"`
}

type CreatePlanVersionInput struct {
        PlatformPlanPath
        PlatformCommandHeaders
        Body CreatePlanVersionRequest
}

// ----------------------------------------------------------------------------
// Platform Business Management DTOs (Contract §8-14)
// ----------------------------------------------------------------------------

type PlatformBusinessView struct {
        ID                     string  `json:"id"`
        Name                   string  `json:"name"`
        Slug                   string  `json:"slug"`
        PlatformStatus         string  `json:"platform_status"`
        OwnerIdentitySummary  *string `json:"owner_identity_summary,omitempty"`
        SubscriptionSummary   *string `json:"subscription_summary,omitempty"`
        CreatedAt             string  `json:"created_at"`
        UpdatedAt            string  `json:"updated_at"`
}

type PlatformBusinessListInput struct {
        Search string `query:"search,omitempty"`
        Status string `query:"status,omitempty"`
        // CreatedFrom / CreatedTo use a string (RFC 3339) rather than *time.Time
        // because Huma does not support pointer types for query parameters.
        // Empty string = no filter on that bound.
        CreatedFrom string `query:"created_from,omitempty" format:"date-time"`
        CreatedTo   string `query:"created_to,omitempty" format:"date-time"`
        Limit       int    `query:"limit,omitempty" minimum:"1" maximum:"1000"`
}

type PlatformBusinessSuspendInput struct {
        PlatformBusinessPath
        PlatformCommandHeaders
}

type PlatformBusinessReactivateInput struct {
        PlatformBusinessPath
        PlatformCommandHeaders
}

type PlatformBusinessArchiveInput struct {
        PlatformBusinessPath
        PlatformCommandHeaders
}

// ----------------------------------------------------------------------------
// Platform Audit DTOs (Contract §45-49)
// ----------------------------------------------------------------------------

type PlatformAuditEventView struct {
        ID                  string  `json:"id"`
        ActorPlatformAdminID *string `json:"actor_platform_admin_id,omitempty"`
        Action              string  `json:"action"`
        TargetType          string  `json:"target_type"`
        TargetID            *string `json:"target_id,omitempty"`
        BusinessID          *string `json:"business_id,omitempty"`
        Result              string  `json:"result"`
        FailureCode         *string `json:"failure_code,omitempty"`
        CorrelationID       *string `json:"correlation_id,omitempty"`
        OccurredAt          string  `json:"occurred_at"`
        Metadata            map[string]any `json:"metadata"`
}

type PlatformAuditListInput struct {
        Action       string `query:"action,omitempty"`
        TargetType   string `query:"target_type,omitempty"`
        TargetID     string `query:"target_id,omitempty"`
        BusinessID   string `query:"business_id,omitempty"`
        Result       string `query:"result,omitempty"`
        // OccurredFrom / OccurredTo are RFC 3339 strings — Huma does not support
        // *time.Time for query parameters. Empty = no filter on that bound.
        OccurredFrom string `query:"occurred_from,omitempty" format:"date-time"`
        OccurredTo   string `query:"occurred_to,omitempty" format:"date-time"`
        Limit        int    `query:"limit,omitempty" minimum:"1" maximum:"1000"`
        Cursor       string `query:"cursor,omitempty"`
}

type PlatformAuditEventPathInput struct {
        EventID string `path:"event_id"`
}

// ----------------------------------------------------------------------------
// AI Operations DTOs (Contract §73-108, AIUsageTokenTelemetry.md)
// ----------------------------------------------------------------------------

type PlatformAIOverviewView struct {
        Runtime struct {
                Status string `json:"status"`
                Health string `json:"health"`
        } `json:"runtime"`
        Provider struct {
                Provider string `json:"provider"`
                Model    string `json:"model"`
                Enabled  bool   `json:"enabled"`
                Health   string `json:"health"`
        } `json:"provider"`
}

type PlatformAIRuntimeDisableInput struct {
        PlatformCommandHeaders
}

type PlatformAIRuntimeEnableInput struct {
        PlatformCommandHeaders
}

type PlatformAIHealthCheckInput struct {
        PlatformCommandHeaders
}

type PlatformAIHealthCheckResult struct {
        Provider    string `json:"provider"`
        Model       string `json:"model"`
        Result      string `json:"result"`
        LatencyMS   int64  `json:"latency_ms"`
        CheckedAt   string `json:"checked_at"`
        FailureCode string `json:"failure_code,omitempty"`
}

// ----------------------------------------------------------------------------
// Subscription AI Usage (AIUsageTokenTelemetry.md §32-34)
// ----------------------------------------------------------------------------

type SubscriptionAIUsageView struct {
        SubscriptionID string `json:"subscription_id"`
        AIReplyLimit   int    `json:"ai_reply_limit"`
        AIRepliesUsed  int    `json:"ai_replies_used"`
        AIRepliesRemaining int `json:"ai_replies_remaining"`
        InputTokens       int64 `json:"input_tokens"`
        CachedInputTokens int64 `json:"cached_input_tokens"`
        OutputTokens      int64 `json:"output_tokens"`
        ModelRequests     int   `json:"model_requests"`
        ToolCalls         int   `json:"tool_calls"`
        ActualProviderCostYER    int `json:"actual_provider_cost_yer"`
        InternalCostBudgetYER    int `json:"internal_cost_budget_yer"`
        CostRemainingYER         int `json:"cost_remaining_yer"`
        AverageCostPerReplyYER   int `json:"average_cost_per_reply_yer"`
        ProjectedRemainingCostYER int `json:"projected_remaining_cost_yer"`
        ProjectedTotalCostYER     int `json:"projected_total_cost_yer"`
        BudgetStatus              string `json:"budget_status"`
}

type SubscriptionAICostBudgetOverrideRequest struct {
        BudgetYER int    `json:"budget_yer" minimum:"1"`
        Reason    string `json:"reason" minLength:"1" maxLength:"500"`
}

type SubscriptionAICostBudgetOverrideInput struct {
        PlatformSubscriptionPath
        PlatformCommandHeaders
        Body SubscriptionAICostBudgetOverrideRequest
}

// ----------------------------------------------------------------------------
// Subscription Lifecycle DTOs (Contract §20-36)
// ----------------------------------------------------------------------------

type SubscriptionView struct {
        ID                          string  `json:"id"`
        BusinessID                  string  `json:"business_id"`
        PlanID                      string  `json:"plan_id"`
        PlanCode                    string  `json:"plan_code"`
        PlanVersion                 int     `json:"plan_version"`
        PeriodStart                 string  `json:"period_start"`
        PeriodEnd                   string  `json:"period_end"`
        Status                      string  `json:"status"`
        AIReplyLimit                int     `json:"ai_reply_limit"`
        AICatalogLimit              int     `json:"ai_catalog_limit"`
        ChannelLimit                int     `json:"channel_limit"`
        InternalAICostBudgetYER     int     `json:"internal_ai_cost_budget_yer"`
        CostBudgetOverrideYER       *int    `json:"cost_budget_override_yer,omitempty"`
        CostBudgetOverrideReason    *string `json:"cost_budget_override_reason,omitempty"`
        CancelledAt                 *string `json:"cancelled_at,omitempty"`
        CancelledReason             *string `json:"cancelled_reason,omitempty"`
        CreatedAt                   string  `json:"created_at"`
        UpdatedAt                   string  `json:"updated_at"`
}

type CreateSubscriptionRequest struct {
        PlanID string `json:"plan_id" format:"uuid"`
}

type CreateSubscriptionInput struct {
        PlatformBusinessPath
        PlatformCommandHeaders
        Body CreateSubscriptionRequest
}

type SubscriptionListInput struct {
        BusinessID string `query:"business_id,omitempty"`
        Status     string `query:"status,omitempty"`
        Limit      int    `query:"limit,omitempty" minimum:"1" maximum:"1000"`
        Cursor     string `query:"cursor,omitempty"`
}

type CancelSubscriptionRequest struct {
        Reason string `json:"reason" minLength:"1" maxLength:"500"`
}

type CancelSubscriptionInput struct {
        PlatformSubscriptionPath
        PlatformCommandHeaders
        Body CancelSubscriptionRequest
}

// ----------------------------------------------------------------------------
// Payment DTOs (Contract §25-28)
// ----------------------------------------------------------------------------

type PaymentView struct {
        ID             string `json:"id"`
        SubscriptionID string `json:"subscription_id"`
        BusinessID     string `json:"business_id"`
        AmountYER      int    `json:"amount_yer"`
        Method         string `json:"method"`
        Reference      string `json:"reference"`
        PaidAt         string `json:"paid_at"`
        RecordedBy     string `json:"recorded_by"`
        CreatedAt      string `json:"created_at"`
}

type RecordPaymentRequest struct {
        AmountYER int    `json:"amount_yer" minimum:"1"`
        Method    string `json:"method" enum:"CASH,BANK_TRANSFER,MOBILE_MONEY,OTHER"`
        Reference string `json:"reference" minLength:"1" maxLength:"256"`
        PaidAt    string `json:"paid_at" format:"date-time"`
}

type RecordPaymentInput struct {
        PlatformSubscriptionPath
        PlatformCommandHeaders
        Body RecordPaymentRequest
}

type PaymentListInput struct {
        SubscriptionID string `query:"subscription_id,omitempty"`
        BusinessID     string `query:"business_id,omitempty"`
        Method         string `query:"method,omitempty"`
        Limit          int    `query:"limit,omitempty" minimum:"1" maximum:"1000"`
        Cursor         string `query:"cursor,omitempty"`
}

// ----------------------------------------------------------------------------
// Support Management DTOs (Contract §37-44)
// ----------------------------------------------------------------------------

type SupportTicketView struct {
        ID              string  `json:"id"`
        BusinessID      string  `json:"business_id"`
        Subject         string  `json:"subject"`
        Status          string  `json:"status"`
        Priority        string  `json:"priority"`
        Category        string  `json:"category"`
        CreatedBy       string  `json:"created_by"`
        CreatedByType   string  `json:"created_by_type"`
        ResolvedAt      *string `json:"resolved_at,omitempty"`
        ResolvedBy      *string `json:"resolved_by,omitempty"`
        ClosedAt        *string `json:"closed_at,omitempty"`
        ClosedBy        *string `json:"closed_by,omitempty"`
        LastMessageAt   *string `json:"last_message_at,omitempty"`
        CreatedAt       string  `json:"created_at"`
        UpdatedAt       string  `json:"updated_at"`
}

type SupportMessageView struct {
        ID          string `json:"id"`
        TicketID    string `json:"ticket_id"`
        BusinessID  string `json:"business_id"`
        AuthorType  string `json:"author_type"`
        AuthorID    string `json:"author_id"`
        Body        string `json:"body"`
        CreatedAt   string `json:"created_at"`
}

type CreateSupportTicketRequest struct {
        Subject   string `json:"subject" minLength:"1" maxLength:"256"`
        Priority  string `json:"priority,omitempty" enum:"NORMAL,HIGH,CRITICAL"`
        Category  string `json:"category,omitempty" enum:"ACCOUNT,CHANNEL,AI,CATALOG,SALES,BILLING,OTHER"`
}

type CreateSupportTicketInput struct {
        PlatformBusinessPath
        PlatformCommandHeaders
        Body CreateSupportTicketRequest
}

type SupportTicketListInput struct {
        BusinessID   string `query:"business_id,omitempty"`
        Status       string `query:"status,omitempty"`
        Priority     string `query:"priority,omitempty"`
        Category     string `query:"category,omitempty"`
        CreatedFrom  string `query:"created_from,omitempty" format:"date-time"`
        CreatedTo    string `query:"created_to,omitempty" format:"date-time"`
        Limit        int    `query:"limit,omitempty" minimum:"1" maximum:"1000"`
        Cursor       string `query:"cursor,omitempty"`
}

type CreateSupportMessageRequest struct {
        Body string `json:"body" minLength:"1" maxLength:"10000"`
}

type CreateSupportMessageInput struct {
        PlatformSupportTicketPath
        PlatformCommandHeaders
        Body CreateSupportMessageRequest
}

type SupportMessageListInput struct {
        TicketID string `query:"ticket_id,omitempty"`
        Limit    int    `query:"limit,omitempty" minimum:"1" maximum:"1000"`
        Cursor   string `query:"cursor,omitempty"`
}

type StartSupportTicketInput struct {
        PlatformSupportTicketPath
        PlatformCommandHeaders
}

type ResolveSupportTicketInput struct {
        PlatformSupportTicketPath
        PlatformCommandHeaders
}

type CloseSupportTicketInput struct {
        PlatformSupportTicketPath
        PlatformCommandHeaders
}

// ----------------------------------------------------------------------------
// AI Provider Listing DTOs (Contract §85, §107)
// ----------------------------------------------------------------------------

type PlatformProviderView struct {
	ProviderID      string `json:"provider_id"`
	ProviderType    string `json:"provider_type"`
	DisplayName     string `json:"display_name"`
	AdminState      string `json:"admin_state"`
	HealthState     string `json:"health_state"`
	Configured      bool   `json:"configured"`
	Model           string `json:"model,omitempty"`
	LastHealthCheck *string `json:"last_health_check,omitempty"`
	LastSuccess     *string `json:"last_success,omitempty"`
	LastFailure     *string `json:"last_failure,omitempty"`
	LastFailureCode *string `json:"last_failure_code,omitempty"`
}

type PlatformProviderPathInput struct {
	ProviderID string `path:"provider_id"`
}

type PlatformProviderHealthCheckInput struct {
	PlatformProviderPathInput
	PlatformCommandHeaders
}

// ----------------------------------------------------------------------------
// Channel Operations DTOs (Contract §89-96, §107)
// ----------------------------------------------------------------------------

type PlatformChannelView struct {
	BusinessID           string  `json:"business_id"`
	ConnectionID         string  `json:"connection_id"`
	Channel              string  `json:"channel"`
	Provider             string  `json:"provider"`
	Status               string  `json:"status"`
	Health               string  `json:"health"`
	ProviderAccountRef   *string `json:"provider_account_ref,omitempty"`
	ProviderConnectionRef *string `json:"provider_connection_ref,omitempty"`
	LastHealthCheckAt    *string `json:"last_health_check_at,omitempty"`
	LastSuccess          *string `json:"last_success,omitempty"`
	LastFailure          *string `json:"last_failure,omitempty"`
	FailureCode          *string `json:"failure_code,omitempty"`
}

type PlatformChannelListInput struct {
	BusinessID string `query:"business_id,omitempty"`
	Status     string `query:"status,omitempty"`
	Channel    string `query:"channel,omitempty"`
	Limit      int    `query:"limit,omitempty" minimum:"1" maximum:"1000"`
}

type PlatformChannelPathInput struct {
	ConnectionID UUID `path:"connection_id" format:"uuid"`
}

type PlatformChannelHealthCheckInput struct {
	PlatformChannelPathInput
	PlatformCommandHeaders
}

// ----------------------------------------------------------------------------
// Platform-wide AI Usage DTOs (AIUsageTokenTelemetry.md §27, §34)
// ----------------------------------------------------------------------------

type PlatformAIUsageOverviewView struct {
	TotalAIReplies       int64 `json:"total_ai_replies"`
	TotalInputTokens     int64 `json:"total_input_tokens"`
	TotalCachedTokens    int64 `json:"total_cached_tokens"`
	TotalOutputTokens    int64 `json:"total_output_tokens"`
	TotalModelRequests   int   `json:"total_model_requests"`
	TotalToolCalls       int   `json:"total_tool_calls"`
	TotalProviderCostYER int   `json:"total_provider_cost_yer"`
	AverageCostPerReply  int   `json:"average_cost_per_reply"`
}

// ----------------------------------------------------------------------------
// Create Business DTOs (Contract §9, §13)
// ----------------------------------------------------------------------------

type CreateBusinessRequest struct {
	Name            string `json:"name" minLength:"1" maxLength:"256"`
	Slug            string `json:"slug" minLength:"1" maxLength:"128"`
	VerticalType    string `json:"vertical_type" enum:"retail,travel,services,restaurant,clinic,hospitality,education,real_estate,other"`
	Timezone        string `json:"timezone" default:"Asia/Aden"`
	DefaultCurrency string `json:"default_currency" minLength:"3" maxLength:"3"`
	Locale          string `json:"locale" minLength:"2" maxLength:"10"`
}

type CreateBusinessInput struct {
	PlatformCommandHeaders
	Body CreateBusinessRequest
}
