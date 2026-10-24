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

func TestSalesWorkflowRejectsExpiredAndForeignCustomerButtonsAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	r, err := f.apply(t, "expiry")
	if err != nil {
		t.Fatal(err)
	}
	foreign := newSalesWorkflowFixture(t)
	if _, err := foreign.button(t, r, "accept"); err == nil {
		t.Fatal("foreign customer accepted another tenant's request")
	}
	if _, err := f.a.Pool().Exec(context.Background(), `UPDATE sales_confirmation_requests SET expires_at=$2 WHERE business_id=$1::uuid`, f.business, f.now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.button(t, r, "accept"); err == nil {
		t.Fatal("expired request was accepted")
	}
	tr, err := NewTransactionRepository(f.a).Get(context.Background(), f.business, *r.Workflow.TransactionID)
	if err != nil || tr.State != "awaiting_confirmation" {
		t.Fatalf("invalid action changed agreement: %+v %v", tr, err)
	}
}

func TestSalesWorkflowIncompleteAgreementsHaveNoButtonsAgainstPostgres(t *testing.T) {
	for _, kind := range []string{"unknown_price", "booking_schema", "booking_fields"} {
		t.Run(kind, func(t *testing.T) {
			f := newSalesWorkflowFixture(t)
			proposal := ports.SalesWorkflowProposal{TransactionType: "order", EvidenceMessageIDs: []string{f.message}, Lines: []ports.SalesWorkflowLine{{ItemID: f.item, OfferID: &f.offer, Quantity: "1"}}}
			switch kind {
			case "unknown_price":
				if _, err := f.a.Pool().Exec(context.Background(), `UPDATE offers SET price_verification_status='unverified' WHERE id=$1::uuid`, f.offer); err != nil {
					t.Fatal(err)
				}
			case "booking_schema":
				proposal.TransactionType = "booking"
			case "booking_fields":
				proposal.TransactionType = "booking"
				err := NewSalesWorkflowRepository(f.a).PutRequirement(context.Background(), f.business, ports.SalesRequirement{TransactionType: "booking", SchemaVersion: 1, Definitions: []ports.SalesRequirementDefinition{{Key: "date", Label: "موعد الحجز", DataType: "date", Required: true}}})
				if err != nil {
					t.Fatal(err)
				}
			}
			var result ports.SalesWorkflowResult
			err := f.a.Within(context.Background(), func(tx context.Context) error {
				var err error
				result, err = NewSalesWorkflowRepository(f.a).Apply(tx, ports.SalesWorkflowInput{BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation, SourceMessageID: f.message, IdempotencyKey: kind, Capability: ports.CustomerSalesCapabilityCreateDraft, Proposal: proposal, Now: f.now})
				return err
			})
			if err != nil || result.Workflow.State != "draft" || result.ConfirmationID != "" {
				t.Fatalf("incomplete agreement requested a vote: %+v %v", result, err)
			}
		})
	}
}

func TestSalesWorkflowDeliveryRequiresReviewAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	if _, err := f.a.Pool().Exec(context.Background(), `UPDATE offers SET fulfillment_mode='delivery' WHERE id=$1::uuid`, f.offer); err != nil {
		t.Fatal(err)
	}
	r, err := f.apply(t, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.button(t, r, "accept")
	if err != nil || accepted.Workflow.State != "awaiting_review" {
		t.Fatalf("delivery skipped human review: %+v %v", accepted, err)
	}
}

func TestSalesWorkflowConcurrentIdempotencyAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	type outcome struct {
		result ports.SalesWorkflowResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	for i := 0; i < 2; i++ {
		go func() { r, err := f.apply(t, "concurrent"); outcomes <- outcome{r, err} }()
	}
	one, two := <-outcomes, <-outcomes
	if one.err != nil || two.err != nil || one.result.Workflow.ID != two.result.Workflow.ID || one.result.Duplicate == two.result.Duplicate {
		t.Fatalf("concurrent requests were not serialized: %+v %+v", one, two)
	}
	var count int
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT COUNT(*) FROM commercial_transactions WHERE business_id=$1::uuid`, f.business).Scan(&count); err != nil || count != 1 {
		t.Fatalf("created %d agreements: %v", count, err)
	}
}

func TestSalesWorkflowDraftUpdateSupersedesPreviousButtonsAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	previous, err := f.apply(t, "before-update")
	if err != nil {
		t.Fatal(err)
	}
	proposal := previous.Workflow.Proposal
	proposal.WorkflowID = previous.Workflow.ID
	proposal.ExpectedVersion = previous.Workflow.ResourceVersion
	proposal.Lines[0].Quantity = "2"
	var updated ports.SalesWorkflowResult
	err = f.a.Within(context.Background(), func(tx context.Context) error {
		var err error
		updated, err = NewSalesWorkflowRepository(f.a).Apply(tx, ports.SalesWorkflowInput{BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation, SourceMessageID: f.message, IdempotencyKey: "after-update", Capability: ports.CustomerSalesCapabilityUpdateDraft, Proposal: proposal, Now: f.now})
		return err
	})
	if err != nil || updated.ConfirmationID == previous.ConfirmationID || updated.TotalAmount == nil || *updated.TotalAmount != "10000.0000" {
		t.Fatalf("invalid revised agreement: %+v %v", updated, err)
	}
	if _, err := f.button(t, previous, "accept"); err == nil {
		t.Fatal("old agreement button accepted updated draft")
	}
	if result, err := f.button(t, updated, "accept"); err != nil || result.Workflow.State != "confirmed" {
		t.Fatalf("current agreement could not confirm: %+v %v", result, err)
	}
}

func TestSalesWorkflowConversationOverrideRequiresApprovalAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	r, err := f.apply(t, "mode-change")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.Pool().Exec(context.Background(), `UPDATE conversations SET ai_mode_override='draft_only' WHERE id=$1::uuid`, f.conversation); err != nil {
		t.Fatal(err)
	}
	if _, err := f.apply(t, "new-execution"); err == nil {
		t.Fatal("draft-only override executed automatically")
	}
	accepted, err := f.button(t, r, "accept")
	if err != nil || accepted.Workflow.State != "awaiting_review" {
		t.Fatalf("conversation override skipped review: %+v %v", accepted, err)
	}
}

func TestSalesWorkflowRequirementsRemainPinnedAndGuardHumanConfirmationAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	repo := NewSalesWorkflowRepository(f.a)
	req := ports.SalesRequirement{TransactionType: "booking", SchemaVersion: 1, Definitions: []ports.SalesRequirementDefinition{{Key: "date", Label: "موعد الحجز", DataType: "date", Required: true}}}
	if err := repo.PutRequirement(context.Background(), f.business, req); err != nil {
		t.Fatal(err)
	}
	if err := repo.PutRequirement(context.Background(), f.business, req); err == nil {
		t.Fatal("replaced immutable requirement version")
	}
	var r ports.SalesWorkflowResult
	err := f.a.Within(context.Background(), func(tx context.Context) error {
		var err error
		r, err = repo.Apply(tx, ports.SalesWorkflowInput{BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation, SourceMessageID: f.message, IdempotencyKey: "booking", Capability: ports.CustomerSalesCapabilityCreateDraft, Proposal: ports.SalesWorkflowProposal{TransactionType: "booking", TransactionData: map[string]any{"date": "2026-11-01"}, EvidenceMessageIDs: []string{f.message}, Lines: []ports.SalesWorkflowLine{{ItemID: f.item, OfferID: &f.offer, Quantity: "1"}}}, Now: f.now})
		return err
	})
	if err != nil || r.ConfirmationID == "" {
		t.Fatalf("complete booking not ready: %+v %v", r, err)
	}
	accepted, err := f.button(t, r, "accept")
	if err != nil || accepted.Workflow.State != "awaiting_review" {
		t.Fatalf("booking bypassed review: %+v %v", accepted, err)
	}
	req.SchemaVersion = 2
	req.Definitions = append(req.Definitions, ports.SalesRequirementDefinition{Key: "phone", Label: "هاتف", DataType: "text", Required: true})
	if err := repo.PutRequirement(context.Background(), f.business, req); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.Pool().Exec(context.Background(), `UPDATE commercial_transactions SET transaction_data='{}' WHERE id=$1::uuid`, *r.Workflow.TransactionID); err != nil {
		t.Fatal(err)
	}
	review := ports.SalesWorkflowReview{BusinessID: f.business, WorkflowID: r.Workflow.ID, ActorReference: uuid.NewString(), ExpectedVersion: accepted.Workflow.ResourceVersion, Approved: true, Now: f.now.Add(2 * time.Minute)}
	err = f.a.Within(context.Background(), func(tx context.Context) error { _, err := repo.ReviewWorkflow(tx, review); return err })
	if err == nil {
		t.Fatal("employee confirmation skipped missing required fields")
	}
	if _, err := f.a.Pool().Exec(context.Background(), `UPDATE commercial_transactions SET transaction_data='{"date":"2026-11-01"}' WHERE id=$1::uuid`, *r.Workflow.TransactionID); err != nil {
		t.Fatal(err)
	}
	err = f.a.Within(context.Background(), func(tx context.Context) error { _, err := repo.ReviewWorkflow(tx, review); return err })
	if err != nil {
		t.Fatalf("new requirement version changed a pinned agreement: %v", err)
	}
}

func TestSalesWorkflowManualDraftUpdateSynchronizesWorkflowAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	r, err := f.apply(t, "manual-revision")
	if err != nil {
		t.Fatal(err)
	}
	repository := NewTransactionRepository(f.a)
	tr, err := repository.Get(context.Background(), f.business, *r.Workflow.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	err = f.a.Within(context.Background(), func(tx context.Context) error {
		_, err := repository.UpdateDraft(tx, ports.TransactionPatch{BusinessID: f.business, ID: tr.ID, ExpectedVersion: tr.ResourceVersion, Lines: []ports.TransactionLineDraft{{ID: uuid.NewString(), CatalogItemID: f.item, OfferID: &f.offer, Quantity: "3", SelectedAttributes: []byte(`{}`)}}, UpdatedAt: f.now.Add(time.Minute)})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := NewSalesWorkflowRepository(f.a).GetWorkflow(context.Background(), f.business, r.Workflow.ID)
	if err != nil || updated.State != "draft" || updated.ResourceVersion <= r.Workflow.ResourceVersion || len(updated.Proposal.Lines) != 1 || updated.Proposal.Lines[0].Quantity != "3" {
		t.Fatalf("manual revision left old workflow context: %+v %v", updated, err)
	}
	if _, err := f.button(t, r, "accept"); err == nil {
		t.Fatal("manual revision left old button usable")
	}
	tr, err = repository.Get(context.Background(), f.business, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = f.a.Within(context.Background(), func(tx context.Context) error {
		_, err := repository.Cancel(tx, ports.TransactionCancelPatch{BusinessID: f.business, ID: tr.ID, ExpectedVersion: tr.ResourceVersion, Reason: "cancelled by employee", UpdatedAt: f.now.Add(2 * time.Minute)})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err = NewSalesWorkflowRepository(f.a).GetWorkflow(context.Background(), f.business, r.Workflow.ID)
	if err != nil || updated.State != "cancelled" {
		t.Fatalf("cancelled agreement remained active: %+v %v", updated, err)
	}
}

func TestSalesWorkflowReadPreservesOriginalAgreementSnapshotAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	r, err := f.apply(t, "snapshot-read")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.Pool().Exec(context.Background(), `UPDATE catalog_items SET name='اسم جديد' WHERE id=$1::uuid`, f.item); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.Pool().Exec(context.Background(), `UPDATE offers SET amount=6000,resource_version=resource_version+1 WHERE id=$1::uuid`, f.offer); err != nil {
		t.Fatal(err)
	}
	repo := NewTransactionRepository(f.a)
	tr, err := repo.Get(context.Background(), f.business, *r.Workflow.TransactionID)
	if err != nil || len(tr.Lines) != 1 || tr.Lines[0].ItemNameSnapshot != "سماعة" || tr.Lines[0].UnitPriceSnapshot == nil || *tr.Lines[0].UnitPriceSnapshot != "5000.0000" || tr.TotalAmount == nil || *tr.TotalAmount != "5000.0000" {
		t.Fatalf("read used current catalog instead of agreement: %+v %v", tr, err)
	}
	page, err := repo.List(context.Background(), f.business, "", "", "", 20, "")
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Lines) != 1 || page.Items[0].Lines[0].ItemNameSnapshot != "سماعة" {
		t.Fatalf("list lost saved agreement: %+v %v", page, err)
	}
	foreign := newSalesWorkflowFixture(t)
	if _, err := repo.Get(context.Background(), foreign.business, tr.ID); err == nil {
		t.Fatal("another tenant read the agreement")
	}
}

func TestSalesWorkflowLeadToPartialSubscriptionToConfirmedReadyAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	repo := NewSalesWorkflowRepository(f.a)
	if err := repo.PutRequirement(context.Background(), f.business, ports.SalesRequirement{
		TransactionType: "subscription", SchemaVersion: 1,
		Definitions: []ports.SalesRequirementDefinition{
			{Key: "field_business", Label: "اسم النشاط", DataType: "text", Required: true},
			{Key: "field_phone", Label: "رقم التلفون", DataType: "text", Required: true},
		},
	}); err != nil {
		t.Fatal(err)
	}
	apply := func(key string, capability ports.CustomerSalesCapability, proposal ports.SalesWorkflowProposal) (ports.SalesWorkflowResult, error) {
		t.Helper()
		var result ports.SalesWorkflowResult
		err := f.a.Within(context.Background(), func(tx context.Context) error {
			var applyErr error
			result, applyErr = repo.Apply(tx, ports.SalesWorkflowInput{
				BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation,
				SourceMessageID: f.message, IdempotencyKey: key, Capability: capability, Proposal: proposal, Now: f.now,
			})
			return applyErr
		})
		return result, err
	}
	lead, err := apply("record-lead", ports.CustomerSalesCapabilityRecordLead, ports.SalesWorkflowProposal{EvidenceMessageIDs: []string{f.message}})
	if err != nil || lead.Workflow.TransactionID != nil || lead.Workflow.ResourceVersion != 1 {
		t.Fatalf("lead did not create a standalone opportunity: %+v %v", lead, err)
	}
	create := ports.SalesWorkflowProposal{
		WorkflowID: lead.Workflow.ID, ExpectedVersion: lead.Workflow.ResourceVersion,
		TransactionType: "subscription", EvidenceMessageIDs: []string{f.message},
		Lines: []ports.SalesWorkflowLine{{ItemID: f.item, OfferID: &f.offer, Quantity: "1"}},
		TransactionData: map[string]any{"field_business": "متجر"},
	}
	draft, err := apply("create-draft", ports.CustomerSalesCapabilityCreateDraft, create)
	if err != nil || draft.Workflow.ID != lead.Workflow.ID || draft.Workflow.State != "draft" ||
		draft.Workflow.ResourceVersion != 2 || draft.ConfirmationID != "" {
		t.Fatalf("lead was not promoted to incomplete draft: %+v %v", draft, err)
	}
	update := ports.SalesWorkflowProposal{
		WorkflowID: draft.Workflow.ID, ExpectedVersion: draft.Workflow.ResourceVersion,
		TransactionType: "subscription", EvidenceMessageIDs: []string{f.message},
		Lines: []ports.SalesWorkflowLine{{ItemID: f.item, OfferID: &f.offer, Quantity: "1"}},
		TransactionData: map[string]any{"field_phone": "776900067"},
	}
	ready, err := apply("add-phone", ports.CustomerSalesCapabilityUpdateDraft, update)
	if err != nil || ready.Workflow.State != "awaiting_confirmation" || ready.ConfirmationID == "" ||
		ready.Workflow.ResourceVersion != 3 || ready.Workflow.Proposal.TransactionData["field_business"] != "متجر" ||
		ready.Workflow.Proposal.TransactionData["field_phone"] != "776900067" {
		t.Fatalf("partial answer was not merged into confirmable subscription: %+v %v", ready, err)
	}
	if ready.Workflow.TransactionID == nil {
		t.Fatal("confirmed-ready subscription missing transaction")
	}
	var data map[string]any
	if err := f.a.Pool().QueryRow(context.Background(),
		`SELECT transaction_data FROM commercial_transactions WHERE business_id=$1::uuid AND id=$2::uuid`,
		f.business, *ready.Workflow.TransactionID).Scan(&data); err != nil ||
		data["field_business"] != "متجر" || data["field_phone"] != "776900067" {
		t.Fatalf("persisted commercial transaction lost customer data: %v %v", data, err)
	}
	if _, err := apply("stale-update", ports.CustomerSalesCapabilityUpdateDraft, update); !IsRepositoryKind(err, RepositoryStale) {
		t.Fatalf("stale workflow version was accepted: %v", err)
	}
	create.ExpectedVersion = ready.Workflow.ResourceVersion
	if _, err := apply("duplicate-create", ports.CustomerSalesCapabilityCreateDraft, create); err == nil {
		t.Fatal("create_transaction_draft overwrote an existing commercial draft")
	}
}
