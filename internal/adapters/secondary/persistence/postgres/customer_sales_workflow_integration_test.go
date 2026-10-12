//go:build integration

package postgres

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
	"os"
	"testing"
	"time"
)

type salesWorkflowFixture struct {
	a                                                                             *Adapter
	business, customer, conversation, reference, connection, item, offer, message string
	now                                                                           time.Time
}

func newSalesWorkflowFixture(t *testing.T) salesWorkflowFixture {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx := context.Background()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	a, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	f := salesWorkflowFixture{a: a, business: uuid.NewString(), customer: uuid.NewString(), conversation: uuid.NewString(), reference: uuid.NewString(), connection: uuid.NewString(), item: uuid.NewString(), offer: uuid.NewString(), message: uuid.NewString(), now: time.Now().UTC()}
	catalog := uuid.NewString()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := a.Pool().Exec(ctx, q, args...); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	exec(`INSERT INTO businesses(id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at) VALUES($1::uuid,'Sales','sales-'||$1::text,'active','retail','Asia/Aden','YER','ar-YE',now(),now())`, f.business)
	exec(`INSERT INTO business_policies(business_id,ai_mode,default_human_review,allow_auto_reply,allow_auto_lead_creation,allow_auto_transaction_draft,allow_auto_confirmation,sales_workflow_enabled,allow_sales_interactions,created_at,updated_at) VALUES($1::uuid,'restricted_auto',false,false,true,true,true,true,true,now(),now())`, f.business)
	exec(`INSERT INTO customers(id,business_id,profile,contact_points,status,created_at,updated_at) VALUES($1::uuid,$2::uuid,'{}','[]','active',now(),now())`, f.customer, f.business)
	exec(`INSERT INTO channel_connections(id,business_id,provider_ref,channel,provider_account_ref,provider_connection_ref,status,secret_reference,created_at,updated_at) VALUES($1::uuid,$2::uuid,'socialapi','instagram',$1::text,$1::text,'active','env://test',now(),now())`, f.connection, f.business)
	exec(`INSERT INTO conversations(id,business_id,customer_id,state,ownership,priority,last_activity_at,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,'open','ai','normal',now(),now(),now())`, f.conversation, f.business, f.customer)
	exec(`INSERT INTO conversation_references(id,business_id,conversation_id,system,provider_ref,resource_type,resource_id,connection_id,is_current,mapping_status,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,'provider','socialapi','conversation',$3::text,$4::uuid,true,'active',now(),now())`, f.reference, f.business, f.conversation, f.connection)
	exec(`INSERT INTO communication_messages(id,business_id,conversation_reference_id,direction,origin,transport,provider_message_id,content_type,text_content,content_reference,occurred_at,created_at) VALUES($1::uuid,$2::uuid,$3::uuid,'inbound','customer','provider',$1::text,'text','أريد واحدة','inline://fixture',now(),now())`, f.message, f.business, f.reference)
	exec(`INSERT INTO catalogs(id,business_id,name,description,status,created_at,updated_at) VALUES($1::uuid,$2::uuid,'Items','','active',now(),now())`, catalog, f.business)
	exec(`INSERT INTO catalog_items(id,business_id,catalog_id,item_type,name,status,pricing_mode,availability_mode,fulfillment_mode,requires_confirmation,attributes,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,'service','سماعة','active','fixed','always_available','pickup',true,'{}',now(),now())`, f.item, f.business, catalog)
	exec(`INSERT INTO offers(id,business_id,catalog_item_id,name,pricing_mode,amount,currency,price_source,price_verification_status,availability_mode,availability_status,availability_source,fulfillment_mode,status,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,'Offer','fixed',5000,'YER','merchant','verified','always_available','available','merchant','pickup','active',now(),now())`, f.offer, f.business, f.item)
	// Append-only audit remains; fixtures use unique tenant ids and are confined to the test database.
	return f
}
func (f salesWorkflowFixture) apply(t *testing.T, key string) (ports.SalesWorkflowResult, error) {
	t.Helper()
	var result ports.SalesWorkflowResult
	repo := NewSalesWorkflowRepository(f.a)
	err := f.a.Within(context.Background(), func(tx context.Context) error {
		var err error
		result, err = repo.Apply(tx, ports.SalesWorkflowInput{BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation, SourceMessageID: f.message, IdempotencyKey: key, Capability: ports.CustomerSalesCapabilityCreateDraft, Proposal: ports.SalesWorkflowProposal{TransactionType: "order", EvidenceMessageIDs: []string{f.message}, Lines: []ports.SalesWorkflowLine{{ItemID: f.item, OfferID: &f.offer, Quantity: "1"}}}, Now: f.now})
		return err
	})
	return result, err
}
func (f salesWorkflowFixture) button(t *testing.T, r ports.SalesWorkflowResult, choice string) (ports.SalesWorkflowResult, error) {
	t.Helper()
	message := uuid.NewString()
	ctx := context.Background()
	if _, err := f.a.Pool().Exec(ctx, `INSERT INTO communication_messages(id,business_id,conversation_reference_id,direction,origin,transport,provider_message_id,content_type,text_content,content_reference,occurred_at,created_at) VALUES($1::uuid,$2::uuid,$3::uuid,'inbound','customer','provider',$1::text,'text','نعم','inline://fixture',now(),now())`, message, f.business, f.reference); err != nil {
		t.Fatal(err)
	}
	var result ports.SalesWorkflowResult
	err := f.a.Within(ctx, func(tx context.Context) error {
		var err error
		result, err = NewSalesWorkflowRepository(f.a).ApplyButton(tx, ports.SalesButtonAction{BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation, SourceMessageID: message, Payload: "m24.sales:" + r.ConfirmationID + ":" + choice, Now: f.now.Add(time.Minute)})
		return err
	})
	return result, err
}
func TestSalesWorkflowConfirmationAndIdempotencyAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	r, err := f.apply(t, "same-request")
	if err != nil {
		t.Fatal(err)
	}
	if r.Workflow.State != "awaiting_confirmation" || r.TotalAmount == nil || *r.TotalAmount != "5000.0000" {
		t.Fatalf("invalid draft: %+v", r)
	}
	again, err := f.apply(t, "same-request")
	if err != nil || !again.Duplicate || again.Workflow.ID != r.Workflow.ID {
		t.Fatalf("duplicate created another workflow: %+v %v", again, err)
	}
	accepted, err := f.button(t, r, "accept")
	if err != nil || accepted.Workflow.State != "confirmed" {
		t.Fatalf("confirm: %+v %v", accepted, err)
	}
	lead, err := NewLeadRepository(f.a).Get(context.Background(), f.business, *r.Workflow.LeadID)
	if err != nil || lead.Status != "won" || lead.QualificationState != "converted" {
		t.Fatalf("lead not converted: %+v %v", lead, err)
	}
	duplicate, err := f.button(t, r, "accept")
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("second button execution was not suppressed: %+v %v", duplicate, err)
	}
}
func TestSalesWorkflowPriceChangeAndReviewGuardsAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	r, err := f.apply(t, "price-request")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.Pool().Exec(context.Background(), `UPDATE offers SET amount=6000,resource_version=resource_version+1 WHERE id=$1::uuid`, f.offer); err != nil {
		t.Fatal(err)
	}
	if _, err := f.button(t, r, "accept"); err == nil {
		t.Fatal("confirmed a changed price")
	}
	var state string
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT state FROM commercial_transactions WHERE id=$1::uuid`, *r.Workflow.TransactionID).Scan(&state); err != nil || state != "awaiting_confirmation" {
		t.Fatalf("failed confirmation mutated transaction: %s %v", state, err)
	}
}
func TestSalesWorkflowHumanReviewKeepsAgreementVersionAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	if _, err := f.a.Pool().Exec(context.Background(), `UPDATE business_policies SET allow_auto_confirmation=false WHERE business_id=$1::uuid`, f.business); err != nil {
		t.Fatal(err)
	}
	r, err := f.apply(t, "review-request")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.button(t, r, "accept")
	if err != nil || accepted.Workflow.State != "awaiting_review" {
		t.Fatalf("review was bypassed: %+v %v", accepted, err)
	}
	repo := NewTransactionRepository(f.a)
	tr, err := repo.Get(context.Background(), f.business, *r.Workflow.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	err = f.a.Within(context.Background(), func(tx context.Context) error {
		_, err := repo.Confirm(tx, ports.TransactionConfirmPatch{BusinessID: f.business, ID: tr.ID, ExpectedVersion: tr.ResourceVersion, EvidenceReference: "manual", UpdatedAt: f.now.Add(time.Minute)})
		return err
	})
	if err == nil {
		t.Fatal("confirmation bypassed pending review")
	}
	var reviewed ports.SalesWorkflowResult
	err = f.a.Within(context.Background(), func(tx context.Context) error {
		var err error
		reviewed, err = NewSalesWorkflowRepository(f.a).ReviewWorkflow(tx, ports.SalesWorkflowReview{BusinessID: f.business, WorkflowID: accepted.Workflow.ID, ActorReference: uuid.NewString(), ExpectedVersion: accepted.Workflow.ResourceVersion, Approved: true, Now: f.now.Add(2 * time.Minute)})
		return err
	})
	if err != nil || reviewed.Workflow.State != "confirmed" {
		t.Fatalf("review invalidated customer agreement: %+v %v", reviewed, err)
	}
}
func TestSalesWorkflowNotificationUsesExistingOutboxAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	s := services.CustomerSalesWorkflowService{Repository: NewSalesWorkflowRepository(f.a), Transactions: f.a, References: NewConversationReferenceRepository(f.a), Connections: NewChannelConnectionRepository(f.a), Outbound: NewOutboundMessageRepository(f.a), Messages: NewMessageRepository(f.a), Outbox: NewPostgresOutboxStore(f.a), Audit: NewAuditEventRepository(f.a)}
	err := f.a.Within(context.Background(), func(tx context.Context) error {
		_, err := s.Execute(tx, ports.SalesWorkflowInput{BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation, SourceMessageID: f.message, IdempotencyKey: "notify", Capability: ports.CustomerSalesCapabilityCreateDraft, Proposal: ports.SalesWorkflowProposal{TransactionType: "order", EvidenceMessageIDs: []string{f.message}, Lines: []ports.SalesWorkflowLine{{ItemID: f.item, OfferID: &f.offer, Quantity: "1"}}}, Now: f.now})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var reference string
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT content_reference FROM outbound_messages WHERE business_id=$1::uuid`, f.business).Scan(&reference); err != nil {
		t.Fatal(err)
	}
	m, err := services.DecodeInteractiveContentReference(reference)
	if err != nil || len(m.Buttons) != 2 {
		t.Fatalf("buttons were lost before transport: %+v %v", m, err)
	}
}
