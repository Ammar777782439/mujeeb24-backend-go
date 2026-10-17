package services

import (
	"context"
	"errors"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// NativeMerchantUsageRecorder is a thin application ledger: LiteLLM owns
// token pricing, while Mujeeb owns subscription association and entitlement.
// Tool-round calls are internal expenses, never customer AI replies.
type NativeMerchantUsageRecorder struct {
	Usage         ports.AIUsageRepository
	Subscriptions ports.SubscriptionRepository
	FXRateYER     string
}

func (r NativeMerchantUsageRecorder) Record(ctx context.Context, businessID, sessionID string, usage ports.CustomerSalesUsageTelemetry) error {
	if r.Usage == nil || r.Subscriptions == nil {
		return errors.New("merchant AI accounting is not configured")
	}
	page, err := r.Subscriptions.List(ctx, ports.SubscriptionListFilter{BusinessID: businessID, Status: "ACTIVE", Limit: 1})
	if err != nil {
		return err
	}
	if len(page.Items) == 0 {
		return errors.New("merchant AI has no active subscription for usage accounting")
	}
	version, usd, rate, failed := nativeProviderSpend(usage, r.FXRateYER)
	status := "success"
	if failed {
		status = "pricing_failed"
	}
	now := time.Now().UTC()
	attemptID := usage.GatewayCallID
	if attemptID == "" { attemptID = uuid.NewString() }
	correlation := "merchant:" + sessionID + ":" + attemptID
	input := ports.AIUsageAppend{
		ID: uuid.NewString(), BusinessID: businessID, SubscriptionID: page.Items[0].ID,
		Provider: usageProvider(usage.Provider), Model: usage.Model,
		InputTokens:       int64(usage.InputTokens - usage.CachedTokens),
		CachedInputTokens: int64(usage.CachedTokens), OutputTokens: int64(usage.OutputTokens),
		ModelRequests: maxNativeRequests(usage.ModelRequests), ToolCalls: 0, FinalAIReplies: 0,
		ProviderCostYER: 0, PricingVersion: version, Status: status,
		CorrelationID: &correlation, StartedAt: now, CompletedAt: now, Now: now,
	}
	if input.InputTokens < 0 {
		input.InputTokens = 0
	}
	input.GatewayCostUSD, input.GatewayFXRateYER = usd, rate
	input.GatewayCallID, input.GatewayDeploymentID = usage.GatewayCallID, usage.GatewayDeploymentID
	_, err = r.Usage.AppendRecord(ctx, input)
	return err
}

func maxNativeRequests(n int) int { if n<1 { return 1 }; return n }
