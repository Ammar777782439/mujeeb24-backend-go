package services

import (
    "context"
    "errors"
    "testing"

    "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type brandCleanupFixture struct {
    brandID string
    canDelete bool
    reserveCalls int
    completeCalls int
    err error
}
func (f *brandCleanupFixture) ReserveUnused(_ context.Context, businessID, providerRef string) (string,bool,error) {
    f.reserveCalls++
    if businessID != "merchant-one" || providerRef != "socialapi" {
        return "",false,errors.New("cross-tenant brand cleanup")
    }
    return f.brandID,f.canDelete,nil
}
func (f *brandCleanupFixture) CompleteDeletion(_ context.Context,businessID,providerRef,brandID string) error {
    f.completeCalls++
    if businessID != "merchant-one" || providerRef != "socialapi" || brandID != f.brandID {
        return errors.New("wrong brand deleted")
    }
    return nil
}
type failingBrandProvider struct { calls int; err error }
func (f *failingBrandProvider) ListBrands(context.Context) ([]ports.ProviderBrandRecord,error) {return nil,nil}
func (f *failingBrandProvider) CreateBrand(context.Context,string) (ports.ProviderBrandRecord,error) {return ports.ProviderBrandRecord{},nil}
func (f *failingBrandProvider) DeleteBrand(context.Context,string) error {f.calls++;return f.err}

func TestDisconnectBrandCleanupSkipsSharedAndNonSocialProviders(t *testing.T) {
    store:=&brandCleanupFixture{brandID:"brand-one",canDelete:false}
    remote:=&failingBrandProvider{}
    service:=ChannelRuntimeService{BrandCleanup:store,BrandProvider:remote}
    if err:=service.deleteProviderBrandIfUnused(context.Background(),"merchant-one","socialapi");err!=nil {t.Fatal(err)}
    if store.reserveCalls!=1 || remote.calls!=0 || store.completeCalls!=0 {t.Fatalf("shared brand deleted: %#v %#v",store,remote)}
    if err:=service.deleteProviderBrandIfUnused(context.Background(),"merchant-one","other");err!=nil {t.Fatal(err)}
    if store.reserveCalls!=1 {t.Fatalf("unexpected provider cleanup")}
}
func TestDisconnectBrandCleanupDeletesAndRemovesMappingOnlyAfterProviderSuccess(t *testing.T) {
    store:=&brandCleanupFixture{brandID:"brand-one",canDelete:true}
    remote:=&failingBrandProvider{err:errors.New("temporary provider failure")}
    service:=ChannelRuntimeService{BrandCleanup:store,BrandProvider:remote}
    if err:=service.deleteProviderBrandIfUnused(context.Background(),"merchant-one","socialapi");err==nil {
        t.Fatal("remote error swallowed")
    }
    if store.completeCalls!=0 {t.Fatal("mapping removed despite failed remote deletion")}
    remote.err=nil
    if err:=service.deleteProviderBrandIfUnused(context.Background(),"merchant-one","socialapi");err!=nil {t.Fatal(err)}
    if remote.calls!=2 || store.completeCalls!=1 {t.Fatalf("cleanup retry did not complete: %#v %#v",remote,store)}
}
