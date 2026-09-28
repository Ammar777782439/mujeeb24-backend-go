//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
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
	const catalogA = "00000000-0000-0000-0000-000000000081"
	const catalogB = "00000000-0000-0000-0000-000000000082"
	const catalogC = "00000000-0000-0000-0000-000000000093"
	const schemaA = "00000000-0000-0000-0000-000000000083"
	const schemaB = "00000000-0000-0000-0000-000000000084"
	const definitionA = "00000000-0000-0000-0000-000000000085"
	const definitionB = "00000000-0000-0000-0000-000000000086"
	const itemA = "00000000-0000-0000-0000-000000000087"
	const itemB = "00000000-0000-0000-0000-000000000088"
	const itemC = "00000000-0000-0000-0000-000000000094"
	const variantA = "00000000-0000-0000-0000-000000000089"
	const variantB = "00000000-0000-0000-0000-000000000090"
	const variantC = "00000000-0000-0000-0000-000000000095"
	const offerA = "00000000-0000-0000-0000-000000000091"
	const offerB = "00000000-0000-0000-0000-000000000092"
	const offerC = "00000000-0000-0000-0000-000000000096"
	const catalogWrite = "00000000-0000-0000-0000-000000000097"
	const schemaWrite = "00000000-0000-0000-0000-000000000098"
	const definitionWrite = "00000000-0000-0000-0000-000000000099"
	const itemWrite = "00000000-0000-0000-0000-000000000100"
	const variantWrite = "00000000-0000-0000-0000-000000000101"
	const offerWrite = "00000000-0000-0000-0000-000000000102"
	const catalogRollback = "00000000-0000-0000-0000-000000000103"
	_, _ = pool.Exec(ctx, `DELETE FROM offers WHERE id IN ($1::uuid, $2::uuid, $3::uuid)`, offerA, offerB, offerC)
	_, _ = pool.Exec(ctx, `DELETE FROM offers WHERE id = $1::uuid`, offerWrite)
	_, _ = pool.Exec(ctx, `DELETE FROM variants WHERE id IN ($1::uuid, $2::uuid, $3::uuid)`, variantA, variantB, variantC)
	_, _ = pool.Exec(ctx, `DELETE FROM variants WHERE id = $1::uuid`, variantWrite)
	_, _ = pool.Exec(ctx, `DELETE FROM catalog_items WHERE id IN ($1::uuid, $2::uuid, $3::uuid)`, itemA, itemB, itemC)
	_, _ = pool.Exec(ctx, `DELETE FROM catalog_items WHERE id = $1::uuid`, itemWrite)
	_, _ = pool.Exec(ctx, `DELETE FROM attribute_definitions WHERE id IN ($1::uuid, $2::uuid)`, definitionA, definitionB)
	_, _ = pool.Exec(ctx, `DELETE FROM attribute_definitions WHERE id = $1::uuid`, definitionWrite)
	_, _ = pool.Exec(ctx, `DELETE FROM attribute_schemas WHERE id IN ($1::uuid, $2::uuid)`, schemaA, schemaB)
	_, _ = pool.Exec(ctx, `DELETE FROM attribute_schemas WHERE id = $1::uuid`, schemaWrite)
	_, _ = pool.Exec(ctx, `DELETE FROM catalogs WHERE id IN ($1::uuid, $2::uuid, $3::uuid)`, catalogA, catalogB, catalogC)
	_, _ = pool.Exec(ctx, `DELETE FROM catalogs WHERE id IN ($1::uuid, $2::uuid)`, catalogWrite, catalogRollback)
	for _, id := range []string{businessA, businessB} {
		_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1::uuid`, id)
	}
	_, err = pool.Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now()), ($4::uuid, $5, $6, 'active', 'travel', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, businessA, "Tenant A", "tenant-a", businessB, "Tenant B", "tenant-b")
	if err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessA, businessB)
	defer pool.Exec(context.Background(), `DELETE FROM catalogs WHERE id IN ($1::uuid, $2::uuid)`, catalogWrite, catalogRollback)
	defer pool.Exec(context.Background(), `DELETE FROM attribute_schemas WHERE id = $1::uuid`, schemaWrite)
	defer pool.Exec(context.Background(), `DELETE FROM attribute_definitions WHERE id = $1::uuid`, definitionWrite)
	defer pool.Exec(context.Background(), `DELETE FROM catalog_items WHERE id = $1::uuid`, itemWrite)
	defer pool.Exec(context.Background(), `DELETE FROM variants WHERE id = $1::uuid`, variantWrite)
	defer pool.Exec(context.Background(), `DELETE FROM offers WHERE id = $1::uuid`, offerWrite)
	_, err = pool.Exec(ctx, `INSERT INTO catalogs (id, business_id, name, description, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'Electronics', 'Devices and accessories', 'active', '2025-01-01T09:00:00Z', '2025-01-01T09:00:00Z'), ($3::uuid, $4::uuid, 'Travel', 'Travel offers', 'draft', '2025-01-01T09:01:00Z', '2025-01-01T09:01:00Z'), ($5::uuid, $6::uuid, 'Services', 'Service catalog', 'active', '2025-01-01T09:10:00Z', '2025-01-01T09:10:00Z')`, catalogA, businessA, catalogB, businessB, catalogC, businessA)
	if err != nil {
		t.Fatalf("insert catalogs: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM catalogs WHERE id IN ($1::uuid, $2::uuid, $3::uuid)`, catalogA, catalogB, catalogC)
	_, err = pool.Exec(ctx, `INSERT INTO attribute_schemas (id, business_id, name, version, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'Product attributes', 1, '2025-01-01T09:02:00Z', '2025-01-01T09:02:00Z'), ($3::uuid, $4::uuid, 'Travel attributes', 1, '2025-01-01T09:03:00Z', '2025-01-01T09:03:00Z')`, schemaA, businessA, schemaB, businessB)
	if err != nil {
		t.Fatalf("insert attribute schemas: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM attribute_schemas WHERE id IN ($1::uuid, $2::uuid)`, schemaA, schemaB)
	_, err = pool.Exec(ctx, `INSERT INTO attribute_definitions (id, schema_id, attribute_key, label, data_type, is_required, is_searchable, validation_rules, display_order, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'color', 'Color', 'text', true, true, '{}'::jsonb, 0, '2025-01-01T09:02:01Z', '2025-01-01T09:02:01Z'), ($3::uuid, $4::uuid, 'capacity', 'Capacity', 'number', false, true, '{"min":1}'::jsonb, 1, '2025-01-01T09:02:02Z', '2025-01-01T09:02:02Z')`, definitionA, schemaA, definitionB, schemaB)
	if err != nil {
		t.Fatalf("insert attribute definitions: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM attribute_definitions WHERE id IN ($1::uuid, $2::uuid)`, definitionA, definitionB)
	_, err = pool.Exec(ctx, `INSERT INTO catalog_items (id, business_id, catalog_id, attribute_schema_id, attribute_schema_version, item_type, name, status, pricing_mode, availability_mode, fulfillment_mode, requires_confirmation, attributes, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 1, 'physical_good', 'Phone', 'active', 'fixed', 'stock', 'delivery', false, '{"brand":"Mujeeb"}'::jsonb, '2025-01-01T09:04:00Z', '2025-01-01T09:04:00Z'), ($5::uuid, $6::uuid, $7::uuid, $8::uuid, 1, 'service', 'Travel booking', 'draft', 'quote_required', 'supplier_check', 'travel', true, '{"route":"Sanaa-Cairo"}'::jsonb, '2025-01-01T09:05:00Z', '2025-01-01T09:05:00Z'), ($9::uuid, $10::uuid, $11::uuid, $12::uuid, 1, 'service', 'Repair service', 'active', 'quote_required', 'supplier_check', 'manual', false, '{"category":"repair"}'::jsonb, '2025-01-01T09:11:00Z', '2025-01-01T09:11:00Z')`, itemA, businessA, catalogA, schemaA, itemB, businessB, catalogB, schemaB, itemC, businessA, catalogC, schemaA)
	if err != nil {
		t.Fatalf("insert catalog items: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM catalog_items WHERE id IN ($1::uuid, $2::uuid, $3::uuid)`, itemA, itemB, itemC)
	_, err = pool.Exec(ctx, `INSERT INTO variants (id, business_id, catalog_item_id, name, attributes, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'Black', '{"color":"black"}'::jsonb, 'active', '2025-01-01T09:06:00Z', '2025-01-01T09:06:00Z'), ($4::uuid, $5::uuid, $6::uuid, 'Economy', '{"class":"economy"}'::jsonb, 'inactive', '2025-01-01T09:07:00Z', '2025-01-01T09:07:00Z'), ($7::uuid, $8::uuid, $9::uuid, 'Standard', '{"tier":"standard"}'::jsonb, 'active', '2025-01-01T09:12:00Z', '2025-01-01T09:12:00Z')`, variantA, businessA, itemA, variantB, businessB, itemB, variantC, businessA, itemC)
	if err != nil {
		t.Fatalf("insert variants: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM variants WHERE id IN ($1::uuid, $2::uuid, $3::uuid)`, variantA, variantB, variantC)
	_, err = pool.Exec(ctx, `INSERT INTO offers (id, business_id, catalog_item_id, variant_id, name, pricing_mode, amount, currency, availability_mode, availability_status, fulfillment_mode, price_verification_status, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 'Phone offer', 'fixed', 1250.0000, 'YER', 'stock', 'available', 'delivery', 'verified', 'active', '2025-01-01T09:08:00Z', '2025-01-01T09:08:00Z'), ($5::uuid, $6::uuid, $7::uuid, NULL, 'Travel quote', 'quote_required', NULL, NULL, 'supplier_check', 'requires_check', 'travel', 'unverified', 'draft', '2025-01-01T09:09:00Z', '2025-01-01T09:09:00Z'), ($8::uuid, $9::uuid, $10::uuid, $11::uuid, 'Repair offer', 'quote_required', NULL, NULL, 'supplier_check', 'unknown', 'manual', 'unverified', 'draft', '2025-01-01T09:13:00Z', '2025-01-01T09:13:00Z')`, offerA, businessA, itemA, variantA, offerB, businessB, itemB, offerC, businessA, itemC, variantC)
	if err != nil {
		t.Fatalf("insert offers: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM offers WHERE id IN ($1::uuid, $2::uuid, $3::uuid)`, offerA, offerB, offerC)
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
	_, err = pool.Exec(ctx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'facebook', 'account-a', 'connection-a', 'active', 'secret-ref-a', now(), now())`, connectionA, businessA)
	if err != nil {
		t.Fatalf("insert channel connection: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM channel_connections WHERE id = $1::uuid`, connectionA)
	_, err = pool.Exec(ctx, `INSERT INTO channel_connection_capabilities (connection_id, capability, enabled, checked_at, evidence_source) VALUES ($1::uuid, 'receive_messages', true, '2025-01-01T10:00:00Z', 'provider-contract'), ($1::uuid, 'send_messages', false, '2025-01-01T10:01:00Z', 'health-check'), ($1::uuid, 'media_inbound', true, '2025-01-01T10:02:00Z', NULL)`, connectionA)
	if err != nil {
		t.Fatalf("insert channel capabilities: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM channel_connection_capabilities WHERE connection_id = $1::uuid`, connectionA)
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
	resolvedConnection, err := connectionRepo.GetByProviderReferences(ctx, "socialapi", "account-a", "")
	if err != nil || resolvedConnection.BusinessID != businessA || resolvedConnection.ID != connectionA {
		t.Fatalf("provider reference lookup: %#v err=%v", resolvedConnection, err)
	}
	if _, err := connectionRepo.GetByProviderReferences(ctx, "socialapi", "account-from-other-tenant", ""); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("provider reference lookup should not cross tenant data: %v", err)
	}
	capabilityRepo := NewChannelCapabilityRepository(adapter)
	capabilities, err := capabilityRepo.ListByConnection(ctx, businessA, connectionA)
	if err != nil || len(capabilities) != 3 || capabilities[0].Name != "media_inbound" || capabilities[1].Name != "receive_messages" || capabilities[2].Name != "send_messages" || capabilities[1].EvidenceSource == nil || *capabilities[1].EvidenceSource != "provider-contract" || capabilities[2].Enabled || !capabilities[0].CheckedAt.Equal(time.Date(2025, 1, 1, 10, 2, 0, 0, time.UTC)) {
		t.Fatalf("capability read/order: %#v err=%v", capabilities, err)
	}
	if _, err := capabilityRepo.ListByConnection(ctx, businessB, connectionA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("capability crossed tenant boundary: %v", err)
	}
	capabilityService := services.ConnectionCapabilitiesQueryService{Repository: capabilityRepo}
	capabilityView, err := capabilityService.Handle(ctx, queries.GetConnectionCapabilitiesQuery{Meta: queries.QueryMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}}, ConnectionID: commands.ConnectionID(connectionA)})
	if err != nil || len(capabilityView.Items) != 3 || capabilityView.Items[1].Name != "receive_messages" || !capabilityView.Items[1].Enabled || !capabilityView.Items[1].CheckedAt.Equal(time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)) || capabilityView.Items[1].EvidenceSource != "provider-contract" {
		t.Fatalf("application capability mapping: %#v err=%v", capabilityView, err)
	}
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		executor, txErr := adapter.Executor(txCtx)
		if txErr != nil {
			return txErr
		}
		if _, txErr = executor.Exec(txCtx, `UPDATE channel_connection_capabilities SET enabled = true WHERE connection_id = $1::uuid AND capability = 'send_messages'`, connectionA); txErr != nil {
			return txErr
		}
		inside, txErr := capabilityRepo.ListByConnection(txCtx, businessA, connectionA)
		if txErr != nil || len(inside) != 3 || !inside[2].Enabled {
			return errors.New("capability transaction update not visible")
		}
		return nil
	}); err != nil {
		t.Fatalf("capability commit transaction: %v", err)
	}
	capabilities, err = capabilityRepo.ListByConnection(ctx, businessA, connectionA)
	if err != nil || !capabilities[2].Enabled {
		t.Fatalf("capability commit not visible: %#v err=%v", capabilities, err)
	}
	rollbackErr := errors.New("force capability rollback")
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		executor, txErr := adapter.Executor(txCtx)
		if txErr != nil {
			return txErr
		}
		if _, txErr = executor.Exec(txCtx, `UPDATE channel_connection_capabilities SET enabled = false WHERE connection_id = $1::uuid AND capability = 'send_messages'`, connectionA); txErr != nil {
			return txErr
		}
		return rollbackErr
	}); !errors.Is(err, rollbackErr) {
		t.Fatalf("expected capability rollback error, got %v", err)
	}
	capabilities, err = capabilityRepo.ListByConnection(ctx, businessA, connectionA)
	if err != nil || !capabilities[2].Enabled {
		t.Fatalf("capability rollback leaked: %#v err=%v", capabilities, err)
	}
	catalogRepo := NewCatalogRepository(adapter)
	catalogPage, err := catalogRepo.ListCatalogs(ctx, businessA, "active", 1, "")
	if err != nil || len(catalogPage.Items) != 1 || catalogPage.Items[0].ID != catalogC || !catalogPage.HasMore || catalogPage.NextCursor == "" {
		t.Fatalf("catalog list first page: %#v err=%v", catalogPage, err)
	}
	catalogPage, err = catalogRepo.ListCatalogs(ctx, businessA, "active", 1, catalogPage.NextCursor)
	if err != nil || len(catalogPage.Items) != 1 || catalogPage.Items[0].ID != catalogA || catalogPage.HasMore {
		t.Fatalf("catalog list cursor page: %#v err=%v", catalogPage, err)
	}
	if _, err := catalogRepo.ListCatalogs(ctx, businessA, "", 25, "not-a-cursor"); !IsRepositoryKind(err, RepositoryInvalid) {
		t.Fatalf("expected catalog malformed cursor invalid error, got %v", err)
	}
	catalog, err := catalogRepo.GetCatalog(ctx, businessA, catalogA)
	if err != nil || catalog.Description == nil || *catalog.Description != "Devices and accessories" || catalog.Status != "active" {
		t.Fatalf("catalog read: %#v err=%v", catalog, err)
	}
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		executor, txErr := adapter.Executor(txCtx)
		if txErr != nil {
			return txErr
		}
		if _, txErr = executor.Exec(txCtx, `UPDATE catalogs SET status = 'archived' WHERE id = $1::uuid`, catalogA); txErr != nil {
			return txErr
		}
		inside, txErr := catalogRepo.GetCatalog(txCtx, businessA, catalogA)
		if txErr != nil || inside.Status != "archived" {
			return errors.New("catalog transaction update not visible")
		}
		return nil
	}); err != nil {
		t.Fatalf("catalog transaction commit: %v", err)
	}
	catalog, err = catalogRepo.GetCatalog(ctx, businessA, catalogA)
	if err != nil || catalog.Status != "archived" {
		t.Fatalf("catalog transaction commit not visible: %#v err=%v", catalog, err)
	}
	catalogRollbackErr := errors.New("force catalog rollback")
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		executor, txErr := adapter.Executor(txCtx)
		if txErr != nil {
			return txErr
		}
		if _, txErr = executor.Exec(txCtx, `UPDATE catalogs SET status = 'active' WHERE id = $1::uuid`, catalogA); txErr != nil {
			return txErr
		}
		return catalogRollbackErr
	}); !errors.Is(err, catalogRollbackErr) {
		t.Fatalf("expected catalog rollback error, got %v", err)
	}
	catalog, err = catalogRepo.GetCatalog(ctx, businessA, catalogA)
	if err != nil || catalog.Status != "archived" {
		t.Fatalf("catalog transaction rollback leaked: %#v err=%v", catalog, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE catalogs SET status = 'active' WHERE id = $1::uuid`, catalogA); err != nil {
		t.Fatalf("restore catalog fixture status: %v", err)
	}
	if _, err := catalogRepo.GetCatalog(ctx, businessB, catalogA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("catalog crossed tenant boundary: %v", err)
	}
	itemPage, err := catalogRepo.ListCatalogItems(ctx, businessA, catalogA, "phone", "active", 25, "")
	if err != nil || len(itemPage.Items) != 1 || itemPage.Items[0].ID != itemA || !jsonObjectsEqual(itemPage.Items[0].Attributes, `{"brand":"Mujeeb"}`) {
		t.Fatalf("catalog item list: %#v err=%v", itemPage, err)
	}
	item, err := catalogRepo.GetCatalogItem(ctx, businessA, catalogA, itemA)
	if err != nil || item.AttributeSchemaID == nil || *item.AttributeSchemaID != schemaA || item.AttributeSchemaVersion == nil || *item.AttributeSchemaVersion != 1 {
		t.Fatalf("catalog item read: %#v err=%v", item, err)
	}
	if _, err := catalogRepo.ListCatalogItems(ctx, businessB, catalogA, "", "", 25, ""); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("catalog item crossed tenant boundary: %v", err)
	}
	offerPage, err := catalogRepo.ListOffers(ctx, businessA, itemA, "active", 25, "")
	if err != nil || len(offerPage.Items) != 1 || offerPage.Items[0].ID != offerA || offerPage.Items[0].Amount == nil || *offerPage.Items[0].Amount != "1250.0000" || offerPage.Items[0].Currency == nil || *offerPage.Items[0].Currency != "YER" || offerPage.Items[0].AvailabilityStatus != "available" {
		t.Fatalf("offer list: %#v err=%v", offerPage, err)
	}
	if _, err := catalogRepo.ListOffers(ctx, businessB, itemA, "", 25, ""); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("offer crossed tenant boundary: %v", err)
	}
	variantPage, err := catalogRepo.ListVariants(ctx, businessA, itemA, "active", 25, "")
	if err != nil || len(variantPage.Items) != 1 || variantPage.Items[0].ID != variantA || !jsonObjectsEqual(variantPage.Items[0].Attributes, `{"color":"black"}`) {
		t.Fatalf("variant list: %#v err=%v", variantPage, err)
	}
	if _, err := catalogRepo.ListVariants(ctx, businessB, itemA, "", 25, ""); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("variant crossed tenant boundary: %v", err)
	}
	schemaPage, err := catalogRepo.ListAttributeSchemas(ctx, businessA, "Product attributes", nil, 25, "")
	if err != nil || len(schemaPage.Items) != 1 || schemaPage.Items[0].ID != schemaA || schemaPage.Items[0].Version != 1 || len(schemaPage.Items[0].Definitions) != 1 || schemaPage.Items[0].Definitions[0].Key != "color" {
		t.Fatalf("attribute schema list: %#v err=%v", schemaPage, err)
	}
	schema, err := catalogRepo.GetAttributeSchema(ctx, businessA, schemaA)
	if err != nil || len(schema.Definitions) != 1 || schema.Definitions[0].Key != "color" || schema.Definitions[0].DataType != "text" || !schema.Definitions[0].Required {
		t.Fatalf("attribute schema read: %#v err=%v", schema, err)
	}
	if _, err := catalogRepo.GetAttributeSchema(ctx, businessB, schemaA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("attribute schema crossed tenant boundary: %v", err)
	}
	catalogQueryService := services.ListCatalogsQueryService{Repository: catalogRepo}
	catalogViewPage, err := catalogQueryService.Handle(ctx, queries.ListCatalogsQuery{Meta: queries.QueryMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}}, Limit: 1, Status: "active"})
	if err != nil || len(catalogViewPage.Items) != 1 || catalogViewPage.Items[0].Name != "Services" || !catalogViewPage.HasMore || catalogViewPage.NextCursor == "" {
		t.Fatalf("catalog application mapping: %#v err=%v", catalogViewPage, err)
	}
	schemaQueryService := services.GetAttributeSchemaQueryService{Repository: catalogRepo}
	schemaView, err := schemaQueryService.Handle(ctx, queries.GetAttributeSchemaQuery{Meta: queries.QueryMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}}, SchemaID: commands.AttributeSchemaID(schemaA)})
	if err != nil || len(schemaView.Definitions) != 1 || schemaView.Definitions[0].Key != "color" {
		t.Fatalf("schema application mapping: %#v err=%v", schemaView, err)
	}
	generatedIDs := []string{catalogWrite, definitionWrite, schemaWrite, itemWrite, offerWrite, variantWrite}
	generatedIndex := 0
	commandServices := services.NewCatalogCommandServices(catalogRepo, adapter)
	commandServices.Now = func() time.Time { return time.Date(2025, time.January, 1, 10, 0, 0, 0, time.UTC) }
	commandServices.NewID = func() string {
		id := generatedIDs[generatedIndex]
		generatedIndex++
		return id
	}
	createCatalogService := services.CreateCatalogCommandService{CatalogCommandServices: commandServices}
	createdCatalog, err := createCatalogService.Handle(ctx, commands.CreateCatalogCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}}, Name: "Created Catalog", Description: "Created through application"})
	if err != nil || createdCatalog.Catalog.ID != commands.CatalogID(catalogWrite) || createdCatalog.ResourceVersion != "1" {
		t.Fatalf("catalog command create: %#v err=%v", createdCatalog, err)
	}
	updateCatalogService := services.UpdateCatalogCommandService{CatalogCommandServices: commandServices}
	newCatalogName := "Updated Catalog"
	updatedCatalog, err := updateCatalogService.Handle(ctx, commands.UpdateCatalogCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}, ExpectedVersion: resourceVersion("1")}, CatalogID: commands.CatalogID(catalogWrite), Name: &newCatalogName})
	if err != nil || updatedCatalog.Catalog.Name != newCatalogName || updatedCatalog.ResourceVersion != "2" {
		t.Fatalf("catalog command update: %#v err=%v", updatedCatalog, err)
	}
	if _, err := updateCatalogService.Handle(ctx, commands.UpdateCatalogCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}, ExpectedVersion: resourceVersion("1")}, CatalogID: commands.CatalogID(catalogWrite), Name: &newCatalogName}); !isApplicationCode(err, "stale_resource") {
		t.Fatalf("expected stale catalog update, got %v", err)
	}
	createSchemaService := services.CreateAttributeSchemaVersionCommandService{CatalogCommandServices: commandServices}
	createdSchema, err := createSchemaService.Handle(ctx, commands.CreateAttributeSchemaVersionCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}}, Name: "Write schema", Definitions: []commands.AttributeDefinition{{Key: "brand", Label: "Brand", DataType: "text", Required: true, Searchable: true, DisplayOrder: 0}}})
	if err != nil || createdSchema.Schema.ID != commands.AttributeSchemaID(schemaWrite) || createdSchema.Schema.Version != 1 || len(createdSchema.Schema.Definitions) != 1 {
		t.Fatalf("schema command create: %#v err=%v", createdSchema, err)
	}
	createItemService := services.CreateCatalogItemCommandService{CatalogCommandServices: commandServices}
	createdItem, err := createItemService.Handle(ctx, commands.CreateCatalogItemCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}}, CatalogID: commands.CatalogID(catalogWrite), AttributeSchemaID: attributeSchemaID(schemaWrite), ItemType: "physical_good", Name: "Created phone", PricingMode: "fixed", AvailabilityMode: "stock", FulfillmentMode: "delivery", Attributes: map[string]any{"brand": "Mujeeb"}})
	if err != nil || createdItem.Item.ID != commands.CatalogItemID(itemWrite) || createdItem.Item.AttributeSchemaVersion == nil || *createdItem.Item.AttributeSchemaVersion != 1 {
		t.Fatalf("item command create: %#v err=%v", createdItem, err)
	}
	createOfferService := services.CreateOfferCommandService{CatalogCommandServices: commandServices}
	amountMinor := int64(125050)
	createdOffer, err := createOfferService.Handle(ctx, commands.CreateOfferCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}}, CatalogItemID: commands.CatalogItemID(itemWrite), Name: "Created offer", PricingMode: "fixed", AmountMinor: &amountMinor, Currency: stringPointer("YER"), AvailabilityMode: "stock", AvailabilityStatus: "available", FulfillmentMode: "delivery", Status: "active"})
	if err != nil || createdOffer.Offer.ID != commands.OfferID(offerWrite) || createdOffer.Offer.Amount == nil || *createdOffer.Offer.Amount != "1250.5000" || createdOffer.ResourceVersion != "1" {
		t.Fatalf("offer command create: %#v err=%v", createdOffer, err)
	}
	createVariantService := services.CreateVariantCommandService{CatalogCommandServices: commandServices}
	createdVariant, err := createVariantService.Handle(ctx, commands.CreateVariantCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}}, CatalogItemID: commands.CatalogItemID(itemWrite), Name: "Created black", Attributes: map[string]any{"color": "black"}})
	if err != nil || createdVariant.Variant.ID != commands.VariantID(variantWrite) || createdVariant.ResourceVersion != "1" {
		t.Fatalf("variant command create: %#v err=%v", createdVariant, err)
	}
	writeRollbackErr := errors.New("force catalog write rollback")
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		_, err := catalogRepo.CreateCatalog(txCtx, ports.CatalogDraft{ID: catalogRollback, BusinessID: businessA, Name: "Rolled back catalog", Status: "draft", CreatedAt: commandServices.Now(), UpdatedAt: commandServices.Now()})
		if err != nil {
			return err
		}
		return writeRollbackErr
	}); !errors.Is(err, writeRollbackErr) {
		t.Fatalf("expected catalog write rollback error, got %v", err)
	}
	if _, err := catalogRepo.GetCatalog(ctx, businessA, catalogRollback); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("catalog write rollback leaked: %v", err)
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
	providerMediaDraft := invalidTextDraft
	providerMediaDraft.ID = communicationD
	providerMediaDraft.ContentType = "image"
	providerMediaDraft.TextContent = nil
	providerMediaDraft.ProviderMessageID = strptr("provider-message-media-1")
	providerMediaDraft.ContentReference = "content-provider-media"
	providerMediaDraft.OccurredAt = time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	providerMediaDraft.CreatedAt = time.Date(2025, 1, 1, 8, 0, 1, 0, time.UTC)
	if _, err := messageRepo.Record(ctx, providerMediaDraft); err != nil {
		t.Fatalf("provider media record: %v", err)
	}
	duplicateProviderMedia := providerMediaDraft
	duplicateProviderMedia.ID = communicationE
	if _, err := messageRepo.Record(ctx, duplicateProviderMedia); !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("expected provider media uniqueness conflict, got %v", err)
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
	messageRollbackErr := errors.New("force message rollback")
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		if _, txErr := messageRepo.Record(txCtx, ports.CommunicationMessageDraft{ID: rollbackID, BusinessID: businessA, ConversationReferenceID: referenceA, Direction: "inbound", Origin: "customer", Transport: "provider", ContentType: "text", TextContent: &rollbackText, ContentReference: "content-rollback", OccurredAt: time.Date(2025, 1, 1, 10, 4, 0, 0, time.UTC), CreatedAt: time.Date(2025, 1, 1, 10, 4, 1, 0, time.UTC)}); txErr != nil {
			return txErr
		}
		return messageRollbackErr
	}); !errors.Is(err, messageRollbackErr) {
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

func jsonObjectsEqual(got []byte, want string) bool {
	var gotObject any
	var wantObject any
	if json.Unmarshal(got, &gotObject) != nil || json.Unmarshal([]byte(want), &wantObject) != nil {
		return false
	}
	return reflect.DeepEqual(gotObject, wantObject)
}

func resourceVersion(value string) *commands.ResourceVersion {
	version := commands.ResourceVersion(value)
	return &version
}

func attributeSchemaID(value string) *commands.AttributeSchemaID {
	id := commands.AttributeSchemaID(value)
	return &id
}

func stringPointer(value string) *string {
	return &value
}

func isApplicationCode(err error, code string) bool {
	var typed *appErrors.Error
	return errors.As(err, &typed) && string(typed.Code) == code
}
