MUJEEB 24

AI Usage, Token Telemetry & Subscription Consumption Contract

العقد النهائي لاستهلاك AI والتوكنات والتكلفة

الحالة: CLOSED

هذا العقد جزء إلزامي من:

Platform Administration Contract V1

---

1. الفصل الأساسي

يوجد في Mujeeb 24 مقياسان مختلفان تمامًا:

Merchant Entitlement
        ↓
AI Replies

و:

Platform Consumption
        ↓
Tokens
Model Requests
Tool Calls
Provider Cost

ولا يجوز دمجهما في حقل واحد.

---

2. ما يشتريه التاجر

التاجر لا يشتري:

Tokens
Model Requests
Gemini Calls
Tool Calls

التاجر يشتري:

AI Replies

والباقات الحالية:

Basic
5,000 YER
500 AI Replies

Growth
10,000 YER
1,500 AI Replies

Business
20,000 YER
4,000 AI Replies

وهذه هي الـMerchant Entitlement.

---

3. ما الذي يحسب كـAI Reply؟

يُحسب Reply واحد فقط عندما ينتج النظام:

Final AI Response

ويصبح صالحًا كاستجابة نهائية للتاجر/العميل.

لا يحسب:

Tool Call
Discovery Call
Model Retry
Internal Validation
Prompt Construction
Context Construction
Provider Request

كـAI Reply مستقل.

---

4. حساب المتبقي للتاجر

لكل Subscription:

ai_reply_limit
ai_replies_used
ai_replies_remaining

والقيمة:

ai_replies_remaining =
MAX(ai_reply_limit - ai_replies_used, 0)

مثال:

Plan:
Basic

Limit:
500

Used:
173

Remaining:
327

---

5. التوكنات لا تملك Remaining Quota افتراضيًا

لا نعرّف:

500,000 tokens included

ولا:

1,000,000 tokens remaining

للعميل.

لأن العقد التجاري لا يبيع Tokens.

لذلك:

AI Replies
= Entitlement

Tokens
= Consumption Telemetry

---

6. Token Telemetry

كل AI Execution يسجل:

ai_usage_record

ويحتوي:

id
business_id
subscription_id

provider
model

input_tokens
cached_input_tokens
output_tokens

model_requests
tool_calls

final_ai_replies

provider_cost
provider_currency

started_at
completed_at

status
failure_code
correlation_id

---

7. لماذا نخزن Subscription ID؟

حتى نعرف بالضبط:

Business
   ↓
Subscription #12
   ↓
AI Usage

ولا نحسب استهلاك تاجر على الاشتراك الحالي فقط.

إذا انتقل التاجر من:

Basic #1

إلى:

Growth #2

تبقى جميع أرقام Basic #1 تاريخية.

---

8. Subscription AI Usage

لكل Subscription يوجد Aggregate:

SubscriptionAIUsage

ويحتوي:

ai_reply_limit
ai_replies_used
ai_replies_remaining

input_tokens
cached_input_tokens
output_tokens

model_requests
tool_calls

provider_cost

---

9. Platform Admin يرى الاثنين معًا

صفحة:

/admin/subscriptions/{subscription_id}/ai

تعرض:

AI Replies
────────────────
Limit
Used
Remaining

Provider Consumption
────────────────────
Input Tokens
Cached Input Tokens
Output Tokens
Model Requests
Tool Calls

Cost
────
Actual Provider Cost
Internal Cost Budget
Cost Consumed %
Cost Remaining
Average Cost / AI Reply
Projected Remaining Cost

وهذه هي الشاشة التي يحتاجها مدير النظام فعلًا.

---

10. Average Cost per AI Reply

يحسب:

average_cost_per_reply =
provider_cost / ai_replies_used

مع التعامل مع حالة:

ai_replies_used = 0

بحيث لا يحدث Division by Zero.

---

11. Projected Remaining Cost

مدير النظام يحتاج أن يعرف:

«لو استهلك التاجر كل الردود المتبقية، كم نتوقع أن ندفع لـGemini؟»

لذلك:

projected_remaining_cost =
average_cost_per_reply
×
ai_replies_remaining

---

12. Projected Total Cost

ويعرض النظام أيضًا:

projected_total_cost =
provider_cost
+
projected_remaining_cost

وهذا يعطي مدير النظام صورة واضحة عن تكلفة الاشتراك كاملة.

---

13. Internal AI Cost Budget

لكل خطة يوجد:

internal_ai_cost_budget_yer

القيم المعتمدة في baseline الحالي:

Basic
1,000 YER / month

Growth
3,500 YER / month

Business
9,000 YER / month

وهذه ليست رسومًا إضافية على التاجر وليست Token Quota.

إنها:

Platform Cost Budget

تستخدمها المنصة لمراقبة اقتصاد الاشتراك.

---

14. Cost Budget Remaining

لكل Subscription:

cost_budget_yer
cost_consumed_yer
cost_remaining_yer

والحساب:

cost_remaining_yer =
MAX(cost_budget_yer - provider_cost_yer, 0)

مثال:

Growth

Budget:
3,500 YER

Consumed:
1,420 YER

Remaining:
2,080 YER

---

15. Cost Budget لا يساوي Token Budget

إذا كان لدينا:

Provider Cost:
1,420 YER

فلا نقول:

Remaining Tokens = X

لأن تكلفة Token تعتمد على:

model
input
cached input
output
pricing version

لذلك القيمة authoritative هي:

Actual Provider Cost

وليس تحويلًا تخمينيًا من YER إلى Tokens.

---

16. Cost per Reply

مدير النظام يرى:

Average AI Cost / Reply

وهو أهم مؤشر اقتصادي.

مثلاً:

Basic

AI Replies Used:
200

Provider Cost:
430 YER

Average:
2.15 YER / AI Reply

وهذا يسمح للمدير بمعرفة هل الـ500 Reply المدرجة اقتصاديًا مناسبة فعلًا أم لا.

---

17. Cost Budget Status

لكل Subscription:

NORMAL
WARNING
EXCEEDED

القواعد:

NORMAL
< 80%

WARNING
>= 80% and < 100%

EXCEEDED
>= 100%

---

18. ماذا يحدث عند WARNING؟

لا نوقف AI.

يظهر Platform Alert:

AI cost budget nearing limit

ويرى المدير:

Business
Plan
Current Cost
Budget
Remaining Replies
Projected Cost

---

19. ماذا يحدث عند EXCEEDED؟

عند تجاوز الـInternal AI Cost Budget:

لا نحذف الاشتراك.

ولا نغير:

ai_replies_remaining

ولا نحذف بيانات التاجر.

لكن يتم تفعيل:

AI Cost Protection

وتنتقل معالجة AI الخاصة بهذا الاشتراك إلى:

restricted_ai

والقاعدة التشغيلية:

لا يبدأ Auto AI Execution جديد

إذا كان تنفيذ AI سيؤدي إلى تجاوز Cost Guardrail.

ويظل:

Human Handling
Human Replies
Merchant Dashboard
Customer Data
Leads
Orders

عاملًا بشكل طبيعي.

---

20. لماذا هذه القاعدة ضرورية؟

لأن لدينا حالتين ممكنتين:

500 AI Replies

لكن كل Reply أصبح يستهلك عدة Model Requests وDiscovery بشكل غير اقتصادي.

لو اعتمدنا Replies فقط دون Cost Protection:

Merchant
500 Replies
        ↓
Provider Cost
↑↑↑
        ↓
Mujeeb Loss

وهذا غير مقبول اقتصاديًا.

---

21. Cost Protection ليس Round Limit

لا نضع:

Max 1 Tool Call
Max 3 Requests
Max 6 Rounds

لأن هذا يخالف قرارنا السابق.

الـAI يستطيع تنفيذ عدد الطلبات اللازمة لإكمال المهمة، لكن داخل:

Subscription Entitlement
+
Provider Limits
+
Platform Cost Protection
+
Security

وهذا يتوافق مع القرار الاقتصادي الحالي الذي رفض سقف جولات ثابتًا.

---

22. Platform Admin يستطيع التغيير

مدير النظام يستطيع تعديل:

Plan Price
AI Reply Limit
AI Catalog Limit
Channel Limit
Internal AI Cost Budget

لكن لا نعدل Plan Version مستخدمة تاريخيًا.

عند تغيير هذه القيم:

Current Plan Version
        ↓
New Plan Version

---

23. مثال تغيير AI Cost Budget

مثلاً:

Basic v1
AI Cost Budget = 1,000 YER

ثم يكتشف المدير من البيانات الفعلية أن التكلفة تغيرت.

ينشئ:

Basic v2
AI Cost Budget = 1,300 YER

الاشتراكات القديمة تبقى مرتبطة بـ:

Basic v1

والاشتراكات الجديدة تستخدم:

Basic v2

ولا نغير الماضي.

---

24. Platform Admin يستطيع Override للاشتراك الحالي

هناك حالة تشغيلية مختلفة عن تغيير Plan.

إذا احتاج المدير معالجة اشتراك محدد، يوجد:

AI Cost Budget Override

على مستوى Subscription.

يُسجل:

subscription_id
old_budget
new_budget
reason
actor_platform_admin_id
created_at

ويظهر في Platform Audit.

هذا لا يغير Plan Version.

---

25. لا يوجد Token Override

لا يوجد:

Increase Tokens
Decrease Tokens
Reset Tokens

لأن Tokens ليست Merchant Entitlement.

يوجد:

Increase AI Reply Entitlement
Decrease AI Reply Entitlement
Increase Cost Budget
Decrease Cost Budget

---

26. Reset Usage

Usage لا يعمل بزر:

Reset Tokens

عند انتهاء Subscription:

Subscription #1

تنتهي.

ثم:

Subscription #2

تبدأ Usage جديدة.

وبالتالي:

AI Replies Used

تبدأ من:

0

للاشتراك الجديد.

أما سجلات الاشتراك القديم فتبقى تاريخية.

---

27. Usage Dashboard

مدير النظام يرى على مستوى المنصة:

/admin/operations/ai

Platform AI Usage

Total AI Replies
Total Input Tokens
Total Cached Tokens
Total Output Tokens

Total Model Requests
Total Tool Calls

Total Provider Cost

Average Cost / Reply

Active AI Cost Budgets
Budget Consumed
Budget Remaining

---

28. Merchant Ranking غير مطلوب

لا نبني:

Top 10 expensive merchants
Worst merchants
Most costly merchants

بدون سبب.

لكن يجوز وجود:

AI Cost Alerts

لتحديد الاشتراكات التي تحتاج تدخلًا.

---

29. Cost Alert

عندما:

cost_budget >= 80%

يظهر:

WARNING

وعند:

cost_budget >= 100%

يظهر:

EXCEEDED

ويطبق "AI Cost Protection".

---

30. Provider Pricing

لا نضع أسعار Gemini داخل Business أو Subscription نفسها.

الأسعار تدخل من:

AI Provider Pricing

ويرتبط Usage Record بـ:

pricing_version

حتى لا تصبح الحسابات التاريخية خاطئة إذا تغير سعر المزود.

---

31. Usage Calculation

التكلفة الفعلية تعتمد على:

input_tokens
cached_input_tokens
output_tokens
provider pricing version

ولا تعتمد على:

AI Replies × fixed cost

لأن Reply واحدة قد تحتاج عدة Model Requests أو Tool Calls.

وهذا بالضبط سبب تسجيل Telemetry الفعلية. baseline المشروع ينص على قياس input/cached/output tokens وmodel requests وtool calls وcatalog projection tokens وcost per AI reply قبل الاعتماد الاقتصادي النهائي.

---

32. Final Subscription AI View

لكل اشتراك تظهر للـPlatform Admin بطاقة:

PLAN
Growth v1

AI REPLIES
1,500 Limit
620 Used
880 Remaining

TOKENS
Input:
...

Cached:
...

Output:
...

REQUESTS
Model Requests:
...

Tool Calls:
...

COST
Actual:
1,420 YER

Budget:
3,500 YER

Remaining Budget:
2,080 YER

Average / Reply:
...

Projected Remaining Cost:
...

Status:
NORMAL

---

33. هذه هي الإجابة الدقيقة على "كم عاد باقي؟"

مدير النظام سيجد أربع قيم مختلفة:

1. AI Replies Remaining
   ما تبقى للتاجر من حقه التجاري.

2. Cost Budget Remaining
   ما تبقى من الميزانية الاقتصادية الداخلية.

3. Actual Provider Cost
   ما دفعناه فعليًا للمزود حتى الآن.

4. Projected Remaining Cost
   ما نتوقع دفعه إذا استُخدمت بقية الردود.

أما:

Tokens Remaining

فلا يوجد كقيمة اشتراك، لأن الاشتراك ليس Token-based.

---

34. API النهائي

Subscription AI Usage

GET /api/v1/platform/subscriptions/{subscription_id}/ai-usage

Platform AI Usage

GET /api/v1/platform/ai/usage
GET /api/v1/platform/ai/usage/by-subscription
GET /api/v1/platform/ai/usage/by-business

Cost Budget Override

POST /api/v1/platform/subscriptions/{subscription_id}/ai-cost-budget

Request:

{
  "budget_yer": 3500,
  "reason": "Updated internal AI cost guardrail"
}

Plan AI Cost Budget

يعدل من خلال:

POST /api/v1/platform/plans/{plan_id}/versions

ولا يوجد تعديل صامت للخطة التاريخية.

---

35. Database Ownership

يضاف إلى Platform-side usage model:

ai_usage_records
subscription_ai_usage
ai_provider_pricing_versions

ويحتفظ:

subscriptions

بالـPlan/Period reference.

---

36. Audit

هذه العمليات كلها Audited:

AI Runtime enabled
AI Runtime disabled

AI Cost Budget changed
Subscription Cost Budget overridden

Plan AI Reply Limit changed
Plan Internal AI Cost Budget changed

Provider pricing version changed

---

37. القرار النهائي

من الآن لدينا:

Merchant Billing Metric
        =
AI Replies

و:

Platform Operational Metric
        =
Tokens + Requests + Tool Calls + Provider Cost

و:

Platform Economic Guardrail
        =
Internal AI Cost Budget

و:

Platform Visibility
        =
Remaining Replies
+
Actual Token Consumption
+
Actual Cost
+
Projected Cost
+
Budget Remaining

وهكذا مدير النظام لا يرى فقط:

«"التاجر معه 327 رد."»

بل يرى الصورة التي يحتاجها فعلًا:

«التاجر معه 327 رد، استهلك حتى الآن X input tokens وY cached tokens وZ output tokens، كلفنا ذلك X ريال، ومتوقع أن يكلفنا تشغيل الـ327 المتبقية Y ريال، والميزانية الداخلية المتبقية Z ريال.»

وهذا هو المستوى الصحيح لإدارة اقتصاد Gemini في Mujeeb 24.