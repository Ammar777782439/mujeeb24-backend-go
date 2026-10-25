package merchantassist

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

type customerReadStub struct {
	calls int
	business string
	result ports.CustomerRecord
	err error
}
func (s *customerReadStub) GetByID(_ context.Context, business, _ string) (ports.CustomerRecord,error) {
	s.calls++;s.business=business
	return s.result,s.err
}
type conversationReadStub struct {
	calls int
	business string
	result ports.ConversationRecord
	err error
}
func (s *conversationReadStub) GetByID(_ context.Context,business,_ string)(ports.ConversationRecord,error){
	s.calls++;s.business=business
	return s.result,s.err
}
type leadReadStub struct {
	calls int
	business string
	result ports.LeadRecord
	err error
}
func (s *leadReadStub) Get(_ context.Context,business,_ string)(ports.LeadRecord,error){
	s.calls++;s.business=business
	return s.result,s.err
}
type transactionReadStub struct {
	calls int
	business string
	result ports.TransactionRecord
	err error
}
func (s *transactionReadStub) Get(_ context.Context,business,_ string)(ports.TransactionRecord,error){
	s.calls++;s.business=business
	return s.result,s.err
}

func recordActor() commands.ActorContext {
	return commands.ActorContext{
		PrincipalID:"principal-1",BusinessID:"business-1",
		Role:"member",Permissions:[]string{"merchant_ai:use","customers:read","conversations:read","sales:read"},
	}
}
func recordRequest(kind RecordKind) RecordReadRequest {
	return RecordReadRequest{Actor:recordActor(),Business:"business-1",Kind:kind,RecordID:"record-1",Task:contracts.TaskExplain}
}
func customerRecord() ports.CustomerRecord {
	return ports.CustomerRecord{ID:"record-1",BusinessID:"business-1",Status:"active",ResourceVersion:2,
		Profile:[]byte(`{"notes":"private profile secret"}`),
		ContactPoints:[]byte(`{"phone":"sensitive-phone"}`)}
}
func conversationRecord() ports.ConversationRecord {
	return ports.ConversationRecord{ID:"record-1",BusinessID:"business-1",CustomerID:"customer-1",
		State:"waiting_human",Ownership:"human",Priority:"normal",ResourceVersion:3,
		LastActivityAt:readNow,AssignmentReference:ptrString("private-assignment")}
}
func leadRecord() ports.LeadRecord {
	return ports.LeadRecord{ID:"record-1",BusinessID:"business-1",CustomerID:"customer-1",
		Status:"qualified",QualificationState:"qualified",ResourceVersion:4,
		QualificationContext:[]byte(`{"personal_data":"private qualification"}`)}
}
func transactionRecord() ports.TransactionRecord {
	currency, amount := "YER","5000.0000"
	return ports.TransactionRecord{ID:"record-1",BusinessID:"business-1",CustomerID:"customer-1",
		TransactionType:"order",State:"awaiting_confirmation",ResourceVersion:5,
		RequiresHumanReview:true,Currency:&currency,TotalAmount:&amount,
		Lines:[]ports.TransactionLineRecord{{ItemNameSnapshot:"never-publish-line",SelectedAttributesSnapshot:[]byte(`{"private":"line-json"}`)}}}
}
func ptrString(s string)*string{return &s}

func makeRecordReader()(*RecordReader,*guardFixture,*customerReadStub,*conversationReadStub,*leadReadStub,*transactionReadStub){
	g:=&guardFixture{}
	c:=&customerReadStub{result:customerRecord()}
	v:=&conversationReadStub{result:conversationRecord()}
	l:=&leadReadStub{result:leadRecord()}
	tx:=&transactionReadStub{result:transactionRecord()}
	r:=&RecordReader{Customers:c,Conversations:v,Leads:l,Transactions:tx,AIGuard:g,Now:func()time.Time{return readNow}}
	return r,g,c,v,l,tx
}
func allReads(c *customerReadStub,v *conversationReadStub,l *leadReadStub,tx *transactionReadStub)int{
	return c.calls+v.calls+l.calls+tx.calls
}

func TestRecordReaderTypedEvidenceAndPIIAllowlist(t *testing.T){
	cases:=[]struct{kind RecordKind;src contracts.EvidenceSourceType;version string;forbidden string}{
		{RecordCustomer,contracts.SourceCustomer,"2","sensitive-phone"},
		{RecordConversation,contracts.SourceConversation,"3","private-assignment"},
		{RecordLead,contracts.SourceLead,"4","private qualification"},
		{RecordTransaction,contracts.SourceTransaction,"5","line-json"},
	}
	for _,tc:=range cases{
		t.Run(string(tc.kind),func(t *testing.T){
			r,g,c,v,l,tx:=makeRecordReader()
			out,err:=r.Read(context.Background(),recordRequest(tc.kind))
			if err!=nil{t.Fatal(err)}
			if g.calls!=1 || allReads(c,v,l,tx)!=1 {t.Fatal("unexpected tenant repository/guard calls")}
			if out.Evidence.SourceType!=tc.src || out.Evidence.ResourceVersion==nil ||
				*out.Evidence.ResourceVersion!=tc.version ||
				out.Evidence.Completeness!=contracts.EvidencePartial ||
				out.Evidence.ValidUntil==nil ||
				!out.Evidence.ValidUntil.After(readNow) {t.Fatalf("unsafe provenance: %+v",out.Evidence)}
			if err:=out.Evidence.Validate(readNow);err!=nil{t.Fatal(err)}
			data,err:=json.Marshal(out);if err!=nil{t.Fatal(err)}
			if strings.Contains(string(data),tc.forbidden) || strings.Contains(string(data),"private profile secret") ||
				strings.Contains(string(data),"never-publish-line") {
				t.Fatalf("sensitive source fields leaked for %s: %s",tc.kind,data)
			}
		})
	}
}

func TestRecordReaderRejectsPermissionsAndTenantBeforeRepository(t *testing.T){
	cases:=[]struct{name string;kind RecordKind;modify func(*RecordReadRequest)}{
		{"cross-business",RecordCustomer,func(r *RecordReadRequest){r.Business="business-2"}},
		{"missing principal",RecordCustomer,func(r *RecordReadRequest){r.Actor.PrincipalID=""}},
		{"missing AI permission",RecordCustomer,func(r *RecordReadRequest){r.Actor.Permissions=[]string{"customers:read"}}},
		{"missing customers read",RecordCustomer,func(r *RecordReadRequest){r.Actor.Permissions=[]string{"merchant_ai:use"}}},
		{"missing conversations read",RecordConversation,func(r *RecordReadRequest){r.Actor.Permissions=[]string{"merchant_ai:use"}}},
		{"missing sales read",RecordLead,func(r *RecordReadRequest){r.Actor.Permissions=[]string{"merchant_ai:use"}}},
		{"transaction requires sales read",RecordTransaction,func(r *RecordReadRequest){r.Actor.Permissions=[]string{"merchant_ai:use"}}},
	}
	for _,tc:=range cases {
		t.Run(tc.name,func(t *testing.T){
			r,g,c,v,l,tx:=makeRecordReader()
			req:=recordRequest(tc.kind);tc.modify(&req)
			_,err:=r.Read(context.Background(),req)
			if !errors.Is(err,ErrReadDenied)||g.calls!=0||allReads(c,v,l,tx)!=0 {
				t.Fatalf("unauthorized read: err=%v guard=%d",err,g.calls)
			}
		})
	}
}

func TestRecordReaderChecksReturnedTenantIDAndVersion(t *testing.T){
	for _,kind:=range []RecordKind{RecordCustomer,RecordConversation,RecordLead,RecordTransaction}{
		t.Run(string(kind),func(t *testing.T){
			for _,mutate:=range []string{"tenant","id","version"}{
				r,_,c,v,l,tx:=makeRecordReader()
				switch kind{
				case RecordCustomer:
					if mutate=="tenant"{c.result.BusinessID="other"}
					if mutate=="id"{c.result.ID="other"}
					if mutate=="version"{c.result.ResourceVersion=0}
				case RecordConversation:
					if mutate=="tenant"{v.result.BusinessID="other"}
					if mutate=="id"{v.result.ID="other"}
					if mutate=="version"{v.result.ResourceVersion=0}
				case RecordLead:
					if mutate=="tenant"{l.result.BusinessID="other"}
					if mutate=="id"{l.result.ID="other"}
					if mutate=="version"{l.result.ResourceVersion=0}
				case RecordTransaction:
					if mutate=="tenant"{tx.result.BusinessID="other"}
					if mutate=="id"{tx.result.ID="other"}
					if mutate=="version"{tx.result.ResourceVersion=0}
				}
				res,err:=r.Read(context.Background(),recordRequest(kind))
				if !errors.Is(err,ErrInvalidSourceEvidence)||res.Customer!=nil||res.Conversation!=nil||res.Lead!=nil||res.Transaction!=nil{
					t.Fatalf("invalid %s %s returned: err=%v",kind,mutate,err)
				}
			}
		})
	}
}

func TestRecordReaderPolicyDisabledAndMalformedInput(t *testing.T){
	r,g,c,v,l,tx:=makeRecordReader()
	g.err=ports.ErrBusinessAIDisabled
	if _,err:=r.Read(context.Background(),recordRequest(RecordCustomer));!errors.Is(err,ports.ErrBusinessAIDisabled)||allReads(c,v,l,tx)!=0 {
		t.Fatalf("merchant AI opt-out ignored: %v",err)
	}
	g.err=nil
	for _,modify:=range []func(*RecordReadRequest){
		func(x *RecordReadRequest){x.Task=contracts.TaskPrepareFields},
		func(x *RecordReadRequest){x.RecordID=""},
		func(x *RecordReadRequest){x.RecordID=" some-id"},
		func(x *RecordReadRequest){x.RecordID=strings.Repeat("a",129)},
		func(x *RecordReadRequest){x.Kind=RecordKind("query_any_table")},
	}{
		q:=recordRequest(RecordCustomer);modify(&q)
		if _,err:=r.Read(context.Background(),q);!errors.Is(err,ErrInvalidReadRequest){
			t.Fatalf("invalid record read allowed: %+v error %v",q,err)
		}
	}
}

func TestRecordReaderCancelledAndInvalidCommercialFact(t *testing.T){
	r,_,_,_,_,tx:=makeRecordReader()
	ctx,cancel:=context.WithCancel(context.Background());cancel()
	if _,err:=r.Read(ctx,recordRequest(RecordTransaction));!errors.Is(err,context.Canceled){t.Fatal(err)}
	tx.result.TotalAmount=ptrString("not-a-price")
	out,err:=r.Read(context.Background(),recordRequest(RecordTransaction))
	if !errors.Is(err,ErrInvalidSourceEvidence)||out.Transaction!=nil {t.Fatalf("bad money accepted: %v",err)}
}
