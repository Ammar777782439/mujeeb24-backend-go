Mujeeb 24 — ⑥ AI Validation + Authorization Boundary

الحالة

CLOSED

هذه الخطوة تحسم ما يحدث بعد أن يعيد Gemini:

AI Proposal

والقاعدة الأساسية:

«Gemini يقترح فقط.
Mujeeb يتحقق.
Policy/Authorization يحدد ما هو مسموح.
Execution لا يبدأ إلا بعد نجاح التحقق والتفويض.»

---

1. AI Proposal ليس قرارًا نهائيًا

المخرج القادم من Gemini هو:

AI Proposal

وليس:

Final Decision

مثال:

{
  "status": "resolved",
  "action": "answer",
  "response_text": "السعر 12000 ريال",
  "selected": []
}

هذا مجرد Proposal.

لا يتم إرساله أو تنفيذه مباشرة.

---

2. مسار ما بعد Gemini

Gemini
   ↓
AI Proposal
   ↓
Structural Validation
   ↓
Reference Validation
   ↓
Tenant / Ownership Validation
   ↓
Business Policy Evaluation
   ↓
Authorization
   ↓
Effective Decision
   ↓
Execution

إذا فشل أي حاجز، لا ننتقل إلى الحاجز التالي.

---

3. Structural Validation

أول شيء يفعله Mujeeb هو التحقق من أن مخرج Gemini يطابق الـAI Output Contract.

التحقق يشمل:

status
action
response_text
selected

ويجب أن تكون الأنواع والبنية صحيحة.

الـStructured Output من Gemini يساعد في ضمان البنية المطلوبة، لكنه لا يجعل القيم صحيحة تجاريًا؛ لذلك هذا التحقق داخل Mujeeb إلزامي.

---

4. Allowed Status

الـAI Contract المغلق يستخدم:

resolved
ambiguous
not_found
needs_more_data

أي قيمة أخرى تعتبر:

Invalid AI Proposal

ولا تنتقل إلى مرحلة Authorization أو Execution.

---

5. Allowed Action

الـAI Contract المغلق يستخدم:

answer
clarification
human_request
lead_draft
order_draft

أي قيمة أخرى تعتبر Proposal غير صالح.

---

6. Reference Validation

إذا أعاد Gemini:

{
  "selected": [
    {
      "item_id": "...",
      "variant_id": "...",
      "offer_id": "..."
    }
  ]
}

فـMujeeb لا يصدق هذه المراجع لمجرد أن Gemini أرسلها.

يتحقق من أن:

item_id
variant_id
offer_id

مراجع حقيقية ضمن البيانات المتاحة للمحادثة/التقييم الحالي.

---

7. لا نسمح لمخرج Gemini بإنشاء Reference من نفسه

مثال غير صالح:

Gemini
   ↓
offer_id = "invented-id"

حتى لو كان UUID صحيح الشكل.

التحقق يجب أن يكون ضد بيانات Mujeeb الحقيقية.

---

8. Tenant Validation

كل Reference يجب أن يكون ضمن Business الحالي.

المسار:

Authenticated Business
        ↓
Catalog Scope
        ↓
Referenced Item / Variant / Offer

ولا يكفي:

WHERE id = ?

بل يجب أن يكون الوصول داخل الـBusiness Scope الصحيح.

إذا كان الـReference من Business آخر:

Do not expose the resource
Do not treat it as valid
Do not leak its existence

ويظل مبدأ Tenant Isolation هو نفسه المقفل في Mujeeb.

---

9. لا نثق في business_id من Gemini

Gemini لا يملك سلطة تحديد:

business_id
tenant_id

ولا يمكن أن يقول:

{
  "business_id": "other-business"
}

ثم يستخدمه Mujeeb.

الـBusiness يأتي من:

Authenticated Context

داخل Mujeeb فقط.

---

10. Evidence Validation

عند وجود:

selected

يجب أن تكون المراجع مبنية على بيانات Catalog التي كانت متاحة فعلًا لـGemini.

أي:

Actual Catalog Data
        ↓
AI Projection
        ↓
Gemini
        ↓
selected references
        ↓
Mujeeb validates

ولا نسمح باستخدام Reference لم يظهر ضمن النطاق/البيانات التي تعامل معها AI.

---

11. Mujeeb لا يعيد تفسير نية العميل

هذه نقطة مهمة جدًا.

Mujeeb لا يفعل:

Gemini said:
"أرغب بقميص أسود"

Mujeeb:
"أنا سأبحث عن القميص الأسود"

هذا ممنوع.

دور Mujeeb هنا هو Validation وليس إعادة بناء قرار Gemini.

Mujeeb يتحقق من:

هل المرجع موجود؟
هل ينتمي للتاجر؟
هل البيانات صالحة؟
هل الـAction مسموح؟
هل Policy تسمح؟

ولا يقوم بعمل Semantic Product Matching جديد.

---

12. Policy Evaluation

بعد نجاح Structural + Reference + Tenant Validation:

AI Proposal
    ↓
PolicyEvaluator

ويتم تطبيق Business Policy الموجودة فعليًا في Mujeeb.

مثال موجود في العقد:

{
  "send_message_requires_approval": true,
  "allow_handoff": true
}

لكن هذه القيم لا تأتي من Gemini كسلطة نهائية.

تأتي من Mujeeb.

---

13. requires_approval

Gemini لا يقرر القرار النهائي:

requires_approval

حتى لو حاول إرساله.

القرار النهائي يتم حسابه بواسطة:

Mujeeb PolicyEvaluator

مثال:

Gemini Proposal
requires_approval = false

لكن:

Business Policy
send_message_requires_approval = true

فالقرار الفعلي:

requires_approval = true

لأن Mujeeb هو صاحب السلطة.

---

14. Human Handoff

إذا كانت نتيجة Gemini:

action = human_request

فإن Mujeeb يطبق سياسة الـhandoff الحالية.

Gemini لا يستطيع تجاوز:

Business Policy
Ownership
Conversation State

والـhandoff لا يتم بمجرد نص في الـprompt.

---

15. Lead Draft

إذا كانت النتيجة:

action = lead_draft

فهذا يعني:

AI prepared a Lead proposal

ولا يعني:

Lead created

إنشاء Lead فعلي لا يحدث إلا بعد المرور عبر حدود Mujeeb المقررة لهذا الفعل.

---

16. Order Draft

إذا كانت:

action = order_draft

فهذا يعني:

AI prepared an Order proposal

ولا يعني:

Order created

إنشاء Order فعلي منفصل عن Gemini.

والقاعدة المقفلة سابقًا:

Prepare Draft
↓
Ask Missing Info
↓
Confirmation
↓
Order

الـAI لا ينشئ Order من تخمين.

---

17. Effective Decision

بعد اكتمال:

Structural Validation
+
Reference Validation
+
Tenant Validation
+
Policy Evaluation
+
Authorization

ننتج:

Effective Decision

وهذه هي النتيجة التي يمكن أن تدخل مسار التنفيذ.

ليست هي نفس AI Proposal بالضرورة.

---

18. الفرق بين AI Proposal و Effective Decision

AI Proposal
=
ما اقترحه Gemini

Effective Decision
=
ما سمح به Mujeeb بعد التحقق والسياسات والتفويض

مثال:

Gemini:
action = answer

لكن Policy تمنع الإرسال التلقائي.

إذن:

AI Proposal
   ↓
PolicyEvaluator
   ↓
Approval Required

---

19. Execution Boundary

حتى بعد Authorization:

Mujeeb
   ↓
Authorized Action
   ↓
Executor

ولا يحدث:

Gemini
   ↓
Facebook / WhatsApp / Social API

مباشرة.

الـAI لا يملك قناة تنفيذ مباشرة.

---

20. Validation لا تعني Semantic Re-Reasoning

Mujeeb لا يصبح AI ثانيًا.

لذلك لا نضيف:

semantic validator
AI matcher
secondary LLM
product-ranking validator

كطبقات جديدة.

التحقق هنا Deterministic قدر الإمكان:

Schema
Reference
Ownership
Tenant
Policy
Authorization
State

أما الاستدلال وفهم العميل فيبقى عند Gemini.

---

21. ما الذي يحدث إذا كان Output غير صالح؟

إذا فشل الـProposal في Structural Validation أو Reference Validation أو Tenant Validation:

لا Execution

ولا يتم اعتبار الـProposal قرارًا صالحًا.

ولا نحوله إلى Action قسريًا.

ولا نخترع بيانات بديلة.

---

22. لا نضيف Status جديد للمستخدم

لا نضيف إلى AI Contract:

invalid
rejected
unauthorized

كـAI statuses جديدة.

هذه نتائج داخلية لطبقة Mujeeb Validation/Authorization، وليست حالات Gemini التجارية المقفلة.

---

23. المصدر النهائي للحقيقة

بالترتيب:

Catalog Truth
      ↓
PostgreSQL / Mujeeb

Conversation Truth
      ↓
Mujeeb

Business Policy Truth
      ↓
Mujeeb

AI Reasoning
      ↓
Gemini

Final Authorization
      ↓
Mujeeb

Execution
      ↓
Mujeeb

---

24. المسار النهائي الكامل

Customer
   ↓
Message
   ↓
Mujeeb Context Builder
   ↓
Gemini
   ↓
AI Proposal
   ↓
Structural Validation
   ↓
Reference Validation
   ↓
Tenant / Ownership Validation
   ↓
PolicyEvaluator
   ↓
Authorization
   ↓
Effective Decision
   ↓
Executor
   ↓
External Channel / Domain Action

---

25. المسؤوليات النهائية

Gemini

Understand
Reason
Compare
Use evidence
Propose

Mujeeb

Validate
Scope
Verify references
Apply policy
Authorize
Execute

---

26. ما تم إغلاقه

✅ AI Proposal is not final authority
✅ Structured validation
✅ Allowed status validation
✅ Allowed action validation
✅ Reference validation
✅ Tenant validation
✅ Ownership validation
✅ Business policy evaluation
✅ requires_approval controlled by Mujeeb
✅ Human handoff controlled by Mujeeb
✅ Lead remains draft until authorized process
✅ Order remains draft until authorized process
✅ No direct Gemini execution
✅ No semantic re-matching inside Mujeeb
✅ No secondary AI validator required
✅ Effective Decision separated from AI Proposal
✅ Execution only after authorization

27. العقد النهائي

GEMINI
   │
   │ AI Proposal
   ▼
MUJEEB VALIDATOR
   │
   ├── Structure
   ├── References
   ├── Tenant
   ├── Ownership
   └── State
   │
   ▼
POLICY EVALUATOR
   │
   └── Business Policies
   │
   ▼
AUTHORIZATION
   │
   ▼
EFFECTIVE DECISION
   │
   ▼
EXECUTION

القرار

⑥ AI Validation + Authorization Boundary = CLOSED