# مراجعة مصفوفة PR-008 — HTTP Contract وApplication Boundary

## قرار بنية المجلدات

في هذا المشروع، `internal/adapters/primary/http/contract` هو حزمة **HTTP DTOs + Huma operation registration** معًا. هذا مقصود لأن سياسة DTO-first تعتمد نفس Go types في parsing وفي توليد OpenAPI، ولا نريد نسخة ثانية من DTOs داخل `http/dto` تسبب drift. لذلك كان `http/dto/.gitkeep` placeholder غير مستخدم، وتم حذفه. أما `http/handlers` فهو الحزمة الفعلية للتحويل إلى Commands/Queries، وتم حذف placeholder منها بعد دخول `core.go`. `middleware` يبقى فارغًا مؤقتًا إلى أن يبدأ Auth/Scope middleware في إغلاق PR-008.

وجود DTOs داخل `contract` لا يعني أن كل Functional Application handlers مكتملة؛ المصفوفة التالية تفصل بين وجود العقد وبين runtime wiring الفعلي.

## Operations
addPrivateNote
approveTransactionReview
assignConversation
authenticatePrincipal
beginChannelConnection
cancelTransaction
confirmTransaction
createAttributeSchemaVersion
createCatalog
createCatalogItem
createCustomer
createLead
createOffer
createOutboundMessage
createTransactionDraft
createVariant
disconnectChannel
getAIDecision
getAttributeSchema
getAuditEvent
getBusiness
getBusinessPolicy
getCatalog
getCatalogItem
getChannelConnection
getConnectionCapabilities
getConversation
getCurrentPrincipal
getCustomer
getDashboardOverview
getLead
getLiveness
getMetrics
getReadiness
getTransaction
getTransactionReview
ingestChatwootWebhook
ingestSocialAPIWebhook
listAIDecisions
listAccessibleBusinesses
listAttributeSchemas
listAuditEvents
listCatalogItems
listCatalogs
listChannelConnections
listConversationMessages
listConversations
listCustomerConversations
listCustomerTransactions
listCustomers
listLeadAttributions
listLeadScores
listLeads
listOffers
listTransactions
listVariants
markLeadLost
mergeCustomer
qualifyLead
reconnectChannel
rejectTransactionReview
requestHumanReview
revokeRefreshSession
rotateRefreshSession
submitTransactionReview
updateBusinessPolicy
updateBusinessProfile
updateCatalog
updateCatalogItem
updateConversation
updateConversationLabels
updateCustomer
updateLead
updateOffer
updateTransactionDraft
updateVariant
## Command aliases
type AddPrivateNoteHandler = CommandHandler[AddPrivateNoteCommand, MessageResult]
type ApproveTransactionReviewHandler = CommandHandler[ApproveTransactionReviewCommand, TransactionReviewResult]
type AssignConversationHandler = CommandHandler[AssignConversationCommand, ConversationResult]
type AuthenticatePrincipalHandler = CommandHandler[AuthenticatePrincipalCommand, AuthResult]
type BeginChannelConnectionHandler = CommandHandler[BeginChannelConnectionCommand, BeginChannelConnectionResult]
type CancelTransactionHandler = CommandHandler[CancelTransactionCommand, TransactionResult]
type ConfirmTransactionHandler = CommandHandler[ConfirmTransactionCommand, TransactionResult]
type CreateAttributeSchemaVersionHandler = CommandHandler[CreateAttributeSchemaVersionCommand, AttributeSchemaResult]
type CreateCatalogHandler = CommandHandler[CreateCatalogCommand, CatalogResult]
type CreateCatalogItemHandler = CommandHandler[CreateCatalogItemCommand, CatalogItemResult]
type CreateCustomerHandler = CommandHandler[CreateCustomerCommand, CustomerResult]
type CreateLeadHandler = CommandHandler[CreateLeadCommand, LeadResult]
type CreateOfferHandler = CommandHandler[CreateOfferCommand, OfferResult]
type CreateOutboundMessageHandler = CommandHandler[CreateOutboundMessageCommand, MessageResult]
type CreateTransactionDraftHandler = CommandHandler[CreateTransactionDraftCommand, TransactionResult]
type CreateVariantHandler = CommandHandler[CreateVariantCommand, VariantResult]
type DisconnectChannelHandler = CommandHandler[DisconnectChannelCommand, ChannelConnectionResult]
type MarkLeadLostHandler = CommandHandler[MarkLeadLostCommand, LeadResult]
type MergeCustomerHandler = CommandHandler[MergeCustomerCommand, CustomerResult]
type QualifyLeadHandler = CommandHandler[QualifyLeadCommand, LeadResult]
type ReconnectChannelHandler = CommandHandler[ReconnectChannelCommand, ChannelConnectionResult]
type RejectTransactionReviewHandler = CommandHandler[RejectTransactionReviewCommand, TransactionReviewResult]
type RequestHumanReviewHandler = CommandHandler[RequestHumanReviewCommand, AIDecisionResult]
type RevokeRefreshSessionHandler = CommandHandler[RevokeRefreshSessionCommand, EmptyResult]
type RotateRefreshSessionHandler = CommandHandler[RotateRefreshSessionCommand, AuthResult]
type SubmitTransactionReviewHandler = CommandHandler[SubmitTransactionReviewCommand, TransactionReviewResult]
type UpdateBusinessPolicyHandler = CommandHandler[UpdateBusinessPolicyCommand, UpdateBusinessPolicyResult]
type UpdateBusinessProfileHandler = CommandHandler[UpdateBusinessProfileCommand, UpdateBusinessProfileResult]
type UpdateCatalogHandler = CommandHandler[UpdateCatalogCommand, CatalogResult]
type UpdateCatalogItemHandler = CommandHandler[UpdateCatalogItemCommand, CatalogItemResult]
type UpdateConversationHandler = CommandHandler[UpdateConversationCommand, ConversationResult]
type UpdateConversationLabelsHandler = CommandHandler[UpdateConversationLabelsCommand, ConversationResult]
type UpdateCustomerHandler = CommandHandler[UpdateCustomerCommand, CustomerResult]
type UpdateLeadHandler = CommandHandler[UpdateLeadCommand, LeadResult]
type UpdateOfferHandler = CommandHandler[UpdateOfferCommand, OfferResult]
type UpdateTransactionDraftHandler = CommandHandler[UpdateTransactionDraftCommand, TransactionResult]
type UpdateVariantHandler = CommandHandler[UpdateVariantCommand, VariantResult]
## Query aliases
type GetAIDecisionHandler = commands.QueryHandler[GetAIDecisionQuery, commands.AIDecisionView]
type GetAttributeSchemaHandler = commands.QueryHandler[GetAttributeSchemaQuery, commands.AttributeSchemaView]
type GetAuditEventHandler = commands.QueryHandler[GetAuditEventQuery, commands.AuditEventView]
type GetBusinessHandler = commands.QueryHandler[GetBusinessQuery, commands.BusinessView]
type GetBusinessPolicyHandler = commands.QueryHandler[GetBusinessPolicyQuery, commands.BusinessPolicyView]
type GetCatalogHandler = commands.QueryHandler[GetCatalogQuery, commands.CatalogView]
type GetCatalogItemHandler = commands.QueryHandler[GetCatalogItemQuery, commands.CatalogItemView]
type GetChannelConnectionHandler = commands.QueryHandler[GetChannelConnectionQuery, commands.ChannelConnectionView]
type GetConnectionCapabilitiesHandler = commands.QueryHandler[GetConnectionCapabilitiesQuery, commands.ListResult[ConnectionCapabilityView]]
type GetConversationHandler = commands.QueryHandler[GetConversationQuery, commands.ConversationView]
type GetCurrentPrincipalHandler = commands.QueryHandler[GetCurrentPrincipalQuery, PrincipalView]
type GetCustomerHandler = commands.QueryHandler[GetCustomerQuery, commands.CustomerView]
type GetDashboardOverviewHandler = commands.QueryHandler[GetDashboardOverviewQuery, DashboardOverviewView]
type GetLeadHandler = commands.QueryHandler[GetLeadQuery, commands.LeadView]
type GetTransactionHandler = commands.QueryHandler[GetTransactionQuery, commands.TransactionView]
type GetTransactionReviewHandler = commands.QueryHandler[GetTransactionReviewQuery, TransactionReviewView]
type ListAIDecisionsHandler = commands.QueryHandler[ListAIDecisionsQuery, commands.ListResult[commands.AIDecisionView]]
type ListAccessibleBusinessesHandler = commands.QueryHandler[ListAccessibleBusinessesQuery, commands.ListResult[BusinessMembershipView]]
type ListAttributeSchemasHandler = commands.QueryHandler[ListAttributeSchemasQuery, commands.ListResult[commands.AttributeSchemaView]]
type ListAuditEventsHandler = commands.QueryHandler[ListAuditEventsQuery, commands.ListResult[commands.AuditEventView]]
type ListCatalogItemsHandler = commands.QueryHandler[ListCatalogItemsQuery, commands.ListResult[commands.CatalogItemView]]
type ListCatalogsHandler = commands.QueryHandler[ListCatalogsQuery, commands.ListResult[commands.CatalogView]]
type ListChannelConnectionsHandler = commands.QueryHandler[ListChannelConnectionsQuery, commands.ListResult[commands.ChannelConnectionView]]
type ListConversationMessagesHandler = commands.QueryHandler[ListConversationMessagesQuery, commands.ListResult[commands.MessageView]]
type ListConversationsHandler = commands.QueryHandler[ListConversationsQuery, commands.ListResult[commands.ConversationView]]
type ListCustomerConversationsHandler = commands.QueryHandler[ListCustomerConversationsQuery, commands.ListResult[commands.ConversationView]]
type ListCustomerTransactionsHandler = commands.QueryHandler[ListCustomerTransactionsQuery, commands.ListResult[commands.TransactionView]]
type ListCustomersHandler = commands.QueryHandler[ListCustomersQuery, commands.ListResult[commands.CustomerView]]
type ListLeadAttributionsHandler = commands.QueryHandler[ListLeadAttributionsQuery, commands.ListResult[LeadAttributionView]]
type ListLeadScoresHandler = commands.QueryHandler[ListLeadScoresQuery, commands.ListResult[LeadScoreView]]
type ListLeadsHandler = commands.QueryHandler[ListLeadsQuery, commands.ListResult[commands.LeadView]]
type ListOffersHandler = commands.QueryHandler[ListOffersQuery, commands.ListResult[commands.OfferView]]
type ListTransactionsHandler = commands.QueryHandler[ListTransactionsQuery, commands.ListResult[commands.TransactionView]]
type ListVariantsHandler = commands.QueryHandler[ListVariantsQuery, commands.ListResult[commands.VariantView]]
## HTTP named inputs
type AIDecisionListInput struct {
type AIHumanInput struct {
type AttributeDefinitionInput struct {
type AttributeSchemaCreateInput struct {
type AttributeSchemasInput struct {
type AuditListInput struct {
type BusinessBodyInput struct {
type BusinessListInput struct {
type BusinessPolicyInput struct {
type BusinessUpdateInput struct {
type CatalogCreateInput struct {
type CatalogItemCreateInput struct {
type CatalogItemUpdateInput struct {
type CatalogItemsInput struct {
type CatalogUpdateInput struct {
type ChatwootWebhookInput struct {
type ConnectionActionInput struct {
type ConnectionCapabilityInput struct{ ConnectionPath }
type ConnectionCreateInput struct {
type ConnectionListInput struct {
type ContactPointInput struct {
type ConversationAssignInput struct {
type ConversationInput struct{ ConversationPath }
type ConversationLabelsInput struct {
type ConversationListInput struct {
type ConversationMessageInput struct {
type ConversationMessageListInput struct {
type ConversationNoteInput struct {
type ConversationUpdateInput struct {
type CreateCustomerInput struct {
type CustomerConversationsInput struct {
type CustomerInput struct{ CustomerPath }
type CustomerListInput struct {
type CustomerMergeInput struct {
type CustomerTransactionsInput struct {
type CustomerUpdateInput struct {
type EmptyInput struct{}
type ItemOffersInput struct {
type ItemVariantsInput struct {
type LeadAttributionsInput struct {
type LeadCreateInput struct {
type LeadListInput struct {
type LeadLostInput struct {
type LeadQualifyInput struct {
type LeadScoresInput struct {
type LeadUpdateInput struct {
type LoginInput struct {
type LogoutInput struct{}
type OfferCreateInput struct {
type OfferUpdateInput struct {
type RefreshInput struct{}
type SocialWebhookInput struct {
type TransactionCancelInput struct {
type TransactionConfirmInput struct {
type TransactionCreateInput struct {
type TransactionLineInput struct {
type TransactionListInput struct {
type TransactionReviewDecisionInput struct {
type TransactionReviewSubmitInput struct {
type TransactionUpdateInput struct {
type VariantCreateInput struct {
type VariantUpdateInput struct {
