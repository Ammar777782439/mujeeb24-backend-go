# مراجعة المدير — Domain Contract 07: AI وAudit

## الحكم التنفيذي

التحليل المرفق **صحيح ومهم، ويغلق Domain بالكامل تقريبًا**. وهو متوافق مع مبدأ مجيب 24 الأساسي:

```text
LLM ≠ Executor
AI Decision ≠ Authorization
Audit ≠ Event Ledger
```

سنعتمد الهيكل العام، مع تصحيحات تمنع خلط قرار AI بالتنفيذ أو تكرار سجل التدقيق.

## ما نعتمده

| القرار | الحكم |
|---|---|
| Intent يفهم رغبة العميل ولا ينفذ | معتمد |
| Entities مرتبطة بـCatalog/AttributeSchema | معتمد |
| Entity Source وEvidence | معتمد |
| Confidence ليست Authorization | معتمد |
| KnowledgeContext ليس Knowledge Base كاملة | معتمد |
| Knowledge Facts تحمل Source/Authority/Validity | معتمد |
| AIDecision Structured | معتمد |
| Actions محدودة Typed | معتمد |
| `CREATE_TRANSACTION_DRAFT` بدل `CREATE_ORDER` العام | معتمد |
| Missing Information جزء أساسي من القرار | معتمد |
| Human requirement قد تأتي من Policy | معتمد |
| PolicyDecision منفصلة عن AI Decision | معتمد |
| Prompt/Model settings خارج Domain | معتمد |
| عدم تخزين Chain-of-Thought | معتمد |
| Audit append-only ومحدد بالغرض | معتمد |
| Audit منفصل عن Event Ledger | معتمد |
| كل سياق تجاري يحمل business_id | معتمد |

## التصحيحات الإلزامية

### 1. Intent يحتاج Base Intent وContext اختياري

القائمة المقترحة جيدة، لكن لا نريد Enum مسطحًا يصبح ضخمًا مع القطاعات. العقد الأفضل:

```text
Intent
├── base
├── domain_context?
├── confidence_band
├── evidence_references
└── schema_version
```

أمثلة:

```text
base=booking, domain_context=travel
base=appointment, domain_context=clinic
base=service_request, domain_context=maintenance
```

`domain_context` يصف القطاع أو الـworkflow الإضافي، ولا ينشئ AI Agent مستقلًا لكل قطاع.

### 2. Actions يجب أن تكون Provider-neutral وBusiness-safe

نعتمد:

```text
answer
ask_clarification
create_lead
update_lead
create_transaction_draft
request_availability_check
request_price_check
request_human
no_action
```

لا نضع `CALL_FACEBOOK_API` أو `CALL_WHATSAPP_API` أو `DELETE_CUSTOMER` أو `REFUND` داخل AI Action.

لكن إذا طلب العميل إلغاء أو استرداد، نحتاج تمثيل نية الطلب دون إعطاء صلاحية التنفيذ:

```text
intent = cancellation
requested_action = request_human
reason_code = high_risk_transaction_change
```

أو Action Domain آمن مثل `request_transaction_change_review`، ثم Application يقرر.

### 3. AIDecision لا يملك Status تنفيذيًا كاملًا

التحليل صحح هذه النقطة. نميز بين:

```text
AIDecision lifecycle:
proposed → validated → policy_evaluated → expired/rejected
```

وبين:

```text
Application Action Execution:
requested → authorized → queued → executed/failed/unknown
```

لا نضع `executed` في AIDecision إلا كـoutcome/reference مختصر؛ مصدر حالة التنفيذ هو Action/Outbox/Delivery أو Transaction المناسبة.

### 4. KnowledgeContext يجب أن يحمل Evidence قابلة للانتهاء

كل KnowledgeFact ينبغي أن يملك:

```text
source_type
source_reference
authority
observed_at
valid_until?
content_reference
```

`AI_GENERATED` لا يكون Authoritative للسعر أو التوفر. `CUSTOMER_MESSAGE` دليل على ما طلبه العميل، وليس دليلًا على السعر أو سياسة التاجر.

لا نرسل Full Business Database إلى LLM. Context يضم relevant items/offers/policies والمراجع اللازمة فقط.

### 5. Entity Source يحتاج حالة التحقق

المصدر وحده لا يكفي. نضيف:

```text
source = customer_message | catalog | knowledge | system | ai_generated
validation = unverified | validated | conflicting | rejected
```

مثلاً `quantity=2` من رسالة العميل قابل للاستخدام بعد parsing، لكن `price=180000` من AI_GENERATED يظل مرفوضًا حتى يطابق Offer موثوقًا.

### 6. PolicyDecision ليست Authorization كاملة

التسلسل الصحيح:

```text
AIDecision
  ↓
PolicyEvaluation
  ↓
Application Authorization
  ↓
Command / Outbox Action
```

`PolicyDecision=allowed` تعني أن القواعد تسمح مبدئيًا، لكنها لا تتجاوز Transaction invariants أو Connection capability أو authorization البشرية المطلوبة.

القيم:

```text
allowed
requires_approval
denied
```

### 7. AI Mode مصدره Conversation/Business وليس AI Domain

نستخدم `AI Mode` في Conversation أو Business Policy:

```text
ai_allowed
ai_draft_only
ai_disabled
```

Domain AI يستقبل mode في Context ولا يملك Conversation. و`ai_allowed` لا يسمح بأفعال غير موجودة في Policy.

### 8. AuditEvent وDecisionAudit ليسا مصدرين مستقلين

التحليل يقترح ملفين، لكن لا نريد سجلين متنافسين. القرار:

```text
AuditEvent = canonical append-only record
DecisionAudit = typed view/projection أو helper contract
```

إذا احتجنا `DecisionAudit` للاستعلام، فهو Projection من AuditEvent، وليس سجلًا ثانيًا يجب الحفاظ على اتساقه يدويًا.

### 9. Audit لا يساوي Event Ledger

```text
Event Ledger:
هل استقبلنا/عالجنا الحدث؟

Audit:
من نفذ أو قرر ماذا ولماذا؟
```

ليس كل Webhook صغير Audit Event. الأحداث ذات الأثر التجاري أو الأمني أو قرار AI المهم تسجل Audit، بينما كل Webhook يدخل Event Ledger وفق reliability policy.

### 10. Actor Type يحتاج Human Agent وCustomer

نعتمد:

```text
customer
human_agent
ai
automation
system
provider
```

ولا نستخدم `USER` الغامضة إذا كان الفرق بين التاجر والعميل والموظف مهمًا في التدقيق.

## العقد النهائي لـdomain/ai

```text
internal/domain/ai/
├── intent.go
├── entities.go
├── knowledge_context.go
├── ai_decision.go
└── policy_decision.go
```

المفاهيم الأساسية:

```text
Intent
ExtractedEntity
KnowledgeContext
KnowledgeFact
AIDecision
PolicyEvaluation
AIAction
```

ولا يدخل هنا:

```text
OpenAI/Gemini/Anthropic SDK
Prompt templates
HTTP calls
Tool execution
Database access
SocialAPI calls
Chatwoot calls
```

## العقد النهائي لـdomain/audit

```text
internal/domain/audit/
├── audit_event.go
└── decision_audit.go  # projection/helper contract, not a second source of truth
```

`AuditEvent` append-only، يحمل `business_id` و`actor` و`action` و`resource` و`correlation_id` و`decision_reference` وsafe metadata، ولا يحمل secrets أو Chain-of-Thought أو raw payload كامل.

## Domain Events مقابل Audit

Domain Events تعبر عن تغير ذي معنى داخل Aggregate ويمكن أن تذهب إلى Outbox. Audit Events تثبت قرارًا أو side effect لأغراض المراجعة. Event Ledger يثبت استقبال ومعالجة الأحداث الخارجية. قد ينتج حدث واحد أكثر من أثر، لكن لا نخلط الجداول أو المسؤوليات.

## معايير إغلاق AI وAudit

1. لا يوجد LLM → Executor.
2. كل AI Action محدود ومحدد.
3. كل Decision تحمل Evidence وPolicy Version وModel Reference عند الحاجة.
4. Unknown/Missing/Stale حالات واضحة.
5. Application هي التي تنفذ authorization وside effects.
6. Decision المنتهية لا تنفذ.
7. Audit append-only ومقيد بـBusiness.
8. DecisionAudit ليس سجلًا ثانيًا.
9. لا نخزن Chain-of-Thought أو أسرارًا.
10. correlation/causation تربط Inbound وAI وAction وOutbound وTransaction.

## القرار

**أعتمد تحليل الذكاء الاصطناعي بعد هذه التصحيحات.** أصبح Domain Contract لمجيب 24 مغلقًا تصميميًا:

```text
shared ✅
business ✅
channel ✅
identity ✅
communication ✅
catalog ✅
sales ✅
ai ✅
audit ✅
```

الآن فقط ننتقل إلى `application/ports`، ونكتب العقود الخارجية دون implementations: ChannelProvider وCommunicationWorkspace وRepositories وEventStore وOutbox وAIService وSecretStore وClock وTransaction.
