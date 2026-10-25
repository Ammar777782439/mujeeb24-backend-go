package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestLeadEvidenceExactJSONFieldsAndMigratedSQLTypes(t *testing.T) {
	type field struct {
		SQL  string
		Type string
	}
	cases := []struct {
		table    string
		value    any
		expected map[string]field
	}{
		{"lead_attributions", LeadAttributionAssistV1{}, map[string]field{
			"attribution_ref":  {"id", "uuid"},
			"lead_ref":         {"lead_id", "uuid"},
			"channel":          {"source_channel", "text"},
			"conversation_ref": {"source_conversation_id", "uuid"},
			"catalog_item_ref": {"catalog_item_id", "uuid"},
			"offer_ref":        {"offer_id", "uuid"},
			"captured_at":      {"captured_at", "timestamp with time zone"},
		}},
		{"lead_scores", LeadScoreSnapshotAssistV1{}, map[string]field{
			"score_ref":     {"id", "uuid"},
			"lead_ref":      {"lead_id", "uuid"},
			"value":         {"value", "numeric(5,2)"},
			"band":          {"band", "text"},
			"calculated_at": {"calculated_at", "timestamp with time zone"},
		}},
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "platform", "database", "testdata", "merchant_assist_schema_pg16.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Columns []struct {
			Table string `json:"table"`
			Name  string `json:"name"`
			Type  string `json:"type"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	columnTypes := map[string]string{}
	for _, c := range snapshot.Columns {
		columnTypes[c.Table+"."+c.Name] = c.Type
	}
	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			typ := reflect.TypeOf(tc.value)
			actual := make([]string, 0, typ.NumField())
			for i := 0; i < typ.NumField(); i++ {
				key := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
				spec, known := tc.expected[key]
				if key == "" || !known {
					t.Fatalf("field %s was not approved for AI", typ.Field(i).Name)
				}
				actual = append(actual, key)
				if got := columnTypes[tc.table+"."+spec.SQL]; got != spec.Type {
					t.Fatalf("%s.%s changed type from %s to %s", tc.table, spec.SQL, spec.Type, got)
				}
			}
			expected := make([]string, 0, len(tc.expected))
			for fieldName := range tc.expected {
				expected = append(expected, fieldName)
			}
			sort.Strings(actual)
			sort.Strings(expected)
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("reviewed fields changed: got %v want %v", actual, expected)
			}
		})
	}
}

func TestLeadEvidenceContractRefusesUnsupportedCommercialMeaning(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	score := LeadScoreSnapshotAssistV1{
		ScoreRef: "score", LeadRef: "lead", Band: "high",
		Value: "95.25", CalculatedAt: now.Add(-time.Minute),
	}
	if err := score.Validate(now); err != nil {
		t.Fatal(err)
	}
	score.Value = "100.01"
	if err := score.Validate(now); err == nil {
		t.Fatal("scoring outside 0-100 must be rejected")
	}
	attr := LeadAttributionAssistV1{
		AttributionRef: "attribution", LeadRef: "lead",
		CapturedAt: now.Add(-time.Minute),
	}
	if err := attr.Validate(now); err != nil {
		t.Fatal(err)
	}
	channel := "other-channel"
	attr.Channel = &channel
	if err := attr.Validate(now); err == nil {
		t.Fatal("unknown channel must be rejected")
	}
}
