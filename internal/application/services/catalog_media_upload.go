package services

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/base64"
 "errors"
 "image"
 _ "image/jpeg"
 _ "image/png"
 "net/http"
 "strings"
 "time"

 "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
 "github.com/google/uuid"
)

const MaxCatalogMediaBytes = 10*1024*1024

// CatalogImageUploadService handles bounded, server-verified image uploads.
// No browser-supplied MIME/hash/dimensions are trusted. Object storage I/O
// happens outside SQL transactions.
type CatalogImageUploadService struct {
 Repository ports.CatalogMediaRepository
 Objects ports.MediaObjectStorage
 Now func()time.Time
 NewID func()string
}
func(s CatalogImageUploadService)UploadBase64(ctx context.Context,businessID,encoded string,createdBy *string)(ports.MediaAsset,error){
 if s.Repository==nil||s.Objects==nil{return ports.MediaAsset{},errors.New("media uploads are not configured")}
 if _,err:=uuid.Parse(businessID);err!=nil{return ports.MediaAsset{},errors.New("invalid business")}
 if encoded==""||len(encoded)>base64.StdEncoding.EncodedLen(MaxCatalogMediaBytes){return ports.MediaAsset{},errors.New("image upload size exceeds limit")}
 data,err:=base64.StdEncoding.DecodeString(encoded)
 if err!=nil||len(data)==0||len(data)>MaxCatalogMediaBytes{return ports.MediaAsset{},errors.New("invalid image base64 or upload size")}
 // Only formats whose dimensions are checked by the Go standard library are
 // admitted in the first release. WebP is intentionally rejected, not mislabeled.
 format:=http.DetectContentType(data)
 if format!="image/jpeg"&&format!="image/png"{return ports.MediaAsset{},errors.New("unsupported image format; use JPEG or PNG")}
 config,decodeErr:=image.DecodeConfig(bytes.NewReader(data))
 if decodeErr!=nil||config.Width<1||config.Height<1||config.Width>12000||config.Height>12000{
  return ports.MediaAsset{},errors.New("invalid image dimensions")
 }
 if format=="image/jpeg"&&config.ColorModel==nil{return ports.MediaAsset{},errors.New("invalid jpeg")}
 id:=uuid.NewString();if s.NewID!=nil{id=s.NewID()}
 now:=time.Now().UTC();if s.Now!=nil{now=s.Now().UTC()}
 key:="catalog/"+businessID+"/"+id
 _,err=s.Repository.CreatePendingAsset(ctx,ports.PendingMediaAsset{
  ID:id,BusinessID:businessID,StorageKey:key,UploadExpiresAt:now.Add(30*time.Minute),CreatedBy:createdBy,
 })
 if err!=nil{return ports.MediaAsset{},err}
 if err=s.Objects.Put(ctx,key,data);err!=nil{return ports.MediaAsset{},err}
 digest:=sha256.Sum256(data)
 asset,err:=s.Repository.MarkAssetReady(ctx,businessID,id,ports.VerifiedImage{
  MimeType:format,SizeBytes:int64(len(data)),SHA256:digest[:],Width:config.Width,Height:config.Height,
 })
 if err!=nil {
  // Failure remains visible in a staged row for reconciliation. Storage is
  // best-effort deleted to avoid orphan bytes if SQL completion fails.
  cleanupCtx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
  defer cancel();_ = s.Objects.Delete(cleanupCtx,key)
  return ports.MediaAsset{},err
 }
 asset.MimeType=strings.ToLower(asset.MimeType)
 return asset,nil
}
