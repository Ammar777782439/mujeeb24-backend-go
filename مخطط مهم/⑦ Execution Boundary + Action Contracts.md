Mujeeb 24 — ⑦ Execution Boundary + Action Contracts

الحالة

CLOSED

هذه الخطوة تحسم ما يحدث بعد:

AI Proposal
   ↓
Validation
   ↓
Policy
   ↓
Authorization
   ↓
Effective Decision

وتحدد الحدود النهائية بين:

AI
Application
Domain
Provider

---

1. المبدأ الأساسي

Gemini لا ينفذ.

Gemini ينتج:

AI Proposal

ثم Mujeeb يتحقق ويطبق Policy ويصدر:

Effective Decision

ثم طبقة التنفيذ في Mujeeb تنفذ القرار المصرح به.

المسار:

Gemini
   ↓
AI Proposal
   ↓
Mujeeb Validation
   ↓
PolicyEvaluator
   ↓
Authorization
   ↓
Effective Decision
   ↓
Application Execution
   ↓
Provider / Domain

وهذا يطابق القرار المغلق سابقًا:

Event
 ↓
Context
 ↓
Intent
 ↓
Knowledge
 ↓
Policy
 ↓
AI Decision
 ↓
Authorization
 ↓
Action

مع القاعدة:

LLM ≠ Executor

---

2. Effective Decision

"Effective Decision" هي النتيجة التي يسمح بها Mujeeb بعد:

Structural Validation
Reference Validation
Tenant Validation
Policy Evaluation
Authorization

وهي الوحيدة التي يمكن أن تدخل التنفيذ.

Gemini لا يستطيع تجاوزها.

---

3. AI Actions المعتمدة

الـAI Contract المغلق يحتوي على:

answer
clarification
human_request
lead_draft
order_draft

هذه اقتراحات AI وليست عمليات تنفيذ تلقائية.

---

4. Action: answer

المعنى:

«Gemini يقترح إرسال إجابة للعميل.»

المسار:

AI
 ↓
answer
 ↓
Validation
 ↓
Policy
 ↓
Authorization

بعدها نتيجتان:

Allowed without approval
        ↓
Send Message

Approval required
        ↓
Pending Approval

Mujeeb هو الذي يحدد أي مسار ينطبق بناءً على Policy.

---

5. Action: clarification

المعنى:

«Gemini لم يملك معلومات كافية لإعطاء الإجابة النهائية، ويقترح سؤالًا توضيحيًا.»

المسار:

AI
 ↓
clarification
 ↓
Validation
 ↓
Policy / Authorization
 ↓
Send clarification

ولا تعتبر الرسالة النهائية حلًا تجاريًا إذا كان الـAI قد صنف الحالة كـ"clarification".

---

6. Action: human_request

المعنى:

«العميل أو سياق المحادثة يحتاج انتقالًا إلى التعامل البشري.»

المسار:

AI
 ↓
human_request
 ↓
Validation
 ↓
Policy
 ↓
Human Handoff

بعد الـhandoff لا يتجاوز AI حالة المحادثة أو ملكية التعامل البشري.

وهذا متوافق مع الحالات المغلقة للمحادثة:

WAITING_HUMAN
HUMAN_HANDLING

---

7. Action: lead_draft

المعنى:

«Gemini أعد معلومات/اقتراحًا يمكن أن ينتقل إلى مسار Lead.»

هذا لا يعني:

Lead Created

ولا يعني:

Lead Won

ولا يعني أن Gemini أنشأ Lead مباشرة.

الحد:

Gemini
 ↓
lead_draft
 ↓
Mujeeb Validation / Authorization
 ↓
Sales Application Flow

وعقد الـLead نفسه مستقل وله حالاته المغلقة:

NEW
INTERESTED
QUALIFIED
WON
LOST

---

8. Action: order_draft

المعنى:

«Gemini أعد مسودة Order بناءً على المعلومات المتاحة.»

لا يعني:

Order Created

ولا:

Order Confirmed

ولا:

Payment Confirmed

المسار المقفول:

AI
 ↓
Order Draft
 ↓
Missing Information / Clarification عند الحاجة
 ↓
Confirmation
 ↓
Order

وهذا هو العقد الموجود للمشروع:

Prepare Draft
↓
Ask Missing Info
↓
Confirmation
↓
Order

---

9. Approval Boundary

إذا كانت Business Policy تتطلب موافقة بشرية:

Effective Decision
      ↓
Pending Approval
      ↓
Human
   ├── Send
   ├── Edit
   └── Reject

ولا يكون Gemini هو صاحب قرار الموافقة.

والـPolicy الموجودة في قاعدة البيانات هي المرجع.

مثال:

send_message_requires_approval = true

يعني أن إرسال الرسالة لا يتم مباشرة حتى تتم الموافقة المطلوبة.

---

10. Send Message Boundary

الـAI لا يتصل بالقناة مباشرة.

ممنوع:

Gemini
   ↓
Facebook API

أو:

Gemini
   ↓
WhatsApp API

أو أي Provider مباشرة.

المسار:

Gemini
 ↓
AI Proposal
 ↓
Mujeeb Validation
 ↓
Policy
 ↓
Authorization
 ↓
Application Service
 ↓
Provider Port
 ↓
Provider Adapter
 ↓
External Channel

وهذا هو الفصل الذي تم اعتماده في Workflow الخاص بالمشروع.

---

11. Provider Port

الـDomain/Application لا يعرف تفاصيل مزود القناة.

لا نضع داخله:

FacebookMessage
InstagramMessage
WhatsAppMessage

بصورة مرتبطة بالمزود.

لدينا حدود داخلية موحدة، ثم Adapter خاص بالمزود يتولى التحويل.

الهدف:

Mujeeb Domain/Application
        ↓
Provider Port
        ↓
Provider Adapter
        ↓
Social Provider

وهذا يمنع AI من الارتباط مباشرة بـMeta أو أي مزود آخر.

---

12. Execution لا يعيد فهم العميل

Executor لا يقوم بـ:

intent detection
product matching
semantic search
AI reasoning

هو يأخذ:

Effective Decision

وينفذها.

بمعنى:

AI
=
Reasoning

Application
=
Decision enforcement

Executor
=
Execution

---

13. Execution لا يعيد القرار التجاري

بعد وصول:

Effective Decision

لا نعيد إدخال LLM آخر ليقرر:

هل نرسل؟
هل ننشئ Lead؟
هل ننشئ Order؟

هذا محسوم قبل التنفيذ.

---

14. Idempotency

تنفيذ Action يجب ألا يؤدي إلى تكرار العملية عند إعادة نفس الحدث أو إعادة المحاولة.

خصوصًا في:

Send Message
Lead Flow
Order Flow

حدود التنفيذ يجب أن تتعامل مع إعادة المحاولة بشكل آمن.

ولا يُنشئ Retry نسخة جديدة من العملية لمجرد أن الطلب أعيد.

---

15. Failure Boundary

إذا فشل التنفيذ بعد Authorization:

Authorization
      ↓
Execution Failure

لا نعتبر العملية ناجحة.

ولا نعود إلى Gemini لكي "يخمن" أن العملية تمت.

يتم التعامل مع الفشل داخل Mujeeb.

---

16. Execution Result

يجب أن يكون هناك فصل بين:

Effective Decision

و:

Execution Result

لأن القرار قد يكون مصرحًا به لكن التنفيذ قد يفشل.

مثال:

Effective Decision
action = answer

ثم:

Execution
Provider Failure

النتيجة:

Action was authorized
but execution did not complete

ولا نغير ذلك إلى نجاح مزيف.

---

17. Human Approval لا يساوي Execution

عند:

Pending Approval

لا يتم تنفيذ العملية بعد.

المسار:

AI Proposal
 ↓
Validation
 ↓
Policy
 ↓
Pending Approval
 ↓
Human Decision
 ↓
Execution

إذا رفض الإنسان:

Reject
 ↓
No Execution

إذا عدل:

Edit
 ↓
Approved Content
 ↓
Execution

ولا نعتبر التعديل تلقائيًا Feedback تدريبي للنموذج في V1؛ هذا مذكور ضمن الـworkflow الحالي.

---

18. Action Boundary لا يغيّر AI Contract

لا نضيف إلى Gemini:

execute
send_now
create_now
approve
authorized

كصلاحيات.

Gemini يبقى داخل:

AI Proposal

والصلاحيات تبقى داخل Mujeeb.

---

19. المبدأ الخاص بالـLead والـOrder

Lead

AI
 ↓
lead_draft
 ↓
Mujeeb Sales Flow

Order

AI
 ↓
order_draft
 ↓
Missing Information
 ↓
Confirmation
 ↓
Mujeeb Order Flow

وهذا يحافظ على الفصل بين:

AI Reasoning

و:

Sales Domain

---

20. لا توجد صلاحية مباشرة للقناة

Gemini لا يحصل على:

provider token
access token
API credential
channel credential

ولا يمررها أصلًا في Proposal.

كل credentials تبقى داخل طبقة البنية التحتية/Provider Adapter في Mujeeb.

---

21. المسار النهائي

Customer
   ↓
Inbound Event
   ↓
Mujeeb
   ↓
Context
   ↓
Gemini
   ↓
AI Proposal
   ↓
Validation
   ↓
PolicyEvaluator
   ↓
Authorization
   ↓
Effective Decision
   ↓
Application Service
   ↓
Action Executor
   ↓
Provider Port / Domain Service
   ↓
External Provider or Mujeeb Domain

---

22. مثال: رد تلقائي

Customer
"كم سعر القميص؟"

↓
Gemini

AI Proposal:
action = answer
response_text = "12,000 ريال"

↓
Validation

↓
PolicyEvaluator

↓
Authorization

↓
Send Message

↓
Provider Port

↓
Channel

---

23. مثال: الموافقة البشرية

Customer
"أريد إلغاء الطلب"

↓
Gemini

AI Proposal

↓
Validation

↓
Policy

↓
Approval Required

↓
Pending Approval

↓
Human

Approve / Edit / Reject

لا يقوم Gemini بإلغاء الطلب بنفسه.

---

24. مثال: Lead

Customer
"أريد أشتري بالجملة"

↓
Gemini

lead_draft

↓
Validation

↓
Authorization

↓
Sales Application Flow

ولا يتحول بمجرد وجود Proposal إلى:

WON

---

25. مثال: Order

Customer
"أريد المنتج"

↓
Gemini

order_draft

↓
هل توجد معلومات ناقصة؟
    ↓ نعم
clarification

    ↓ لا

Confirmation

    ↓

Order Flow

هذا مطابق لعقد الـOrder الموجود في المشروع.

---

26. حدود الطبقات

Gemini

Understand
Reason
Propose

Application

Validate
Apply Policy
Authorize
Orchestrate

Domain

Business Rules
Lead
Order
Conversation
Sales State

Executor / Provider

Perform Authorized Action

---

27. ما لا نضعه في Gemini

DB credentials
Provider credentials
Authorization decision
Execution permission
Tenant identity
Direct provider access

---

28. ما لا نضعه في Executor

Customer intent interpretation
Semantic matching
LLM reasoning
Policy invention

---

29. ما تم إغلاقه

✅ LLM ≠ Executor
✅ AI Proposal ≠ Effective Decision
✅ Effective Decision ≠ Execution Result
✅ Authorization precedes Execution
✅ Approval is enforced by Mujeeb Policy
✅ answer → message execution path
✅ clarification → clarification message path
✅ human_request → human handoff path
✅ lead_draft → Sales Application Flow
✅ order_draft → Order Flow
✅ Gemini has no direct Provider access
✅ Provider credentials stay outside AI
✅ Provider abstraction remains outside Domain
✅ Execution does not re-interpret customer intent
✅ Execution does not perform semantic search
✅ Execution does not perform product matching
✅ Retry must be idempotent
✅ Execution failure is not reported as success
✅ Human edit is not automatic model training

30. القرار النهائي

Gemini
   ↓
Proposal

Mujeeb
   ↓
Validation
   ↓
Policy
   ↓
Authorization

Mujeeb Application
   ↓
Effective Action

Executor
   ↓
Authorized Execution

⑦ Execution Boundary + Action Contracts = CLOSED