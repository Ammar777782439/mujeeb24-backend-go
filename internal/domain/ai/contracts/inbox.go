package contracts

import (
	"fmt"
	"strings"
	"time"
)

// Customer profile and contact_points are JSONB with unreviewed PII and must
// not appear in AI Assist until separately scrubbed and explicitly approved.
type CustomerAssistV1 struct {
	CustomerRef      string  `json:"customer_ref"`
	LocalePreference *string `json:"locale_preference,omitempty"`
	Status           string  `json:"status"`
}

func (c CustomerAssistV1) Validate() error {
	if err := requireText("customer_ref", c.CustomerRef); err != nil {
		return err
	}
	if c.LocalePreference != nil {
		if err := requireText("locale_preference", *c.LocalePreference); err != nil {
			return err
		}
	}
	return requireEnum("customer status", c.Status, "active", "merged", "archived")
}

type ConversationAssistV1 struct {
	ConversationRef string    `json:"conversation_ref"`
	CustomerRef     string    `json:"customer_ref"`
	State           string    `json:"state"`
	Ownership       string    `json:"ownership"`
	Priority        string    `json:"priority"`
	AIModeOverride  *string   `json:"ai_mode_override,omitempty"`
	LastActivityAt  time.Time `json:"last_activity_at"`
}

func (c ConversationAssistV1) Validate() error {
	if err := requireText("conversation_ref", c.ConversationRef); err != nil {
		return err
	}
	if err := requireText("customer_ref", c.CustomerRef); err != nil {
		return err
	}
	if c.LastActivityAt.IsZero() {
		return fmt.Errorf("%w: missing activity time", ErrInvalidReviewedProjection)
	}
	if err := requireEnum("conversation state", c.State, "open", "ai_handling", "waiting_customer", "waiting_human", "human_handling", "closed"); err != nil {
		return err
	}
	if err := requireEnum("ownership", c.Ownership, "none", "ai", "human"); err != nil {
		return err
	}
	if err := requireEnum("priority", c.Priority, "low", "normal", "high", "urgent"); err != nil {
		return err
	}
	if c.AIModeOverride != nil {
		return requireEnum("ai_mode_override", *c.AIModeOverride, "allowed", "draft_only", "disabled")
	}
	return nil
}

// V1 drafts replies only from visible public text. Private notes and provider
// metadata are not part of this projection. Source text must be sanitized
// and authorization-checked before any external model request.
type PublicTextMessageAssistV1 struct {
	MessageRef  string    `json:"message_ref"`
	Direction   string    `json:"direction"`
	Origin      string    `json:"origin"`
	Visibility  string    `json:"visibility"`
	ContentType string    `json:"content_type"`
	Text        string    `json:"text"`
	OccurredAt  time.Time `json:"occurred_at"`
}

func (m PublicTextMessageAssistV1) Validate() error {
	if err := requireText("message_ref", m.MessageRef); err != nil {
		return err
	}
	if err := requireText("text", m.Text); err != nil {
		return err
	}
	if m.OccurredAt.IsZero() {
		return fmt.Errorf("%w: missing occurred_at", ErrInvalidReviewedProjection)
	}
	if err := requireEnum("direction", m.Direction, "inbound", "outbound"); err != nil {
		return err
	}
	if err := requireEnum("origin", m.Origin, "customer", "ai", "human", "automation", "system"); err != nil {
		return err
	}
	if err := requireEnum("visibility", m.Visibility, "public"); err != nil {
		return err
	}
	return requireEnum("content_type", m.ContentType, "text")
}

// A summary MUST reference the message at which the committed summary
// stopped. Newer messages must be fetched independently by the reader.
// AI-generated summary text is lower-trust than actual customer messages.
type ConversationSummaryAssistV1 struct {
	Summary               string `json:"summary"`
	LastCoveredMessageRef string `json:"last_covered_message_ref"`
	CoveredTurnCount      int    `json:"covered_turn_count"`
}

func (s ConversationSummaryAssistV1) Validate() error {
	if err := requireText("summary", s.Summary); err != nil {
		return err
	}
	if err := requireText("last_covered_message_ref", s.LastCoveredMessageRef); err != nil {
		return err
	}
	if s.CoveredTurnCount < 0 {
		return fmt.Errorf("%w: negative turn count", ErrInvalidReviewedProjection)
	}
	return nil
}

type CannedReplyAssistV1 struct {
	Title    string `json:"title"`
	Shortcut string `json:"shortcut"`
	Body     string `json:"body"`
	Status   string `json:"status"`
}

func (r CannedReplyAssistV1) Validate() error {
	for _, v := range []struct{ field, value string }{{"title", r.Title}, {"shortcut", r.Shortcut}, {"body", r.Body}} {
		if err := requireText(v.field, v.value); err != nil {
			return err
		}
	}
	if err := requireEnum("status", r.Status, "active", "archived"); err != nil {
		return err
	}
	if r.Shortcut != normalizeShortcut(r.Shortcut) {
		return fmt.Errorf("%w: shortcut not normalized", ErrInvalidReviewedProjection)
	}
	return nil
}

func normalizeShortcut(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
