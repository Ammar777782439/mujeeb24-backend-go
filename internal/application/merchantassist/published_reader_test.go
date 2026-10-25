package merchantassist

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

var readNow = time.Date(2026, 10, 25, 18, 0, 0, 0, time.UTC)

type policyFixture struct {
	seenBusiness string
	calls int
	records []ports.BusinessPolicyRecord
	err error
}
func (p *policyFixture) ListPublished(_ context.Context, businessID, _ string, _ time.Time, _ int) ([]ports.BusinessPolicyRecord, error) {
	p.calls++
	p.seenBusiness=businessID
	return p.records,p.err
}

type knowledgeFixture struct {
	seenBusiness string
	calls int
	records []ports.KnowledgeDocumentRecord
	err error
}
func (k *knowledgeFixture) ListPublished(_ context.Context, businessID, _ string, _ time.Time, _ int) ([]ports.KnowledgeDocumentRecord, error) {
	k.calls++
	k.seenBusiness=businessID
	return k.records,k.err
}

type guardFixture struct {
	calls int
	business string
	err error
}
func (g *guardFixture) Check(_ context.Context, businessID string) error {
	g.calls++
	g.business=businessID
	return g.err
}

func readerActor() commands.ActorContext {
	return commands.ActorContext{
		BusinessID:"business-1",
		PrincipalID:"principal-1",
		Role:"member",
		Permissions: []string{"business:read","merchant_ai:use"},
	}
}

func readerInput(kind PublishedKind) PublishedReadRequest {
	return PublishedReadRequest{
		Actor:readerActor(),Business:"business-1",Kind:kind,
		Task:contracts.TaskExplain,Search:"delivery",Limit:2,
	}
}

func validPolicyRecord() ports.BusinessPolicyRecord {
	return ports.BusinessPolicyRecord{
		ID:"policy-1",BusinessID:"business-1",PolicyKey:"delivery",
		Category:"delivery",Title:"Delivery Policy",Summary:"Delivery within working hours.",
		Authority:"merchant",Status:"published",Version:3,
		ValidFrom:readNow.Add(-time.Hour),
	}
}

func validKnowledgeRecord() ports.KnowledgeDocumentRecord {
	return ports.KnowledgeDocumentRecord{
		ID:"knowledge-1",BusinessID:"business-1",KnowledgeKey:"delivery",
		Title:"Delivery Hours",Content:"Delivery is available on Sunday.",
		ContentType:"hours",Authority:"merchant",Status:"published",Version:2,
		ValidFrom:readNow.Add(-time.Hour),
	}
}

func TestPublishedReaderPolicyProvenanceAndNoRawRules(t *testing.T) {
	p:=&policyFixture{records:[]ports.BusinessPolicyRecord{validPolicyRecord()}}
	p.records[0].Rules=[]byte(`{"confidential_token":"must-not-leak"}`)
	g:=&guardFixture{}
	r:=&PublishedReader{Policies:p,AIGuard:g,Now:func()time.Time{return readNow}}
	result,err:=r.Read(context.Background(),readerInput(PublishedPolicies))
	if err!=nil {t.Fatal(err)}
	if len(result.Policies)!=1||len(result.Knowledge)!=0 {t.Fatalf("unexpected result: %+v",result)}
	item:=result.Policies[0]
	if item.Value.PolicyKey!="delivery" || item.Evidence.ResourceRef!="policy:policy-1" ||
		item.Evidence.Authority!=contracts.AuthorityPublishedPolicy ||
		item.Evidence.Completeness!=contracts.EvidenceComplete ||
		!item.Evidence.UsableAsCurrentFact(readNow) || *item.Evidence.ResourceVersion!="3" {
		t.Fatalf("missing validated policy provenance: %+v",item)
	}
	if p.calls!=1||p.seenBusiness!="business-1"||g.calls!=1||g.business!="business-1" {t.Fatal("wrong repository/guard scope")}
	// No sensitive SQL JSONB is in the serialized public projection.
	if strings.Contains(strings.TrimSpace(item.Value.Summary),"confidential_token") {t.Fatal("raw policy rules appeared in public contract")}
}

func TestPublishedReaderKnowledgeEvidence(t *testing.T) {
	k:=&knowledgeFixture{records:[]ports.KnowledgeDocumentRecord{validKnowledgeRecord()}}
	g:=&guardFixture{}
	r:=&PublishedReader{Knowledge:k,AIGuard:g,Now:func()time.Time{return readNow}}
	result,err:=r.Read(context.Background(),readerInput(PublishedKnowledge))
	if err!=nil{t.Fatal(err)}
	if len(result.Knowledge)!=1||len(result.Policies)!=0 {t.Fatalf("wrong evidence set: %+v",result)}
	item:=result.Knowledge[0]
	if item.Evidence.ResourceRef!="knowledge:knowledge-1" ||
		item.Evidence.Authority!=contracts.AuthorityMerchantRecord ||
		item.Value.Title!="Delivery Hours" ||
		!item.Evidence.UsableAsCurrentFact(readNow) {t.Fatalf("unexpected evidence: %+v",item)}
}

func TestPublishedReaderChecksTenantAndPermissionsBeforeAnyRead(t *testing.T) {
	denials:=[]struct{name string;edit func(*PublishedReadRequest)}{
		{"wrong business route",func(r *PublishedReadRequest){r.Business="business-2"}},
		{"missing principal",func(r *PublishedReadRequest){r.Actor.PrincipalID=""}},
		{"missing AI permission",func(r *PublishedReadRequest){r.Actor.Permissions=[]string{"business:read"}}},
		{"missing domain permission",func(r *PublishedReadRequest){r.Actor.Permissions=[]string{"merchant_ai:use"}}},
		{"missing tenant",func(r *PublishedReadRequest){r.Business=""}},
	}
	for _,tc:=range denials {
		t.Run(tc.name,func(t *testing.T){
			p:=&policyFixture{records:[]ports.BusinessPolicyRecord{validPolicyRecord()}}
			g:=&guardFixture{}
			r:=&PublishedReader{Policies:p,AIGuard:g,Now:func()time.Time{return readNow}}
			req:=readerInput(PublishedPolicies)
			tc.edit(&req)
			_,err:=r.Read(context.Background(),req)
			if !errors.Is(err,ErrReadDenied)||g.calls!=0||p.calls!=0{
				t.Fatalf("denied input read any data: err=%v guard=%d repo=%d",err,g.calls,p.calls)
			}
		})
	}
}

func TestPublishedReaderGuardAndSourceFailClosed(t *testing.T) {
	p:=&policyFixture{records:[]ports.BusinessPolicyRecord{validPolicyRecord()}}
	g:=&guardFixture{err:ports.ErrBusinessAIDisabled}
	r:=&PublishedReader{Policies:p,AIGuard:g,Now:func()time.Time{return readNow}}
	_,err:=r.Read(context.Background(),readerInput(PublishedPolicies))
	if !errors.Is(err,ErrReadDenied)||!errors.Is(err,ports.ErrBusinessAIDisabled)||p.calls!=0{
		t.Fatalf("AI opt-out bypass: %v",err)
	}
	g.err=nil
	for _,test:=range []struct{name string;edit func(*ports.BusinessPolicyRecord)}{
		{"different tenant",func(p *ports.BusinessPolicyRecord){p.BusinessID="business-2"}},
		{"draft",func(p *ports.BusinessPolicyRecord){p.Status="draft"}},
		{"future",func(p *ports.BusinessPolicyRecord){p.ValidFrom=readNow.Add(time.Hour)}},
		{"expired",func(p *ports.BusinessPolicyRecord){end:=readNow.Add(-time.Second);p.ValidUntil=&end}},
		{"missing id",func(p *ports.BusinessPolicyRecord){p.ID=""}},
		{"bad enum",func(p *ports.BusinessPolicyRecord){p.Category="unknown"}},
		{"oversized",func(p *ports.BusinessPolicyRecord){p.Summary=strings.Repeat("A",maxPublishedItemBytes)}},
	} {
		t.Run(test.name,func(t *testing.T){
			record:=validPolicyRecord()
			test.edit(&record)
			p.records=[]ports.BusinessPolicyRecord{record}
			result,err:=r.Read(context.Background(),readerInput(PublishedPolicies))
			if !errors.Is(err,ErrInvalidSourceEvidence)||len(result.Policies)!=0{
				t.Fatalf("invalid record returned: err=%v result=%+v",err,result)
			}
		})
	}
	p.records=make([]ports.BusinessPolicyRecord,3)
	for i:=range p.records {p.records[i]=validPolicyRecord()}
	if _,err:=r.Read(context.Background(),readerInput(PublishedPolicies));!errors.Is(err,ErrInvalidSourceEvidence) {
		t.Fatalf("repository exceeds authorized limit: %v",err)
	}
}

func TestPublishedReaderRejectsInvalidQueryAndOversizedKnowledge(t *testing.T) {
	k:=&knowledgeFixture{records:[]ports.KnowledgeDocumentRecord{validKnowledgeRecord()}}
	g:=&guardFixture{}
	r:=&PublishedReader{Knowledge:k,AIGuard:g,Now:func()time.Time{return readNow}}
	for _,req:=range []PublishedReadRequest{
		{Actor:readerActor(),Business:"business-1",Kind:PublishedKnowledge,Task:contracts.TaskExplain,Search:"",Limit:1},
		{Actor:readerActor(),Business:"business-1",Kind:PublishedKnowledge,Task:contracts.TaskExplain,Search:strings.Repeat("x",161),Limit:1},
		{Actor:readerActor(),Business:"business-1",Kind:PublishedKnowledge,Task:contracts.TaskPrepareFields,Search:"x",Limit:1},
		{Actor:readerActor(),Business:"business-1",Kind:PublishedKnowledge,Task:contracts.TaskExplain,Search:"x",Limit:6},
	} {
		_,err:=r.Read(context.Background(),req)
		if !errors.Is(err,ErrInvalidReadRequest)||g.calls!=0||k.calls!=0 {t.Fatalf("invalid read accessed repository: %v",err)}
	}
	k.records[0].Content=strings.Repeat("x",maxPublishedItemBytes)
	if result,err:=r.Read(context.Background(),readerInput(PublishedKnowledge));!errors.Is(err,ErrInvalidSourceEvidence)||len(result.Knowledge)>0 {
		t.Fatalf("oversized content must be rejected without truncation: %v",err)
	}
	k.records=[]ports.KnowledgeDocumentRecord{validKnowledgeRecord()}
	k.records[0].BusinessID="other"
	if _,err:=r.Read(context.Background(),readerInput(PublishedKnowledge));!errors.Is(err,ErrInvalidSourceEvidence) {
		t.Fatalf("cross-tenant knowledge returned: %v",err)
	}
}

func TestPublishedReaderCancellationAndMissingDependencies(t *testing.T) {
	r:=&PublishedReader{Policies:&policyFixture{},AIGuard:&guardFixture{},Now:func()time.Time{return readNow}}
	ctx,cancel:=context.WithCancel(context.Background())
	cancel()
	if _,err:=r.Read(ctx,readerInput(PublishedPolicies));!errors.Is(err,context.Canceled){t.Fatalf("context not propagated: %v",err)}
	r.AIGuard=nil
	if _,err:=r.Read(context.Background(),readerInput(PublishedPolicies));!errors.Is(err,ErrReadUnavailable){t.Fatalf("missing guard not closed: %v",err)}
}
