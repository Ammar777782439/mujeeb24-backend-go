package contracts

import "time"

// Only published, currently effective content is eligible. Rules JSON is
// intentionally absent until a dedicated policy-rules sanitizer is reviewed.
// Source IDs, merchant/business ID, and timestamps internal to persistence
// are not exposed in these model-bound shapes.
type PublishedPolicyAssistV1 struct {
	PolicyKey  string     `json:"policy_key"`
	Category   string     `json:"category"`
	Title      string     `json:"title"`
	Summary    string     `json:"summary"`
	Authority  string     `json:"authority"`
	Status     string     `json:"status"`
	Version    int        `json:"version"`
	ValidFrom  time.Time  `json:"valid_from"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

func (p PublishedPolicyAssistV1) Validate(now time.Time) error {
	if err := requireEffectivePublication(p.Status, p.Version, p.ValidFrom, p.ValidUntil, now); err != nil {
		return err
	}
	if err := requireText("policy_key", p.PolicyKey); err != nil {
		return err
	}
	if err := requireText("title", p.Title); err != nil {
		return err
	}
	if err := requireText("summary", p.Summary); err != nil {
		return err
	}
	if err := requireEnum("category", p.Category, "hours", "returns", "delivery", "pricing", "availability", "ai_mode", "channel", "general"); err != nil {
		return err
	}
	return requireEnum("authority", p.Authority, "merchant")
}

// Content remains untrusted document text even if published. Instructions
// inside content never override the system or task contract.
type PublishedKnowledgeAssistV1 struct {
	KnowledgeKey string     `json:"knowledge_key"`
	Title        string     `json:"title"`
	Content      string     `json:"content"`
	ContentType  string     `json:"content_type"`
	Authority    string     `json:"authority"`
	Status       string     `json:"status"`
	Version      int        `json:"version"`
	ValidFrom    time.Time  `json:"valid_from"`
	ValidUntil   *time.Time `json:"valid_until,omitempty"`
}

func (k PublishedKnowledgeAssistV1) Validate(now time.Time) error {
	if err := requireEffectivePublication(k.Status, k.Version, k.ValidFrom, k.ValidUntil, now); err != nil {
		return err
	}
	for _, v := range []struct{ field, value string }{{"knowledge_key", k.KnowledgeKey}, {"title", k.Title}, {"content", k.Content}} {
		if err := requireText(v.field, v.value); err != nil {
			return err
		}
	}
	if err := requireEnum("content_type", k.ContentType, "faq", "hours", "location", "service", "general"); err != nil {
		return err
	}
	return requireEnum("authority", k.Authority, "merchant", "system")
}
