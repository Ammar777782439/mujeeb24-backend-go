package handlers

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func TestCatalogEntityContractExportsIndependentLifecycleDimensions(t *testing.T) {
	server := NewServer(Dependencies{
		Scope:                    fakeScope{actor: commands.ActorContext{PrincipalID: "principal-1", BusinessID: "business-1", Role: "owner"}},
		GetCatalogEntityContract: services.GetCatalogEntityContractQueryService{},
	})
	result, handled := server.dispatchAdditionalQuery(context.Background(), "getCatalogEntityContract", &contract.CatalogEntityContractPath{BusinessID: "business-1"})
	if !handled {
		t.Fatal("entity contract route was not handled")
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Body struct {
			Data map[string]map[string]string `json:"data"`
		}
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	d := services.DefaultCatalogEntityContractDescriptor()
	for key, want := range map[string]map[string]string{
		"catalog_statuses": d.CatalogStatuses, "item_statuses": d.ItemStatuses,
		"variant_statuses": d.VariantStatuses, "offer_statuses": d.OfferStatuses,
		"availability_statuses": d.AvailabilityStatuses,
	} {
		if !reflect.DeepEqual(response.Body.Data[key], want) {
			t.Errorf("%s was dropped or changed: %s", key, data)
		}
	}
	if _, ok := response.Body.Data["offer_statuses"]["expired"]; !ok {
		t.Fatal("offer expired status is missing")
	}
	if _, ok := response.Body.Data["item_statuses"]["expired"]; ok {
		t.Fatal("offer status leaked into item status")
	}
}
