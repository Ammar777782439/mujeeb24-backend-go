package ports

import (
	"reflect"
	"testing"
)

func TestSalesMessageEvidenceReferencesUsesCurrentAndExposedCustomerHistoryOnly(t *testing.T) {
	recent := []CustomerSalesRecentMessageEvidence{
		{Reference: "past-customer-1", Direction: "inbound", Origin: "customer", Text: "طلب اشتراك"},
		{Reference: "assistant-1", Direction: "outbound", Origin: "ai", Text: "أهلًا"},
		{Reference: "employee-1", Direction: "outbound", Origin: "employee", Text: "تم"},
		{Reference: "empty-1", Direction: "inbound", Origin: "customer", Text: ""},
		{Reference: "past-customer-2", Direction: "inbound", Origin: "customer", Text: "الاسم"},
		{Reference: "past-customer-2", Direction: "inbound", Origin: "customer", Text: "نسخة مكررة"},
	}
	got := SalesMessageEvidenceReferences("provider-source-current", recent)
	want := []string{"provider-source-current", "past-customer-1", "past-customer-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("allowed evidence mismatch: got=%v want=%v", got, want)
	}
}
