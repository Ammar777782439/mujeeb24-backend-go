package ports

import (
	"context"
	"time"
)

type LeadRecord struct {
	ID                    string
	BusinessID            string
	CustomerID            string
	Status                string
	QualificationState    string
	CurrentScoreValue     *string
	CurrentScoreBand      *string
	ScoreRuleVersion      *string
	ScoreModelReference   *string
	QualificationContext  []byte
	CreatedBy             string
	QualifiedBy           *string
	QualificationReason   *string
	QualificationEvidence []byte
	LostReason            *string

	SourceConversationReferenceID *string
	SourceChannel                 *string
	IntentReference               *string
	AssignedOwnershipReference    *string
	NextActionAt                  *time.Time
	ResourceVersion               int64
	CreatedAt                     time.Time
	UpdatedAt                     time.Time
}

type LeadPage struct {
	Items      []LeadRecord
	NextCursor string
	HasMore    bool
}

type LeadAttributionRecord struct {
	ID                         string
	BusinessID                 string
	LeadID                     string
	SourceConversationID       *string
	SourceChannel              *string
	SourceInteractionReference *string
	CatalogItemID              *string
	OfferID                    *string
	CampaignReference          *string
	CapturedAt                 time.Time
}

type LeadAttributionPage struct {
	Items      []LeadAttributionRecord
	NextCursor string
	HasMore    bool
}

type LeadScoreRecord struct {
	ID             string
	BusinessID     string
	LeadID         string
	Value          string
	Band           string
	Factors        []byte
	RuleVersion    *string
	ModelReference *string
	CalculatedAt   time.Time
	CreatedAt      time.Time
}

type LeadScorePage struct {
	Items      []LeadScoreRecord
	NextCursor string
	HasMore    bool
}

type LeadDraft struct {
	ID                   string
	BusinessID           string
	CustomerID           string
	QualificationContext []byte
	CreatedBy            string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type LeadPatch struct {
	ID                   string
	BusinessID           string
	QualificationContext []byte
	ExpectedVersion      int64
	UpdatedAt            time.Time
}

type LeadQualificationPatch struct {
	ID                    string
	BusinessID            string
	QualifiedBy           string
	QualificationReason   string
	QualificationEvidence []byte
	ExpectedVersion       int64
	UpdatedAt             time.Time
}

type LeadLostPatch struct {
	ID              string
	BusinessID      string
	LostReason      string
	ExpectedVersion int64
	UpdatedAt       time.Time
}

type LeadRepository interface {
	List(ctx context.Context, businessID, status, customerID, scoreBand string, limit int, cursor string) (LeadPage, error)
	Get(ctx context.Context, businessID, leadID string) (LeadRecord, error)
	Create(ctx context.Context, draft LeadDraft) (LeadRecord, error)
	Update(ctx context.Context, patch LeadPatch) (LeadRecord, error)
	Qualify(ctx context.Context, patch LeadQualificationPatch) (LeadRecord, error)
	MarkLost(ctx context.Context, patch LeadLostPatch) (LeadRecord, error)
	ListAttributions(ctx context.Context, businessID, leadID string, limit int, cursor string) (LeadAttributionPage, error)
	ListScores(ctx context.Context, businessID, leadID string, limit int, cursor string) (LeadScorePage, error)
}

type TransactionRecord struct {
	ID                            string
	BusinessID                    string
	CustomerID                    string
	LeadID                        *string
	TransactionType               string
	State                         string
	SourceConversationReferenceID *string
	Currency                      *string
	TotalAmount                   *string
	SchemaVersion                 int
	RequiresHumanReview           bool
	CancellationReason            *string
	ResourceVersion               int64
	CreatedAt                     time.Time
	UpdatedAt                     time.Time
}

type TransactionPage struct {
	Items      []TransactionRecord
	NextCursor string
	HasMore    bool
}

type TransactionLineRecord struct {
	ID                         string
	BusinessID                 string
	TransactionID              string
	CatalogItemID              string
	OfferID                    *string
	VariantID                  *string
	ItemNameSnapshot           string
	SelectedAttributesSnapshot []byte
	PricingSnapshot            []byte
	AvailabilitySnapshot       []byte
	FulfillmentSnapshot        []byte
	Quantity                   string
	UnitPriceSnapshot          *string
	LineTotalSnapshot          *string
	Currency                   *string
}

type TransactionReviewRecord struct {
	ID                string
	BusinessID        string
	TransactionID     string
	Required          bool
	Status            string
	ReasonCodes       []byte
	ReviewerReference *string
	DecisionReason    *string
	DecidedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type TransactionConfirmationRecord struct {
	ID                string
	BusinessID        string
	TransactionID     string
	Status            string
	ConfirmedBy       *string
	ConfirmedAt       *time.Time
	EvidenceReference *string
	PolicyVersion     *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type TransactionDraft struct {
	ID                  string
	BusinessID          string
	CustomerID          string
	LeadID              *string
	TransactionType     string
	Currency            *string
	SchemaVersion       int
	RequiresHumanReview bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Lines               []TransactionLineDraft
}

type TransactionLineDraft struct {
	ID                 string
	CatalogItemID      string
	OfferID            *string
	VariantID          *string
	Quantity           string
	SelectedAttributes []byte
}

type TransactionPatch struct {
	ID              string
	BusinessID      string
	Currency        *string
	ExpectedVersion int64
	UpdatedAt       time.Time
	Lines           []TransactionLineDraft
}

type TransactionConfirmPatch struct {
	ID                string
	BusinessID        string
	EvidenceReference string
	PolicyVersion     string
	ExpectedVersion   int64
	UpdatedAt         time.Time
}

type TransactionCancelPatch struct {
	ID              string
	BusinessID      string
	Reason          string
	ExpectedVersion int64
	UpdatedAt       time.Time
}

type TransactionReviewPatch struct {
	ID              string
	BusinessID      string
	ReasonCodes     []byte
	ExpectedVersion int64
	UpdatedAt       time.Time
}

type TransactionReviewDecisionPatch struct {
	ID                string
	BusinessID        string
	ReviewerReference string
	Reason            string
	Approved          bool
	ExpectedVersion   int64
	UpdatedAt         time.Time
}

type TransactionRepository interface {
	List(ctx context.Context, businessID, state, transactionType, customerID string, limit int, cursor string) (TransactionPage, error)
	Get(ctx context.Context, businessID, transactionID string) (TransactionRecord, error)
	GetReview(ctx context.Context, businessID, transactionID string) (TransactionReviewRecord, error)
	CreateDraft(ctx context.Context, draft TransactionDraft) (TransactionRecord, error)
	UpdateDraft(ctx context.Context, patch TransactionPatch) (TransactionRecord, error)
	Confirm(ctx context.Context, patch TransactionConfirmPatch) (TransactionRecord, error)
	Cancel(ctx context.Context, patch TransactionCancelPatch) (TransactionRecord, error)
	SubmitReview(ctx context.Context, patch TransactionReviewPatch) (TransactionReviewRecord, error)
	DecideReview(ctx context.Context, patch TransactionReviewDecisionPatch) (TransactionReviewRecord, error)
}
