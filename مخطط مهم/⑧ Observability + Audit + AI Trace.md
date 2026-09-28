Mujeeb 24 — ⑧ Observability + Audit + AI Trace

الحالة

CLOSED

هذه الخطوة تحسم كيف نعرف:

- ماذا حدث؟
- أين حدث؟
- لأي Business؟
- مع أي Conversation؟
- ما الذي فعله Gemini؟
- ما الذي فعله Mujeeb؟
- هل تم تنفيذ القرار أم فشل؟
- وما تكلفة تشغيل AI؟

مع الفصل بين:

Observability
Audit
AI Trace

---

1. Observability

Observability مخصصة لمعرفة حالة النظام أثناء التشغيل.

تشمل على مستوى المنظومة:

Metrics
Logs
Errors
Latency
Health
Alerts

وتغطي على الأقل:

AI Requests
AI Errors
Tool Calls
Execution Failures
Provider Failures
Webhook Failures
Queue/Worker Failures

الهدف:

«معرفة أن النظام يعمل، ومعرفة أين فشل عندما لا يعمل.»

---

2. Audit

Audit مختلف عن Observability.

الـAudit يجيب:

«ماذا حدث تجاريًا أو أمنيًا، ومن أي Business، وما النتيجة؟»

يجب أن يكون الـAudit مرتبطًا بالـTenant:

business_id

ولا يجوز تسجيل Audit بدون Business Scope عندما يكون الحدث خاصًا بتاجر.

وهذا متوافق مع قرار المشروع أن أسطح AI/Audit تستخدم "business_id" وأن عزل الـtenant جزء من العقد.

---

3. AI Trace

AI Trace هو السجل التشغيلي لمسار AI الواحد.

مثال:

Customer Message
      ↓
AI Interaction
      ↓
Catalog Evaluation
      ↓
Tool Calls
      ↓
Final Proposal
      ↓
Validation
      ↓
Authorization
      ↓
Execution

نريد أن نستطيع ربط كل هذه الخطوات ببعضها.

---

4. Correlation Identity

لكل AI execution نحتاج معرفًا موحدًا يربط كل الخطوات:

ai_run_id

ويرتبط بسياق Mujeeb:

business_id
conversation_id
message_id

وعند وجود تفاعل Gemini:

gemini_interaction_id

وعند وجود خطوة Tool:

tool_call_id

الهدف هو:

Business
 ↓
Conversation
 ↓
Message
 ↓
AI Run
 ├── Interaction
 ├── Tool Calls
 ├── Proposal
 ├── Validation
 ├── Authorization
 └── Execution

---

5. AI Run

"AI Run" يمثل محاولة AI واحدة لمعالجة رسالة/حدث.

يرتبط على الأقل بـ:

ai_run_id
business_id
conversation_id
message_id
status

ويجب أن نستطيع معرفة حالته التشغيلية.

لا ننشئ حالات Domain جديدة للمحادثة بسبب AI Run.

AI Run هو Trace/Operational record، وليس Conversation State.

---

6. Gemini Interaction Trace

عند استخدام Gemini، نسجل المرجع الذي يسمح بربط العملية بمزود AI:

gemini_interaction_id

وعند وجود:

previous_interaction_id

يمكن ربط التفاعل بالسياق السابق.

لكن:

Gemini history
≠
Mujeeb canonical conversation

مصدر الحقيقة يظل Mujeeb.

---

7. Tool Trace

كل Tool Call يجب أن يكون قابلًا للتتبع.

نسجل على مستوى الـTrace:

tool_call_id
tool_name
status

وعند انتهاء التنفيذ:

tool_result

أو نتيجة الفشل.

الهدف:

Gemini
 ↓
Tool Call
 ↓
Mujeeb
 ↓
Tool Result

والقدرة على معرفة أي خطوة فشلت.

---

8. AI Usage / Cost Telemetry

هذه نقطة مغلقة من الـeconomic baseline.

قبل الإنتاج يجب أن نقيس:

input tokens
cached tokens
output tokens
model requests
tool calls
catalog projection tokens
cost per AI reply

هذه البيانات تشغيلية واقتصادية، وليست Metric يقدمها التاجر كعدد مباشر من طلبات Gemini.

الـbaseline يفرق بين:

Customer-facing:
AI Replies
Catalog Capacity

Internal:
AI cost
model calls
tool calls
tokens
cache

---

9. Latency

نحتاج معرفة زمن كل جزء من المسار.

على الأقل:

AI Run latency
Tool latency
Validation latency
Execution latency

حتى نستطيع معرفة أين يضيع الزمن:

Gemini
vs
Catalog
vs
Mujeeb
vs
Provider

---

10. Errors

يجب التمييز بين أنواع الفشل بدل تسجيل:

AI failed

بشكل عام فقط.

المسار المفاهيمي:

AI Provider Failure
Tool Failure
Validation Failure
Authorization Failure
Execution Failure
Provider/Channel Failure

هذا يسمح بتحديد مكان المشكلة.

---

11. Audit لا يخزن كل شيء لمجرد أننا نستطيع

الـAudit ليس نسخة كاملة من كل Prompt وكل البيانات.

نسجل الأحداث التي نحتاجها لإثبات ما حصل.

مثل:

AI proposal produced
AI proposal validated
Policy evaluated
Approval requested
Human approved
Action executed
Action failed
Human takeover

أما الـObservability فيركز على التشغيل والأداء.

---

12. AI Trace لا يصبح Business Truth

مثلاً:

AI قال:
amount = 12000

هذا لا يجعل السعر حقيقة.

الحقيقة التجارية تأتي من:

Catalog / Business Data

والـAI Trace يسجل فقط:

what AI proposed

وهذا يحافظ على:

AI Reasoning
≠
Business Truth

---

13. Validation Trace

يجب أن نعرف نتيجة كل حاجز:

Structural Validation
Reference Validation
Tenant Validation
Policy Evaluation
Authorization

مثال:

AI Proposal
   ↓
Structural = passed
Reference   = passed
Tenant      = passed
Policy      = passed
Authorization = approved

أو:

Structural = passed
Reference   = failed

وعندها لا يصل التنفيذ.

---

14. Execution Trace

بعد Authorization:

Effective Decision
      ↓
Execution

نحتاج معرفة:

authorized
started
completed
failed

لكن لا نسجل نجاحًا قبل أن يحدث التنفيذ فعليًا.

---

15. Human Approval Trace

إذا كانت العملية تحتاج موافقة:

AI Proposal
   ↓
Approval Required
   ↓
Pending Approval

نسجل:

approval requested
approved / rejected

ولا نعتبر الموافقة تنفيذًا.

التنفيذ يأتي بعدها.

---

16. Human Takeover Trace

عند انتقال Conversation إلى الإنسان:

AI
 ↓
Human Handoff
 ↓
Human Handling

يجب أن يظهر ذلك في الـTrace/Audit.

ويتوافق ذلك مع الفصل المغلق بين:

State
Ownership
Assignment
AI Mode

---

17. Tenant Isolation في Trace

كل Trace خاص بتاجر يجب أن يكون Scoped.

أي أن:

Business A

لا يستطيع الوصول إلى Trace الخاص بـ:

Business B

والـAudit نفسه يخضع لنفس قاعدة الـtenant isolation الموجودة في بقية النظام.

---

18. لا نستخدم AI Trace كذاكرة Gemini

لا نقول:

AI Trace
   ↓
Conversation Memory

بل:

Mujeeb Conversation State
   ↓
Canonical

AI Trace
   ↓
Historical / Operational Evidence

---

19. لا نربط Observability بالـDomain

Metrics مثل:

latency
error_count
token_usage
request_count

ليست Domain Entities.

هي Observability/Infrastructure concerns.

ولا نلوث Domain بها.

---

20. لا نضع تفاصيل Provider داخل Domain

يمكن تسجيل:

provider = gemini
model = ...

في AI Trace/Infrastructure.

لكن Domain لا يصبح:

GeminiConversation
GeminiMessage
GeminiToken

---

21. المسار الكامل

Customer Message
      ↓
AI Run
      ↓
Gemini Interaction
      ↓
Catalog Evaluation / Tool Calls
      ↓
AI Proposal
      ↓
Validation
      ↓
Policy
      ↓
Authorization
      ↓
Execution
      ↓
Execution Result

وفي نفس الوقت:

                     AI Trace
                        │
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
   Gemini Steps     Validation       Execution
        │
    Tool Calls
        │
      Usage
        │
     Latency
        │
      Errors

---

22. ما الذي نحتاجه لمعرفة "لماذا حصل هذا"؟

عند فشل أو سلوك غير متوقع، يجب أن نتمكن من ربط:

business_id
conversation_id
message_id
ai_run_id
gemini_interaction_id
tool_call_id

ثم نرى التسلسل:

Input
↓
Context
↓
Gemini
↓
Tool
↓
Proposal
↓
Validation
↓
Policy
↓
Authorization
↓
Execution

هذا هو الهدف الحقيقي من AI Trace.

---

23. حدود التخزين

لا نخزن أسرارًا أو بيانات حساسة لمجرد أنها كانت متاحة أثناء التشغيل.

خصوصًا:

API keys
access tokens
provider secrets
credentials

ولا نضعها في Audit أو AI Trace.

وكذلك لا نعتبر Audit مكانًا لنسخ قاعدة البيانات.

---

24. ما تم إغلاقه

✅ Observability منفصلة عن Audit
✅ AI Trace منفصل عن Domain
✅ AI Run له Correlation
✅ business_id جزء من Tenant Scope
✅ conversation_id قابل للتتبع
✅ message_id قابل للتتبع
✅ Gemini interaction قابل للتتبع
✅ Tool Call قابل للتتبع
✅ Validation قابل للتتبع
✅ Authorization قابل للتتبع
✅ Execution قابل للتتبع
✅ Usage metrics قابلة للقياس
✅ Token metrics قابلة للقياس
✅ Tool calls قابلة للقياس
✅ Cost per AI reply قابلة للقياس
✅ Latency قابلة للقياس
✅ Errors قابلة للتشخيص
✅ Approval قابل للتتبع
✅ Human takeover قابل للتتبع
✅ لا أسرار داخل Trace/Audit
✅ لا AI Trace كـBusiness Truth
✅ لا AI Trace كـConversation Memory
✅ لا Provider-specific entities داخل Domain

25. القرار النهائي

OBSERVABILITY
=
هل النظام يعمل؟ وكيف يعمل؟

AUDIT
=
ماذا حدث تجاريًا/أمنيًا؟

AI TRACE
=
ماذا حدث داخل AI Run؟

والترابط:

Business
   ↓
Conversation
   ↓
Message
   ↓
AI Run
   ├── Gemini Interaction
   ├── Tool Calls
   ├── Usage
   ├── Proposal
   ├── Validation
   ├── Authorization
   └── Execution

⑧ Observability + Audit + AI Trace = CLOSED