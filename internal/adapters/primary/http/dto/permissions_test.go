package dto

import (
	"reflect"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/danielgtaylor/huma/v2"
)

func TestGrantablePermissionsSchemaTracksAuthorizationRegistry(t *testing.T) {
	schema := (GrantablePermissions{}).Schema(nil)
	assertPermissionSchema(t, schema, authorization.KnownPermissions())
}

func TestEffectivePermissionsSchemaIncludesOwnerWildcard(t *testing.T) {
	want := append([]string{authorization.PermissionAll}, authorization.KnownPermissions()...)
	schema := (Permissions{}).Schema(nil)
	assertPermissionSchema(t, schema, want)
}

func assertPermissionSchema(t *testing.T, schema *huma.Schema, want []string) {
	t.Helper()
	if schema == nil || schema.Type != huma.TypeArray || schema.Items == nil || schema.Items.Type != huma.TypeString {
		t.Fatalf("unexpected permission schema: %#v", schema)
	}
	got := make([]string, 0, len(schema.Items.Enum))
	for _, value := range schema.Items.Enum {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("permission enum contains non-string value: %#v", value)
		}
		got = append(got, text)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("permission enum drift: got=%v want=%v", got, want)
	}
}
