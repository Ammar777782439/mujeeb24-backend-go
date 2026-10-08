//go:build integration

package postgres

import (
    "context"
    "os"
    "testing"
    "time"

    "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
    "github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
    "github.com/google/uuid"
)

func TestProviderBrandCleanupWaitsForLastChannelAndPendingOAuth(t *testing.T) {
    dsn:=os.Getenv("POSTGRES_TEST_DSN")
    if dsn=="" {t.Skip("POSTGRES_TEST_DSN is not set")}
    ctx,cancel:=context.WithTimeout(context.Background(),70*time.Second)
    defer cancel()
    if _,err:=database.RunMigrations(ctx,dsn,time.Now().UTC());err!=nil{t.Fatal(err)}
    adapter,err:=Open(ctx,dsn,DefaultPoolConfig())
    if err!=nil{t.Fatal(err)}
    defer adapter.Close()
    pool:=adapter.Pool()
    merchant:=uuid.NewString()
    otherMerchant:=uuid.NewString()
    first:=uuid.NewString()
    second:=uuid.NewString()
    other:=uuid.NewString()
    session:=uuid.NewString()
    defer func() {
        clean:=context.Background()
        _,_=pool.Exec(clean,`DELETE FROM channel_provisioning_sessions WHERE business_id=$1::uuid`,merchant)
        _,_=pool.Exec(clean,`DELETE FROM channel_provider_brands WHERE business_id IN ($1::uuid,$2::uuid)`,merchant,otherMerchant)
        _,_=pool.Exec(clean,`DELETE FROM channel_connections WHERE business_id IN ($1::uuid,$2::uuid)`,merchant,otherMerchant)
        _,_=pool.Exec(clean,`DELETE FROM businesses WHERE id IN ($1::uuid,$2::uuid)`,merchant,otherMerchant)
    }()
    must:=func(sql string,args ...any) {
        t.Helper()
        if _,err:=pool.Exec(ctx,sql,args...);err!=nil{t.Fatalf("SQL: %v",err)}
    }
    must(`INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at)
        VALUES($1::uuid,'Cleanup Merchant',$1,'active','retail','Asia/Aden','YER','ar-YE',now(),now()),
              ($2::uuid,'Other Merchant',$2,'active','retail','Asia/Aden','YER','ar-YE',now(),now())`,merchant,otherMerchant)
    for _,row:=range []struct{business,conn,channel,status string}{
        {merchant,first,"facebook","active"},
        {merchant,second,"instagram","active"},
        {otherMerchant,other,"facebook","active"},
    } {
        must(`INSERT INTO channel_connections
            (id,business_id,provider_ref,channel,provider_connection_ref,provider_account_ref,status,secret_reference,created_at,updated_at)
            VALUES($1::uuid,$2::uuid,'socialapi',$3,$1,$1,$4,'local-ref',now(),now())`,row.conn,row.business,row.channel,row.status)
    }
    store:=NewProviderBrandRepository(adapter)
    a,err:=store.Create(ctx,ports.ProviderBrandRecord{BusinessID:merchant,ProviderRef:"socialapi",ProviderBrandRef:"brand-for-merchant",DisplayName:"Merchant"})
    if err!=nil{t.Fatal(err)}
    b,err:=store.Create(ctx,ports.ProviderBrandRecord{BusinessID:otherMerchant,ProviderRef:"socialapi",ProviderBrandRef:"brand-for-other-merchant",DisplayName:"Other"})
    if err!=nil{t.Fatal(err)}
    if a.LifecycleState!="active" || b.LifecycleState!="active" {t.Fatalf("new brand not active: %+v %+v",a,b)}
    if _,reserved,err:=store.ReserveUnused(ctx,merchant,"socialapi");err!=nil || reserved {t.Fatalf("deleted shared active brand: reserved=%v err=%v",reserved,err)}
    must(`UPDATE channel_connections SET status='disconnected' WHERE business_id=$1::uuid AND id=$2::uuid`,merchant,first)
    if _,reserved,err:=store.ReserveUnused(ctx,merchant,"socialapi");err!=nil || reserved {t.Fatalf("deleted brand while Instagram active: reserved=%v err=%v",reserved,err)}
    must(`UPDATE channel_connections SET status='disconnected' WHERE business_id=$1::uuid AND id=$2::uuid`,merchant,second)
    must(`INSERT INTO channel_provisioning_sessions
        (id,business_id,idempotency_key,provider_ref,channel,display_name,status,created_at,updated_at)
        VALUES($1::uuid,$2::uuid,'pending-key','socialapi','facebook','Pending account','pending_authorization',now(),now())`,session,merchant)
    if _,reserved,err:=store.ReserveUnused(ctx,merchant,"socialapi");err!=nil || reserved {t.Fatalf("deleted brand during live OAuth: reserved=%v err=%v",reserved,err)}
    must(`UPDATE channel_provisioning_sessions SET status='failed' WHERE business_id=$1::uuid AND id=$2::uuid`,merchant,session)
    brandRef,reserved,err:=store.ReserveUnused(ctx,merchant,"socialapi")
    if err!=nil||!reserved||brandRef!="brand-for-merchant" {t.Fatalf("last channel not reserved: %q %v %v",brandRef,reserved,err)}
    current,found,err:=store.Get(ctx,merchant,"socialapi")
    if err!=nil||!found||current.LifecycleState!="deleting" {t.Fatalf("deleting state not visible: %+v %v %v",current,found,err)}
    if err:=store.CompleteDeletion(ctx,merchant,"socialapi",brandRef);err!=nil{t.Fatal(err)}
    if _,found,err:=store.Get(ctx,merchant,"socialapi");err!=nil||found{t.Fatalf("local provider brand remains: found=%v err=%v",found,err)}
    stillThere,found,err:=store.Get(ctx,otherMerchant,"socialapi")
    if err!=nil||!found||stillThere.ProviderBrandRef!="brand-for-other-merchant" {
        t.Fatalf("other merchant affected: %+v found=%v err=%v",stillThere,found,err)
    }
}
