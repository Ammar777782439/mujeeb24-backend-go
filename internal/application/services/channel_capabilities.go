package services

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type ConnectionCapabilitiesQueryService struct {
	Repository ports.ChannelCapabilityRepository
}

func (s ConnectionCapabilitiesQueryService) Handle(ctx context.Context, query queries.GetConnectionCapabilitiesQuery) (commands.ListResult[queries.ConnectionCapabilityView], error) {
	if s.Repository == nil {
		return commands.ListResult[queries.ConnectionCapabilityView]{}, appErrors.NotImplemented()
	}
	records, err := s.Repository.ListByConnection(ctx, string(query.Meta.Actor.BusinessID), string(query.ConnectionID))
	if err != nil {
		return commands.ListResult[queries.ConnectionCapabilityView]{}, err
	}
	items := make([]queries.ConnectionCapabilityView, 0, len(records))
	for _, record := range records {
		evidence := ""
		if record.EvidenceSource != nil {
			evidence = *record.EvidenceSource
		}
		items = append(items, queries.ConnectionCapabilityView{
			Name:           record.Name,
			Enabled:        record.Enabled,
			CheckedAt:      record.CheckedAt,
			EvidenceSource: evidence,
		})
	}
	return commands.ListResult[queries.ConnectionCapabilityView]{Items: items}, nil
}

var _ queries.GetConnectionCapabilitiesHandler = ConnectionCapabilitiesQueryService{}
