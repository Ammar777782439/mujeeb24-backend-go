package merchantassist

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

func TestFullCursorScanIncludesAllCustomerPages(t *testing.T) {
	calls := []string{}
	scan, err := scanAllPages(context.Background(), func(cursor string) (commands.ListResult[int], error) {
		calls = append(calls, cursor)
		switch cursor {
		case "":
			return commands.ListResult[int]{Items: []int{1, 2, 3}, HasMore: true, NextCursor: "next"}, nil
		case "next":
			return commands.ListResult[int]{Items: []int{4, 5}, HasMore: false}, nil
		default:
			t.Fatalf("unexpected cursor: %q", cursor)
			return commands.ListResult[int]{}, nil
		}
	}, func(x int) (map[string]any, string, string, *DiscoveryLink, error) {
		return map[string]any{"id": x, "status": "active"}, fmt.Sprint(x), "active", nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ev := scan.Evidence()
	if len(calls) != 2 || calls[1] != "next" || scan.Total != 5 || scan.Statuses["active"] != 5 ||
		ev["pagination_exhausted"] != true || ev["all_records_in_sample"] != true {
		t.Fatalf("cursor scan stopped early: calls=%v evidence=%+v", calls, ev)
	}
}

func TestFullCursorScanRejectsBogusHasMore(t *testing.T) {
	scenarios := []commands.ListResult[int]{
		{Items: []int{1}, HasMore: true, NextCursor: ""},
		{Items: []int{}, HasMore: true, NextCursor: "a"},
		{Items: []int{1}, HasMore: true, NextCursor: "a"},
	}
	for _, first := range scenarios {
		calls := 0
		_, err := scanAllPages(context.Background(), func(cursor string) (commands.ListResult[int], error) {
			calls++
			if calls == 1 {
				return first, nil
			}
			return commands.ListResult[int]{Items: []int{1}, HasMore: true, NextCursor: "a"}, nil
		}, func(x int) (map[string]any, string, string, *DiscoveryLink, error) {
			return map[string]any{"id": x}, fmt.Sprint(x), "active", nil, nil
		})
		if !errors.Is(err, ErrInvalidSourceEvidence) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestFullCursorScanRejectsRepeatedIDs(t *testing.T) {
	_, err := scanAllPages(context.Background(), func(cursor string) (commands.ListResult[int], error) {
		if cursor == "" {
			return commands.ListResult[int]{Items: []int{1}, HasMore: true, NextCursor: "next"}, nil
		}
		return commands.ListResult[int]{Items: []int{1}}, nil
	}, func(x int) (map[string]any, string, string, *DiscoveryLink, error) {
		return map[string]any{"id": x}, fmt.Sprint(x), "active", nil, nil
	})
	if !errors.Is(err, ErrInvalidSourceEvidence) {
		t.Fatalf("duplicate counted: %v", err)
	}
}

func TestFullCursorScanNeverReportsIncompleteCount(t *testing.T) {
	_, err := scanAllPages(context.Background(), func(cursor string) (commands.ListResult[int], error) {
		if cursor == "" {
			return commands.ListResult[int]{Items: []int{1}, HasMore: true, NextCursor: "next"}, nil
		}
		return commands.ListResult[int]{}, errors.New("db query failed")
	}, func(x int) (map[string]any, string, string, *DiscoveryLink, error) {
		return map[string]any{"id": x}, fmt.Sprint(x), "active", nil, nil
	})
	if !errors.Is(err, ErrReadUnavailable) {
		t.Fatalf("partial count returned as successful: %v", err)
	}
}
