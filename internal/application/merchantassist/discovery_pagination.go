package merchantassist

import (
	"context"
	"errors"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

// A reporting request must exhaust the source cursor, rather than confusing
// a UI page size with the full number of records. The small sample keeps model
// prompts bounded; count and status totals include every successfully read page.
const (
	assistScanPageSize   = 250
	assistScanMaxRecords = 20000
	assistSampleSize     = 40
	assistLinkLimit      = 12
)

var ErrFullScanTooLarge = errors.New("merchant assist complete listing exceeds safe scan budget")

type assistListScan struct {
	Total    int
	Statuses map[string]int
	Sample   []map[string]any
	Links    []DiscoveryLink
}

func (s assistListScan) Evidence() map[string]any {
	return map[string]any{
		"total": s.Total, "status_counts": s.Statuses, "sample": s.Sample,
		"sample_count": len(s.Sample), "all_records_in_sample": len(s.Sample) == s.Total,
		"pagination_exhausted": true,
	}
}

// scanAllPages guards every cursor transition, refuses duplicated record IDs,
// and fails rather than reporting a truncated total as a complete result.
func scanAllPages[T any](
	ctx context.Context,
	load func(string) (commands.ListResult[T], error),
	project func(T) (map[string]any, string, string, *DiscoveryLink, error),
) (assistListScan, error) {
	out := assistListScan{Statuses: map[string]int{}, Sample: []map[string]any{}, Links: []DiscoveryLink{}}
	cursor := ""
	seenCursors := map[string]bool{}
	seenIDs := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return assistListScan{}, err
		}
		page, err := load(cursor)
		if err != nil {
			return assistListScan{}, errors.Join(ErrReadUnavailable, err)
		}
		if len(page.Items) > assistScanPageSize || (page.HasMore && (len(page.Items) == 0 ||
			page.NextCursor == "" || page.NextCursor == cursor || seenCursors[page.NextCursor])) {
			return assistListScan{}, ErrInvalidSourceEvidence
		}
		for _, record := range page.Items {
			row, id, status, link, err := project(record)
			if err != nil {
				return assistListScan{}, err
			}
			if id == "" || seenIDs[id] || row == nil {
				return assistListScan{}, ErrInvalidSourceEvidence
			}
			seenIDs[id] = true
			out.Total++
			if out.Total > assistScanMaxRecords {
				return assistListScan{}, ErrFullScanTooLarge
			}
			if status != "" {
				out.Statuses[status]++
			}
			if len(out.Sample) < assistSampleSize {
				out.Sample = append(out.Sample, row)
			}
			if link != nil && len(out.Links) < assistLinkLimit {
				out.Links = append(out.Links, *link)
			}
		}
		if !page.HasMore {
			return out, nil
		}
		seenCursors[page.NextCursor] = true
		cursor = page.NextCursor
	}
}
