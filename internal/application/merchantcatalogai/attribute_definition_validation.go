package merchantcatalogai

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/catalogvalidation"
)

// ValidateAttributeDefinitions validates proposed human-readable labels and their
// semantic aliases. requireLabels enforces coverage of free keys in the AI wire
// transport; legacy API proposals may have older custom attributes.
func (p Proposal) ValidateAttributeDefinitions(requireLabels bool) error {
	if len(p.AttributeDefinitions) > 100 {
		return errors.New("too many merchant attribute definitions")
	}
	byKey := map[string]ProposedAttributeDefinition{}
	for _, d := range p.AttributeDefinitions {
		if !attributeKeyPattern.MatchString(d.Key) || strings.HasPrefix(d.Key, "attr_") {
			return fmt.Errorf("invalid semantic attribute alias %q", d.Key)
		}
		if _, exists := byKey[d.Key]; exists {
			return fmt.Errorf("duplicate attribute definition %q", d.Key)
		}
		if strings.TrimSpace(d.Label) == "" || len([]rune(d.Label)) > 120 {
			return errors.New("attribute display label is missing or too long")
		}
		hasArabic := false
		for _, r := range d.Label {
			if unicode.Is(unicode.Arabic, r) {
				hasArabic = true
			}
			if unicode.IsControl(r) {
				return errors.New("attribute label has control characters")
			}
		}
		if !hasArabic {
			return fmt.Errorf("attribute %q requires an Arabic display label", d.Key)
		}
		if err := catalogvalidation.ValidateDefinition(d.DataType, map[string]any{}); err != nil {
			return err
		}
		byKey[d.Key] = d
	}
	used := map[string]bool{}
	visit := func(attrs map[string]any) error {
		for key := range attrs {
			if strings.HasPrefix(key, "attr_") {
				continue
			}
			used[key] = true
			if requireLabels {
				if _, ok := byKey[key]; !ok {
					return fmt.Errorf("missing Arabic attribute label for %q", key)
				}
			}
		}
		return nil
	}
	if p.Create != nil {
		if err := visit(p.Create.Attributes); err != nil {
			return err
		}
		for _, v := range p.Create.Variants {
			if err := visit(v.Attributes); err != nil {
				return err
			}
		}
	}
	if p.Update != nil {
		if err := visit(p.Update.Changes.Attributes); err != nil {
			return err
		}
		for _, v := range p.Update.ExistingVariants {
			if err := visit(v.Attributes); err != nil {
				return err
			}
		}
		for _, v := range p.Update.NewVariants {
			if err := visit(v.Attributes); err != nil {
				return err
			}
		}
	}
	for key := range byKey {
		if !used[key] {
			return fmt.Errorf("attribute label %q is not used in the proposal", key)
		}
	}
	if !p.IsMutation() && len(byKey) > 0 {
		return errors.New("read-only merchant proposal cannot contain attribute definitions")
	}
	return nil
}
