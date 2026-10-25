package contracts

import (
 "encoding/json"
 "os"
 "path/filepath"
 "reflect"
 "regexp"
 "sort"
 "testing"
)

// SQL CHECK declarations are authoritative. These tests reject enum additions
// or removals until projection validators and the domain contract are reviewed.
func TestReviewedDomainSQLStatusEnums(t *testing.T) {
 raw,err:=os.ReadFile(filepath.Join("..","..","..","platform","database","testdata","merchant_assist_schema_pg16.json"));if err!=nil{t.Fatal(err)}
 var snapshot struct {Constraints []struct {Name,Definition string} `json:"constraints"`}
 if err:=json.Unmarshal(raw,&snapshot);err!=nil{t.Fatal(err)}
 byName:=map[string]string{};for _,c:=range snapshot.Constraints{byName[c.Name]=c.Definition}
 expected:=map[string][]string{
  "business_policy_versions_status_chk":{"draft","published","archived"},
  "business_policy_versions_category_chk":{"hours","returns","delivery","pricing","availability","ai_mode","channel","general"},
  "business_policy_versions_authority_chk":{"merchant"},
  "knowledge_documents_status_chk":{"draft","published","archived"},
  "knowledge_documents_content_type_chk":{"faq","hours","location","service","general"},
  "knowledge_documents_authority_chk":{"merchant","system"},
  "customers_status_chk":{"active","merged","archived"},
  "conversations_state_chk":{"open","ai_handling","waiting_customer","waiting_human","human_handling","closed"},
  "conversations_ownership_chk":{"none","ai","human"},
  "conversations_priority_chk":{"low","normal","high","urgent"},
  "conversations_ai_mode_chk":{"allowed","draft_only","disabled"},
  "communication_messages_direction_chk":{"inbound","outbound"},
  "communication_messages_origin_chk":{"customer","ai","human","automation","system"},
  "communication_messages_visibility_chk":{"public","private"},
  "communication_messages_content_type_chk":{"text","image","video","audio","file","mixed","unknown"},
  "canned_replies_status_chk":{"active","archived"},
  "leads_status_chk":{"new","interested","qualified","won","lost"},
  "leads_qualification_state_chk":{"new","qualified","working","converted","lost","disqualified"},
  "leads_score_band_chk":{"unknown","low","medium","high"},
  "commercial_transactions_type_chk":{"order","booking","appointment","service_request","reservation","quote","subscription"},
  "commercial_transactions_state_chk":{"draft","needs_information","awaiting_confirmation","confirmed","in_fulfillment","completed","cancelled","expired","rejected"},
  "transaction_reviews_status_chk":{"pending","approved","rejected","bypassed"},
  "transaction_confirmations_status_chk":{"not_required","pending","confirmed","rejected"},
  "sales_workflows_state_check":{"gathering","draft","awaiting_confirmation","awaiting_review","confirmed","declined","cancelled"},
  "sales_transaction_requirements_transaction_type_check":{"order","booking","appointment","service_request","reservation","quote","subscription"},
 }
 rx:=regexp.MustCompile(`'([^']+)'::text`)
 for name,want:=range expected {
  definition:=byName[name]
  if definition==""{t.Fatalf("SQL CHECK not found: %s",name)}
  matches:=rx.FindAllStringSubmatch(definition,-1)
  unique:=map[string]bool{}
  for _,m:=range matches {unique[m[1]]=true}
  got:=make([]string,0,len(unique))
  for v:=range unique {got=append(got,v)}
  sort.Strings(got);sort.Strings(want)
  if !reflect.DeepEqual(got,want){t.Errorf("SQL enum %s changed: actual %v expected %v",name,got,want)}
 }
}
