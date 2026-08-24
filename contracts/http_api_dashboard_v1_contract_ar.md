# عقد HTTP API — Mujeeb 24 Dashboard V1

## حالة العقد

**الحالة: مغلق تصميميًا ومحوّل إلى DTO-first source.** هذه الوثيقة هي المرجع التجاري بين Dashboard Frontend وMujeeb 24 Backend. مصدر التنفيذ هو Go Request/Response DTOs وoperation registration داخل `internal/adapters/primary/http/contract`، ومنها يتولد OpenAPI تلقائيًا. تعتمد V1 **JWT Access Tokens** كآلية Authentication. لا تكشف الوثيقة PostgreSQL أو SocialAPI أو Chatwoot.

> هذا العقد يحدد ما يستطيع Dashboard طلبه وما يراه. أما طريقة التنفيذ الداخلية فتظل مسؤولية Application وPorts وAdapters.

## 1. حدود النظام

```text
Mujeeb 24 Dashboard
        ↓ HTTP API
Primary HTTP Adapter
        ↓
Application Commands / Queries
        ↓
Domain + Application Ports
        ↓
PostgreSQL / SocialAPI / Chatwoot
```

التاجر يستخدم Dashboard مجيب 24 فقط. لا يتصل Dashboard بـChatwoot أو SocialAPI، ولا يستقبل Provider DTOs أو Provider secrets أو raw payloads أو LLM internals.

### ما يملكه هذا العقد

يملك العقد عمليات التاجر المتعلقة بـBusiness وConnections وInbox وCustomers وCatalog وLeads وCommercial Transactions وAI Decisions وAudit Queries.

### ما لا يملكه هذا العقد

لا يملك العقد SQL details أو Redis/Asynq jobs أو Chatwoot Admin API أو SocialAPI credentials أو LLM prompts أو Chain-of-Thought أو Provider webhook payloads. ولا يحول كل جدول إلى CRUD Endpoint بلا Use Case.

## 2. إصدار ومسارات API

Base path:

```text
/api/v1
```

جميع عمليات Dashboard التجارية تستخدم Business Scope صريحًا:

```text
/api/v1/businesses/{business_id}/...
```

وجود `business_id` في المسار لا يمنح الوصول. الخادم يتحقق من:

```text
Authenticated Principal
→ Business Membership
→ Role
→ Permission
→ Application Command / Query
```

لا يقبل الخادم `business_id` في Request Body لتحديد Tenant. إذا وجد في Body لأغراض أخرى، يرفضه أو يتجاهله وفق DTO contract؛ مصدر النطاق هو المسار والـauthenticated scope.

## 3. Authentication وTenant Context

### 3.1 HTTP boundary

كل Dashboard endpoint، ما عدا Health وWebhook، يحتاج Principal موثوقًا. تعتمد V1 **JWT Access Token** في:

```http
Authorization: Bearer <access-token>
X-Request-ID: <optional-client-request-id>
X-Correlation-ID: <optional-correlation-id>
```

الخادم ينشئ `request_id` إذا لم يرسله العميل، ويعيده في Response Header وResponse Body. `X-Correlation-ID` يربط Command وAudit وOutbox وProvider attempts، ولا يقبل قيمة تتجاوز حدود الحجم أو تحتوي بيانات سرية.

### 3.2 JWT Contract

نستخدم JWT موقّعًا بخوارزمية **EdDSA/Ed25519**، مع `kid` لتدوير المفاتيح. يحفظ المفتاح الخاص في Secret Store/KMS ولا يدخل المستودع أو Environment المطبوع في logs. يحتفظ الخادم بالمفاتيح العامة الحالية والسابقة خلال فترة التدوير حتى لا تنكسر Access Tokens القصيرة.

| العنصر | القرار V1 |
|---|---|
| نوع الرمز | JWT Access Token بصيغة Bearer |
| الخوارزمية | EdDSA/Ed25519 فقط؛ أي Algorithm آخر مرفوض |
| Access TTL | 15 دقيقة |
| Refresh | Refresh Token opaque، مخزن Hash فقط، مع Rotation عند كل استخدام |
| Refresh session | حد أقصى 30 يومًا وسياسة إبطال عند Logout أو reuse detection |
| Audience | `mujeeb24-dashboard` |
| Issuer | `mujeeb24-api` |
| التخزين في المتصفح | لا نضع Access Token في `localStorage`؛ يحتفظ به Frontend في الذاكرة، ويُرسل Bearer |
| Refresh Cookie | `HttpOnly` و`Secure` و`SameSite` وفق deployment، مع CSRF protection عند الحاجة |

Claims المسموح بها في Access Token:

```json
{
  "iss": "mujeeb24-api",
  "sub": "principal-uuid",
  "aud": "mujeeb24-dashboard",
  "exp": 0,
  "iat": 0,
  "nbf": 0,
  "jti": "access-token-id",
  "sid": "auth-session-id",
  "typ": "access",
  "auth_version": 1
}
```

لا نضع `business_id` أو Role أو Permissions داخل JWT كمصدر حقيقة. يستطيع المستخدم تبديل Business من Dashboard، وتغيير Membership يجب أن يصبح فعّالًا دون انتظار انتهاء Token؛ لذلك يحدد Route `business_id` ويعيد الخادم فحص Membership/Permission من Auth/Application boundary.

Middleware يتحقق بالترتيب التالي:

```text
اقرأ Bearer Token
→ اقرأ kid
→ اختر Public Key من Key Set
→ اقبل EdDSA فقط
→ تحقق من signature
→ تحقق من iss/aud/typ/sub/jti/nbf/exp
→ أنشئ Authenticated Principal
→ تحقق من Business Membership وPermission
→ مرر Request إلى Handler
```

رمز JWT لا يقرر أنه يملك Business. إذا انتهت صلاحيته نعيد `401 token_expired`، وإذا كان التوقيع أوIssuer أوAudience غير صحيح نعيد `401 invalid_token`. لا نعيد سببًا يكشف تفاصيل المفاتيح.

### 3.3 Auth Endpoints

هذه endpoints تثبت JWT transport contract، أما User وMembership وPassword Policy وAuth Storage فتحتاج Auth Contract مستقلًا قبل تنفيذها:

| Method | Path | Auth | Application operation | Success |
|---|---|---|---|---:|
| `POST` | `/api/v1/auth/login` | public | `AuthenticatePrincipal` | `200` |
| `POST` | `/api/v1/auth/refresh` | refresh cookie فقط | `RotateRefreshSession` | `200` |
| `POST` | `/api/v1/auth/logout` | access + refresh | `RevokeRefreshSession` | `204` |
| `GET` | `/api/v1/me` | JWT | `GetCurrentPrincipal` | `200` |
| `GET` | `/api/v1/me/businesses` | JWT | `ListAccessibleBusinesses` | `200` |

`login` يعيد Access Token ووقت انتهاءه وPrincipal projection، ويضع Refresh Token في Cookie آمن بدل إعادته في JSON. `refresh` يدور Refresh Token ويصدر Access Token جديدًا. عند اكتشاف reuse لرمز Refresh قديم، نلغي Session كاملة ونرجع `401 refresh_reuse_detected`.

هذه السياسة لا تضيف جداول Auth إلى Migration Schema المغلقة الآن؛ Auth persistence سيكون Contract/Migration منفصلًا قبل تنفيذ Login الحقيقي.

### 3.4 الأدوار

| الدور | الصلاحيات العامة |
|---|---|
| `owner` | كل عمليات Business، Connections، Policy، Approval، Audit |
| `admin` | الإدارة التشغيلية وConnections وCatalog وSales وInbox، دون تغيير ملكية الحساب |
| `agent` | Inbox وCustomers وLeads وDraft Transactions، وعمليات الإرسال المسموحة |
| `viewer` | قراءات Dashboard وInbox وCatalog وSales وAudit المسموح بها |

الصلاحية الدقيقة تُفحص في Application، ولا يكفي Role وحده عندما تكون العملية حساسة. مثال ذلك تأكيد Transaction أو تغيير Policy أو Customer Merge.

### 3.5 نتائج الهوية والنطاق

| الحالة | HTTP |
|---|---:|
| لا يوجد Principal | `401` |
| Principal موجود بلا Membership أو Permission | `403` أو `404` وفق سياسة عدم كشف المورد |
| Business غير موجود ضمن Scope | `404` |
| Business suspended/archived | القراءة المسموحة حسب السياسة، والـCommands التجارية ترفض غالبًا بـ`409` أو `422` |

## 4. قواعد Request وResponse المشتركة

### 4.1 Headers

| Header | الاستخدام |
|---|---|
| `Authorization` | هوية المستخدم أو الجلسة الموثوقة |
| `X-Request-ID` | معرف الطلب من العميل، ويُعاد كما هو بعد التحقق أو يُنشأ بديلًا |
| `X-Correlation-ID` | ربط آثار العملية عبر Application وAudit وOutbox |
| `Idempotency-Key` | إلزامي للـCommands ذات Side Effect المحدد أدناه |
| `If-Match` | مطلوب لتعديلات الحالة الحساسة ومنع Lost Updates |
| `Content-Type: application/json` | Request JSON في V1 |

### 4.2 ID ووقت

كل IDs التي يراها Dashboard هي Mujeeb internal IDs بصيغة UUID. Provider IDs وChatwoot IDs لا تكون هوية العرض الأساسية، وقد تظهر فقط كمراجع آمنة في Connection أو Reference projection.

كل timestamps في JSON تكون ISO-8601 UTC، مثل:

```text
2026-08-24T12:30:00Z
```

### 4.3 Response envelope

Response المفرد:

```json
{
  "data": {},
  "request_id": "req_..."
}
```

Response القائمة:

```json
{
  "data": [],
  "pagination": {
    "next_cursor": "opaque-cursor-or-null",
    "has_more": true
  },
  "request_id": "req_..."
}
```

لا يرسل الخادم `null` و`[]` بشكل متناقض دون تعريف؛ القوائم الفارغة تكون `[]`، وحقول الموارد الاختيارية تكون `null`.

### 4.4 Pagination

كل List Query تدعم:

```text
limit: default 25, maximum 100
cursor: opaque string
```

لا يفسر Frontend محتوى Cursor ولا يبنيه. الترتيب الافتراضي ثابت ومذكور في كل Query، وعادة يكون `updated_at DESC, id DESC` أو `created_at DESC, id DESC`.

الفلاتر لا تغير Tenant Scope. أي `customer_id` أو `connection_id` أو `catalog_id` في Query يجب أن يكون داخل Business نفسه وإلا تكون النتيجة `404` أو قائمة فارغة وفق سياسة المورد.

### 4.5 Error Contract

كل خطأ قابل للعرض:

```json
{
  "error": {
    "code": "validation_error",
    "message": "The request contains invalid fields",
    "fields": {
      "name": "must not be empty"
    },
    "details": {},
    "retryable": false
  },
  "request_id": "req_..."
}
```

`message` آمن للعرض ولا يحتوي SQL أو Stack Trace أو secrets. `details` لا يحتوي Provider raw payload. رموز الأخطاء الأساسية:

| HTTP | Error code examples | المعنى |
|---:|---|---|
| `400` | `malformed_request`, `invalid_json` | الطلب غير قابل للفهم |
| `401` | `unauthenticated` | لا توجد هوية موثوقة |
| `403` | `forbidden`, `approval_required` | الهوية موجودة لكن لا تملك الإذن |
| `404` | `not_found` | المورد غير موجود ضمن Scope |
| `409` | `conflict`, `idempotency_conflict`, `stale_resource`, `invalid_state_transition` | تعارض حالة أو طلب مكرر بمحتوى مختلف |
| `422` | `validation_error`, `business_rule_violation`, `evidence_required`, `availability_unknown` | الطلب مفهوم لكنه يخالف Domain/Application rule |
| `429` | `rate_limited` | تجاوز معدل الطلب |
| `500` | `internal_error` | خطأ غير متوقع، لا يعرض تفاصيله |
| `502/503` | `external_dependency_unavailable`, `provider_reconnect_required` | اعتماد خارجي مؤقت أو غير متاح |

### 4.6 Idempotency

نستخدم `Idempotency-Key` للعمليات التي قد يعيدها Frontend بعد Timeout أو Network retry. المفتاح لا يجعل العملية synchronous، ولا يعني أن Provider سلّم الرسالة.

القواعد:

1. نفس المفتاح مع نفس العملية وPayload مكافئ يعيد نفس Logical Result أو نفس `operation_reference`.
2. نفس المفتاح مع Payload مختلف يعيد `409 idempotency_conflict`.
3. المفتاح scoped داخل Business واسم العملية، ولا يكون Globalًا.
4. Frontend لا يعيد استخدام المفتاح لعملية تجارية مختلفة.
5. عمليات GET لا تحتاج المفتاح.
6. حفظ idempotency يكون في Application/Persistence boundary المناسبة، ولا نفترض جدولًا عامًا مستقلًا قبل Use Case.

العمليات التي تحتاجه محددة في جدول Endpoint أدناه.

### 4.7 Optimistic concurrency

Response الموارد التي يمكن تعديلها يعرض `etag` أو `resource_version` opaque. عند تعديل Conversation أو Transaction أو Policy أو Connection باستخدام `PATCH` أو Command حساس، يرسل Frontend:

```http
If-Match: "resource-version"
```

إذا تغير المورد منذ آخر قراءة، تكون النتيجة `409 stale_resource`. لا يعتمد Frontend على `updated_at` لتكوين ETag بنفسه.

## 5. Bootstrap وBusiness API

### 5.1 Current Principal

| Method | Path | Role | Application operation | Success |
|---|---|---|---|---:|
| `GET` | `/api/v1/me` | authenticated | `GetCurrentPrincipal` | `200` |
| `GET` | `/api/v1/me/businesses` | authenticated | `ListAccessibleBusinesses` | `200` |

`GET /me` يعيد `principal_id` وdisplay information والصلاحيات العامة دون Token أو Secret. `GET /me/businesses` يعيد Business memberships وRole وstatus لا قاعدة البيانات الداخلية.

### 5.2 Business وPolicy

| Method | Path | Role | Request | Application operation | Success |
|---|---|---|---|---|---:|
| `GET` | `/businesses/{business_id}` | all members | none | `GetBusiness` | `200` |
| `PATCH` | `/businesses/{business_id}` | owner/admin | profile fields فقط + `If-Match` | `UpdateBusinessProfile` | `200` |
| `GET` | `/businesses/{business_id}/policy` | owner/admin/viewer | none | `GetBusinessPolicy` | `200` |
| `PATCH` | `/businesses/{business_id}/policy` | owner/admin | policy fields + `If-Match` | `UpdateBusinessPolicy` | `200` |

لا يسمح `PATCH Business` بتغيير `id` أو التاريخ أو Sales state. تعديل Policy يسجل Audit، ولا يفعّل AI auto-confirmation وحده؛ Transaction وEvidence وAuthorization تبقى شروطًا مستقلة.

Request Policy:

```json
{
  "ai_mode": "assist|approval|restricted_auto|disabled",
  "default_human_review": true,
  "allow_auto_reply": false,
  "allow_auto_lead_creation": true,
  "allow_auto_transaction_draft": true,
  "allow_auto_confirmation": false
}
```

## 6. Channel Connections API

Dashboard يتعامل مع Connection Mujeeb، لا مع Provider credential. أي authorization URL أو provisioning result يكون Reference مؤقتًا وآمنًا، ولا يعاد Secret.

| Method | Path | Role | Request | Application operation | Success |
|---|---|---|---|---|---:|
| `GET` | `/businesses/{business_id}/channel-connections` | all members | filters + cursor | `ListChannelConnections` | `200` |
| `GET` | `/businesses/{business_id}/channel-connections/{id}` | all members | none | `GetChannelConnection` | `200` |
| `POST` | `/businesses/{business_id}/channel-connections` | owner/admin | provider/channel/display name | `BeginChannelConnection` | `201/202` |
| `POST` | `/businesses/{business_id}/channel-connections/{id}/reconnect` | owner/admin | optional reconnect reason | `ReconnectChannel` | `202` |
| `POST` | `/businesses/{business_id}/channel-connections/{id}/disconnect` | owner/admin | reason + `Idempotency-Key` | `DisconnectChannel` | `202` |
| `GET` | `/businesses/{business_id}/channel-connections/{id}/capabilities` | all members | none | `GetConnectionCapabilities` | `200` |

`POST create` لا يقبل `access_token` أو `secret` في Body. Request المبدئي:

```json
{
  "provider": "socialapi",
  "channel": "facebook|instagram|whatsapp",
  "display_name": "Facebook Page"
}
```

Response قد تكون:

```json
{
  "data": {
    "id": "...",
    "provider": "socialapi",
    "channel": "facebook",
    "status": "pending",
    "authorization": {
      "required": true,
      "url": "https://...",
      "expires_at": "..."
    }
  },
  "request_id": "..."
}
```

وجود `authorization.url` اختياري Provider-neutral. لا نثبت شكل OAuth داخل Domain أو Dashboard Contract. `reconnect` و`disconnect` تحتاجان Idempotency، وتعيدان `202` لأن فحص Provider أو provisioning قد يكون غير متزامن.

## 7. Dashboard Overview API

| Method | Path | Role | Application operation | Success |
|---|---|---|---|---:|
| `GET` | `/businesses/{business_id}/dashboard/overview` | all members | `GetDashboardOverview` | `200` |

Query parameters الاختيارية:

```text
from
until
timezone=business|UTC
```

Response Projection يعرض أرقامًا آمنة مثل `open_conversations` و`waiting_human` و`new_leads` و`transactions_needing_review` وConnection health summary. لا يعيد SQL rows ولا يخلط Workspace counts مع Sales truth دون تسمية واضحة.

## 8. Conversations وInbox API

### 8.1 القراءة

| Method | Path | Role | Query/Request | Application operation | Success |
|---|---|---|---|---|---:|
| `GET` | `/businesses/{business_id}/conversations` | all members | state, ownership, channel, assignee, customer_id, cursor, limit | `ListConversations` | `200` |
| `GET` | `/businesses/{business_id}/conversations/{id}` | all members | none | `GetConversation` | `200` |
| `GET` | `/businesses/{business_id}/conversations/{id}/messages` | all members | cursor, limit | `ListConversationMessages` | `200` |

Conversation Projection:

```json
{
  "id": "...",
  "customer": { "id": "...", "display_name": "..." },
  "state": "open|ai_handling|waiting_customer|waiting_human|human_handling|closed",
  "ownership": "none|ai|human",
  "ai_mode": "allowed|draft_only|disabled",
  "priority": "low|normal|high|urgent",
  "assignment": { "reference": "...", "display_name": "..." },
  "last_activity_at": "...",
  "resource_version": "..."
}
```

لا يعيد `chatwoot_conversation_id` كهوية رئيسية. يمكن إظهار `external_references` بصيغة آمنة إذا احتاجت شاشة التشخيص ذلك وبصلاحية مناسبة.

### 8.2 Commands

| Method | Path | Role | Request | Idempotency/Concurrency | Application operation | Success |
|---|---|---|---|---|---|---:|
| `PATCH` | `/businesses/{business_id}/conversations/{id}` | agent/admin | state, priority, ai_mode + `If-Match` | `If-Match` | `UpdateConversation` | `200` |
| `POST` | `/businesses/{business_id}/conversations/{id}/assign` | agent/admin | assignee_reference + `If-Match` | `If-Match` | `AssignConversation` | `200/202` |
| `POST` | `/businesses/{business_id}/conversations/{id}/labels` | agent/admin | `{ "add": [], "remove": [] }` | `Idempotency-Key` | `UpdateConversationLabels` | `200/202` |
| `POST` | `/businesses/{business_id}/conversations/{id}/notes` | agent/admin | `{ "text": "..." }` | `Idempotency-Key` | `AddPrivateNote` | `202` |
| `POST` | `/businesses/{business_id}/conversations/{id}/messages` | agent/admin، وفق policy | content + optional client reference | `Idempotency-Key` | `CreateOutboundMessage` | `202` |

إرسال الرسالة لا يقبل `connection_id` أو `provider_message_id` من Frontend. Application يحدد Connection وConversation Reference. Request V1 text-only:

```json
{
  "content": {
    "type": "text",
    "text": "مرحبًا، كيف نساعدك؟"
  }
}
```

Response:

```json
{
  "data": {
    "message_id": "...",
    "conversation_id": "...",
    "status": "pending",
    "correlation_id": "...",
    "created_at": "..."
  },
  "request_id": "..."
}
```

`202` تعني أن Mujeeb حفظ Outbound intent وOutbox obligation، ولا تعني `sent` أو `delivered`. حالات الرسالة اللاحقة هي `pending`, `sending`, `accepted`, `sent`, `delivered`, `read`, `failed`, `unknown`.

## 9. Customers API

Customer قد يأتي من Channel، لكن Dashboard يستطيع عرضه وتحديث profile fields المسموحة. لا يستخدم Customer Chatwoot Contact كهوية.

| Method | Path | Role | Request/Query | Application operation | Success |
|---|---|---|---|---|---:|
| `GET` | `/businesses/{business_id}/customers` | all members | search, status, cursor, limit | `ListCustomers` | `200` |
| `POST` | `/businesses/{business_id}/customers` | agent/admin | profile/contact points | `CreateCustomer` | `201` |
| `GET` | `/businesses/{business_id}/customers/{id}` | all members | none | `GetCustomer` | `200` |
| `PATCH` | `/businesses/{business_id}/customers/{id}` | agent/admin | allowed profile fields + `If-Match` | `UpdateCustomer` | `200` |
| `GET` | `/businesses/{business_id}/customers/{id}/conversations` | all members | cursor, limit | `ListCustomerConversations` | `200` |
| `GET` | `/businesses/{business_id}/customers/{id}/transactions` | permitted members | cursor, limit | `ListCustomerTransactions` | `200` |
| `POST` | `/businesses/{business_id}/customers/{id}/merge` | owner/admin | target_customer_id + reason + `If-Match` | `MergeCustomer` | `202` |

`merge` تحتاج `Idempotency-Key` وAudit، ولا تسمح بBusiness مختلف أو دمج بالاسم وحده. لا يوجد `DELETE customer` عادي؛ lifecycle هو archive/merge وفق Policy.

## 10. Catalog API

### 10.1 Catalogs وSchemas

| Method | Path | Role | Request/Query | Application operation | Success |
|---|---|---|---|---|---:|
| `GET` | `/businesses/{business_id}/catalogs` | all members | status, cursor, limit | `ListCatalogs` | `200` |
| `POST` | `/businesses/{business_id}/catalogs` | admin/owner | name, description | `CreateCatalog` | `201` |
| `GET` | `/businesses/{business_id}/catalogs/{id}` | all members | none | `GetCatalog` | `200` |
| `PATCH` | `/businesses/{business_id}/catalogs/{id}` | admin/owner | name, description, status + `If-Match` | `UpdateCatalog` | `200` |
| `GET` | `/businesses/{business_id}/attribute-schemas` | all members | name, version, cursor, limit | `ListAttributeSchemas` | `200` |
| `POST` | `/businesses/{business_id}/attribute-schemas` | admin/owner | name, definitions | `CreateAttributeSchemaVersion` | `201` |
| `GET` | `/businesses/{business_id}/attribute-schemas/{id}` | all members | none | `GetAttributeSchema` | `200` |

إنشاء نسخة Schema جديدة لا يعدل Version منشورة in-place. `attribute_key` فريد داخل Schema Version، و`data_type` من الأنواع المغلقة في Domain Contract.

### 10.2 Items وOffers وVariants

| Method | Path | Role | Request/Query | Application operation | Success |
|---|---|---|---|---|---:|
| `GET` | `/businesses/{business_id}/catalogs/{catalog_id}/items` | all members | status, search, cursor, limit | `ListCatalogItems` | `200` |
| `POST` | `/businesses/{business_id}/catalogs/{catalog_id}/items` | admin/owner | item fields + schema reference | `CreateCatalogItem` | `201` |
| `GET` | `/businesses/{business_id}/catalogs/{catalog_id}/items/{item_id}` | all members | none | `GetCatalogItem` | `200` |
| `PATCH` | `/businesses/{business_id}/catalogs/{catalog_id}/items/{item_id}` | admin/owner | editable fields + `If-Match` | `UpdateCatalogItem` | `200` |
| `POST` | `/businesses/{business_id}/catalog-items/{item_id}/offers` | admin/owner | pricing/availability/fulfillment | `CreateOffer` | `201` |
| `GET` | `/businesses/{business_id}/catalog-items/{item_id}/offers` | all members | status, cursor, limit | `ListOffers` | `200` |
| `PATCH` | `/businesses/{business_id}/offers/{offer_id}` | admin/owner | offer fields + `If-Match` | `UpdateOffer` | `200` |
| `POST` | `/businesses/{business_id}/catalog-items/{item_id}/variants` | admin/owner | name, typed attributes | `CreateVariant` | `201` |
| `GET` | `/businesses/{business_id}/catalog-items/{item_id}/variants` | all members | status, cursor, limit | `ListVariants` | `200` |
| `PATCH` | `/businesses/{business_id}/variants/{variant_id}` | admin/owner | fields + `If-Match` | `UpdateVariant` | `200` |

لا يوجد حذف تدميري لـCatalog أو Item أو Offer أو Variant بعد دخولها في Transaction؛ نستخدم `archived` أو status transition. `attributes` لا تعتبر صحيحة لمجرد أنها JSON؛ Application يتحقق منها مقابل `attribute_schema_id` و`attribute_schema_version`.

## 11. Leads API

| Method | Path | Role | Request/Query | Application operation | Success |
|---|---|---|---|---|---:|
| `GET` | `/businesses/{business_id}/leads` | all members | status, customer_id, score_band, cursor, limit | `ListLeads` | `200` |
| `POST` | `/businesses/{business_id}/leads` | agent/admin | customer_id + context | `CreateLead` | `201` |
| `GET` | `/businesses/{business_id}/leads/{id}` | all members | none | `GetLead` | `200` |
| `PATCH` | `/businesses/{business_id}/leads/{id}` | agent/admin | allowed fields + `If-Match` | `UpdateLead` | `200` |
| `POST` | `/businesses/{business_id}/leads/{id}/qualify` | agent/admin أو policy | reason/context + `If-Match` | `QualifyLead` | `200` |
| `POST` | `/businesses/{business_id}/leads/{id}/mark-lost` | agent/admin | lost_reason + `If-Match` | `MarkLeadLost` | `200` |
| `GET` | `/businesses/{business_id}/leads/{id}/attributions` | all members | cursor, limit | `ListLeadAttributions` | `200` |
| `GET` | `/businesses/{business_id}/leads/{id}/scores` | all members | cursor, limit | `ListLeadScores` | `200` |

`qualify` و`mark-lost` تحتاجان `Idempotency-Key` إذا أرسلها Frontend كCommand قابل للإعادة. Attribution وScore history قراءات أو آثار Application، ولا يسمح Dashboard بتعديلها كـCRUD عام.

## 12. Commercial Transactions API

### 12.1 القراءة والإنشاء

| Method | Path | Role | Request/Query | Application operation | Success |
|---|---|---|---|---|---:|
| `GET` | `/businesses/{business_id}/transactions` | permitted members | state, type, customer_id, cursor, limit | `ListTransactions` | `200` |
| `POST` | `/businesses/{business_id}/transactions` | agent/admin | draft fields + line snapshots | `CreateTransactionDraft` | `201` |
| `GET` | `/businesses/{business_id}/transactions/{id}` | permitted members | none | `GetTransaction` | `200` |
| `PATCH` | `/businesses/{business_id}/transactions/{id}` | agent/admin | draft fields + `If-Match` | `UpdateTransactionDraft` | `200` |
| `GET` | `/businesses/{business_id}/transactions/{id}/reviews` | permitted members | none | `GetTransactionReview` | `200` |

`POST /transactions` لا ينشئ Transaction confirmed. يبدأ عادة بـ`draft` أو `needs_information`. لا يسمح Frontend بإرسال `state=confirmed` في Create/PATCH.

Request مختصر:

```json
{
  "customer_id": "...",
  "lead_id": "...",
  "transaction_type": "order|booking|appointment|service_request|reservation|quote|subscription",
  "source_conversation_reference_id": "...",
  "currency": "YER",
  "lines": [
    {
      "catalog_item_id": "...",
      "offer_id": "...",
      "variant_id": "...",
      "quantity": 1,
      "selected_attributes": {}
    }
  ]
}
```

Backend هو الذي يبني snapshots ويتحقق من Catalog/Offer/Variant وBusiness boundary. لا يقبل `unit_price_snapshot` من Frontend كحقيقة نهائية؛ السعر يأتي من Offer/Evidence أو يصبح Quote Required.

### 12.2 Commands الحساسة

| Method | Path | Role | Request | Required headers | Application operation | Success |
|---|---|---|---|---|---|---:|
| `POST` | `/businesses/{business_id}/transactions/{id}/confirm` | owner/admin أو صلاحية صريحة | confirmation evidence + expected state | `Idempotency-Key`, `If-Match` | `ConfirmTransaction` | `200/202` |
| `POST` | `/businesses/{business_id}/transactions/{id}/cancel` | agent/admin وفق policy | reason | `Idempotency-Key`, `If-Match` | `CancelTransaction` | `200/202` |
| `POST` | `/businesses/{business_id}/transactions/{id}/submit-review` | agent/admin | reason_codes | `Idempotency-Key`, `If-Match` | `SubmitTransactionReview` | `200` |
| `POST` | `/businesses/{business_id}/transactions/{id}/reviews/approve` | owner/admin | reviewer reference + decision | `Idempotency-Key`, `If-Match` | `ApproveTransactionReview` | `200` |
| `POST` | `/businesses/{business_id}/transactions/{id}/reviews/reject` | owner/admin | reason | `Idempotency-Key`, `If-Match` | `RejectTransactionReview` | `200` |

`confirm` لا ينجح إذا كان السعر أو Availability غير صالحين أو كانت Human Review مطلوبة ولم تعتمد أو كانت Policy تمنع العملية. ثقة AI لا تعتبر Confirmation.

إذا أدى Confirm أو Cancel إلى Outbound أو Provider effect، يعيد الخادم `202` مع operation reference، ولا يساوي ذلك نجاح Provider النهائي.

## 13. AI Decisions API

في V1، Dashboard يقرأ Structured AI Decisions ويرى سبب طلب Human Review. لا يستدعي LLM ولا يرسل Prompt ولا ينفذ Action مباشرة.

| Method | Path | Role | Query/Request | Application operation | Success |
|---|---|---|---|---|---:|
| `GET` | `/businesses/{business_id}/ai-decisions` | owner/admin/agent وفق PII policy | lifecycle, conversation_id, requires_human, cursor, limit | `ListAIDecisions` | `200` |
| `GET` | `/businesses/{business_id}/ai-decisions/{id}` | owner/admin/agent وفق PII policy | none | `GetAIDecision` | `200` |
| `POST` | `/businesses/{business_id}/ai-decisions/{id}/request-human` | agent/admin | reason + `If-Match` | `RequestHumanReview` | `200/202` |

لا ننشئ الآن `approve-ai` أو `execute-ai-action` لأن موافقة AI ليست Authorization ولا يوجد بعد Action execution contract مستقل. عندما يظهر Use Case محدد، نضيف Command صريحًا مثل `send-message` أو `create-transaction-draft`، لا Endpoint عامًا ينفذ أي AI action.

## 14. Audit API

| Method | Path | Role | Query/Request | Application operation | Success |
|---|---|---|---|---|---:|
| `GET` | `/businesses/{business_id}/audit-events` | owner/admin، وviewer حسب policy | actor, action, resource_type, from, until, cursor, limit | `ListAuditEvents` | `200` |
| `GET` | `/businesses/{business_id}/audit-events/{id}` | owner/admin | none | `GetAuditEvent` | `200` |

لا يوجد `POST/PATCH/DELETE` لـAudit من Dashboard. Audit Event append-only، و`decision_audits` ليس Endpoint أو جدولًا إلزاميًا في V1؛ إذا ظهر Read Model لاحقًا يكون Projection من `audit_events`.

## 15. Webhook Boundaries

Webhooks ليست Dashboard API ولا تستخدم Business ID القادم من العميل.

| Method | Path | Auth | Operation | Response |
|---|---|---|---|---:|
| `POST` | `/api/v1/webhooks/socialapi/{route_key}` | Provider signature على raw body | verify → decode → `RecordIfAbsent` | `2xx` بعد durable commit |
| `POST` | `/api/v1/webhooks/chatwoot/{route_key}` | Chatwoot HMAC على raw body | verify → decode → workspace event intake | `2xx` بعد durable commit |

القواعد:

```text
raw body validation
→ signature verification
→ provider/account reference extraction
→ Event Ledger insert if absent
→ database commit
→ ACK
→ asynchronous processing
```

إذا لم يُحسم Business أو Connection، يحفظ الحدث `unresolved`. لا يعيد Webhook raw payload إلى Dashboard، ولا ينشئ Conversation داخل HTTP request، ولا ينفذ Provider side effect قبل ACK.

`route_key` ليس `business_id` ولا Secret Provider. توقيع Provider والـConnection mapping هما مصدر الثقة.

## 16. Operational Endpoints خارج Dashboard Contract

هذه ليست Merchant API، لكنها ضرورية للتشغيل:

| Method | Path | الاستخدام |
|---|---|---|
| `GET` | `/health/live` | Process liveness، بلا اعتماد خارجي |
| `GET` | `/health/ready` | Readiness للـPostgreSQL والاعتمادات المطلوبة |
| `GET` | `/metrics` | داخلي ومقيد، إذا فُعّل Observability endpoint |

لا يعاد فيها Business data أو secrets.

## 17. Matrix مختصر للـIdempotency

| العملية | Idempotency-Key | If-Match | Side Effect محتمل |
|---|---:|---:|---|
| Create Connection | نعم | لا | provisioning خارجي |
| Disconnect/Reconnect | نعم | اختياري/مطلوب حسب state | Provider operation |
| Send Message | نعم | Conversation version | Outbound + Provider send |
| Add Note/Labels | نعم | Conversation version | Workspace mutation |
| Customer Merge | نعم | Customer version | نقل references + Audit |
| Create Lead | نعم | لا | Lead creation |
| Qualify/Mark Lost | نعم | نعم | Lead state change |
| Create Transaction Draft | نعم | لا | Transaction + snapshots |
| Confirm/Cancel Transaction | نعم | نعم | Commercial state + possible Outbox |
| Submit/Approve/Reject Review | نعم | نعم | Sensitive state change |
| Update Policy | لا أو نعم حسب implementation | نعم | Audit + policy state |
| GET queries | لا | لا | لا يوجد |

## 18. قواعد منع الخلط بين الطبقات

### HTTP Adapter

يقرأ HTTP ويحقق شكل JSON وHeaders وPath Parameters، ثم يحول DTO إلى Command/Query. لا يكتب SQL ولا يستدعي SocialAPI أو Chatwoot مباشرة.

### Application

يتحقق من Principal وBusiness Scope وPermission وIdempotency وDomain Rules، وينسق Transaction وOutbox وPorts.

### Domain

يقرر invariants وstate transitions، ولا يعرف HTTP status أو Provider DTO أو Chatwoot.

### Persistence Adapter

يطبق Repositories وEventStore وOutboxStore على PostgreSQL، ولا يقرر Use Case.

### Provider/Workspace Adapters

تطبق Ports فقط. SocialAPI ينقل القنوات الخارجية، وChatwoot ينفذ Workspace mirror/operations. لا يصبح أي منهما Sales Truth.

## 19. Deferred Scope — لا نضيفه الآن

هذه العناصر ليست Endpoints مفقودة؛ هي مؤجلة عمدًا حتى يغلق Use Case/Contract الخاص بها:

```text
Auth provider-specific login/callback endpoints
Subscription/Billing
Payments and refunds
Delivery tracking
Travel booking details
Clinic appointment details
Restaurant kitchen workflow
Media upload/attachment API
Bulk import/export
Advanced reports
Native Chatwoot custom channel controls
Generic AI execute endpoint
```

إضافة أي Endpoint من هذه القائمة تحتاج Domain/Application Contract واختبارًا، لا مجرد إضافة Route.

## 20. معايير إغلاق HTTP API Contract

لا يعتبر Endpoint مغلقًا إلا إذا حُسمت البنود التالية:

```text
Method + Path
Auth + Role + Permission
Business Scope
Request DTO
Field validation
Application Command/Query
Domain/Application rejection cases
Persistence effect
External side effect
Idempotency rule
Concurrency rule
Response projection
Status codes
Error codes
Audit requirement
```

والعقد الحالي يحقق ذلك للـCore Dashboard وWebhooks، مع Deferred Scope موثق بدل التخمين.

## 21. المرحلة التالية

بعد اعتماد هذه الوثيقة:

```text
HTTP API Contract
→ Go shared/endpoint DTOs وoperation registration
→ generated OpenAPI v1
→ shared Error/Pagination/ID validation
→ route/handler skeletons
→ Application Commands/Queries
→ PostgreSQL Adapter + Reliability implementation
```

لا نكتب Handlers التنفيذية قبل مراجعة Go DTO source وgenerated OpenAPI مرة واحدة. لا نعيد فتح Domain أو Persistence بسبب اختلاف تسمية في DTO؛ نضيف Mapping واضحًا. ملف `api/openapi/mujeeb24-dashboard-v1.generated.yaml` artifact مولد ولا يُحرر يدويًا.

## References

- `contracts/domain_shared_business_contract_ar.md`
- `contracts/domain_channel_contract_review_ar.md`
- `contracts/domain_identity_communication_contract_review_ar.md`
- `contracts/domain_catalog_contract_review_ar.md`
- `contracts/domain_sales_contract_review_ar.md`
- `contracts/domain_ai_audit_contract_review_ar.md`
- `contracts/application_ports_contract_review_ar.md`
- `contracts/persistence_application_boundary_review_ar.md`
- `docs/migration-execution-policy-ar.md`
