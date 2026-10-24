// Package salesdiag provides opt-in, value-redacted sales model diagnostics.
//
// Enable only for a short-lived test with MUJEEB_SALES_DIAGNOSTICS=true.
// Logs show which keys arrived, not customer names, phone numbers, or values.
package salesdiag

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Enabled defaults to false and is evaluated at each diagnostic point.
func Enabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("MUJEEB_SALES_DIAGNOSTICS")), "true")
}

// IsSalesTool keeps the probe off normal replies, catalog evaluation and summaries.
func IsSalesTool(name string) bool {
	switch name {
	case "record_lead", "create_transaction_draft", "update_transaction_draft":
		return true
	default:
		return false
	}
}

// DescribeArguments inspects the model-selected sales tool wire arguments.
// It intentionally does not include customer-supplied values in log output.
func DescribeArguments(raw string) string {
	var args struct {
		Sales struct {
			WorkflowID          *string           `json:"workflow_id"`
			ExpectedVersion     *int64            `json:"expected_version"`
			TransactionType     *string           `json:"transaction_type"`
			Lines               []json.RawMessage `json:"lines"`
			TransactionDataJSON *string           `json:"transaction_data_json"`
			EvidenceMessageIDs  []string          `json:"evidence_message_ids"`
		} `json:"sales"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return "args_parse=invalid_json"
	}
	s := args.Sales
	if s.TransactionDataJSON == nil {
		return fmt.Sprintf("workflow=%q expected_version=%s transaction_type=%q data_field=missing lines=%d evidence_count=%d",
			stringValue(s.WorkflowID), intValue(s.ExpectedVersion), stringValue(s.TransactionType),
			len(s.Lines), len(s.EvidenceMessageIDs))
	}
	rawData := *s.TransactionDataJSON
	var fields map[string]any
	err := json.Unmarshal([]byte(rawData), &fields)
	if err != nil || fields == nil {
		return fmt.Sprintf("workflow=%q expected_version=%s transaction_type=%q data_json_bytes=%d data_json_valid_object=false lines=%d evidence_count=%d",
			stringValue(s.WorkflowID), intValue(s.ExpectedVersion), stringValue(s.TransactionType),
			len(rawData), len(s.Lines), len(s.EvidenceMessageIDs))
	}
	return fmt.Sprintf("workflow=%q expected_version=%s transaction_type=%q data_json_bytes=%d data_json_valid_object=true data_json_empty_object=%t data=%s lines=%d evidence_count=%d",
		stringValue(s.WorkflowID), intValue(s.ExpectedVersion), stringValue(s.TransactionType),
		len(rawData), len(fields) == 0, DescribeMap(fields), len(s.Lines), len(s.EvidenceMessageIDs))
}

// DescribeMap preserves field names and whether values were supplied, but
// redacts all actual values. The source map is not mutated.
func DescribeMap(fields map[string]any) string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "keys=[] count=0"
	}
	type fieldShape struct {
		Key      string `json:"key"`
		Kind     string `json:"kind"`
		NonEmpty bool   `json:"non_empty"`
	}
	shapes := make([]fieldShape, 0, len(keys))
	for _, key := range keys {
		v := fields[key]
		shape := fieldShape{Key: key, NonEmpty: v != nil, Kind: "value"}
		switch typed := v.(type) {
		case nil:
			shape.Kind = "null"
		case string:
			shape.Kind = "string"
			shape.NonEmpty = strings.TrimSpace(typed) != ""
		case bool:
			shape.Kind = "boolean"
		case float64, float32, int, int64, json.Number:
			shape.Kind = "number"
		case []any:
			shape.Kind = "array"
			shape.NonEmpty = len(typed) != 0
		case map[string]any:
			shape.Kind = "object"
			shape.NonEmpty = len(typed) != 0
		}
		shapes = append(shapes, shape)
	}
	encoded, _ := json.Marshal(shapes)
	return fmt.Sprintf("keys=%s count=%d", string(encoded), len(keys))
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func intValue(v *int64) string {
	if v == nil {
		return "null"
	}
	return fmt.Sprintf("%d", *v)
}
