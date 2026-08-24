# مراجعة المدير — Persistence Model وSQL Migrations

## الحكم التنفيذي

الحزمة المرفقة **قوية ومتماسكة بنسبة كبيرة**، وتطابق Domain وPorts وBootstrap. وهي مناسبة للانتقال إلى SQL، لكن لا نكتب migrations حرفيًا قبل تصحيح بعض القيود؛ لأن ثلاثة منها قد تؤدي إلى تكرار رسائل أو ضعف عزل المستأجرين:

1. استعمال `connection_id` nullable داخل مفتاح Idempotency.
2. استعمال أعمدة متعددة nullable داخل Unique Constraint للقنوات.
3. Unique mapping للمحادثات لا يحدد الحساب الخارجي/الـconnection بشكل كافٍ.

بعد التصحيحات التالية يصبح Persistence Model Contract مغلقًا للتنفيذ.

## ما نعتمده

| القرار | الحكم |
|---|---|
| PostgreSQL مصدر الحقيقة | معتمد |
| SQL migrations versioned | معتمد |
| لا AutoMigrate | معتمد |
| كل Domain row تجاري tenant-scoped | معتمد |
| Chatwoot/SocialAPI IDs مراجع فقط | معتمد |
| Event Ledger دائم في PostgreSQL | معتمد |
| Outbox دائم وAsynq منفذ فقط | معتمد |
| OutboundMessage منفصل عن OutboxEntry | معتمد |
| Snapshots للمعاملة | معتمد |
| عدم إنشاء جداول Booking/Appointment بلا Workflow | معتمد |
| Typed attributes وSchema version | معتمد |
| AI confidence لا تساوي confirmation | معتمد |

## التصحيحات غير القابلة للتفاوض

### 1. هوية الاتصال الخارجي يجب ألا تعتمد على Nullable Columns

القيد المقترح:

```text
UNIQUE (business_id, provider, channel,
        provider_account_id, provider_connection_id)
```

قد يكون غير كافٍ في PostgreSQL إذا كانت بعض الأعمدة `NULL`؛ إذ تسمح Unique constraints بتكرارات متعددة للقيم الفارغة.

العقد النهائي يحتاج مرجعًا خارجيًا canonical غير فارغ:

```text
channel_connections
├── id
├── business_id
├── provider
├── channel
├── external_account_ref NOT NULL
├── external_connection_ref NOT NULL
├── status
└── secret_reference
```

ثم:

```text
UNIQUE (provider, external_connection_ref)
```

وإذا كان نفس المرجع قد يظهر عند أكثر من Provider، يبقى `provider` جزءًا من القيد. أما `business_id` فيحمي من ربط نفس الاتصال بتاجرين مختلفين، ويجب أن يكون هناك قيد إضافي يمنع ذلك حسب طبيعة Provider.

### 2. Event Ledger يحتاج Provider Connection Reference دائمًا

لا ننتظر `business_id` أو internal `connection_id` إذا كان mapping متعطلًا.

العقد المقترح:

```text
inbound_event_ledger
├── provider NOT NULL
├── provider_connection_ref NOT NULL
├── connection_id NULLABLE
├── business_id NULLABLE
├── provider_event_id NOT NULL
├── status
└── payload_reference
```

والمفتاح الذري:

```text
UNIQUE (provider, provider_connection_ref, provider_event_id)
```

بهذا نستطيع حفظ الحدث حتى لو لم نحل Business بعد. ثم تضيف Reconciliation `connection_id/business_id` عندما يصبح الربط ممكنًا.

إذا كان Provider لا يعطي Event ID، يجب أن يملك العقد fallback واضحًا ويصنف الحدث `dedupe_uncertain`; لا نستخدم `hash(message.text)` كهوية.

### 3. ACK بعد Ledger Insert فقط

التدفق التشغيلي الذي يجب أن يخدمه الـSchema:

```text
Verify signature
→ RecordIfAbsent
→ durable commit
→ ACK 2xx
→ resolve tenant/process asynchronously
```

لا ننفذ AI أو Chatwoot أو SocialAPI داخل Webhook request.

### 4. Conversation Reference يجب أن يحدد External Account

القيد:

```text
UNIQUE (business_id, provider,
        provider_account_ref, provider_conversation_id)
```

وإذا كانت المحادثة الخارجية مميزة على مستوى connection:

```text
UNIQUE (business_id, connection_id, provider_conversation_id)
```

لا يكفي:

```text
UNIQUE (conversation_id, provider)
```

لأن Provider واحدًا قد يملك عدة Pages/Accounts/Channels، ويجب ألا نخلط مراجعها.

### 5. Tenant Isolation يحتاج Composite Foreign Keys

وجود `business_id` في كل جدول وحده لا يمنع أن يشير `catalog_item` إلى `attribute_schema` من Business آخر إذا كان الـFK يستخدم `attribute_schema_id` فقط.

في الجداول الحساسة نحتاج إما:

```text
UNIQUE (id, business_id)
FOREIGN KEY (id, business_id)
  REFERENCES parent(id, business_id)
```

أو تصميمًا يجعل Business مشتقًا دون إمكانية crossing، مع اختبارات constraints. هذا مهم لـ:

```text
catalog → catalog_items → offers → variants
customer → external_identity
conversation → references
lead → attribution
transaction → order_lines
```

### 6. Outbound Idempotency يجب أن يكون scoped

لا نستخدم:

```text
UNIQUE (provider_idempotency_key)
```

بشكل عالمي؛ قد تتشابه المفاتيح بين Provider أو Connection مختلفين.

الأدق:

```text
UNIQUE (
  provider,
  connection_id,
  provider_idempotency_key
)
```

مع `business_id` للـtenant defense. ويجب أن تكون قيمة المفتاح deterministic لكل logical send، لا لكل retry.

### 7. Outbox يحتاج Dedupe Key وBusiness Scope وLease

`outbox_entries` يجب أن يتضمن:

```text
business_id
command_type
aggregate_type
aggregate_id
dedupe_key
status
attempt_count
next_attempt_at
lease_owner
lease_expires_at
```

وUnique constraint مناسب:

```text
UNIQUE (business_id, command_type, dedupe_key)
```

لا نعتمد على `aggregate_type + aggregate_id` وحدهما؛ فقد يحتاج Aggregate أكثر من عملية مشروعة.

### 8. Outbox وOutboundMessage حالتان مختلفتان

يجب أن يبقى ممكنًا أن يكون:

```text
OutboxEntry = completed
OutboundMessage = unknown
```

لأن Job نفذ محاولة، لكن نتيجة Provider غير محسومة. Worker عند إعادة تشغيل Job يفحص الحالة الحالية قبل أي side effect.

### 9. AuditEvent وDecisionAudit

لا ننشئ مصدر حقيقة مزدوجًا. إذا ظهر جدول `decision_audits` لاحقًا فهو Projection أو read model من `audit_events`، وليس سجلًا مستقلًا ينافسه.

`audit_events` يجب أن يكون append-only، ويحتوي `business_id` و`actor_type` و`correlation_id` و`causation_id` وsafe metadata، ولا يحتوي secrets أو Chain-of-Thought أو raw Provider payload كاملًا.

### 10. AI Decision Persistence

`entities` و`missing_information` يمكن أن تكون JSONB، لكن يجب أن تحمل:

```text
schema_version
source/evidence references
validation state
knowledge version
policy version
expires_at?
```

ولا نضع AI-generated price أو availability كحقيقة authoritative.

## مراجعة الجداول

### Businesses وPolicies

`businesses.id` هو Tenant Identity، و`slug` معرف URL فقط. `business_policies` one-to-one مناسب، لكن لا نضع فيه Provider credentials أو كامل AI model settings.

### Customers وExternal Identities

الفصل صحيح. `phone` و`email` indexes مساعدة وليست identity keys. `external_identities` يجب أن يفرض:

```text
UNIQUE (business_id, connection_id, external_user_id)
```

أو Provider Account Reference المكافئ، لا `provider/channel` فقط إذا كان للتاجر عدة اتصالات.

### Conversations

وجود `conversations` داخل Mujeeb مقبول إذا كان يمثل Sales Context وstate/ownership/ai_mode، وليس نسخة من Chatwoot. ولا ننشئ `chatwoot_messages` table في V1.

### Catalog

الترتيب صحيح:

```text
catalogs
→ catalog_items
→ offers
→ variants
```

لكن `CatalogItem` يجب أن يرث Business بشكل قابل للتحقق، و`Offer` و`Variant` لا يعبران Catalog/Business boundary. Attributes JSONB لا تكون حرة؛ تُتحقق من `attribute_schema_id/version` في Application/Domain.

`offers.amount` nullable عندما يكون السعر Dynamic أو Quote Required، مع حفظ pricing mode وvalidity وevidence.

### Sales

نبدأ بـ:

```text
commercial_transactions
order_lines
transaction_confirmations
transaction_reviews
```

ولا ننشئ `booking_details` أو `appointment_details` أو `subscription_details` قبل وجود workflow/invariants مستقلة.

`order_lines` تحتاج snapshots غير قابلة للتغيير للسعر والاسم والخصائص، ويفضل أن تكون `line_total_snapshot` محسوبة وفق Money contract لا Float.

### Audit وAI

`audit_events` و`ai_decisions` كافيان في V1. `decision_audits` لا يضاف إلا كProjection واضح.

## Migration Ordering النهائي المبدئي

نستخدم sequence واحدة فقط؛ لا نكرر migration لشرح أن Outbox لا يعتمد على Redis، لأن ذلك قرار معماري وليس جدولًا:

```text
000001 businesses
000002 business_policies
000003 channel_connections
000004 channel_connection_capabilities
000005 customers
000006 external_identities
000007 identity_matches
000008 conversations
000009 conversation_references
000010 inbound_event_ledger
000011 outbound_messages
000012 outbox_entries
000013 catalogs
000014 attribute_schemas
000015 attribute_definitions
000016 catalog_items
000017 offers
000018 variants
000019 leads
000020 lead_attributions
000021 lead_scores
000022 commercial_transactions
000023 transaction_confirmations
000024 transaction_reviews
000025 order_lines
000026 audit_events
000027 ai_decisions
```

نراجع الأرقام عند كتابة SQL الفعلي، لكن لا نخلط قائمة التخطيط مع migration files.

## معايير SQL قبل التنفيذ

1. كل row تجاري قابل للـtenant scoping.
2. كل external identity فريدة داخل Business وConnection.
3. كل event فريد بـProvider وProvider Connection Reference وEvent ID.
4. كل outbound logical send يملك scoped idempotency key.
5. كل Outbox row يملك dedupe key وlease semantics.
6. لا cross-business foreign keys.
7. لا Network calls داخل DB transaction.
8. لا ACK قبل durable ledger commit.
9. لا raw Provider database أو Chatwoot database داخل Mujeeb.
10. لا AutoMigrate.
11. لا جداول workflow لم تُغلق Domain Contracts الخاصة بها.
12. اختبارات القيود جزء من Migration Foundation.

## القرار النهائي

**أعتمد Persistence Model وMigration Design بعد هذه التصحيحات.** لا نكتب كل SQL في رسالة واحدة، ولا نبدأ Catalog/Sales أولًا.

الخطوة التنفيذية التالية هي كتابة ومراجعة أول Foundation migrations فقط:

```text
businesses
business_policies
channel_connections
channel_connection_capabilities
customers
external_identities
identity_matches
conversations
conversation_references
inbound_event_ledger
outbound_messages
outbox_entries
```

ثم نضيف اختبارًا لكل قيد: duplicate identity، duplicate event، duplicate outbound، cross-tenant reference، invalid foreign key، وlease conflict. بعدها نبدأ Event Ledger وIdempotency وOutbox implementation.
