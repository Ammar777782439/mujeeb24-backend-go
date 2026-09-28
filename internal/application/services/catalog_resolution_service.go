// Package services — CatalogResolutionService implements the deterministic
// catalog selection logic for the B2B MerchantCatalogAIAgent (ADR-041).
//
// Per ADR-041, the catalog selection is FORBIDDEN to be done by AI. The AI
// is only allowed to:
//   (a) read the merchant_catalogs evidence (so it knows what catalogs exist)
//   (b) formulate a question to the merchant when this service says "ask"
//   (c) answer informational queries ("how many catalogs?") by reading the evidence
//
// The AI is NEVER allowed to populate TargetCatalogID itself. This service
// does the selection deterministically, in this strict priority:
//
//   1. HTTP parameter (target_catalog_id from the dashboard dropdown).
//      The merchant explicitly picks a catalog before sending the message;
//      every turn in that session is scoped to it.
//
//   2. Session-stored sticky catalog_id (target_catalog_id in
//      merchant_ai_sessions). After the merchant confirms a catalog once,
//      subsequent turns reuse it without re-asking.
//
//   3. Single-catalog auto-select. If the merchant has exactly one active
//      catalog, it's used without asking.
//
//   4. Failure path (NeedsAsk=true). If all three layers above fail (no
//      explicit param, no sticky, 0 or 2+ catalogs), the service returns
//      NeedsAsk=true with a question template. The HandleTurn code converts
//      the proposed operation into "ask_merchant" with this question.
//
// This separation guarantees NO hallucination: the AI never picks a catalog
// ID. The code either picks one deterministically, or forces a merchant
// question.

package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// CatalogResolutionService is the deterministic catalog selector. It has
// NO AI dependency — pure Go logic with explicit priority layers.
type CatalogResolutionService struct {
	// SessionWriter persists the sticky catalog_id after a successful
	// resolution (layer 1 or layer 3). Optional — if nil, sticky
	// persistence is skipped.
	SessionWriter MerchantAISessionCatalogWriter
}

// MerchantAISessionCatalogWriter is the subset of MerchantAISessionWriter
// used by the CatalogResolutionService for sticky catalog persistence.
// Defined separately so the service can be tested with a mock without
// pulling in the full MerchantAISessionWriter interface.
type MerchantAISessionCatalogWriter interface {
	SetTargetCatalog(ctx context.Context, businessID, sessionID, catalogID string) error
}

// CatalogResolution is the output of CatalogResolutionService.Resolve.
// Exactly one of (CatalogID, NeedsAsk) is meaningful:
//   - If Resolved=true → CatalogID is set, Reason explains which layer
//     resolved it, NeedsAsk=false.
//   - If NeedsAsk=true → CatalogID is empty, AskQuestion is set,
//     Resolved=false. The HandleTurn code converts the proposed operation
//     into "ask_merchant" with AskQuestion as the response_text.
type CatalogResolution struct {
	Resolved        bool
	CatalogID       string
	Reason          string // "explicit" | "session_sticky" | "single" | "explicit_not_found" | "none" | "ambiguous"
	NeedsAsk        bool
	AskQuestion     string // populated when NeedsAsk=true
	ShouldSetSticky bool   // true if caller should persist catalog_id as sticky
}

// Resolve applies the 4-layer priority to pick a catalog deterministically.
//
// Inputs:
//   - targetCatalogIDFromRequest: layer 1 — HTTP param from dashboard
//   - sessionStickyCatalogID:     layer 2 — sticky from merchant_ai_sessions
//   - merchantCatalogs:           the catalog evidence loaded by ContextBuilder
//
// The function is pure: no DB writes, no AI calls. Side effects (sticky
// persistence) happen in HandleTurn based on ShouldSetSticky.
func (s *CatalogResolutionService) Resolve(
	targetCatalogIDFromRequest string,
	sessionStickyCatalogID string,
	merchantCatalogs []ports.MerchantCatalogEntry,
) CatalogResolution {
	// Layer 1: HTTP parameter (highest priority — explicit merchant choice).
	if strings.TrimSpace(targetCatalogIDFromRequest) != "" {
		if entry, ok := findCatalogByID(targetCatalogIDFromRequest, merchantCatalogs); ok {
			return CatalogResolution{
				Resolved:        true,
				CatalogID:       entry.ID,
				Reason:          "explicit",
				ShouldSetSticky: true, // make it sticky so subsequent turns don't need the param
			}
		}
		// Explicit param points to a catalog that doesn't exist (or isn't
		// active). This is an error — we don't fall through to layer 2
		// because the merchant was explicit.
		return CatalogResolution{
			NeedsAsk:    true,
			Reason:      "explicit_not_found",
			AskQuestion: fmt.Sprintf("الكتالوج المحدد (معرّف %s) غير موجود أو غير نشط. الرجاء اختيار كتالوج من القائمة.", targetCatalogIDFromRequest),
		}
	}

	// Layer 2: session sticky.
	if strings.TrimSpace(sessionStickyCatalogID) != "" {
		if entry, ok := findCatalogByID(sessionStickyCatalogID, merchantCatalogs); ok {
			return CatalogResolution{
				Resolved:    true,
				CatalogID:   entry.ID,
				Reason:      "session_sticky",
				// ShouldSetSticky=false — it's already sticky, no need to re-set
			}
		}
		// Sticky points to a deleted catalog — fall through to layer 3.
		// We don't ask here because the merchant didn't see anything wrong;
		// we just try the next layer.
	}

	// Layer 3: single-catalog auto-select.
	if len(merchantCatalogs) == 1 {
		return CatalogResolution{
			Resolved:        true,
			CatalogID:       merchantCatalogs[0].ID,
			Reason:          "single",
			ShouldSetSticky: true, // persist so future turns skip layer 3
		}
	}

	// Layer 4: failure — need to ask the merchant.
	if len(merchantCatalogs) == 0 {
		return CatalogResolution{
			NeedsAsk:    true,
			Reason:      "none",
			AskQuestion: "ما عندك أي كتالوج نشط حاليًا. تبغى أنشئ كتالوج جديد أولًا قبل إضافة منتج؟",
		}
	}
	// Multiple catalogs, no explicit choice, no sticky → ask the merchant.
	// Build a list with names and item counts so the merchant can pick.
	return CatalogResolution{
		NeedsAsk:    true,
		Reason:      "ambiguous",
		AskQuestion: fmt.Sprintf("عندك %d كتالوج. لأي كتالوج تبغى تضيف المنتج؟ %s",
			len(merchantCatalogs), FormatCatalogList(merchantCatalogs)),
	}
}

// findCatalogByID scans the catalog list and returns the entry matching
// the given ID (case-insensitive, trimmed). Returns ok=false if not found.
func findCatalogByID(id string, catalogs []ports.MerchantCatalogEntry) (ports.MerchantCatalogEntry, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ports.MerchantCatalogEntry{}, false
	}
	for _, c := range catalogs {
		if strings.EqualFold(c.ID, id) {
			return c, true
		}
	}
	return ports.MerchantCatalogEntry{}, false
}

// FormatCatalogList produces a human-readable Arabic string of catalog
// names + item counts. Used by HandleTurn when the AI needs to ask the
// merchant which catalog.
//
// Example: "المواسم (5 منتج) / الصيفي (3 منتج) / الشتوي (12 منتج)"
func FormatCatalogList(catalogs []ports.MerchantCatalogEntry) string {
	if len(catalogs) == 0 {
		return "لا يوجد كتالوجات"
	}
	parts := make([]string, 0, len(catalogs))
	for _, c := range catalogs {
		parts = append(parts, fmt.Sprintf("%s (%d منتج)", c.Name, c.ItemsCount))
	}
	return strings.Join(parts, " / ")
}
