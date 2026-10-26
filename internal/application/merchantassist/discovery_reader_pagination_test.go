package merchantassist

import (
	"context"
	"fmt"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type customerPages25 struct { calls []string }
func (s *customerPages25) Handle(_ context.Context, q queries.ListCustomersQuery) (commands.ListResult[commands.CustomerView], error) {
	s.calls = append(s.calls, q.Cursor)
	if q.Limit != assistScanPageSize { return commands.ListResult[commands.CustomerView]{}, fmt.Errorf("wrong page size %d", q.Limit) }
	var first, second []commands.CustomerView
	for i := 1; i <= 25; i++ {
		row := commands.CustomerView{
			ID: commands.CustomerID(fmt.Sprintf("customer-%d", i)),
			BusinessID: "business-1",
			DisplayName: "أحمد",
			Status: "active",
		}
		if i <= 12 { first = append(first, row) } else { second = append(second, row) }
	}
	switch q.Cursor {
	case "": return commands.ListResult[commands.CustomerView]{Items: first, HasMore: true, NextCursor: "page2"}, nil
	case "page2": return commands.ListResult[commands.CustomerView]{Items: second}, nil
	default: return commands.ListResult[commands.CustomerView]{}, fmt.Errorf("invalid cursor %q", q.Cursor)
	}
}

func TestDiscoverCustomersReadsAll25AndCountsDuplicateNamesSeparately(t *testing.T) {
	pages := &customerPages25{}
	reader := &DiscoveryReader{Guard: &guardFixture{}, Customers: pages}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "customers:read"}}
	got, err := reader.Read(context.Background(), actor, "business-1", DiscoveryCall{Name: "find_customers"})
	if err != nil { t.Fatal(err) }
	evidence, ok := got.Rows.(map[string]any)
	if !ok { t.Fatalf("unexpected evidence: %T", got.Rows) }
	if evidence["total"] != 25 || evidence["pagination_exhausted"] != true ||
		evidence["all_records_in_sample"] != true || got.HasMore ||
		got.Coverage != "all_pages_scanned_non_snapshot" || len(pages.calls) != 2 ||
		pages.calls[1] != "page2" {
		t.Fatalf("not a complete count: %+v calls=%v", got, pages.calls)
	}
	rows, ok := evidence["sample"].([]map[string]any)
	if !ok || len(rows) != 25 { t.Fatalf("customer sample omitted records: %#v", evidence) }
	if len(got.Links) > assistLinkLimit { t.Fatal("too many UI links") }
}
