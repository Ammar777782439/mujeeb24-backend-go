package contracts

import "testing"

func TestMerchantAssistRegistryDenyByDefault(t *testing.T) {
	r, err := NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Lookup("arbitrary_sql"); ok {
		t.Fatal("unknown tool registered")
	}
	if err := r.CheckSelection("lead_evidence.v1", TaskExplain, "scores", "sales:read"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, field, perm string
		task            AssistTask
	}{
		{"lead_evidence.v1", "factors", "sales:read", TaskExplain},
		{"lead_evidence.v1", "scores", "customers:read", TaskExplain},
		{"transaction.v1", "total_amount", "sales:read", TaskPrepareFields},
		{"customer.v1", "contact_points", "customers:read", TaskExplain},
		{"conversation_history.v1", "provider_message_id", "conversations:read", TaskExplain},
	} {
		if err := r.CheckSelection(tc.id, tc.task, tc.field, tc.perm); err == nil {
			t.Fatalf("unsafe registry selection permitted: %+v", tc)
		}
	}
}
