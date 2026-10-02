package merchantcatalogai

import (
	"context"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type SessionMessage struct {
	ID         string
	SenderType string
	Text       string
	CreatedAt  time.Time
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

// MerchantCatalogDiscoveryToolDefinition describes a read-only B2B catalog
// discovery tool exposed to the merchant catalog authoring AI.
type MerchantCatalogDiscoveryToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// MerchantCatalogDiscoveryExecutionContext carries trusted server-side scope
// for a merchant catalog discovery call.
type MerchantCatalogDiscoveryExecutionContext struct {
	BusinessID     string
	ConversationID string
	PrincipalID    string
}

// MerchantCatalogDiscoveryResult contains factual read-only data returned by a
// merchant catalog discovery tool. Evidence is tracked as opaque references;
// B2C customer-sales evidence types do not cross this boundary.
type MerchantCatalogDiscoveryResult struct {
	Data                     any
	EvidenceReferences       []string
	AttributeSchemaReferences []string
	HasMore                  bool
	NextCursor               string
	Operation                string
}

// MerchantCatalogDiscoveryTool is one bounded read-only capability for B2B
// merchant catalog authoring.
type MerchantCatalogDiscoveryTool interface {
	Definition() MerchantCatalogDiscoveryToolDefinition
	Execute(context.Context, MerchantCatalogDiscoveryExecutionContext, []byte) (MerchantCatalogDiscoveryResult, error)
}

// MerchantCatalogDiscoveryPort dispatches the read-only tools available to the
// merchant catalog authoring AI. It is intentionally separate from B2C tools.
type MerchantCatalogDiscoveryPort interface {
	Definitions() []MerchantCatalogDiscoveryToolDefinition
	Execute(context.Context, MerchantCatalogDiscoveryExecutionContext, string, []byte) (MerchantCatalogDiscoveryResult, error)
}
type MerchantCatalogAuthoringInput struct {
	BusinessID      string
	PrincipalID     string
	SessionID       string
	Message         string
	DefaultCurrency string
	SelectedCatalog ports.CatalogRecord
	History         []SessionMessage
	EntityContract  []byte
	Capabilities    MerchantCatalogDiscoveryPort
}

type MerchantCatalogAuthoringPort interface {
	Propose(context.Context, MerchantCatalogAuthoringInput) (Proposal, error)
}

type ExecutionInput struct {
	BusinessID      string
	PrincipalID     string
	SessionID       string
	SelectedCatalog ports.CatalogRecord
	Proposal        Proposal
}

type ProposalExecutor interface {
	Execute(context.Context, ExecutionInput) (ExecutionResult, error)
}

type ExecutionResult struct {
	ItemID     string
	VariantIDs []string
	OfferIDs   []string
}

type CatalogSelection struct {
	Catalog ports.CatalogRecord
	Reason  string
}

type CatalogSelector interface {
	Select(context.Context, CatalogSelectionInput) (CatalogSelection, error)
}

type CatalogSelectionInput struct {
	BusinessID        string
	ExplicitCatalogID string
	StickyCatalogID   string
}
