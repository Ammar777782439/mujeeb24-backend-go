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
