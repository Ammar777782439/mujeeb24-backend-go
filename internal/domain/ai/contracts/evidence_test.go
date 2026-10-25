package contracts

import (
	"errors"
	"testing"
	"time"
)

func validEvidence() EvidenceEnvelopeV1 {
	return EvidenceEnvelopeV1{
		SchemaVersion: EvidenceSchemaVersion,
		ResourceRef:   "item:abc",
		SourceType:    "catalog_item",
		Authority:     AuthorityMerchantRecord,
		Completeness:  EvidenceComplete,
		RetrievedAt:   time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC),
	}
}

func TestEvidenceValidationAndFreshness(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 5, 0, 0, time.UTC)
	if err := validEvidence().Validate(now); err != nil {
		t.Fatal(err)
	}
	if !validEvidence().UsableAsCurrentFact(now) {
		t.Fatal("complete non-expired evidence should be eligible for further business validation")
	}
	expired := validEvidence()
	then := now.Add(-time.Minute)
	expired.ValidUntil = &then
	if !errors.Is(expired.Validate(now), ErrExpiredEvidence) {
		t.Fatal("expired complete evidence must fail closed")
	}
	if expired.UsableAsCurrentFact(now) {
		t.Fatal("expired evidence cannot justify a current commercial claim")
	}
	for _, status := range []EvidenceCompleteness{EvidenceMissing, EvidencePartial, EvidenceStale} {
		e := validEvidence()
		e.Completeness = status
		if e.UsableAsCurrentFact(now) {
			t.Fatalf("%s cannot justify a complete current fact", status)
		}
	}
}

func TestEvidenceRejectsInvalidMetadata(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 5, 0, 0, time.UTC)
	cases := map[string]func(*EvidenceEnvelopeV1){
		"unknown version":      func(e *EvidenceEnvelopeV1) { e.SchemaVersion++ },
		"no reference":         func(e *EvidenceEnvelopeV1) { e.ResourceRef = " " },
		"no source":            func(e *EvidenceEnvelopeV1) { e.SourceType = "" },
		"unknown authority":    func(e *EvidenceEnvelopeV1) { e.Authority = "ai_generated" },
		"unknown completeness": func(e *EvidenceEnvelopeV1) { e.Completeness = "verified" },
		"future retrieval":     func(e *EvidenceEnvelopeV1) { e.RetrievedAt = now.Add(time.Minute) },
		"zero retrieval":       func(e *EvidenceEnvelopeV1) { e.RetrievedAt = time.Time{} },
		"expiration before retrieval": func(e *EvidenceEnvelopeV1) {
			d := e.RetrievedAt.Add(-time.Second)
			e.ValidUntil = &d
		},
		"blank version": func(e *EvidenceEnvelopeV1) {
			v := " "
			e.ResourceVersion = &v
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			e := validEvidence()
			edit(&e)
			if err := e.Validate(now); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
