package merchantassist

import (
	"errors"
	"log"
	"regexp"
)

var readDiagnosticSQLState = regexp.MustCompile(`^[A-Z0-9]{5}$`)

// logAssistReadFailure records safe diagnostics only. Database errors may
// contain customer fields or SQL values; their raw messages never reach logs.
func logAssistReadFailure(tool string, err error) {
	cause := "query_or_source_error"
	switch {
	case errors.Is(err, ErrReadDenied):
		cause = "permission_denied"
	case errors.Is(err, ErrInvalidReadRequest):
		cause = "invalid_read_request"
	case errors.Is(err, ErrInvalidSourceEvidence):
		cause = "invalid_source_evidence"
	case errors.Is(err, ErrReadUnavailable):
		cause = "source_unavailable"
	}
	sqlState := "none"
	var stateCarrier interface{ SQLState() string }
	if errors.As(err, &stateCarrier) {
		code := stateCarrier.SQLState()
		if readDiagnosticSQLState.MatchString(code) {
			sqlState = code
		}
	}
	log.Printf("[MerchantAssistSource] stage=read_failed tool=%s cause=%s sqlstate=%s", tool, cause, sqlState)
}
