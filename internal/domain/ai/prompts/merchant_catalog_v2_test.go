package prompts

import (
	"strings"
	"testing"
)

func TestMerchantCatalogPromptIsShortAndScoped(t *testing.T) {
	prompt := MerchantCatalogAIV2SystemPrompt
	if size := len([]rune(prompt)); size > 2600 {
		t.Fatalf("merchant catalog behavior prompt grew to %d runes; keep transport and domain schema out of system prompt", size)
	}
	if strings.Contains(prompt, "Merchant Assist") || strings.Contains(prompt, "merchant_assist") {
		t.Fatal("merchant catalog prompt must not share Merchant Assist instructions")
	}
}

func TestMerchantCatalogPromptPreservesMerchantFactsAndSafetyBoundary(t *testing.T) {
	prompt := MerchantCatalogAIV2SystemPrompt
	for _, guardrail := range []string{
		"مصادر الحقيقة",
		"سياق المحادثة",
		"أدوات القراءة",
		"بالمعنى",
		"لا تستبدل قيم التاجر",
		"لا تملأ كل الخيارات",
		"اتركه فارغًا",
		"false",
		"معلومة مشروطة",
		"الافتراضات الرسمية",
		"quote_required",
		"price_source=not_stated",
		"default_currency",
		"starting_from",
		"سعر عنصر آخر",
		"العنصر المشابه",
		"فقط إذا",
		"operation=ask_merchant",
		"لا ترسل بيانات create/update/delete",
		"لا تدّع حفظًا أو تنفيذًا",
	} {
		if !strings.Contains(prompt, guardrail) {
			t.Errorf("missing merchant catalog guardrail: %q", guardrail)
		}
	}
}
