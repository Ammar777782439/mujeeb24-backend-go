package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// SalesStartRepository implements the consent boundary on the existing transaction executor.
type SalesStartRepository struct{ adapter *Adapter }

func NewSalesStartRepository(a *Adapter) *SalesStartRepository {
	return &SalesStartRepository{adapter: a}
}

func startTokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// Create must be called in the same transaction that persists the original interactive outbound.
func (r *SalesStartRepository) Create(ctx context.Context, in ports.SalesStartRequest) error {
	if _, ok := transactionFromContext(ctx); !ok {
		return errors.New("sales start creation requires an application transaction")
	}
	if in.ID == "" || in.BusinessID == "" || in.CustomerID == "" || in.ConversationID == "" || in.OutboundMessageID == "" || in.SourceMessageID == "" || !in.ExpiresAt.After(in.CreatedAt) {
		return invalidRepositoryInput("sales.start.create", "complete identity, source, outbound and expiry are required")
	}
	e, err := r.adapter.Executor(ctx)
	if err != nil { return err }
	var conversation string
	if err := e.QueryRow(ctx, `SELECT id::text FROM conversations
		WHERE business_id=$1::uuid AND customer_id=$2::uuid AND id=$3::uuid FOR UPDATE`,
		in.BusinessID, in.CustomerID, in.ConversationID).Scan(&conversation); err != nil { return err }
	policy, err := NewBusinessRepository(r.adapter).GetRuntimePolicy(ctx, in.BusinessID)
	if err != nil { return err }
	if !policy.SalesWorkflowEnabled || !policy.AllowSalesInteractions || !policy.AllowAutoReply || policy.AIMode != "restricted_auto" {
		return invalidRepositoryInput("sales.start.policy", "sales start offer is not authorized")
	}
	source, err := salesSourceMessage(ctx,e,in.BusinessID,in.ConversationID,in.CustomerID,in.SourceMessageID)
	if err != nil { return err }
	var outbound string
	if err := e.QueryRow(ctx, `SELECT id::text FROM outbound_messages
		WHERE business_id=$1::uuid AND conversation_id=$2::uuid AND id=$3::uuid
		AND transport='provider' AND origin='ai'`,
		in.BusinessID,in.ConversationID,in.OutboundMessageID).Scan(&outbound); err != nil { return err }
	if _, err := e.Exec(ctx, `UPDATE sales_start_requests SET status='superseded',
		resolved_at=$3,updated_at=$3 WHERE business_id=$1::uuid AND conversation_id=$2::uuid AND status='pending'`,
		in.BusinessID,in.ConversationID,in.CreatedAt); err != nil { return err }
	_, err = e.Exec(ctx, `INSERT INTO sales_start_requests
		(id,business_id,customer_id,conversation_id,outbound_message_id,source_message_id,status,
		 accept_token_hash,decline_token_hash,expires_at,created_at,updated_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6::uuid,'pending',$7,$8,$9,$10,$10)`,
		in.ID,in.BusinessID,in.CustomerID,in.ConversationID,outbound,source,
		startTokenHash("m24.sales.start:"+in.ID+":accept"),
		startTokenHash("m24.sales.start:"+in.ID+":decline"),in.ExpiresAt,in.CreatedAt)
	return err
}

// Decide accepts only a signed SocialAPI postback attached to this actual customer/conversation.
// SocialAPI's postback contract has a payload, but does not expose the original message ID;
// the persisted outbound and opaque request token provide the origin binding instead.
func (r *SalesStartRepository) Decide(ctx context.Context, in ports.SalesButtonAction) (ports.SalesStartDecision,error) {
	var out ports.SalesStartDecision
	if _, ok := transactionFromContext(ctx); !ok {
		return out,errors.New("sales start decision requires an application transaction")
	}
	parts := strings.Split(in.Payload,":")
	if len(parts)!=3 || parts[0]!="m24.sales.start" || (parts[2]!="accept" && parts[2]!="decline") {
		return out,invalidRepositoryInput("sales.start.action","invalid start consent payload")
	}
	if _,err:=uuid.Parse(parts[1]);err!=nil{return out,invalidRepositoryInput("sales.start.action","invalid start request id")}
	if in.BusinessID=="" || in.CustomerID=="" || in.ConversationID=="" || in.SourceMessageID=="" || in.Now.IsZero(){
		return out,invalidRepositoryInput("sales.start.action","trusted scope, source and time required")
	}
	e,err:=r.adapter.Executor(ctx);if err!=nil{return out,err}
	var conv string
	if err:=e.QueryRow(ctx,`SELECT id::text FROM conversations
		WHERE business_id=$1::uuid AND customer_id=$2::uuid AND id=$3::uuid FOR UPDATE`,
		in.BusinessID,in.CustomerID,in.ConversationID).Scan(&conv);err!=nil{return out,err}
	var status,sourceID,acceptHash,declineHash string
	var expires time.Time
	var originalOutbound string
	err=e.QueryRow(ctx,`SELECT status,source_message_id::text,accept_token_hash,decline_token_hash,expires_at,outbound_message_id::text
		FROM sales_start_requests WHERE business_id=$1::uuid AND id=$2::uuid
		AND customer_id=$3::uuid AND conversation_id=$4::uuid FOR UPDATE`,
		in.BusinessID,parts[1],in.CustomerID,in.ConversationID).
		Scan(&status,&sourceID,&acceptHash,&declineHash,&expires,&originalOutbound)
	if err!=nil{return out,invalidRepositoryInput("sales.start.action","no matching start request")}
	if (parts[2]=="accept" && startTokenHash(in.Payload)!=acceptHash) ||
	   (parts[2]=="decline" && startTokenHash(in.Payload)!=declineHash){
		return out,invalidRepositoryInput("sales.start.action","invalid start token")
	}
	out=ports.SalesStartDecision{ID:parts[1],BusinessID:in.BusinessID,CustomerID:in.CustomerID,ConversationID:in.ConversationID,SourceMessageID:sourceID}
	if status=="accepted" || status=="declined" {
		out.Duplicate=true
		return out,nil
	}
	if status!="pending" || !in.Now.Before(expires) {
		return out,invalidRepositoryInput("sales.start.action","start request expired, cancelled or superseded")
	}
	source,err:=salesSourceMessage(ctx,e,in.BusinessID,in.ConversationID,in.CustomerID,in.SourceMessageID)
	if err!=nil{return out,err}
	// Inbound ledger signature, postback kind, channel connection and original outbound must agree.
	var verified string
	err=e.QueryRow(ctx,`SELECT m.id::text FROM communication_messages m
		JOIN inbound_event_ledger l ON l.id=m.inbound_event_id AND l.business_id=m.business_id
		JOIN outbound_messages o ON o.business_id=m.business_id AND o.id=$5::uuid
		JOIN sales_action_jobs j ON j.business_id=m.business_id AND j.message_id=m.id AND j.payload=$4
		WHERE m.business_id=$1::uuid AND m.id=$2::uuid AND o.conversation_id=$3::uuid
		AND l.connection_id=o.connection_id AND l.provider_ref=o.provider_ref
		AND l.event_type='interaction_received' AND l.interaction_kind='dm'
		AND l.signature_verified=true AND m.origin='customer' AND m.direction='inbound'`,
		in.BusinessID,source,in.ConversationID,in.Payload,originalOutbound).Scan(&verified)
	if err!=nil{return out,invalidRepositoryInput("sales.start.action","untrusted or unbound postback")}
	policy,err:=NewBusinessRepository(r.adapter).GetRuntimePolicy(ctx,in.BusinessID)
	if err!=nil{return out,err}
	if !policy.SalesWorkflowEnabled || !policy.AllowSalesInteractions || policy.AIMode=="disabled" {
		return out,invalidRepositoryInput("sales.start.policy","sales workflow is disabled")
	}
	out.RequiresApproval=policy.AIMode!="restricted_auto" || policy.DefaultHumanReview || !policy.AllowAutoLeadCreation
	out.Accepted=parts[2]=="accept"
	next:="declined"
	continuation:="not_required"
	if out.Accepted {next="accepted";continuation="pending"}
	_,err=e.Exec(ctx,`UPDATE sales_start_requests SET status=$3,resolved_at=$4,
		decision_message_id=$5::uuid,continuation_state=$6,updated_at=$4
		WHERE business_id=$1::uuid AND id=$2::uuid AND status='pending'`,
		in.BusinessID,out.ID,next,in.Now,verified,continuation)
	return out,err
}

var _ ports.SalesStartRepository = (*SalesStartRepository)(nil)
