package contracts

import (
 "encoding/json"
 "errors"
 "fmt"
 "strings"
)

// QualificationReferenceSetV1 is the only approved projection of arbitrary
// qualification_evidence JSONB. The persisted value is an array of evidence
// reference strings (SalesCommandServices stringArrayJSON), not an open map.
// References are NOT claims until resolved against a scoped source.
type QualificationReferenceSetV1 struct {
 ReferenceCount int `json:"reference_count"`
 VerificationState string `json:"verification_state"`
}
var ErrQualificationEvidence=errors.New("invalid qualification evidence")
func ParseQualificationReferences(raw []byte)(QualificationReferenceSetV1,error){
 var result QualificationReferenceSetV1
 if len(raw)==0||len(raw)>8192{return result,ErrQualificationEvidence}
 var refs []string
 if err:=json.Unmarshal(raw,&refs);err!=nil{return result,fmt.Errorf("%w: non-string evidence array",ErrQualificationEvidence)}
 if refs==nil||len(refs)>24{return result,ErrQualificationEvidence}
 seen:=map[string]bool{}
 for _,ref:=range refs {
  if strings.TrimSpace(ref)!=ref||ref==""||len(ref)>160||seen[ref]{
   return result,ErrQualificationEvidence
  }
  seen[ref]=true
 }
 return QualificationReferenceSetV1{ReferenceCount:len(refs),VerificationState:"unverified_references"},nil
}
