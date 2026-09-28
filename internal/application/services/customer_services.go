package services

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/google/uuid"
)

type CustomerRuntimeService struct {
	Repository   ports.CustomerRuntimeRepository
	Transactions ports.TransactionManager
}
type ListCustomersQueryService struct{ CustomerRuntimeService }
type CreateCustomerCommandService struct{ CustomerRuntimeService }
type UpdateCustomerCommandService struct{ CustomerRuntimeService }
type MergeCustomerCommandService struct{ CustomerRuntimeService }

func (s ListCustomersQueryService) Handle(ctx context.Context, query queries.ListCustomersQuery) (commands.ListResult[commands.CustomerView], error) {
	if s.Repository == nil {
		return commands.ListResult[commands.CustomerView]{}, appErrors.NotImplemented()
	}
	page, err := s.Repository.List(ctx, string(query.Meta.Actor.BusinessID), query.Search, query.Status, query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.CustomerView]{}, err
	}
	items := make([]commands.CustomerView, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, customerView(item))
	}
	return commands.ListResult[commands.CustomerView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

func (s CreateCustomerCommandService) Handle(ctx context.Context, command commands.CreateCustomerCommand) (commands.CustomerResult, error) {
	if s.Repository == nil || s.Transactions == nil {
		return commands.CustomerResult{}, appErrors.NotImplemented()
	}
	profile, contacts, locale, err := customerPayload(command.Profile, command.ContactPoints, command.LocalePreference)
	if err != nil {
		return commands.CustomerResult{}, err
	}
	var result commands.CustomerResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, createErr := s.Repository.Create(txCtx, ports.CustomerCreate{ID: uuid.NewString(), BusinessID: string(command.Meta.Actor.BusinessID), Profile: profile, ContactPoints: contacts, LocalePreference: locale})
		if createErr != nil {
			return createErr
		}
		result.Customer = customerView(record)
		result.ResourceVersion = result.Customer.ResourceVersion
		return nil
	})
	return result, err
}

func (s UpdateCustomerCommandService) Handle(ctx context.Context, command commands.UpdateCustomerCommand) (commands.CustomerResult, error) {
	if s.Repository == nil || s.Transactions == nil {
		return commands.CustomerResult{}, appErrors.NotImplemented()
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.CustomerResult{}, err
	}
	profile, contacts, locale, err := customerPayload(command.Profile, command.ContactPoints, command.LocalePreference)
	if err != nil {
		return commands.CustomerResult{}, err
	}
	var result commands.CustomerResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, updateErr := s.Repository.Update(txCtx, ports.CustomerUpdate{BusinessID: string(command.Meta.Actor.BusinessID), CustomerID: string(command.CustomerID), ExpectedVersion: expected, Profile: profile, ContactPoints: contacts, LocalePreference: locale})
		if updateErr != nil {
			return updateErr
		}
		result.Customer = customerView(record)
		result.ResourceVersion = result.Customer.ResourceVersion
		return nil
	})
	return result, err
}

func (s MergeCustomerCommandService) Handle(ctx context.Context, command commands.MergeCustomerCommand) (commands.CustomerResult, error) {
	if s.Repository == nil || s.Transactions == nil {
		return commands.CustomerResult{}, appErrors.NotImplemented()
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.CustomerResult{}, err
	}
	if strings.TrimSpace(command.Reason) == "" {
		return commands.CustomerResult{}, appErrors.New(appErrors.CodeValidation, "merge reason is required")
	}
	var result commands.CustomerResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, mergeErr := s.Repository.Merge(txCtx, ports.CustomerMerge{BusinessID: string(command.Meta.Actor.BusinessID), CustomerID: string(command.CustomerID), TargetCustomerID: string(command.TargetCustomerID), ExpectedVersion: expected, Reason: command.Reason})
		if mergeErr != nil {
			return mergeErr
		}
		result.Customer = customerView(record)
		result.ResourceVersion = result.Customer.ResourceVersion
		return nil
	})
	return result, err
}

func customerPayload(profile map[string]any, points []commands.ContactPoint, locale string) ([]byte, []byte, *string, error) {
	profileBytes := []byte(nil)
	var err error
	if profile != nil {
		profileBytes, err = json.Marshal(profile)
		if err != nil {
			return nil, nil, nil, appErrors.New(appErrors.CodeValidation, "customer profile is invalid")
		}
	}
	contactsBytes := []byte(nil)
	if points != nil {
		values := make([]map[string]string, 0, len(points))
		for _, point := range points {
			if strings.TrimSpace(point.Kind) == "" || strings.TrimSpace(point.Value) == "" {
				return nil, nil, nil, appErrors.New(appErrors.CodeValidation, "contact point kind and value are required")
			}
			values = append(values, map[string]string{"kind": strings.TrimSpace(point.Kind), "value_normalized": strings.TrimSpace(point.Value), "verification_status": "unverified"})
		}
		contactsBytes, err = json.Marshal(values)
		if err != nil {
			return nil, nil, nil, appErrors.New(appErrors.CodeValidation, "customer contact points are invalid")
		}
	}
	var localePtr *string
	if strings.TrimSpace(locale) != "" {
		normalized := strings.TrimSpace(locale)
		localePtr = &normalized
	}
	return profileBytes, contactsBytes, localePtr, nil
}

var _ queries.ListCustomersHandler = ListCustomersQueryService{}
var _ commands.CreateCustomerHandler = CreateCustomerCommandService{}
var _ commands.UpdateCustomerHandler = UpdateCustomerCommandService{}
var _ commands.MergeCustomerHandler = MergeCustomerCommandService{}
