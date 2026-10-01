package prompts

import (
	"strings"
	"testing"
)

// TestCustomerSalesSystemPromptStructure verifies the prompt has the key
// sections required for the B2C Customer Sales AI agent role. Per spec §14.1
// + §14.4: the prompt must exist + contain customer-specific behavior.
func TestCustomerSalesSystemPromptStructure(t *testing.T) {
	if strings.TrimSpace(CustomerSalesSystemPrompt) == "" {
		t.Fatal("CustomerSalesSystemPrompt is empty")
	}

	requiredSections := []string{
		"السياق الذي تتلقاه",           // context list
		"Entity Contract Authority",   // authority section (per spec §5)
		"قاعدة التوجيه الهرمي",          // catalog navigation (per spec §10)
		"قاعدة الرد على منتج محدد",      // golden rule (per spec §9)
		"قاعدة منع التبديل والبديل القريب", // anti-substitution (per spec §6)
		"قاعدة عدم الثقة بأقوال العميل",  // anti-customer-claim-trust (per spec §6)
		"قاعدة فهم العميل",              // dialect + intent (per spec §8)
		"قاعدة منع التكرار الإشاري",      // anti-repetition (per spec §8)
		"قاعدة السياسات",               // policy questions (per spec §9)
		"قاعدة مقاومة التشتيت",           // anti-jailbreak
		"المخرجات",                     // output section
	}

	for _, section := range requiredSections {
		if !strings.Contains(CustomerSalesSystemPrompt, section) {
			t.Errorf("CustomerSalesSystemPrompt missing required section: %q", section)
		}
	}
}

// TestCustomerSalesSystemPromptVersion verifies the version is bumped to v10
// after the ADR-045 Entity Contract Authority cleanup.
func TestCustomerSalesSystemPromptVersion(t *testing.T) {
	if CustomerSalesSystemPromptVersion != "customer-sales-v10" {
		t.Errorf("CustomerSalesSystemPromptVersion = %q, want %q", CustomerSalesSystemPromptVersion, "customer-sales-v10")
	}
}

// TestCustomerSalesSystemPromptNoDuplicateEnumValues verifies the prompt
// does NOT repeat catalog enum values inline — per spec §3 + §5, the Entity
// Contract is the authority for enum values. Per A4 audit, the old prompt
// had these violations on line 73 (availability_status values) + lines
// 114-115 (status/action enum lists).
func TestCustomerSalesSystemPromptNoDuplicateEnumValues(t *testing.T) {
	// Per spec §5: the prompt must NOT repeat availability_status values
	// inline — the Entity Contract already defines them.
	bannedInlineAvailStatus := `availability_status = "unknown" أو "stale" أو "requires_check"`
	if strings.Contains(CustomerSalesSystemPrompt, bannedInlineAvailStatus) {
		t.Errorf("CustomerSalesSystemPrompt still contains inline availability_status values — should defer to Entity Contract")
	}

	// Per spec §5 + §11: the prompt must NOT repeat status/action enum
	// lists — responseSchema enforces them structurally (per B2B v5 pattern).
	bannedStatusEnum := "status: resolved | needs_more_data | ambiguous | not_found"
	if strings.Contains(CustomerSalesSystemPrompt, bannedStatusEnum) {
		t.Errorf("CustomerSalesSystemPrompt still contains inline status enum list — responseSchema enforces this")
	}
	bannedActionEnum := "action: answer | clarification | human_request | lead_draft | order_draft"
	if strings.Contains(CustomerSalesSystemPrompt, bannedActionEnum) {
		t.Errorf("CustomerSalesSystemPrompt still contains inline action enum list — responseSchema enforces this")
	}
}

// TestCustomerSalesSystemPromptDefersToEntityContract verifies the prompt
// explicitly declares the Entity Contract as the authority — per spec §5:
// "Entity Contract يجب أن يكون Authority حقيقيًا".
func TestCustomerSalesSystemPromptDefersToEntityContract(t *testing.T) {
	if !strings.Contains(CustomerSalesSystemPrompt, "Entity Contract Authority") {
		t.Error("CustomerSalesSystemPrompt missing dedicated Entity Contract Authority section")
	}
	if !strings.Contains(CustomerSalesSystemPrompt, "المصدر الوحيد") {
		t.Error("CustomerSalesSystemPrompt missing 'المصدر الوحيد' authority declaration")
	}
	if !strings.Contains(CustomerSalesSystemPrompt, "لا تخترع قيمًا غير موجودة في Contract") {
		t.Error("CustomerSalesSystemPrompt missing 'do not invent values not in Contract' rule")
	}
}

// TestCustomerSalesSystemPromptDefersToResponseSchema verifies the prompt
// defers to responseSchema for output shape — per spec §11 + B2B v5 pattern.
func TestCustomerSalesSystemPromptDefersToResponseSchema(t *testing.T) {
	if !strings.Contains(CustomerSalesSystemPrompt, "responseSchema") {
		t.Error("CustomerSalesSystemPrompt missing responseSchema reference — should defer to it for output shape")
	}
}

// TestFinalEvaluationSystemPromptSuffixNoHardcodedExamples verifies the
// suffix does NOT contain hardcoded product names (per spec §4 + A4 audit:
// "لا يحتوي أمثلة قد تصبح متعارضة مع الـdomain").
func TestFinalEvaluationSystemPromptSuffixNoHardcodedExamples(t *testing.T) {
	bannedProductNames := []string{"iPhone 15 Pro Max", "iPhone 16 Pro Max", "ايفون 15 برو ماكس"}
	for _, name := range bannedProductNames {
		if strings.Contains(FinalEvaluationSystemPromptSuffix, name) {
			t.Errorf("FinalEvaluationSystemPromptSuffix contains hardcoded product name %q — should use generic [X]/[Y] placeholders", name)
		}
	}
}

// TestFinalEvaluationSystemPromptSuffixNoInlineEnumValues verifies the suffix
// does NOT repeat availability_status values inline — per spec §5, the Entity
// Contract is the authority.
func TestFinalEvaluationSystemPromptSuffixNoInlineEnumValues(t *testing.T) {
	bannedInlineAvailStatus := `availability_status is "unknown", "stale", or "requires_check"`
	if strings.Contains(FinalEvaluationSystemPromptSuffix, bannedInlineAvailStatus) {
		t.Errorf("FinalEvaluationSystemPromptSuffix still contains inline availability_status values — should defer to Entity Contract")
	}
	bannedInlineAvailStatus2 := `availability is "unknown", "stale", or "requires_check"`
	if strings.Contains(FinalEvaluationSystemPromptSuffix, bannedInlineAvailStatus2) {
		t.Errorf("FinalEvaluationSystemPromptSuffix still contains inline availability values — should defer to Entity Contract")
	}
}

// TestMerchantCatalogSystemPromptUnchanged verifies the B2B prompt is NOT
// accidentally modified — per spec §12 + §15: "لا تغيير Merchant Catalog AI
// behavior بدون ضرورة مثبتة".
func TestMerchantCatalogSystemPromptUnchanged(t *testing.T) {
	if MerchantCatalogSystemPromptVersion != "merchant-catalog-v5" {
		t.Errorf("MerchantCatalogSystemPromptVersion = %q, want %q (B2B must NOT change)", MerchantCatalogSystemPromptVersion, "merchant-catalog-v5")
	}
	if strings.TrimSpace(MerchantCatalogSystemPrompt) == "" {
		t.Fatal("MerchantCatalogSystemPrompt is empty — B2B prompt must not be deleted")
	}
	// B2B prompt must still have its own authority section (not deleted).
	if !strings.Contains(MerchantCatalogSystemPrompt, "Entity Contract") {
		t.Error("MerchantCatalogSystemPrompt missing Entity Contract reference")
	}
}
