//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

func TestKnowledgeAndBusinessPolicyRepositoriesAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()

	businessA := uuid.NewString()
	businessB := uuid.NewString()
	knowledgeCurrent := uuid.NewString()
	knowledgeExpired := uuid.NewString()
	knowledgeDraft := uuid.NewString()
	knowledgeOther := uuid.NewString()
	policyCurrent := uuid.NewString()
	policyFuture := uuid.NewString()
	policyOther := uuid.NewString()
	now := time.Date(2026, 8, 25, 21, 0, 0, 0, time.UTC)
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)
	defer func() {
		cleanupCtx := context.Background()
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM business_policy_versions WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM knowledge_documents WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessA, businessB)
	}()

	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Policy Tenant A', $1, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $3, $3), ($2::uuid, 'Policy Tenant B', $2, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $3, $3)`, businessA, businessB, now); err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO knowledge_documents (id, business_id, knowledge_key, title, content, content_type, source_reference, authority, status, version, valid_from, valid_until, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'opening-hours', 'Opening hours', 'We open Friday 09:00 to 17:00', 'hours', 'merchant-doc-hours', 'merchant', 'published', 1, $3, $4, $3, $3), ($5::uuid, $2::uuid, 'returns', 'Returns', 'Returns within seven days', 'faq', 'merchant-doc-returns', 'merchant', 'published', 1, $6, $7, $6, $6), ($8::uuid, $2::uuid, 'draft-note', 'Draft note', 'Not published', 'general', 'merchant-draft', 'merchant', 'draft', 1, $3, NULL, $3, $3), ($9::uuid, $10::uuid, 'opening-hours', 'Other hours', 'Other tenant hours', 'hours', 'other-merchant-doc', 'merchant', 'published', 1, $3, NULL, $3, $3)`, knowledgeCurrent, businessA, past, future, knowledgeExpired, past.Add(-48*time.Hour), past.Add(-24*time.Hour), knowledgeDraft, knowledgeOther, businessB); err != nil {
		t.Fatalf("insert knowledge documents: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO business_policy_versions (id, business_id, policy_key, category, title, summary, rules, authority, status, version, valid_from, valid_until, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'opening-hours', 'hours', 'Hours policy', 'Merchant hours are authoritative', '{"friday":"09:00-17:00"}'::jsonb, 'merchant', 'published', 1, $3, NULL, $3, $3), ($4::uuid, $2::uuid, 'future-policy', 'general', 'Future policy', 'Not active yet', '{}'::jsonb, 'merchant', 'published', 1, $5, NULL, $5, $5), ($6::uuid, $7::uuid, 'opening-hours', 'hours', 'Other policy', 'Other tenant policy', '{"friday":"closed"}'::jsonb, 'merchant', 'published', 1, $3, NULL, $3, $3)`, policyCurrent, businessA, past, policyFuture, future, policyOther, businessB); err != nil {
		t.Fatalf("insert business policies: %v", err)
	}

	knowledgeRepo := NewKnowledgeDocumentRepository(adapter)
	knowledge, err := knowledgeRepo.ListPublished(ctx, businessA, "", now, 20)
	if err != nil {
		t.Fatalf("list knowledge: %v", err)
	}
	if len(knowledge) != 2 || knowledge[0].ID != knowledgeCurrent || knowledge[1].ID != knowledgeExpired || knowledge[0].BusinessID != businessA || knowledge[1].ValidUntil == nil {
		t.Fatalf("knowledge validity/tenant/stale candidate mismatch: %#v", knowledge)
	}
	knowledgeSearch, err := knowledgeRepo.ListPublished(ctx, businessA, "opening", now, 20)
	if err != nil || len(knowledgeSearch) != 1 || knowledgeSearch[0].ID != knowledgeCurrent {
		t.Fatalf("knowledge search mismatch: records=%#v err=%v", knowledgeSearch, err)
	}
	knowledgeEmpty, err := knowledgeRepo.ListPublished(ctx, businessA, "does-not-exist", now, 20)
	if err != nil || len(knowledgeEmpty) != 0 {
		t.Fatalf("knowledge missing search mismatch: records=%#v err=%v", knowledgeEmpty, err)
	}

	policyRepo := NewBusinessPolicyRepository(adapter)
	policies, err := policyRepo.ListPublished(ctx, businessA, "", now, 20)
	if err != nil {
		t.Fatalf("list policies: %v", err)
	}
	if len(policies) != 1 || policies[0].ID != policyCurrent || policies[0].BusinessID != businessA || !json.Valid(policies[0].Rules) {
		t.Fatalf("policy validity/tenant/rules mismatch: %#v", policies)
	}
	policySearch, err := policyRepo.ListPublished(ctx, businessA, "hours", now, 20)
	if err != nil || len(policySearch) != 1 || policySearch[0].ID != policyCurrent {
		t.Fatalf("policy search mismatch: records=%#v err=%v", policySearch, err)
	}

	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO business_policy_versions (id, business_id, policy_key, category, title, summary, rules, authority, status, version, valid_from, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'opening-hours', 'hours', 'Duplicate', 'Duplicate published key', '{}'::jsonb, 'merchant', 'published', 2, $3, $3, $3)`, uuid.NewString(), businessA, past); err == nil {
		t.Fatal("duplicate published policy key was accepted")
	} else if errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected context cancellation: %v", err)
	}

	var count int
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM business_policy_versions WHERE business_id = $1::uuid`, businessB).Scan(&count); err != nil {
		t.Fatalf("count other tenant policies: %v", err)
	}
	if count != 1 {
		t.Fatalf("unexpected other tenant policy count: %d", count)
	}

	var _ ports.KnowledgeDocumentRepository = knowledgeRepo
	var _ ports.BusinessPolicyRepository = policyRepo
}
