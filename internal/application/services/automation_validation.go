package services

import (
	"encoding/json"
	"errors"
	"strings"

	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/google/uuid"
)

func validateAutomationRuleInput(name string, conditions []byte, actionKind string, payload []byte, position int) (string, []byte, string, []byte, int, error) {
	normalizedName, err := requiredCannedReplyText(name, "automation rule name", 200, false)
	if err != nil {
		return "", nil, "", nil, 0, err
	}
	conditions, err = normalizeAutomationConditions(conditions)
	if err != nil {
		return "", nil, "", nil, 0, err
	}
	actionKind, payload, err = normalizeAutomationAction(actionKind, payload)
	if err != nil {
		return "", nil, "", nil, 0, err
	}
	if position <= 0 {
		position = 100
	}
	return normalizedName, conditions, actionKind, payload, position, nil
}

func optionalAutomationName(value *string) (*string, error) {
	return optionalCannedReplyText(value, "automation rule name", 200, false)
}
func optionalAutomationStatus(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized != "active" && normalized != "disabled" {
		return nil
	}
	return &normalized
}
func optionalAutomationConditions(value []byte) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return normalizeAutomationConditions(value)
}
func optionalAutomationAction(kind *string, payload []byte) (*string, []byte, error) {
	if kind == nil && payload == nil {
		return nil, nil, nil
	}
	if kind == nil || payload == nil {
		return nil, nil, appErrors.New(appErrors.CodeValidation, "automation action kind and payload must change together")
	}
	normalizedKind, normalizedPayload, err := normalizeAutomationAction(*kind, payload)
	if err != nil {
		return nil, nil, err
	}
	return &normalizedKind, normalizedPayload, nil
}
func normalizeAutomationConditions(raw []byte) ([]byte, error) {
	var conditions struct {
		Channel      string `json:"channel"`
		TextContains string `json:"text_contains"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &conditions) != nil {
		return nil, appErrors.New(appErrors.CodeValidation, "automation conditions must be a valid JSON object")
	}
	conditions.Channel = strings.ToLower(strings.TrimSpace(conditions.Channel))
	conditions.TextContains = strings.ToLower(strings.TrimSpace(conditions.TextContains))
	if conditions.Channel == "" && conditions.TextContains == "" {
		return nil, appErrors.New(appErrors.CodeValidation, "automation conditions require channel or text_contains")
	}
	result, _ := json.Marshal(conditions)
	return result, nil
}
func normalizeAutomationAction(kind string, raw []byte) (string, []byte, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "add_label" && kind != "set_priority" && kind != "assign_human" {
		return "", nil, appErrors.New(appErrors.CodeValidation, "automation action must be add_label, set_priority, or assign_human")
	}
	if len(raw) == 0 || !json.Valid(raw) {
		return "", nil, appErrors.New(appErrors.CodeValidation, "automation action payload must be a valid JSON object")
	}
	var normalized []byte
	switch kind {
	case "add_label":
		var payload struct {
			Label string `json:"label"`
		}
		if json.Unmarshal(raw, &payload) != nil || strings.TrimSpace(payload.Label) == "" {
			return "", nil, appErrors.New(appErrors.CodeValidation, "automation add_label payload requires label")
		}
		payload.Label = strings.ToLower(strings.TrimSpace(payload.Label))
		normalized, _ = json.Marshal(payload)
	case "set_priority":
		var payload struct {
			Priority string `json:"priority"`
		}
		if json.Unmarshal(raw, &payload) != nil || !validPriority(payload.Priority) {
			return "", nil, appErrors.New(appErrors.CodeValidation, "automation set_priority payload requires valid priority")
		}
		payload.Priority = strings.ToLower(strings.TrimSpace(payload.Priority))
		normalized, _ = json.Marshal(payload)
	case "assign_human":
		var payload struct {
			AssigneePrincipalID string `json:"assignee_principal_id"`
		}
		payload.AssigneePrincipalID = strings.TrimSpace(payload.AssigneePrincipalID)
		if json.Unmarshal(raw, &payload) != nil || uuid.Validate(payload.AssigneePrincipalID) != nil {
			return "", nil, appErrors.New(appErrors.CodeValidation, "automation assign_human payload requires assignee_principal_id UUID")
		}
		normalized, _ = json.Marshal(payload)
	}
	return kind, normalized, nil
}
func ruleMatchesInbound(raw []byte, channel, text string) bool {
	var conditions struct {
		Channel      string `json:"channel"`
		TextContains string `json:"text_contains"`
	}
	if json.Unmarshal(raw, &conditions) != nil {
		return false
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	text = strings.ToLower(strings.TrimSpace(text))
	return (conditions.Channel == "" || conditions.Channel == channel) && (conditions.TextContains == "" || strings.Contains(text, conditions.TextContains))
}
func validPriority(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "normal", "high", "urgent":
		return true
	default:
		return false
	}
}
func automationReasonCode(err error) string {
	var typed *appErrors.Error
	if errors.As(err, &typed) && typed.Code == appErrors.CodeNotFound {
		return "target_not_found"
	}
	return "action_rejected"
}
