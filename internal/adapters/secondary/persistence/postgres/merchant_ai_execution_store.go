package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

type MerchantAIExecutionStore struct{ Adapter *Adapter }

func (s MerchantAIExecutionStore) Save(ctx context.Context, p merchantcatalogai.StoredProposal) error {
	exec, e := s.Adapter.Executor(ctx)
	if e != nil {
		return e
	}
	raw, e := json.Marshal(p)
	if e != nil {
		return e
	}
	tag, e := exec.Exec(ctx, `INSERT INTO merchant_ai_proposals(business_id,id,principal_id,session_id,catalog_id,payload) SELECT $1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6::jsonb FROM merchant_ai_sessions WHERE business_id=$1::uuid AND id=$4::uuid AND principal_id=$3::uuid`, p.BusinessID, p.ID, p.PrincipalID, p.SessionID, p.CatalogID, raw)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("merchant proposal session ownership mismatch")
	}
	return nil
}
func (s MerchantAIExecutionStore) Lock(ctx context.Context, businessID, principalID, id string) (merchantcatalogai.StoredProposal, error) {
	var p merchantcatalogai.StoredProposal
	exec, e := s.Adapter.Executor(ctx)
	if e != nil {
		return p, e
	}
	var raw, result []byte
	e = exec.QueryRow(ctx, `SELECT payload,result FROM merchant_ai_proposals WHERE business_id=$1::uuid AND principal_id=$2::uuid AND id=$3::uuid FOR UPDATE`, businessID, principalID, id).Scan(&raw, &result)
	if e != nil {
		return p, catalogRepositoryError("merchant_ai.proposal.lock", e)
	}
	if e = json.Unmarshal(raw, &p); e != nil {
		return p, e
	}
	// Lock the existing target and its children before checking snapshot versions.
	// Concurrent ordinary catalog edits then serialize with this approval.
	if result == nil {
		itemID := ""
		if p.Proposal.Update != nil {
			itemID = p.Proposal.Update.ItemID
		}
		if p.Proposal.Delete != nil {
			itemID = p.Proposal.Delete.ItemID
		}
		if itemID != "" {
			var lockedID string
			if e = exec.QueryRow(ctx, `SELECT id::text FROM catalog_items WHERE business_id=$1::uuid AND catalog_id=$2::uuid AND id=$3::uuid FOR UPDATE`, businessID, p.CatalogID, itemID).Scan(&lockedID); e != nil {
				return p, e
			}
			for _, query := range []string{`SELECT id FROM offers WHERE business_id=$1::uuid AND catalog_item_id=$2::uuid ORDER BY id FOR UPDATE`, `SELECT id FROM variants WHERE business_id=$1::uuid AND catalog_item_id=$2::uuid ORDER BY id FOR UPDATE`} {
				rows, err := exec.Query(ctx, query, businessID, itemID)
				if err != nil {
					return p, err
				}
				for rows.Next() {
				}
				err = rows.Err()
				rows.Close()
				if err != nil {
					return p, err
				}
			}
		}
	}
	if result != nil {
		p.Result = &merchantcatalogai.ExecutionResult{}
		e = json.Unmarshal(result, p.Result)
	}
	return p, e
}
func (s MerchantAIExecutionStore) Complete(ctx context.Context, businessID, id string, result merchantcatalogai.ExecutionResult) error {
	exec, e := s.Adapter.Executor(ctx)
	if e != nil {
		return e
	}
	raw, e := json.Marshal(result)
	if e != nil {
		return e
	}
	tag, e := exec.Exec(ctx, `UPDATE merchant_ai_proposals SET result=$3::jsonb,executed_at=now() WHERE business_id=$1::uuid AND id=$2::uuid AND result IS NULL`, businessID, id, raw)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("proposal completion conflict")
	}
	return nil
}
