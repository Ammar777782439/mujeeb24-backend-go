package merchantassist

import (
	"context"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

func TestPlatformGuideShowsCatalogHandoffOnlyWhenAuthorized(t *testing.T) {
	reader := &DiscoveryReader{Guard: &guardFixture{}}
	actor := commands.ActorContext{
		BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "catalog:read"},
	}
	result, err := reader.Read(context.Background(), actor, "business-1", DiscoveryCall{Name: "platform_guide"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage != "verified_navigation_with_actor_permissions" {
		t.Fatalf("invalid help coverage: %s", result.Coverage)
	}
	if len(result.Links) != 1 || result.Links[0].URL != "/catalog" {
		t.Fatalf("expected permitted catalog entry only, got %+v", result.Links)
	}
	actor.Permissions = []string{"merchant_ai:use"}
	limited, err := reader.Read(context.Background(), actor, "business-1", DiscoveryCall{Name: "platform_guide"})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited.Links) != 0 {
		t.Fatalf("unauthorized page revealed: %+v", limited.Links)
	}
}

func TestNativeAssistCanTeachMerchantAboutCatalogWithoutModifyingIt(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	model := &nativeModelStub{replies: []ToolCompletion{
		nativeCall("help-1", "platform_guide", "{}"),
		nativeAnswer("لإضافة منتج افتح الكتالوج، ثم استخدم مساعد الكتالوج المخصص؛ لن أعدل المنتجات من هنا."),
	}}
	actor := commands.ActorContext{
		BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "catalog:read"},
	}
	assistant := &Assistant{
		Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}},
	}
	response, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1",
		Task: contracts.TaskExplain, Question: "مش فاهم كيف أضيف منتج، ساعدني",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Answer, "الكتالوج") || !strings.Contains(string(response.Evidence), "/catalog") {
		t.Fatalf("missing catalog guidance: %+v", response)
	}
	if len(model.seen) != 2 {
		t.Fatal("expected native tool round and final answer")
	}
}

func TestPlatformGuideUsesRealMerchantRoutesAndPermissionBoundaries(t *testing.T) {
	reader := &DiscoveryReader{Guard: &guardFixture{}}
	actor := commands.ActorContext{
		BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "sales:read", "automation:read", "team:read"},
	}
	result, err := reader.Read(context.Background(), actor, "business-1", DiscoveryCall{Name: "platform_guide"})
	if err != nil { t.Fatal(err) }
	allowed := map[string]bool{"/leads": true, "/transactions": true, "/sales-workflows": true,
		"/settings/sales": true, "/settings/automation-rules": true, "/settings/canned-replies": true,
		"/team": true}
	found := map[string]bool{}
	for _, link := range result.Links {
		if !allowed[link.URL] { t.Fatalf("unauthorized route was shown: %s", link.URL) }
		found[link.URL] = true
	}
	if len(found) != len(allowed) { t.Fatalf("missing permitted routes: %+v", result.Links) }
}

func TestPlatformGuideDoesNotClaimCatalogWritesForReader(t *testing.T) {
	reader := &DiscoveryReader{Guard: &guardFixture{}}
	actor := commands.ActorContext{BusinessID:"business-1", PrincipalID:"principal-1",
		Permissions:[]string{"merchant_ai:use","catalog:read"}}
	result, err := reader.Read(context.Background(),actor,"business-1",DiscoveryCall{Name:"platform_guide"})
	if err != nil {t.Fatal(err)}
	rows, ok := result.Rows.(map[string]any)
	if !ok {t.Fatal("no area contract")}
	areas, ok := rows["areas"].([]map[string]any)
	if !ok || len(areas)!=1 {t.Fatalf("invalid authorized areas: %+v",rows)}
	detail, _ := areas[0]["description"].(string)
	if strings.Contains(detail,"لإضافة") || !strings.Contains(detail,"لا تعديل") && !strings.Contains(detail,"للقراءة فقط") {
		t.Fatalf("catalog reader was offered write guidance: %s",detail)
	}
	actor.Permissions=append(actor.Permissions,"catalog:write")
	enabled, err:=reader.Read(context.Background(),actor,"business-1",DiscoveryCall{Name:"platform_guide"})
	if err != nil { t.Fatal(err) }
	more:=enabled.Rows.(map[string]any)["areas"].([]map[string]any)
	guide, _:=more[0]["description"].(string)
	if !strings.Contains(guide,"مساعد الكتالوج المستقل") {
		t.Fatalf("write-enabled user missing reviewed catalog route: %s",guide)
	}
}
