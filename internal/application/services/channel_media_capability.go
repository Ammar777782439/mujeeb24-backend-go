package services

import (
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// hasVerifiedChannelCapability fails closed unless a capability is enabled,
// checked, and accompanied by a non-empty evidence source.
func hasVerifiedChannelCapability(records []ports.ChannelCapabilityRecord, name string) bool {
	for _, record := range records {
		if record.Name != name || !record.Enabled || record.CheckedAt.IsZero() || record.EvidenceSource == nil {
			continue
		}
		if strings.TrimSpace(*record.EvidenceSource) != "" {
			return true
		}
	}
	return false
}
