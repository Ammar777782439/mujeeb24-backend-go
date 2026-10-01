package merchantcatalogai

import (
	"context"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type SessionMessage struct {
	ID string
	SenderType string
	Text string
	CreatedAt time.Time
}

type SessionStore interface {
	CreateSession(context.Context, string, string) (string, error)
	ListMessages(context.Context, string, string, int) ([]SessionMessage, error)
	AppendMessage(context.Context, string, string, string, string) (string, error)
	GetStickyCatalogID(context.Context, string, string) (string, error)
	SetStickyCatalogID(context.Context, string, string, string) error
}

type EntityContractProvider interface {
	Payload(context.Context) ([]byte, error)
}

type RuntimeInput struct {
	BusinessID string
	PrincipalID string
	SessionID string
	Message string
	SelectedCatalog ports.CatalogRecord
	History []SessionMessage
	EntityContract []byte
	Capabilities ports.AICapabilityDispatcher
}

type Runtime interface {
	Decide(context.Context, RuntimeInput) (Proposal, error)
}

type ExecutionInput struct {
	BusinessID string
	PrincipalID string
	SessionID string
	SelectedCatalog ports.CatalogRecord
	Proposal Proposal
}

type ProposalExecutor interface {
	Execute(context.Context, ExecutionInput) (ExecutionResult, error)
}

type ExecutionResult struct {
	ItemID string
	VariantIDs []string
	OfferIDs []string
}

type CatalogSelection struct {
	Catalog ports.CatalogRecord
	Reason string
}

type CatalogSelector interface {
	Select(context.Context, CatalogSelectionInput) (CatalogSelection, error)
}

type CatalogSelectionInput struct {
	BusinessID string
	ExplicitCatalogID string
	StickyCatalogID string
}
