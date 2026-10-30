package socialapi

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"

 "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestSendMessageWithCatalogPhotoUsesSameProviderRequest(t *testing.T){
 requests:=0
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  requests++
  if r.URL.Path!="/v1/inbox/conversations/conv-image/messages"||r.Method!=http.MethodPost{
   t.Fatalf("wrong provider route: %s %s",r.Method,r.URL)
  }
  var body struct{
   Text string `json:"text"`
   Attachments []struct{ Type string `json:"type"`;SourceType string `json:"source_type"`;Source string `json:"source"` } `json:"attachments"`
  }
  if err:=json.NewDecoder(r.Body).Decode(&body);err!=nil{t.Fatal(err)}
  if body.Text!="هذا المنتج متوفر"||len(body.Attachments)!=1||
    body.Attachments[0].Type!="image"||body.Attachments[0].SourceType!="url"||
    body.Attachments[0].Source!="https://private.example.test/product.jpg?sig=123"{
   t.Fatalf("invalid mixed provider send: %+v",body)
  }
  w.Header().Set("Content-Type","application/json")
  _,_=w.Write([]byte(`{"success":true,"message_ids":["m_image"]}`))
 }))
 defer server.Close()
 client:=NewClient(Config{BaseURL:server.URL,APIKey:"test-key"})
 result,err:=client.SendMessage(context.Background(),ports.SendMessageCommand{
  ProviderAccountID:"acc1",ProviderConversationID:"conv-image",
  Channel:"whatsapp",Text:"هذا المنتج متوفر",IdempotencyKey:"outbound-1",
  Attachments:[]ports.OutboundAttachment{{Type:"image",URL:"https://private.example.test/product.jpg?sig=123"}},
 })
 if err!=nil{t.Fatal(err)}
 if requests!=1||result.ProviderMessageID!="m_image"{t.Fatalf("unexpected send result %+v, calls=%d",result,requests)}
}
func TestSendMessageRejectsUnsafeCatalogURL(t *testing.T){
 client:=NewClient(Config{APIKey:"x"})
 _,err:=client.SendMessage(context.Background(),ports.SendMessageCommand{
  ProviderAccountID:"acc",ProviderConversationID:"conv",Channel:"whatsapp",
  Text:"msg",IdempotencyKey:"x",Attachments:[]ports.OutboundAttachment{{Type:"image",URL:"http://localhost/private"}},
 })
 if err==nil{t.Fatal("must reject non-HTTPS signed media URL")}
}
