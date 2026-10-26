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
	if err != nil { t.Fatal(err) }
	if result.Coverage != "verified_navigation_with_actor_permissions" { t.Fatalf("invalid help coverage: %s", result.Coverage) }
	if len(result.Links) != 1 || result.Links[0].URL != "/catalog" { t.Fatalf("expected permitted catalog entry only, got %+v", result.Links) }
	actor.Permissions = []string{"merchant_ai:use"}
	limited, err := reader.Read(context.Background(), actor, "business-1", DiscoveryCall{Name: "platform_guide"})
	if err != nil { t.Fatal(err) }
	if len(limited.Links) != 0 { t.Fatalf("unauthorized page revealed: %+v", limited.Links) }
}

func TestNativeAssistCanTeachMerchantAboutCatalogWithoutModifyingIt(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil { t.Fatal(err) }
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
	if err != nil { t.Fatal(err) }
	if !strings.Contains(response.Answer, "الكتالوج") || !strings.Contains(string(response.Evidence), "/catalog") {
		t.Fatalf("missing catalog guidance: %+v", response)
	}
	if len(model.seen) != 2 { t.Fatal("expected native tool round and final answer") }
}
