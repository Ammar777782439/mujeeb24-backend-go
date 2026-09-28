# Domain Contract — Shared وBusiness

## حالة الوثيقة

هذه وثيقة تصميم قبل الكود. لا تحتوي على Go implementation ولا SQL migration. لا ننتقل إلى `channel` قبل تثبيت هذا العقد.

## 1. قواعد Domain العامة

كل كيان Domain له هوية داخلية مستقلة، ولا يستخدم Provider ID أو Chatwoot ID كهوية له. الكيانات لا تعرف HTTP أو PostgreSQL أو Redis أو AI SDK.

كل Aggregate يطبق قواعده داخليًا. Application Service ينسق بين Aggregates، ولا ينشئ كائنات تتجاوز invariants الخاصة بها.

العلاقات بين Aggregates تكون عبر IDs أو References، وليس عبر object graph كبير:

```text
BusinessID
CustomerID
ChannelConnectionID
ConversationReferenceID
CatalogItemID
LeadID
TransactionID
```

## 2. shared/

### 2.1 Identifiers

نستخدم IDs داخلية غير قابلة للتخمين. الاختيار التنفيذي المقترح هو UUIDv7 أو بديل زمني آمن، محفوظ في PostgreSQL كـUUID. لا نستخدم أرقامًا متسلسلة كـpublic IDs.

أما IDs القادمة من Providers فتظل `ExternalID` نصية مع اسم provider وconnection scope:

```text
InternalID       = هوية Mujeeb
ProviderID       = هوية SocialAPI/القناة الخارجية
WorkspaceID      = هوية Chatwoot أو Communication Workspace
```

الـDomain لا يفترض أن هذه القيم قابلة للتبادل.

### 2.2 Money

`Money` Value Object وليس `float64`.

```text
Money
├── amount_minor: integer
└── currency: ISO currency code
```

القواعد:

| القاعدة | القرار |
|---|---|
| التخزين | integer بوحدة minor أو decimal مضبوط، لا float |
| العملة | رمز ISO مثل YER أو USD، ولا نضع العملة في Business logic كنص حر |
| المقارنة | لا نقارن مبلغين بعملتين مختلفتين |
| السعر | لا يكون سالبًا |
| الخصم/التعديل | يسجل كـAdjustment مستقل، ويمكن أن يكون سالبًا ضمن transaction حسابية |
| التحويل | لا يتم تلقائيًا داخل Domain دون سعر صرف ومصدر ووقت صلاحية |

العملة الافتراضية للتاجر يمكن أن تكون YER، لكن هذا إعداد Business وليس افتراضًا عالميًا لكل معاملة.

### 2.3 Time

كل timestamps في Domain هي UTC. يحتفظ Business بـtimezone للعرض وحساب المواعيد، لكن لا نخزن «وقت محلي بلا منطقة زمنية» في الأحداث.

نفرق بين:

```text
created_at          وقت إنشاء سجل Mujeeb
occurred_at         وقت وقوع Domain Event
external_created_at وقت الحدث عند Provider إن توفر
received_at         وقت وصوله إلى Go
```

لا نخلط هذه الأوقات في حقل واحد.

### 2.4 DomainError

الأخطاء العامة تكون typed ومحددة، ولا تحمل secrets أو raw payload:

```text
validation_error
invariant_violation
not_found
conflict
unauthorized_action
unsupported_capability
stale_data
external_dependency_error
transient_failure
permanent_failure
```

Application يترجمها إلى HTTP أو Job result. Domain لا يعرف HTTP status codes.

### 2.5 Pagination وReferences

قراءات Dashboard تستخدم cursor pagination. لا نعتمد على offset pagination في جداول الأحداث والرسائل ذات النمو المستمر.

Reference هو كائن صغير يربط كيانًا خارجيًا دون استيراد DTO الخارجي إلى Domain:

```text
ExternalReference
├── provider
├── external_id
└── scope
```

## 3. business/

### 3.1 Business Aggregate

يمثل مساحة التاجر في Mujeeb 24، وليس Chatwoot Account وليس SocialAPI workspace.

```text
Business
├── id
├── name
├── slug
├── status
├── vertical_type
├── timezone
├── default_currency
├── locale
├── created_at
└── updated_at
```

`vertical_type` هو Default Configuration فقط:

```text
retail
travel
services
restaurant
clinic
hospitality
education
real_estate
other
```

لا يسمح `vertical_type` بإيقاف أنواع CatalogItem التي يمكن للتاجر إنشاءها.

### 3.2 BusinessProfile

معلومات العرض التجاري العامة التي قد تظهر للعميل:

```text
BusinessProfile
├── display_name
├── description
├── phone
├── email
├── address_reference
├── logo_reference
└── public_links
```

لا نخزن channel credentials أو Chatwoot tokens داخل BusinessProfile.

### 3.3 BusinessPolicy

سياسات التشغيل التي يحتاجها Application وAI، مع عدم تنفيذ AI داخل Business Aggregate:

```text
BusinessPolicy
├── ai_mode: disabled | assist | approval | restricted_auto
├── default_human_review: boolean
├── allow_auto_reply: boolean
├── allow_auto_lead_creation: boolean
├── allow_auto_transaction_draft: boolean
├── allow_auto_confirmation: boolean
├── business_hours_reference
└── escalation_policy_reference
```

`allow_auto_confirmation` لا يمنح AI صلاحية تأكيد سعر أو توفر غير موثق؛ authorization النهائي يعتمد أيضًا على Catalog/Offer/Availability والسياسة الخاصة بالمعاملة.

### 3.4 BusinessStatus

```text
pending_setup
active
suspended
archived
```

الانتقالات المسموحة:

```text
pending_setup → active
pending_setup → archived
active → suspended
active → archived
suspended → active
suspended → archived
```

لا نعيد `archived` إلى `active` في V1؛ إعادة الفتح عملية إدارية منفصلة تحتاج قرارًا واضحًا لاحقًا.

### 3.5 Invariants

لا يصبح Business `active` إلا إذا كان لديه اسم صالح، timezone صالح، عملة افتراضية، وحساب مالك/إدارة صالح حسب Auth Contract. لا نربط نجاح Business بوجود Chatwoot أو SocialAPI؛ يمكن إنشاء Business قبل ربط القنوات.

لا يجوز تعديل `id` بعد الإنشاء. تغيير `vertical_type` لا يعيد كتابة Catalog Items القديمة ولا يغير transaction snapshots. تغيير العملة الافتراضية لا يغير العملات التاريخية.

لا يحذف Business حذفًا فعليًا بسبب وجود أحداث ومبيعات؛ الانتقال النهائي هو `archived` مع سياسات retention منفصلة.

### 3.6 Ownership وRelationships

Business هو مالك منطقي لـ:

```text
ChannelConnections
Customers
Catalogs
Leads
CommercialTransactions
BusinessPolicies
```

لكنه لا يضمها كقائمة داخل Aggregate في الذاكرة. كل وحدة تملك Aggregate مستقلًا وتربط بـ`business_id`.

Chatwoot Account وSocialAPI Account مراجع خارجية، وليست مالكًا للتاجر. لا نستنتج Business من display name أو من نص الرسالة.

### 3.7 Domain Events

الأحداث المقترحة:

```text
BusinessCreated
BusinessActivated
BusinessSuspended
BusinessReactivated
BusinessArchived
BusinessPolicyChanged
BusinessProfileUpdated
```

كل Event يحتوي على:

```text
id
aggregate_id
aggregate_type
event_type
occurred_at
business_id
schema_version
metadata
```

`metadata` لا يحتوي secrets ولا raw provider payload. الأحداث الداخلية تنشر عبر Outbox لاحقًا، ولا نربط Domain مباشرة بـAsynq.

## 4. قرارات نؤجلها صراحة

لا نغلق الآن User/Auth/Subscription ككيانات نهائية؛ لأنها تحتاج Contract مستقلًا. لا نضع Catalog داخل Business aggregate. لا نضع Chatwoot Team أو SocialAPI connection object داخل Business domain. ولا نضيف حقول القطاع مثل doctor أو flight أو stock إلى Business.

## 5. معايير قبول هذا الجزء

يُعتبر `shared` و`business` مغلقين عندما نستطيع كتابة Unit Tests لاحقًا تثبت:

1. رفض Money بعملة فارغة أو مبلغ غير صالح.
2. رفض مقارنة عملتين مختلفتين.
3. منع الانتقال غير المسموح في BusinessStatus.
4. منع تعديل Business ID والبيانات التاريخية التابعة له.
5. السماح بإنشاء Business قبل ربط أي قناة.
6. عدم تسرب Provider/Chatwoot DTO إلى Domain.
7. وجود schema version لكل Domain Event.

بعد اعتماد هذا الجزء ننتقل إلى `domain/channel` لأنه يعتمد على `BusinessID` و`ExternalReference` و`Timestamp` و`DomainError`.
