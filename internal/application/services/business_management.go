package services

import (
	"context"
	"strconv"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type BusinessManagementService struct {
	Repository   ports.BusinessManagementRepository
	Transactions ports.TransactionManager
}

type BusinessPolicyQueryService struct{ BusinessManagementService }
type UpdateBusinessProfileService struct{ BusinessManagementService }
type UpdateBusinessPolicyService struct{ BusinessManagementService }

func (s BusinessManagementService) GetPolicy(ctx context.Context, query queries.GetBusinessPolicyQuery) (commands.BusinessPolicyView, error) {
	if s.Repository == nil {
		return commands.BusinessPolicyView{}, appErrors.NotImplemented()
	}
	record, err := s.Repository.GetRuntimePolicy(ctx, string(query.Meta.Actor.BusinessID))
	if err != nil {
		return commands.BusinessPolicyView{}, err
	}
	return businessPolicyView(record), nil
}

func (s BusinessPolicyQueryService) Handle(ctx context.Context, query queries.GetBusinessPolicyQuery) (commands.BusinessPolicyView, error) {
	return s.GetPolicy(ctx, query)
}

func (s BusinessManagementService) UpdateProfile(ctx context.Context, command commands.UpdateBusinessProfileCommand) (commands.UpdateBusinessProfileResult, error) {
	if s.Repository == nil || s.Transactions == nil {
		return commands.UpdateBusinessProfileResult{}, appErrors.NotImplemented()
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.UpdateBusinessProfileResult{}, err
	}
	var result commands.UpdateBusinessProfileResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, updateErr := s.Repository.UpdateProfile(txCtx, ports.BusinessProfileUpdate{BusinessID: string(command.Meta.Actor.BusinessID), ExpectedVersion: expected, Name: command.Name, VerticalType: command.VerticalType, Timezone: command.Timezone, DefaultCurrency: command.DefaultCurrency, Locale: command.Locale})
		if updateErr != nil {
			return updateErr
		}
		result.Business = businessView(record)
		result.ResourceVersion = result.Business.ResourceVersion
		return nil
	})
	return result, err
}

func (s UpdateBusinessProfileService) Handle(ctx context.Context, command commands.UpdateBusinessProfileCommand) (commands.UpdateBusinessProfileResult, error) {
	return s.UpdateProfile(ctx, command)
}

func (s BusinessManagementService) UpdatePolicy(ctx context.Context, command commands.UpdateBusinessPolicyCommand) (commands.UpdateBusinessPolicyResult, error) {
	if s.Repository == nil || s.Transactions == nil {
		return commands.UpdateBusinessPolicyResult{}, appErrors.NotImplemented()
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.UpdateBusinessPolicyResult{}, err
	}
	var result commands.UpdateBusinessPolicyResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, updateErr := s.Repository.UpdateRuntimePolicy(txCtx, ports.BusinessRuntimePolicyUpdate{BusinessID: string(command.Meta.Actor.BusinessID), ExpectedVersion: expected, AIMode: command.AIMode, DefaultHumanReview: command.DefaultHumanReview, AllowAutoReply: command.AllowAutoReply, AllowAutoLeadCreation: command.AllowAutoLeadCreation, AllowAutoTransactionDraft: command.AllowAutoTransactionDraft, AllowAutoConfirmation: command.AllowAutoConfirmation})
		if updateErr != nil {
			return updateErr
		}
		result.Policy = businessPolicyView(record)
		result.ResourceVersion = result.Policy.ResourceVersion
		return nil
	})
	return result, err
}

func (s UpdateBusinessPolicyService) Handle(ctx context.Context, command commands.UpdateBusinessPolicyCommand) (commands.UpdateBusinessPolicyResult, error) {
	return s.UpdatePolicy(ctx, command)
}

func parseBusinessExpectedVersion(value *commands.ResourceVersion) (int64, error) {
	if value == nil || *value == "" {
		return 0, appErrors.New(appErrors.CodeValidation, "If-Match resource version is required")
	}
	parsed, err := strconv.ParseInt(string(*value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, appErrors.New(appErrors.CodeValidation, "If-Match resource version must be a positive integer")
	}
	return parsed, nil
}

func businessPolicyView(record ports.BusinessRuntimePolicyRecord) commands.BusinessPolicyView {
	return commands.BusinessPolicyView{BusinessID: commands.BusinessID(record.BusinessID), AIMode: record.AIMode, DefaultHumanReview: record.DefaultHumanReview, AllowAutoReply: record.AllowAutoReply, AllowAutoLeadCreation: record.AllowAutoLeadCreation, AllowAutoTransactionDraft: record.AllowAutoTransactionDraft, AllowAutoConfirmation: record.AllowAutoConfirmation, ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))}
}

var _ queries.GetBusinessPolicyHandler = BusinessPolicyQueryService{}
var _ commands.UpdateBusinessProfileHandler = UpdateBusinessProfileService{}
var _ commands.UpdateBusinessPolicyHandler = UpdateBusinessPolicyService{}
