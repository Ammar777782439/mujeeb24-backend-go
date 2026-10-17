package native

import (
 "bytes"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "regexp"
 "strings"
)

// nativeSchema deliberately implements only the JSON Schema keywords that
// have been qualified for the native catalog and customer tool transport.
// Unsupported keywords are rejected, never discarded as hints.
type nativeSchema struct {
 Type json.RawMessage `json:"type"`
 Properties map[string]json.RawMessage `json:"properties"`
 Required json.RawMessage `json:"required"`
 AdditionalProperties *bool `json:"additionalProperties"`
 Items json.RawMessage `json:"items"`
 Enum []json.RawMessage `json:"enum"`
 Description string `json:"description"`
 MinItems *int `json:"minItems"`
 MaxItems *int `json:"maxItems"`
 MinLength *int `json:"minLength"`
 MaxLength *int `json:"maxLength"`
 Pattern string `json:"pattern"`
 Minimum *json.Number `json:"minimum"`
 Maximum *json.Number `json:"maximum"`
 kinds []string
 children map[string]*nativeSchema
 item *nativeSchema
 regexp *regexp.Regexp
}

// compileNativeSchema does not rewrite the model-facing schema. It creates an
// independent verifier for requests and responses using exact JSON types.
func compileNativeSchema(raw json.RawMessage) (*nativeSchema, error) {
 return compileNativeSchemaAt(raw,0)
}
func compileNativeSchemaAt(raw json.RawMessage, depth int) (*nativeSchema,error) {
 if depth>32 {return nil,errors.New("JSON Schema nesting exceeds safety limit")}
 var s nativeSchema
 if err:=strictUnmarshal(raw,&s);err!=nil{return nil,fmt.Errorf("unsupported or malformed native JSON Schema: %w",err)}
 if len(s.Type)==0{return nil,errors.New("native schema missing type")}
 var kind string
 if err:=json.Unmarshal(s.Type,&kind);err==nil {
  s.kinds=[]string{kind}
 } else if err:=json.Unmarshal(s.Type,&s.kinds);err!=nil || len(s.kinds)==0 {
  return nil,errors.New("invalid native schema type union")
 }
 seen:=map[string]bool{}
 for _,k:=range s.kinds {
  if seen[k] {return nil,errors.New("duplicate JSON Schema type")}
  seen[k]=true
  switch k {case "object","array","string","integer","number","boolean","null": default:return nil,fmt.Errorf("unsupported schema type %q",k)}
 }
 if s.MinItems!=nil && *s.MinItems<0 || s.MaxItems!=nil && *s.MaxItems<0 ||
  s.MinLength!=nil && *s.MinLength<0 || s.MaxLength!=nil && *s.MaxLength<0 {
  return nil,errors.New("negative schema size bound")
 }
 if s.MinItems!=nil && s.MaxItems!=nil && *s.MinItems>*s.MaxItems ||
  s.MinLength!=nil && s.MaxLength!=nil && *s.MinLength>*s.MaxLength {
  return nil,errors.New("invalid schema size bounds")
 }
 if s.Pattern!="" {
  var err error
  s.regexp,err=regexp.Compile(s.Pattern)
  if err!=nil{return nil,errors.New("invalid schema pattern")}
 }
 if seen["object"] {
  if s.Properties==nil||s.AdditionalProperties==nil||*s.AdditionalProperties||len(s.Required)==0 ||
   bytes.Equal(bytes.TrimSpace(s.Required),[]byte("null")) {
   return nil,errors.New("strict object requires properties, required, and additionalProperties=false")
  }
  var names []string
  if err:=json.Unmarshal(s.Required,&names);err!=nil||names==nil||len(names)!=len(s.Properties) {
   return nil,errors.New("strict object requires all properties")
  }
  seenNames:=map[string]bool{}
  for _,n:=range names {
   if seenNames[n] {return nil,errors.New("duplicate required property")}
   if _,ok:=s.Properties[n];!ok{return nil,errors.New("required name absent from properties")}
   seenNames[n]=true
  }
  s.children=make(map[string]*nativeSchema,len(s.Properties))
  for name,childRaw:=range s.Properties {
   child,err:=compileNativeSchemaAt(childRaw,depth+1)
   if err!=nil{return nil,fmt.Errorf("property %s: %w",name,err)}
   s.children[name]=child
  }
 } else if len(s.Properties)>0||s.AdditionalProperties!=nil||len(s.Required)>0 {
  return nil,errors.New("object keywords on non-object schema")
 }
 if seen["array"] {
  if len(s.Items)==0 {return nil,errors.New("array requires items schema")}
  var err error
  s.item,err=compileNativeSchemaAt(s.Items,depth+1)
  if err!=nil{return nil,err}
 } else if len(s.Items)>0||s.MinItems!=nil||s.MaxItems!=nil {
  return nil,errors.New("array keywords on non-array schema")
 }
 if !seen["string"] && (s.MinLength!=nil||s.MaxLength!=nil||s.Pattern!="") {
  return nil,errors.New("string keywords on non-string schema")
 }
 if !seen["number"]&&!seen["integer"]&&(s.Minimum!=nil||s.Maximum!=nil){
  return nil,errors.New("number bounds on non-number schema")
 }
 if s.Minimum!=nil && s.Maximum!=nil {
  var lo,hi float64
  if err:=json.Unmarshal([]byte(s.Minimum.String()),&lo);err!=nil{return nil,err}
  if err:=json.Unmarshal([]byte(s.Maximum.String()),&hi);err!=nil{return nil,err}
  if lo>hi{return nil,errors.New("invalid number bounds")}
 }
 for _,choice:=range s.Enum {
  var value any
  if err:=strictUnmarshalValue(choice,&value);err!=nil{return nil,errors.New("invalid enum value")}
  if !s.valueTypeAllowed(value) {return nil,errors.New("enum value violates schema type")}
 }
 return &s,nil
}

func strictUnmarshal(raw []byte,target any)error {
 dec:=json.NewDecoder(bytes.NewReader(raw))
 dec.DisallowUnknownFields()
 if err:=dec.Decode(target);err!=nil{return err}
 var tail any
 if err:=dec.Decode(&tail);err!=io.EOF {return errors.New("trailing JSON values")}
 return nil
}
func strictUnmarshalValue(raw []byte,target any)error {
 dec:=json.NewDecoder(bytes.NewReader(raw));dec.UseNumber()
 if err:=dec.Decode(target);err!=nil{return err}
 var tail any
 if err:=dec.Decode(&tail);err!=io.EOF{return errors.New("trailing JSON values")}
 return nil
}
func (s *nativeSchema) valueTypeAllowed(v any)bool{
 for _,t:=range s.kinds {
  switch t{
  case "null":if v==nil{return true}
  case "object":if _,ok:=v.(map[string]any);ok{return true}
  case "array":if _,ok:=v.([]any);ok{return true}
  case "string":if _,ok:=v.(string);ok{return true}
  case "boolean":if _,ok:=v.(bool);ok{return true}
  case "number":if _,ok:=v.(json.Number);ok{return true}
  case "integer":if n,ok:=v.(json.Number);ok&&!strings.ContainsAny(n.String(),".eE"){if _,err:=n.Int64();err==nil{return true}}
  }
 }
 return false
}
func (s *nativeSchema) validateValue(v any,depth int)error{
 if depth>32{return errors.New("model result nesting exceeds safety limit")}
 if !s.valueTypeAllowed(v){return errors.New("model result type mismatch")}
 if len(s.Enum)>0{
  found:=false
  for _,raw:=range s.Enum {
   var choice any
   if err:=strictUnmarshalValue(raw,&choice);err!=nil{return err}
   candidate,_:=json.Marshal(choice)
   current,_:=json.Marshal(v)
   if bytes.Equal(candidate,current){found=true;break}
  }
  if !found{return errors.New("model result violates allowed enum")}
 }
 switch x:=v.(type){
 case map[string]any:
  if len(x)!=len(s.children){return errors.New("model result has extra or missing object fields")}
  for name,child:=range s.children {
   item,ok:=x[name]
   if !ok{return errors.New("required model field missing")}
   if err:=child.validateValue(item,depth+1);err!=nil{return fmt.Errorf("%s: %w",name,err)}
  }
 case []any:
  if s.MinItems!=nil&&len(x)<*s.MinItems||s.MaxItems!=nil&&len(x)>*s.MaxItems{return errors.New("array size violates schema")}
  for i,item:=range x{
   if err:=s.item.validateValue(item,depth+1);err!=nil{return fmt.Errorf("item %d: %w",i,err)}
  }
 case string:
  n:=len([]rune(x))
  if s.MinLength!=nil&&n<*s.MinLength||s.MaxLength!=nil&&n>*s.MaxLength{return errors.New("string length violates schema")}
  if s.regexp!=nil&&!s.regexp.MatchString(x){return errors.New("string pattern violates schema")}
 case json.Number:
  if s.Minimum!=nil||s.Maximum!=nil {
   // Bounds are evaluated on decimal values without loss of precision.
   return errors.New("numeric bounds need precision-preserving validator")
  }
 }
 return nil
}
func validateNativeSchemaResult(raw json.RawMessage,schema json.RawMessage)error{
 compiled,err:=compileNativeSchema(schema)
 if err!=nil{return err}
 var value any
 if err:=strictUnmarshalValue(raw,&value);err!=nil{return err}
 return compiled.validateValue(value,0)
}
