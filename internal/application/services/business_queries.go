package services

import (
	"context"
	"strconv"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type GetBusinessQueryService struct{ Repository ports.BusinessRepository }

func (s GetBusinessQueryService) Handle(ctx context.Context, query queries.GetBusinessQuery) (commands.BusinessView, error) {
	if s.Repository == nil {
		return commands.BusinessView{}, appErrors.NotImplemented()
	}
	record, err := s.Repository.GetByID(ctx, string(query.Meta.Actor.BusinessID))
	if err != nil {
		return commands.BusinessView{}, err
	}
	return businessView(record), nil
}

func businessView(record ports.BusinessRecord) commands.BusinessView {
	return commands.BusinessView{ID: commands.BusinessID(record.ID), Name: record.Name, Slug: record.Slug, Status: record.Status, VerticalType: record.VerticalType, Timezone: record.Timezone, DefaultCurrency: record.DefaultCurrency, Locale: record.Locale, ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10)), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

var _ queries.GetBusinessHandler = GetBusinessQueryService{}
