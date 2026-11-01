package services

import (
	"errors"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

// Translate known persistence outcomes at the application boundary. Unexpected
// database/audit failures stay internal; provider/SQL details never reach users.
func mapGovernanceRepositoryError(err error, conflictMessage string) error {
	if err == nil {
		return nil
	}
	var kinded interface{ ErrorKind() string }
	if !errors.As(err, &kinded) {
		return err
	}
	switch kinded.ErrorKind() {
	case "conflict":
		return appErrors.New(appErrors.CodeConflict, conflictMessage)
	case "not_found":
		return appErrors.New(appErrors.CodeNotFound, "المنشأة أو العضو غير موجود.")
	default:
		return err
	}
}
