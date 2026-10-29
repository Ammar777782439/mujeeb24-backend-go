package merchantassist

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

// RecordKind is a closed set of single-record, read-only dashboard requests.
// Customer messages and raw JSONB fields are deliberately not supported.
type RecordKind string

const (
	RecordCustomer     RecordKind = "customer"
	RecordConversation RecordKind = "conversation"
	RecordLead         RecordKind = "lead"
	RecordTransaction  RecordKind = "transaction"
)

type RecordReadRequest struct {
	Actor    commands.ActorContext
	Business string
	Kind     RecordKind
	RecordID string
	Task     contracts.AssistTask
}

// Narrow interfaces allow reuse of existing business-scoped repository reads
// without accidentally exposing create/update/approval operations.
type CustomerGetter interface {
	GetByID(context.Context, string, string) (ports.CustomerRecord, error)
}
type ConversationGetter interface {
	GetByID(context.Context, string, string) (ports.ConversationRecord, error)
}
type LeadGetter interface {
	Get(context.Context, string, string) (ports.LeadRecord, error)
}
type TransactionGetter interface {
	Get(context.Context, string, string) (ports.TransactionRecord, error)
}

type RecordReadResult struct {
	Evidence     contracts.EvidenceEnvelopeV1    `json:"evidence"`
	Customer     *contracts.CustomerAssistV1     `json:"customer,omitempty"`
	Conversation *contracts.ConversationAssistV1 `json:"conversation,omitempty"`
	Lead         *contracts.LeadAssistV1         `json:"lead,omitempty"`
	Transaction  *contracts.TransactionAssistV1  `json:"transaction,omitempty"`
}

// RecordReader is an application facade only. It does not assemble a model
// prompt, authorize writes, load conversation messages or call a provider.
type RecordReader struct {
	Customers     CustomerGetter
	Conversations ConversationGetter
	Leads         LeadGetter
	Transactions  TransactionGetter
	AIGuard       ports.BusinessAIExecutionGuard
	Now           func() time.Time
}

func (r *RecordReader) Read(ctx context.Context, req RecordReadRequest) (RecordReadResult, error) {
	var empty RecordReadResult
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if r == nil || r.AIGuard == nil || r.Now == nil {
		return empty, ErrReadUnavailable
	}
	if req.Business == "" || string(req.Actor.BusinessID) != req.Business ||
		strings.TrimSpace(string(req.Actor.PrincipalID)) == "" ||
		!authorization.Has(req.Actor, authorization.PermissionMerchantAIUse) {
		return empty, ErrReadDenied
	}
	var permission string
	switch req.Kind {
	case RecordCustomer:
		permission = authorization.PermissionCustomersRead
		if r.Customers == nil {
			return empty, ErrReadUnavailable
		}
	case RecordConversation:
		permission = authorization.PermissionConversationsRead
		if r.Conversations == nil {
			return empty, ErrReadUnavailable
		}
	case RecordLead, RecordTransaction:
		permission = authorization.PermissionSalesRead
		if (req.Kind == RecordLead && r.Leads == nil) ||
			(req.Kind == RecordTransaction && r.Transactions == nil) {
			return empty, ErrReadUnavailable
		}
	default:
		return empty, ErrInvalidReadRequest
	}
	if !authorization.Has(req.Actor, permission) {
		return empty, ErrReadDenied
	}
	// There is no general search, pagination or guessable cross-tenant listing.
	if req.Task != contracts.TaskExplain && req.Task != contracts.TaskSummarize ||
		len(req.RecordID) < 1 || len(req.RecordID) > 128 ||
		strings.TrimSpace(req.RecordID) != req.RecordID ||
		strings.ContainsAny(req.RecordID, " \n\r\t") {
		return empty, ErrInvalidReadRequest
	}
	if err := r.AIGuard.Check(ctx, req.Business); err != nil {
		return empty, fmt.Errorf("%w: %w", ErrReadDenied, err)
	}
	now := r.Now().UTC()
	if now.IsZero() {
		return empty, ErrReadUnavailable
	}
	// The evidence records only the exact bounded projection and carries a
	// short expiration. It does not claim the *entire* underlying record is
	// complete, nor that a historical total is a current catalog price.
	evidence := contracts.EvidenceEnvelopeV1{
		SchemaVersion: contracts.EvidenceSchemaVersion,
		RetrievedAt:   now,
		Completeness:  contracts.EvidencePartial,
		Authority:     contracts.AuthoritySystemRecord,
	}
	expires := now.Add(2 * time.Minute)
	evidence.ValidUntil = &expires
	var out RecordReadResult
	switch req.Kind {
	case RecordCustomer:
		record, err := r.Customers.GetByID(ctx, req.Business, req.RecordID)
		if err != nil {
			return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
		}
		if err := validateScopedRecord(req, record.ID, record.BusinessID, record.ResourceVersion); err != nil {
			return empty, err
		}
		value := contracts.CustomerAssistV1{
			CustomerRef: record.ID, LocalePreference: record.LocalePreference,
			Status: record.Status,
		}
		if err := value.Validate(); err != nil {
			return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
		}
		evidence.SourceType = contracts.SourceCustomer
		evidence.ResourceRef = "customer:" + record.ID
		evidence.ResourceVersion = versionPointer(record.ResourceVersion)
		out.Customer = &value
	case RecordConversation:
		record, err := r.Conversations.GetByID(ctx, req.Business, req.RecordID)
		if err != nil {
			return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
		}
		if err := validateScopedRecord(req, record.ID, record.BusinessID, record.ResourceVersion); err != nil {
			return empty, err
		}
		value := contracts.ConversationAssistV1{
			ConversationRef: record.ID, CustomerRef: record.CustomerID,
			State: record.State, Ownership: record.Ownership,
			Priority: record.Priority, AIModeOverride: record.AIModeOverride,
			LastActivityAt: record.LastActivityAt,
		}
		if err := value.Validate(); err != nil {
			return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
		}
		evidence.SourceType = contracts.SourceConversation
		evidence.ResourceRef = "conversation:" + record.ID
		evidence.ResourceVersion = versionPointer(record.ResourceVersion)
		out.Conversation = &value
	case RecordLead:
		record, err := r.Leads.Get(ctx, req.Business, req.RecordID)
		if err != nil {
			return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
		}
		if err := validateScopedRecord(req, record.ID, record.BusinessID, record.ResourceVersion); err != nil {
			return empty, err
		}
		value := contracts.LeadAssistV1{
			LeadRef: record.ID, CustomerRef: record.CustomerID,
			Status: record.Status, QualificationState: record.QualificationState,
			CurrentScoreBand:  record.CurrentScoreBand,
			CurrentScoreValue: record.CurrentScoreValue, NextActionAt: record.NextActionAt,
		}
		if err := value.Validate(); err != nil {
			return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
		}
		evidence.SourceType = contracts.SourceLead
		evidence.ResourceRef = "lead:" + record.ID
		evidence.ResourceVersion = versionPointer(record.ResourceVersion)
		out.Lead = &value
	case RecordTransaction:
		record, err := r.Transactions.Get(ctx, req.Business, req.RecordID)
		if err != nil {
			return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
		}
		if err := validateScopedRecord(req, record.ID, record.BusinessID, record.ResourceVersion); err != nil {
			return empty, err
		}
		value := contracts.TransactionAssistV1{
			TransactionRef: record.ID, CustomerRef: record.CustomerID,
			TransactionType: record.TransactionType, State: record.State,
			RequiresHumanReview: record.RequiresHumanReview,
			Currency:            record.Currency, TotalAmount: record.TotalAmount,
		}
		if err := value.Validate(); err != nil {
			return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
		}
		evidence.SourceType = contracts.SourceTransaction
		evidence.ResourceRef = "transaction:" + record.ID
		evidence.ResourceVersion = versionPointer(record.ResourceVersion)
		out.Transaction = &value
	}
	if err := evidence.Validate(now); err != nil {
		return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
	}
	out.Evidence = evidence
	if err := accountSize(out, new(int)); err != nil {
		return empty, err
	}
	return out, nil
}

func versionPointer(v int64) *string {
	version := strconv.FormatInt(v, 10)
	return &version
}

func validateScopedRecord(req RecordReadRequest, recordID, businessID string, version int64) error {
	if recordID != req.RecordID || businessID != req.Business || version <= 0 {
		return ErrInvalidSourceEvidence
	}
	return nil
}
