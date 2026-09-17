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
	ConversationState      *ConversationStateRecord
	RecentMessages         []AIRecentMessageEvidence
}

type AIContextBuilder interface {
	Build(context.Context, ContextBuildInput) (AIContext, error)
}

type AIPolicyEvaluator interface {
	Evaluate(AIDecisionProposal, *AIContext) AIDecisionProposal
}

type AIContext struct {
	SchemaVersion          int
	Freshness              string
	Business               AIContextBusiness
	Conversation           AIContextConversation
	Customer               AIContextCustomer
	CatalogEvidence        []AICatalogEvidence
	OfferEvidence          []AIOfferEvidence
	VariantEvidence        []AIVariantEvidence
	KnowledgeEvidence      []AIKnowledgeEvidence
	BusinessPolicyEvidence []AIBusinessPolicyEvidence
	RecentMessages         []AIRecentMessageEvidence
	PolicyEvidence         AIPolicyEvidence
	KnowledgeState         string
	ConversationState      *ConversationStateRecord
	GeneratedAt            time.Time
	ExpiresAt              time.Time
}

type AIStateProposal struct {
	Focus         *ConversationFocus      `json:"focus,omitempty"`
	Comparison    *ConversationComparison `json:"comparison,omitempty"`
	Kind          string                  `json:"kind"`
	ReferenceText string                  `json:"reference_text,omitempty"`
	Alternatives  []ConversationFocus     `json:"alternatives,omitempty"`
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

type AIKnowledgeEvidence struct {
	Reference       string
	KnowledgeKey    string
	Title           string
	Content         string
	ContentType     string
	SourceReference string
	Authority       string
	EvidenceState   string
	Version         int
	ValidFrom       time.Time
	ValidUntil      *time.Time
	RetrievedAt     time.Time
	SchemaVersion   int
}

type AIBusinessPolicyEvidence struct {
	Reference     string
	PolicyKey     string
	Category      string
	Title         string
	Summary       string
	Rules         []byte
	Authority     string
	EvidenceState string
	Version       int
	ValidFrom     time.Time
	ValidUntil    *time.Time
	RetrievedAt   time.Time
	SchemaVersion int
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

// Explicit Catalog Retrieval States for the current AI decision.
const (
	CatalogRetrievalNoneRequired          = "none_required"
	CatalogRetrievalInProgress            = "in_progress"
	CatalogRetrievalExhausted             = "exhausted"
	CatalogRetrievalSafetyBudgetExhausted = "safety_budget_exhausted"
)

// CatalogStreamState tracks an individual paginated operation cursor chain.
type CatalogStreamState struct {
	Operation    string `json:"operation"`
	StreamKey    string `json:"stream_key"`
	HasMore      bool   `json:"has_more"`
	NextCursor   string `json:"next_cursor"`
	PagesFetched int    `json:"pages_fetched"`
}

// CatalogRetrievalSession tracks all initiated catalog operations and cursor chains
// for an AI decision to determine true data-driven catalog completeness.
type CatalogRetrievalSession struct {
	Streams map[string]*CatalogStreamState `json:"streams"`
}

func NewCatalogRetrievalSession() *CatalogRetrievalSession {
	return &CatalogRetrievalSession{
		Streams: make(map[string]*CatalogStreamState),
	}
}

func (s *CatalogRetrievalSession) RecordOperation(operation, streamKey string, hasMore bool, nextCursor string) {
	if s == nil || streamKey == "" {
		return
	}
	existing, ok := s.Streams[streamKey]
	if !ok {
		s.Streams[streamKey] = &CatalogStreamState{
			Operation:    operation,
			StreamKey:    streamKey,
			HasMore:      hasMore,
			NextCursor:   nextCursor,
			PagesFetched: 1,
		}
		return
	}
	existing.HasMore = hasMore
	existing.NextCursor = nextCursor
	existing.PagesFetched++
}

func (s *CatalogRetrievalSession) State(safetyBudgetExhausted bool) string {
	if s == nil || len(s.Streams) == 0 {
		if safetyBudgetExhausted {
			return CatalogRetrievalSafetyBudgetExhausted
		}
		return CatalogRetrievalNoneRequired
	}
	if safetyBudgetExhausted {
		return CatalogRetrievalSafetyBudgetExhausted
	}
	for _, stream := range s.Streams {
		if stream.HasMore {
			return CatalogRetrievalInProgress
		}
	}
	return CatalogRetrievalExhausted
}

func (s *CatalogRetrievalSession) HasIncompleteStreams() bool {
	if s == nil || len(s.Streams) == 0 {
		return false
	}
	for _, stream := range s.Streams {
		if stream.HasMore {
			return true
		}
	}
	return false
}

func (s *CatalogRetrievalSession) IncompleteStreams() []CatalogStreamState {
	if s == nil || len(s.Streams) == 0 {
		return nil
	}
	var res []CatalogStreamState
	for _, stream := range s.Streams {
		if stream.HasMore {
			res = append(res, *stream)
		}
	}
	return res
}

func (s *CatalogRetrievalSession) AllStreams() []CatalogStreamState {
	if s == nil || len(s.Streams) == 0 {
		return nil
	}
	res := make([]CatalogStreamState, 0, len(s.Streams))
	for _, stream := range s.Streams {
		res = append(res, *stream)
	}
	return res
}

type AIDecisionProposal struct {
	IntentBase                string
	DomainContext             string
	Entities                  []byte
	EvidenceReferences        []byte
	RequestedAction           string
	ResponseText              string
	ConfidenceValue           string
	ConfidenceBand            string
	RequiresHuman             bool
	MissingInformation        []byte
	ReasonCodes               []byte
	PolicyDecision            string
	PolicyVersion             string
	KnowledgeVersion          string
	ModelReference            string
	SchemaVersion             int
	StateProposal             *AIStateProposal
	DiscoveredCatalogEvidence []AICatalogEvidence
	DiscoveredOfferEvidence   []AIOfferEvidence
	DiscoveredVariantEvidence []AIVariantEvidence
	CatalogRetrievalState     string
	CatalogStreams            []CatalogStreamState
	CatalogIncomplete         bool
	SafetyBudgetExhausted     bool
}
