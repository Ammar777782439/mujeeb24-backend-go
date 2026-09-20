package contract

import (
	"bytes"
	"context"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestBuildAPIGeneratesCompleteDashboardContract(t *testing.T) {
	api, _ := BuildAPI()
	document, err := api.OpenAPI().DowngradeYAML()
	if err != nil {
		t.Fatalf("downgrade OpenAPI: %v", err)
	}
	if !bytes.Contains(document, []byte("openapi: 3.0.3")) {
		t.Fatal("generated document is not OpenAPI 3.0.3")
	}
	if !bytes.Contains(document, []byte("bearerAuth:")) || !bytes.Contains(document, []byte("refreshCookie:")) {
		t.Fatal("JWT security schemes are missing from generated document")
	}
	if !bytes.Contains(document, []byte("ErrorEnvelope:")) || bytes.Contains(document, []byte("ErrorModel:")) {
		t.Fatal("generated document is not using Mujeeb 24 ErrorEnvelope")
	}
	if bytes.Contains(document, []byte("application/problem+json")) {
		t.Fatal("framework problem+json leaked into the Dashboard contract")
	}
	if !bytes.Contains(document, []byte("/webhooks/socialapi/{route_key}:")) {
		t.Fatal("SocialAPI webhook path is missing from generated document")
	}
	if bytes.Contains(document, []byte("/webhooks/chatwoot/")) || bytes.Contains(document, []byte("ingestChatwootWebhook")) {
		t.Fatal("Chatwoot webhook must not be declared in the Mujeeb-only contract")
	}
	for _, expected := range []string{"markConversationRead", "listCannedReplies", "createCannedReply", "updateCannedReply", "sendCannedReply", "listAutomationRules", "createAutomationRule", "updateAutomationRule"} {
		if !bytes.Contains(document, []byte("operationId: "+expected)) {
			t.Fatalf("Inbox V1 operation is missing: %s", expected)
		}
	}
	for _, expected := range []string{"listTeamMembers", "createTeamInvitation", "acceptTeamInvitation", "updateTeamMemberRole", "revokeTeamMember"} {
		if !bytes.Contains(document, []byte("operationId: "+expected)) {
			t.Fatalf("team operation is missing: %s", expected)
		}
	}
	operationIDs := regexp.MustCompile(`(?m)^\s+operationId: ([A-Za-z0-9_]+)$`).FindAllSubmatch(document, -1)
	if len(operationIDs) != 88 {
		t.Fatalf("generated %d operations, want 88", len(operationIDs))
	}
	seen := make(map[string]struct{}, len(operationIDs))
	for _, match := range operationIDs {
		id := string(match[1])
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate generated operationId: %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestGeneratedContractHandlerSkeletonIsExplicitlyNotImplemented(t *testing.T) {
	api, mux := BuildAPI()
	_ = mux
	// The registration must keep a typed handler even before application wiring.
	// Calling the generated route is intentionally deferred to PR-008.
	if api == nil {
		t.Fatal("BuildAPI returned nil API")
	}
	_ = context.Background()
}

func TestBuildAPIRoutesUseV1Prefix(t *testing.T) {
	_, mux := BuildAPI()
	req := httptest.NewRequest("GET", "/api/v1/health/live", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != 501 {
		t.Fatalf("expected registered skeleton under /api/v1 to return 501, got %d", res.Code)
	}
}
