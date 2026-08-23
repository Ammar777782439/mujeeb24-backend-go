# مراجعة المدير — Persistence / Application Boundary Contract

## الحكم التنفيذي

التحليل المرفق **صحيح وقابل للتحويل إلى أساس Production**، وهو يغلق الفجوة بين Domain/Ports وبين SQL/Reliability. أعتمده بعد تصحيحين حاسمين:

1. لا يجوز أن يتوقف حفظ الحدث الخارجي على نجاح Tenant Resolution؛ يجب أن نملك مسار `unresolved/quarantine` حتى لا نفقد Webhook أثناء عطل mapping أو قاعدة البيانات الجزئية.
2. `DecisionAudit` لا يكون مصدر حقيقة ثانيًا بجانب `AuditEvent`؛ هو Projection/typed view من سجل التدقيق canonical.

## الملكية النهائية

```text
PostgreSQL
├── Mujeeb Domain State
├── Inbound Event Ledger
├── Outbound Message State
├── Outbox Obligations
├── Audit Events
└── Idempotency Constraints/State

Asynq/Redis = Job Execution فقط
```

لا ننسخ قاعدة SocialAPI أو Chatwoot، ولا نعتمد Redis للحقيقة الدائمة.

## Atomicity

Application Service يحدد Use Case، وTransactionManager يضمن أن التغييرات الداخلية التي يجب أن تظهر كوحدة واحدة تُحفظ داخل transaction واحدة:

```text
BEGIN
  Save Aggregate
  Append Domain/Audit Event عند الحاجة
  Create Outbox Obligation
COMMIT
```

لا نستدعي SocialAPI أو Chatwoot أو أي Network Provider داخل DB transaction. الإرسال الخارجي يحدث بعد Commit عبر Outbox Worker.

لا نضع transaction داخل كل Repository method؛ Repository يستخدم transaction context الذي يمرره Unit of Work.

## Inbound Event Ingestion

التدفق المصحح:

```text
Provider Webhook
  ↓
HTTP size/content validation
  ↓
Signature verification
  ↓
Extract provider/account reference
  ↓
RecordIfAbsent في Event Ledger
  ↓
ACK 2xx
  ↓
Async processing
```

إذا كان Provider/Connection معروفًا، يُحفظ event مع `business_id`. وإذا تعذر resolve بسبب عطل مؤقت، لا نرفض الحدث ولا نبدأ processing؛ نحفظه في حالة `unresolved` أو Intake Quarantine مع Provider Account Reference، ثم تعيد Reconciliation ربطه لاحقًا.

القاعدة:

> ACK يعني أن الحدث حُفظ بشكل دائم، وليس أن Business Workflow اكتمل.

لا نعطي 2xx قبل نجاح durable insert. ولا نعالج Webhook كاملًا داخل HTTP request.

## Event Ledger وIdempotency

نستخدم سجلًا دائمًا واحدًا للأحداث الواردة، مع عملية ذرية:

```text
RecordIfAbsent(
  provider,
  provider_connection_ref,
  provider_event_id
)
```

المفتاح الفريد المقترح:

```text
provider + provider_connection_ref + provider_event_id
```

لكن إذا كان Provider Event ID غير موثوق أو غائبًا، لا نستخدم hash للنص كهوية. نحتاج fallback contract صريح يضم message/reference/time bucket، ويصنف الحدث `dedupe_uncertain` عند الغموض بدل حذف رسالة شرعية.

حالات Event Ledger:

```text
received
unresolved
processing
processed
retryable_failed
dead_letter
rejected
```

عملية Claim تحتاج lease/owner token ووقت انتهاء؛ `processing` بلا lease قد يعلق الحدث بعد crash.

## Outbox وOutbound Message

```text
OutboundMessage = business communication intent/state
OutboxEntry      = technical obligation/job
```

يُنشأ الاثنان داخل نفس DB transaction عندما يقرر Use Case إرسالًا خارجيًا:

```text
BEGIN
  Create OutboundMessage
  Create OutboxEntry with dedupe_key
COMMIT
```

بعد Commit:

```text
Outbox
  → claim with lease
  → call Provider
  → classify result
  → persist delivery state
  → complete/fail outbox job
```

يجب ألا تعتبر `Outbox COMPLETED` أن Provider أرسل الرسالة بنجاح؛ هذه فقط تعني أن Job نفذ محاولته. حالة Provider تُحفظ في Outbound Delivery.

## Retry وUnknown وReconciliation

لا نعيد كل خطأ بلا نهاية:

| النتيجة | الإجراء |
|---|---|
| Transient timeout/5xx | Retry بحدود وBackoff |
| Rate limit | Backoff وفق Retry-After إن وجد |
| Auth failure | إيقاف الإرسال وConnection reconnect |
| Invalid request | Permanent failure وHuman/Developer review |
| Unknown بعد timeout | لا Retry أعمى؛ Reconciliation |
| Retry exhausted | Dead Letter قابل لإعادة التشغيل |

عند `unknown` نبحث في Provider Status باستخدام idempotency/reference. إذا ثبت أنه أُرسل نحدّث الحالة، وإذا ثبت أنه لم يُرسل نعيد المحاولة بأمان، وإذا بقي مجهولًا نرفع Human Review ولا نرسل نسختين.

## Repository Boundaries

Repository يعرف Aggregate/Read Model وحدوده، ولا ينفذ Workflow:

```text
Application Service
  → Load Aggregate
  → Domain Decision
  → Save Aggregate
  → Append Outbox/Audit حسب Use Case
```

نرفض Generic Repository من نوع `Save(any)/Find(any)/Update(any)`. نستخدم عقودًا متخصصة، مع Queries scoped بـBusiness.

`AuditWriter.Append` append-only ولا يملك Update/Delete. `EventLedgerStore` يملك atomic dedupe وlease. `OutboxStore` يملك claim/complete/fail/requeue semantics.

## Tenant Scoping

لا نثق بـ`business_id` من HTTP body. النطاق يأتي من Authenticated User → Membership → Business Scope → Application Command.

Repository يفرض Business Scope كدفاع إضافي:

```sql
SELECT ...
FROM leads
WHERE business_id = $1 AND id = $2;
```

كل cross-tenant reference يُرفض. وأي event لم يُحسم Businessه لا يدخل Business Domain قبل أن يمر بمرحلة resolution آمنة.

## Raw Payload

Domain يحتفظ بـ`raw_payload_reference` و`payload_hash` وربما content type/size، وليس raw Provider DTO. التخزين الخام يكون في encrypted object storage أو retention-controlled store عند الحاجة. الأسرار لا تدخل raw payload logs أو Audit.

## Audit وDecisionAudit

```text
Event Ledger = هل استقبلنا/عالجنا الحدث؟
AuditEvent   = من فعل ماذا ولماذا؟
DecisionAudit = Projection typed لقرارات AI/Policy من AuditEvent
```

لا ننشئ مصدرَي حقيقة. العمليات الحساسة مثل Create Transaction وCancel وRefund وPrice Override وHuman Approval تحتاج Audit واضحًا مع `business_id` و`correlation_id` و`causation_id` وsafe references.

## Migration Boundary

نستخدم versioned SQL migrations مع pgx وsqlc/explicit repositories، ولا نستخدم AutoMigrate.

الترتيب المنطقي المبدئي:

```text
businesses
business_policies
channel_connections
customers
external_identities
conversations/references
inbound_event_ledger
outbound_messages
outbox_entries
catalog/sales
ai/audit
```

لكن لا نثبت أرقام migrations النهائية قبل تثبيت typed IDs وforeign keys وtenant constraints وEvent/Outbox indexes.

## ما لا نضيف الآن

لا نضيف Cache Port أو Event Bus أو Notification Port أو Availability Provider قبل وجود Use Case واختبار. لا نفصل Reconciler كخدمة مستقلة قبل أن يثبت حجم التشغيل الحاجة إليه؛ يمكن تشغيله كـWorker Job في V1.

## معايير الإغلاق

1. Aggregate وEvent Ledger وOutbox يمكن حفظها ذريًا عند الحاجة.
2. ACK لا يصدر قبل durable event insert.
3. Event Ledger يملك dedupe atomic وlease.
4. Outbox لا يعتمد على Redis للحقيقة.
5. Outbound Delivery منفصل عن Outbox Job.
6. Unknown لا يعاد إرساله أعمى.
7. Unresolved events لا تضيع بسبب فشل tenant mapping.
8. كل Repository tenant-aware.
9. Audit canonical append-only، وDecisionAudit Projection.
10. لا Network calls داخل DB transactions.

## القرار

**أعتمد عقد Persistence/Application Boundary بعد هذه التصحيحات.** أصبح لدينا أساس كافٍ للانتقال إلى كتابة Go interfaces في `application/ports`، بشرط أن تترجم هذه الدلالات حرفيًا، ثم ننتقل إلى Bootstrap/Config وSQL migrations.
