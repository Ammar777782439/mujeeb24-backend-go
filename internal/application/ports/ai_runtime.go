package ports

import (
	"context"
	"time"
)

type AIRuntime interface {
	Decide(context.Context, AIDecisionInput) (AIDecisionProposal, error)
}

type AIDecisionInput struct {
	BusinessID             string
	ConversationID         string
	SourceMessageReference string
	Text                   string
	Channel                string
	PolicyVersion          string
	Context                *AIContext
}

type ContextBuildInput struct {
	BusinessID             string
	ConversationID         string
	SourceMessageReference string
	Text                   string
	Channel                string
	PolicyVersion          string
}

type AIContextBuilder interface {
	Build(context.Context, ContextBuildInput) (AIContext, error)
}

type AIContext struct {
	SchemaVersion   int
	Freshness       string
	Business        AIContextBusiness
	Conversation    AIContextConversation
	Customer        AIContextCustomer
	CatalogEvidence []AICatalogEvidence
	OfferEvidence   []AIOfferEvidence
	VariantEvidence []AIVariantEvidence
	RecentMessages  []AIRecentMessageEvidence
	PolicyEvidence  AIPolicyEvidence
	KnowledgeState  string
	GeneratedAt     time.Time
	ExpiresAt       time.Time
}

type AIContextBusiness struct {
	Reference       string
	Name            string
	VerticalType    string
	Locale          string
	DefaultCurrency string
}

type AIContextConversation struct {
	Reference           string
	CustomerReference   string
	State               string
	Ownership           string
	Priority            string
	AIModeOverride      string
	AssignmentReference string
}

type AIContextCustomer struct {
	Reference        string
	LocalePreference string
	Status           string
	Profile          []byte
	ContactPoints    []byte
}

type AICatalogEvidence struct {
	Reference        string
	CatalogReference string
	ItemType         string
	Name             string
	Status           string
	Attributes       []byte
	EvidenceState    string
	RetrievedAt      time.Time
	SchemaVersion    int
}

type AIOfferEvidence struct {
	Reference            string
	CatalogItemReference string
	VariantReference     string
	Name                 string
	PricingMode          string
	Amount               string
	Currency             string
	AvailabilityState    string
	Status               string
	EvidenceState        string
	RetrievedAt          time.Time
	SchemaVersion        int
}

type AIVariantEvidence struct {
	Reference            string
	CatalogItemReference string
	Name                 string
	Status               string
	Attributes           []byte
	EvidenceState        string
	RetrievedAt          time.Time
	SchemaVersion        int
}

type AIRecentMessageEvidence struct {
	Reference     string
	Direction     string
	Origin        string
	Text          string
	OccurredAt    time.Time
	EvidenceState string
	SchemaVersion int
}

type AIPolicyEvidence struct {
	Reference     string
	Version       string
	State         string
	MissingReason string
	RetrievedAt   time.Time
	SchemaVersion int
}

type AIDecisionProposal struct {
	IntentBase         string
	DomainContext      string
	Entities           []byte
	EvidenceReferences []byte
	RequestedAction    string
	ResponseText       string
	ConfidenceValue    string
	ConfidenceBand     string
	RequiresHuman      bool
	MissingInformation []byte
	ReasonCodes        []byte
	PolicyDecision     string
	PolicyVersion      string
	KnowledgeVersion   string
	ModelReference     string
	SchemaVersion      int
}
