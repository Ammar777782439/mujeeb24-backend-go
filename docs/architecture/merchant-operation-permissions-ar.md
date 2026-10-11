# مصفوفة صلاحيات عمليات التاجر

## قاعدة التنفيذ

كل عملية تاجر تحت `/api/v1/businesses/{business_id}` تمر بالترتيب التالي:

`JWT principal → active membership → matching business_id → operation permission → handler`

السياسة `default deny`: أي `operationID` غير مصنف يرفض قبل استدعاء Application Handler. دور `owner` يملك كل الصلاحيات ضمن متجره، ولا يمكن منح `*` عبر Team API. يستطيع `admin` منح الموظف مجموعة فرعية فقط من صلاحياته الفعلية.

| الصلاحية | العمليات |
|---|---|
| `business:read` | `getBusiness`, `getBusinessPolicy` |
| `business:settings` | `updateBusinessProfile`, `updateBusinessPolicy` |
| `dashboard:read` | `getDashboardOverview` |
| `team:read` | `listTeamMembers`, `listTeamInvitations` |
| `team:manage` | `createTeamInvitation`, `revokeTeamInvitation`, `resendTeamInvitation`, `updateTeamMemberRole`, `updateTeamMemberAccess`, `revokeTeamMember` |
| `conversations:read` | `listConversations`, `getConversation`, `listConversationMessages`, `listCustomerConversations`, `markConversationRead` |
| `conversations:reply` | `createOutboundMessage`, `addPrivateNote`, `updateConversationLabels`, `sendCannedReply`, `listCannedReplies`, `createCannedReply`, `updateCannedReply` |
| `conversations:assign` | `assignConversation`, `updateConversation` |
| `customers:read` | `listCustomers`, `getCustomer` |
| `customers:write` | `createCustomer`, `updateCustomer`, `mergeCustomer` |
| `catalog:read` | `listCatalogs`, `getCatalog`, `getCatalogEntityContract`, `listCatalogItems`, `getCatalogItem`, `listOffers`, `listVariants`, `listAttributeSchemas`, `getAttributeSchema` |
| `catalog:write` | `createCatalog`, `updateCatalog`, `createCatalogItem`, `updateCatalogItem`, `createOffer`, `updateOffer`, `createVariant`, `updateVariant`, `createAttributeSchemaVersion` |
| `sales:read` | `listLeads`, `getLead`, `listLeadAttributions`, `listLeadScores`, `listTransactions`, `listCustomerTransactions`, `getTransaction`, `getTransactionReview` |
| `sales:write` | `createLead`, `updateLead`, `qualifyLead`, `markLeadLost`, `createTransactionDraft`, `updateTransactionDraft`, `confirmTransaction`, `cancelTransaction`, `submitTransactionReview` |
| `sales:approve` | `approveTransactionReview`, `rejectTransactionReview` |
| `channels:read` | `listChannelConnections`, `getChannelConnection`, `getConnectionCapabilities`, `previewChannelHistoryPurge` |
| `channels:manage` | `beginChannelConnection`, `reconnectChannel`, `disconnectChannel`, `purgeChannelHistory` |
| `automation:read` | `listAutomationRules` |
| `automation:manage` | `createAutomationRule`, `updateAutomationRule` |
| `audit:read` | `listAuditEvents`, `getAuditEvent`, `listAIDecisions`, `getAIDecision` |
| `merchant_ai:use` | `merchantAIChat`, `merchantAIExecute`, `requestHumanReview` |

عمليات المصادقة العامة أو الخاصة بالهوية لا تستخدم Business Scope: تسجيل الدخول والتجديد، فحص دعوة، قبول حساب جديد، وقبول حساب قائم بعد المصادقة. عمليات `/platform/*` لها Platform Scope منفصل ولا تمنح مدير المنصة عضوية تاجر.
