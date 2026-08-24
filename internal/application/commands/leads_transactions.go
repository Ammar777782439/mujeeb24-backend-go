package commands

type CreateLeadCommand struct {
	Meta                 CommandMeta
	CustomerID           CustomerID
	QualificationContext map[string]any
}
type UpdateLeadCommand struct {
	Meta                 CommandMeta
	LeadID               LeadID
	QualificationContext map[string]any
}
type QualifyLeadCommand struct {
	Meta               CommandMeta
	LeadID             LeadID
	Reason             string
	EvidenceReferences []string
}
type MarkLeadLostCommand struct {
	Meta       CommandMeta
	LeadID     LeadID
	LostReason string
}
type LeadResult struct {
	MutationResult
	Lead LeadView
}
type CreateLeadHandler = CommandHandler[CreateLeadCommand, LeadResult]
type UpdateLeadHandler = CommandHandler[UpdateLeadCommand, LeadResult]
type QualifyLeadHandler = CommandHandler[QualifyLeadCommand, LeadResult]
type MarkLeadLostHandler = CommandHandler[MarkLeadLostCommand, LeadResult]

type TransactionLine struct {
	CatalogItemID      CatalogItemID
	OfferID            *OfferID
	VariantID          *VariantID
	Quantity           Decimal
	SelectedAttributes map[string]any
}
type CreateTransactionDraftCommand struct {
	Meta            CommandMeta
	CustomerID      CustomerID
	LeadID          *LeadID
	TransactionType string
	Currency        *string
	Lines           []TransactionLine
}
type UpdateTransactionDraftCommand struct {
	Meta          CommandMeta
	TransactionID TransactionID
	Currency      *string
	Lines         []TransactionLine
}
type ConfirmTransactionCommand struct {
	Meta              CommandMeta
	TransactionID     TransactionID
	EvidenceReference string
	PolicyVersion     string
}
type CancelTransactionCommand struct {
	Meta          CommandMeta
	TransactionID TransactionID
	Reason        string
}
type SubmitTransactionReviewCommand struct {
	Meta          CommandMeta
	TransactionID TransactionID
	ReasonCodes   []string
}
type ApproveTransactionReviewCommand struct {
	Meta              CommandMeta
	TransactionID     TransactionID
	ReviewerReference string
	Reason            string
}
type RejectTransactionReviewCommand struct {
	Meta              CommandMeta
	TransactionID     TransactionID
	ReviewerReference string
	Reason            string
}
type TransactionResult struct {
	MutationResult
	Transaction TransactionView
}
type TransactionReviewResult struct {
	MutationResult
	Review TransactionReviewView
}
type TransactionReviewView struct {
	Required          bool
	Status            string
	ReasonCodes       []string
	ReviewerReference string
}
type CreateTransactionDraftHandler = CommandHandler[CreateTransactionDraftCommand, TransactionResult]
type UpdateTransactionDraftHandler = CommandHandler[UpdateTransactionDraftCommand, TransactionResult]
type ConfirmTransactionHandler = CommandHandler[ConfirmTransactionCommand, TransactionResult]
type CancelTransactionHandler = CommandHandler[CancelTransactionCommand, TransactionResult]
type SubmitTransactionReviewHandler = CommandHandler[SubmitTransactionReviewCommand, TransactionReviewResult]
type ApproveTransactionReviewHandler = CommandHandler[ApproveTransactionReviewCommand, TransactionReviewResult]
type RejectTransactionReviewHandler = CommandHandler[RejectTransactionReviewCommand, TransactionReviewResult]
