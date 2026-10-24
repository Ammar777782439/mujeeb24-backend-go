package services

import "strings"

// usageProvider preserves the provider identity reported by the active adapter.
// Missing metadata stays missing: never attribute costs to an unrelated vendor.
func usageProvider(provider string) string {
	return strings.TrimSpace(provider)
}
