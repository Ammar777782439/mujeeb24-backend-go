package native

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Provider errors are diagnosed without ever logging request messages, account
// records, Authorization headers, or full responses. DeepSeek may echo user
// input in an error, so only a bounded, redacted error.message is retained.
var deepSeekSecretPattern = regexp.MustCompile(`(?i)(?:bearer\s+|sk-)[a-z0-9._-]{6,}`)
var deepSeekEmailPattern = regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}\b`)
var deepSeekUUIDPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
var deepSeekPhonePattern = regexp.MustCompile(`\+?[0-9][0-9 .()-]{8,}[0-9]`)
var deepSeekSafeCodePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,80}$`)

func safeDeepSeekDiagnostic(value string, maximum int) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\n", " "), "\r", " ")
	value = deepSeekSecretPattern.ReplaceAllString(value, "[REDACTED_SECRET]")
	value = deepSeekEmailPattern.ReplaceAllString(value, "[REDACTED_EMAIL]")
	value = deepSeekUUIDPattern.ReplaceAllString(value, "[REDACTED_ID]")
	value = deepSeekPhonePattern.ReplaceAllString(value, "[REDACTED_PHONE]")
	value = strings.TrimSpace(value)
	if len(value) > maximum {
		value = value[:maximum] + "…"
	}
	return value
}

func safeDeepSeekErrorFields(body []byte) (string, string, string) {
	var envelope struct {
		Error struct {
			Code    json.RawMessage `json:"code"`
			Type    string          `json:"type"`
			Message string          `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "unparseable", "unparseable", "invalid JSON error envelope"
	}
	code := strings.Trim(string(envelope.Error.Code), "\"")
	if code == "" {
		code = "unspecified"
	}
	if !deepSeekSafeCodePattern.MatchString(code) {
		code = "nonstandard"
	}
	kind := envelope.Error.Type
	if kind == "" {
		kind = "unspecified"
	}
	if !deepSeekSafeCodePattern.MatchString(kind) {
		kind = "nonstandard"
	}
	message := safeDeepSeekDiagnostic(envelope.Error.Message, 460)
	if message == "" {
		message = "no provider error message"
	}
	return code, kind, message
}
