package contracts

import (
	"fmt"
	"math/big"
	"time"
)

// LeadAttributionAssistV1 contains verified *references* and provenance,
// not unreviewed source_interaction_reference or campaign identifiers.
// A recorded attribution does not prove that the customer purchased.
type LeadAttributionAssistV1 struct {
	AttributionRef string     `json:"attribution_ref"`
	LeadRef        string     `json:"lead_ref"`
	Channel        *string    `json:"channel,omitempty"`
	ConversationRef *string   `json:"conversation_ref,omitempty"`
	CatalogItemRef *string    `json:"catalog_item_ref,omitempty"`
	OfferRef       *string    `json:"offer_ref,omitempty"`
	CapturedAt     time.Time  `json:"captured_at"`
}

func (a LeadAttributionAssistV1) Validate(now time.Time) error {
	if err := requireText("attribution_ref",a.AttributionRef); err != nil {return err}
	if err := requireText("lead_ref",a.LeadRef); err != nil {return err}
	if a.CapturedAt.IsZero()||now.IsZero()||a.CapturedAt.After(now) {
		return fmt.Errorf("%w: invalid attribution timestamp",ErrInvalidReviewedProjection)
	}
	if a.Channel!=nil {
		if err:=requireEnum("source channel",*a.Channel,"facebook","instagram","whatsapp");err!=nil{return err}
	}
	for _,v:=range []struct{name string;ref *string}{{"conversation_ref",a.ConversationRef},{"catalog_item_ref",a.CatalogItemRef},{"offer_ref",a.OfferRef}}{
		if v.ref!=nil {if err:=requireText(v.name,*v.ref);err!=nil{return err}}
	}
	return nil
}

// A bounded historical score, never a claim of current conversion or sale.
// Factors JSONB and model_reference are deliberately excluded.
type LeadScoreSnapshotAssistV1 struct {
	ScoreRef    string    `json:"score_ref"`
	LeadRef     string    `json:"lead_ref"`
	Value       string    `json:"value"`
	Band        string    `json:"band"`
	CalculatedAt time.Time `json:"calculated_at"`
}

func (s LeadScoreSnapshotAssistV1) Validate(now time.Time) error {
	if err:=requireText("score_ref",s.ScoreRef);err!=nil{return err}
	if err:=requireText("lead_ref",s.LeadRef);err!=nil{return err}
	if err:=requireEnum("score band",s.Band,"unknown","low","medium","high");err!=nil{return err}
	value,ok:=new(big.Rat).SetString(s.Value)
	if !fixedDecimal2.MatchString(s.Value)||!ok||value.Cmp(big.NewRat(100,1))>0{
		return fmt.Errorf("%w: score outside numeric(5,2) / 0..100",ErrInvalidReviewedProjection)
	}
	if s.CalculatedAt.IsZero()||now.IsZero()||s.CalculatedAt.After(now){
		return fmt.Errorf("%w: invalid score timestamp",ErrInvalidReviewedProjection)
	}
	return nil
}
