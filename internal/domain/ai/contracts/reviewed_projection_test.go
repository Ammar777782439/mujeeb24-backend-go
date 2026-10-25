package contracts

import (
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "reflect"
 "sort"
 "strings"
 "testing"
 "time"
)

func TestReviewedProjectionsFailClosed(t *testing.T) {
 now:=time.Date(2026,10,25,12,0,0,0,time.UTC)
 p:=PublishedPolicyAssistV1{PolicyKey:"returns",Category:"returns",Title:"Returns",Summary:"Policy summary",Authority:"merchant",Status:"published",Version:1,ValidFrom:now.Add(-time.Hour)}
 if err:=p.Validate(now);err!=nil{t.Fatal(err)}
 p.Status="draft";if !errors.Is(p.Validate(now),ErrInvalidReviewedProjection){t.Fatal("draft policy")}
 p.Status="published";expires:=now.Add(-time.Second);p.ValidUntil=&expires
 if !errors.Is(p.Validate(now),ErrInvalidReviewedProjection){t.Fatal("expired policy")}
 k:=PublishedKnowledgeAssistV1{KnowledgeKey:"hours",Title:"Hours",Content:"Mon-Fri",ContentType:"hours",Authority:"system",Status:"published",Version:1,ValidFrom:now.Add(-time.Hour)}
 if err:=k.Validate(now);err!=nil{t.Fatal(err)}
 k.Authority="ai";if !errors.Is(k.Validate(now),ErrInvalidReviewedProjection){t.Fatal("invented authority")}
 c:=CustomerAssistV1{CustomerRef:"opaque-id",Status:"active"}
 if err:=c.Validate();err!=nil{t.Fatal(err)}
 c.Status="banned";if !errors.Is(c.Validate(),ErrInvalidReviewedProjection){t.Fatal("invented customer state")}
 conv:=ConversationAssistV1{ConversationRef:"conv",CustomerRef:"customer",State:"waiting_human",Ownership:"human",Priority:"urgent",LastActivityAt:now}
 if err:=conv.Validate();err!=nil{t.Fatal(err)}
 conv.State="open_ai";if !errors.Is(conv.Validate(),ErrInvalidReviewedProjection){t.Fatal("invented conversation state")}
 msg:=PublicTextMessageAssistV1{MessageRef:"msg",Direction:"inbound",Origin:"customer",Visibility:"public",ContentType:"text",Text:"Hello",OccurredAt:now}
 if err:=msg.Validate();err!=nil{t.Fatal(err)}
 msg.Visibility="private";if !errors.Is(msg.Validate(),ErrInvalidReviewedProjection){t.Fatal("private notes disclosed")}
 sum:=ConversationSummaryAssistV1{Summary:"Customer asked about pricing",LastCoveredMessageRef:"msg",CoveredTurnCount:2}
 if err:=sum.Validate();err!=nil{t.Fatal(err)}
 sum.LastCoveredMessageRef="";if !errors.Is(sum.Validate(),ErrInvalidReviewedProjection){t.Fatal("summary has no checkpoint")}
 reply:=CannedReplyAssistV1{Title:"Welcome",Shortcut:"welcome",Body:"Hello",Status:"active"}
 if err:=reply.Validate();err!=nil{t.Fatal(err)}
 reply.Shortcut="Welcome";if !errors.Is(reply.Validate(),ErrInvalidReviewedProjection){t.Fatal("invalid shortcut")}
 score:="89.25";lead:=LeadAssistV1{LeadRef:"lead",CustomerRef:"customer",Status:"qualified",QualificationState:"qualified",CurrentScoreValue:&score}
 if err:=lead.Validate();err!=nil{t.Fatal(err)}
 score="100.01";if !errors.Is(lead.Validate(),ErrInvalidReviewedProjection){t.Fatal("score beyond 100")}
 amount,currency:="12.5000","YER"
 tx:=TransactionAssistV1{TransactionRef:"tx",CustomerRef:"customer",TransactionType:"order",State:"draft",Currency:&currency,TotalAmount:&amount}
 if err:=tx.Validate();err!=nil{t.Fatal(err)}
 tx.Currency=nil;if !errors.Is(tx.Validate(),ErrInvalidReviewedProjection){t.Fatal("price without currency")}
 line:=TransactionLineAssistV1{ItemRef:"item",NameSnapshot:"Product",Quantity:"1.2500",Currency:&currency,UnitPriceSnapshot:&amount}
 if err:=line.Validate();err!=nil{t.Fatal(err)}
 line.Quantity="0.0000";if !errors.Is(line.Validate(),ErrInvalidReviewedProjection){t.Fatal("zero quantity")}
 if err:=(TransactionReviewAssistV1{Status:"pending",Required:true}).Validate();err!=nil{t.Fatal(err)}
 if err:=(TransactionReviewAssistV1{Status:"bypassed",Required:true}).Validate();!errors.Is(err,ErrInvalidReviewedProjection){t.Fatal("bypass required review")}
 if err:=(TransactionConfirmationAssistV1{Status:"confirmed"}).Validate();!errors.Is(err,ErrInvalidReviewedProjection){t.Fatal("confirmed without timestamp")}
 wf:=SalesWorkflowAssistV1{WorkflowRef:"workflow",CustomerRef:"customer",State:"awaiting_review"}
 if err:=wf.Validate();err!=nil{t.Fatal(err)}
 wf.State="paid";if !errors.Is(wf.Validate(),ErrInvalidReviewedProjection){t.Fatal("invented workflow status")}
 if err:=(SalesRequirementHeaderAssistV1{TransactionType:"subscription",SchemaVersion:1}).Validate();err!=nil{t.Fatal(err)}
}

func TestReviewedProjectionsMatchPostgresAndFieldAllowlist(t *testing.T) {
 type field struct{ sql string; typ string }
 cases:=[]struct{table string; value any; columns map[string]field}{
 {"business_policy_versions",PublishedPolicyAssistV1{},map[string]field{"policy_key":{"policy_key","text"},"category":{"category","text"},"title":{"title","text"},"summary":{"summary","text"},"authority":{"authority","text"},"status":{"status","text"},"version":{"version","integer"},"valid_from":{"valid_from","timestamp with time zone"},"valid_until":{"valid_until","timestamp with time zone"}}},
 {"knowledge_documents",PublishedKnowledgeAssistV1{},map[string]field{"knowledge_key":{"knowledge_key","text"},"title":{"title","text"},"content":{"content","text"},"content_type":{"content_type","text"},"authority":{"authority","text"},"status":{"status","text"},"version":{"version","integer"},"valid_from":{"valid_from","timestamp with time zone"},"valid_until":{"valid_until","timestamp with time zone"}}},
 {"customers",CustomerAssistV1{},map[string]field{"customer_ref":{"id","uuid"},"locale_preference":{"locale_preference","text"},"status":{"status","text"}}},
 {"conversations",ConversationAssistV1{},map[string]field{"conversation_ref":{"id","uuid"},"customer_ref":{"customer_id","uuid"},"state":{"state","text"},"ownership":{"ownership","text"},"priority":{"priority","text"},"ai_mode_override":{"ai_mode_override","text"},"last_activity_at":{"last_activity_at","timestamp with time zone"}}},
 {"communication_messages",PublicTextMessageAssistV1{},map[string]field{"message_ref":{"id","uuid"},"direction":{"direction","text"},"origin":{"origin","text"},"visibility":{"visibility","text"},"content_type":{"content_type","text"},"text":{"text_content","text"},"occurred_at":{"occurred_at","timestamp with time zone"}}},
 {"conversation_state",ConversationSummaryAssistV1{},map[string]field{"summary":{"summary","text"},"last_covered_message_ref":{"summary_last_message_id","uuid"},"covered_turn_count":{"summary_turn_count","integer"}}},
 {"canned_replies",CannedReplyAssistV1{},map[string]field{"title":{"title","text"},"shortcut":{"shortcut","text"},"body":{"body","text"},"status":{"status","text"}}},
 {"leads",LeadAssistV1{},map[string]field{"lead_ref":{"id","uuid"},"customer_ref":{"customer_id","uuid"},"status":{"status","text"},"qualification_state":{"qualification_state","text"},"current_score_band":{"current_score_band","text"},"current_score_value":{"current_score_value","numeric(5,2)"},"next_action_at":{"next_action_at","timestamp with time zone"}}},
 {"commercial_transactions",TransactionAssistV1{},map[string]field{"transaction_ref":{"id","uuid"},"customer_ref":{"customer_id","uuid"},"transaction_type":{"transaction_type","text"},"state":{"state","text"},"requires_human_review":{"requires_human_review","boolean"},"currency":{"currency","character(3)"},"total_amount":{"total_amount","numeric(20,4)"}}},
 {"order_lines",TransactionLineAssistV1{},map[string]field{"item_ref":{"catalog_item_id","uuid"},"name_snapshot":{"item_name_snapshot","text"},"quantity":{"quantity","numeric(20,4)"},"unit_price_snapshot":{"unit_price_snapshot","numeric(20,4)"},"line_total_snapshot":{"line_total_snapshot","numeric(20,4)"},"currency":{"currency","character(3)"}}},
 {"transaction_reviews",TransactionReviewAssistV1{},map[string]field{"required":{"required","boolean"},"status":{"status","text"}}},
 {"transaction_confirmations",TransactionConfirmationAssistV1{},map[string]field{"status":{"status","text"},"confirmed_at":{"confirmed_at","timestamp with time zone"}}},
 {"sales_workflows",SalesWorkflowAssistV1{},map[string]field{"workflow_ref":{"id","uuid"},"customer_ref":{"customer_id","uuid"},"state":{"state","text"},"transaction_ref":{"transaction_id","uuid"},"lead_ref":{"lead_id","uuid"}}},
 {"sales_transaction_requirements",SalesRequirementHeaderAssistV1{},map[string]field{"transaction_type":{"transaction_type","text"},"schema_version":{"schema_version","integer"}}},
 }
 raw,err:=os.ReadFile(filepath.Join("..","..","..","platform","database","testdata","merchant_assist_schema_pg16.json"));if err!=nil{t.Fatal(err)}
 var snapshot struct {Columns []struct {Table,Name,Type string} `json:"columns"`}
 if err:=json.Unmarshal(raw,&snapshot);err!=nil{t.Fatal(err)}
 types:=map[string]string{};for _,c:=range snapshot.Columns{types[c.Table+"."+c.Name]=c.Type}
 for _,c:=range cases{
  t.Run(c.table,func(t *testing.T){
   typ:=reflect.TypeOf(c.value)
   have:=make([]string,0,typ.NumField())
   for i:=0;i<typ.NumField();i++{
    tag:=strings.Split(typ.Field(i).Tag.Get("json"),",")[0]
    if tag==""||tag=="-"{t.Fatalf("unreviewed json field %s",typ.Field(i).Name)}
    have=append(have,tag)
    expected,ok:=c.columns[tag];if !ok{t.Fatalf("unreviewed field %s",tag)}
    if got:=types[c.table+"."+expected.sql];got!=expected.typ{t.Fatalf("%s.%s SQL type drift got %s expected %s",c.table,expected.sql,got,expected.typ)}
   }
   want:=make([]string,0,len(c.columns));for k:=range c.columns{want=append(want,k)}
   sort.Strings(want);sort.Strings(have)
   if !reflect.DeepEqual(want,have){t.Fatalf("projection field set drift: %v vs %v",have,want)}
  })
 }
}
