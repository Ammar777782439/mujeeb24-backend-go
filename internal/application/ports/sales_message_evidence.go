package ports

import "strings"

// SalesMessageEvidenceReferences returns only message references actually
// exposed to the customer-sales model and valid as customer-origin evidence.
// The current message uses its provider source reference; previous messages
// use the internal reference serialized in recent_messages. These distinct
// namespaces must never be guessed or silently remapped.
func SalesMessageEvidenceReferences(sourceReference string, recent []CustomerSalesRecentMessageEvidence) []string {
	seen := make(map[string]struct{}, len(recent)+1)
	refs := make([]string, 0, len(recent)+1)
	add := func(ref string) {
		if strings.TrimSpace(ref) == "" {
			return
		}
		if _, exists := seen[ref]; exists {
			return
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	add(sourceReference)
	for _, message := range recent {
		if message.Direction == "inbound" && message.Origin == "customer" && strings.TrimSpace(message.Text) != "" {
			add(message.Reference)
		}
	}
	return refs
}
