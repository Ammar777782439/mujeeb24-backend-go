package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

type fakeProvider struct {
	name string
	mu sync.Mutex
	n int
	err error
}
func (p *fakeProvider) Name() string {return p.name}
func (p *fakeProvider) Generate(_ context.Context, _ Request) (Response,error) {
	p.mu.Lock(); defer p.mu.Unlock()
	p.n++
	if p.err != nil { return Response{},p.err }
	return Response{Payload:json.RawMessage(`{"ok":true}`),Model:p.name},nil
}
func (p *fakeProvider) count() int {p.mu.Lock();defer p.mu.Unlock();return p.n}
func mkRouter(t *testing.T, a,b *fakeProvider, threshold int) *Router {
	t.Helper()
	r,err:=NewRouter(a,b,Policy{ConsecutiveFailures:threshold,Cooldown:time.Minute})
	if err != nil {t.Fatal(err)}
	return r
}
func validRequest() Request {return Request{Payload:json.RawMessage(`{"messages":[]}`),Operation:"customer"}}

func TestPrimaryPreferredAndFallbackOn503Only(t *testing.T) {
	a,b:=&fakeProvider{name:"gemini"},&fakeProvider{name:"deepseek"}
	r:=mkRouter(t,a,b,3)
	if _,trace,err:=r.Generate(context.Background(),validRequest());err!=nil||trace.FallbackUsed||b.count()!=0 {t.Fatalf("primary path: %+v %v",trace,err)}
	a.err=&StatusError{Provider:"gemini",Code:http.StatusServiceUnavailable}
	if _,trace,err:=r.Generate(context.Background(),validRequest());err!=nil||!trace.FallbackUsed||len(trace.Attempts)!=2||trace.Attempts[0].HTTPStatus!=503||b.count()!=1 {t.Fatalf("503 fallback: %+v %v",trace,err)}
}

func TestNoFallbackOnValidationOrOtherStatuses(t *testing.T){
	for _,errCase:=range []error{errors.New("invalid schema"),&StatusError{Provider:"gemini",Code:400},&StatusError{Provider:"gemini",Code:429}} {
		a,b:=&fakeProvider{name:"gemini",err:errCase},&fakeProvider{name:"deepseek"}
		r:=mkRouter(t,a,b,1)
		if _,trace,err:=r.Generate(context.Background(),validRequest());err==nil||trace.FallbackUsed||b.count()!=0 {t.Fatalf("unsafe fallback: %+v %v",trace,err)}
	}
}

func TestCircuitBreakerSkipsUnavailablePrimaryAndRecovers(t *testing.T){
	a,b:=&fakeProvider{name:"gemini",err:&StatusError{Provider:"gemini",Code:503}},&fakeProvider{name:"deepseek"}
	r:=mkRouter(t,a,b,2)
	now:=time.Now()
	r.now=func()time.Time{return now}
	for i:=0;i<4;i++ {
		if _,_,err:=r.Generate(context.Background(),validRequest());err!=nil {t.Fatal(err)}
	}
	if a.count()!=2||b.count()!=4 {t.Fatalf("breaker failed primary=%d standby=%d",a.count(),b.count())}
	a.mu.Lock();a.err=nil;a.mu.Unlock()
	now=now.Add(2*time.Minute)
	_,trace,err:=r.Generate(context.Background(),validRequest())
	if err!=nil||trace.FallbackUsed||a.count()!=3 {t.Fatalf("recovery failed: %+v %v",trace,err)}
}

func TestNoProviderCalledForCanceledOrInvalidRequest(t *testing.T){
	a,b:=&fakeProvider{name:"gemini"},&fakeProvider{name:"deepseek"}
	r:=mkRouter(t,a,b,1)
	ctx,cancel:=context.WithCancel(context.Background());cancel()
	if _,_,err:=r.Generate(ctx,validRequest());!errors.Is(err,context.Canceled){t.Fatalf("cancel: %v",err)}
	if _,_,err:=r.Generate(context.Background(),Request{Payload:[]byte("{")});err==nil {t.Fatal("invalid payload accepted")}
	if a.count()!=0||b.count()!=0 {t.Fatal("outbound provider contacted for invalid request")}
}

func TestFailedStandbyCannotBeMistakenForSuccess(t *testing.T){
	a:=&fakeProvider{name:"gemini",err:&StatusError{Provider:"gemini",Code:503}}
	b:=&fakeProvider{name:"deepseek",err:&StatusError{Provider:"deepseek",Code:502}}
	r:=mkRouter(t,a,b,1)
	res,trace,err:=r.Generate(context.Background(),validRequest())
	if err==nil||len(res.Payload)>0||!trace.FallbackUsed||len(trace.Attempts)!=2||!trace.Attempts[1].Failed {t.Fatalf("standby failure: %+v %+v %v",res,trace,err)}
}
