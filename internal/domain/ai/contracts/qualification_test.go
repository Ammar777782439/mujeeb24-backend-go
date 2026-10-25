package contracts

import (
	"errors"
	"testing"
)

func TestQualificationEvidenceStrictArrayNoRawText(t *testing.T) {
	v, err := ParseQualificationReferences([]byte(`["source-1","source-2"]`))
	if err != nil || v.ReferenceCount != 2 || v.VerificationState != "unverified_references" {
		t.Fatalf("incorrect qualification reference state: %+v %v", v, err)
	}
	for _, raw := range []string{`{"note":"secret"}`, `["dup","dup"]`, `[""]`, `null`, `[" x"]`, `[45]`, `["x","x"]`} {
		if _, err := ParseQualificationReferences([]byte(raw)); !errors.Is(err, ErrQualificationEvidence) {
			t.Fatalf("invalid JSONB evidence allowed: %s", raw)
		}
	}
}
