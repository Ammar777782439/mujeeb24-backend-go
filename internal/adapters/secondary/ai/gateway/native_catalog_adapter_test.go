package gateway

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
)
func TestNativeCatalogStrictCapacityEstimationIsConservative(t *testing.T) {
	primary:=&customerNativeProvider{name:"gemini"}
	backup:=&customerNativeProvider{name:"deepseek"}
	router,err:=native.NewRouter(primary,backup,native.Policy{ConsecutiveFailures:2,Cooldown:time.Minute})
	if err!=nil{t.Fatal(err)}
	client,err:=native.NewCompletionClient(router)
	if err!=nil{t.Fatal(err)}
	adapter,err:=NewNativeCatalogAdapter(client,"mujeeb-catalog",2048,65536)
	if err!=nil{t.Fatal(err)}
	req:=chatCompletionRequest{
		Model:"mujeeb-catalog",Messages:[]chatCompletionMessage{
			{Role:"system",Content:"trusted"},{Role:"user",Content:strings.Repeat("large catalog evidence",100)},
		},
		MaxTokens:2048,
	}
	count,err:=adapter.count(context.Background(),req)
	if err!=nil{t.Fatal(err)}
	if count < len(strings.Repeat("large catalog evidence",100))+4096 {
		t.Fatalf("unsafe context estimate: %d",count)
	}
	if err:=adapter.ensureModelCapacity(65536);err==nil {t.Fatal("oversized canonical request allowed")}
	if primary.calls!=0||backup.calls!=0 {t.Fatal("token estimate must not call unavailable AI provider")}
}
