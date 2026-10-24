package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/catalogvalidation"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

func validateSalesSelection(ctx context.Context, e SQLExecutor, business string, line ports.TransactionLineDraft, now time.Time) error {
	var valid bool
	err := e.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_items i JOIN catalogs c ON c.business_id=i.business_id AND c.id=i.catalog_id AND c.status='active'
 WHERE i.business_id=$1::uuid AND i.id=$2::uuid AND i.status='active'
 AND ($3::uuid IS NULL OR EXISTS(SELECT 1 FROM variants v WHERE v.business_id=i.business_id AND v.catalog_item_id=i.id AND v.id=$3::uuid AND v.status='active'))
 AND (($4::uuid IS NULL AND NOT EXISTS(SELECT 1 FROM offers o WHERE o.business_id=i.business_id AND o.catalog_item_id=i.id AND o.status='active'))
 OR EXISTS(SELECT 1 FROM offers o WHERE o.business_id=i.business_id AND o.catalog_item_id=i.id AND o.id=$4::uuid AND o.status='active'
 AND (o.variant_id IS NULL OR o.variant_id=$3::uuid) AND (o.validity_from IS NULL OR o.validity_from<=$5) AND (o.validity_until IS NULL OR o.validity_until>$5))))`, business, line.CatalogItemID, line.VariantID, line.OfferID, now).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return invalidRepositoryInput("transaction.selection", "select a current item, variant and explicit compatible offer")
	}
	// Validate selected overrides with the same typed schema used by Catalog.
	var schemaID *string
	if err := e.QueryRow(ctx, `SELECT attribute_schema_id::text FROM catalog_items WHERE business_id=$1::uuid AND id=$2::uuid`, business, line.CatalogItemID).Scan(&schemaID); err != nil {
		return err
	}
	if schemaID != nil {
		var definitions []ports.AttributeDefinitionRecord
		rows, err := e.Query(ctx, `SELECT attribute_key,label,data_type,is_required,validation_rules FROM attribute_definitions d JOIN attribute_schemas s ON s.id=d.schema_id WHERE s.business_id=$1::uuid AND d.schema_id=$2::uuid`, business, *schemaID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var d ports.AttributeDefinitionRecord
			if err := rows.Scan(&d.Key, &d.Label, &d.DataType, &d.Required, &d.ValidationRules); err != nil {
				rows.Close()
				return err
			}
			definitions = append(definitions, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		var attrs map[string]any
		if len(line.SelectedAttributes) > 0 {
			if err := json.Unmarshal(line.SelectedAttributes, &attrs); err != nil {
				return err
			}
		}
		if err := catalogvalidation.ValidatePartialAttributes(&ports.AttributeSchemaRecord{Definitions: definitions}, attrs); err != nil {
			return invalidRepositoryInput("transaction.attributes", err.Error())
		}
	}
	return nil
}

func (r *TransactionRepository) refreshTotals(ctx context.Context, e SQLExecutor, record ports.TransactionRecord) (ports.TransactionRecord, error) {
	var currencies int
	var currency *string
	var total *string
	if err := e.QueryRow(ctx, `SELECT COUNT(DISTINCT currency),MIN(currency),CASE WHEN COUNT(*)=COUNT(line_total_snapshot) THEN SUM(line_total_snapshot)::text END FROM order_lines WHERE business_id=$1::uuid AND transaction_id=$2::uuid`, record.BusinessID, record.ID).Scan(&currencies, &currency, &total); err != nil {
		return record, err
	}
	if currencies > 1 || (currency != nil && record.Currency != nil && *currency != *record.Currency) {
		return record, invalidRepositoryInput("transaction.currency", "transaction lines must use one matching currency")
	}
	_, err := e.Exec(ctx, `UPDATE commercial_transactions SET currency=COALESCE($3,currency),total_amount=$4::numeric WHERE business_id=$1::uuid AND id=$2::uuid`, record.BusinessID, record.ID, currency, total)
	if err != nil {
		return record, err
	}
	return r.Get(ctx, record.BusinessID, record.ID)
}

func (r *TransactionRepository) validateConfirmation(ctx context.Context, e SQLExecutor, p ports.TransactionConfirmPatch) error {
	var version, agreement int64
	var state string
	var review bool
	var customer string
	var transactionType string
	var schemaVersion int
	var transactionData []byte
	var leadID *string
	if err := e.QueryRow(ctx, `SELECT resource_version,agreement_version,state,requires_human_review,customer_id::text,transaction_type,schema_version,transaction_data,lead_id::text FROM commercial_transactions WHERE business_id=$1::uuid AND id=$2::uuid FOR UPDATE`, p.BusinessID, p.ID).Scan(&version, &agreement, &state, &review, &customer, &transactionType, &schemaVersion, &transactionData, &leadID); err != nil {
		return classifyRepositoryGetError("transaction.confirm", err)
	}
	if version != p.ExpectedVersion {
		return &RepositoryError{Operation: "transaction.confirm", Kind: RepositoryStale, Err: fmt.Errorf("stale transaction version")}
	}
	if state != "draft" && state != "awaiting_confirmation" {
		return invalidRepositoryInput("transaction.confirm", "transaction is not confirmable")
	}
	if review {
		var approved bool
		if err := e.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM transaction_reviews WHERE business_id=$1::uuid AND transaction_id=$2::uuid AND required AND status='approved')`, p.BusinessID, p.ID).Scan(&approved); err != nil {
			return err
		}
		if !approved {
			return invalidRepositoryInput("transaction.confirm", "required human review is not approved")
		}
	}
	// An accepted button is mandatory for workflow-backed transactions, including manual confirmation.
	var workflowCount int
	if err := e.QueryRow(ctx, `SELECT COUNT(*) FROM sales_workflows WHERE business_id=$1::uuid AND transaction_id=$2::uuid`, p.BusinessID, p.ID).Scan(&workflowCount); err != nil {
		return err
	}
	if workflowCount > 0 {
		var raw []byte
		err := e.QueryRow(ctx, `SELECT definitions FROM sales_transaction_requirements WHERE business_id=$1::uuid AND transaction_type=$2 AND schema_version=$3`, p.BusinessID, transactionType, schemaVersion).Scan(&raw)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if errors.Is(err, pgx.ErrNoRows) && transactionType != "order" {
			return invalidRepositoryInput("transaction.requirements", "specialized requirement schema is missing")
		}
		var definitions []ports.SalesRequirementDefinition
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &definitions); err != nil {
				return err
			}
		}
		var data map[string]any
		if err := json.Unmarshal(transactionData, &data); err != nil {
			return err
		}
		if err := catalogvalidation.ValidateAttributes(salesRequirementSchema(definitions), data); err != nil {
			return invalidRepositoryInput("transaction.requirements", err.Error())
		}
		var accepted bool
		if err := e.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sales_confirmation_requests WHERE business_id=$1::uuid AND transaction_id=$2::uuid AND customer_id=$3::uuid AND transaction_version=$4 AND status='accepted' AND expires_at>$5)`, p.BusinessID, p.ID, customer, agreement, p.UpdatedAt).Scan(&accepted); err != nil {
			return err
		}
		if !accepted {
			return invalidRepositoryInput("transaction.confirm", "current customer confirmation is missing or expired")
		}
	}
	if leadID != nil {
		var leadCustomer, leadState string
		if err := e.QueryRow(ctx, `SELECT customer_id::text,status FROM leads WHERE business_id=$1::uuid AND id=$2::uuid FOR UPDATE`, p.BusinessID, *leadID).Scan(&leadCustomer, &leadState); err != nil {
			return err
		}
		if leadCustomer != customer || leadState == "lost" {
			return invalidRepositoryInput("transaction.lead", "lead customer changed or lead was closed as lost")
		}
	}
	// Lock every mutable catalog record used by the agreement, not just its price.
	for _, query := range []string{
		`SELECT c.id FROM catalogs c JOIN catalog_items i ON i.business_id=c.business_id AND i.catalog_id=c.id JOIN order_lines l ON l.business_id=i.business_id AND l.catalog_item_id=i.id WHERE l.business_id=$1::uuid AND l.transaction_id=$2::uuid ORDER BY c.id FOR SHARE OF c`,
		`SELECT i.id FROM catalog_items i JOIN order_lines l ON l.business_id=i.business_id AND l.catalog_item_id=i.id WHERE l.business_id=$1::uuid AND l.transaction_id=$2::uuid ORDER BY i.id FOR SHARE OF i`,
		`SELECT v.id FROM variants v JOIN order_lines l ON l.business_id=v.business_id AND l.variant_id=v.id WHERE l.business_id=$1::uuid AND l.transaction_id=$2::uuid ORDER BY v.id FOR SHARE OF v`,
	} {
		rows, err := e.Query(ctx, query, p.BusinessID, p.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	// Share-lock current catalog evidence until commit so an update cannot race confirmation.
	rows, err := e.Query(ctx, `SELECT o.id FROM offers o JOIN order_lines l ON l.business_id=o.business_id AND l.offer_id=o.id WHERE l.business_id=$1::uuid AND l.transaction_id=$2::uuid ORDER BY o.id FOR SHARE OF o`, p.BusinessID, p.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var invalid int
	var count int
	// A current offer must still match the exact agreement. Unknown price/availability never confirms.
	if err := e.QueryRow(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE o.id IS NULL OR i.status<>'active' OR c.status<>'active' OR o.status<>'active'
 OR o.price_verification_status<>'verified' OR o.pricing_mode NOT IN ('fixed','per_unit','per_person','per_day') OR o.amount IS NULL OR o.currency IS NULL
 OR o.availability_status<>'available' OR o.availability_mode='unknown'
 OR (o.validity_from IS NOT NULL AND o.validity_from>$3) OR (o.validity_until IS NOT NULL AND o.validity_until<=$3)
 OR (o.availability_valid_until IS NOT NULL AND o.availability_valid_until<=$3)
 OR l.pricing_snapshot->>'resource_version' IS DISTINCT FROM o.resource_version::text
 OR l.unit_price_snapshot IS DISTINCT FROM o.amount OR l.currency IS DISTINCT FROM o.currency
 OR (v.id IS NOT NULL AND v.status<>'active') OR (o.variant_id IS NOT NULL AND o.variant_id IS DISTINCT FROM l.variant_id))
 FROM order_lines l JOIN catalog_items i ON i.business_id=l.business_id AND i.id=l.catalog_item_id JOIN catalogs c ON c.business_id=i.business_id AND c.id=i.catalog_id
 LEFT JOIN offers o ON o.business_id=l.business_id AND o.id=l.offer_id LEFT JOIN variants v ON v.business_id=l.business_id AND v.id=l.variant_id
 WHERE l.business_id=$1::uuid AND l.transaction_id=$2::uuid`, p.BusinessID, p.ID, p.UpdatedAt).Scan(&count, &invalid); err != nil {
		return err
	}
	if count == 0 || invalid > 0 {
		return invalidRepositoryInput("transaction.confirm", "price or availability evidence changed, is missing or is not current")
	}
	if strings.TrimSpace(p.EvidenceReference) == "" {
		return invalidRepositoryInput("transaction.confirm", "confirmation evidence required")
	}
	return nil
}

func salesRequirementSchema(definitions []ports.SalesRequirementDefinition) *ports.AttributeSchemaRecord {
	s := &ports.AttributeSchemaRecord{}
	for _, d := range definitions {
		s.Definitions = append(s.Definitions, ports.AttributeDefinitionRecord{Key: d.Key, Label: d.Label, DataType: d.DataType, Required: d.Required, ValidationRules: mustSalesJSON(d.Rules)})
	}
	return s
}

// Manual commands and customer/model commands take the same locks in the same
// order. Otherwise an API transaction can hold the agreement while a button
// holds its workflow, each waiting for the other.
func lockTransactionWorkflow(ctx context.Context, e SQLExecutor, business, transaction string) error {
	for _, query := range []string{
		`SELECT c.id FROM conversations c JOIN sales_workflows w ON w.business_id=c.business_id AND w.conversation_id=c.id WHERE w.business_id=$1::uuid AND w.transaction_id=$2::uuid ORDER BY c.id FOR UPDATE OF c`,
		`SELECT id FROM sales_workflows WHERE business_id=$1::uuid AND transaction_id=$2::uuid ORDER BY id FOR UPDATE`,
	} {
		rows, err := e.Query(ctx, query, business, transaction)
		if err != nil {
			return err
		}
		for rows.Next() {
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}
