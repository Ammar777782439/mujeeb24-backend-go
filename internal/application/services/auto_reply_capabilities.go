package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// Authorization and execution transitions must succeed before business writes.
// Passing the transaction context keeps them atomic with the decision/outbox.
func (s AutoReplyService) transitionRun(ctx context.Context, run ports.AIRunRecord, status string) error {
	if s.RunRepository == nil || run.ID == "" {
		return nil
	}
	lifecycle := NewAIRunLifecycle(s.RunRepository)
	var err error
	switch status {
	case ports.AIRunStatusValidating:
		_, err = lifecycle.MarkValidating(ctx, run.BusinessID, run.ID)
	case ports.AIRunStatusAuthorized:
		_, err = lifecycle.MarkAuthorized(ctx, run.BusinessID, run.ID)
	case ports.AIRunStatusExecuting:
		_, err = lifecycle.MarkExecuting(ctx, run.BusinessID, run.ID)
	default:
		return fmt.Errorf("unsupported auto reply transition %s", status)
	}
	if err != nil {
		return fmt.Errorf("auto reply lifecycle %s: %w", status, err)
	}
	return nil
}

func isTerminalCustomerSalesCapability(capability ports.CustomerSalesCapability) bool {
	definition, ok := ports.CustomerSalesCapabilityDefinitionFor(capability)
	return ok && definition.Terminal
}

// fullCatalogReady checks local wiring and the entity contract before offering retrieval.
// It does not make network calls or promise future provider availability.
func (s AutoReplyService) fullCatalogReady(run ports.AIRunRecord) bool {
	if s.RunRepository == nil || strings.TrimSpace(run.ID) == "" || !s.CatalogBatch.Ready() {
		return false
	}
	var payload CatalogEntityContractPayload
	return json.Unmarshal(s.EntityContractPayload, &payload) == nil && strings.TrimSpace(payload.Contract.CatalogItem.ID) != "" && strings.TrimSpace(payload.Contract.Offer.ID) != ""
}

// legacyCustomerSalesAction is the sole B2C persistence/API compatibility mapping.
// These values never select a runtime executor or drive policy/validation.
func legacyCustomerSalesAction(capability ports.CustomerSalesCapability) string {
	switch capability {
	case ports.CustomerSalesCapabilityReply:
		return "answer"
	case ports.CustomerSalesCapabilityAsk:
		return "clarification"
	case ports.CustomerSalesCapabilityHandoff:
		return "human_request"
	default:
		return "" // blocked unknown choices have no legacy action; audit preserves their name
	}
}
