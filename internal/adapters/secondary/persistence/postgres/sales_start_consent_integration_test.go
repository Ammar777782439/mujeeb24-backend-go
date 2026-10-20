//go:build integration

package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/google/uuid"
)

func createPendingStart(t *testing.T, f salesWorkflowFixture) string {
	t.Helper()
	ctx := context.Background()
	if _, err := f.a.Pool().Exec(ctx, `UPDATE business_policies SET allow_auto_reply=true WHERE business_id=$1::uuid`, f.business); err != nil {
		t.Fatal(err)
	}
	outbound := uuid.NewString()
	_, err := f.a.Pool().Exec(ctx, `INSERT INTO outbound_messages
		(id,business_id,conversation_id,conversation_reference_id,connection_id,provider_ref,channel,origin,direction,transport,content_reference,provider_idempotency_key,status,created_at,updated_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,'socialapi','instagram','ai','outbound','provider','test-content',$6,'pending',now(),now())`,
		outbound, f.business, f.conversation, f.reference, f.connection, "start-req:"+outbound)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	repo := NewSalesStartRepository(f.a)
	err = f.a.Within(ctx, func(tx context.Context) error {
		return repo.Create(tx, ports.SalesStartRequest{
			ID: id, BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation,
			OutboundMessageID: outbound, SourceMessageID: f.message,
			CreatedAt: f.now, ExpiresAt: f.now.Add(ports.SalesStartTTL),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func saveStartPostback(t *testing.T, f salesWorkflowFixture, payload string) string {
	t.Helper()
	ctx := context.Background()
	eventID, messageID := uuid.NewString(), uuid.NewString()
	_, err := f.a.Pool().Exec(ctx, `INSERT INTO inbound_event_ledger
		(id,provider_ref,provider_connection_ref,provider_event_id,dedupe_strategy,
		business_id,connection_id,event_type,interaction_kind,provider_message_id,
		provider_conversation_id,external_user_id,received_at,raw_payload_reference,payload_hash,
		signature_verified,processing_state,processed_at,created_at,updated_at)
		VALUES($1::uuid,'socialapi',$2,$3,'provider_event_id',$4::uuid,$5::uuid,
		'interaction_received','dm',$6,$7,'customer-test',now(),'inline://fixture',$8,
		true,'processed',now(),now(),now())`,
		eventID, f.connection, eventID, f.business, f.connection, messageID, f.conversation, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.a.Pool().Exec(ctx, `INSERT INTO communication_messages
		(id,business_id,conversation_reference_id,inbound_event_id,direction,origin,transport,provider_message_id,
		content_type,text_content,content_reference,occurred_at,created_at)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'inbound','customer','provider',$1::text,'text','نعم','inline://fixture',now(),now())`,
		messageID, f.business, f.reference, eventID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.a.Pool().Exec(ctx, `INSERT INTO sales_action_jobs
		(id,business_id,customer_id,conversation_id,message_id,payload,created_at,updated_at)
		VALUES(gen_random_uuid(),$1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,now(),now())`,
		f.business, f.customer, f.conversation, messageID, payload)
	if err != nil {
		t.Fatal(err)
	}
	return messageID
}

func startFixtureService(f salesWorkflowFixture) services.CustomerSalesWorkflowService {
	return services.CustomerSalesWorkflowService{
		SalesStarts: NewSalesStartRepository(f.a), Repository: NewSalesWorkflowRepository(f.a),
		Transactions: f.a, References: NewConversationReferenceRepository(f.a),
		Connections: NewChannelConnectionRepository(f.a), Outbound: NewOutboundMessageRepository(f.a),
		Messages: NewMessageRepository(f.a), Outbox: NewPostgresOutboxStore(f.a),
		Audit: NewAuditEventRepository(f.a),
	}
}

func TestSalesStartAcceptedOnceEvenAcrossDistinctConcurrentPostbacks(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	id := createPendingStart(t, f)
	messages := []string{saveStartPostback(t, f, "m24.sales.start:"+id+":accept"), saveStartPostback(t, f, "m24.sales.start:"+id+":accept")}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, m := range messages {
		wg.Add(1)
		go func(source string) {
			defer wg.Done()
			_, err := startFixtureService(f).HandleStartButton(context.Background(), ports.SalesButtonAction{
				BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation,
				SourceMessageID: source, Payload: "m24.sales.start:" + id + ":accept",
				Now: f.now.Add(time.Minute),
			})
			errs <- err
		}(m)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var workflows, leads int
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM sales_workflows
		WHERE business_id=$1::uuid`, f.business).Scan(&workflows); err != nil {
		t.Fatal(err)
	}
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM leads
		WHERE business_id=$1::uuid`, f.business).Scan(&leads); err != nil {
		t.Fatal(err)
	}
	if workflows != 1 || leads != 1 {
		t.Fatalf("duplicate business effect: workflows=%d leads=%d", workflows, leads)
	}
	var status, followup string
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT status,continuation_state FROM sales_start_requests WHERE id=$1::uuid`, id).Scan(&status, &followup); err != nil {
		t.Fatal(err)
	}
	if status != "accepted" || followup != "pending" {
		t.Fatalf("bad consent state: %s %s", status, followup)
	}
	// The original offer outbound remains; starting sales creates neither
	// a generic receipt nor an outbox send that would race with future AI resume.
	var outgoing, entries int
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM outbound_messages
		WHERE business_id=$1::uuid`, f.business).Scan(&outgoing); err != nil {
		t.Fatal(err)
	}
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM outbox_entries
		WHERE business_id=$1::uuid`, f.business).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if outgoing != 1 || entries != 0 {
		t.Fatalf("sales start sent an unexpected acknowledgment: outbound=%d outbox=%d", outgoing, entries)
	}
}

func TestSalesStartDeclinedExpiredAndSupersededAreNotAccepted(t *testing.T) {
	for _, status := range []string{"declined", "cancelled", "expired", "superseded"} {
		t.Run(status, func(t *testing.T) {
			f := newSalesWorkflowFixture(t)
			id := createPendingStart(t, f)
			msg := saveStartPostback(t, f, "m24.sales.start:"+id+":decline")
			if status == "declined" {
				_, err := startFixtureService(f).HandleStartButton(context.Background(), ports.SalesButtonAction{
					BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation,
					SourceMessageID: msg, Payload: "m24.sales.start:" + id + ":decline", Now: f.now.Add(time.Minute),
				})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				_, err := f.a.Pool().Exec(context.Background(), `UPDATE sales_start_requests
					SET status=$2,resolved_at=$3,updated_at=$3 WHERE id=$1::uuid`, id, status, f.now)
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err := startFixtureService(f).HandleStartButton(context.Background(), ports.SalesButtonAction{
				BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation,
				SourceMessageID: saveStartPostback(t, f, "m24.sales.start:"+id+":accept"), Payload: "m24.sales.start:" + id + ":accept", Now: f.now.Add(2 * time.Minute),
			})
			if err != nil && status == "declined" {
				t.Fatal(err)
			}
			var count int
			_ = f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM sales_workflows WHERE business_id=$1::uuid`, f.business).Scan(&count)
			if count != 0 {
				t.Fatalf("%s consent created workflow", status)
			}
		})
	}
}

func TestSalesStartRejectsExpiredAndForeignIdentity(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	id := createPendingStart(t, f)
	msg := saveStartPostback(t, f, "m24.sales.start:"+id+":accept")
	for _, tc := range []struct {
		name, business, customer, conversation string
		now                                    time.Time
	}{
				{"foreign customer", f.business, uuid.NewString(), f.conversation, f.now.Add(time.Minute)},
		{"foreign business", uuid.NewString(), f.customer, f.conversation, f.now.Add(time.Minute)},
		{"foreign conversation", f.business, f.customer, uuid.NewString(), f.now.Add(time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := startFixtureService(f).HandleStartButton(context.Background(), ports.SalesButtonAction{
				BusinessID: tc.business, CustomerID: tc.customer, ConversationID: tc.conversation,
				SourceMessageID: msg, Payload: "m24.sales.start:" + id + ":accept", Now: tc.now,
			})
			if err == nil {
				t.Fatal("invalid start consent accepted")
			}
		})
	}
}

// The expected expired-button outcome must COMMIT; an error return from Decide
// would silently roll back the UPDATE when CustomerSalesWorkflowService.Within exits.
func TestSalesStartExpiredPostbackPersistsTerminalStatusAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	id := createPendingStart(t, f)
	message := saveStartPostback(t, f, "m24.sales.start:"+id+":accept")
	action := ports.SalesButtonAction{
		BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation,
		SourceMessageID: message, Payload: "m24.sales.start:"+id+":accept",
		Now: f.now.Add(ports.SalesStartTTL),
	}
	if _, err := startFixtureService(f).HandleStartButton(context.Background(), action); err != nil {
		t.Fatalf("expired button should be an expected outcome: %v", err)
	}
	var status, continuation string
	var resolvedAt time.Time
	var decisionMessage *string
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT status,continuation_state,resolved_at,decision_message_id::text
		FROM sales_start_requests WHERE business_id=$1::uuid AND id=$2::uuid`, f.business, id).
		Scan(&status, &continuation, &resolvedAt, &decisionMessage); err != nil {
		t.Fatalf("expired state was not committed: %v", err)
	}
	if status != "expired" || continuation != "not_required" || !resolvedAt.Equal(action.Now) || decisionMessage != nil {
		t.Fatalf("invalid persisted expiry: status=%s continuation=%s resolved=%s decision=%v", status, continuation, resolvedAt, decisionMessage)
	}
	// Retry with another valid postback must never reopen an already expired start.
	action.SourceMessageID = saveStartPostback(t, f, action.Payload)
	if _, err := startFixtureService(f).HandleStartButton(context.Background(), action); err != nil {
		t.Fatalf("already-expired button should not produce an operational failure: %v", err)
	}
	var workflows, leads, entries int
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM sales_workflows WHERE business_id=$1::uuid`, f.business).Scan(&workflows); err != nil { t.Fatal(err) }
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM leads WHERE business_id=$1::uuid`, f.business).Scan(&leads); err != nil { t.Fatal(err) }
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM outbox_entries WHERE business_id=$1::uuid`, f.business).Scan(&entries); err != nil { t.Fatal(err) }
	if workflows != 0 || leads != 0 || entries != 0 {
		t.Fatalf("expired start created effects: workflows=%d leads=%d outbox=%d", workflows, leads, entries)
	}
}

// Confirmation of a real transaction still sends the existing customer receipt.
func TestSalesStartNoReceiptDoesNotSuppressFinalTransactionConfirmationAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	draft, err := f.apply(t, "final-confirmation-regression")
	if err != nil { t.Fatal(err) }
	responseID := uuid.NewString()
	if _, err := f.a.Pool().Exec(context.Background(), `INSERT INTO communication_messages(
		id,business_id,conversation_reference_id,direction,origin,transport,provider_message_id,
		content_type,text_content,content_reference,occurred_at,created_at)
		VALUES($1::uuid,$2::uuid,$3::uuid,'inbound','customer','provider',$1::text,
		'text','نعم','inline://fixture',now(),now())`, responseID, f.business, f.reference); err != nil {
		t.Fatal(err)
	}
	result, err := startFixtureService(f).HandleButton(context.Background(), ports.SalesButtonAction{
		BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation,
		SourceMessageID: responseID, Payload: "m24.sales:"+draft.ConfirmationID+":accept",
		Now: f.now.Add(time.Minute),
	})
	if err != nil || result.Workflow.State != "confirmed" {
		t.Fatalf("final customer confirmation failed: %+v, %v", result, err)
	}
	var reference string
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT o.content_reference
		FROM outbound_messages o WHERE o.business_id=$1::uuid
		ORDER BY o.created_at DESC LIMIT 1`, f.business).Scan(&reference); err != nil { t.Fatal(err) }
	message, err := services.DecodeInteractiveContentReference(reference)
	if err != nil || message.Text != "تم اعتماد طلبك." {
		t.Fatalf("final-confirmation receipt was changed: %+v, %v", message, err)
	}
	var entries int
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM outbox_entries WHERE business_id=$1::uuid`, f.business).Scan(&entries); err != nil { t.Fatal(err) }
	if entries != 1 { t.Fatalf("expected exactly one final-confirmation send, got %d", entries) }
}
