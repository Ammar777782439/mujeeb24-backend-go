package r2

import (
 "bytes"
 "context"
 "crypto/hmac"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "sort"
 "strings"
 "time"
)

// Client speaks Cloudflare R2's S3-compatible SigV4 protocol using the Go
// standard library. No AWS SDK and no network access during unit tests.
type Client struct {
 AccountID string
 Bucket string
 AccessKeyID string
 SecretAccessKey string
 HTTP *http.Client
 Now func() time.Time
}

func (c *Client) valid() error {
 if c == nil || c.AccountID=="" || c.Bucket=="" || c.AccessKeyID=="" || c.SecretAccessKey=="" {return errors.New("R2 is not configured")}
 for _, s := range []string{c.AccountID,c.Bucket}{
  for _,r:=range s {if !(r>='a'&&r<='z'||r>='A'&&r<='Z'||r>='0'&&r<='9'||r=='-'){return errors.New("invalid R2 account or bucket")}}
 }
 return nil
}
func (c *Client) clock()time.Time{if c.Now!=nil{return c.Now().UTC()};return time.Now().UTC()}
func (c *Client) host()string{return c.AccountID+".r2.cloudflarestorage.com"}
func escapeKey(key string)(string,error){
 if !strings.HasPrefix(key,"catalog/")||strings.Contains(key,"..")||strings.ContainsAny(key,"\\?#") {return "",errors.New("unsafe R2 key")}
 parts:=strings.Split(key,"/");for _,p:=range parts{if p==""||p=="."||p==".."{return "",errors.New("unsafe R2 path")}}
 for i,p:=range parts {parts[i]=url.PathEscape(p)}
 return strings.Join(parts,"/"),nil
}
func sha(v []byte)string{s:=sha256.Sum256(v);return hex.EncodeToString(s[:])}
func hmac256(k []byte,s string)[]byte{m:=hmac.New(sha256.New,k);m.Write([]byte(s));return m.Sum(nil)}
func (c *Client) signingKey(day string)[]byte{
 k:=hmac256([]byte("AWS4"+c.SecretAccessKey),day)
 k=hmac256(k,"auto")
 k=hmac256(k,"s3")
 return hmac256(k,"aws4_request")
}
func (c *Client) request(ctx context.Context,method,key string,body []byte)(*http.Response,error){
 if err:=c.valid();err!=nil{return nil,err}
 escaped,err:=escapeKey(key);if err!=nil{return nil,err}
 uri:="/"+c.Bucket+"/"+escaped
 now:=c.clock();date:=now.Format("20060102");timestamp:=now.Format("20060102T150405Z")
 hash:=sha(body)
 canonicalHeaders:="host:"+c.host()+"\nx-amz-content-sha256:"+hash+"\nx-amz-date:"+timestamp+"\n"
 headers:="host;x-amz-content-sha256;x-amz-date"
 canonical:=strings.Join([]string{method,uri,"",canonicalHeaders,headers,hash},"\n")
 scope:=date+"/auto/s3/aws4_request"
 st:=strings.Join([]string{"AWS4-HMAC-SHA256",timestamp,scope,sha([]byte(canonical))},"\n")
 sign:=hex.EncodeToString(hmac256(c.signingKey(date),st))
 auth:="AWS4-HMAC-SHA256 Credential="+c.AccessKeyID+"/"+scope+", SignedHeaders="+headers+", Signature="+sign
 var reader io.Reader
 if body!=nil{reader=bytes.NewReader(body)}
 req,err:=http.NewRequestWithContext(ctx,method,"https://"+c.host()+uri,reader);if err!=nil{return nil,err}
 req.Header.Set("X-Amz-Date",timestamp)
 req.Header.Set("X-Amz-Content-Sha256",hash)
 req.Header.Set("Authorization",auth)
 if method==http.MethodPut{req.Header.Set("Content-Type","application/octet-stream")}
 client:=c.HTTP;if client==nil{client=&http.Client{Timeout:20*time.Second}}
 res,err:=client.Do(req);if err!=nil{return nil,err}
 if res.StatusCode<200||res.StatusCode>=300{
  defer res.Body.Close()
  _,_=io.Copy(io.Discard,io.LimitReader(res.Body,2048))
  return nil,fmt.Errorf("R2 %s failed: HTTP %d",method,res.StatusCode)
 }
 return res,nil
}
func(c *Client)Put(ctx context.Context,key string,data []byte)error{
 if len(data)==0||len(data)>10*1024*1024{return errors.New("image size exceeds limit")}
 res,err:=c.request(ctx,http.MethodPut,key,data);if err!=nil{return err}
 defer res.Body.Close();_,_=io.Copy(io.Discard,res.Body);return nil
}
func(c *Client)Delete(ctx context.Context,key string)error{
 res,err:=c.request(ctx,http.MethodDelete,key,nil);if err!=nil{return err}
 defer res.Body.Close();_,_=io.Copy(io.Discard,res.Body);return nil
}
// PresignGet is created when the outbox is dispatched, never persisted
// in PostgreSQL. The provider fetches private bytes over a scoped URL.
func(c *Client)PresignGet(_ context.Context,key string,ttl time.Duration)(string,error){
 if err:=c.valid();err!=nil{return "",err}
 if ttl<=0||ttl>time.Hour{return "",errors.New("invalid R2 URL TTL")}
 escaped,err:=escapeKey(key);if err!=nil{return "",err}
 uri:="/"+c.Bucket+"/"+escaped
 now:=c.clock();date:=now.Format("20060102");timestamp:=now.Format("20060102T150405Z")
 scope:=date+"/auto/s3/aws4_request"
 params:=url.Values{}
 params.Set("X-Amz-Algorithm","AWS4-HMAC-SHA256")
 params.Set("X-Amz-Credential",c.AccessKeyID+"/"+scope)
 params.Set("X-Amz-Date",timestamp)
 params.Set("X-Amz-Expires",fmt.Sprintf("%d",int(ttl.Seconds())))
 params.Set("X-Amz-SignedHeaders","host")
 // url.Values.Encode sorts keys, and SigV4 requires RFC3986 %20 encoding.
 query:=strings.ReplaceAll(params.Encode(),"+","%20")
 headers:="host:"+c.host()+"\n"
 canonical:=strings.Join([]string{"GET",uri,query,headers,"host","UNSIGNED-PAYLOAD"},"\n")
 sign:=hex.EncodeToString(hmac256(c.signingKey(date),strings.Join([]string{"AWS4-HMAC-SHA256",timestamp,scope,sha([]byte(canonical))},"\n")))
 params.Set("X-Amz-Signature",sign)
 // Defense in depth: no user-controlled query is included.
 return "https://"+c.host()+uri+"?"+strings.ReplaceAll(params.Encode(),"+","%20"),nil
}
