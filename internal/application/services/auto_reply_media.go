package services

import (
	"context"
	"log"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// selectReplyMedia is post-validation only. The model chooses from exact
// business-scoped approved gallery links; Go never infers intent from keywords.
func (s AutoReplyService) selectReplyMedia(
	ctx context.Context, businessID, conversationID, runID, customerText string,
	decision ports.CapabilityDecision,
) ([]ports.CatalogMediaLink, ports.CustomerSalesUsageTelemetry) {
	if s.CatalogMedia == nil || s.MediaSelection == nil ||
		decision.Capability != ports.CustomerSalesCapabilityRespondWithoutAction ||
		len(decision.Selected) == 0 || len(decision.Selected) > 3 {
		return nil, ports.CustomerSalesUsageTelemetry{}
	}
	options := make([]ports.CustomerMediaOption, 0, 12)
	links := make(map[string]ports.CatalogMediaLink)
	for _, selected := range decision.Selected {
		if strings.TrimSpace(selected.ItemID) == "" {
			continue
		}
		gallery, err := s.CatalogMedia.ListGallery(ctx, businessID, selected.ItemID, selected.VariantID)
		if err != nil {
			log.Printf("[CatalogMedia] stage=gallery_failed business=%s run=%s fallback=text", businessID, runID)
			return nil, ports.CustomerSalesUsageTelemetry{}
		}
		for _, link := range gallery {
			if len(options) >= 12 {
				break
			}
			if link.CatalogItemID != selected.ItemID ||
				!mediaVariantMatches(link.VariantID, selected.VariantID) ||
				link.Status != "active" || link.ApprovedAt == nil {
				continue
			}
			// FindApprovedMediaLink validates catalog/product/variant and asset
			// lifecycle; GetAsset also guards against deleted underlying storage.
			current, err := s.CatalogMedia.FindApprovedMediaLink(ctx, businessID, link.ID)
			if err != nil || current == nil || current.MediaID != link.MediaID {
				continue
			}
			if _, duplicate := links[link.ID]; duplicate {
				continue
			}
			options = append(options, ports.CustomerMediaOption{
				MediaLinkID: link.ID, ItemID: link.CatalogItemID,
				VariantID: link.VariantID, AltText: link.AltText,
				IsPrimary: link.IsPrimary, SortOrder: link.SortOrder,
			})
			links[link.ID] = link
		}
	}
	if len(options) == 0 {
		return nil, ports.CustomerSalesUsageTelemetry{}
	}
	out, err := s.MediaSelection.SelectMedia(ctx, ports.CustomerMediaSelectionInput{
		BusinessID: businessID, ConversationID: conversationID, RunID: runID,
		CustomerMessage: customerText, ResponseText: decision.ResponseText, Options: options,
	})
	if err != nil {
		log.Printf("[CatalogMedia] stage=selection_failed business=%s run=%s fallback=text", businessID, runID)
		return nil, ports.CustomerSalesUsageTelemetry{}
	}
	// Independently validate model output even when another port implementation
	// is injected. Repeated or unapproved IDs can never be sent.
	if len(out.MediaLinkIDs) > 4 {
		return nil, out.Usage
	}
	seen := make(map[string]bool)
	selected := make([]ports.CatalogMediaLink, 0, len(out.MediaLinkIDs))
	for _, id := range out.MediaLinkIDs {
		link, ok := links[id]
		if !ok || seen[id] {
			log.Printf("[CatalogMedia] stage=invalid_model_media business=%s run=%s fallback=text", businessID, runID)
			return nil, out.Usage
		}
		seen[id] = true
		selected = append(selected, link)
	}
	log.Printf("[CatalogMedia] stage=selected business=%s run=%s candidates=%d images=%d", businessID, runID, len(options), len(selected))
	return selected, out.Usage
}
