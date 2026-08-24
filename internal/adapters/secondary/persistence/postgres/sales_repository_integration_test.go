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

func TestSalesRepositoriesAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()
	pool := adapter.Pool()

	const businessA = "00000000-0000-0000-0000-000000000201"
	const businessB = "00000000-0000-0000-0000-000000000202"
	const customerA = "00000000-0000-0000-0000-000000000211"
	const customerB = "00000000-0000-0000-0000-000000000212"
	const catalogA = "00000000-0000-0000-0000-000000000221"
	const itemA = "00000000-0000-0000-0000-000000000222"
	const variantA = "00000000-0000-0000-0000-000000000223"
	const offerA = "00000000-0000-0000-0000-000000000224"
	const leadA = "00000000-0000-0000-0000-000000000231"
	const leadB = "00000000-0000-0000-0000-000000000232"
	const leadCreated = "00000000-0000-0000-0000-000000000235"
	const leadTenantB = "00000000-0000-0000-0000-000000000236"
	const attributionA = "00000000-0000-0000-0000-000000000233"
	const scoreA = "00000000-0000-0000-0000-000000000234"
	const lifecycleTransaction = "00000000-0000-0000-0000-000000000241"
	const updateTransaction = "00000000-0000-0000-0000-000000000242"
	const rejectTransaction = "00000000-0000-0000-0000-000000000243"
	const rollbackTransaction = "00000000-0000-0000-0000-000000000244"
	const lineLifecycle = "00000000-0000-0000-0000-000000000251"
	const lineUpdate = "00000000-0000-0000-0000-000000000252"
	const lineReject = "00000000-0000-0000-0000-000000000253"

	_, _ = pool.Exec(ctx, `DELETE FROM lead_scores WHERE id = $1::uuid`, scoreA)
	_, _ = pool.Exec(ctx, `DELETE FROM lead_attributions WHERE id = $1::uuid`, attributionA)
	_, _ = pool.Exec(ctx, `DELETE FROM order_lines WHERE id IN ($1::uuid, $2::uuid, $3::uuid)`, lineLifecycle, lineUpdate, lineReject)
	_, _ = pool.Exec(ctx, `DELETE FROM transaction_reviews WHERE transaction_id IN ($1::uuid, $2::uuid, $3::uuid, $4::uuid)`, lifecycleTransaction, updateTransaction, rejectTransaction, rollbackTransaction)
	_, _ = pool.Exec(ctx, `DELETE FROM transaction_confirmations WHERE transaction_id IN ($1::uuid, $2::uuid, $3::uuid, $4::uuid)`, lifecycleTransaction, updateTransaction, rejectTransaction, rollbackTransaction)
	_, _ = pool.Exec(ctx, `DELETE FROM commercial_transactions WHERE id IN ($1::uuid, $2::uuid, $3::uuid, $4::uuid)`, lifecycleTransaction, updateTransaction, rejectTransaction, rollbackTransaction)
	_, _ = pool.Exec(ctx, `DELETE FROM leads WHERE id IN ($1::uuid, $2::uuid, $3::uuid, $4::uuid)`, leadA, leadB, leadCreated, leadTenantB)
	_, _ = pool.Exec(ctx, `DELETE FROM offers WHERE id = $1::uuid`, offerA)
	_, _ = pool.Exec(ctx, `DELETE FROM variants WHERE id = $1::uuid`, variantA)
	_, _ = pool.Exec(ctx, `DELETE FROM catalog_items WHERE id = $1::uuid`, itemA)
	_, _ = pool.Exec(ctx, `DELETE FROM catalogs WHERE id = $1::uuid`, catalogA)
	_, _ = pool.Exec(ctx, `DELETE FROM customers WHERE id IN ($1::uuid, $2::uuid)`, customerA, customerB)
	_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessA, businessB)
	if _, err := pool.Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Sales A', 'sales-a', 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now()), ($2::uuid, 'Sales B', 'sales-b', 'active', 'travel', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, businessA, businessB); err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessA, businessB)
	if _, err := pool.Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, status, created_at, updated_at) VALUES ($1::uuid, $3::uuid, '{}'::jsonb, '[]'::jsonb, 'active', now(), now()), ($2::uuid, $4::uuid, '{}'::jsonb, '[]'::jsonb, 'active', now(), now())`, customerA, customerB, businessA, businessB); err != nil {
		t.Fatalf("insert customers: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM customers WHERE id IN ($1::uuid, $2::uuid)`, customerA, customerB)
	if _, err := pool.Exec(ctx, `INSERT INTO catalogs (id, business_id, name, description, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'Sales catalog', 'Sales fixtures', 'active', now(), now())`, catalogA, businessA); err != nil {
		t.Fatalf("insert catalog: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM catalogs WHERE id = $1::uuid`, catalogA)
	if _, err := pool.Exec(ctx, `INSERT INTO catalog_items (id, business_id, catalog_id, item_type, name, status, pricing_mode, availability_mode, fulfillment_mode, requires_confirmation, attributes, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'physical_good', 'Sales phone', 'active', 'fixed', 'stock', 'delivery', false, '{}'::jsonb, now(), now())`, itemA, businessA, catalogA); err != nil {
		t.Fatalf("insert catalog item: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM catalog_items WHERE id = $1::uuid`, itemA)
	if _, err := pool.Exec(ctx, `INSERT INTO variants (id, business_id, catalog_item_id, name, attributes, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'Black', '{"color":"black"}'::jsonb, 'active', now(), now())`, variantA, businessA, itemA); err != nil {
		t.Fatalf("insert variant: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM variants WHERE id = $1::uuid`, variantA)
	if _, err := pool.Exec(ctx, `INSERT INTO offers (id, business_id, catalog_item_id, variant_id, name, pricing_mode, amount, currency, price_source, price_verification_status, availability_mode, availability_status, availability_source, fulfillment_mode, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 'Sales offer', 'fixed', 1250.50, 'YER', 'merchant', 'verified', 'stock', 'available', 'merchant', 'delivery', 'active', now(), now())`, offerA, businessA, itemA, variantA); err != nil {
		t.Fatalf("insert offer: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM offers WHERE id = $1::uuid`, offerA)
	if _, err := pool.Exec(ctx, `INSERT INTO leads (id, business_id, customer_id, status, qualification_context, created_by, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'new', '{}'::jsonb, 'human', '2026-01-01T10:00:00Z', '2026-01-01T10:00:00Z'), ($4::uuid, $2::uuid, $3::uuid, 'new', '{}'::jsonb, 'human', '2026-01-01T10:01:00Z', '2026-01-01T10:01:00Z'), ($5::uuid, $6::uuid, $7::uuid, 'new', '{}'::jsonb, 'human', '2026-01-01T10:02:00Z', '2026-01-01T10:02:00Z')`, leadA, businessA, customerA, leadB, leadTenantB, businessB, customerB); err != nil {
		t.Fatalf("insert leads: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM leads WHERE id IN ($1::uuid, $2::uuid, $3::uuid, $4::uuid)`, leadA, leadB, leadCreated, leadTenantB)
	if _, err := pool.Exec(ctx, `INSERT INTO lead_attributions (id, business_id, lead_id, source_channel, source_interaction_reference, captured_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'whatsapp', 'wa-1', '2026-01-01T10:02:00Z')`, attributionA, businessA, leadA); err != nil {
		t.Fatalf("insert attribution: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM lead_attributions WHERE id = $1::uuid`, attributionA)
	if _, err := pool.Exec(ctx, `INSERT INTO lead_scores (id, business_id, lead_id, value, band, factors, rule_version, calculated_at, created_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 82.50, 'high', '{"intent":"strong"}'::jsonb, 'r1', '2026-01-01T10:03:00Z', '2026-01-01T10:03:00Z')`, scoreA, businessA, leadA); err != nil {
		t.Fatalf("insert score: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM lead_scores WHERE id = $1::uuid`, scoreA)

	leadRepo := NewLeadRepository(adapter)
	transactionRepo := NewTransactionRepository(adapter)
	leadServices := services.NewSalesCommandServices(leadRepo, transactionRepo, adapter)
	leadServices.Now = func() time.Time { return time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC) }
	leadIDs := []string{leadCreated}
	leadIndex := 0
	leadServices.NewID = func() string { id := leadIDs[leadIndex]; leadIndex++; return id }
	createdLead, err := (services.CreateLeadCommandService{SalesCommandServices: leadServices}).Handle(ctx, commands.CreateLeadCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA), Role: "human"}}, CustomerID: commands.CustomerID(customerA), QualificationContext: map[string]any{"source": "sales"}})
	if err != nil || createdLead.Lead.ResourceVersion != "1" || createdLead.Lead.Status != "new" {
		t.Fatalf("create lead: %#v err=%v", createdLead, err)
	}
	updatedContext := map[string]any{"source": "sales", "priority": "high"}
	updatedLead, err := (services.UpdateLeadCommandService{SalesCommandServices: leadServices}).Handle(ctx, commands.UpdateLeadCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}, ExpectedVersion: salesResourceVersion("1")}, LeadID: commands.LeadID(leadCreated), QualificationContext: updatedContext})
	if err != nil || updatedLead.Lead.ResourceVersion != "2" {
		t.Fatalf("update lead: %#v err=%v", updatedLead, err)
	}
	qualifiedLead, err := (services.QualifyLeadCommandService{SalesCommandServices: leadServices}).Handle(ctx, commands.QualifyLeadCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA), Role: "human"}, ExpectedVersion: salesResourceVersion("2")}, LeadID: commands.LeadID(leadCreated), Reason: "customer intent confirmed", EvidenceReferences: []string{"message:wa-1"}})
	if err != nil || qualifiedLead.Lead.ResourceVersion != "3" || qualifiedLead.Lead.Status != "qualified" {
		t.Fatalf("qualify lead: %#v err=%v", qualifiedLead, err)
	}
	lostLead, err := (services.MarkLeadLostCommandService{SalesCommandServices: leadServices}).Handle(ctx, commands.MarkLeadLostCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}, ExpectedVersion: salesResourceVersion("3")}, LeadID: commands.LeadID(leadCreated), LostReason: "customer deferred"})
	if err != nil || lostLead.Lead.ResourceVersion != "4" || lostLead.Lead.Status != "lost" {
		t.Fatalf("lost lead: %#v err=%v", lostLead, err)
	}
	if _, err := leadRepo.Get(ctx, businessB, leadA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("lead tenant leak: %v", err)
	}
	leadPage, err := leadRepo.List(ctx, businessA, "", "", "", 1, "")
	if err != nil || len(leadPage.Items) != 1 || !leadPage.HasMore || leadPage.NextCursor == "" {
		t.Fatalf("lead pagination: %#v err=%v", leadPage, err)
	}
	if _, err := leadRepo.List(ctx, businessA, "", "", "", 1, "not-a-cursor"); !IsRepositoryKind(err, RepositoryInvalid) {
		t.Fatalf("lead malformed cursor: %v", err)
	}
	attributionPage, err := leadRepo.ListAttributions(ctx, businessA, leadA, 10, "")
	if err != nil || len(attributionPage.Items) != 1 || salesValueString(attributionPage.Items[0].SourceChannel) != "whatsapp" {
		t.Fatalf("attribution list: %#v err=%v", attributionPage, err)
	}
	scorePage, err := leadRepo.ListScores(ctx, businessA, leadA, 10, "")
	if err != nil || len(scorePage.Items) != 1 || scorePage.Items[0].Band != "high" {
		t.Fatalf("score list: %#v err=%v", scorePage, err)
	}
	leadQueryService := services.ListLeadsQueryService{Repository: leadRepo}
	mappedLeads, err := leadQueryService.Handle(ctx, queries.ListLeadsQuery{Meta: queries.QueryMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}}, Limit: 10, Status: "lost"})
	if err != nil || len(mappedLeads.Items) != 1 || mappedLeads.Items[0].Status != "lost" {
		t.Fatalf("lead application mapping: %#v err=%v", mappedLeads, err)
	}

	transactionTypes := []string{"order", "booking", "appointment", "service_request", "reservation", "quote", "subscription"}
	transactionIDs := []string{lifecycleTransaction, "00000000-0000-0000-0000-000000000245", "00000000-0000-0000-0000-000000000246", "00000000-0000-0000-0000-000000000247", "00000000-0000-0000-0000-000000000248", updateTransaction, rejectTransaction}
	lineIDs := []string{lineLifecycle, "00000000-0000-0000-0000-000000000255", "00000000-0000-0000-0000-000000000256", "00000000-0000-0000-0000-000000000257", "00000000-0000-0000-0000-000000000258", lineUpdate, lineReject}
	for index, transactionType := range transactionTypes {
		if err := adapter.Within(ctx, func(txCtx context.Context) error {
			_, err := transactionRepo.CreateDraft(txCtx, ports.TransactionDraft{ID: transactionIDs[index], BusinessID: businessA, CustomerID: customerA, LeadID: stringPointerForSales(leadA), TransactionType: transactionType, Currency: stringPointerForSales("YER"), SchemaVersion: 1, CreatedAt: time.Date(2026, 1, 1, 12+index, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 1, 1, 12+index, 0, 0, 0, time.UTC), Lines: []ports.TransactionLineDraft{{ID: lineIDs[index], CatalogItemID: itemA, OfferID: stringPointerForSales(offerA), VariantID: stringPointerForSales(variantA), Quantity: "2", SelectedAttributes: []byte(`{"color":"black"}`)}}})
			return err
		}); err != nil {
			t.Fatalf("create universal transaction %s: %v", transactionType, err)
		}
	}
	if _, err := transactionRepo.Get(ctx, businessB, lifecycleTransaction); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("transaction tenant leak: %v", err)
	}
	transactionPage, err := transactionRepo.List(ctx, businessA, "", "", "", 3, "")
	if err != nil || len(transactionPage.Items) != 3 || !transactionPage.HasMore || transactionPage.NextCursor == "" {
		t.Fatalf("transaction pagination: %#v err=%v", transactionPage, err)
	}
	if _, err := transactionRepo.List(ctx, businessA, "", "", "", 3, "invalid"); !IsRepositoryKind(err, RepositoryInvalid) {
		t.Fatalf("transaction malformed cursor: %v", err)
	}
	var snapshot struct {
		ItemName     string
		Pricing      []byte
		Availability []byte
		Fulfillment  []byte
		UnitPrice    string
		LineTotal    string
		Currency     string
	}
	if err := pool.QueryRow(ctx, `SELECT item_name_snapshot, pricing_snapshot, availability_snapshot, fulfillment_snapshot, unit_price_snapshot::text, line_total_snapshot::text, currency FROM order_lines WHERE business_id = $1::uuid AND transaction_id = $2::uuid`, businessA, lifecycleTransaction).Scan(&snapshot.ItemName, &snapshot.Pricing, &snapshot.Availability, &snapshot.Fulfillment, &snapshot.UnitPrice, &snapshot.LineTotal, &snapshot.Currency); err != nil {
		t.Fatalf("snapshot read: %v", err)
	}
	if snapshot.ItemName != "Sales phone" || string(snapshot.Pricing) == "{}" || snapshot.UnitPrice != "1250.5000" || snapshot.LineTotal != "2501.0000" || snapshot.Currency != "YER" {
		t.Fatalf("snapshot mismatch: %#v", snapshot)
	}

	lifecycleRecord, err := transactionRepo.Get(ctx, businessA, lifecycleTransaction)
	if err != nil || lifecycleRecord.ResourceVersion != 1 || lifecycleRecord.State != "draft" {
		t.Fatalf("get lifecycle transaction: %#v err=%v", lifecycleRecord, err)
	}
	updatedRecord, err := transactionRepo.UpdateDraft(ctx, ports.TransactionPatch{ID: updateTransaction, BusinessID: businessA, Currency: stringPointerForSales("YER"), ExpectedVersion: 1, UpdatedAt: time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC), Lines: []ports.TransactionLineDraft{{ID: lineUpdate, CatalogItemID: itemA, OfferID: stringPointerForSales(offerA), VariantID: stringPointerForSales(variantA), Quantity: "3", SelectedAttributes: []byte(`{"color":"black"}`)}}})
	if err != nil || updatedRecord.ResourceVersion != 2 {
		t.Fatalf("update transaction: %#v err=%v", updatedRecord, err)
	}
	if _, err := transactionRepo.UpdateDraft(ctx, ports.TransactionPatch{ID: updateTransaction, BusinessID: businessA, ExpectedVersion: 1, UpdatedAt: time.Now().UTC(), Lines: []ports.TransactionLineDraft{{ID: lineUpdate, CatalogItemID: itemA, Quantity: "1"}}}); !IsRepositoryKind(err, RepositoryStale) {
		t.Fatalf("expected stale transaction update, got %v", err)
	}
	confirmedRecord, err := transactionRepo.Confirm(ctx, ports.TransactionConfirmPatch{ID: lifecycleTransaction, BusinessID: businessA, EvidenceReference: "customer-confirmation-1", PolicyVersion: "sales-v1", ExpectedVersion: 1, UpdatedAt: time.Date(2026, 1, 2, 11, 0, 0, 0, time.UTC)})
	if err != nil || confirmedRecord.State != "confirmed" || confirmedRecord.ResourceVersion != 2 {
		t.Fatalf("confirm transaction: %#v err=%v", confirmedRecord, err)
	}
	review, err := transactionRepo.SubmitReview(ctx, ports.TransactionReviewPatch{ID: lifecycleTransaction, BusinessID: businessA, ReasonCodes: []byte(`["high_value"]`), ExpectedVersion: 2, UpdatedAt: time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)})
	if err != nil || review.Status != "pending" || review.DecisionReason != nil {
		t.Fatalf("submit review: %#v err=%v", review, err)
	}
	approved, err := transactionRepo.DecideReview(ctx, ports.TransactionReviewDecisionPatch{ID: lifecycleTransaction, BusinessID: businessA, ReviewerReference: "agent-1", Reason: "evidence verified", Approved: true, ExpectedVersion: 3, UpdatedAt: time.Date(2026, 1, 2, 13, 0, 0, 0, time.UTC)})
	if err != nil || approved.Status != "approved" || salesValueString(approved.DecisionReason) != "evidence verified" {
		t.Fatalf("approve review: %#v err=%v", approved, err)
	}
	cancelled, err := transactionRepo.Cancel(ctx, ports.TransactionCancelPatch{ID: lifecycleTransaction, BusinessID: businessA, Reason: "customer requested cancellation", ExpectedVersion: 4, UpdatedAt: time.Date(2026, 1, 2, 14, 0, 0, 0, time.UTC)})
	if err != nil || cancelled.State != "cancelled" || cancelled.ResourceVersion != 5 {
		t.Fatalf("cancel transaction: %#v err=%v", cancelled, err)
	}
	if _, err := transactionRepo.Cancel(ctx, ports.TransactionCancelPatch{ID: lifecycleTransaction, BusinessID: businessA, Reason: "stale attempt", ExpectedVersion: 1, UpdatedAt: time.Now().UTC()}); !IsRepositoryKind(err, RepositoryStale) {
		t.Fatalf("expected stale cancel, got %v", err)
	}
	pendingReview, err := transactionRepo.SubmitReview(ctx, ports.TransactionReviewPatch{ID: rejectTransaction, BusinessID: businessA, ReasonCodes: []byte(`["manual_check"]`), ExpectedVersion: 1, UpdatedAt: time.Date(2026, 1, 2, 15, 0, 0, 0, time.UTC)})
	if err != nil || pendingReview.Status != "pending" {
		t.Fatalf("submit reject review: %#v err=%v", pendingReview, err)
	}
	rejected, err := transactionRepo.DecideReview(ctx, ports.TransactionReviewDecisionPatch{ID: rejectTransaction, BusinessID: businessA, ReviewerReference: "agent-2", Reason: "insufficient evidence", Approved: false, ExpectedVersion: 2, UpdatedAt: time.Date(2026, 1, 2, 16, 0, 0, 0, time.UTC)})
	if err != nil || rejected.Status != "rejected" {
		t.Fatalf("reject review: %#v err=%v", rejected, err)
	}
	transactionQueryService := services.ListTransactionsQueryService{Repository: transactionRepo}
	mappedTransactions, err := transactionQueryService.Handle(ctx, queries.ListTransactionsQuery{Meta: queries.QueryMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(businessA)}}, Limit: 20, TransactionType: "subscription"})
	if err != nil || len(mappedTransactions.Items) != 1 || mappedTransactions.Items[0].TransactionType != "subscription" {
		t.Fatalf("transaction application mapping: %#v err=%v", mappedTransactions, err)
	}

	rollbackErr := errors.New("sales transaction rollback")
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		_, err := transactionRepo.CreateDraft(txCtx, ports.TransactionDraft{ID: rollbackTransaction, BusinessID: businessA, CustomerID: customerA, TransactionType: "quote", Currency: stringPointerForSales("YER"), SchemaVersion: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Lines: []ports.TransactionLineDraft{{ID: "00000000-0000-0000-0000-000000000260", CatalogItemID: itemA, OfferID: stringPointerForSales(offerA), Quantity: "1", SelectedAttributes: []byte(`{}`)}}})
		if err != nil {
			return err
		}
		return rollbackErr
	}); !errors.Is(err, rollbackErr) {
		t.Fatalf("expected sales rollback error, got %v", err)
	}
	if _, err := transactionRepo.Get(ctx, businessA, rollbackTransaction); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("transaction rollback leaked: %v", err)
	}
}

func salesResourceVersion(value string) *commands.ResourceVersion {
	version := commands.ResourceVersion(value)
	return &version
}
func stringPointerForSales(value string) *string { return &value }
func salesValueString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
