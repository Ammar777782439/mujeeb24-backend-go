package contracts

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// This validator is for typed, untrusted-to-model projection prototypes.
// Tenant authorization, field-level PII redaction and record linkage remain
// the responsibility of application facades before any provider use.
var ErrInvalidReviewedProjection = errors.New("invalid reviewed AI projection")

func requireText(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s is missing", ErrInvalidReviewedProjection, field)
	}
	return nil
}

func requireEnum(field, value string, allowed ...string) error {
	for _, choice := range allowed {
		if value == choice {
			return nil
		}
	}
	return fmt.Errorf("%w: unsupported %s", ErrInvalidReviewedProjection, field)
}

func requireEffectivePublication(status string, version int, from time.Time, until *time.Time, now time.Time) error {
	if status != "published" || version <= 0 || from.IsZero() || now.IsZero() || now.Before(from) || (until != nil && (!until.After(from) || !now.Before(*until))) {
		return fmt.Errorf("%w: not an effective published revision", ErrInvalidReviewedProjection)
	}
	return nil
}

// PostgreSQL monetary/numeric(20,4) values are transported as decimal
// strings. This only validates shape and sign, never computes totals.
var fixedDecimal4 = regexp.MustCompile(`^(?:0|[1-9][0-9]*)(?:\.[0-9]{1,4})?$`)
var fixedDecimal2 = regexp.MustCompile(`^(?:0|[1-9][0-9]*)(?:\.[0-9]{1,2})?$`)

func validateCurrency(code string) error {
	if len(code) != 3 {
		return fmt.Errorf("%w: currency must be 3 characters", ErrInvalidReviewedProjection)
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("%w: currency is not uppercase ASCII", ErrInvalidReviewedProjection)
		}
	}
	return nil
}
