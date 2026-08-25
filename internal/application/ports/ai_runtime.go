package ports

import "context"

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
