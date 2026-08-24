//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func TestBusinessRepositoryAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()
	repo := NewBusinessRepository(adapter)
	const id = "00000000-0000-0000-0000-000000000009"
	_, err = adapter.Pool().Exec(ctx, `DELETE FROM businesses WHERE id = $1::uuid`, id)
	if err != nil {
		t.Fatalf("cleanup before test: %v", err)
	}
	_, err = adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, id, "Integration Shop", "integration-shop")
	if err != nil {
		t.Fatalf("insert business: %v", err)
	}
	defer adapter.Pool().Exec(context.Background(), `DELETE FROM businesses WHERE id = $1::uuid`, id)

	record, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get business: %v", err)
	}
	if record.ID != id || record.Name != "Integration Shop" || record.DefaultCurrency != "YER" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if _, err := repo.GetByID(ctx, "00000000-0000-0000-0000-000000000010"); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected not-found repository error, got %v", err)
	}

	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(txCtx, `UPDATE businesses SET name = $1 WHERE id = $2::uuid`, "Committed Shop", id)
		return err
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	record, err = repo.GetByID(ctx, id)
	if err != nil || record.Name != "Committed Shop" {
		t.Fatalf("commit not visible: record=%#v err=%v", record, err)
	}

	rollbackErr := errors.New("force rollback")
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		if _, err = executor.Exec(txCtx, `UPDATE businesses SET name = $1 WHERE id = $2::uuid`, "Rolled Back Shop", id); err != nil {
			return err
		}
		return rollbackErr
	}); !errors.Is(err, rollbackErr) {
		t.Fatalf("expected rollback error, got %v", err)
	}
	record, err = repo.GetByID(ctx, id)
	if err != nil || record.Name != "Committed Shop" {
		t.Fatalf("rollback not preserved: record=%#v err=%v", record, err)
	}
}

func TestCoreRepositoriesRespectBusinessScopeAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()
	pool := adapter.Pool()
	const businessA = "00000000-0000-0000-0000-000000000011"
	const businessB = "00000000-0000-0000-0000-000000000012"
	const customerA = "00000000-0000-0000-0000-000000000021"
	const customerB = "00000000-0000-0000-0000-000000000022"
	const conversationA = "00000000-0000-0000-0000-000000000031"
	const connectionA = "00000000-0000-0000-0000-000000000041"
	const referenceA = "00000000-0000-0000-0000-000000000051"
	const outboundA = "00000000-0000-0000-0000-000000000061"
	const outboundB = "00000000-0000-0000-0000-000000000062"
	const communicationA = "00000000-0000-0000-0000-000000000071"
	const communicationB = "00000000-0000-0000-0000-000000000072"
	const communicationC = "00000000-0000-0000-0000-000000000073"
	const communicationD = "00000000-0000-0000-0000-000000000076"
	const communicationE = "00000000-0000-0000-0000-000000000077"
	const communicationF = "00000000-0000-0000-0000-000000000078"
	const communicationG = "00000000-0000-0000-0000-000000000079"
	for _, id := range []string{businessA, businessB} {
		_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1::uuid`, id)
	}
	_, err = pool.Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now()), ($4::uuid, $5, $6, 'active', 'travel', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, businessA, "Tenant A", "tenant-a", businessB, "Tenant B", "tenant-b")
	if err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessA, businessB)
	_, err = pool.Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, locale_preference, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, '{"name":"A"}', '[]', 'ar-YE', 'active', now(), now()), ($3::uuid, $4::uuid, '{"name":"B"}', '[]', 'ar-YE', 'active', now(), now())`, customerA, businessA, customerB, businessB)
	if err != nil {
		t.Fatalf("insert customers: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM customers WHERE id IN ($1::uuid, $2::uuid)`, customerA, customerB)
	_, err = pool.Exec(ctx, `INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'open', 'none', 'normal', now(), now(), now())`, conversationA, businessA, customerA)
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM conversations WHERE id = $1::uuid`, conversationA)
	_, err = pool.Exec(ctx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'facebook', 'connection-a', 'active', 'secret-ref-a', now(), now())`, connectionA, businessA)
	if err != nil {
		t.Fatalf("insert channel connection: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM channel_connections WHERE id = $1::uuid`, connectionA)
	_, err = pool.Exec(ctx, `INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'provider', 'socialapi', 'conversation', 'provider-conversation-a', $4::uuid, 'dm', true, 'active', now(), now())`, referenceA, businessA, conversationA, connectionA)
	if err != nil {
		t.Fatalf("insert conversation reference: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM conversation_references WHERE id = $1::uuid`, referenceA)

	customerRepo := NewCustomerRepository(adapter)
	customer, err := customerRepo.GetByID(ctx, businessA, customerA)
	if err != nil || customer.BusinessID != businessA {
		t.Fatalf("customer read: %#v err=%v", customer, err)
	}
	if _, err := customerRepo.GetByID(ctx, businessB, customerA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("customer crossed tenant boundary: %v", err)
	}
	conversationRepo := NewConversationRepository(adapter)
	conversation, err := conversationRepo.GetByID(ctx, businessA, conversationA)
	if err != nil || conversation.CustomerID != customerA {
		t.Fatalf("conversation read: %#v err=%v", conversation, err)
	}
	if _, err := conversationRepo.GetByID(ctx, businessB, conversationA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("conversation crossed tenant boundary: %v", err)
	}
	connectionRepo := NewChannelConnectionRepository(adapter)
	connection, err := connectionRepo.GetByID(ctx, businessA, connectionA)
	if err != nil || connection.ProviderReference != "socialapi" {
		t.Fatalf("connection read: %#v err=%v", connection, err)
	}
	if _, err := connectionRepo.GetByID(ctx, businessB, connectionA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("connection crossed tenant boundary: %v", err)
	}
	referenceRepo := NewConversationReferenceRepository(adapter)
	reference, err := referenceRepo.GetCurrentByConversation(ctx, businessA, conversationA, "provider")
	if err != nil || reference.ID != referenceA || reference.ConnectionID == nil || *reference.ConnectionID != connectionA {
		t.Fatalf("reference read: %#v err=%v", reference, err)
	}
	if _, err := referenceRepo.GetCurrentByConversation(ctx, businessB, conversationA, "provider"); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("reference crossed tenant boundary: %v", err)
	}
	outboundRepo := NewOutboundMessageRepository(adapter)
	draft := ports.OutboundMessageDraft{ID: outboundA, BusinessID: businessA, ConversationID: conversationA, ConversationReferenceID: referenceA, ConnectionID: connectionA, ProviderRef: "socialapi", Channel: "facebook", Origin: "human", Transport: "provider", ContentReference: "content-ref-a", ProviderIdempotencyKey: "idem-a"}
	outbound, err := outboundRepo.CreatePending(ctx, draft)
	if err != nil || outbound.ID != outboundA || outbound.Status != "pending" || outbound.Direction != "outbound" {
		t.Fatalf("outbound create: %#v err=%v", outbound, err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM outbound_messages WHERE id = $1::uuid`, outboundA)
	loaded, err := outboundRepo.GetByID(ctx, businessA, outboundA)
	if err != nil || loaded.ID != outboundA {
		t.Fatalf("outbound read: %#v err=%v", loaded, err)
	}
	if _, err := outboundRepo.GetByID(ctx, businessB, outboundA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("outbound crossed tenant boundary: %v", err)
	}
	duplicate := draft
	duplicate.ID = outboundB
	if _, err := outboundRepo.CreatePending(ctx, duplicate); !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	messageRepo := NewMessageRepository(adapter)
	textA, textB, textC := "رسالة أولى", "رد ثانٍ", "رسالة ثالثة"
	for _, draft := range []ports.CommunicationMessageDraft{
		{ID: communicationA, BusinessID: businessA, ConversationReferenceID: referenceA, Direction: "inbound", Origin: "customer", Transport: "provider", ProviderMessageID: strptr("provider-message-a"), ContentType: "text", TextContent: &textA, ContentReference: "content-a", OccurredAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), CreatedAt: time.Date(2025, 1, 1, 10, 0, 1, 0, time.UTC)},
		{ID: communicationB, BusinessID: businessA, ConversationReferenceID: referenceA, OutboundMessageID: strptr(outboundA), Direction: "outbound", Origin: "human", Transport: "provider", ProviderMessageID: strptr("provider-message-b"), ContentType: "text", TextContent: &textB, ContentReference: "content-b", OccurredAt: time.Date(2025, 1, 1, 10, 1, 0, 0, time.UTC), CreatedAt: time.Date(2025, 1, 1, 10, 1, 1, 0, time.UTC)},
		{ID: communicationC, BusinessID: businessA, ConversationReferenceID: referenceA, Direction: "inbound", Origin: "customer", Transport: "provider", ProviderMessageID: strptr("provider-message-c"), ContentType: "text", TextContent: &textC, ContentReference: "content-c", OccurredAt: time.Date(2025, 1, 1, 10, 2, 0, 0, time.UTC), CreatedAt: time.Date(2025, 1, 1, 10, 2, 1, 0, time.UTC)},
	} {
		recorded, recordErr := messageRepo.Record(ctx, draft)
		if recordErr != nil || recorded.BusinessID != businessA || recorded.ConversationID != conversationA {
			t.Fatalf("record communication message: %#v err=%v", recorded, recordErr)
		}
	}
	committedID := "00000000-0000-0000-0000-000000000074"
	defer pool.Exec(context.Background(), `DELETE FROM communication_messages WHERE id IN ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6::uuid, $7::uuid)`, communicationA, communicationB, communicationC, communicationD, committedID, communicationE, communicationG)
	invalidTextDraft := ports.CommunicationMessageDraft{ID: communicationE, BusinessID: businessA, ConversationReferenceID: referenceA, Direction: "inbound", Origin: "customer", Transport: "provider", ContentType: "text", ContentReference: "content-invalid", OccurredAt: time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC), CreatedAt: time.Date(2025, 1, 1, 9, 0, 1, 0, time.UTC)}
	if _, err := messageRepo.Record(ctx, invalidTextDraft); !IsRepositoryKind(err, RepositoryInvalid) {
		t.Fatalf("expected text content constraint invalid error, got %v", err)
	}
	crossBusinessDraft := invalidTextDraft
	crossBusinessDraft.ID = communicationF
	crossBusinessDraft.BusinessID = businessB
	crossBusinessDraft.ContentReference = "content-cross-business"
	crossBusinessDraft.TextContent = &textA
	if _, err := messageRepo.Record(ctx, crossBusinessDraft); !IsRepositoryKind(err, RepositoryInvalid) {
		t.Fatalf("expected cross-business FK invalid error, got %v", err)
	}
	chatwootDraft := invalidTextDraft
	chatwootDraft.ID = communicationD
	chatwootDraft.ContentType = "image"
	chatwootDraft.TextContent = nil
	chatwootDraft.ChatwootMessageID = strptr("chatwoot-message-1")
	chatwootDraft.ContentReference = "content-chatwoot"
	chatwootDraft.OccurredAt = time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	chatwootDraft.CreatedAt = time.Date(2025, 1, 1, 8, 0, 1, 0, time.UTC)
	if _, err := messageRepo.Record(ctx, chatwootDraft); err != nil {
		t.Fatalf("chatwoot reference record: %v", err)
	}
	duplicateChatwoot := chatwootDraft
	duplicateChatwoot.ID = communicationE
	if _, err := messageRepo.Record(ctx, duplicateChatwoot); !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("expected chatwoot uniqueness conflict, got %v", err)
	}
	duplicateProvider := ports.CommunicationMessageDraft{ID: communicationG, BusinessID: businessA, ConversationReferenceID: referenceA, Direction: "inbound", Origin: "customer", Transport: "provider", ProviderMessageID: strptr("provider-message-a"), ContentType: "text", TextContent: &textA, ContentReference: "content-provider-duplicate", OccurredAt: time.Date(2025, 1, 1, 7, 0, 0, 0, time.UTC), CreatedAt: time.Date(2025, 1, 1, 7, 0, 1, 0, time.UTC)}
	if _, err := messageRepo.Record(ctx, duplicateProvider); !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("expected provider uniqueness conflict, got %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM communication_messages WHERE id = $1::uuid`, communicationD); err != nil {
		t.Fatalf("cleanup constraint fixture: %v", err)
	}
	if _, err := messageRepo.ListByConversation(ctx, businessA, conversationA, 2, "not-a-cursor"); !IsRepositoryKind(err, RepositoryInvalid) {
		t.Fatalf("expected malformed cursor invalid error, got %v", err)
	}
	commitText := "رسالة transaction committed"
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		_, txErr := messageRepo.Record(txCtx, ports.CommunicationMessageDraft{ID: committedID, BusinessID: businessA, ConversationReferenceID: referenceA, Direction: "inbound", Origin: "customer", Transport: "provider", ContentType: "text", TextContent: &commitText, ContentReference: "content-committed", OccurredAt: time.Date(2025, 1, 1, 10, 3, 0, 0, time.UTC), CreatedAt: time.Date(2025, 1, 1, 10, 3, 1, 0, time.UTC)})
		return txErr
	}); err != nil {
		t.Fatalf("message commit transaction: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM communication_messages WHERE id = $1::uuid`, committedID)
	rollbackID := "00000000-0000-0000-0000-000000000075"
	rollbackText := "رسالة transaction rolled back"
	rollbackErr := errors.New("force message rollback")
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		if _, txErr := messageRepo.Record(txCtx, ports.CommunicationMessageDraft{ID: rollbackID, BusinessID: businessA, ConversationReferenceID: referenceA, Direction: "inbound", Origin: "customer", Transport: "provider", ContentType: "text", TextContent: &rollbackText, ContentReference: "content-rollback", OccurredAt: time.Date(2025, 1, 1, 10, 4, 0, 0, time.UTC), CreatedAt: time.Date(2025, 1, 1, 10, 4, 1, 0, time.UTC)}); txErr != nil {
			return txErr
		}
		return rollbackErr
	}); !errors.Is(err, rollbackErr) {
		t.Fatalf("expected message rollback error, got %v", err)
	}
	var rollbackCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM communication_messages WHERE id = $1::uuid`, rollbackID).Scan(&rollbackCount); err != nil || rollbackCount != 0 {
		t.Fatalf("message rollback leaked row: count=%d err=%v", rollbackCount, err)
	}
	page, err := messageRepo.ListByConversation(ctx, businessA, conversationA, 2, "")
	if err != nil || len(page.Items) != 2 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("first message page: %#v err=%v", page, err)
	}
	if page.Items[0].ID != committedID || page.Items[1].ID != communicationC || page.Items[0].Status != "received" || page.Items[1].Status != "received" {
		t.Fatalf("unexpected message ordering/status: %#v", page.Items)
	}
	second, err := messageRepo.ListByConversation(ctx, businessA, conversationA, 2, page.NextCursor)
	if err != nil || len(second.Items) != 2 || second.HasMore || second.Items[0].ID != communicationB || second.Items[1].ID != communicationA || second.Items[0].Status != "pending" {
		t.Fatalf("second message page: %#v err=%v", second, err)
	}
	if _, err := messageRepo.ListByConversation(ctx, businessB, conversationA, 2, ""); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("cross-business list must be typed not-found, got %v", err)
	}
	if _, err := messageRepo.ListByConversation(ctx, businessA, "00000000-0000-0000-0000-000000000099", 2, ""); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("missing conversation list must be typed not-found, got %v", err)
	}
	service := services.MessageQueryService{Repository: messageRepo}
	viewPage, err := service.Handle(ctx, queries.ListConversationMessagesQuery{Meta: queries.QueryMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}}, ConversationID: commands.ConversationID(conversationA), Limit: 2})
	if err != nil || len(viewPage.Items) != 2 || viewPage.Items[0].Text != commitText || viewPage.Items[1].Text != textC || viewPage.Items[1].ProviderMessageReference == nil || !viewPage.Items[0].OccurredAt.Equal(time.Date(2025, 1, 1, 10, 3, 0, 0, time.UTC)) {
		t.Fatalf("application message view: %#v err=%v", viewPage, err)
	}
}

func strptr(value string) *string { return &value }
