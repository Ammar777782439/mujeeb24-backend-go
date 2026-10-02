package merchantcatalogai

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type ReadOnlyCapabilityRegistry struct {
	capabilities              map[string]MerchantCatalogDiscoveryTool
	selectedCatalogID         string
	evidenceReferences        map[string]struct{}
	attributeSchemaReferences map[string]struct{}
}

func NewReadOnlyCapabilityRegistry(repository ports.CatalogRepository, selectedCatalogID string) *ReadOnlyCapabilityRegistry {
	r := &ReadOnlyCapabilityRegistry{
		capabilities:              make(map[string]MerchantCatalogDiscoveryTool),
		selectedCatalogID:         strings.TrimSpace(selectedCatalogID),
		evidenceReferences:        make(map[string]struct{}),
		attributeSchemaReferences: make(map[string]struct{}),
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
	started := time.Now()
	log.Printf("[MerchantCatalogAI][DISCOVERY] START business=%s session=%s tool=%s params_bytes=%d",
		execCtx.BusinessID, execCtx.ConversationID, name, len(rawParams))

	result, err := capability.Execute(ctx, execCtx, rawParams)
	if err != nil {
		log.Printf("[MerchantCatalogAI][DISCOVERY] ERROR business=%s session=%s tool=%s latency_ms=%d err=%v",
			execCtx.BusinessID, execCtx.ConversationID, name, time.Since(started).Milliseconds(), err)
		return MerchantCatalogDiscoveryResult{}, err
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

	log.Printf("[MerchantCatalogAI][DISCOVERY] OK business=%s session=%s tool=%s operation=%s latency_ms=%d catalog_refs=%d variant_refs=%d offer_refs=%d",
		execCtx.BusinessID, execCtx.ConversationID, name, result.Operation, time.Since(started).Milliseconds(),
		len(result.CatalogEvidence), len(result.VariantEvidence), len(result.OfferEvidence))
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
