# مراجعة المدير — Full Migrations / Persistence Schema

## الحكم التنفيذي

الحزمة المرفقة **تصلح كمسودة Schema شاملة** وتغطي Foundation وCatalog وSales وAI وAudit، لكنها ليست جاهزة للنسخ إلى Production كما هي. نحتاج تصحيح قيود محددة قبل كتابة SQL النهائي، وإلا قد نقع في Cross-Tenant Reference أو duplicate outbound أو فقد أحداث غير مربوطة.

القرار: **لا نعيد تصميم Domain**. نحول العقود المغلقة إلى Persistence، لكن نصلح مفاتيح الربط والـleases والحذف وترتيب migrations.

## ملاحظة عن الملفات المرفقة

يوجد ملف Bootstrap مكرر، وملفان يقدمان Migration Foundation وامتدادها. لا نكرر هذه الملفات في المستودع. نستخدم سلسلة migrations واحدة فقط، ولا نكتب Migration لقرار معماري مثل «Outbox لا يعتمد على Redis».

## ما نعتمده

| المجال | القرار |
|---|---|
| PostgreSQL مصدر الحقيقة | معتمد |
| UUID يولد من Application | معتمد مبدئيًا |
| `timestamptz` | معتمد |
| Versioned SQL migrations | معتمد |
| Business كـTenant Root | معتمد |
| Composite Tenant FKs | إلزامي |
| Event Ledger دائم | معتمد |
| Outbox دائم | معتمد |
| Asynq منفذ Jobs فقط | معتمد |
| Chatwoot/SocialAPI references فقط | معتمد |
| عدم نسخ Messages كاملة إلى V1 | معتمد |
| عدم إنشاء تفاصيل Booking/Appointment بلا Workflow | معتمد |
| Contract tests للقيود | معتمد |

## التصحيحات الحرجة

### 1. لا نحذف Business بـCascade

```text
ON DELETE CASCADE من businesses
```

خطر Production؛ قد يحذف كل بيانات التاجر بالخطأ. نستخدم Business lifecycle (`archived/suspended`) ونمنع الحذف الفيزيائي في V1. العلاقات المهمة تستخدم `RESTRICT` أو تمنع الحذف، مع سياسة retention منفصلة لاحقًا.

Event Ledger وAudit خصوصًا لا يجب أن يختفيا بمجرد حذف Business.

### 2. أسماء المراجع الخارجية يجب أن تكون موحدة

نعتمد أسماء واضحة:

```text
external_account_ref
external_connection_ref
provider_event_id
```

هذه نصوص Provider references وليست UUIDs. يجب أن تكون canonical و`NOT NULL` عندما يعتمد عليها dedupe. لا نخلط `provider_account_id` و`provider_connection_id` و`provider_connection_ref` في جداول مختلفة بلا mapping.

### 3. External Identity ترتبط بـChannel Connection

القيد باستخدام Provider/Channel/Account صحيح فقط إذا كان Account ID يطابق Connection دائمًا. الأفضل:

```text
external_identities
├── business_id
├── connection_id NOT NULL
└── external_user_id NOT NULL
```

```text
UNIQUE (connection_id, external_user_id)
```

مع Composite FK أو قيد اتساق يضمن أن `connection_id.business_id = external_identities.business_id`.

### 4. Composite FKs ليست اختيارية

FK منفصل على `business_id` و`customer_id` لا يمنع تركيب Business A مع Customer B. نضيف parent key:

```text
UNIQUE (id, business_id)
```

ثم مراجع مركبة:

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
order_lines → transactions
```

### 5. Conversation References تحتاج Connection

القيد:

```text
UNIQUE (business_id, provider, provider_account_id,
        provider_conversation_id)
```

مقبول فقط إذا لم توجد عدة Connections للحساب نفسه. العقد النهائي الأفضل أن يحتوي:

```text
connection_id NOT NULL
```

ويستخدم:

```text
UNIQUE (connection_id, provider_conversation_id)
```

أما Chatwoot references فتحتاج partial unique indexes عند عدم كونها NULL، مع عدم فرض وجودها قبل نجاح Chatwoot mirror.

### 6. Event Ledger يجب أن يدعم Unresolved

النسخة التي تجعل `connection_id NOT NULL` تتعارض مع عقد عدم فقد الحدث قبل اكتمال mapping. نعتمد:

```text
provider NOT NULL
provider_connection_ref NOT NULL
provider_event_id NOT NULL
connection_id NULLABLE
business_id NULLABLE
```

```text
UNIQUE (provider, provider_connection_ref, provider_event_id)
```

والحالات:

```text
received
unresolved
processing
processed
retryable_failed
dead_letter
rejected
```

مع:

```text
processing_owner
lease_expires_at
attempt_count
last_error_code
```

ACK يصدر بعد `RecordIfAbsent + commit` فقط.

### 7. Outbound Idempotency ليست Global

نرفض:

```text
UNIQUE (provider_idempotency_key)
```

نعتمد:

```text
UNIQUE (provider, connection_id, provider_idempotency_key)
```

ويجب أن تكون قيمة المفتاح deterministic لكل logical send، لا لكل retry. نضيف `connection_id` و`correlation_id` إلى `outbound_messages`.

### 8. Outbox يحتاج Dedupe وLease

النسخة المرفقة تفتقد:

```text
business_id
command_type
dedupe_key
lease_owner
lease_expires_at
```

نضيف:

```text
UNIQUE (business_id, command_type, dedupe_key)
```

و`locked_at` وحده لا يكفي. `Complete/Fail` يجب أن يتحققا من Owner/Lease حتى لا يكتب Worker قديم فوق Worker جديد.

### 9. OutboundMessage وOutbox حالتهم ليست واحدة

يجب أن يبقى ممكنًا:

```text
OutboxEntry = completed
OutboundMessage = unknown
```

لأن الـJob نفذ المحاولة لكن Provider لم يعطِ نتيجة حاسمة. Worker يعيد قراءة حالة Outbound قبل أي side effect.

### 10. الحذف في Event/Outbox ليس Cascade

لا نحذف Event Ledger أو Outbox أو Audit بسبب حذف Connection أو Business. نستخدم soft lifecycle أو `RESTRICT`، لأن هذه سجلات تشغيل وتدقيق.

### 11. Identity Matches تحتاج اتساقًا عبر Business

`LEAST/GREATEST` لمنع A→B وB→A فكرة صحيحة، لكن يجب فرض أن الهويتين من Business نفسه، ومنع ربط هوية بنفسها، مع قرار `match/no_match/needs_review` واضح.

### 12. Catalog attributes ليست JSON بلا عقد

`attributes JSONB` مقبول للتنوع، بشرط:

```text
attribute_schema_id
attribute_schema_version
Application/Domain validation
```

ويجب منع Offer من Catalog Item تابع لـBusiness آخر عبر Composite FKs أو قيود مكافئة. `validation_rules` ليست مكانًا لأسرار أو payload كبير.

### 13. Pricing وAvailability لا يختزلان في amount

`amount` و`total_amount` يمكن أن يكونا NULL في Dynamic/Quote Required، لكن يلزم حفظ:

```text
pricing_mode
currency?
price_evidence_reference?
observed_at?
valid_until?
```

التحقق من أن السعر النهائي صالح قبل Transaction يبقى Domain/Application invariant، وليس CHECK بسيطًا فقط.

### 14. Sales schema يحتاج فصل Current Projection عن History

نقبل وجود current score في `leads` وhistory في `lead_scores`، لكن لا نكرر أسماء متنافسة مثل `score` و`score_value` بلا قرار. نعتمد:

```text
leads.current_score_value?
leads.current_score_band?
lead_scores history
```

`lead_attributions` مستقلة، و`created_by` و`qualified_by` Actor References وليسا Attribution.

### 15. Transactions تحتاج Constraints إضافية

`commercial_transactions` تحتاج CHECK للحالات والأنواع، ومراجع مركبة إلى Business/Customer/Lead. `source_conversation_id` اختياري.

`transaction_confirmations` يجب أن تميز confirmation البشرية عن AI confidence، ويفضل قيد يمنع أكثر من confirmation active لنفس Transaction.

`transaction_reviews` تبقى إذا كان Human Review Workflow داخل V1؛ وإلا نؤجلها مع تفاصيل workflow، لا ننشئ جدولًا بلا Use Case.

`order_lines` تحفظ name/attributes/price/unit/total snapshots، ولا تتغير بعد اعتماد المعاملة إلا عبر Revision/Audit واضحة.

### 16. AI/Audit

`ai_decisions` تحتاج بالإضافة إلى Intent/Action:

```text
conversation_id?
source_message_reference?
policy_version
knowledge_version
schema_version
expires_at?
outcome
```

لا نخزن Chain-of-Thought أو raw prompt حساس.

`audit_events` هو canonical append-only record. `decision_audits` Projection/read model، لا مصدر حقيقة ثانٍ.

### 17. UUID والوقت

لا نضع `DEFAULT now()` إذا كان القرار أن Application Clock مصدر الوقت في الاختبارات. نولد IDs وtimestamps من Application/Clock، ونمنع NULL. يمكن استخدام UUIDv7 مولد في Application عند الحاجة للترتيب.

### 18. Metadata وPayload References

`metadata JSONB` و`payload_reference` يجب أن يخضعا لحجم وretention policy. لا نضع raw SocialAPI payload أو secrets في Audit أو جداول Domain. Object Storage لا يضاف قبل Use Case حقيقي للوسائط/الاحتفاظ.

## Migration sequence المعتمد

نستخدم سلسلة واحدة فقط:

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
000024 transaction_reviews  # فقط إذا كان Workflow V1
000025 order_lines
000026 audit_events
000027 ai_decisions
```

لا ننفذ 000013 مرة ثانية لشرح Outbox، ولا نستخدم أرقامًا متعارضة بين ملفات التخطيط وملفات SQL.

## Contract Tests قبل التشغيل

```text
duplicate external identity
same event twice
unresolved event is retained
cross-tenant customer reference
cross-tenant catalog/offer reference
conversation mapping duplicate
outbound idempotency conflict
outbox + outbound atomicity
lease conflict and lease expiry
business deletion protection
snapshot immutability
migration rerun/order
```

## القرار النهائي

**أعتمد الحزمة كـFull Schema Design، لا كـSQL Production جاهز.** نثبت التصحيحات أعلاه، ثم نكتب migrations فعليًا من `000001` حتى `000027` على مراحل، ونشغل Contract Tests بعد Foundation وقبل أي Provider أو AI implementation.

الخطوة العملية التالية:

```text
Finalize typed IDs + composite FK strategy
→ Write 000001–000012
→ Run migration/constraint tests
→ Write 000013–000027
→ Run full schema review
→ Implement PostgreSQL Adapter
```
