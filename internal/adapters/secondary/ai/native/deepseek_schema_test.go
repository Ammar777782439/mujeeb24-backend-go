package native

import (
	"encoding/json"
	"strings"
	"testing"
)

const nullableToolSchema = `{"type":"object","properties":{"item_id":{"type":"string"},"offer_id":{"type":["string","null"]},"lines":{"type":"array","items":{"type":"object","properties":{"quantity":{"type":"integer"},"variant_id":{"type":["string","null"]}},"required":["quantity","variant_id"],"additionalProperties":false}}},"required":["item_id","offer_id","lines"],"additionalProperties":false}`

func TestDeepSeekNullableSchemaRoundTripNoBusinessCoercion(t *testing.T){
	translated,err:=deepSeekSchema(json.RawMessage(nullableToolSchema))
	if err!=nil{t.Fatal(err)}
	if strings.Contains(string(translated),`"type":"null"`)||!strings.Contains(string(translated),"anyOf"){
		t.Fatalf("missing strict nullable transport transform: %s",translated)
	}
	providerJSON:=`{"item_id":"item-1","offer_id":{"__mujeeb_null_v1":true},"lines":[{"quantity":3,"variant_id":{"__mujeeb_null_v1":true}}]}`
	result,err:=restoreDeepSeekNulls(providerJSON,json.RawMessage(nullableToolSchema))
	if err!=nil{t.Fatal(err)}
	if result!=`{"item_id":"item-1","lines":[{"quantity":3,"variant_id":null}],"offer_id":null}` {
		t.Fatalf("canonical JSON changed: %s",result)
	}
}
func TestDeepSeekSchemaRejectsAmbiguousOrInventedValues(t *testing.T){
	for _,invalid:=range []string{
		`{"item_id":"item-1","offer_id":{"__mujeeb_null_v1":false},"lines":[]}`,
		`{"item_id":"item-1","offer_id":{"__mujeeb_null_v1":true,"other":"secret"},"lines":[]}`,
		`{"item_id":"item-1","offer_id":{"__mujeeb_null_v1":true},"lines":[{"quantity":"3","variant_id":null}]}`,
		`{"item_id":"item-1","offer_id":{"__mujeeb_null_v1":true},"lines":[],"approved":true}`,
	}{
		if _,err:=restoreDeepSeekNulls(invalid,json.RawMessage(nullableToolSchema));err==nil{
			t.Fatalf("invalid provider value accepted: %s",invalid)
		}
	}
	for _,schema:=range []string{
		`{"type":"object","properties":{"__mujeeb_null_v1":{"type":"boolean"}},"required":["__mujeeb_null_v1"],"additionalProperties":false}`,
		`{"type":"object","properties":{"value":{"type":["string","integer","null"]}},"required":["value"],"additionalProperties":false}`,
	}{
		if _,err:=deepSeekSchema(json.RawMessage(schema));err==nil{t.Fatalf("unsafe schema accepted: %s",schema)}
	}
}
func TestDeepSeekNullableEnumRetained(t *testing.T){
	source:=json.RawMessage(`{"type":"object","properties":{"state":{"type":["string","null"],"enum":["order","reservation",null]}},"required":["state"],"additionalProperties":false}`)
	schema,err:=deepSeekSchema(source)
	if err!=nil{t.Fatal(err)}
	if !strings.Contains(string(schema),"order")||!strings.Contains(string(schema),"reservation"){t.Fatal("lost enum values")}
	for _,args:=range []string{`{"state":{"__mujeeb_null_v1":true}}`,`{"state":"order"}`} {
		if _,err:=restoreDeepSeekNulls(args,source);err!=nil{t.Fatal(err)}
	}
	if _,err:=restoreDeepSeekNulls(`{"state":"free"}`,source);err==nil{t.Fatal("invalid enum accepted")}
}
