package contract

import (
 "net/http"
 "github.com/danielgtaylor/huma/v2"
)

// Merchant assistant is separate from customer sales and merchant catalog
// authoring. The only operation is a read-only, reviewed suggestion.
func registerMerchantAssistOperations(api huma.API,dispatcher DashboardOperationHandler){
 register(api,dispatcher,huma.Operation{
  OperationID:"merchantAIAssist",
  Method:http.MethodPost,
  Path:"/businesses/{business_id}/ai-assist",
  Tags:[]string{"MerchantAssist"},
  Summary:"Generate a review-only merchant assistant answer from approved business evidence",
  Security:dashboardSecurity,
 },MerchantAssistInput{},Single[MerchantAssistResponse]{})
}
