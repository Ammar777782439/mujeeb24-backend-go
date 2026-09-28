package services

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type ListLeadsQueryService struct{ Repository ports.LeadRepository }

func (s ListLeadsQueryService) Handle(ctx context.Context, query queries.ListLeadsQuery) (commands.ListResult[commands.LeadView], error) {
	page, err := s.Repository.List(ctx, string(query.Meta.Actor.BusinessID), query.Status, optionalIDString(query.CustomerID), query.ScoreBand, query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.LeadView]{}, mapSalesRepositoryError(err)
	}
	items := make([]commands.LeadView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, leadView(record))
	}
	return commands.ListResult[commands.LeadView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

type GetLeadQueryService struct{ Repository ports.LeadRepository }

func (s GetLeadQueryService) Handle(ctx context.Context, query queries.GetLeadQuery) (commands.LeadView, error) {
	record, err := s.Repository.Get(ctx, string(query.Meta.Actor.BusinessID), string(query.LeadID))
	if err != nil {
		return commands.LeadView{}, mapSalesRepositoryError(err)
	}
	return leadView(record), nil
}

type ListLeadAttributionsQueryService struct{ Repository ports.LeadRepository }

func (s ListLeadAttributionsQueryService) Handle(ctx context.Context, query queries.ListLeadAttributionsQuery) (commands.ListResult[queries.LeadAttributionView], error) {
	page, err := s.Repository.ListAttributions(ctx, string(query.Meta.Actor.BusinessID), string(query.LeadID), query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[queries.LeadAttributionView]{}, mapSalesRepositoryError(err)
	}
	items := make([]queries.LeadAttributionView, 0, len(page.Items))
	for _, record := range page.Items {
		item := queries.LeadAttributionView{ID: commands.ID(record.ID), LeadID: commands.LeadID(record.LeadID), SourceChannel: valueString(record.SourceChannel)}
		if record.SourceConversationID != nil {
			id := commands.ConversationID(*record.SourceConversationID)
			item.SourceConversationID = &id
		}
		items = append(items, item)
	}
	return commands.ListResult[queries.LeadAttributionView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

type ListLeadScoresQueryService struct{ Repository ports.LeadRepository }

func (s ListLeadScoresQueryService) Handle(ctx context.Context, query queries.ListLeadScoresQuery) (commands.ListResult[queries.LeadScoreView], error) {
	page, err := s.Repository.ListScores(ctx, string(query.Meta.Actor.BusinessID), string(query.LeadID), query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[queries.LeadScoreView]{}, mapSalesRepositoryError(err)
	}
	items := make([]queries.LeadScoreView, 0, len(page.Items))
	for _, record := range page.Items {
		value, parseErr := strconv.ParseFloat(record.Value, 64)
		if parseErr != nil {
			return commands.ListResult[queries.LeadScoreView]{}, appErrors.New(appErrors.CodeValidation, "lead score value is invalid")
		}
		items = append(items, queries.LeadScoreView{ID: commands.ID(record.ID), LeadID: commands.LeadID(record.LeadID), Value: value, Band: record.Band})
	}
	return commands.ListResult[queries.LeadScoreView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

type ListTransactionsQueryService struct{ Repository ports.TransactionRepository }

func (s ListTransactionsQueryService) Handle(ctx context.Context, query queries.ListTransactionsQuery) (commands.ListResult[commands.TransactionView], error) {
	page, err := s.Repository.List(ctx, string(query.Meta.Actor.BusinessID), query.State, query.TransactionType, optionalIDString(query.CustomerID), query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.TransactionView]{}, mapSalesRepositoryError(err)
	}
	items := make([]commands.TransactionView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, transactionView(record))
	}
	return commands.ListResult[commands.TransactionView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

type ListCustomerTransactionsQueryService struct{ Repository ports.TransactionRepository }

func (s ListCustomerTransactionsQueryService) Handle(ctx context.Context, query queries.ListCustomerTransactionsQuery) (commands.ListResult[commands.TransactionView], error) {
	page, err := s.Repository.List(ctx, string(query.Meta.Actor.BusinessID), "", "", string(query.CustomerID), query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.TransactionView]{}, mapSalesRepositoryError(err)
	}
	items := make([]commands.TransactionView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, transactionView(record))
	}
	return commands.ListResult[commands.TransactionView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

type GetTransactionQueryService struct{ Repository ports.TransactionRepository }

func (s GetTransactionQueryService) Handle(ctx context.Context, query queries.GetTransactionQuery) (commands.TransactionView, error) {
	record, err := s.Repository.Get(ctx, string(query.Meta.Actor.BusinessID), string(query.TransactionID))
	if err != nil {
		return commands.TransactionView{}, mapSalesRepositoryError(err)
	}
	return transactionView(record), nil
}

type GetTransactionReviewQueryService struct{ Repository ports.TransactionRepository }

func (s GetTransactionReviewQueryService) Handle(ctx context.Context, query queries.GetTransactionReviewQuery) (queries.TransactionReviewView, error) {
	record, err := s.Repository.GetReview(ctx, string(query.Meta.Actor.BusinessID), string(query.TransactionID))
	if err != nil {
		return queries.TransactionReviewView{}, mapSalesRepositoryError(err)
	}
	return queries.TransactionReviewView{Required: record.Required, Status: record.Status, ReasonCodes: jsonStrings(record.ReasonCodes), ReviewerReference: valueString(record.ReviewerReference)}, nil
}

func leadView(record ports.LeadRecord) commands.LeadView {
	return commands.LeadView{ID: commands.LeadID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), CustomerID: commands.CustomerID(record.CustomerID), Status: record.Status, ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))}
}
func transactionView(record ports.TransactionRecord) commands.TransactionView {
	return commands.TransactionView{ID: commands.TransactionID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), CustomerID: commands.CustomerID(record.CustomerID), State: record.State, TransactionType: record.TransactionType, ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))}
}
func optionalIDString(value *commands.CustomerID) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
func valueString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func jsonStrings(raw []byte) []string {
	var values []string
	if len(raw) == 0 {
		return values
	}
	_ = json.Unmarshal(raw, &values)
	return values
}
func mapSalesRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	var kinded interface{ ErrorKind() string }
	if !errors.As(err, &kinded) {
		return err
	}
	switch kinded.ErrorKind() {
	case "not_found":
		return appErrors.New(appErrors.CodeNotFound, "sales resource was not found")
	case "stale":
		return appErrors.New(appErrors.CodeStaleResource, "sales resource version is stale")
	case "conflict":
		return appErrors.New(appErrors.CodeInvalidState, "sales state transition is invalid")
	case "invalid":
		return appErrors.New(appErrors.CodeValidation, "sales persistence rejected the request")
	default:
		return err
	}
}

var _ queries.ListLeadsHandler = ListLeadsQueryService{}
var _ queries.GetLeadHandler = GetLeadQueryService{}
var _ queries.ListLeadAttributionsHandler = ListLeadAttributionsQueryService{}
var _ queries.ListLeadScoresHandler = ListLeadScoresQueryService{}
var _ queries.ListTransactionsHandler = ListTransactionsQueryService{}
var _ queries.ListCustomerTransactionsHandler = ListCustomerTransactionsQueryService{}
var _ queries.GetTransactionHandler = GetTransactionQueryService{}
var _ queries.GetTransactionReviewHandler = GetTransactionReviewQueryService{}
