package postgres

import "testing"

func TestAddonTermsAreFixedAndComplete(t *testing.T) {
	cases := map[string]struct{ price, replies, catalog, seats, channels int }{
		"AI_REPLIES_100":       {1500, 100, 0, 0, 0},
		"AI_REPLIES_500":       {6000, 500, 0, 0, 0},
		"CATALOG_PRODUCTS_100": {1500, 0, 100, 0, 0},
		"EXTRA_SEAT":           {500, 0, 0, 1, 0},
		"CHANNEL_PASS":         {5000, 0, 0, 0, 1},
	}
	for kind, want := range cases {
		price, replies, catalog, seats, channels, ok := addonTerms(kind)
		if !ok || price != want.price || replies != want.replies || catalog != want.catalog || seats != want.seats || channels != want.channels {
			t.Fatalf("terms for %s = (%d,%d,%d,%d,%d,%t), want %#v", kind, price, replies, catalog, seats, channels, ok, want)
		}
	}
	if _, _, _, _, _, ok := addonTerms("made_up"); ok {
		t.Fatal("unknown addon must be rejected")
	}
}
