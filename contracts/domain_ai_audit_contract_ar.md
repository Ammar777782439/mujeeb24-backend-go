# Domain Contract — AI وAudit

## قرار الملكية

AI في مجيب 24 **مستشار منظم وصانع قرار مقترح**، وليس مالكًا للإرسال أو السعر أو المخزون أو تأكيد الطلب.

```text
Conversation + Business Context + Catalog Evidence
                    ↓
             Intent / Entities
                    ↓
             Knowledge Validation
                    ↓
             Policy Evaluation
                    ↓
             Structured AI Decision
                    ↓
       Authorization + Deterministic Validation
                    ↓
        Application Action Executor
```

LLM SDK وPrompts وJSON provider DTOs تبقى خارج Domain، في Adapter/AI Provider. Domain يتعامل مع Decision contract فقط.

## 1. ai/

### 1.1 Intent

Intent هو تصنيف لغرض العميل، وليس أمر تنفيذ مباشر.

```text
Intent
├── code
├── category
├── confidence
├── evidence_references
└── schema_version
```

الفئات الأولية:

```text
information_request
product_question
price_question
availability_question
purchase_intent
booking_request
appointment_request
service_request
quote_request
order_status
complaint
handoff_request
unknown
```

Intent قد يكون `unknown` أو منخفض الثقة. لا نُجبر كل رسالة على Intent معروف.

### 1.2 Entity

```text
ExtractedEntity
├── key
├── value
├── data_type
├── confidence
├── source_span_reference
├── validation_status
└── schema_version
```

أمثلة:

```text
catalog_item_id
variant_id
quantity
origin
destination
departure_date
doctor_specialty
preferred_time
location
phone
```

Entity التي لم تتحقق من Catalog/Transaction Schema تبقى `unverified` ولا تستخدم لتأكيد عملية.

### 1.3 KnowledgeContext

AI لا يقرأ قاعدة البيانات عشوائيًا. Application يبني Context يحتوي على أدلة محددة:

```text
KnowledgeContext
├── business_reference
├── conversation_reference
├── customer_context_reference
├── catalog_evidence
├── offer_evidence
├── availability_evidence
├── policy_evidence
├── recent_message_references
├── generated_at
└── expires_at
```

كل evidence له source ووقت تحقق وschema version. Knowledge قد تكون `fresh` أو `stale` أو `missing`.

### 1.4 Confidence

Confidence ليست إذن تنفيذ.

```text
ConfidenceBand
├── high
├── medium
├── low
└── unknown
```

يجب أن نحتفظ بالدرجة الرقمية إن قدمها Model، لكن Domain يعتمد على band وevidence وPolicy، لا على رقم وحده.

### 1.5 AIAction

الأفعال المتاحة محدودة ومعلنة:

```text
AIAction
├── no_op
├── ask_clarifying_question
├── answer_with_verified_knowledge
├── create_lead
├── update_lead
├── create_order_draft
├── create_booking_draft
├── create_appointment_request
├── create_service_request
├── create_quote_draft
├── recommend_human_handoff
├── assign_human
└── send_message
```

`send_message` لا يرسل بنفسه؛ هو اقتراح لإنشاء OutboundMessage يمر عبر Policy وOutbox وProvider Adapter.

لا نسمح في Domain بـ`execute_arbitrary_tool` أو نص LLM حر يتحول إلى command.

### 1.6 AIMode

```text
disabled
assist
approval
restricted_auto
```

المعنى:

| الوضع | السلوك |
|---|---|
| `disabled` | لا ينشئ AI قرارًا تشغيليًا |
| `assist` | يقترح للموظف ولا يرسل تلقائيًا |
| `approval` | ينشئ قرارًا يحتاج موافقة قبل side effect |
| `restricted_auto` | يسمح بأفعال محددة مسبقًا وبحدود Policy |

`restricted_auto` لا يعني أن AI يملك صلاحية عامة. كل Action يحتاج capability وPolicy وEvidence.

### 1.7 PolicyEvaluation

```text
PolicyEvaluation
├── result: allowed | requires_approval | denied
├── rules_evaluated
├── missing_evidence
├── denial_reasons
├── evaluated_at
└── policy_version
```

أمثلة منع:

```text
- سعر dynamic بلا تحقق
- Availability unknown
- طلب بيانات حساسة غير لازمة
- إرسال WhatsApp خارج قواعد القناة دون template مناسب
- تأكيد Order دون customer confirmation
- تنفيذ Refund أو Discount غير مصرح
```

### 1.8 AIDecision

```text
AIDecision
├── id
├── business_id
├── conversation_reference_id
├── source_message_reference_id
├── intent
├── entities
├── proposed_action
├── confidence_band
├── evidence_references
├── policy_evaluation
├── status
├── model_reference
├── prompt_policy_version
├── expires_at
├── created_at
└── decided_at
```

الحالات:

```text
proposed
validated
requires_approval
authorized
rejected
expired
executed
partially_executed
execution_failed
```

AI Decision لا يصبح `authorized` بمجرد أن Model أعاده. يحتاج Application validation وBusiness Policy.

### 1.9 AI Invariants

لا يجوز لـAI:

1. اختراع سعر أو مخزون أو موعد.
2. تحويل `unknown` إلى `available` أو `confirmed`.
3. تنفيذ HTTP أو SQL أو Provider API مباشرة.
4. تغيير Customer أو Lead أو Transaction بلا Application command.
5. تأكيد Booking أو Order دون evidence وcustomer confirmation حيث يلزم.
6. استخدام Decision منتهية الصلاحية بعد تغير Catalog/Offer/Availability.
7. إرسال رسالة خارج القناة أو capability المسموحة.

إذا نقصت البيانات، الإجراء الصحيح غالبًا هو `ask_clarifying_question` أو `recommend_human_handoff`، وليس إجابة مصطنعة.

### 1.10 AI Domain Events

```text
AIContextBuilt
IntentResolved
EntitiesExtracted
AIDecisionProposed
AIDecisionValidated
AIDecisionApprovalRequired
AIDecisionAuthorized
AIDecisionRejected
AIDecisionExpired
AIDecisionExecuted
AIDecisionExecutionFailed
```

الأحداث تحفظ model reference وpolicy version وevidence references، ولا تحفظ API keys أو prompt secrets أو raw private data بلا سياسة retention.

## 2. audit/

### 2.1 AuditEvent

AuditEvent سجل غير قابل للتعديل يمثل من فعل ماذا ولماذا وبأي سياق.

```text
AuditEvent
├── id
├── business_id
├── actor_type
├── actor_reference
├── action
├── subject_type
├── subject_id
├── decision_reference
├── correlation_id
├── causation_id
├── result
├── reason_code
├── before_reference
├── after_reference
├── occurred_at
├── schema_version
└── redaction_version
```

`actor_type`:

```text
customer
human_agent
ai
automation
system
provider
```

`result`:

```text
accepted | rejected | failed | completed | skipped
```

### 2.2 Audit Invariants

AuditEvent append-only. لا نعدّل سجلًا تاريخيًا؛ التصحيح يسجل Event جديدًا يشير إلى السابق.

لا يحتوي AuditEvent على:

```text
access tokens
webhook secrets
passwords
raw provider payloads كاملة
نصوص حساسة غير لازمة
```

يمكن أن يحتوي على references أو hashes أو redacted summaries وفق retention policy.

كل side effect تجاري أو خارجي يجب أن يملك `correlation_id`. هذا يربط:

```text
InboundEvent
→ AIDecision
→ OutboundMessage
→ Provider Delivery
→ Chatwoot Mirror
```

### 2.3 Audit Actions

```text
business.created
channel.connected
channel.disconnected
inbound.accepted
inbound.duplicated
identity.linked
conversation.mapped
ai.decision.created
ai.decision.approved
ai.decision.rejected
message.send_requested
message.send_accepted
message.send_failed
lead.created
transaction.draft_created
transaction.confirmed
transaction.cancelled
human.handoff_requested
```

لا نستخدم نصًا حرًا فقط كـaction؛ نستخدم action code مع optional safe metadata.

## 3. العلاقة بين AI وSales

AI يقترح، وSales Domain يقرر صلاحية الانتقال:

```text
AIDecision: create_order_draft
        ↓
Transaction Service validates:
- CatalogItem/Offer
- selected attributes
- pricing state
- availability state
- customer confirmation
- business policy
        ↓
Order Draft أو رفض/طلب بيانات
```

إذا قالت الرسالة «أريد آيفون 13 أسود 128»، يمكن AI استخراج item وvariant والكمية. لكن إنشاء Draft يحتاج أن تكون النتائج قابلة للمطابقة والتحقق.

إذا قالت الرسالة «أريد السفر صنعاء القاهرة الخميس»، يمكن AI إنشاء Booking Draft ناقص أو طلب date/year/class، لكنه لا يؤكد سعرًا أو مقعدًا مجهولًا.

## 4. معايير إغلاق ai وaudit

1. كل AI output يمر عبر Structured Decision، لا نص حر تنفيذي.
2. كل Decision يذكر evidence وpolicy version وstatus.
3. كل Action محدود بـEnum معروف.
4. AI لا يملك Provider/DB/HTTP side effects مباشرة.
5. Unknown وStale وRequiresCheck حالات حقيقية.
6. Audit append-only وبدون أسرار.
7. correlation_id يربط الرسالة بالقرار والنتيجة الخارجية.
8. Decision المنتهية لا تنفذ بعد تغير الدليل.

بعد هذا العقد يمكن الانتقال إلى تدقيق الترابط الكامل، ثم كتابة Domain files كـGo types وValue Objects وconstructors واختبارات invariants فقط، دون Repositories أو API.
