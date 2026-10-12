package ports

import "testing"

func TestSalesQuantityDoesNotRoundOrAcceptRationalSyntax(t *testing.T) {
	for _, s := range []string{"1", "0.0001", "125.5000"} {
		if err := ValidateSalesQuantity(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"0", "-1", "1/3", "1e3", "0.00001", "10000000000000000", "NaN", " 1"} {
		if err := ValidateSalesQuantity(s); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
}
