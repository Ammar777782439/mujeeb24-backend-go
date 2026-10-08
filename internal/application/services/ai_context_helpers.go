package services

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Shared context helpers used by customer, knowledge, policy, and catalog evidence.
func normalizeArabic(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'أ', 'إ', 'آ', 'ٱ':
			b.WriteRune('ا')
		case 'ة':
			b.WriteRune('ه')
		case 'ى':
			b.WriteRune('ي')
		case 'ؤ':
			b.WriteRune('و')
		case 'ئ':
			b.WriteRune('ي')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func tokenize(text string) []string {
	normalized := normalizeArabic(strings.ToLower(text))
	fields := strings.Fields(normalized)
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, ".,!?؟:;()[]{}\"'")
		if len([]rune(field)) >= 2 {
			result = append(result, field)
		}
	}
	return result
}

func safeJSONObject(value []byte) []byte {
	if len(value) == 0 {
		return []byte(`{}`)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(value, &object) != nil {
		return []byte(`{}`)
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return []byte(`{}`)
	}
	return encoded
}

func safeJSONDocument(value []byte) []byte {
	if len(value) == 0 || !json.Valid(value) {
		return []byte(`{}`)
	}
	return append([]byte(nil), value...)
}

func evidenceStateForValidity(now, validFrom time.Time, validUntil *time.Time) string {
	if now.Before(validFrom) {
		return CustomerSalesContextMissing
	}
	if validUntil != nil && !now.Before(*validUntil) {
		return CustomerSalesContextStale
	}
	return CustomerSalesContextFresh
}

func formatInt(value int) string {
	return strconv.Itoa(value)
}
