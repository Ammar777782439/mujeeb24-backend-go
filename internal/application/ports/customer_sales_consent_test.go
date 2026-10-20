package ports

import "testing"

func TestSalesStartCapabilityIsExplicitAndCannotAutoCreateBeforeConsent(t *testing.T) {
	before := CustomerSalesCapabilitiesForConsent(true, true, false)
	after := CustomerSalesCapabilitiesForConsent(true, true, true)
	has := func(caps []CustomerSalesCapability, name CustomerSalesCapability) bool {
		for _, c := range caps {
			if c == name {
				return true
			}
		}
		return false
	}
	if !has(before, CustomerSalesCapabilityOfferSalesStart) ||
		has(before, CustomerSalesCapabilityRecordLead) ||
		has(before, CustomerSalesCapabilityCreateDraft) ||
		has(before, CustomerSalesCapabilityUpdateDraft) {
		t.Fatalf("wrong capabilities before consent: %v", before)
	}
	if has(after, CustomerSalesCapabilityOfferSalesStart) ||
		!has(after, CustomerSalesCapabilityRecordLead) ||
		!has(after, CustomerSalesCapabilityCreateDraft) ||
		!has(after, CustomerSalesCapabilityUpdateDraft) {
		t.Fatalf("wrong capabilities after consent: %v", after)
	}
	off := CustomerSalesCapabilitiesForConsent(true, false, false)
	if has(off, CustomerSalesCapabilityOfferSalesStart) || has(off, CustomerSalesCapabilityRecordLead) {
		t.Fatalf("sales executor was offered while disabled: %v", off)
	}
}
