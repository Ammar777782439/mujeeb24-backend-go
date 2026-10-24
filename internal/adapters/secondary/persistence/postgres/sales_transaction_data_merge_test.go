package postgres

import (
	"reflect"
	"testing"
)

func TestMergeSalesTransactionDataKeepsPriorAnswersAndAcceptsCorrections(t *testing.T) {
	old := map[string]any{"field_business": "متجر", "field_phone": "771234567"}
	partial := map[string]any{"field_business": "متجر جديد", "field_phone": " ", "field_city": "صنعاء"}
	got := mergeSalesTransactionData(old, partial)
	want := map[string]any{"field_business": "متجر جديد", "field_phone": "771234567", "field_city": "صنعاء"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge lost or changed answers: got=%v want=%v", got, want)
	}
	if !reflect.DeepEqual(old, map[string]any{"field_business": "متجر", "field_phone": "771234567"}) {
		t.Fatal("mutated persisted proposal")
	}
	if !reflect.DeepEqual(partial, map[string]any{"field_business": "متجر جديد", "field_phone": " ", "field_city": "صنعاء"}) {
		t.Fatal("mutated incoming proposal")
	}
	if got := mergeSalesTransactionData(old, map[string]any{"field_business": nil}); got["field_business"] != "متجر" {
		t.Fatal("null cleared an existing answer")
	}
}
