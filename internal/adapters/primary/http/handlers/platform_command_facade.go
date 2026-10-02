package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PlatformDeps holds the optional Platform-side dependencies. When nil, the
// corresponding /api/v1/platform/* route returns 501 NotImplemented (per
// Contract §57 "Error Contract" — the API exists in OpenAPI but is not
// wired). When set, the route executes the corresponding command/query
// handler with Platform Audit logging.
type PlatformDeps struct {
	// Lifecycle repositories
	Plans             ports.PlanRepository
	PlatformBusiness  ports.PlatformBusinessLifecyclePort
	PlatformAudit     ports.PlatformAuditRepository
	Subscriptions     ports.SubscriptionRepository
	Payments          ports.PaymentRepository
	Support           ports.SupportRepository
	AIUsage           ports.AIUsageRepository
	AIProviderPricing ports.AIProviderPricingRepository
	// Platform Operations (AI kill switch + provider/channel health) — in-memory
	Operations ports.PlatformOperationsPort
	// Channel Reader — platform-scoped read of channel_connections (no secrets)
	ChannelReader ports.PlatformChannelReadPort
	// Per §1-12: AI Provider Configuration management
	AIConfigRepo   ports.AIProviderConfigService
	AIConfigCache  *services.AIConfigurationCache
	ModelDiscovery ports.ModelDiscoveryClient
	AssignBusinessOwner *services.AssignBusinessOwnerService
}

// WithPlatformDeps is the explicit setter for Platform-side dependencies.
// Per Platform Administration Contract §55: the Platform middleware
// (RequirePlatformAdminHuma) enforces Platform Scope independently —
// wiring these deps does NOT bypass merchant authorization.
func (s *Server) WithPlatformDeps(deps PlatformDeps) *Server {
	s.platformDeps = deps
	return s
}

// dispatchPlatformCommand is the entry point for /api/v1/platform/* operations.
// Returns (result, handled). When handled=true, the dispatcher short-circuits.
//
// Per Platform Administration Contract §55: every platform command requires
//  1. Authenticated (handled upstream by RequireAccessTokenHuma)
//  2. Platform principal (verified via middleware.RequirePlatformAdminHuma)
//  3. Platform scope (set in context via middleware.WithPlatformAdmin)
//
// Step 3 is verified here via middleware.PlatformAdminID(ctx). If the request
// reached this code path WITHOUT Platform Scope set, we return 403 — this
// should never happen if RequirePlatformAdminHuma is correctly wired, but
// defense-in-depth: never trust the absence of a check.
func (s *Server) dispatchPlatformCommand(ctx context.Context, operationID string, input any) (any, bool) {
	if !strings.HasPrefix(operationID, "platform") {
		return nil, false
	}
	platformAdminID, ok := middleware.PlatformAdminID(ctx)
	if !ok {
		return mapApplicationError(appErrors.New(appErrors.CodeForbidden, "platform super admin scope required")), true
	}
	_ = platformAdminID

	// Per Contract §59: enforce Idempotency-Key for the 4 operations that
	// the contract explicitly requires: Business creation, Subscription
	// creation, Payment recording, Support message creation. A retry with
	// the same Idempotency-Key returns the cached result — no duplicate.
	idempotencyKey := extractIdempotencyKey(operationID, input)
	if idempotencyKey != "" && s.idempotency != nil {
		cacheKey := makeIdempotencyKey(operationID, idempotencyKey)
		if cachedResult, cachedErr, found := s.idempotency.Get(cacheKey); found {
			if cachedErr != nil {
				return cachedErr, true
			}
			return cachedResult, true
		}
		// Execute the operation, then cache the result ONLY on success.
		// Per Contract §59: retry must not produce a duplicate. If the first
		// request fails, the cache is NOT populated — the caller can retry
		// with the same key without getting a cached error.
		result, handled := s.dispatchPlatformCommandInner(ctx, operationID, input)
		if handled {
			// Only cache non-error results. An error result (which implements
			// the error interface via *dashboardHTTPError) means the operation
			// did NOT succeed — caching it would block legitimate retries.
			if _, isErr := result.(error); !isErr {
				s.idempotency.Set(cacheKey, result, nil)
			}
		}
		return result, handled
	}

	return s.dispatchPlatformCommandInner(ctx, operationID, input)
}

// extractIdempotencyKey pulls the Idempotency-Key from the input struct
// for the 4 operations that require idempotency per Contract §59.
// Returns "" if the operation doesn't require idempotency or the key is absent.
func extractIdempotencyKey(operationID string, input any) string {
	if input == nil {
		return ""
	}
	switch operationID {
	case "platformCreateBusiness":
		if in, ok := input.(*dto.CreateBusinessInput); ok {
			return in.IdempotencyKey
		}
	case "platformCreateSubscription":
		if in, ok := input.(*dto.CreateSubscriptionInput); ok {
			return in.IdempotencyKey
		}
	case "platformRecordPayment":
		if in, ok := input.(*dto.RecordPaymentInput); ok {
			return in.IdempotencyKey
		}
	case "platformCreateSupportMessage":
		if in, ok := input.(*dto.CreateSupportMessageInput); ok {
			return in.IdempotencyKey
		}
	}
	return ""
}

// dispatchPlatformCommandInner is the original switch statement, extracted
// so the idempotency wrapper can call it.
func (s *Server) dispatchPlatformCommandInner(ctx context.Context, operationID string, input any) (any, bool) {
	switch operationID {
	// ---- Plan Management (Contract §19) ----
	case "platformCreatePlan":
		return s.platformCreatePlan(ctx, input.(*dto.CreatePlanInput))
	case "platformListPlans":
		return s.platformListPlans(ctx, input.(*dto.PlanListInput))
	case "platformGetPlan":
		return s.platformGetPlan(ctx, input.(*dto.PlatformPlanPath))
	case "platformActivatePlan":
		return s.platformActivatePlan(ctx, input.(*dto.PlanActivateInput))
	case "platformRetirePlan":
		return s.platformRetirePlan(ctx, input.(*dto.PlanRetireInput))
	case "platformCreatePlanVersion":
		return s.platformCreatePlanVersion(ctx, input.(*dto.CreatePlanVersionInput))

	// ---- Business Management (Contract §13-14) ----
	case "platformCreateBusiness":
		return s.platformCreateBusiness(ctx, input.(*dto.CreateBusinessInput))
case "platformAssignBusinessOwner":
		return s.platformAssignBusinessOwner(ctx, input.(*dto.AssignBusinessOwnerInput))
	case "platformListBusinesses":
		return s.platformListBusinesses(ctx, input.(*dto.PlatformBusinessListInput))
	case "platformGetBusiness":
		return s.platformGetBusiness(ctx, input.(*dto.PlatformBusinessPath))
	case "platformSuspendBusiness":
		return s.platformSuspendBusiness(ctx, input.(*dto.PlatformBusinessSuspendInput))
	case "platformReactivateBusiness":
		return s.platformReactivateBusiness(ctx, input.(*dto.PlatformBusinessReactivateInput))
	case "platformArchiveBusiness":
		return s.platformArchiveBusiness(ctx, input.(*dto.PlatformBusinessArchiveInput))

	// ---- Platform Audit (Contract §49) ----
	case "platformListAuditEvents":
		return s.platformListAuditEvents(ctx, input.(*dto.PlatformAuditListInput))
	case "platformGetAuditEvent":
		return s.platformGetAuditEvent(ctx, input.(*dto.PlatformAuditEventPathInput))

	// ---- AI Operations (Contract §75-108) ----
	case "platformGetAIOverview":
		return s.platformGetAIOverview(ctx)
	case "platformAIDisable":
		return s.platformAIDisable(ctx, input.(*dto.PlatformAIRuntimeDisableInput))
	case "platformAIEnable":
		return s.platformAIEnable(ctx, input.(*dto.PlatformAIRuntimeEnableInput))
	case "platformAIHealthCheck":
		return s.platformAIHealthCheck(ctx, input.(*dto.PlatformAIHealthCheckInput))
	case "platformGetSubscriptionAIUsage":
		return s.platformGetSubscriptionAIUsage(ctx, input.(*dto.PlatformSubscriptionPath))
	case "platformOverrideSubscriptionAICostBudget":
		return s.platformOverrideSubscriptionAICostBudget(ctx, input.(*dto.SubscriptionAICostBudgetOverrideInput))
	// ---- AI Provider Listing (Contract §85, §107) ----
	case "platformListAIProviders":
		return s.platformListAIProviders(ctx)
	case "platformGetAIProvider":
		return s.platformGetAIProvider(ctx, input.(*dto.PlatformProviderPathInput))
	// ---- Channel Operations (Contract §89-96, §107) ----
	case "platformListChannels":
		return s.platformListChannels(ctx, input.(*dto.PlatformChannelListInput))
	case "platformGetChannel":
		return s.platformGetChannel(ctx, input.(*dto.PlatformChannelPathInput))
	case "platformChannelHealthCheck":
		return s.platformChannelHealthCheck(ctx, input.(*dto.PlatformChannelHealthCheckInput))
	// ---- Provider Operations (Contract §97-98, §107) ----
	case "platformListProviders":
		return s.platformListProviders(ctx)
	case "platformGetProvider":
		return s.platformGetProvider(ctx, input.(*dto.PlatformProviderPathInput))
	case "platformProviderHealthCheck":
		return s.platformProviderHealthCheck(ctx, input.(*dto.PlatformProviderHealthCheckInput))
	// ---- Platform-wide AI Usage (AIUsageTokenTelemetry.md §27, §34) ----
	case "platformGetAIUsageOverview":
		return s.platformGetAIUsageOverview(ctx)
	case "platformGetAIUsageBySubscription":
		return s.platformGetAIUsageBySubscription(ctx, input.(*dto.SubscriptionListInput))
	case "platformGetAIUsageByBusiness":
		return s.platformGetAIUsageByBusiness(ctx, input.(*dto.PlatformBusinessListInput))

	// ---- AI Provider Configuration (§13) ----
	case "platformListAIProvidersConfig":
		return s.platformListAIProvidersConfig(ctx)
	case "platformAddAICredential":
		return s.platformAddAICredential(ctx, input.(*dto.AddCredentialInput))
	case "platformTestAIConnection":
		return s.platformTestAIConnection(ctx, input.(*dto.TestConnectionInput))
	case "platformDiscoverAIModels":
		return s.platformDiscoverAIModels(ctx, input.(*dto.DiscoverModelsInput))
	case "platformGetAIConfiguration":
		return s.platformGetAIConfiguration(ctx)
	case "platformUpdateAIConfiguration":
		return s.platformUpdateAIConfiguration(ctx, input.(*dto.UpdateConfigurationInput))

	// ---- Subscription Lifecycle (Contract §20-36) ----
	case "platformListSubscriptions":
		return s.platformListSubscriptions(ctx, input.(*dto.SubscriptionListInput))
	case "platformGetSubscription":
		return s.platformGetSubscription(ctx, input.(*dto.PlatformSubscriptionPath))
	case "platformCreateSubscription":
		return s.platformCreateSubscription(ctx, input.(*dto.CreateSubscriptionInput))
	case "platformCancelSubscription":
		return s.platformCancelSubscription(ctx, input.(*dto.CancelSubscriptionInput))

	// ---- Manual Payment Recording (Contract §25-28) ----
	case "platformRecordPayment":
		return s.platformRecordPayment(ctx, input.(*dto.RecordPaymentInput))
	case "platformListPayments":
		return s.platformListPayments(ctx, input.(*dto.PaymentListInput))

	// ---- Support Management (Contract §37-44) ----
	case "platformCreateSupportTicket":
		return s.platformCreateSupportTicket(ctx, input.(*dto.CreateSupportTicketInput))
	case "platformListSupportTickets":
		return s.platformListSupportTickets(ctx, input.(*dto.SupportTicketListInput))
	case "platformGetSupportTicket":
		return s.platformGetSupportTicket(ctx, input.(*dto.PlatformSupportTicketPath))
	case "platformCreateSupportMessage":
		return s.platformCreateSupportMessage(ctx, input.(*dto.CreateSupportMessageInput))
	case "platformStartSupportTicket":
		return s.platformStartSupportTicket(ctx, input.(*dto.StartSupportTicketInput))
	case "platformResolveSupportTicket":
		return s.platformResolveSupportTicket(ctx, input.(*dto.ResolveSupportTicketInput))
	case "platformCloseSupportTicket":
		return s.platformCloseSupportTicket(ctx, input.(*dto.CloseSupportTicketInput))
	}
	return nil, false
}

// ----------------------------------------------------------------------------
// Helpers — audit append + projections
// ----------------------------------------------------------------------------

func (s *Server) appendPlatformAudit(ctx context.Context, action, targetType string, targetID, businessID *string, result, failureCode string, metadata map[string]any) {
	if s.platformDeps.PlatformAudit == nil {
		return
	}
	adminID, _ := middleware.PlatformAdminID(ctx)
	var adminIDPtr *string
	if adminID != "" {
		s := string(adminID)
		adminIDPtr = &s
	}
	var metadataBytes []byte
	if metadata == nil {
		metadataBytes = []byte(`{}`)
	} else {
		if encoded, err := json.Marshal(metadata); err == nil {
			metadataBytes = encoded
		} else {
			metadataBytes = []byte(`{}`)
		}
	}
	_, _ = s.platformDeps.PlatformAudit.Append(ctx, ports.PlatformAuditDraft{
		ActorPlatformAdminID: adminIDPtr,
		Action:               action,
		TargetType:           targetType,
		TargetID:             targetID,
		BusinessID:           businessID,
		Result:               result,
		FailureCode:          &failureCode,
		OccurredAt:           time.Now().UTC(),
		Metadata:             metadataBytes,
	})
}

func planProjection(p ports.PlanRecord) dto.PlanView {
	view := dto.PlanView{
		ID:                      p.ID,
		Code:                    p.Code,
		Version:                 p.Version,
		DisplayName:             p.DisplayName,
		PriceYER:                p.PriceYER,
		BillingInterval:         p.BillingInterval,
		AIReplyLimit:            p.AIReplyLimit,
		AICatalogLimit:          p.AICatalogLimit,
		ChannelLimit:            p.ChannelLimit,
		InternalAICostBudgetYER: p.InternalAICostBudgetYER,
		Status:                  p.Status,
		CreatedAt:               p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:               p.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if p.RetiredAt != nil {
		formatted := p.RetiredAt.UTC().Format(time.RFC3339)
		view.RetiredAt = &formatted
	}
	return view
}

func platformBusinessProjection(b ports.PlatformBusinessRecord) dto.PlatformBusinessView {
	return dto.PlatformBusinessView{
		ID:                   b.ID,
		Name:                 b.Name,
		Slug:                 b.Slug,
		PlatformStatus:       strings.ToUpper(b.PlatformStatus),
		OwnerIdentitySummary: b.OwnerIdentitySummary,
		SubscriptionSummary:  b.SubscriptionSummary,
		CreatedAt:            b.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:            b.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func platformAuditProjection(e ports.PlatformAuditEvent) dto.PlatformAuditEventView {
	view := dto.PlatformAuditEventView{
		ID:                   e.ID,
		ActorPlatformAdminID: e.ActorPlatformAdminID,
		Action:               e.Action,
		TargetType:           e.TargetType,
		TargetID:             e.TargetID,
		BusinessID:           e.BusinessID,
		Result:               e.Result,
		FailureCode:          e.FailureCode,
		CorrelationID:        e.CorrelationID,
		OccurredAt:           e.OccurredAt.UTC().Format(time.RFC3339),
		Metadata:             map[string]any{},
	}
	if len(e.Metadata) > 0 {
		_ = json.Unmarshal(e.Metadata, &view.Metadata)
	}
	return view
}

// ----------------------------------------------------------------------------
// Plan command/query façades
// ----------------------------------------------------------------------------

func (s *Server) platformCreatePlan(ctx context.Context, in *dto.CreatePlanInput) (any, bool) {
	if s.platformDeps.Plans == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "plan repository is not wired")), true
	}
	record, err := s.platformDeps.Plans.Create(ctx, ports.PlanCreate{
		ID:                      uuid.NewString(),
		Code:                    in.Body.Code,
		Version:                 1, // Per Contract §17: new code starts at v1
		DisplayName:             in.Body.DisplayName,
		PriceYER:                in.Body.PriceYER,
		BillingInterval:         in.Body.BillingInterval,
		AIReplyLimit:            in.Body.AIReplyLimit,
		AICatalogLimit:          in.Body.AICatalogLimit,
		ChannelLimit:            in.Body.ChannelLimit,
		InternalAICostBudgetYER: in.Body.InternalAICostBudgetYER,
		Now:                     time.Now().UTC(),
	})
	if err != nil {
		result := "FAILURE"
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "plan.created", "plan", &record.ID, nil, result, failureCode, map[string]any{"code": in.Body.Code})
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "plan.created", "plan", &record.ID, nil, "SUCCESS", "", map[string]any{
		"code": record.Code, "version": record.Version, "price_yer": record.PriceYER,
	})
	out := &contract.Single[dto.PlanView]{}
	out.Body.Data = planProjection(record)
	return out, true
}

func (s *Server) platformListPlans(ctx context.Context, in *dto.PlanListInput) (any, bool) {
	if s.platformDeps.Plans == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "plan repository is not wired")), true
	}
	page, err := s.platformDeps.Plans.List(ctx, ports.PlanListFilter{
		Status: in.Status,
		Code:   in.Code,
		Limit:  in.Limit,
		Cursor: in.Cursor,
	})
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.List[dto.PlanView]{}
	items := make([]dto.PlanView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, planProjection(record))
	}
	out.Body.Data = items
	out.Body.Pagination = contract.Page{HasMore: page.HasMore, NextCursor: platformCursorPtr(page.NextCursor)}
	return out, true
}

func (s *Server) platformGetPlan(ctx context.Context, in *dto.PlatformPlanPath) (any, bool) {
	if s.platformDeps.Plans == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "plan repository is not wired")), true
	}
	record, err := s.platformDeps.Plans.GetByID(ctx, string(in.PlanID))
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[dto.PlanView]{}
	out.Body.Data = planProjection(record)
	return out, true
}

func (s *Server) platformActivatePlan(ctx context.Context, in *dto.PlanActivateInput) (any, bool) {
	if s.platformDeps.Plans == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "plan repository is not wired")), true
	}
	planIDStr := string(in.PlanID)
	record, err := s.platformDeps.Plans.Activate(ctx, planIDStr, time.Now().UTC())
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "plan.activated", "plan", &planIDStr, nil, "FAILURE", failureCode, nil)
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "plan.activated", "plan", &record.ID, nil, "SUCCESS", "", nil)
	out := &contract.Single[dto.PlanView]{}
	out.Body.Data = planProjection(record)
	return out, true
}

func (s *Server) platformRetirePlan(ctx context.Context, in *dto.PlanRetireInput) (any, bool) {
	if s.platformDeps.Plans == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "plan repository is not wired")), true
	}
	planIDStr := string(in.PlanID)
	record, err := s.platformDeps.Plans.Retire(ctx, planIDStr, time.Now().UTC())
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "plan.retired", "plan", &planIDStr, nil, "FAILURE", failureCode, nil)
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "plan.retired", "plan", &record.ID, nil, "SUCCESS", "", nil)
	out := &contract.Single[dto.PlanView]{}
	out.Body.Data = planProjection(record)
	return out, true
}

// platformCreatePlanVersion implements Contract §17 + AIUsageTokenTelemetry.md
// §22-23. Per the contract:
//   - The base plan row is NEVER edited.
//   - A new plan row is inserted with the SAME code + incremented version.
//   - The base plan is atomically transitioned to RETIRED.
//   - The new version is ACTIVE (only ACTIVE plans accept new subscriptions).
//   - Subscriptions already on the old version keep their snapshot of limits.
//
// Per Contract §46 audit: plan.version_created + the per-field changes
// (price/limits/budget) are recorded in metadata for forensics.
//
// Per AIUsageTokenTelemetry.md §36: plan.ai_reply_limit_changed +
// plan.internal_ai_cost_budget_changed are audited when those specific
// fields differ from the base version.
func (s *Server) platformCreatePlanVersion(ctx context.Context, in *dto.CreatePlanVersionInput) (any, bool) {
	if s.platformDeps.Plans == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "plan repository is not wired")), true
	}
	if strings.TrimSpace(in.Body.DisplayName) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "display_name is required")), true
	}
	if in.Body.PriceYER <= 0 || in.Body.AIReplyLimit <= 0 || in.Body.ChannelLimit <= 0 || in.Body.InternalAICostBudgetYER <= 0 {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "price, ai_reply_limit, channel_limit, and internal_ai_cost_budget must be positive")), true
	}
	if in.Body.AICatalogLimit < 0 {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "ai_catalog_limit must be non-negative")), true
	}
	if strings.TrimSpace(in.Body.Reason) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "reason is required")), true
	}
	adminID, _ := middleware.PlatformAdminID(ctx)
	planIDStr := string(in.PlanID)
	// Fetch the base plan so we can compare fields + emit per-change audit
	// events (Contract §36).
	base, err := s.platformDeps.Plans.GetByID(ctx, planIDStr)
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "plan.version_created", "plan", &planIDStr, nil, "FAILURE", failureCode, map[string]any{"reason": "base plan lookup failed"})
		return mapApplicationError(err), true
	}
	if base.Status != "ACTIVE" {
		s.appendPlatformAudit(ctx, "plan.version_created", "plan", &planIDStr, nil, "FAILURE", "BASE_PLAN_NOT_ACTIVE", map[string]any{"base_status": base.Status})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict, "base plan must be ACTIVE to create a new version")), true
	}
	newRecord, err := s.platformDeps.Plans.CreateVersion(ctx, ports.PlanVersionCreate{
		BasePlanID:              base.ID,
		DisplayName:             in.Body.DisplayName,
		PriceYER:                in.Body.PriceYER,
		BillingInterval:         in.Body.BillingInterval,
		AIReplyLimit:            in.Body.AIReplyLimit,
		AICatalogLimit:          in.Body.AICatalogLimit,
		ChannelLimit:            in.Body.ChannelLimit,
		InternalAICostBudgetYER: in.Body.InternalAICostBudgetYER,
		ChangedBy:               string(adminID),
		Reason:                  in.Body.Reason,
		Now:                     time.Now().UTC(),
	})
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "plan.version_created", "plan", &planIDStr, nil, "FAILURE", failureCode, map[string]any{"reason": in.Body.Reason})
		return mapApplicationError(err), true
	}
	// Audit the new version + emit per-change audit events when relevant
	// fields differ from the base (Contract §36).
	metadata := map[string]any{
		"base_plan_id": base.ID, "base_version": base.Version,
		"new_plan_id": newRecord.ID, "new_version": newRecord.Version,
		"reason": in.Body.Reason,
	}
	if base.PriceYER != newRecord.PriceYER {
		metadata["price_changed"] = map[string]int{"from": base.PriceYER, "to": newRecord.PriceYER}
	}
	if base.AIReplyLimit != newRecord.AIReplyLimit {
		s.appendPlatformAudit(ctx, "plan.ai_reply_limit_changed", "plan", &newRecord.ID, nil, "SUCCESS", "", map[string]any{
			"from": base.AIReplyLimit, "to": newRecord.AIReplyLimit, "reason": in.Body.Reason,
		})
	}
	if base.InternalAICostBudgetYER != newRecord.InternalAICostBudgetYER {
		s.appendPlatformAudit(ctx, "plan.internal_ai_cost_budget_changed", "plan", &newRecord.ID, nil, "SUCCESS", "", map[string]any{
			"from": base.InternalAICostBudgetYER, "to": newRecord.InternalAICostBudgetYER, "reason": in.Body.Reason,
		})
	}
	s.appendPlatformAudit(ctx, "plan.version_created", "plan", &newRecord.ID, nil, "SUCCESS", "", metadata)
	out := &contract.Single[dto.PlanView]{}
	out.Body.Data = planProjection(newRecord)
	return out, true
}

// ----------------------------------------------------------------------------
func (s *Server) platformAssignBusinessOwner(ctx context.Context, in *dto.AssignBusinessOwnerInput) (any, bool) {
	if s.platformDeps.AssignBusinessOwner == nil {
		return mapApplicationError(appErrors.NotImplemented()), true
	}
	result, err := s.platformDeps.AssignBusinessOwner.Handle(ctx, services.AssignBusinessOwnerInput{
		BusinessID:  string(in.BusinessID),
		Email:       in.Body.Email,
		DisplayName: in.Body.DisplayName,
		Password:    in.Body.Password,
		Now:         time.Now().UTC(),
	})
	if err != nil {
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "business.owner_assigned", "business", &result.BusinessID, &result.BusinessID, "SUCCESS", "", map[string]any{
		"principal_id": result.PrincipalID,
		"role":         result.Role,
	})
	out := &contract.Single[dto.AssignBusinessOwnerView]{}
	out.Body.Data = dto.AssignBusinessOwnerView{
		PrincipalID:    result.PrincipalID,
		Email:          result.Email,
		DisplayName:    result.DisplayName,
		Role:           result.Role,
		BusinessStatus: result.BusinessStatus,
	}
	return out, true
}

// Business lifecycle façades
// ----------------------------------------------------------------------------

func (s *Server) platformListBusinesses(ctx context.Context, in *dto.PlatformBusinessListInput) (any, bool) {
	if s.platformDeps.PlatformBusiness == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform business repository is not wired")), true
	}
	filter := ports.PlatformBusinessListFilter{
		Search: in.Search,
		Status: in.Status,
		Limit:  in.Limit,
	}
	if from, err := parseOptionalRFC3339(in.CreatedFrom); err == nil {
		filter.CreatedFrom = from
	}
	if to, err := parseOptionalRFC3339(in.CreatedTo); err == nil {
		filter.CreatedTo = to
	}
	page, err := s.platformDeps.PlatformBusiness.List(ctx, filter)
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.List[dto.PlatformBusinessView]{}
	items := make([]dto.PlatformBusinessView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, platformBusinessProjection(record))
	}
	out.Body.Data = items
	out.Body.Pagination = contract.Page{HasMore: page.HasMore, NextCursor: platformCursorPtr(page.NextCursor)}
	return out, true
}

func (s *Server) platformGetBusiness(ctx context.Context, in *dto.PlatformBusinessPath) (any, bool) {
	if s.platformDeps.PlatformBusiness == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform business repository is not wired")), true
	}
	record, err := s.platformDeps.PlatformBusiness.GetByID(ctx, string(in.BusinessID))
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[dto.PlatformBusinessView]{}
	out.Body.Data = platformBusinessProjection(record)
	return out, true
}

func (s *Server) platformSuspendBusiness(ctx context.Context, in *dto.PlatformBusinessSuspendInput) (any, bool) {
	if s.platformDeps.PlatformBusiness == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform business repository is not wired")), true
	}
	record, err := s.platformDeps.PlatformBusiness.Suspend(ctx, string(in.BusinessID), time.Now().UTC())
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		bid := string(in.BusinessID)
		s.appendPlatformAudit(ctx, "business.suspended", "business", &bid, &bid, "FAILURE", failureCode, nil)
		return mapApplicationError(err), true
	}
	bid := record.ID
	s.appendPlatformAudit(ctx, "business.suspended", "business", &bid, &bid, "SUCCESS", "", nil)
	out := &contract.Single[dto.PlatformBusinessView]{}
	out.Body.Data = platformBusinessProjection(record)
	return out, true
}

func (s *Server) platformReactivateBusiness(ctx context.Context, in *dto.PlatformBusinessReactivateInput) (any, bool) {
	if s.platformDeps.PlatformBusiness == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform business repository is not wired")), true
	}
	record, err := s.platformDeps.PlatformBusiness.Reactivate(ctx, string(in.BusinessID), time.Now().UTC())
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		bid := string(in.BusinessID)
		s.appendPlatformAudit(ctx, "business.reactivated", "business", &bid, &bid, "FAILURE", failureCode, nil)
		return mapApplicationError(err), true
	}
	bid := record.ID
	s.appendPlatformAudit(ctx, "business.reactivated", "business", &bid, &bid, "SUCCESS", "", nil)
	out := &contract.Single[dto.PlatformBusinessView]{}
	out.Body.Data = platformBusinessProjection(record)
	return out, true
}

func (s *Server) platformArchiveBusiness(ctx context.Context, in *dto.PlatformBusinessArchiveInput) (any, bool) {
	if s.platformDeps.PlatformBusiness == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform business repository is not wired")), true
	}
	record, err := s.platformDeps.PlatformBusiness.Archive(ctx, string(in.BusinessID), time.Now().UTC())
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		bid := string(in.BusinessID)
		s.appendPlatformAudit(ctx, "business.archived", "business", &bid, &bid, "FAILURE", failureCode, nil)
		return mapApplicationError(err), true
	}
	bid := record.ID
	s.appendPlatformAudit(ctx, "business.archived", "business", &bid, &bid, "SUCCESS", "", nil)
	out := &contract.Single[dto.PlatformBusinessView]{}
	out.Body.Data = platformBusinessProjection(record)
	return out, true
}

// ----------------------------------------------------------------------------
// Platform Audit façades
// ----------------------------------------------------------------------------

func (s *Server) platformListAuditEvents(ctx context.Context, in *dto.PlatformAuditListInput) (any, bool) {
	if s.platformDeps.PlatformAudit == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform audit repository is not wired")), true
	}
	filter := ports.PlatformAuditFilter{
		Action:     in.Action,
		TargetType: in.TargetType,
		TargetID:   in.TargetID,
		BusinessID: in.BusinessID,
		Result:     in.Result,
		Limit:      in.Limit,
		Cursor:     in.Cursor,
	}
	if from, err := parseOptionalRFC3339(in.OccurredFrom); err == nil {
		filter.OccurredFrom = from
	}
	if to, err := parseOptionalRFC3339(in.OccurredTo); err == nil {
		filter.OccurredTo = to
	}
	page, err := s.platformDeps.PlatformAudit.List(ctx, filter)
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.List[dto.PlatformAuditEventView]{}
	items := make([]dto.PlatformAuditEventView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, platformAuditProjection(record))
	}
	out.Body.Data = items
	out.Body.Pagination = contract.Page{HasMore: page.HasMore, NextCursor: platformCursorPtr(page.NextCursor)}
	return out, true
}

func (s *Server) platformGetAuditEvent(ctx context.Context, in *dto.PlatformAuditEventPathInput) (any, bool) {
	if s.platformDeps.PlatformAudit == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform audit repository is not wired")), true
	}
	record, err := s.platformDeps.PlatformAudit.GetByID(ctx, in.EventID)
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[dto.PlatformAuditEventView]{}
	out.Body.Data = platformAuditProjection(record)
	return out, true
}

// ----------------------------------------------------------------------------
// Error classifier — translates RepositoryError kinds into audit failure codes
// ----------------------------------------------------------------------------

func classifyPlatformRepoErrorKind(err error) string {
	if err == nil {
		return ""
	}
	var repoErr interface{ ErrorKind() string }
	if errors.As(err, &repoErr) {
		switch repoErr.ErrorKind() {
		case "not_found":
			return "NOT_FOUND"
		case "conflict":
			return "CONFLICT"
		case "stale":
			return "STALE_VERSION"
		case "invalid":
			return "INVALID_INPUT"
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "NOT_FOUND"
	}
	return "INTERNAL_ERROR"
}

// platformCursorPtr converts an empty cursor string to nil (so the JSON
// next_cursor field is omitted) — matches dto.Page.NextCursor *string shape.
func platformCursorPtr(cursor string) *string {
	if strings.TrimSpace(cursor) == "" {
		return nil
	}
	return &cursor
}

// parseOptionalRFC3339 parses an RFC 3339 timestamp string. Returns nil if
// the input is empty. Returns an error if the input is non-empty but not a
// valid RFC 3339 timestamp — callers typically ignore the error and skip
// the filter (treat invalid input as "no filter").
func parseOptionalRFC3339(value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	utc := parsed.UTC()
	return &utc, nil
}

// compile-time assertion that the contract DTO imports remain in sync
var _ = fmt.Sprintf

// ----------------------------------------------------------------------------
// Projections — Subscription + Payment
// ----------------------------------------------------------------------------

func subscriptionProjection(s ports.SubscriptionRecord) dto.SubscriptionView {
	view := dto.SubscriptionView{
		ID:                       s.ID,
		BusinessID:               s.BusinessID,
		PlanID:                   s.PlanID,
		PlanCode:                 s.PlanCode,
		PlanVersion:              s.PlanVersion,
		PeriodStart:              s.PeriodStart.UTC().Format(time.RFC3339),
		PeriodEnd:                s.PeriodEnd.UTC().Format(time.RFC3339),
		Status:                   s.Status,
		AIReplyLimit:             s.AIReplyLimit,
		AICatalogLimit:           s.AICatalogLimit,
		ChannelLimit:             s.ChannelLimit,
		InternalAICostBudgetYER:  s.InternalAICostBudgetYER,
		CostBudgetOverrideYER:    s.CostBudgetOverrideYER,
		CostBudgetOverrideReason: s.CostBudgetOverrideReason,
		CreatedAt:                s.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:                s.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if s.CancelledAt != nil {
		formatted := s.CancelledAt.UTC().Format(time.RFC3339)
		view.CancelledAt = &formatted
	}
	view.CancelledReason = s.CancelledReason
	return view
}

func paymentProjection(p ports.PaymentRecord) dto.PaymentView {
	return dto.PaymentView{
		ID:             p.ID,
		SubscriptionID: p.SubscriptionID,
		BusinessID:     p.BusinessID,
		AmountYER:      p.AmountYER,
		Method:         p.Method,
		Reference:      p.Reference,
		PaidAt:         p.PaidAt.UTC().Format(time.RFC3339),
		RecordedBy:     p.RecordedBy,
		CreatedAt:      p.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// ----------------------------------------------------------------------------
// Subscription façades
// ----------------------------------------------------------------------------

func (s *Server) platformListSubscriptions(ctx context.Context, in *dto.SubscriptionListInput) (any, bool) {
	if s.platformDeps.Subscriptions == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "subscription repository is not wired")), true
	}
	page, err := s.platformDeps.Subscriptions.List(ctx, ports.SubscriptionListFilter{
		BusinessID: in.BusinessID,
		Status:     in.Status,
		Limit:      in.Limit,
		Cursor:     in.Cursor,
	})
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.List[dto.SubscriptionView]{}
	items := make([]dto.SubscriptionView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, subscriptionProjection(record))
	}
	out.Body.Data = items
	out.Body.Pagination = contract.Page{HasMore: page.HasMore, NextCursor: platformCursorPtr(page.NextCursor)}
	return out, true
}

func (s *Server) platformGetSubscription(ctx context.Context, in *dto.PlatformSubscriptionPath) (any, bool) {
	if s.platformDeps.Subscriptions == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "subscription repository is not wired")), true
	}
	record, err := s.platformDeps.Subscriptions.GetByID(ctx, string(in.SubscriptionID))
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[dto.SubscriptionView]{}
	out.Body.Data = subscriptionProjection(record)
	return out, true
}

// platformCreateSubscription implements the create + initial-state flow per
// Contract §20, §22, §28:
//   - New subscription_id (no reuse of old rows per §22).
//   - Period = calendar month (§23).
//   - Plan limits snapshot at creation (§17) — never mutated by plan edits.
//   - New subscription is in PENDING status. Activation requires payment (§28).
//
// Per Contract §34 + §35: catalog/channel overrun detection is TODO for V1+
// — the contract requires "no silent data deletion" but the current merchant
// side has no AI-active catalog count query; the check is deferred until the
// subscription activation flow is wired to merchant entitlements. For now
// we just snapshot the plan's limits.
func (s *Server) platformCreateSubscription(ctx context.Context, in *dto.CreateSubscriptionInput) (any, bool) {
	if s.platformDeps.Subscriptions == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "subscription repository is not wired")), true
	}
	if s.platformDeps.Plans == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "plan repository is not wired")), true
	}
	planIDStr := strings.TrimSpace(string(in.Body.PlanID))
	if planIDStr == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "plan_id is required")), true
	}
	plan, err := s.platformDeps.Plans.GetByID(ctx, planIDStr)
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "subscription.created", "subscription", nil, nullableStringPtr(string(in.BusinessID)), "FAILURE", failureCode, map[string]any{"plan_id": planIDStr, "reason": "plan lookup failed"})
		return mapApplicationError(err), true
	}
	if plan.Status != "ACTIVE" {
		s.appendPlatformAudit(ctx, "subscription.created", "subscription", nil, nullableStringPtr(string(in.BusinessID)), "FAILURE", "PLAN_NOT_ACTIVE", map[string]any{"plan_id": planIDStr, "plan_status": plan.Status})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict, "plan is not ACTIVE — cannot create subscription")), true
	}
	now := time.Now().UTC()
	periodStart := now
	periodEnd := periodStart.AddDate(0, 1, 0) // +1 calendar month
	record, err := s.platformDeps.Subscriptions.Create(ctx, ports.SubscriptionCreate{
		ID:                      uuid.NewString(),
		BusinessID:              string(in.BusinessID),
		PlanID:                  plan.ID,
		PeriodStart:             periodStart,
		PeriodEnd:               periodEnd,
		AIReplyLimit:            plan.AIReplyLimit,
		AICatalogLimit:          plan.AICatalogLimit,
		ChannelLimit:            plan.ChannelLimit,
		InternalAICostBudgetYER: plan.InternalAICostBudgetYER,
		Now:                     now,
	})
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "subscription.created", "subscription", nil, nullableStringPtr(string(in.BusinessID)), "FAILURE", failureCode, map[string]any{"plan_id": planIDStr})
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "subscription.created", "subscription", &record.ID, &record.BusinessID, "SUCCESS", "", map[string]any{
		"plan_id": record.PlanID, "plan_code": record.PlanCode, "plan_version": record.PlanVersion,
	})
	out := &contract.Single[dto.SubscriptionView]{}
	out.Body.Data = subscriptionProjection(record)
	return out, true
}

func (s *Server) platformCancelSubscription(ctx context.Context, in *dto.CancelSubscriptionInput) (any, bool) {
	if s.platformDeps.Subscriptions == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "subscription repository is not wired")), true
	}
	if strings.TrimSpace(in.Body.Reason) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "cancellation reason is required")), true
	}
	adminID, _ := middleware.PlatformAdminID(ctx)
	record, err := s.platformDeps.Subscriptions.Cancel(ctx, string(in.SubscriptionID), in.Body.Reason, string(adminID), time.Now().UTC())
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		subID := string(in.SubscriptionID)
		s.appendPlatformAudit(ctx, "subscription.cancelled", "subscription", &subID, nil, "FAILURE", failureCode, map[string]any{"reason": in.Body.Reason})
		return mapApplicationError(err), true
	}
	subID := record.ID
	s.appendPlatformAudit(ctx, "subscription.cancelled", "subscription", &subID, &record.BusinessID, "SUCCESS", "", map[string]any{"reason": in.Body.Reason})
	out := &contract.Single[dto.SubscriptionView]{}
	out.Body.Data = subscriptionProjection(record)
	return out, true
}

// ----------------------------------------------------------------------------
// Payment façades — Contract §25-28 (Manual Payment + Activation)
// ----------------------------------------------------------------------------

// platformRecordPayment implements Contract §28 activation flow:
//  1. Verify subscription exists (and belongs to the path's business_id).
//  2. Append a PaymentRecord (append-only per §27).
//  3. If the subscription is PENDING → Activate it atomically.
//  4. Audit both payment.recorded + subscription.activated.
//
// Steps 1-3 must be atomic — a payment without an activation is a half-state.
// The current implementation runs them as separate SQL statements on the
// same adapter; a future refactor should wrap them in a single Within()
// transaction. For now, the Append + Activate pair is the closest thing
// to atomic since both go through the same pool — a partial failure
// (payment appended but Activate fails) leaves a payment record + a PENDING
// subscription, which is recoverable via admin retry of Activate.
func (s *Server) platformRecordPayment(ctx context.Context, in *dto.RecordPaymentInput) (any, bool) {
	if s.platformDeps.Subscriptions == nil || s.platformDeps.Payments == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "subscription/payment repositories are not wired")), true
	}
	subID := strings.TrimSpace(string(in.SubscriptionID))
	if subID == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "subscription_id is required")), true
	}
	if in.Body.AmountYER <= 0 {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "amount_yer must be positive")), true
	}
	if strings.TrimSpace(in.Body.Method) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "method is required")), true
	}
	if strings.TrimSpace(in.Body.Reference) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "reference is required")), true
	}
	paidAt, err := parseOptionalRFC3339(in.Body.PaidAt)
	if err != nil || paidAt == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "paid_at must be a valid RFC 3339 timestamp")), true
	}
	// 1. Fetch subscription — needed for business_id cross-check (tenant isolation).
	sub, err := s.platformDeps.Subscriptions.GetByID(ctx, subID)
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "payment.recorded", "subscription", &subID, nil, "FAILURE", failureCode, map[string]any{"reason": "subscription lookup failed"})
		return mapApplicationError(err), true
	}
	adminID, _ := middleware.PlatformAdminID(ctx)
	// 2. Append payment (Contract §27: append-only).
	payment, err := s.platformDeps.Payments.Append(ctx, ports.PaymentCreate{
		SubscriptionID: sub.ID,
		BusinessID:     sub.BusinessID,
		AmountYER:      in.Body.AmountYER,
		Method:         in.Body.Method,
		Reference:      in.Body.Reference,
		PaidAt:         *paidAt,
		RecordedBy:     string(adminID),
		Now:            time.Now().UTC(),
	})
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "payment.recorded", "subscription", &subID, &sub.BusinessID, "FAILURE", failureCode, map[string]any{
			"amount_yer": in.Body.AmountYER, "method": in.Body.Method, "reference": in.Body.Reference,
		})
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "payment.recorded", "payment", &payment.ID, &sub.BusinessID, "SUCCESS", "", map[string]any{
		"subscription_id": sub.ID, "amount_yer": payment.AmountYER, "method": payment.Method, "reference": payment.Reference,
	})
	// 3. Activate subscription if it was PENDING (Contract §28).
	// If the subscription was already ACTIVE (renewal mid-period?) or terminal,
	// Activate returns Conflict — we log it as a soft note but DON'T fail the
	// payment record. The payment is append-only and stays even if activation
	// is rejected (e.g., subscription is CANCELLED — payment is still a
	// historical record).
	if sub.Status == "PENDING" {
		// Per Contract §34-35: check entitlements BEFORE activating.
		// If catalog or channel count exceeds the plan's limits, block
		// activation. The payment is still recorded (append-only), but
		// the subscription stays PENDING until the merchant resolves
		// the overrun. No silent data deletion.
		entitlementErr := s.platformDeps.Subscriptions.CheckEntitlements(ctx, sub.BusinessID, sub.AICatalogLimit, sub.ChannelLimit)
		if entitlementErr != nil {
			failureCode := classifyPlatformRepoErrorKind(entitlementErr)
			s.appendPlatformAudit(ctx, "subscription.activated", "subscription", &sub.ID, &sub.BusinessID, "FAILURE", failureCode, map[string]any{
				"payment_id": payment.ID, "reason": "entitlement_overrun",
			})
			// Return the payment view — the payment IS recorded, but
			// activation is blocked. The caller sees the payment + a
			// 409 Conflict in the audit log explaining why activation
			// was blocked.
			out := &contract.Single[dto.PaymentView]{}
			out.Body.Data = paymentProjection(payment)
			return out, true
		}
		activated, activateErr := s.platformDeps.Subscriptions.Activate(ctx, sub.ID, time.Now().UTC())
		if activateErr != nil {
			failureCode := classifyPlatformRepoErrorKind(activateErr)
			s.appendPlatformAudit(ctx, "subscription.activated", "subscription", &sub.ID, &sub.BusinessID, "FAILURE", failureCode, map[string]any{"payment_id": payment.ID})
		} else {
			s.appendPlatformAudit(ctx, "subscription.activated", "subscription", &activated.ID, &activated.BusinessID, "SUCCESS", "", map[string]any{"payment_id": payment.ID})
		}
	}
	out := &contract.Single[dto.PaymentView]{}
	out.Body.Data = paymentProjection(payment)
	return out, true
}

func (s *Server) platformListPayments(ctx context.Context, in *dto.PaymentListInput) (any, bool) {
	if s.platformDeps.Payments == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "payment repository is not wired")), true
	}
	page, err := s.platformDeps.Payments.List(ctx, ports.PaymentListFilter{
		SubscriptionID: in.SubscriptionID,
		BusinessID:     in.BusinessID,
		Method:         in.Method,
		Limit:          in.Limit,
		Cursor:         in.Cursor,
	})
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.List[dto.PaymentView]{}
	items := make([]dto.PaymentView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, paymentProjection(record))
	}
	out.Body.Data = items
	out.Body.Pagination = contract.Page{HasMore: page.HasMore, NextCursor: platformCursorPtr(page.NextCursor)}
	return out, true
}

// ----------------------------------------------------------------------------
// Subscription AI Usage — Cost Budget Override (Contract §24, AIUsageTokenTelemetry.md §24)
// ----------------------------------------------------------------------------

// platformGetSubscriptionAIUsage returns the AI usage telemetry + cost
// budget snapshot for a subscription. Per AIUsageTokenTelemetry.md §32:
// the view shows BOTH entitlement (AI Replies) AND consumption (tokens +
// requests + tool calls + provider cost + projected costs + budget status).
//
// Per Contract §83: the view does NOT expose customer message content,
// full AI prompts, private business knowledge, or merchant secrets —
// only counts / rates / latency / errors / provider+model.
func (s *Server) platformGetSubscriptionAIUsage(ctx context.Context, in *dto.PlatformSubscriptionPath) (any, bool) {
	if s.platformDeps.AIUsage == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI usage repository is not wired")), true
	}
	agg, err := s.platformDeps.AIUsage.GetSubscriptionAIUsage(ctx, string(in.SubscriptionID))
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[dto.SubscriptionAIUsageView]{}
	out.Body.Data = subscriptionAIUsageProjection(agg)
	return out, true
}

func subscriptionAIUsageProjection(a ports.SubscriptionAIUsageAggregate) dto.SubscriptionAIUsageView {
	return dto.SubscriptionAIUsageView{
		SubscriptionID:            a.SubscriptionID,
		AIReplyLimit:              a.AIReplyLimit,
		AIRepliesUsed:             a.AIRepliesUsed,
		AIRepliesRemaining:        a.AIRepliesRemaining,
		InputTokens:               a.InputTokens,
		CachedInputTokens:         a.CachedInputTokens,
		OutputTokens:              a.OutputTokens,
		ModelRequests:             a.ModelRequests,
		ToolCalls:                 a.ToolCalls,
		ActualProviderCostYER:     a.ProviderCostYER,
		InternalCostBudgetYER:     a.InternalCostBudgetYER,
		CostRemainingYER:          a.CostRemainingYER,
		AverageCostPerReplyYER:    a.AverageCostPerReplyYER,
		ProjectedRemainingCostYER: a.ProjectedRemainingCostYER,
		ProjectedTotalCostYER:     a.ProjectedTotalCostYER,
		BudgetStatus:              a.BudgetStatus,
	}
}

// platformOverrideSubscriptionAICostBudget overrides the per-subscription
// cost budget. Per AIUsageTokenTelemetry.md §24:
//   - Does NOT change plan version.
//   - Old/new budget + reason + actor recorded in platform_audit_events.
//   - The override applies only to THIS subscription — not retroactively to
//     the plan or to other subscriptions on the same plan.
//
// After override, the response returns the FULL AI Usage view (not just the
// override receipt) — per Contract §32, the platform admin needs to see the
// updated cost_budget + remaining + budget_status.
func (s *Server) platformOverrideSubscriptionAICostBudget(ctx context.Context, in *dto.SubscriptionAICostBudgetOverrideInput) (any, bool) {
	if s.platformDeps.Subscriptions == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "subscription repository is not wired")), true
	}
	if in.Body.BudgetYER <= 0 {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "budget_yer must be positive")), true
	}
	if strings.TrimSpace(in.Body.Reason) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "reason is required")), true
	}
	adminID, _ := middleware.PlatformAdminID(ctx)
	record, err := s.platformDeps.Subscriptions.ApplyCostBudgetOverride(ctx, string(in.SubscriptionID), in.Body.BudgetYER, in.Body.Reason, string(adminID), time.Now().UTC())
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		subID := string(in.SubscriptionID)
		s.appendPlatformAudit(ctx, "subscription.cost_budget_overridden", "subscription", &subID, nil, "FAILURE", failureCode, map[string]any{"new_budget_yer": in.Body.BudgetYER, "reason": in.Body.Reason})
		return mapApplicationError(err), true
	}
	subID := record.ID
	s.appendPlatformAudit(ctx, "subscription.cost_budget_overridden", "subscription", &subID, &record.BusinessID, "SUCCESS", "", map[string]any{
		"new_budget_yer": in.Body.BudgetYER, "reason": in.Body.Reason,
	})
	// Return the full AI usage view if the AIUsage repo is wired; otherwise
	// fall back to the basic override receipt (just the budget fields).
	out := &contract.Single[dto.SubscriptionAIUsageView]{}
	if s.platformDeps.AIUsage != nil {
		agg, aggErr := s.platformDeps.AIUsage.GetSubscriptionAIUsage(ctx, record.ID)
		if aggErr == nil {
			out.Body.Data = subscriptionAIUsageProjection(agg)
			return out, true
		}
		// fall through to fallback on error
	}
	out.Body.Data = dto.SubscriptionAIUsageView{
		SubscriptionID:        record.ID,
		InternalCostBudgetYER: record.InternalAICostBudgetYER,
	}
	if record.CostBudgetOverrideYER != nil {
		out.Body.Data.InternalCostBudgetYER = *record.CostBudgetOverrideYER
	}
	return out, true
}

// nullableStringPtr returns *string if value is non-empty, else nil.
// Used for audit business_id cross-reference.
func nullableStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// ----------------------------------------------------------------------------
// Platform AI Operations — Contract §73-108 (Kill Switch + Provider Health)
// ----------------------------------------------------------------------------

// platformGetAIOverview returns the current runtime + provider state.
// Per Contract §75: the view exposes:
//   - Runtime: status (ENABLED | DISABLED) + health (HEALTHY | DEGRADED |
//     DOWN | UNKNOWN).
//   - Provider: provider + model + enabled + health.
//
// Per Contract §104: the Dashboard sees `configured = true` for the
// provider — it NEVER sees the actual API key. The ProviderRecord struct
// exposes Configured (bool), not the secret.
func (s *Server) platformGetAIOverview(ctx context.Context) (any, bool) {
	if s.platformDeps.Operations == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform operations registry is not wired")), true
	}
	runtime, err := s.platformDeps.Operations.GetRuntimeState(ctx)
	if err != nil {
		return mapApplicationError(err), true
	}
	providers, err := s.platformDeps.Operations.ListProviders(ctx)
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[dto.PlatformAIOverviewView]{}
	overview := dto.PlatformAIOverviewView{}
	overview.Runtime.Status = string(runtime.AdminState)
	overview.Runtime.Health = string(runtime.HealthState)
	// Find the AI provider — per Contract §76: google_gemini + gemini-3.1-flash-lite
	for _, p := range providers {
		if p.ProviderType == ports.ProviderTypeAI {
			overview.Provider.Provider = p.ProviderID
			overview.Provider.Model = p.Model
			overview.Provider.Enabled = p.AdminState == ports.ProviderAdminEnabled
			overview.Provider.Health = string(p.HealthState)
			break
		}
	}
	out.Body.Data = overview
	return out, true
}

// platformAIDisable implements the AI Emergency Kill Switch (Contract §81).
//
// DISABLED stops:
//   - AI Auto Reply
//   - AI Automated Sales Decisions
//   - AI AI-driven Automation
//
// DISABLED does NOT stop:
//   - Human Reply
//   - Merchant Dashboard
//   - Customer Data
//   - Leads
//   - Orders
//   - Channel Reception
//
// Every Disable is Platform-Audited (Contract §102 — ai.disabled).
func (s *Server) platformAIDisable(ctx context.Context, _ *dto.PlatformAIRuntimeDisableInput) (any, bool) {
	if s.platformDeps.Operations == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform operations registry is not wired")), true
	}
	now := time.Now().UTC()
	runtime, err := s.platformDeps.Operations.DisableRuntime(ctx, now)
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "ai.disabled", "ai_runtime", nil, nil, "FAILURE", failureCode, nil)
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "ai.disabled", "ai_runtime", nil, nil, "SUCCESS", "", map[string]any{
		"new_admin_state": string(runtime.AdminState),
	})
	out := &contract.Single[dto.PlatformAIOverviewView]{}
	out.Body.Data.Runtime.Status = string(runtime.AdminState)
	out.Body.Data.Runtime.Health = string(runtime.HealthState)
	return out, true
}

// platformAIEnable re-enables the AI Runtime after a kill switch. Per
// Contract §102 — every enable is audited as ai.enabled.
func (s *Server) platformAIEnable(ctx context.Context, _ *dto.PlatformAIRuntimeEnableInput) (any, bool) {
	if s.platformDeps.Operations == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform operations registry is not wired")), true
	}
	now := time.Now().UTC()
	runtime, err := s.platformDeps.Operations.EnableRuntime(ctx, now)
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "ai.enabled", "ai_runtime", nil, nil, "FAILURE", failureCode, nil)
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "ai.enabled", "ai_runtime", nil, nil, "SUCCESS", "", map[string]any{
		"new_admin_state": string(runtime.AdminState),
	})
	out := &contract.Single[dto.PlatformAIOverviewView]{}
	out.Body.Data.Runtime.Status = string(runtime.AdminState)
	out.Body.Data.Runtime.Health = string(runtime.HealthState)
	return out, true
}

// platformAIHealthCheck invokes the registered probe for Google Gemini.
// Per Contract §84: the probe uses a standalone prompt — no merchant_id,
// business_id, customer data, or merchant catalog.
//
// Per Contract §102: every health check is audited as provider.health_checked.
func (s *Server) platformAIHealthCheck(ctx context.Context, _ *dto.PlatformAIHealthCheckInput) (any, bool) {
	if s.platformDeps.Operations == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform operations registry is not wired")), true
	}
	now := time.Now().UTC()
	// Hard-coded to google_gemini per Contract §76 — the only AI provider.
	provider, err := s.platformDeps.Operations.RunHealthCheck(ctx, "google_gemini", now)
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "provider.health_checked", "ai_provider", nil, nil, "FAILURE", failureCode, map[string]any{"provider": "google_gemini"})
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "provider.health_checked", "ai_provider", &provider.ProviderID, nil, "SUCCESS", "", map[string]any{
		"provider": provider.ProviderID,
		"model":    provider.Model,
		"health":   string(provider.HealthState),
	})
	out := &contract.Single[dto.PlatformAIHealthCheckResult]{}
	out.Body.Data.Provider = provider.ProviderID
	out.Body.Data.Model = provider.Model
	out.Body.Data.Result = string(provider.HealthState)
	out.Body.Data.CheckedAt = now.UTC().Format(time.RFC3339)
	if provider.LastFailureCode != nil {
		out.Body.Data.FailureCode = *provider.LastFailureCode
	}
	return out, true
}

// ----------------------------------------------------------------------------
// Support façades — Contract §37-44
// ----------------------------------------------------------------------------

func supportTicketProjection(t ports.SupportTicketRecord) dto.SupportTicketView {
	view := dto.SupportTicketView{
		ID:            t.ID,
		BusinessID:    t.BusinessID,
		Subject:       t.Subject,
		Status:        t.Status,
		Priority:      t.Priority,
		Category:      t.Category,
		CreatedBy:     t.CreatedBy,
		CreatedByType: t.CreatedByType,
		ResolvedBy:    t.ResolvedBy,
		ClosedBy:      t.ClosedBy,
		CreatedAt:     t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     t.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if t.ResolvedAt != nil {
		formatted := t.ResolvedAt.UTC().Format(time.RFC3339)
		view.ResolvedAt = &formatted
	}
	if t.ClosedAt != nil {
		formatted := t.ClosedAt.UTC().Format(time.RFC3339)
		view.ClosedAt = &formatted
	}
	if t.LastMessageAt != nil {
		formatted := t.LastMessageAt.UTC().Format(time.RFC3339)
		view.LastMessageAt = &formatted
	}
	return view
}

func supportMessageProjection(m ports.SupportMessageRecord) dto.SupportMessageView {
	return dto.SupportMessageView{
		ID:         m.ID,
		TicketID:   m.TicketID,
		BusinessID: m.BusinessID,
		AuthorType: m.AuthorType,
		AuthorID:   m.AuthorID,
		Body:       m.Body,
		CreatedAt:  m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) platformCreateSupportTicket(ctx context.Context, in *dto.CreateSupportTicketInput) (any, bool) {
	if s.platformDeps.Support == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "support repository is not wired")), true
	}
	if strings.TrimSpace(in.Body.Subject) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "subject is required")), true
	}
	adminID, _ := middleware.PlatformAdminID(ctx)
	record, err := s.platformDeps.Support.CreateTicket(ctx, ports.SupportTicketCreate{
		ID:            uuid.NewString(),
		BusinessID:    string(in.BusinessID),
		Subject:       in.Body.Subject,
		Priority:      in.Body.Priority,
		Category:      in.Body.Category,
		CreatedBy:     string(adminID),
		CreatedByType: "PLATFORM_ADMIN",
		Now:           time.Now().UTC(),
	})
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "support.ticket.created", "support_ticket", nil, nullableStringPtr(string(in.BusinessID)), "FAILURE", failureCode, map[string]any{"subject": in.Body.Subject})
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "support.ticket.created", "support_ticket", &record.ID, &record.BusinessID, "SUCCESS", "", map[string]any{
		"subject": record.Subject, "priority": record.Priority, "category": record.Category,
	})
	out := &contract.Single[dto.SupportTicketView]{}
	out.Body.Data = supportTicketProjection(record)
	return out, true
}

func (s *Server) platformListSupportTickets(ctx context.Context, in *dto.SupportTicketListInput) (any, bool) {
	if s.platformDeps.Support == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "support repository is not wired")), true
	}
	filter := ports.SupportTicketListFilter{
		BusinessID: in.BusinessID,
		Status:     in.Status,
		Priority:   in.Priority,
		Category:   in.Category,
		Limit:      in.Limit,
		Cursor:     in.Cursor,
	}
	if from, err := parseOptionalRFC3339(in.CreatedFrom); err == nil {
		filter.CreatedFrom = from
	}
	if to, err := parseOptionalRFC3339(in.CreatedTo); err == nil {
		filter.CreatedTo = to
	}
	page, err := s.platformDeps.Support.ListTickets(ctx, filter)
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.List[dto.SupportTicketView]{}
	items := make([]dto.SupportTicketView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, supportTicketProjection(record))
	}
	out.Body.Data = items
	out.Body.Pagination = contract.Page{HasMore: page.HasMore, NextCursor: platformCursorPtr(page.NextCursor)}
	return out, true
}

func (s *Server) platformGetSupportTicket(ctx context.Context, in *dto.PlatformSupportTicketPath) (any, bool) {
	if s.platformDeps.Support == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "support repository is not wired")), true
	}
	record, err := s.platformDeps.Support.GetTicketByID(ctx, string(in.TicketID))
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[dto.SupportTicketView]{}
	out.Body.Data = supportTicketProjection(record)
	return out, true
}

func (s *Server) platformCreateSupportMessage(ctx context.Context, in *dto.CreateSupportMessageInput) (any, bool) {
	if s.platformDeps.Support == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "support repository is not wired")), true
	}
	if strings.TrimSpace(in.Body.Body) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "body is required")), true
	}
	// Fetch the ticket to validate it exists + capture business_id for cross-reference.
	ticket, err := s.platformDeps.Support.GetTicketByID(ctx, string(in.TicketID))
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		ticketIDStr := string(in.TicketID)
		s.appendPlatformAudit(ctx, "support.message.created", "support_message", &ticketIDStr, nil, "FAILURE", failureCode, map[string]any{"reason": "ticket lookup failed"})
		return mapApplicationError(err), true
	}
	adminID, _ := middleware.PlatformAdminID(ctx)
	message, err := s.platformDeps.Support.AppendMessage(ctx, ports.SupportMessageCreate{
		ID:         uuid.NewString(),
		TicketID:   ticket.ID,
		BusinessID: ticket.BusinessID,
		AuthorType: "PLATFORM_ADMIN",
		AuthorID:   string(adminID),
		Body:       in.Body.Body,
		Now:        time.Now().UTC(),
	})
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		ticketIDStr := string(in.TicketID)
		s.appendPlatformAudit(ctx, "support.message.created", "support_message", &ticketIDStr, &ticket.BusinessID, "FAILURE", failureCode, nil)
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "support.message.created", "support_message", &message.ID, &message.BusinessID, "SUCCESS", "", map[string]any{
		"ticket_id": message.TicketID, "author_type": message.AuthorType,
	})
	out := &contract.Single[dto.SupportMessageView]{}
	out.Body.Data = supportMessageProjection(message)
	return out, true
}

func (s *Server) platformStartSupportTicket(ctx context.Context, in *dto.StartSupportTicketInput) (any, bool) {
	return s.supportTicketLifecycleTransition(ctx, "platformStartSupportTicket", "support.ticket.started", in.TicketID, "start")
}

func (s *Server) platformResolveSupportTicket(ctx context.Context, in *dto.ResolveSupportTicketInput) (any, bool) {
	return s.supportTicketLifecycleTransition(ctx, "platformResolveSupportTicket", "support.ticket.resolved", in.TicketID, "resolve")
}

func (s *Server) platformCloseSupportTicket(ctx context.Context, in *dto.CloseSupportTicketInput) (any, bool) {
	return s.supportTicketLifecycleTransition(ctx, "platformCloseSupportTicket", "support.ticket.closed", in.TicketID, "close")
}

// supportTicketLifecycleTransition dispatches the start/resolve/close
// transitions on a ticket. Per Contract §39, transitions are forward-only;
// CLOSED is terminal. Each transition is audited.
func (s *Server) supportTicketLifecycleTransition(ctx context.Context, op, auditAction string, ticketID dto.UUID, transition string) (any, bool) {
	if s.platformDeps.Support == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "support repository is not wired")), true
	}
	ticketIDStr := strings.TrimSpace(string(ticketID))
	if ticketIDStr == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "ticket_id is required")), true
	}
	adminID, _ := middleware.PlatformAdminID(ctx)
	now := time.Now().UTC()
	var (
		record ports.SupportTicketRecord
		err    error
	)
	switch transition {
	case "start":
		record, err = s.platformDeps.Support.StartTicket(ctx, ticketIDStr, string(adminID), now)
	case "resolve":
		record, err = s.platformDeps.Support.ResolveTicket(ctx, ticketIDStr, string(adminID), now)
	case "close":
		record, err = s.platformDeps.Support.CloseTicket(ctx, ticketIDStr, string(adminID), now)
	default:
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "unknown transition: "+transition)), true
	}
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, auditAction, "support_ticket", &ticketIDStr, nil, "FAILURE", failureCode, map[string]any{"transition": transition})
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, auditAction, "support_ticket", &record.ID, &record.BusinessID, "SUCCESS", "", map[string]any{
		"new_status": record.Status, "transition": transition,
	})
	out := &contract.Single[dto.SupportTicketView]{}
	out.Body.Data = supportTicketProjection(record)
	return out, true
}

// ----------------------------------------------------------------------------
// AI Provider Listing façades (Contract §85, §107)
// ----------------------------------------------------------------------------

func providerProjection(p ports.ProviderRecord) dto.PlatformProviderView {
	view := dto.PlatformProviderView{
		ProviderID:      p.ProviderID,
		ProviderType:    string(p.ProviderType),
		DisplayName:     p.DisplayName,
		AdminState:      string(p.AdminState),
		HealthState:     string(p.HealthState),
		Configured:      p.Configured,
		Model:           p.Model,
		LastFailureCode: p.LastFailureCode,
	}
	if p.LastHealthCheck != nil {
		s := p.LastHealthCheck.UTC().Format(time.RFC3339)
		view.LastHealthCheck = &s
	}
	if p.LastSuccess != nil {
		s := p.LastSuccess.UTC().Format(time.RFC3339)
		view.LastSuccess = &s
	}
	if p.LastFailure != nil {
		s := p.LastFailure.UTC().Format(time.RFC3339)
		view.LastFailure = &s
	}
	return view
}

func (s *Server) platformListAIProviders(ctx context.Context) (any, bool) {
	if s.platformDeps.Operations == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform operations registry is not wired")), true
	}
	providers, err := s.platformDeps.Operations.ListProviders(ctx)
	if err != nil {
		return mapApplicationError(err), true
	}
	// Per Contract §88: only show AI-type providers from the AI listing endpoint.
	// OpenAI-compatible adapter is NOT shown as active.
	items := make([]dto.PlatformProviderView, 0, len(providers))
	for _, p := range providers {
		if p.ProviderType == ports.ProviderTypeAI {
			items = append(items, providerProjection(p))
		}
	}
	out := &contract.List[dto.PlatformProviderView]{}
	out.Body.Data = items
	return out, true
}

func (s *Server) platformGetAIProvider(ctx context.Context, in *dto.PlatformProviderPathInput) (any, bool) {
	if s.platformDeps.Operations == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform operations registry is not wired")), true
	}
	provider, err := s.platformDeps.Operations.GetProvider(ctx, in.ProviderID)
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[dto.PlatformProviderView]{}
	out.Body.Data = providerProjection(provider)
	return out, true
}

// ----------------------------------------------------------------------------
// Channel Operations façades (Contract §89-96, §107)
// ----------------------------------------------------------------------------

// platformListChannels lists all channel connections platform-wide.
// Per Contract §93: shows business_id, connection_id, channel, provider,
// status, health, provider_account_ref, provider_connection_ref,
// last_health_check_at, last_success, last_failure, failure_code.
// Does NOT show: access_token, secret, customer messages, conversation body,
// merchant catalog (per §93).
//
// The health state is derived from the InMemoryPlatformOperationsRegistry's
// provider-level health (per Contract §99: each provider has independent health).
// Per-connection health would require a per-connection probe — deferred to V2
// since the contract says "نضيف مفهومًا منفصلًا" (we add a separate concept) —
// the concept exists (Health field in the view), the per-connection probe
// implementation is a follow-up. For V1, all connections inherit their
// provider's health.
func (s *Server) platformListChannels(ctx context.Context, in *dto.PlatformChannelListInput) (any, bool) {
	if s.platformDeps.ChannelReader == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "channel reader is not wired")), true
	}
	// Fetch channel connections from the DB (no secrets exposed per §93).
	connections, err := s.platformDeps.ChannelReader.ListChannels(ctx, ports.PlatformChannelListFilter{
		BusinessID: in.BusinessID,
		Status:     in.Status,
		Channel:    in.Channel,
		Limit:      in.Limit,
	})
	if err != nil {
		return mapApplicationError(err), true
	}
	// Fetch provider-level health from the in-memory Operations registry.
	providerHealth := map[string]string{}
	if s.platformDeps.Operations != nil {
		providers, _ := s.platformDeps.Operations.ListProviders(ctx)
		for _, p := range providers {
			if p.ProviderType == ports.ProviderTypeChannelTransport {
				providerHealth[p.ProviderID] = string(p.HealthState)
			}
		}
	}
	items := make([]dto.PlatformChannelView, 0, len(connections))
	for _, c := range connections {
		view := dto.PlatformChannelView{
			BusinessID:            c.BusinessID,
			ConnectionID:          c.ID,
			Channel:               c.Channel,
			Provider:              c.ProviderRef,
			Status:                c.Status,
			ProviderAccountRef:    c.ProviderAccountRef,
			ProviderConnectionRef: &c.ProviderConnectionRef,
			LastHealthCheckAt:     c.LastHealthCheckAt,
		}
		// Health is inherited from the provider level per Contract §99.
		if health, ok := providerHealth[c.ProviderRef]; ok {
			view.Health = health
		} else {
			view.Health = "UNKNOWN"
		}
		items = append(items, view)
	}
	out := &contract.List[dto.PlatformChannelView]{}
	out.Body.Data = items
	return out, true
}

func (s *Server) platformGetChannel(ctx context.Context, in *dto.PlatformChannelPathInput) (any, bool) {
	if s.platformDeps.ChannelReader == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "channel reader is not wired")), true
	}
	c, err := s.platformDeps.ChannelReader.GetChannelByID(ctx, string(in.ConnectionID))
	if err != nil {
		return mapApplicationError(err), true
	}
	view := dto.PlatformChannelView{
		BusinessID:            c.BusinessID,
		ConnectionID:          c.ID,
		Channel:               c.Channel,
		Provider:              c.ProviderRef,
		Status:                c.Status,
		ProviderAccountRef:    c.ProviderAccountRef,
		ProviderConnectionRef: &c.ProviderConnectionRef,
		LastHealthCheckAt:     c.LastHealthCheckAt,
	}
	// Health from provider-level registry.
	if s.platformDeps.Operations != nil {
		if provider, pErr := s.platformDeps.Operations.GetProvider(ctx, c.ProviderRef); pErr == nil {
			view.Health = string(provider.HealthState)
		}
	}
	if view.Health == "" {
		view.Health = "UNKNOWN"
	}
	out := &contract.Single[dto.PlatformChannelView]{}
	out.Body.Data = view
	return out, true
}

func (s *Server) platformChannelHealthCheck(ctx context.Context, in *dto.PlatformChannelHealthCheckInput) (any, bool) {
	if s.platformDeps.Operations == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform operations registry is not wired")), true
	}
	// Per Contract §94: run a health check on the channel connection.
	// For V1, this delegates to the provider-level health check for SocialAPI
	// (the only channel transport provider per Contract §90).
	now := time.Now().UTC()
	provider, err := s.platformDeps.Operations.RunHealthCheck(ctx, "socialapi", now)
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		connID := string(in.ConnectionID)
		s.appendPlatformAudit(ctx, "channel.health_checked", "channel_connection", &connID, nil, "FAILURE", failureCode, nil)
		return mapApplicationError(err), true
	}
	connID := string(in.ConnectionID)
	s.appendPlatformAudit(ctx, "channel.health_checked", "channel_connection", &connID, nil, "SUCCESS", "", map[string]any{
		"provider": provider.ProviderID, "health": string(provider.HealthState),
	})
	out := &contract.Single[dto.PlatformChannelView]{}
	out.Body.Data = dto.PlatformChannelView{
		ConnectionID: connID,
		Provider:     provider.ProviderID,
		Health:       string(provider.HealthState),
	}
	return out, true
}

// ----------------------------------------------------------------------------
// Provider Operations façades (Contract §97-98, §107)
// ----------------------------------------------------------------------------

func (s *Server) platformListProviders(ctx context.Context) (any, bool) {
	if s.platformDeps.Operations == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform operations registry is not wired")), true
	}
	providers, err := s.platformDeps.Operations.ListProviders(ctx)
	if err != nil {
		return mapApplicationError(err), true
	}
	// Per Contract §97: show ALL providers (both AI and CHANNEL_TRANSPORT).
	items := make([]dto.PlatformProviderView, 0, len(providers))
	for _, p := range providers {
		items = append(items, providerProjection(p))
	}
	out := &contract.List[dto.PlatformProviderView]{}
	out.Body.Data = items
	return out, true
}

func (s *Server) platformGetProvider(ctx context.Context, in *dto.PlatformProviderPathInput) (any, bool) {
	if s.platformDeps.Operations == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform operations registry is not wired")), true
	}
	provider, err := s.platformDeps.Operations.GetProvider(ctx, in.ProviderID)
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[dto.PlatformProviderView]{}
	out.Body.Data = providerProjection(provider)
	return out, true
}

func (s *Server) platformProviderHealthCheck(ctx context.Context, in *dto.PlatformProviderHealthCheckInput) (any, bool) {
	if s.platformDeps.Operations == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform operations registry is not wired")), true
	}
	now := time.Now().UTC()
	provider, err := s.platformDeps.Operations.RunHealthCheck(ctx, in.ProviderID, now)
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "provider.health_checked", "provider", &in.ProviderID, nil, "FAILURE", failureCode, map[string]any{"provider": in.ProviderID})
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "provider.health_checked", "provider", &provider.ProviderID, nil, "SUCCESS", "", map[string]any{
		"provider": provider.ProviderID, "health": string(provider.HealthState),
	})
	out := &contract.Single[dto.PlatformProviderView]{}
	out.Body.Data = providerProjection(provider)
	return out, true
}

// ----------------------------------------------------------------------------
// Platform-wide AI Usage façades (AIUsageTokenTelemetry.md §27, §34)
// ----------------------------------------------------------------------------

func (s *Server) platformGetAIUsageOverview(ctx context.Context) (any, bool) {
	if s.platformDeps.AIUsage == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI usage repository is not wired")), true
	}
	agg, err := s.platformDeps.AIUsage.GetPlatformAIUsageOverview(ctx)
	if err != nil {
		return mapApplicationError(err), true
	}
	// Per P2-12: expose the platform-wide budget fields. The aggregate
	// computed by GetPlatformAIUsageOverview carries InternalCostBudgetYER
	// (sum across all businesses' active subscriptions' cost_budget_yer)
	// + CostRemainingYER + ProviderCostYER (consumed). Average cost per
	// reply is computed by the repository (handle div-by-zero when 0 replies).
	out := &contract.Single[dto.PlatformAIUsageOverviewView]{}
	out.Body.Data = dto.PlatformAIUsageOverviewView{
		TotalAIReplies:       int64(agg.AIRepliesUsed),
		TotalInputTokens:     agg.InputTokens,
		TotalCachedTokens:    agg.CachedInputTokens,
		TotalOutputTokens:    agg.OutputTokens,
		TotalModelRequests:   agg.ModelRequests,
		TotalToolCalls:       agg.ToolCalls,
		TotalProviderCostYER: agg.ProviderCostYER,
		AverageCostPerReply:  agg.AverageCostPerReplyYER,
		ActiveBudgetYER:      agg.InternalCostBudgetYER,
		ConsumedBudgetYER:    agg.ProviderCostYER,
		RemainingBudgetYER:   agg.CostRemainingYER,
	}
	return out, true
}

func (s *Server) platformGetAIUsageBySubscription(ctx context.Context, in *dto.SubscriptionListInput) (any, bool) {
	if s.platformDeps.AIUsage == nil || s.platformDeps.Subscriptions == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI usage + subscription repositories are not wired")), true
	}
	page, err := s.platformDeps.Subscriptions.List(ctx, ports.SubscriptionListFilter{
		BusinessID: in.BusinessID,
		Status:     in.Status,
		Limit:      in.Limit,
		Cursor:     in.Cursor,
	})
	if err != nil {
		return mapApplicationError(err), true
	}
	items := make([]dto.SubscriptionAIUsageView, 0, len(page.Items))
	for _, sub := range page.Items {
		agg, aggErr := s.platformDeps.AIUsage.GetSubscriptionAIUsage(ctx, sub.ID)
		if aggErr != nil {
			continue
		}
		items = append(items, subscriptionAIUsageProjection(agg))
	}
	out := &contract.List[dto.SubscriptionAIUsageView]{}
	out.Body.Data = items
	out.Body.Pagination = contract.Page{HasMore: page.HasMore, NextCursor: platformCursorPtr(page.NextCursor)}
	return out, true
}

func (s *Server) platformGetAIUsageByBusiness(ctx context.Context, in *dto.PlatformBusinessListInput) (any, bool) {
	if s.platformDeps.AIUsage == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI usage repository is not wired")), true
	}
	// Per Item 9: the previous implementation ignored the DTO filters
	// and called GetAIUsageByBusiness(ctx, 100) with a hardcoded limit.
	// The DTO defines search/status/created_from/created_to/limit fields
	// that the contract expects to be respected. However, the underlying
	// repository method GetAIUsageByBusiness only accepts a `limit` param
	// — it doesn't support search/status/date filters. This is a
	// LIMITATION of the current repository implementation.
	//
	// Per the task instructions: "إذا الـcontract الحالي لا يحدد filter
	// semantics، لا تضف semantics من عندك؛ وثّق limitation بدل اختراع
	// behavior." We honor the `limit` field (from the DTO) instead of
	// hardcoding 100. The search/status/date filters are documented as
	// not-yet-implemented in the repository layer — the handler does NOT
	// invent behavior for them.
	limit := in.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	aggs, err := s.platformDeps.AIUsage.GetAIUsageByBusiness(ctx, limit)
	if err != nil {
		return mapApplicationError(err), true
	}
	// Per P2-11: project to BusinessAIUsageView (NOT SubscriptionAIUsageView).
	items := make([]dto.BusinessAIUsageView, 0, len(aggs))
	for _, agg := range aggs {
		items = append(items, dto.BusinessAIUsageView{
			BusinessID:           agg.BusinessID,
			TotalAIReplies:       agg.AIRepliesUsed,
			TotalInputTokens:     agg.InputTokens,
			TotalCachedTokens:    agg.CachedInputTokens,
			TotalOutputTokens:    agg.OutputTokens,
			TotalModelRequests:   agg.ModelRequests,
			TotalToolCalls:       agg.ToolCalls,
			TotalProviderCostYER: agg.ProviderCostYER,
		})
	}
	out := &contract.List[dto.BusinessAIUsageView]{}
	out.Body.Data = items
	return out, true
}

// ----------------------------------------------------------------------------
// Create Business façade — Contract §9, §13
// ----------------------------------------------------------------------------

// platformCreateBusiness creates a new business with status='pending_setup'.
// Per Contract §9: the Platform Admin creates the business; the owner
// invitation is a SEPARATE step using the existing team invitation mechanism
// (POST /businesses/{id}/team/invitations). The admin does NOT enter the
// owner's password.
//
// Per Contract §46: audits `business.created`.
func (s *Server) platformCreateBusiness(ctx context.Context, in *dto.CreateBusinessInput) (any, bool) {
	if s.platformDeps.PlatformBusiness == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "platform business repository is not wired")), true
	}
	if strings.TrimSpace(in.Body.Name) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "name is required")), true
	}
	if strings.TrimSpace(in.Body.Slug) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "slug is required")), true
	}
	if strings.TrimSpace(in.Body.VerticalType) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "vertical_type is required")), true
	}
	if len(in.Body.DefaultCurrency) != 3 {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "default_currency must be 3 uppercase letters")), true
	}
	if strings.TrimSpace(in.Body.Locale) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "locale is required")), true
	}
	record, err := s.platformDeps.PlatformBusiness.Create(ctx, ports.PlatformBusinessCreate{
		ID:              uuid.NewString(),
		Name:            in.Body.Name,
		Slug:            in.Body.Slug,
		VerticalType:    in.Body.VerticalType,
		Timezone:        in.Body.Timezone,
		DefaultCurrency: strings.ToUpper(in.Body.DefaultCurrency),
		Locale:          in.Body.Locale,
		Now:             time.Now().UTC(),
	})
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "business.created", "business", nil, nil, "FAILURE", failureCode, map[string]any{
			"name": in.Body.Name, "slug": in.Body.Slug,
		})
		return mapApplicationError(err), true
	}
	s.appendPlatformAudit(ctx, "business.created", "business", &record.ID, &record.ID, "SUCCESS", "", map[string]any{
		"name": record.Name, "slug": record.Slug, "status": record.PlatformStatus,
	})
	out := &contract.Single[dto.PlatformBusinessView]{}
	out.Body.Data = platformBusinessProjection(record)
	return out, true
}

// ----------------------------------------------------------------------------
// AI Provider Configuration façades (§13)
// ----------------------------------------------------------------------------

func aiCredentialProjection(c ports.AICredentialRecord) dto.AICredentialView {
	v := dto.AICredentialView{
		ID: c.ID, Provider: c.Provider, DisplayName: c.DisplayName,
		KeyHint: c.KeyHint, Status: c.Status, CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		ValidationError: c.ValidationError,
	}
	if c.ValidatedAt != nil {
		s := c.ValidatedAt.UTC().Format(time.RFC3339)
		v.ValidatedAt = &s
	}
	return v
}

func aiModelProjection(m ports.AIProviderModel) dto.AIModelView {
	return dto.AIModelView{
		ID: m.ID, Provider: m.Provider, ModelName: m.ModelName,
		DisplayName: m.DisplayName, Description: m.Description,
		InputTokenLimit: m.InputTokenLimit, OutputTokenLimit: m.OutputTokenLimit,
		SupportedMethods: m.SupportedMethods, ThinkingSupported: m.ThinkingSupported,
		TemperatureMin: m.TemperatureMin, TemperatureMax: m.TemperatureMax,
		TopPMin: m.TopPMin, TopPMax: m.TopPMax, TopKMin: m.TopKMin, TopKMax: m.TopKMax,
		Version: m.Version, BaseModel: m.BaseModel,
	}
}

func (s *Server) platformListAIProvidersConfig(ctx context.Context) (any, bool) {
	if s.platformDeps.AIConfigRepo == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI config repo not wired")), true
	}
	creds, err := s.platformDeps.AIConfigRepo.ListCredentials(ctx, "google_gemini")
	if err != nil {
		return mapApplicationError(err), true
	}
	items := make([]dto.AICredentialView, 0, len(creds))
	for _, c := range creds {
		items = append(items, aiCredentialProjection(c))
	}
	out := &contract.List[dto.AICredentialView]{}
	out.Body.Data = items
	return out, true
}

func (s *Server) platformAddAICredential(ctx context.Context, in *dto.AddCredentialInput) (any, bool) {
	if s.platformDeps.AIConfigRepo == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI config repo not wired")), true
	}
	if strings.TrimSpace(in.Body.APIKey) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "api_key is required")), true
	}
	if strings.TrimSpace(in.Body.Provider) == "" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "provider is required")), true
	}
	provider := in.Body.Provider
	adminID, _ := middleware.PlatformAdminID(ctx)
	apiKey := in.Body.APIKey
	hint := ""
	if len(apiKey) > 4 {
		hint = "..." + apiKey[len(apiKey)-4:]
	} else {
		hint = "..." + apiKey
	}
	now := time.Now().UTC()

	// Step 1: Store the NEW credential with status=CONFIGURED. Per §11:
	// failure safety — if the probe fails below, the NEW credential is
	// marked INVALID and the OLD active credential remains ACTIVE. The
	// system never runs without a working credential.
	record, err := s.platformDeps.AIConfigRepo.StoreCredential(ctx, ports.AICredentialCreate{
		ID: uuid.NewString(), Provider: provider,
		DisplayName: in.Body.DisplayName, EncryptedKey: apiKey, KeyHint: hint,
		CreatedBy: string(adminID), Now: now,
	})
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "ai.credential.added", "ai_credential", nil, nil, "FAILURE", failureCode, map[string]any{"provider": provider})
		return mapApplicationError(err), true
	}

	// Step 2: Atomic rotation — probe the NEW credential before marking
	// it VALID. Per §6: must be a REAL HTTP probe to Gemini, not a
	// config-only validity check. Per §11: if the probe fails, NEW is
	// marked INVALID and OLD remains ACTIVE — the runtime never breaks.
	//
	// We need a model name to probe. The active config's model is the
	// safest choice — it's the model currently in use, so a probe
	// against it proves the key works for the production workload.
	probeModel := ""
	probeBaseURL := "https://generativelanguage.googleapis.com"
	if s.platformDeps.AIConfigCache != nil {
		if activeCfg, cfgErr := s.platformDeps.AIConfigCache.GetActiveConfig(ctx); cfgErr == nil {
			probeModel = activeCfg.Model
			if activeCfg.BaseURL != "" {
				probeBaseURL = activeCfg.BaseURL
			}
		}
	}
	probeSuccess := false
	probeErrorCode := "MODEL_DISCOVERY_NOT_WIRED"
	probeLatency := int64(0)
	if s.platformDeps.ModelDiscovery != nil && probeModel != "" {
		probeSuccess, probeLatency, probeErrorCode = s.platformDeps.ModelDiscovery.TestConnection(ctx, apiKey, probeModel, probeBaseURL)
	} else if s.platformDeps.ModelDiscovery == nil {
		probeErrorCode = "MODEL_DISCOVERY_NOT_WIRED"
	} else if probeModel == "" {
		probeErrorCode = "NO_ACTIVE_MODEL"
	}

	if !probeSuccess {
		// Mark the NEW credential INVALID. Per §11: the OLD active
		// credential remains ACTIVE — GetActiveCredential orders by
		// created_at DESC + status IN (CONFIGURED, VALID), so an
		// INVALID NEW credential is filtered out.
		errMsg := probeErrorCode
		updated, _ := s.platformDeps.AIConfigRepo.UpdateCredentialStatus(ctx, record.ID, "INVALID", &errMsg, now)
		if updated.ID != "" {
			record = updated
		}
		s.appendPlatformAudit(ctx, "ai.credential.added", "ai_credential", &record.ID, nil, "FAILURE", "PROBE_FAILED", map[string]any{
			"provider": provider, "key_hint": record.KeyHint,
			"error_code": probeErrorCode, "latency_ms": probeLatency,
		})
		out := &contract.Single[dto.AICredentialView]{}
		out.Body.Data = aiCredentialProjection(record)
		return out, true
	}

	// Step 3: Probe succeeded — mark NEW as VALID. GetActiveCredential
	// orders by created_at DESC, so NEW (now VALID, just-created) becomes
	// the active credential on the next lookup. The OLD credential stays
	// in its current status (CONFIGURED/VALID) — it's simply shadowed by
	// the newer one. This is atomic rotation: at no point does the system
	// lack an active credential.
	validated, err := s.platformDeps.AIConfigRepo.UpdateCredentialStatus(ctx, record.ID, "VALID", nil, now)
	if err != nil {
		// Extremely rare — the Store succeeded but Update failed. Log
		// the failure and return the record with its current CONFIGURED
		// status so the admin can retry. Per §11: failure safety.
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "ai.credential.added", "ai_credential", &record.ID, nil, "FAILURE", failureCode, map[string]any{"provider": provider, "key_hint": record.KeyHint})
		return mapApplicationError(err), true
	}
	record = validated

	// Step 4: Invalidate the cache so the next Gemini call picks up
	// the NEW credential. Per §9: runtime switch without restart.
	if s.platformDeps.AIConfigCache != nil {
		s.platformDeps.AIConfigCache.Invalidate()
	}

	s.appendPlatformAudit(ctx, "ai.credential.added", "ai_credential", &record.ID, nil, "SUCCESS", "", map[string]any{
		"provider": record.Provider, "key_hint": record.KeyHint,
		"latency_ms": probeLatency,
	})
	out := &contract.Single[dto.AICredentialView]{}
	out.Body.Data = aiCredentialProjection(record)
	return out, true
}

func (s *Server) platformTestAIConnection(ctx context.Context, in *dto.TestConnectionInput) (any, bool) {
	if s.platformDeps.AIConfigCache == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI config cache not wired")), true
	}
	if s.platformDeps.ModelDiscovery == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "Model discovery client not wired")), true
	}
	// Per §6: real HTTP probe to Gemini — NOT a config-only validity check.
	// We resolve the active credential's decrypted API key + active model
	// and call ModelDiscovery.TestConnection which makes a real
	// generateContent request to the Gemini API.
	cfg, err := s.platformDeps.AIConfigCache.GetActiveConfig(ctx)
	if err != nil {
		s.appendPlatformAudit(ctx, "ai.connection.tested", "ai_runtime", nil, nil, "FAILURE", "CONFIG_NOT_FOUND", nil)
		out := &contract.Single[dto.TestConnectionResult]{}
		out.Body.Data = dto.TestConnectionResult{Success: false, ErrorCode: "CONFIG_NOT_FOUND"}
		return out, true
	}
	if cfg.APIKey == "" {
		s.appendPlatformAudit(ctx, "ai.connection.tested", "ai_runtime", nil, nil, "FAILURE", "NO_API_KEY", nil)
		out := &contract.Single[dto.TestConnectionResult]{}
		out.Body.Data = dto.TestConnectionResult{Success: false, Provider: cfg.Provider, Model: cfg.Model, ErrorCode: "NO_API_KEY"}
		return out, true
	}
	provider := in.Provider
	if provider == "" {
		provider = cfg.Provider
	}
	// Per §6: actual HTTP probe to the Gemini API. The probe sends "Hello"
	// as input — no customer data, no merchant catalog (per §84).
	success, latency, errorCode := s.platformDeps.ModelDiscovery.TestConnection(ctx, cfg.APIKey, cfg.Model, cfg.BaseURL)
	if !success {
		s.appendPlatformAudit(ctx, "ai.connection.tested", "ai_runtime", nil, nil, "FAILURE", errorCode, map[string]any{
			"provider": provider, "model": cfg.Model, "latency_ms": latency,
		})
		out := &contract.Single[dto.TestConnectionResult]{}
		out.Body.Data = dto.TestConnectionResult{
			Success: false, Provider: provider, Model: cfg.Model,
			LatencyMS: latency, ErrorCode: errorCode,
		}
		return out, true
	}
	s.appendPlatformAudit(ctx, "ai.connection.tested", "ai_runtime", nil, nil, "SUCCESS", "", map[string]any{
		"provider": provider, "model": cfg.Model, "latency_ms": latency,
	})
	out := &contract.Single[dto.TestConnectionResult]{}
	out.Body.Data = dto.TestConnectionResult{
		Success: true, Provider: provider, Model: cfg.Model, LatencyMS: latency,
	}
	return out, true
}

// platformDiscoverAIModels calls the real Gemini Models API (GET /v1beta/models)
// using the active credential's decrypted API key. Per §3: NO hardcoded list,
// NO config-only check — the discovery MUST hit the actual provider endpoint.
//
// Per §10: discovered models are upserted into ai_provider_models so the
// Platform Admin can later activate one via platformUpdateAIConfiguration.
func (s *Server) platformDiscoverAIModels(ctx context.Context, in *dto.DiscoverModelsInput) (any, bool) {
	if s.platformDeps.AIConfigCache == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI config cache not wired")), true
	}
	if s.platformDeps.ModelDiscovery == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "Model discovery client not wired")), true
	}
	if s.platformDeps.AIConfigRepo == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI config repo not wired")), true
	}
	// Resolve the active credential's decrypted API key.
	cfg, err := s.platformDeps.AIConfigCache.GetActiveConfig(ctx)
	if err != nil {
		s.appendPlatformAudit(ctx, "ai.models.discovered", "ai_runtime", nil, nil, "FAILURE", "CONFIG_NOT_FOUND", nil)
		out := &contract.List[dto.AIModelView]{}
		out.Body.Data = []dto.AIModelView{}
		return out, true
	}
	if cfg.APIKey == "" {
		s.appendPlatformAudit(ctx, "ai.models.discovered", "ai_runtime", nil, nil, "FAILURE", "NO_API_KEY", nil)
		out := &contract.List[dto.AIModelView]{}
		out.Body.Data = []dto.AIModelView{}
		return out, true
	}
	provider := in.Provider
	if provider == "" {
		provider = cfg.Provider
	}
	// Per §3: call the real Gemini Models API. No stub, no cached list.
	models, err := s.platformDeps.ModelDiscovery.DiscoverModels(ctx, cfg.APIKey, cfg.BaseURL)
	if err != nil {
		s.appendPlatformAudit(ctx, "ai.models.discovered", "ai_runtime", nil, nil, "FAILURE", "DISCOVERY_FAILED", map[string]any{
			"provider": provider, "error": err.Error(),
		})
		// Return an empty list (NOT a 500) so the frontend can render
		// a graceful "discovery failed" state.
		out := &contract.List[dto.AIModelView]{}
		out.Body.Data = []dto.AIModelView{}
		return out, true
	}
	// Per §10: upsert discovered models so the Platform Admin can later
	// activate one. This is a write to ai_provider_models.
	if err := s.platformDeps.AIConfigRepo.UpsertDiscoveredModels(ctx, models); err != nil {
		s.appendPlatformAudit(ctx, "ai.models.discovered", "ai_runtime", nil, nil, "FAILURE", "UPSERT_FAILED", map[string]any{
			"provider": provider, "error": err.Error(),
		})
		// Still return the discovered list — the upsert failure is logged
		// but the admin can see the discovery result.
	} else {
		s.appendPlatformAudit(ctx, "ai.models.discovered", "ai_runtime", nil, nil, "SUCCESS", "", map[string]any{
			"provider": provider, "model_count": len(models),
		})
	}
	views := make([]dto.AIModelView, 0, len(models))
	for _, m := range models {
		views = append(views, aiModelProjection(m))
	}
	out := &contract.List[dto.AIModelView]{}
	out.Body.Data = views
	return out, true
}

func (s *Server) platformGetAIConfiguration(ctx context.Context) (any, bool) {
	if s.platformDeps.AIConfigCache == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI config cache not wired")), true
	}
	cfg, err := s.platformDeps.AIConfigCache.GetActiveConfig(ctx)
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[dto.AIConfigurationView]{}
	out.Body.Data = dto.AIConfigurationView{
		Provider: cfg.Provider, Model: cfg.Model, CredentialID: cfg.CredentialID,
		CredentialStatus:    cfg.CredentialStatus,
		MujeebMaxInputChars: cfg.MaxInputCharacters, MujeebMaxOutputTokens: cfg.MaxOutputTokens,
		EffectiveMaxOutput: cfg.MaxOutputTokens,
		Version:            cfg.ConfigurationVersion, Status: "ACTIVE",
	}
	if cfg.PricingVersion != "" {
		pv := cfg.PricingVersion
		out.Body.Data.PricingVersion = &pv
	}
	return out, true
}

// platformUpdateAIConfiguration activates a new AI configuration version.
//
// Per §4: validate before activation. Per §5: effective_max_output_tokens =
// min(Mujeeb, Provider). Per §7: if validation fails, OLD config remains
// active. Per §8: invalidate cache after activation. Per §11: failure
// safety — OLD config remains active on failure.
//
// Six checks MUST pass before activation:
//  1. Model exists in discovered models (must have been discovered first)
//  2. Model supports generateContent (the generation method Mujeeb uses)
//  3. Provider output limit is known (model.OutputTokenLimit > 0)
//  4. Provider input limit is known (model.InputTokenLimit > 0)
//  5. Pricing version exists for this provider+model (GetCurrentForProvider)
//  6. Credential is not INVALID/REVOKED (GetCredentialByID + status check)
//
// Plus a real model probe: TestConnection(decryptedKey, model, baseURL) MUST
// succeed. Per §6: this is a REAL HTTP probe, not a config-only check.
//
// If any check fails, return 409 Conflict with a code identifying the
// failure. The OLD active configuration remains ACTIVE — the runtime is
// unaffected.
func (s *Server) platformUpdateAIConfiguration(ctx context.Context, in *dto.UpdateConfigurationInput) (any, bool) {
	if s.platformDeps.AIConfigRepo == nil || s.platformDeps.AIConfigCache == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI config not wired")), true
	}
	if s.platformDeps.ModelDiscovery == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "Model discovery client not wired")), true
	}
	adminID, _ := middleware.PlatformAdminID(ctx)
	provider := "google_gemini"
	now := time.Now().UTC()

	// -- Check 1: Model exists in discovered models ----------------------
	model, err := s.platformDeps.AIConfigRepo.GetModel(ctx, provider, in.Body.Model)
	if err != nil {
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", nil, nil, "FAILURE", "MODEL_NOT_FOUND", map[string]any{"model": in.Body.Model})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict, "model "+in.Body.Model+" not found in discovered models — run platformDiscoverAIModels first")), true
	}

	// -- Check 2: Model supports generateContent -------------------------
	supportsGenerate := false
	for _, m := range model.SupportedMethods {
		if m == "generateContent" {
			supportsGenerate = true
			break
		}
	}
	if !supportsGenerate {
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", nil, nil, "FAILURE", "UNSUPPORTED_GENERATION_METHOD", map[string]any{
			"model": in.Body.Model, "supported_methods": model.SupportedMethods,
		})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict, "model "+in.Body.Model+" does not support generateContent")), true
	}

	// -- Check 3: Provider output limit is known -------------------------
	if model.OutputTokenLimit == nil || *model.OutputTokenLimit <= 0 {
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", nil, nil, "FAILURE", "PROVIDER_OUTPUT_LIMIT_UNKNOWN", map[string]any{"model": in.Body.Model})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict, "provider output token limit is unknown for model "+in.Body.Model)), true
	}

	// -- Check 4: Provider input limit is known --------------------------
	if model.InputTokenLimit == nil || *model.InputTokenLimit <= 0 {
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", nil, nil, "FAILURE", "PROVIDER_INPUT_LIMIT_UNKNOWN", map[string]any{"model": in.Body.Model})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict, "provider input token limit is unknown for model "+in.Body.Model)), true
	}

	// -- Check 5: Pricing version exists --------------------------------
	if s.platformDeps.AIProviderPricing == nil {
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", nil, nil, "FAILURE", "PRICING_REPO_NOT_WIRED", map[string]any{"model": in.Body.Model})
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "AI provider pricing repository not wired")), true
	}
	pricing, pricingErr := s.platformDeps.AIProviderPricing.GetCurrentForProvider(ctx, provider, in.Body.Model)
	if pricingErr != nil {
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", nil, nil, "FAILURE", "PRICING_NOT_FOUND", map[string]any{"model": in.Body.Model})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict, "no pricing version found for model "+in.Body.Model+" — activation blocked per §10")), true
	}

	// -- Check 6: Credential is not INVALID/REVOKED ---------------------
	cred, err := s.platformDeps.AIConfigRepo.GetCredentialByID(ctx, in.Body.CredentialID)
	if err != nil {
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", nil, nil, "FAILURE", "CREDENTIAL_NOT_FOUND", map[string]any{"credential_id": in.Body.CredentialID})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict, "credential "+in.Body.CredentialID+" not found")), true
	}
	if cred.Status == "INVALID" || cred.Status == "REVOKED" {
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", nil, nil, "FAILURE", "CREDENTIAL_NOT_VALID", map[string]any{
			"credential_id": in.Body.CredentialID, "status": cred.Status,
		})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict, "credential "+in.Body.CredentialID+" has status "+cred.Status+" — cannot activate")), true
	}

	// -- Real model probe: TestConnection(decryptedKey, model, baseURL) --
	// Per §6: actual HTTP probe to Gemini — verifies that this specific
	// credential+model pair works together. This catches cases where the
	// credential is VALID for one model but not another (e.g., a model
	// not enabled for the API key's project).
	decryptedKey, err := s.platformDeps.AIConfigRepo.GetDecryptedKeyByID(ctx, in.Body.CredentialID)
	if err != nil {
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", nil, nil, "FAILURE", "CREDENTIAL_KEY_DECRYPT_FAILED", map[string]any{"credential_id": in.Body.CredentialID})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict, "failed to decrypt credential key")), true
	}
	probeBaseURL := "https://generativelanguage.googleapis.com"
	if s.platformDeps.AIConfigCache != nil {
		if activeCfg, cfgErr := s.platformDeps.AIConfigCache.GetActiveConfig(ctx); cfgErr == nil && activeCfg.BaseURL != "" {
			probeBaseURL = activeCfg.BaseURL
		}
	}
	probeSuccess, probeLatency, probeErrorCode := s.platformDeps.ModelDiscovery.TestConnection(ctx, decryptedKey, in.Body.Model, probeBaseURL)
	if !probeSuccess {
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", nil, nil, "FAILURE", "MODEL_PROBE_FAILED", map[string]any{
			"model": in.Body.Model, "error_code": probeErrorCode, "latency_ms": probeLatency,
		})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict, "model probe failed for "+in.Body.Model+" — error: "+probeErrorCode)), true
	}

	// -- Compute effective limits per §5 + §6 ---------------------------
	// effective_max_output = min(Mujeeb, Provider)
	providerOutputLimit := *model.OutputTokenLimit
	effMax := in.Body.MujeebMaxOutputTokens
	if effMax > providerOutputLimit {
		effMax = providerOutputLimit
	}
	// Note: the input capability is enforced separately — Mujeeb's
	// max_input_chars is the operational guard (see ContractClient line
	// 144), and the provider's input_token_limit is enforced by Gemini
	// itself when the request is sent. The backend here just records
	// the limits so the runtime can clamp.

	// -- Create version + activate (atomic per repo) ------------------
	pricingVersion := pricing.PricingVersion
	record, err := s.platformDeps.AIConfigRepo.CreateVersion(ctx, ports.AIConfigurationCreate{
		ID: uuid.NewString(), Provider: provider, Model: in.Body.Model,
		CredentialID: in.Body.CredentialID, MujeebMaxInputChars: in.Body.MujeebMaxInputChars,
		MujeebMaxOutputTokens:    in.Body.MujeebMaxOutputTokens,
		EffectiveMaxOutputTokens: effMax,
		PricingVersion:           &pricingVersion, CreatedBy: string(adminID),
		Now: now,
	})
	if err != nil {
		failureCode := classifyPlatformRepoErrorKind(err)
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", nil, nil, "FAILURE", failureCode, map[string]any{"model": in.Body.Model})
		return mapApplicationError(err), true
	}
	// Activate the new version (deactivates the old one atomically).
	activated, err := s.platformDeps.AIConfigRepo.ActivateVersion(ctx, record.ID, now)
	if err != nil {
		s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", &record.ID, nil, "FAILURE", classifyPlatformRepoErrorKind(err), nil)
		return mapApplicationError(err), true
	}

	// -- Per §8: invalidate cache so the next Gemini call uses the new --
	// config without restart. The next GetActiveConfig call triggers a
	// ReloadFromDB which reads the just-activated version + the
	// credential's decrypted key. Per §9: runtime switch without restart.
	s.platformDeps.AIConfigCache.Invalidate()

	s.appendPlatformAudit(ctx, "ai.configuration.updated", "ai_config", &activated.ID, nil, "SUCCESS", "", map[string]any{
		"provider": activated.Provider, "model": activated.Model, "version": activated.Version,
		"effective_max_output": activated.EffectiveMaxOutputTokens,
		"pricing_version":      pricingVersion,
		"probe_latency_ms":     probeLatency,
	})
	out := &contract.Single[dto.AIConfigurationView]{}
	out.Body.Data = dto.AIConfigurationView{
		Provider: activated.Provider, Model: activated.Model,
		CredentialID: activated.CredentialID, CredentialStatus: "VALID",
		MujeebMaxInputChars:   activated.MujeebMaxInputChars,
		MujeebMaxOutputTokens: activated.MujeebMaxOutputTokens,
		EffectiveMaxOutput:    activated.EffectiveMaxOutputTokens,
		Version:               activated.Version, Status: activated.Status,
	}
	if activated.PricingVersion != nil {
		out.Body.Data.PricingVersion = activated.PricingVersion
	}
	if activated.ActivatedAt != nil {
		activatedAtStr := activated.ActivatedAt.UTC().Format(time.RFC3339)
		out.Body.Data.ActivatedAt = &activatedAtStr
	}
	return out, true
}

// (aiModelProjection is defined above near the AI Provider Configuration DTOs.)
