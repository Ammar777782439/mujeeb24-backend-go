package postgres

import "strings"

// mergeSalesTransactionData preserves previous nonempty customer answers when
// a model returns only new fields in a subsequent turn. A nil/empty string
// never clears an earlier answer; explicit nonempty corrections may replace it.
// This function is pure: no mutation of the stored proposal or incoming map.
func mergeSalesTransactionData(previous, incoming map[string]any) map[string]any {
	merged := make(map[string]any, len(previous)+len(incoming))
	for key, value := range previous {
		merged[key] = value
	}
	for key, value := range incoming {
		if value == nil {
			continue
		}
		if str, ok := value.(string); ok && strings.TrimSpace(str) == "" {
			continue
		}
		merged[key] = value
	}
	return merged
}
