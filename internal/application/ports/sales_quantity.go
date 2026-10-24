package ports

import (
	"errors"
	"math/big"
	"regexp"
)

var salesQuantityPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,15})(\.[0-9]{1,4})?$`)

// Exact NUMERIC(20,4) input: reject fractions, exponents and silent database rounding.
func ValidateSalesQuantity(value string) error {
	if !salesQuantityPattern.MatchString(value) {
		return errors.New("quantity must be a positive decimal with at most 16 integer and 4 fractional digits")
	}
	n, ok := new(big.Rat).SetString(value)
	if !ok || n.Sign() <= 0 {
		return errors.New("quantity must be positive")
	}
	return nil
}
