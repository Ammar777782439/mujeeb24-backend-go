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

// Catalog Retrieval State tracking has been REMOVED per contract ④ §5 + ⑥ §20.
// Per contract ⑧ §5, the operational trace (including catalog batch coverage)
// now lives in the ai_runs + ai_catalog_batches tables, NOT in the AI Proposal.
// Per contract ⑥ §20, validation is deterministic — no semantic re-matching.
// Contract ② §3 Coverage enforcement is handled by CatalogBatchController
// in services/catalog_batch_controller.go (using PartialProgressPolicy).


// AIDecisionProposal is the LEGACY proposal shape used only by ai_decisions row.
//
// Per contract ④ §5, the contract-aligned output shape is AIGeminiProposal
// (status + action + response_text + selected[]) defined in ai_gemini_contract.go.
// New code MUST use AIGeminiProposal + ValidationPipeline, not AIDecisionProposal.
//
// This struct is kept only as the persistence shape for ai_decisions (the
// business decision row), NOT as Gemini's output contract.
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
        // Deprecated Mujeeb-side tracking fields (per contract ④ §5 + ⑥ §20):
        // Removed in favor of ai_runs table (operational) + ValidationPipeline
        // (deterministic). Kept struct minimal for persistence migration.
}
