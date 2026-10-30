package gateway

import (
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestMediaSelectionAcceptsFourExactLinks(t *testing.T) {
	options := []ports.CustomerMediaOption{{MediaLinkID: "one"}, {MediaLinkID: "two"}, {MediaLinkID: "three"}, {MediaLinkID: "four"}}
	if err := validateSelectedMediaLinks([]string{"two", "one", "four", "three"}, options); err != nil {
		t.Fatalf("four valid images should be allowed: %v", err)
	}
	if err := validateSelectedMediaLinks([]string{}, options); err != nil {
		t.Fatalf("empty selection should be allowed: %v", err)
	}
	for _, bad := range [][]string{
		{"one", "two", "three", "four", "five"},
		{"one", "one"}, {"one", "outside"}, {""},
	} {
		if err := validateSelectedMediaLinks(bad, options); err == nil {
			t.Fatalf("invalid selection was accepted: %v", bad)
		}
	}
	if err := validateSelectedMediaLinks(nil, []ports.CustomerMediaOption{{MediaLinkID: "same"}, {MediaLinkID: "same"}}); err == nil {
		t.Fatal("duplicate candidate gallery evidence should fail")
	}
}
