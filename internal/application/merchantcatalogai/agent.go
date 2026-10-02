package merchantcatalogai

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type TurnInput struct {
	BusinessID        string
	PrincipalID       string
	SessionID         string
	Message           string
	ExplicitCatalogID string
}

type TurnResult struct {
	SessionID       string
	SelectedCatalog ports.CatalogRecord
	Proposal        Proposal
}

type Agent struct {
	Sessions            SessionStore
	Selector            CatalogSelector
	Business            ports.BusinessRepository
	EntityContract      EntityContractProvider
	Authoring           MerchantCatalogAuthoringPort
	CapabilitiesFactory func(selectedCatalogID string) ports.AICapabilityDispatcher
}

func (a *Agent) HandleTurn(ctx context.Context, in TurnInput) (TurnResult, error) {
	if a == nil || a.Sessions == nil || a.Selector == nil || a.EntityContract == nil || a.Authoring == nil || a.Business == nil {
		return TurnResult{}, errors.New("merchant catalog AI is not fully configured")
	}
	if strings.TrimSpace(in.BusinessID) == "" || strings.TrimSpace(in.PrincipalID) == "" {
		return TurnResult{}, errors.New("business_id and principal_id are required")
	}
	if strings.TrimSpace(in.Message) == "" {
		return TurnResult{}, errors.New("merchant message is required")
	}

	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		var err error
		sessionID, err = a.Sessions.CreateSession(ctx, in.BusinessID, in.PrincipalID)
		if err != nil {
			return TurnResult{}, err
		}
	}

	if _, err := a.Sessions.AppendMessage(ctx, in.BusinessID, sessionID, "merchant", in.Message); err != nil {
		return TurnResult{}, err
	}

	stickyCatalogID, err := a.Sessions.GetStickyCatalogID(ctx, in.BusinessID, sessionID)
	if err != nil {
		return TurnResult{}, err
	}

	selected, err := a.Selector.Select(ctx, CatalogSelectionInput{
		BusinessID:        in.BusinessID,
		ExplicitCatalogID: in.ExplicitCatalogID,
		StickyCatalogID:   stickyCatalogID,
	})
	if err != nil {
		if errors.Is(err, ErrCatalogSelectionRequired) {
			return TurnResult{SessionID: sessionID, Proposal: Proposal{
				SchemaVersion: 1,
				Status:        StatusNeedsMoreData,
				Operation:     OperationAskMerchant,
				ResponseText:  "حدّد الكتالوج الذي تريد إدارة بياناته أولًا.",
				MissingInformation: []MissingField{{
					Path:        "target_catalog_id",
					DisplayName: "الكتالوج",
					DataType:    "uuid",
					Reason:      "يوجد أكثر من كتالوج صالح ولم يتم تحديد الكتالوج المستهدف.",
				}},
			}}, nil
		}
		return TurnResult{}, err
	}

	if in.ExplicitCatalogID != "" || stickyCatalogID == "" {
		_ = a.Sessions.SetStickyCatalogID(ctx, in.BusinessID, sessionID, selected.Catalog.ID)
	}

	history, err := a.Sessions.ListMessages(ctx, in.BusinessID, sessionID, 12)
	if err != nil {
		return TurnResult{}, err
	}

	entityContract, err := a.EntityContract.Payload(ctx)
	if err != nil {
		return TurnResult{}, err
	}

	business, err := a.Business.GetByID(ctx, in.BusinessID)
	if err != nil {
		return TurnResult{}, err
	}

	var capabilities ports.AICapabilityDispatcher
	if a.CapabilitiesFactory != nil {
		capabilities = a.CapabilitiesFactory(selected.Catalog.ID)
	}

	proposal, err := a.Authoring.Propose(ctx, MerchantCatalogAuthoringInput{
		BusinessID:      in.BusinessID,
		PrincipalID:     in.PrincipalID,
		SessionID:       sessionID,
		Message:         in.Message,
		DefaultCurrency: business.DefaultCurrency,
		SelectedCatalog: selected.Catalog,
		History:         history,
		EntityContract:  entityContract,
		Capabilities:    capabilities,
	})
	if err != nil {
		return TurnResult{}, err
	}
	if err := proposal.Validate(); err != nil {
		return TurnResult{}, err
	}

	if _, err := a.Sessions.AppendMessage(ctx, in.BusinessID, sessionID, "assistant", proposal.ResponseText); err != nil {
		return TurnResult{}, err
	}

	return TurnResult{
		SessionID:       sessionID,
		SelectedCatalog: selected.Catalog,
		Proposal:        proposal,
	}, nil
}
