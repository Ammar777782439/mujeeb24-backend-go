Mujeeb 24 — ⑨ AI Runtime Lifecycle + Failure / Retry / Timeout Contract

الحالة

CLOSED

هذه الخطوة تحسم دورة تشغيل AI الواحدة داخل Mujeeb 24، وكيف نتعامل مع:

- النجاح.
- انتظار Tool.
- الفشل.
- Retry.
- Timeout.
- Provider Failure.
- Validation Failure.
- Execution Failure.
- Duplicate Processing.
- Cancellation.

ولا تغيّر أي قرار مغلق سابقًا.

---

1. AI Run

كل معالجة لرسالة/حدث AI تمثل:

AI Run

ولها معرف:

ai_run_id

وهو معرف تتبع وتشغيل، وليس Domain Entity تجاري.

يرتبط بالسياق:

business_id
conversation_id
message_id

وعند استخدام Gemini:

gemini_interaction_id

---

2. Lifecycle النهائي

الحالات التشغيلية:

RECEIVED
CONTEXT_BUILT
RUNNING
WAITING_TOOL
VALIDATING
AUTHORIZED
EXECUTING
COMPLETED
FAILED
CANCELLED

المعنى:

RECEIVED

تم إنشاء AI Run للحدث المطلوب معالجته.

CONTEXT_BUILT

تم تجهيز:

System Rules
Business Context
Conversation Context
Conversation State
Catalog Entity Contract
Current Message

RUNNING

AI Runtime يتعامل مع Gemini.

WAITING_TOOL

Gemini طلب Tool، وMujeeb لم يُكمل الجولة بعد.

VALIDATING

وصل AI Proposal إلى Mujeeb وبدأ التحقق.

AUTHORIZED

اكتمل التحقق والتفويض وأصبح هناك Effective Decision صالح للتنفيذ.

EXECUTING

بدأ تنفيذ Effective Decision.

COMPLETED

اكتملت العملية النهائية بنجاح.

FAILED

انتهى الـRun بسبب فشل غير قابل للاستكمال أو بعد استنفاد Retry المسموح.

CANCELLED

تم إلغاء التشغيل قبل اكتماله.

---

3. المسار الطبيعي

RECEIVED
   ↓
CONTEXT_BUILT
   ↓
RUNNING
   ↓
WAITING_TOOL   ← عند الحاجة
   ↓
RUNNING
   ↓
VALIDATING
   ↓
AUTHORIZED
   ↓
EXECUTING
   ↓
COMPLETED

وفي حالة عدم وجود Action تنفيذية:

VALIDATING
   ↓
COMPLETED

بحسب القرار الناتج.

---

4. Tool Loop

إذا احتاج Gemini بيانات إضافية:

RUNNING
   ↓
WAITING_TOOL
   ↓
Mujeeb executes Tool
   ↓
Tool Result
   ↓
RUNNING

يمكن تكرار ذلك.

ولا يوجد:

max 6 rounds

ولا عدد جولات ثابت.

هذا القرار مغلق سابقًا. استمرار الـAI يتوقف عند كفاية الأدلة أو حدود الاقتصاد/الأمان/المزوّد/الموارد.

---

5. Retry Principle

الـRetry ليس متاحًا لكل الأخطاء.

نقسم الفشل إلى:

Retryable
Non-Retryable

---

6. Retryable Failures

الفشل القابل لإعادة المحاولة هو الفشل المؤقت الذي يمكن أن ينجح عند المحاولة التالية.

أمثلة الفئات:

Provider temporary failure
Transient network failure
Temporary infrastructure failure
Rate-limit condition
Temporary Tool dependency failure

الـRetry يتم من خلال Runtime/Infrastructure وليس Gemini.

---

7. Non-Retryable Failures

لا نعيد المحاولة عند أخطاء مثل:

Invalid AI Output
Invalid Tool Arguments
Tenant Violation
Invalid Reference
Policy Denial
Authorization Denial
Unsupported Action
Corrupt/invalid request

لأن إعادة نفس المدخل غالبًا لن تحل المشكلة.

---

8. لا يوجد رقم ثابت للـRetries في العقد

لا نثبت:

3 retries

أو:

5 retries

داخل الـDomain Contract.

عدد المحاولات وسلوك Backoff يكون Configuration تشغيليًا في Runtime.

السبب:

- مزود AI قد يختلف.
- نوع الفشل قد يختلف.
- البيئة قد تختلف.
- حدود الموارد قد تختلف.

لكن يجب أن يكون هناك حد تشغيلي لمنع Retry Loop غير المنتهي.

---

9. Backoff

عند Retry لفشل مؤقت، لا نعيد الطلب فورًا في Loop سريع.

يستخدم Runtime:

Retry
+
Backoff

والـBackoff قيمة تشغيلية قابلة للضبط.

لا توجد قيمة رقمية ثابتة داخل هذا العقد.

---

10. Provider Rate Limit

عند وصول حد مزود AI:

Gemini
   ↓
Rate Limit

لا نعتبر الـAI Decision فاشلًا معنويًا.

بل:

Temporary Provider Failure

ويطبق Runtime سياسة Retry/Backoff المخصصة لذلك.

---

11. Timeout

كل عملية خارجية يجب أن تعمل ضمن Timeout.

يشمل ذلك:

Gemini Request
Tool Execution
Provider Execution

والـTimeout configuration جزء من Runtime، وليس Domain.

---

12. AI Timeout

إذا انتهى Timeout أثناء انتظار Gemini:

RUNNING
   ↓
TIMEOUT

ويُعامل كفشل تشغيلي.

إذا كان قابلًا لإعادة المحاولة وفق Runtime:

Retry

وإلا:

FAILED

لا ننتج Proposal مزيفًا.

---

13. Tool Timeout

إذا فشل Tool بسبب Timeout:

WAITING_TOOL
   ↓
Tool Timeout

ثم:

Retryable
   ↓
Retry Tool

Non-Retryable
   ↓
FAILED

Gemini لا يفترض أن Tool نجح إذا لم يصل Result ناجح.

---

14. Execution Timeout

إذا حصل:

AUTHORIZED
   ↓
EXECUTING
   ↓
Timeout

فلا نغير النتيجة إلى "COMPLETED".

نحتاج Execution Result صريحًا.

إذا كانت العملية الخارجية تدعم Retry الآمن:

Retry

وإلا:

FAILED

---

15. Idempotency

كل AI Run يجب أن يكون قابلًا للتمييز عن إعادة نفس الحدث.

الهدف:

«Retry لا ينتج معالجة تجارية مكررة.»

نستخدم:

ai_run_id

كهوية تشغيلية أساسية.

وعند وجود خطوات/Actions قابلة للتكرار، تحتاج العملية التنفيذية إلى Idempotency Key مناسب للعملية.

---

16. AI Run لا يُنشأ مرتين لنفس الحدث

إذا وصل نفس "message/event" مرة أخرى بسبب Retry أو Duplicate Event:

same source event
       ↓
existing AI Run
       ↓
do not create duplicate logical run

بل يُستأنف/يُفحص حسب حالة الـRun الموجودة.

---

17. لا نستخدم Gemini كآلية Retry

إذا فشل:

Network
Provider
Tool
Execution

Mujeeb Runtime هو الذي يدير Retry.

ولا نرسل إلى Gemini:

«"حاول مرة ثانية لأن الطلب السابق فشل."»

إلا إذا كان هناك AI Interaction جديدة مبررة ضمن Runtime.

---

18. Validation Failure

إذا رجع Gemini Proposal غير صالح:

RUNNING
   ↓
VALIDATING
   ↓
Validation Failed
   ↓
FAILED

ولا يتم:

Retry Execution

ولا:

Authorization

ولا:

Send

---

19. Policy / Authorization Failure

إذا كان:

Policy = denied

أو:

Authorization = denied

فالنتيجة:

No Execution

وهذا ليس Provider Failure.

ولا يتم Retry تلقائي.

---

20. Execution Failure

إذا كان القرار مصرحًا:

AUTHORIZED
   ↓
EXECUTING

لكن التنفيذ فشل:

Execution Failed

فهذا لا يلغي حقيقة أن القرار كان:

AUTHORIZED

لكن:

Execution Result = FAILED

ولا نسجل نجاحًا غير حقيقي.

---

21. Partial Progress

إذا كانت العملية عبارة عن عدة خطوات Tool:

Batch 1 ✅
Batch 2 ✅
Batch 3 ❌

لا نعيد كل العملية تلقائيًا من البداية.

Runtime يعيد المحاولة من الوحدة القابلة لإعادة التنفيذ فقط، وفق حالة الـRun وIdempotency.

وهذا مهم خصوصًا مع:

Catalog Evaluation

حتى لا نعيد تشغيل كل الـBatches الناجحة بلا سبب.

---

22. Catalog Batch State

كل Batch ضمن عملية التقييم يجب أن يكون معروفًا على مستوى الـRuntime:

PENDING
RUNNING
COMPLETED
FAILED

وبذلك:

Batch 1 = COMPLETED
Batch 2 = COMPLETED
Batch 3 = FAILED
Batch 4 = PENDING

يمكن متابعة العملية دون فقدان التقدم السابق.

---

23. لا نعتبر Catalog Evaluation مكتملة قبل اكتمال النطاق

القاعدة المغلقة سابقًا:

Total Batches
      ↓
Completed Batches
      ↓
Coverage Complete

لا Final Evaluation قبل تحقق الـCoverage المطلوب.

---

24. Cancellation

يمكن إلغاء AI Run قبل اكتماله:

RECEIVED
CONTEXT_BUILT
RUNNING
WAITING_TOOL
VALIDATING

إذا كان الإلغاء ممكنًا.

النتيجة:

CANCELLED

ولا يتم تنفيذ Action بعد الإلغاء.

---

25. Cancel أثناء Execution

إذا بدأت العملية الخارجية بالفعل:

EXECUTING

فالإلغاء لا يعني تلقائيًا أن العملية الخارجية انعكست.

لذلك يجب التفريق بين:

Run cancelled

و:

External action successfully rolled back

ولا نفترض وجود Rollback إلا إذا كان الـAction نفسه يدعمه.

---

26. State Transition Rule

لا نسمح بانتقالات عشوائية.

مثال:

COMPLETED
   ↓
RUNNING

غير صالح.

وكذلك:

FAILED
   ↓
EXECUTING

غير صالح إلا عبر Runtime retry flow مصرح به يبدأ كتشغيل جديد/محاولة تشغيلية واضحة.

---

27. Attempt vs AI Run

نفرق بين:

AI Run

و:

Attempt

"AI Run" = العملية المنطقية الكاملة.

"Attempt" = محاولة تشغيل داخل الـRun.

مثال:

AI Run #100
   ├── Attempt 1 → timeout
   ├── Attempt 2 → provider success
   └── Completed

هذا مهم للتتبع والمراقبة.

---

28. Runtime لا يغيّر Business State مباشرة أثناء Retry

Retry:

Infrastructure concern

ولا يجب أن يؤدي إلى:

Lead status mutation
Order mutation
Conversation ownership mutation

لمجرد أنه Retry.

Business State يتغير عند وصول نتيجة صحيحة إلى Application/Domain.

---

29. Provider Failure لا يغيّر Conversation إلى Human تلقائيًا

فشل Gemini وحده لا يعني تلقائيًا:

WAITING_HUMAN

هذه نتيجة سياسة/Workflow منفصلة.

إذا كان النظام أو Policy يقرر Handoff:

Failure
   ↓
Application Policy
   ↓
Human Handoff

أما Runtime فلا يخترع هذا القرار.

---

30. Failure Visibility

كل فشل مهم يجب أن يكون ظاهرًا في:

AI Trace
Observability

ويظهر على الأقل:

ai_run_id
failure stage
failure category
attempt
timestamp

حتى نعرف:

هل فشل Gemini؟
Tool؟
Validation؟
Authorization؟
Execution؟

---

31. Final Result

الـAI Run ينتهي إلى حالة نهائية:

COMPLETED
FAILED
CANCELLED

و"COMPLETED" لا تعني دائمًا:

«تم إرسال رسالة خارجية.»

بل تعني أن الـRun نفسه انتهى وفق مساره بنجاح.

أما نجاح Action الخارجية فيظهر من:

Execution Result

---

32. المسار النهائي

Event
 ↓
AI Run
 ↓
CONTEXT_BUILT
 ↓
RUNNING
 │
 ├── Tool needed
 │      ↓
 │  WAITING_TOOL
 │      ↓
 │  Tool Result
 │      ↓
 │  RUNNING
 │
 └── AI Proposal
        ↓
    VALIDATING
        ↓
    Policy / Authorization
        ↓
    AUTHORIZED
        ↓
    EXECUTING
        ↓
    Execution Result
        │
        ├── Success → COMPLETED
        └── Failure → Retry / FAILED

---

33. المسؤوليات

AI Runtime

Start Run
Build/Request Context
Call Gemini
Handle Tool Loop
Track Attempts
Apply Timeout
Apply Retry
Propagate Results
Finish Run

Application

Validate Proposal
Apply Policy
Authorize
Create Effective Decision
Start Authorized Action

Domain

Business Rules
Conversation State
Lead
Order
Sales State

Infrastructure

HTTP
Queue
Retry Scheduler
Persistence
Provider Adapters
Observability

---

34. ما لم نفعله

لا نضع:

❌ Max 6 AI rounds
❌ Fixed retry count in Domain
❌ Fixed timeout in Domain
❌ Semantic retry
❌ Second AI to repair AI
❌ Automatic human takeover on every AI failure
❌ Re-run completed Catalog batches
❌ Duplicate order on retry
❌ Duplicate outbound message on retry

---

35. القرار النهائي

AI Run
=
Logical AI Processing

Attempt
=
Operational Execution Attempt

Retry
=
Infrastructure/Runtime responsibility

Policy
=
Mujeeb responsibility

Authorization
=
Mujeeb responsibility

Execution
=
Mujeeb responsibility

Gemini
=
Reasoning only

ما تم إغلاقه

✅ AI Run lifecycle
✅ Attempt concept
✅ Tool waiting state
✅ Retry boundary
✅ Timeout boundary
✅ Retryable vs non-retryable failures
✅ Idempotency principle
✅ Duplicate event handling
✅ Catalog batch recovery
✅ Validation failure handling
✅ Policy denial handling
✅ Authorization denial handling
✅ Execution failure handling
✅ Cancellation
✅ Partial progress
✅ Coverage completion
✅ Failure observability
✅ Final Run states
✅ No fixed AI round ceiling
✅ No fixed retry count in Domain

القرار

⑨ AI Runtime Lifecycle + Failure / Retry / Timeout Contract = CLOSED