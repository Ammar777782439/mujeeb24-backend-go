package handlers

import "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"

// catalogInitialMediaStrings only converts already schema-validated UUID DTOs.
// Tenant ownership and asset readiness are validated in the DB transaction.
func catalogInitialMediaStrings(ids []contract.UUID) []string {
	if len(ids) == 0 {
		return nil
	}
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		result = append(result, string(id))
	}
	return result
}
