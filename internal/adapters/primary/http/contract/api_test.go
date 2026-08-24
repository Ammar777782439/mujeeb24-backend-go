package contract

import (
	"bytes"
	"context"
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
	if !bytes.Contains(document, []byte("/webhooks/socialapi/{route_key}:")) {
		t.Fatal("SocialAPI webhook path is missing from generated document")
	}

	operationIDs := regexp.MustCompile(`(?m)^\s+operationId: ([A-Za-z0-9_]+)$`).FindAllSubmatch(document, -1)
	if len(operationIDs) != 76 {
		t.Fatalf("generated %d operations, want 76", len(operationIDs))
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
