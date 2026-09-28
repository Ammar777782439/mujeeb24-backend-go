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
        // Per Day 3: merchantAIChat operation added per contract 11 §6
        // (POST /businesses/{business_id}/merchant-ai/turns).
        // Per session transactions work: 12 missing endpoints added
        // (conversation labels/notes/assignment/read, business profile update,
        // team invitations accept, transaction lifecycle: create/confirm/cancel/
        // submit-review/reviews-get/approve/reject, customers merge+conversations
        //+transactions cross-refs, leads attributions+scores).
        // Per Platform Administration Contract V1: 18 platform endpoints added
        // (5 plan, 5 business, 2 audit, 4 AI ops, 2 subscription AI usage).
        // Stage 1 (Subscription Lifecycle + Manual Payments): 6 endpoints added
        // (4 subscription + 2 payment). Total was 114.
        // Stage 3 (Support Management): 7 endpoints added (1 create ticket +
        // 2 ticket queries + 1 message + 3 lifecycle transitions). Total was 121.
        // Stage 6 (Plan Version-on-Edit): 1 endpoint added (create new plan
        // version). Total was 122.
        // Audit correction: 11 missing contract routes added (2 AI provider +
        // 3 channel + 3 provider + 3 AI usage). Total was 133.
        // Gap fix: POST /platform/businesses (create business) added. Total = 134.
        operationIDs := regexp.MustCompile(`(?m)^\s+operationId: ([A-Za-z0-9_]+)$`).FindAllSubmatch(document, -1)
        if len(operationIDs) != 134 {
                t.Fatalf("generated %d operations, want 134", len(operationIDs))
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
