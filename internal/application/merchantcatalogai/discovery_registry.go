package merchantcatalogai

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type ReadOnlyCapabilityRegistry struct {
	repository                ports.CatalogRepository
	capabilities              map[string]MerchantCatalogDiscoveryTool
	selectedCatalogID         string
	evidenceReferences        map[string]struct{}
	attributeSchemaReferences map[string]struct{}
	issuedCursors             map[string]string // Latest trusted continuation per read scope.
}

func NewReadOnlyCapabilityRegistry(repository ports.CatalogRepository, selectedCatalogID string) *ReadOnlyCapabilityRegistry {
	r := &ReadOnlyCapabilityRegistry{
		repository:                repository,
		capabilities:              make(map[string]MerchantCatalogDiscoveryTool),
		selectedCatalogID:         strings.TrimSpace(selectedCatalogID),
		evidenceReferences:        make(map[string]struct{}),
		attributeSchemaReferences: make(map[string]struct{}),
		issuedCursors:             make(map[string]string),
	}
	if repository == nil {
		return r
	}
	r.capabilities["merchant_catalog_list_items"] = listItemsCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	r.capabilities["merchant_catalog_get_item"] = getItemCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	r.capabilities["merchant_catalog_list_variants"] = listVariantsCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	r.capabilities["merchant_catalog_list_offers"] = listOffersCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	r.capabilities["merchant_catalog_list_attribute_schemas"] = listAttributeSchemasCapability{repository: repository}
	log.Printf("[MerchantCatalogAI][DISCOVERY] registry_initialized selected_catalog=%s tools=%d",
		r.selectedCatalogID, len(r.capabilities))
	return r
}

func (r *ReadOnlyCapabilityRegistry) Definitions() []MerchantCatalogDiscoveryToolDefinition {
	out := make([]MerchantCatalogDiscoveryToolDefinition, 0, len(r.capabilities))
	for _, capability := range r.capabilities {
		out = append(out, capability.Definition())
	}
	log.Printf("[MerchantCatalogAI][DISCOVERY] definitions_ready selected_catalog=%s count=%d",
		r.selectedCatalogID, len(out))
	return out
}

func (r *ReadOnlyCapabilityRegistry) Execute(ctx context.Context, execCtx MerchantCatalogDiscoveryExecutionContext, name string, rawParams []byte) (MerchantCatalogDiscoveryResult, error) {
	capability, ok := r.capabilities[name]
	if !ok {
		return MerchantCatalogDiscoveryResult{}, errors.New("merchant catalog capability is not available: " + name)
	}
	var cursorScope string
	if strings.HasPrefix(name, "merchant_catalog_list_") {
		var cursor string
		var err error
		cursorScope, cursor, err = merchantReadCursorScope(r.selectedCatalogID, name, execCtx, rawParams)
		if err != nil {
			return MerchantCatalogDiscoveryResult{}, err
		}
		if cursor != "" && r.issuedCursors[cursorScope] != cursor {
			return MerchantCatalogDiscoveryResult{}, errors.New("merchant catalog pagination cursor was not issued for this read scope")
		}
	}
	started := time.Now()
	log.Printf("[MerchantCatalogAI][DISCOVERY] START business=%s session=%s tool=%s params_bytes=%d",
		execCtx.BusinessID, execCtx.ConversationID, name, len(rawParams))

	result, err := capability.Execute(ctx, execCtx, rawParams)
	if err != nil {
		log.Printf("[MerchantCatalogAI][DISCOVERY] ERROR business=%s session=%s tool=%s latency_ms=%d err=%v",
			execCtx.BusinessID, execCtx.ConversationID, name, time.Since(started).Milliseconds(), err)
		return MerchantCatalogDiscoveryResult{}, err
	}

	if cursorScope != "" {
		if result.HasMore && result.NextCursor == "" {
			return MerchantCatalogDiscoveryResult{}, errors.New("merchant catalog continuation cursor is missing")
		}
		if !result.HasMore && result.NextCursor != "" {
			return MerchantCatalogDiscoveryResult{}, errors.New("merchant catalog returned a cursor after the final page")
		}
		if result.HasMore {
			r.issuedCursors[cursorScope] = result.NextCursor
		} else {
			delete(r.issuedCursors, cursorScope)
		}
	}
	r.recordEvidence(result)

	if name == "merchant_catalog_list_attribute_schemas" {
		schemaIDs, normalizeErr := normalizeAttributeSchemaReferences(result.Data)
		if normalizeErr != nil {
			log.Printf("[MerchantCatalogAI][DISCOVERY] NORMALIZE_ERROR business=%s session=%s tool=%s latency_ms=%d err=%v",
				execCtx.BusinessID, execCtx.ConversationID, name, time.Since(started).Milliseconds(), normalizeErr)
			return MerchantCatalogDiscoveryResult{}, normalizeErr
		}
		for _, id := range schemaIDs {
			r.attributeSchemaReferences[id] = struct{}{}
		}
		log.Printf("[MerchantCatalogAI][DISCOVERY] SCHEMAS business=%s session=%s count=%d ids=%v",
			execCtx.BusinessID, execCtx.ConversationID, len(schemaIDs), schemaIDs)
	}

	log.Printf("[MerchantCatalogAI][DISCOVERY] OK business=%s session=%s tool=%s operation=%s latency_ms=%d evidence_refs=%d",
		execCtx.BusinessID, execCtx.ConversationID, name, result.Operation, time.Since(started).Milliseconds(),
		len(result.EvidenceReferences))
	return result, nil
}

func (r *ReadOnlyCapabilityRegistry) recordEvidence(result MerchantCatalogDiscoveryResult) {
	for _, reference := range result.EvidenceReferences {
		if ref := strings.TrimSpace(reference); ref != "" {
			r.evidenceReferences[ref] = struct{}{}
		}
	}
}

func (r *ReadOnlyCapabilityRegistry) Get(name string) (MerchantCatalogDiscoveryTool, bool) {
	capability, ok := r.capabilities[name]
	return capability, ok
}

// merchantReadCursorScope binds an opaque, server-issued continuation to the
// exact tool, catalog, merchant and read parameters for this agent turn.
func merchantReadCursorScope(catalogID, tool string, execCtx MerchantCatalogDiscoveryExecutionContext, rawParams []byte) (string, string, error) {
	params, err := jsonParams(rawParams)
	if err != nil {
		return "", "", err
	}
	var cursor string
	if value, present := params["cursor"]; present && value != nil {
		var ok bool
		cursor, ok = value.(string)
		if !ok {
			return "", "", errors.New("merchant catalog pagination cursor must be a string or null")
		}
	}
	delete(params, "cursor")
	query, err := json.Marshal(params)
	if err != nil {
		return "", "", errors.New("invalid merchant catalog pagination scope")
	}
	scope := strings.Join([]string{tool, catalogID, execCtx.BusinessID, execCtx.PrincipalID, execCtx.ConversationID, string(query)}, "\x00")
	return scope, cursor, nil
}


// ValidateProposalSchemaAttributes is a server-side, tenant-scoped preflight
// using the existing schema, not model-provided or newly invented definitions.
func (r *ReadOnlyCapabilityRegistry) ValidateProposalSchemaAttributes(ctx context.Context, businessID string, p Proposal) error {
	if r == nil || r.repository == nil {
		return errors.New("merchant catalog schema preflight repository is unavailable")
	}
	return ValidateProposalSchemaAttributes(ctx, r.repository, businessID, r.selectedCatalogID, p)
}
