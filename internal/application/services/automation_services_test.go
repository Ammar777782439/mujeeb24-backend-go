package services

import "testing"

func TestAutomationRuleValidation(t *testing.T) {
	conditions, err := normalizeAutomationConditions([]byte(`{"channel":" WhatsApp ","text_contains":"عرض"}`))
	if err != nil || !ruleMatchesInbound(conditions, "whatsapp", "هل يوجد عرض اليوم؟") || ruleMatchesInbound(conditions, "instagram", "هل يوجد عرض اليوم؟") || ruleMatchesInbound(conditions, "whatsapp", "مرحبا") {
		t.Fatalf("unexpected normalized/matched conditions=%s err=%v", conditions, err)
	}
	if _, err := normalizeAutomationConditions([]byte(`{}`)); err == nil {
		t.Fatal("empty automation conditions must be rejected")
	}
	if _, _, err := normalizeAutomationAction("send_message", []byte(`{"text":"unsafe"}`)); err == nil {
		t.Fatal("external message action must be rejected")
	}
	kind, payload, err := normalizeAutomationAction("set_priority", []byte(`{"priority":"HIGH"}`))
	if err != nil || kind != "set_priority" || string(payload) != `{"priority":"high"}` {
		t.Fatalf("unexpected normalized action kind=%s payload=%s err=%v", kind, payload, err)
	}
}

func TestNormalizeAutomationActionAssignHumanWhitespaceHandling(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	whitespacePayload := []byte(`{"assignee_principal_id":"  ` + validUUID + `  "}`)
	kind, payload, err := normalizeAutomationAction("assign_human", whitespacePayload)
	if err != nil {
		t.Fatalf("whitespace UUID should be accepted after trim: %v", err)
	}
	if kind != "assign_human" {
		t.Fatalf("kind=%s", kind)
	}
	if string(payload) != `{"assignee_principal_id":"`+validUUID+`"}` {
		t.Fatalf("payload not normalized trimmed: %s", payload)
	}
	// Without whitespace should also work
	kind2, payload2, err := normalizeAutomationAction("assign_human", []byte(`{"assignee_principal_id":"`+validUUID+`"}`))
	if err != nil || kind2 != "assign_human" || string(payload2) != `{"assignee_principal_id":"`+validUUID+`"}` {
		t.Fatalf("plain UUID failed: kind=%s payload=%s err=%v", kind2, payload2, err)
	}
	// Invalid JSON must be rejected
	if _, _, err := normalizeAutomationAction("assign_human", []byte(`{invalid}`)); err == nil {
		t.Fatal("invalid JSON must be rejected")
	}
	// Empty/whitespace input must be rejected
	if _, _, err := normalizeAutomationAction("assign_human", []byte(``)); err == nil {
		t.Fatal("empty payload must be rejected")
	}
	if _, _, err := normalizeAutomationAction("assign_human", []byte(`   `)); err == nil {
		t.Fatal("whitespace payload must be rejected")
	}
	if _, _, err := normalizeAutomationAction("assign_human", []byte(`{"assignee_principal_id":"   "}`)); err == nil {
		t.Fatal("whitespace-only UUID must be rejected")
	}
	if _, _, err := normalizeAutomationAction("assign_human", []byte(`{"assignee_principal_id":"not-a-uuid"}`)); err == nil {
		t.Fatal("non-UUID must be rejected")
	}
}

func TestNormalizeAutomationActionAddLabelWhitespace(t *testing.T) {
	kind, payload, err := normalizeAutomationAction("add_label", []byte(`{"label":"  VIP  "}`))
	if err != nil || kind != "add_label" || string(payload) != `{"label":"vip"}` {
		t.Fatalf("add_label whitespace: kind=%s payload=%s err=%v", kind, payload, err)
	}
	if _, _, err := normalizeAutomationAction("add_label", []byte(`{"label":"   "}`)); err == nil {
		t.Fatal("whitespace label must be rejected")
	}
}
