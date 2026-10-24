package handlers

import (
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"testing"
	"time"
)

func TestTransactionProjectionPreservesStoredReceipt(t *testing.T) {
	total, price, currency, offer, variant := "10000.0000", "5000.0000", "YER", "offer", "variant"
	lead := commands.LeadID("lead")
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	v := commands.TransactionView{ID: "transaction", BusinessID: "business", CustomerID: "customer", LeadID: &lead, State: "awaiting_confirmation", TransactionType: "order", ResourceVersion: "7", Currency: &currency, TotalAmount: &total, SchemaVersion: 3, CreatedAt: now, UpdatedAt: now, Lines: []commands.TransactionLineSnapshotView{{ID: "line", CatalogItemID: "item", OfferID: &offer, VariantID: &variant, ItemNameSnapshot: "سماعة", Quantity: "2.0000", UnitPriceSnapshot: &price, Currency: &currency}}}
	receipt := transactionProjection(v)
	if receipt.TotalAmount == nil || *receipt.TotalAmount != 10000 || receipt.Currency == nil || *receipt.Currency != "YER" || receipt.SchemaVersion != 3 || receipt.CreatedAt != now || receipt.LeadID == nil || len(receipt.Lines) != 1 {
		t.Fatalf("receipt was lost: %+v", receipt)
	}
	line := receipt.Lines[0]
	if line.ItemNameSnapshot != "سماعة" || line.Quantity != 2 || line.UnitPriceSnapshot == nil || *line.UnitPriceSnapshot != 5000 || line.OfferID == nil || line.VariantID == nil {
		t.Fatalf("snapshot was lost: %+v", line)
	}
	v.TotalAmount = nil
	v.Lines[0].UnitPriceSnapshot = nil
	receipt = transactionProjection(v)
	if receipt.TotalAmount != nil || receipt.Lines[0].UnitPriceSnapshot != nil {
		t.Fatal("unknown price was fabricated as zero")
	}
}
