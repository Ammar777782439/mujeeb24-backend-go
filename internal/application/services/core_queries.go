package services

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type GetCustomerQueryService struct{ Repository ports.CustomerRepository }

func (s GetCustomerQueryService) Handle(ctx context.Context, query queries.GetCustomerQuery) (commands.CustomerView, error) {
	if s.Repository == nil {
		return commands.CustomerView{}, appErrors.NotImplemented()
	}
	record, err := s.Repository.GetByID(ctx, string(query.Meta.Actor.BusinessID), string(query.CustomerID))
	if err != nil {
		return commands.CustomerView{}, err
	}
	return customerView(record), nil
}

type GetConversationQueryService struct {
	Repository ports.ConversationRepository
	Labels     ports.ConversationLabelRepository
}

func (s GetConversationQueryService) Handle(ctx context.Context, query queries.GetConversationQuery) (commands.ConversationView, error) {
	if s.Repository == nil {
		return commands.ConversationView{}, appErrors.NotImplemented()
	}
	record, err := s.Repository.GetByID(ctx, string(query.Meta.Actor.BusinessID), string(query.ConversationID))
	if err != nil {
		return commands.ConversationView{}, err
	}
	view := conversationView(record)
	if s.Labels != nil {
		labels, labelErr := s.Labels.List(ctx, string(query.Meta.Actor.BusinessID), string(query.ConversationID))
		if labelErr != nil {
			return commands.ConversationView{}, labelErr
		}
		view.Labels = labels
	}
	return view, nil
}

func customerView(record ports.CustomerRecord) commands.CustomerView {
	var profile struct {
		DisplayName string `json:"display_name"`
	}
	_ = json.Unmarshal(record.Profile, &profile)
	return commands.CustomerView{ID: commands.CustomerID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), DisplayName: profile.DisplayName, Status: record.Status, ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))}
}

func conversationView(record ports.ConversationRecord) commands.ConversationView {
	aiMode := ""
	if record.AIModeOverride != nil {
		aiMode = *record.AIModeOverride
	}
	return commands.ConversationView{ID: commands.ConversationID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), CustomerID: commands.CustomerID(record.CustomerID), State: record.State, Ownership: record.Ownership, AIMode: aiMode, ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))}
}

var _ queries.GetCustomerHandler = GetCustomerQueryService{}
var _ queries.GetConversationHandler = GetConversationQueryService{}
