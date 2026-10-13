package services

import (
	"context"
	"errors"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type fakeCustomerDecisionProvider struct {
	calls int
	last ports.CustomerSalesDecisionInput
	output ports.CustomerSalesDecisionOutput
	err error
}
func (f *fakeCustomerDecisionProvider) Decide(_ context.Context, input ports.CustomerSalesDecisionInput) (ports.CustomerSalesDecisionOutput, error) {
	f.calls++
	f.last = input
	return f.output, f.err
}

type retryableProviderError bool
func (e retryableProviderError) Error() string { return "provider failed" }
func (e retryableProviderError) Retryable() bool { return bool(e) }

func TestCustomerSalesFailoverTransientOnly(t *testing.T) {
	tests := []struct{
		name string
		primaryErr error
		wantSecondary bool
	}{
		{"503", retryableProviderError(true), true},
		{"400", retryableProviderError(false), false},
		{"schema", errors.New("invalid capability arguments"), false},
		{"canceled", context.Canceled, false},
		{"healthy", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name,func(t *testing.T){
			primary:=&fakeCustomerDecisionProvider{err:tc.primaryErr}
			secondary:=&fakeCustomerDecisionProvider{output:ports.CustomerSalesDecisionOutput{
				Decision:ports.CapabilityDecision{Capability:ports.CustomerSalesCapabilityAsk, ResponseText:"كم العدد؟"},
			}}
			port:=FailoverCustomerSalesDecisionPort{Primary:primary,Fallback:secondary}
			input:=ports.CustomerSalesDecisionInput{
				Request:ports.CustomerSalesDecisionRequest{Text:"أريد الاشتراك"},
				Capabilities:[]ports.CustomerSalesCapability{ports.CustomerSalesCapabilityAsk},
			}
			out,err:=port.Decide(context.Background(),input)
			if primary.calls!=1 {t.Fatalf("primary called %d times",primary.calls)}
			if (secondary.calls==1)!=tc.wantSecondary {t.Fatalf("unexpected fallback calls %d",secondary.calls)}
			if tc.wantSecondary {
				if err!=nil || out.Decision.Capability!=ports.CustomerSalesCapabilityAsk || secondary.last.Request.Text!=input.Request.Text ||
					len(secondary.last.Capabilities)!=1 {t.Fatalf("fallback lost request or outcome: %+v %v",out,err)}
			} else if tc.primaryErr!=nil && !errors.Is(err,tc.primaryErr) {
				t.Fatalf("unexpected primary error: %v",err)
			}
		})
	}
}

func TestCustomerSalesFailoverNeverRetriesCanceledContext(t *testing.T) {
	primary:=&fakeCustomerDecisionProvider{}
	fallback:=&fakeCustomerDecisionProvider{}
	ctx,cancel:=context.WithCancel(context.Background());cancel()
	_,err:= (FailoverCustomerSalesDecisionPort{Primary:primary,Fallback:fallback}).Decide(ctx,ports.CustomerSalesDecisionInput{})
	if !errors.Is(err,context.Canceled)||primary.calls!=0||fallback.calls!=0{
		t.Fatalf("cancellation not respected: %v",err)
	}
}
