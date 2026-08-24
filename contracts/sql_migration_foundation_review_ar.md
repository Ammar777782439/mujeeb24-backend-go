# مراجعة المدير — SQL Migration Foundation

## الحكم التنفيذي

التحليل المرفق **مناسب جدًا للانتقال من العقد إلى SQL فعلي**، لكنه ليس جاهزًا للنسخ إلى Production كما هو. توجد أخطاء تصميمية محددة يجب إصلاحها قبل كتابة أول migration، خصوصًا في **Tenant Isolation، Idempotency، مفاتيح القنوات، وOutbox leases**.

القرار: نعتمد Migration Foundation كخطة، وننفذ SQL بعد إغلاق التصحيحات التالية. لا ننتقل إلى Catalog أو AI قبل نجاح اختبارات Foundation.

## ما نعتمده

| القرار | الحكم |
|---|---|
| PostgreSQL مصدر الحقيقة | معتمد |
| UUID و`timestamptz` وSQL migrations | معتمد |
| لا AutoMigrate | معتمد |
| Business كجذر Tenant | معتمد |
| Chatwoot/SocialAPI كمراجع فقط | معتمد |
| عدم إنشاء `chatwoot_messages` | معتمد |
| OutboundMessage منفصل عن OutboxEntry | معتمد |
| Asynq منفذ لا مصدر حقيقة | معتمد |
| عدم إضافة جداول Workflow غير منفذة | معتمد |
| Contract tests للقيود | معتمد |

## التصحيحات الحرجة قبل SQL

### 1. `channel_connections` لا تستخدم Unique متعددًا مع مراجع nullable

النسخة المرفقة تستخدم `provider_account_id` و`provider_connection_id` كحقول نصية غير فارغة، وهذا جيد إذا كانت القيم canonical دائمًا. نثبت ذلك صراحة:

```text
provider_account_ref       NOT NULL
provider_connection_ref   NOT NULL
```

ونستخدم أسماء موحدة مع Domain Contract. لا نخلط `provider_account_id` في جدول و`provider_connection_ref` في Event Ledger بلا mapping واضح.

القيد المقترح:

```text
UNIQUE (provider, provider_connection_ref)
```

مع قيد يضمن أن الاتصال الخارجي لا يرتبط بأكثر من Business، حسب semantics الفعلية للProvider. `business_id` وحده داخل unique ليس كافيًا إذا كان نفس الاتصال يجب أن يكون ملك Business واحدًا عالميًا.

### 2. `external_identities` يجب أن ترتبط بـConnection لا Provider/Channel فقط

المفتاح المقترح:

```text
UNIQUE (
  business_id,
  provider,
  channel,
  external_account_id,
  external_user_id
)
```

مقبول فقط إذا كان `external_account_id` يساوي الحساب الذي يطابق `ChannelConnection`. الأفضل في Mujeeb:

```text
connection_id NOT NULL
UNIQUE (connection_id, external_user_id)
```

وإذا احتجنا provider/account references للبحث السريع، تبقى حقولًا مشتقة أو مراجع مع قيد اتساق. هذا يمنع ظهور هوية واحدة مرتين بسبب اختلاف تسمية Account ID.

### 3. Composite Foreign Keys إلزامية في العلاقات الحساسة

وجود `business_id` في كل جدول لا يمنع وحده crossing. يجب أن نضيف unique parent keys مثل:

```text
UNIQUE (id, business_id)
```

ثم:

```text
FOREIGN KEY (business_id, customer_id)
REFERENCES customers (business_id, id)
```

نطبق ذلك على:

```text
external_identities → customers
conversations → customers
conversation_references → conversations
leads → customers
transactions → customers/leads
catalog_items → catalogs
offers/variants → catalog_items
order_lines → transactions/catalog items
```

أو نضمن equivalent database constraints؛ لا نعتمد على Application checks فقط.

### 4. `identity_matches` يحتاج حماية Business واتجاهًا canonical

الـ`LEAST/GREATEST` unique index فكرة صحيحة، لكن PostgreSQL expressions تحتاج صياغة DDL دقيقة، ويجب التأكد أن الهويتين من Business نفسه:

```text
left_identity.business_id = right_identity.business_id = identity_matches.business_id
```

نفرض ذلك عبر Composite FKs أو تصميم parent key مناسب. كما نمنع pair مكررًا بغض النظر عن الاتجاه.

### 5. Event Ledger لا يجعل `connection_id` غير معروف مستحيلًا

النسخة المرفقة تجعل:

```text
connection_id UUID NOT NULL
```

لكن العقد السابق يسمح بحفظ الحدث قبل اكتمال Tenant/Connection resolution حتى لا نفقد Webhook. لذلك نحتاج:

```text
provider_connection_ref NOT NULL
connection_id UUID NULLABLE
business_id UUID NULLABLE
```

والمفتاح:

```text
UNIQUE (provider, provider_connection_ref, provider_event_id)
```

إذا كانت البنية تضمن resolve قبل insert دائمًا، يجب إثبات ذلك باختبار failure؛ وإلا نعتمد `unresolved/quarantine`.

### 6. Event Ledger يحتاج حالات وLease كاملة

الحالات ليست فقط `received/processing/processed/failed`. نحتاج على الأقل:

```text
received
unresolved
processing
processed
retryable_failed
dead_letter
rejected
```

ونحتاج:

```text
processing_owner
lease_expires_at
attempt_count
last_error_code
```

حتى لا يبقى سجل `processing` عالقًا بعد crash.

### 7. Outbound idempotency key يجب أن يكون Scoped

القيد العالمي:

```text
UNIQUE (provider_idempotency_key)
```

غير آمن إذا استخدم Providerان أو Connectionان نفس صيغة المفاتيح. نعتمد:

```text
UNIQUE (provider, connection_id, provider_idempotency_key)
```

ويجب أن يكون المفتاح deterministic لكل logical send، لا لكل retry.

### 8. `outbound_messages` تحتاج مرجع Connection وLease semantics

الجدول يحتاج، بالإضافة إلى Business وConversation:

```text
connection_id NOT NULL
correlation_id NOT NULL
causation_id?
```

حتى يعرف Worker أي اتصال خارجي يستخدم. `next_retry_at` وحده لا يكفي؛ يتم التحكم في claim/lease داخل Outbox أو DeliveryAttempt حسب العقد النهائي.

### 9. Outbox يحتاج Business وDedupe وLease Owner

النسخة المرفقة لا تحتوي `business_id` أو `dedupe_key` أو `lease_owner`. نضيفها:

```text
business_id
command_type
dedupe_key
lease_owner
lease_expires_at
```

مع:

```text
UNIQUE (business_id, command_type, dedupe_key)
```

ونفصل `OutboxEntry.status` عن `OutboundMessage.status`. قد تكون Outbox `completed` بينما Delivery `unknown`.

### 10. `locked_at` وحده ليس Lease

لا يكفي timestamp. العامل يحتاج Owner/Token ووقت انتهاء، وعمليات complete/fail يجب أن تتحقق من أن العامل الحالي يملك lease. وإلا يستطيع Worker قديم الكتابة فوق نتيجة Worker جديد.

### 11. `content_reference` لا يعني أن Content غير موجود

في أول Vertical Slice يمكن دعم structured text صغير داخل OutboundMessage أو Content Store محدود، لكن يجب قرار retention واضح. لا نضيف S3/MinIO قبل Use Case للوسائط أو raw payloads، ولا نخزن raw secrets في payload.

### 12. AI/Audit ترتيب التنفيذ

`ai_decisions` يمكن تأجيله إلى ما بعد Reliability Foundation. `audit_events` قد يبدأ مبكرًا فقط للعمليات الحساسة. و`decision_audits` ليس جدولًا منافسًا؛ يكون Projection من Audit canonical إذا احتجناه.

## تصحيح ترتيب Migration

الترتيب المرفق جيد، لكن لا نسمي قرارًا معماريًا Migration مستقلًا. نثبت Foundation هكذا:

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
```

بعد نجاح Contract Tests نضيف:

```text
catalog
sales
ai/audit
```

ولا نثبت أرقام Catalog/Sales النهائية قبل مراجعة Composite FKs وTyped IDs.

## اختبارات Foundation غير القابلة للتفاوض

| الاختبار | النتيجة المطلوبة |
|---|---|
| Duplicate external identity | Conflict واحد داخل نفس Connection |
| Same external user في Connections مختلفة | لا دمج تلقائي بلا Identity Policy |
| Duplicate provider event | صف Ledger واحد |
| Event بلا Tenant mapping | محفوظ `unresolved` ولا يضيع |
| Cross-tenant customer reference | مرفوض من DB |
| Cross-tenant catalog/offer reference | مرفوض من DB |
| Duplicate outbound logical send | صف واحد بحسب scoped key |
| Outbox + Outbound atomicity | كلاهما ينجح أو كلاهما يتراجع |
| Lease conflict | Worker واحد يملك المعالجة |
| Expired lease | Worker جديد يستعيد بأمان |
| Unknown delivery | لا إعادة إرسال أعمى |
| Migration rerun | آمن ومضبوط عبر migration tool |

## القرار النهائي

**أعتمد SQL Foundation كتصميم بعد هذه التصحيحات، ولا أعتمد SQL المرفق حرفيًا بعد.**

المسار الصحيح الآن:

```text
Finalize Persistence Model
→ Write migrations 000001–000012
→ Run PostgreSQL constraint tests
→ Implement Event Ledger
→ Implement Outbox + leases
→ Implement Asynq worker
→ Build FakeSocialProvider
```

لا نبدأ Chatwoot أو SocialAPI الحقيقي قبل نجاح هذه الطبقة.
