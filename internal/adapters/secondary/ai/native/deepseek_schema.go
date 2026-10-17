package native

import (
	"encoding/json"
	"errors"
	"fmt"
)

// DeepSeek's documented strict schema types exclude JSON null. Encode a
// nullable scalar as a disjoint tagged object, then restore null before the
// original Mujeeb schema is validated. This never guesses a commercial value.
const deepSeekNullTag = "__mujeeb_null_v1"

func deepSeekSchema(raw json.RawMessage) (json.RawMessage,error) {
	if _,err:=compileNativeSchema(raw);err!=nil {
		return nil,fmt.Errorf("canonical schema unsupported: %w",err)
	}
	var source map[string]any
	if err:=strictUnmarshalValue(raw,&source);err!=nil||source==nil {
		return nil,errors.New("invalid native function schema")
	}
	transformed,err:=deepSeekSchemaNode(source,0)
	if err!=nil {return nil,err}
	return json.Marshal(transformed)
}

func deepSeekSchemaNode(source map[string]any, depth int)(map[string]any,error){
	if depth>32{return nil,errors.New("DeepSeek schema nesting too deep")}
	node:=make(map[string]any,len(source))
	for k,v:=range source {node[k]=v}
	if props,ok:=node["properties"].(map[string]any);ok {
		if _,collision:=props[deepSeekNullTag];collision {
			return nil,errors.New("canonical function schema collides with reserved null encoding")
		}
		replaced:=make(map[string]any,len(props))
		for name,raw:=range props {
			child,ok:=raw.(map[string]any)
			if !ok{return nil,errors.New("malformed schema property")}
			nested,err:=deepSeekSchemaNode(child,depth+1)
			if err!=nil{return nil,err}
			replaced[name]=nested
		}
		node["properties"]=replaced
	}
	if item,ok:=node["items"];ok {
		typed,ok:=item.(map[string]any)
		if !ok{return nil,errors.New("malformed schema array item")}
		out,err:=deepSeekSchemaNode(typed,depth+1)
		if err!=nil{return nil,err}
		node["items"]=out
	}
	typeList,ok:=node["type"].([]any)
	if !ok{return node,nil}
	nullable:=false
	kinds:=make([]string,0,len(typeList))
	for _,raw:=range typeList {
		t,ok:=raw.(string)
		if !ok{return nil,errors.New("invalid JSON schema union")}
		if t=="null" {nullable=true}else{kinds=append(kinds,t)}
	}
	if !nullable {
		if len(kinds)!=1 {return nil,errors.New("DeepSeek strict mode cannot represent multi-type union")}
		node["type"]=kinds[0]
		return node,nil
	}
	if len(kinds)>1{return nil,errors.New("nullable union with multiple concrete types not supported")}
	// Preserve enum exactly: a null branch exists only when null is actually
	// allowed by both type and enum. No nullable value is replaced by "" or 0.
	nullAllowed:=true
	if raw,hasEnum:=node["enum"];hasEnum{
		values,ok:=raw.([]any)
		if !ok{return nil,errors.New("invalid enum values")}
		filtered:=make([]any,0,len(values))
		nullAllowed=false
		for _,value:=range values{
			if value==nil {nullAllowed=true}else{filtered=append(filtered,value)}
		}
		node["enum"]=filtered
	}
	if len(kinds)==1 {node["type"]=kinds[0]}
	baseAvailable:=len(kinds)==1
	if _,hasEnum:=node["enum"];hasEnum{
		if values,ok:=node["enum"].([]any);ok&&len(values)==0{baseAvailable=false}
	}
	if !nullAllowed {
		if !baseAvailable{return nil,errors.New("schema excludes all values")}
		return node,nil
	}
	marker:=map[string]any{
		"type":"object",
		"properties":map[string]any{deepSeekNullTag:map[string]any{
			"type":"boolean","enum":[]any{true},
		}},
		"required":[]string{deepSeekNullTag},
		"additionalProperties":false,
	}
	if !baseAvailable{return marker,nil}
	return map[string]any{"anyOf":[]any{node,marker}},nil
}

func restoreDeepSeekNulls(raw string, canonical json.RawMessage)(string,error){
	schema,err:=compileNativeSchema(canonical)
	if err!=nil{return "",err}
	var value any
	if err:=strictUnmarshalValue([]byte(raw),&value);err!=nil{
		return "",errors.New("invalid DeepSeek function JSON")
	}
	restored,err:=deepSeekRestoreValue(value,0)
	if err!=nil{return "",err}
	if err:=schema.validateValue(restored,0);err!=nil{
		return "",fmt.Errorf("DeepSeek function violates canonical schema: %w",err)
	}
	data,err:=json.Marshal(restored)
	if err!=nil{return "",err}
	return string(data),nil
}
func deepSeekRestoreValue(value any,depth int)(any,error){
	if depth>32{return nil,errors.New("DeepSeek response nesting too deep")}
	switch v:=value.(type){
	case map[string]any:
		if _,found:=v[deepSeekNullTag];found {
			if len(v)!=1||v[deepSeekNullTag]!=true {
				return nil,errors.New("invalid DeepSeek null marker")
			}
			return nil,nil
		}
		result:=make(map[string]any,len(v))
		for k,val:=range v{
			next,err:=deepSeekRestoreValue(val,depth+1)
			if err!=nil{return nil,err}
			result[k]=next
		}
		return result,nil
	case []any:
		result:=make([]any,len(v))
		for i,item:=range v{
			next,err:=deepSeekRestoreValue(item,depth+1)
			if err!=nil{return nil,err}
			result[i]=next
		}
		return result,nil
	default:
		return value,nil
	}
}
