package merchantassist

import (
 "context"
 "errors"
 "regexp"
 "strings"
 "unicode"
 "unicode/utf8"
)

// ConservativeSanitizer is a deterministic privacy MINIMUM, not a claim to
// recognize all Arabic names/addresses or all personal data. Callers must
// classify customer free text separately before any external model transfer.
// It fails closed on high-risk credential / identifier indicators and strips
// recognized contacts and URLs without keeping their original values.
type ConservativeSanitizer struct{}

var (
 errSensitiveContent=errors.New("content requires private human review")
 emailPattern=regexp.MustCompile(`(?i)[\p{L}0-9._%+\-]+@[\p{L}0-9.\-]+\.[a-z]{2,}`)
 urlPattern=regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s<>"']+`)
 phonePattern=regexp.MustCompile(`(?:\+?\d[\d\s().\-]{7,}\d)`)
 apiKeyPattern=regexp.MustCompile(`(?i)(?:sk-[a-z0-9_\-]{12,}|(?:api[_ -]?key|password|secret|token|كلمة السر|كلمة المرور|رمز التحقق|الرمز السري)\s*[:=]\s*\S+)`)
 longDigitPattern=regexp.MustCompile(`\d{9,}`)
 riskMarkerPattern=regexp.MustCompile(`(?i)(?:بطاقة الهوية|رقم الهوية|رقم جواز|رقم البطاقة|passport|national id|credit card|cvv|otp|private key|authorization\s*:\s*bearer)`)
)

func (ConservativeSanitizer) Sanitize(ctx context.Context,kind ContentKind,source string)(string,error){
 if err:=ctx.Err();err!=nil{return "",err}
 if kind!=ContentMessage && kind!=ContentSummary{return "",errSensitiveContent}
 if source=="" || len(source)>historyMaxTextBytes*4 || !utf8.ValidString(source) {return "",errSensitiveContent}
 for _,r:=range source {if r==0 || unicode.IsControl(r) && r!='\n' && r!='\t' && r!='\r'{return "",errSensitiveContent}}
 if apiKeyPattern.MatchString(source)||riskMarkerPattern.MatchString(source){return "",errSensitiveContent}
 text:=emailPattern.ReplaceAllString(source,"[بريد محجوب]")
 text=urlPattern.ReplaceAllString(text,"[رابط محجوب]")
 text=phonePattern.ReplaceAllString(text,"[رقم محجوب]")
 text=longDigitPattern.ReplaceAllString(text,"[معرّف محجوب]")
 text=strings.TrimSpace(text)
 if text=="" || len(text)>historyMaxTextBytes {return "",errSensitiveContent}
 return text,nil
}
