package contracts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func validBusinessContext() BusinessContextV1 {
	return BusinessContextV1{
		Name:            "Example merchant",
		Status:          "active",
		VerticalType:    "retail",
		Timezone:        "Asia/Aden",
		DefaultCurrency: "YER",
		Locale:          "ar",
	}
}

func TestBusinessContextSQLAlignedEnums(t *testing.T) {
	b := validBusinessContext()
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"pending_setup", "active", "suspended", "archived"} {
		test := b
		test.Status = status
		if err := test.Validate(); err != nil {
			t.Errorf("valid SQL status %q was rejected: %v", status, err)
		}
	}
	for _, vertical := range []string{"retail", "travel", "services", "restaurant", "clinic", "hospitality", "education", "real_estate", "other"} {
		test := b
		test.VerticalType = vertical
		if err := test.Validate(); err != nil {
			t.Errorf("valid SQL vertical %q was rejected: %v", vertical, err)
		}
	}
	for _, modify := range []func(*BusinessContextV1){
		func(b *BusinessContextV1) { b.Name = "" },
		func(b *BusinessContextV1) { b.Status = "unknown" },
		func(b *BusinessContextV1) { b.VerticalType = "invented" },
		func(b *BusinessContextV1) { b.Timezone = " " },
		func(b *BusinessContextV1) { b.Locale = "" },
		func(b *BusinessContextV1) { b.DefaultCurrency = "yeR" },
		func(b *BusinessContextV1) { b.DefaultCurrency = "EURO" },
	} {
		test := b
		modify(&test)
		if !errors.Is(test.Validate(), ErrInvalidBusinessProjection) {
			t.Errorf("malformed business projection was accepted: %+v", test)
		}
	}
	for _, mode := range []string{"disabled", "assist", "approval", "restricted_auto"} {
		if err := (BusinessRuntimePolicyContextV1{AIMode: mode}).Validate(); err != nil {
			t.Errorf("valid SQL mode %q was rejected: %v", mode, err)
		}
	}
	if err := (BusinessRuntimePolicyContextV1{AIMode: "full_auto"}).Validate(); !errors.Is(err, ErrInvalidBusinessProjection) {
		t.Fatal("invented policy mode must fail closed")
	}
}

func jsonFieldNames(t *testing.T, value any) []string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	result := make([]string, 0, len(fields))
	for key := range fields {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func TestBusinessProjectionsExactAllowlistedJSONFields(t *testing.T) {
	profile := jsonFieldNames(t, validBusinessContext())
	wantProfile := []string{"default_currency", "locale", "name", "status", "timezone", "vertical_type"}
	if !reflect.DeepEqual(profile, wantProfile) {
		t.Fatalf("business context serialized fields changed without review: %v", profile)
	}
	policy := jsonFieldNames(t, BusinessRuntimePolicyContextV1{})
	wantPolicy := []string{
		"ai_mode", "allow_auto_confirmation", "allow_auto_lead_creation",
		"allow_auto_reply", "allow_auto_transaction_draft", "allow_sales_interactions",
		"default_human_review", "sales_workflow_enabled",
	}
	if !reflect.DeepEqual(policy, wantPolicy) {
		t.Fatalf("runtime policy serialized fields changed without review: %v", policy)
	}
	for _, field := range append(profile, policy...) {
		if strings.Contains(field, "business_id") || strings.Contains(field, "secret") ||
			strings.Contains(field, "created_at") || strings.Contains(field, "updated_at") ||
			strings.Contains(field, "provider") || strings.Contains(field, "slug") {
			t.Fatalf("unreviewed internal business field leaked into AI DTO: %s", field)
		}
	}
}

// Compare the typed projection against *migrated* PostgreSQL 16 metadata,
// not the stale schema.sql. A future changed column fails closed.
func TestBusinessProjectionFieldsExistInMigratedSchema(t *testing.T) {
	path := filepath.Join("..", "..", "..", "platform", "database", "testdata", "merchant_assist_schema_pg16.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var baseline struct {
		Columns []struct {
			Table string `json:"table"`
			Name  string `json:"name"`
			Type  string `json:"type"`
		} `json:"columns"`
		Constraints []struct {
			Table      string `json:"table"`
			Name       string `json:"name"`
			Definition string `json:"definition"`
		} `json:"constraints"`
	}
	if err := json.Unmarshal(raw, &baseline); err != nil {
		t.Fatal(err)
	}
	columnTypes := map[string]string{}
	for _, c := range baseline.Columns {
		if c.Table == "businesses" || c.Table == "business_policies" {
			columnTypes[c.Table+"."+c.Name] = c.Type
		}
	}
	for _, field := range jsonFieldNames(t, BusinessContextV1{}) {
		if columnTypes["businesses."+field] != "text" && !(field == "default_currency" && columnTypes["businesses."+field] == "character(3)") {
			t.Fatalf("review business column type for businesses.%s: %s", field, columnTypes["businesses."+field])
		}
	}
	for _, field := range jsonFieldNames(t, BusinessRuntimePolicyContextV1{}) {
		expected := "boolean"
		if field == "ai_mode" {
			expected = "text"
		}
		if columnTypes["business_policies."+field] != expected {
			t.Fatalf("review runtime policy column type business_policies.%s: got %s, wanted %s", field, columnTypes["business_policies."+field], expected)
		}
	}
	// SQL enums must not silently drift away from Go validators.
	required := map[string][]string{
		"businesses_status_chk":         {"pending_setup", "active", "suspended", "archived"},
		"businesses_vertical_type_chk":  {"retail", "travel", "services", "restaurant", "clinic", "hospitality", "education", "real_estate", "other"},
		"business_policies_ai_mode_chk": {"disabled", "assist", "approval", "restricted_auto"},
	}
	for constraint, values := range required {
		var definition string
		for _, c := range baseline.Constraints {
			if c.Name == constraint {
				definition = c.Definition
				break
			}
		}
		if definition == "" {
			t.Fatalf("missing SQL CHECK %s", constraint)
		}
		for _, v := range values {
			if !strings.Contains(definition, "'"+v+"'") {
				t.Fatalf("SQL constraint %s no longer contains enum %q", constraint, v)
			}
		}
	}
}
