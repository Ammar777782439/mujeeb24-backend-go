package services

import (
	"encoding/json"
	"testing"
	"time"
)

func TestContextEvidenceValidityBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	for _, tc := range []struct {
		name  string
		from  time.Time
		until *time.Time
		want  string
	}{
		{"not yet valid", future, nil, CustomerSalesContextMissing},
		{"valid from now", now, nil, CustomerSalesContextFresh},
		{"expires now", now.Add(-time.Hour), &now, CustomerSalesContextStale},
		{"still valid", now, &future, CustomerSalesContextFresh},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := evidenceStateForValidity(now, tc.from, tc.until); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestContextJSONHelpersRetainValidEvidence(t *testing.T) {
	for _, input := range []string{"", "broken", `{"brand":"مجيب"}`} {
		if got := safeJSONObject([]byte(input)); !json.Valid(got) {
			t.Fatalf("invalid object for %q: %s", input, got)
		}
	}
	original := []byte(`{"name":"مجيب"}`)
	copied := safeJSONDocument(original)
	original[2] = 'x'
	if string(copied) != `{"name":"مجيب"}` {
		t.Fatal("JSON document must retain an independent copy")
	}
	if normalizeArabic("أإآٱةىؤئ") != "ااااهيوي" {
		t.Fatal("Arabic normalization changed")
	}
}
