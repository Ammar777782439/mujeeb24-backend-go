package contracts

import (
	"errors"
	"testing"
	"time"
)

func validDescriptor() ProjectionDescriptor {
	return ProjectionDescriptor{
		ID:                 "catalog.item.v1",
		Version:            1,
		SourceEntities:     []string{"catalog_items", "attribute_definitions"},
		AllowedFields:      []string{"id", "name", "attributes"},
		RequiredPermission: "catalog:read",
		AllowedTasks:       []AssistTask{TaskImproveText, TaskExplain},
		ReadFacade:         "catalog_items_read",
		MaxRows:            1,
		MaxBytes:           8192,
		MaxAge:             time.Minute,
		RedactionPolicy:    "catalog_items_v1",
		Owner:              "catalog",
	}
}

func TestRegistryDenyByDefault(t *testing.T) {
	empty, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(empty.CheckSelection("catalog.item.v1", TaskExplain, "name", "catalog:read"), ErrUnknownProjection) {
		t.Fatal("unregistered projection must fail closed")
	}
	r, err := NewRegistry(validDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.CheckSelection("catalog.item.v1", TaskExplain, "name", "catalog:read"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		task       AssistTask
		field      string
		permission string
	}{
		{TaskExplain, "password_hash", "catalog:read"},
		{TaskExplain, "name", "sales:read"},
		{TaskPrepareFields, "name", "catalog:read"},
		{TaskExplain, "name", ""},
		{"arbitrary_task", "name", "catalog:read"},
	}
	for _, c := range cases {
		if !errors.Is(r.CheckSelection("catalog.item.v1", c.task, c.field, c.permission), ErrDeniedProjection) {
			t.Fatalf("expected denial for task=%s field=%s permission=%s", c.task, c.field, c.permission)
		}
	}
}

func TestRegistryRejectsInvalidAndDuplicateContracts(t *testing.T) {
	d := validDescriptor()
	d.AllowedTasks = append(d.AllowedTasks, AssistTask("unknown"))
	if _, err := NewRegistry(d); !errors.Is(err, ErrInvalidProjection) {
		t.Fatal("unknown task must be rejected")
	}
	d = validDescriptor()
	d.AllowedFields = append(d.AllowedFields, "name")
	if _, err := NewRegistry(d); !errors.Is(err, ErrInvalidProjection) {
		t.Fatal("duplicate field must be rejected")
	}
	d = validDescriptor()
	d.RequiredPermission = ""
	if _, err := NewRegistry(d); !errors.Is(err, ErrInvalidProjection) {
		t.Fatal("missing permission must be rejected")
	}
	d = validDescriptor()
	d.MaxRows = 0
	if _, err := NewRegistry(d); !errors.Is(err, ErrInvalidProjection) {
		t.Fatal("unbounded rows must be rejected")
	}
	d = validDescriptor()
	if _, err := NewRegistry(d, d); !errors.Is(err, ErrInvalidProjection) {
		t.Fatal("duplicate contract IDs must be rejected")
	}
}

func TestRegistryDefensiveCopy(t *testing.T) {
	d := validDescriptor()
	r, err := NewRegistry(d)
	if err != nil {
		t.Fatal(err)
	}
	d.AllowedFields[0] = "password_hash"
	first, ok := r.Lookup("catalog.item.v1")
	if !ok || first.AllowedFields[0] == "password_hash" {
		t.Fatal("registry must not share caller memory")
	}
	first.AllowedFields[0] = "password_hash"
	second, ok := r.Lookup("catalog.item.v1")
	if !ok || second.AllowedFields[0] == "password_hash" {
		t.Fatal("lookup must not expose mutable registry internals")
	}
}
