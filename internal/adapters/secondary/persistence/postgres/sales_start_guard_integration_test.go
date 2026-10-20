//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestSalesWorkflowRepositoryRejectsUnconsentedAIExecutionAgainstPostgres(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	repo := NewSalesWorkflowRepository(f.a)
	err := f.a.Within(context.Background(), func(tx context.Context) error {
		_, err := repo.Apply(tx, ports.SalesWorkflowInput{
			BusinessID: f.business, CustomerID: f.customer,
			ConversationID: f.conversation, SourceMessageID: f.message,
			IdempotencyKey: "no-consent", Capability: ports.CustomerSalesCapabilityRecordLead,
			Proposal: ports.SalesWorkflowProposal{EvidenceMessageIDs: []string{f.message}},
			Now:      f.now,
		})
		return err
	})
	if err == nil {
		t.Fatal("unconsented automated lead was created")
	}
	var workflows, leads int
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM sales_workflows WHERE business_id=$1::uuid`, f.business).Scan(&workflows); err != nil {
		t.Fatal(err)
	}
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM leads WHERE business_id=$1::uuid`, f.business).Scan(&leads); err != nil {
		t.Fatal(err)
	}
	if workflows != 0 || leads != 0 {
		t.Fatalf("unconsented effect persisted: workflows=%d leads=%d", workflows, leads)
	}
}
