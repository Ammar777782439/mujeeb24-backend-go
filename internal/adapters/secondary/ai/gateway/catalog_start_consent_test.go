package gateway

import (
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func TestFinalCatalogToolsRequireSalesStartConsent(t *testing.T) {
	ctx := ports.CustomerSalesContext{Sales: &ports.SalesWorkflowContext{Enabled: true, AllowInteractions: true}}
	builder := &catalogRequestBuilder{}
	config := &catalogRequestConfig{model: "test-model", maxOutputTokens: 256}
	tools := func() map[string]bool {
		request := builder.buildFinalEvaluateRequest(services.FinalEvaluationInput{ConversationContext: ctx}, "customer asks", config)
		result := make(map[string]bool)
		for _, tool := range request.capabilityTools {
			result[tool.Name] = true
		}
		return result
	}
	before := tools()
	if !before[string(ports.CustomerSalesCapabilityOfferSalesStart)] ||
		before[string(ports.CustomerSalesCapabilityRecordLead)] ||
		before[string(ports.CustomerSalesCapabilityCreateDraft)] ||
		before[string(ports.CustomerSalesCapabilityUpdateDraft)] {
		t.Fatalf("full-catalog final offered unauthorized sales tools: %v", before)
	}
	ctx.Sales.StartRequestID = "server-owned-consent"
	after := tools()
	if after[string(ports.CustomerSalesCapabilityOfferSalesStart)] ||
		!after[string(ports.CustomerSalesCapabilityRecordLead)] ||
		!after[string(ports.CustomerSalesCapabilityCreateDraft)] ||
		!after[string(ports.CustomerSalesCapabilityUpdateDraft)] {
		t.Fatalf("full-catalog final suppressed authorized workflow tools: %v", after)
	}
}
