package ports

import "testing"

func TestSalesStartContinuationLimitsFirstResponseToConversation(t *testing.T){
	caps:=CustomerSalesCapabilitiesForStartContinuation(true)
	for _,bad:=range []CustomerSalesCapability{CustomerSalesCapabilityOfferSalesStart,CustomerSalesCapabilityRecordLead,CustomerSalesCapabilityCreateDraft,CustomerSalesCapabilityUpdateDraft}{
		for _,got:=range caps {if got==bad{t.Fatalf("unsafe continuation capability: %s",bad)}}
	}
	want:=map[CustomerSalesCapability]bool{
		CustomerSalesCapabilityAsk:false,
		CustomerSalesCapabilityRespondWithoutAction:false,
		CustomerSalesCapabilityHandoff:false,
		CustomerSalesCapabilityFullCatalog:false,
	}
	for _,c:=range caps {if _,ok:=want[c];ok{want[c]=true}}
	for c,seen:=range want {if !seen{t.Fatalf("missing first continuation tool %s",c)}}
}
