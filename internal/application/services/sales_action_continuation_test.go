package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/aierrors"
)

type salesContinuationQuotaStub struct{ calls int }
func(q *salesContinuationQuotaStub) IsAIExecutionAllowed(context.Context,string)(bool,string) {
	q.calls++
	return false,"ai_replies_exhausted"
}

type salesContinuationJobStub struct{ continuation ports.SalesStartContinuation }
func (s *salesContinuationJobStub) Claim(context.Context,int,time.Time)([]ports.SalesActionJob,error){return nil,nil}
func (s *salesContinuationJobStub) Finish(context.Context,string,string,string,time.Time)error{return nil}
func (s *salesContinuationJobStub) LoadContinuation(context.Context,ports.SalesActionJob)(ports.SalesStartContinuation,error){
	return s.continuation,nil
}
func (s *salesContinuationJobStub) CompleteContinuation(context.Context,ports.SalesActionJob,string)error{
	return errors.New("not used")
}
type salesContinuationAIStub struct{ calls int }
func(s *salesContinuationAIStub) Handle(context.Context,commands.AutoReplyCommand)(commands.AutoReplyResult,error){
	s.calls++
	return commands.AutoReplyResult{},nil
}

func TestSalesContinuationHonorsCostProtectionBeforeModelCall(t *testing.T){
	jobs:=&salesContinuationJobStub{continuation:ports.SalesStartContinuation{
		RequestID:"start",WorkflowID:"workflow",BusinessID:"business",CustomerID:"customer",
		ConversationID:"conversation",SourceMessageID:"original",Text:"السماعة السوداء",
		Channel:"instagram",ProviderRef:"socialapi",
	}}
	ai:=&salesContinuationAIStub{}
	quota:=&salesContinuationQuotaStub{}
	processor:=SalesActionProcessor{Jobs:jobs,AutoReply:ai,CostProtection:quota}
	job:=ports.SalesActionJob{ID:"job",LeaseToken:"lease",Phase:"continue_ai"}
	err:=processor.continueSales(context.Background(),job)
	if err==nil || ai.calls!=0 || quota.calls!=1 || retryableSalesContinuation(err){
		t.Fatalf("AI ran despite exceeded entitlement: err=%v ai_calls=%d quota_calls=%d",err,ai.calls,quota.calls)
	}
}

func TestSalesContinuationRetryClassification(t *testing.T){
	for _,tt:=range []struct{name string;err error;retry bool}{
		{"context_deadline",context.DeadlineExceeded,true},
		{"network_provider",aierrors.Wrap(aierrors.Application,errors.New("temporary AI gateway")),true},
		{"invalid_model_output",aierrors.Wrap(aierrors.ModelOutput,errors.New("invalid tool")),false},
		{"merchant_policy",appErrors.New(appErrors.CodeForbidden,"disabled"),false},
		{"contract_input",appErrors.New(appErrors.CodeValidation,"missing"),false},
		{"explicit_permanent",&permanentContinuationError{reason:"quota exhausted"},false},
	}{
		t.Run(tt.name,func(t *testing.T){
			if got:=retryableSalesContinuation(tt.err);got!=tt.retry{
				t.Fatalf("retryable(%v)=%v, want %v",tt.err,got,tt.retry)
			}
		})
	}
}
