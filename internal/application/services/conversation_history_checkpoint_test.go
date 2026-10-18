package services

import (
 "context"
 "strings"
 "testing"
 "time"

 "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// The real PostgreSQL repository returns newest-first pages. The summarizer
// MUST normalize all pages before building context for Gemini OR DeepSeek.
type checkpointMessages struct {
 pages map[string]ports.MessagePage
}
func (m checkpointMessages) Record(context.Context, ports.CommunicationMessageDraft) (ports.CommunicationMessageRecord,error) {
 return ports.CommunicationMessageRecord{},nil
}
func (m checkpointMessages) ListByConversation(_ context.Context, _, _ string, _ int, cursor string) (ports.MessagePage,error) {
 return m.pages[cursor],nil
}
func checkpointMsg(id, direction, text string, age time.Duration) ports.CommunicationMessageRecord {
 return ports.CommunicationMessageRecord{ID:id,Direction:direction,Origin:"customer",TextContent:&text,OccurredAt:time.Now().Add(age)}
}
func TestConversationHistoryIsChronologicalAcrossPages(t *testing.T) {
 m:=checkpointMessages{pages:map[string]ports.MessagePage{
  "":{Items:[]ports.CommunicationMessageRecord{checkpointMsg("m4","outbound","four",0),checkpointMsg("m3","inbound","three",-time.Minute)},HasMore:true,NextCursor:"older"},
  "older":{Items:[]ports.CommunicationMessageRecord{checkpointMsg("m2","outbound","two",-2*time.Minute),checkpointMsg("m1","inbound","one",-3*time.Minute)}},
 }}
 out,err:=listConversationHistory(context.Background(),m,"biz","conv")
 if err!=nil{t.Fatal(err)}
 if len(out)!=4 || out[0].ID!="m1" || out[1].ID!="m2" || out[2].ID!="m3" || out[3].ID!="m4" {t.Fatalf("incorrect message chronology: %#v",out)}
}
func TestConversationHistoryFailsClosedOnBrokenCursor(t *testing.T) {
 m:=checkpointMessages{pages:map[string]ports.MessagePage{
  "":{Items:[]ports.CommunicationMessageRecord{checkpointMsg("one","inbound","text",0)},HasMore:true,NextCursor:""},
 }}
 if _,err:=listConversationHistory(context.Background(),m,"biz","conv");err==nil {t.Fatal("accepted an incomplete conversation history")}
}
func TestTranscriptChunksNeverTruncateArabicMessages(t *testing.T) {
 messages:=[]ports.CommunicationMessageRecord{
  checkpointMsg("m1","inbound","عندي متجر صغير وأريد مقارنة الأسعار", -time.Minute),
  checkpointMsg("m2","outbound","المساعد وعد بالتحويل ولم يثبت التنفيذ",0),
 }
 parts,err:=transcriptChunks(messages,130,3)
 if err!=nil{t.Fatal(err)}
 combined:=strings.Join(parts,"")
 if !strings.Contains(combined,"عندي متجر صغير وأريد مقارنة الأسعار") || !strings.Contains(combined,"المساعد وعد بالتحويل ولم يثبت التنفيذ") {t.Fatalf("message content lost: %q",combined)}
 if !strings.Contains(combined,"المساعد:") {t.Fatalf("assistant not attributed: %q",combined)}
}
func TestTranscriptChunksRejectUnboundedSingleMessage(t *testing.T) {
 msgs:=[]ports.CommunicationMessageRecord{checkpointMsg("long","inbound",strings.Repeat("أ",3000),0)}
 if _,err:=transcriptChunks(msgs,3900,8);err==nil{t.Fatal("oversized message must not be cut mid-UTF8 or silently discarded")}
}
func TestTranscriptChunksBoundInferenceCalls(t *testing.T) {
 var msgs []ports.CommunicationMessageRecord
 for i:=0;i<12;i++ { msgs=append(msgs,checkpointMsg("id","inbound",strings.Repeat("أ",40),time.Duration(-i)*time.Minute)) }
 if _,err:=transcriptChunks(msgs,110,2);err==nil {t.Fatal("unbounded model calls accepted")}
}
