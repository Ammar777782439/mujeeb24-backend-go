package services

import (
    "context"
    "errors"
    "strings"
    "time"

    "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
    "github.com/google/uuid"
)

// CatalogMediaService owns gallery invariants; it never modifies commercial
// offers, invokes AI, or performs network I/O within a SQL transaction.
type CatalogMediaService struct {
    Media ports.CatalogMediaRepository
    Transactions ports.TransactionManager
    Now func() time.Time
    NewID func() string
}

func (s CatalogMediaService) now() time.Time {
    if s.Now != nil { return s.Now().UTC() }
    return time.Now().UTC()
}
func (s CatalogMediaService) newID() string {
    if s.NewID != nil { return s.NewID() }
    return uuid.NewString()
}
func (s CatalogMediaService) configured() error {
    if s.Media == nil || s.Transactions == nil { return errors.New("catalog media dependencies missing") }
    return nil
}

func (s CatalogMediaService) Attach(ctx context.Context, businessID,itemID string, variantID *string, mediaID string, primary bool, order int, alt string, approvedBy *string) (ports.CatalogMediaLink,error) {
    if err:=s.configured(); err!=nil { return ports.CatalogMediaLink{},err }
    if strings.TrimSpace(businessID)=="" || strings.TrimSpace(itemID)=="" ||
        strings.TrimSpace(mediaID)=="" || order<0 || len([]rune(alt))>500 {
        return ports.CatalogMediaLink{},errors.New("invalid catalog media link")
    }
    var created ports.CatalogMediaLink
    err:=s.Transactions.Within(ctx,func(txCtx context.Context) error{
        if err:=s.Media.LockGallery(txCtx,businessID,itemID,variantID);err!=nil{return err}
        existing,err:=s.Media.ListGallery(txCtx,businessID,itemID,variantID)
        if err!=nil{return err}
        // First image is always primary, regardless of the incoming flag.
        // An explicit primary change always occurs while the parent row is locked.
        mustPrimary:=primary || len(existing)==0
        created,err=s.Media.Attach(txCtx,ports.CatalogMediaAttach{
            ID:s.newID(),BusinessID:businessID,CatalogItemID:itemID,
            VariantID:variantID,MediaID:mediaID,SortOrder:order,AltText:alt,
            ApprovedBy:approvedBy,ApprovedAt:s.now(),
        },mustPrimary)
        return err
    })
    if err!=nil{return ports.CatalogMediaLink{},err}
    return created,nil
}

func(s CatalogMediaService) SetPrimary(ctx context.Context,businessID,itemID string, variantID *string,linkID string) error{
    if err:=s.configured();err!=nil{return err}
    if businessID==""||itemID==""||linkID==""{return errors.New("gallery and link required")}
    return s.Transactions.Within(ctx,func(txCtx context.Context)error{
        if err:=s.Media.LockGallery(txCtx,businessID,itemID,variantID);err!=nil{return err}
        return s.Media.SetPrimary(txCtx,businessID,itemID,linkID,variantID,"")
    })
}

func(s CatalogMediaService) Remove(ctx context.Context,businessID,itemID string,variantID *string,linkID string)error{
    if err:=s.configured();err!=nil{return err}
    if businessID==""||itemID==""||linkID==""{return errors.New("gallery and link required")}
    return s.Transactions.Within(ctx,func(txCtx context.Context)error{
        if err:=s.Media.LockGallery(txCtx,businessID,itemID,variantID);err!=nil{return err}
        return s.Media.Remove(txCtx,businessID,itemID,linkID,variantID,"")
    })
}
