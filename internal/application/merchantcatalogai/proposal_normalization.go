package merchantcatalogai

import "strings"

func (p Proposal) Normalize() Proposal {
	if p.Status != StatusResolved && p.IsMutation() {
		p.Operation = OperationAskMerchant
		p.Create = nil
		p.Update = nil
		p.Delete = nil
		if strings.TrimSpace(p.ResponseText) == "" {
			p.ResponseText = "حدّد البيانات الناقصة المطلوبة لإتمام العملية."
		}
		if len(p.MissingInformation) == 0 {
			p.MissingInformation = []MissingField{
				{
					Path:        "proposal",
					DisplayName: "بيانات العملية",
					DataType:    "object",
					Reason:      "لا يمكن اعتماد عملية تعديل أو إنشاء قبل اكتمال البيانات المطلوبة.",
				},
			}
		}
		return p
	}

	if p.Status == StatusResolved && p.Operation == OperationUpdate && p.Update != nil {
		u := p.Update
		emptyItemChanges := u.Changes.Name == nil &&
			u.Changes.ItemType == nil &&
			u.Changes.ShortDescription == nil &&
			u.Changes.LongDescription == nil &&
			u.Changes.Status == nil &&
			u.Changes.PricingMode == nil &&
			u.Changes.AvailabilityMode == nil &&
			u.Changes.FulfillmentMode == nil &&
			u.Changes.Attributes == nil &&
			u.Changes.RequiresConfirmation == nil

		if strings.TrimSpace(u.ItemID) == "" ||
			(emptyItemChanges && len(u.ExistingVariants) == 0 && len(u.NewVariants) == 0 &&
				len(u.ExistingOffers) == 0 && len(u.NewOffers) == 0) {
			p.Status = StatusNeedsMoreData
			p.Operation = OperationAskMerchant
			p.Update = nil
			p.Create = nil
			p.Delete = nil
			p.MissingInformation = []MissingField{
				{Path: "update.item_id", DisplayName: "المنتج", DataType: "catalog_item", Reason: "يجب تحديد المنتج المراد تعديله."},
				{Path: "update.changes", DisplayName: "بيانات التعديل", DataType: "object", Reason: "يجب تحديد القيمة أو البيانات الجديدة المطلوب اعتمادها."},
			}
			p.ResponseText = "حدّد المنتج والبيانات الجديدة التي تريد تعديلها."
		}
	}

	return p
}
