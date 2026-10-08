//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

func TestChannelHistoryPurgePreservesOtherChannelsTenantsAndLeads(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	pool := adapter.Pool()
	businessA, businessB := uuid.NewString(), uuid.NewString()
	channelA, channelSibling, channelB := uuid.NewString(), uuid.NewString(), uuid.NewString()
	customerA, customerSibling, customerB := uuid.NewString(), uuid.NewString(), uuid.NewString()
	conversationA, conversationSibling, conversationB := uuid.NewString(), uuid.NewString(), uuid.NewString()
	referenceA, referenceSibling, referenceB := uuid.NewString(), uuid.NewString(), uuid.NewString()
	messageIDs := []string{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
	eventID := uuid.NewString()
	leadID := uuid.NewString()
	base := time.Now().Add(-24 * time.Hour).UTC()
	t.Cleanup(func() {
		cleanCtx := context.Background()
		for _, q := range []string{
			`DELETE FROM channel_history_purges WHERE business_id IN ($1::uuid,$2::uuid)`,
			`DELETE FROM communication_messages WHERE business_id IN ($1::uuid,$2::uuid)`,
			`DELETE FROM inbound_event_ledger WHERE business_id IN ($1::uuid,$2::uuid)`,
			`DELETE FROM leads WHERE business_id IN ($1::uuid,$2::uuid)`,
			`DELETE FROM conversation_references WHERE business_id IN ($1::uuid,$2::uuid)`,
			`DELETE FROM conversations WHERE business_id IN ($1::uuid,$2::uuid)`,
			`DELETE FROM customers WHERE business_id IN ($1::uuid,$2::uuid)`,
			`DELETE FROM channel_connections WHERE business_id IN ($1::uuid,$2::uuid)`,
			`DELETE FROM businesses WHERE id IN ($1::uuid,$2::uuid)`,
		} {
			_, _ = pool.Exec(cleanCtx, q, businessA, businessB)
		}
	})
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	must(`INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at)
        VALUES ($1::uuid,'History A',$1,'active','retail','Asia/Aden','YER','ar-YE',now(),now()),
               ($2::uuid,'History B',$2,'active','retail','Asia/Aden','YER','ar-YE',now(),now())`, businessA, businessB)
	for _, item := range []struct{ id, business, channel string }{
		{channelA, businessA, "facebook"}, {channelSibling, businessA, "instagram"}, {channelB, businessB, "facebook"},
	} {
		must(`INSERT INTO channel_connections (id,business_id,provider_ref,channel,provider_account_ref,provider_connection_ref,status,secret_reference,created_at,updated_at)
          VALUES ($1::uuid,$2::uuid,'socialapi',$3,$1,$1,'active','secret-ref',now(),now())`, item.id, item.business, item.channel)
	}
	for _, item := range []struct{ id, business string }{
		{customerA, businessA}, {customerSibling, businessA}, {customerB, businessB},
	} {
		must(`INSERT INTO customers (id,business_id,profile,contact_points,status,created_at,updated_at)
             VALUES ($1::uuid,$2::uuid,'{}','[]','active',now(),now())`, item.id, item.business)
	}
	for _, item := range []struct{ id, business, customer string }{
		{conversationA, businessA, customerA}, {conversationSibling, businessA, customerSibling}, {conversationB, businessB, customerB},
	} {
		must(`INSERT INTO conversations (id,business_id,customer_id,state,ownership,priority,last_activity_at,created_at,updated_at)
        VALUES ($1::uuid,$2::uuid,$3::uuid,'open','ai','normal',now(),now(),now())`, item.id, item.business, item.customer)
	}
	for _, item := range []struct{ id, business, conversation, connection string }{
		{referenceA, businessA, conversationA, channelA},
		{referenceSibling, businessA, conversationSibling, channelSibling},
		{referenceB, businessB, conversationB, channelB},
	} {
		must(`INSERT INTO conversation_references (id,business_id,conversation_id,system,provider_ref,resource_type,resource_id,connection_id,is_current,mapping_status,created_at,updated_at)
          VALUES ($1::uuid,$2::uuid,$3::uuid,'provider','socialapi','conversation',$1,$4::uuid,true,'active',now(),now())`,
			item.id, item.business, item.conversation, item.connection)
	}
	for i, item := range []struct{ business, reference, direction, origin string }{
		{businessA, referenceA, "inbound", "customer"},
		{businessA, referenceA, "outbound", "ai"},
		{businessA, referenceSibling, "inbound", "customer"},
		{businessB, referenceB, "inbound", "customer"},
	} {
		must(`INSERT INTO communication_messages (id,business_id,conversation_reference_id,direction,origin,transport,content_type,text_content,content_reference,occurred_at,created_at)
          VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,'provider','text','secret customer text','inline:private',now(),now())`,
			messageIDs[i], item.business, item.reference, item.direction, item.origin)
	}
	raw, err := NewRawPayloadStore(adapter).Put(ctx, "socialapi", uuid.NewString(), []byte(`{"message":"sensitive webhook body"}`))
	if err != nil {
		t.Fatal(err)
	}
	must(`INSERT INTO inbound_event_ledger (id,provider_ref,provider_connection_ref,provider_event_id,dedupe_strategy,
      business_id,connection_id,event_type,received_at,raw_payload_reference,payload_hash,signature_verified,processing_state,created_at,updated_at)
      VALUES ($1::uuid,'socialapi',$2::text,$1,'provider_event_id',$3::uuid,$4::uuid,'interaction_received',$5,$6,$7,true,'received',now(),now())`,
		eventID, channelA, businessA, channelA, base, raw.Reference, raw.SHA256)
	must(`INSERT INTO leads (id,business_id,customer_id,status,created_by,source_conversation_reference_id,created_at,updated_at)
      VALUES ($1::uuid,$2::uuid,$3::uuid,'new','human',$4::uuid,now(),now())`,
		leadID, businessA, customerA, referenceA)
	history := NewChannelHistoryRepository(adapter)
	before, err := history.Preview(ctx, businessA, channelA)
	if err != nil || before.Messages != 2 || before.Conversations != 1 {
		t.Fatalf("preview=%+v err=%v", before, err)
	}
	if _, err := history.Purge(ctx, businessB, channelA, "wrong-tenant", 1); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected tenant-scoped not found, got %v", err)
	}
	result, err := history.Purge(ctx, businessA, channelA, "first-purge", 1)
	if err != nil || result.Messages != 2 || result.Conversations != 1 || result.PurgedAt == nil {
		t.Fatalf("purge=%+v err=%v", result, err)
	}
	again, err := history.Purge(ctx, businessA, channelA, "first-purge", 1)
	if err != nil || again.Messages != 2 {
		t.Fatalf("idempotency=%+v err=%v", again, err)
	}
	for _, check := range []struct {
		query string
		args  []any
		want  int
	}{
		{`SELECT count(*) FROM communication_messages WHERE business_id=$1::uuid`, []any{businessA}, 1},
		{`SELECT count(*) FROM communication_messages WHERE business_id=$1::uuid`, []any{businessB}, 1},
		{`SELECT count(*) FROM conversations WHERE business_id=$1::uuid`, []any{businessA}, 1},
		{`SELECT count(*) FROM channel_connections WHERE business_id=$1::uuid`, []any{businessA}, 2},
		{`SELECT count(*) FROM customers WHERE business_id=$1::uuid`, []any{businessA}, 2},
		{`SELECT count(*) FROM leads WHERE business_id=$1::uuid`, []any{businessA}, 1},
		{`SELECT count(*) FROM inbound_webhook_payloads WHERE id=split_part($1,'/',4)::uuid`, []any{raw.Reference}, 0},
	} {
		var count int
		if err := pool.QueryRow(ctx, check.query, check.args...).Scan(&count); err != nil || count != check.want {
			t.Errorf("%s: count=%d want=%d err=%v", check.query, count, check.want, err)
		}
	}
	var leadReference *string
	if err := pool.QueryRow(ctx, `SELECT source_conversation_reference_id::text FROM leads WHERE business_id=$1::uuid AND id=$2::uuid`, businessA, leadID).Scan(&leadReference); err != nil || leadReference != nil {
		t.Errorf("lead reference not detached: %v / %v", leadReference, err)
	}
	old := base
	if skipped, err := history.ShouldIgnore(ctx, businessA, channelA, eventID, &old); err != nil || !skipped {
		t.Errorf("old event restored, ignored=%v err=%v", skipped, err)
	}
	newTime := time.Now().Add(time.Minute)
	if skipped, err := history.ShouldIgnore(ctx, businessA, channelA, uuid.NewString(), &newTime); err != nil || skipped {
		t.Errorf("new event blocked, ignored=%v err=%v", skipped, err)
	}
}
