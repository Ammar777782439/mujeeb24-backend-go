

Mujeeb 24 — ⑤ Catalog Entity Contract + Actual Catalog Data Boundary

الحالة

CLOSED

هذه الوثيقة تحسم الفرق بين:

1. Catalog Entity Contract
2. Actual Catalog Data
3. Catalog AI Projection
4. Catalog Data Access Boundary

ولا يجوز دمج هذه الطبقات مع بعضها.

---

1. المبدأ الأساسي

Gemini يجب أن يعرف بنية الكتالوج كاملة قبل أن يتعامل مع بيانات الكتالوج.

لكن معرفة البنية لا تعني إعطاؤه بيانات التاجر.

لذلك لدينا مستويان منفصلان:

Catalog Entity Contract
        +
Actual Catalog Data

---

2. Catalog Entity Contract

هذا هو تعريف نظام Catalog نفسه.

وهو مبني مباشرة على Catalog Contract المعتمد في Mujeeb 24.

الكيانات الستة هي:

Catalog
CatalogItem
AttributeSchema
AttributeDefinition
Variant
Offer

Gemini يجب أن يعرف:

- معنى كل كيان.
- حقوله.
- نوع كل حقل.
- العلاقات بين الكيانات.
- القيم المغلقة حيث توجد Enums.
- القيود الخاصة بالحقول.
- معنى pricing modes.
- معنى availability modes.
- معنى fulfillment modes.
- معنى price verification.
- معنى availability status.
- معنى AttributeSchema.
- معنى AttributeDefinition.
- العلاقة بين CatalogItem وVariant وOffer.
- العلاقة بين CatalogItem وAttributeSchema.
- العلاقة بين AttributeSchema وAttributeDefinition.

---

3. Catalog Entity Contract ليس بيانات تاجر

مثال:

Gemini يعرف أن:

Offer

يحتوي على:

amount
currency
pricing_mode
availability_status
...

لكن هذا لا يعني أنه يعرف أن لدى التاجر عرضًا فعليًا بسعر:

12000 YER

الأول:

Entity Definition

والثاني:

Merchant Data

وهما شيئان مختلفان.

---

4. Actual Catalog Data

هذه هي البيانات الفعلية الموجودة في PostgreSQL الخاصة بالتاجر.

وتشمل البيانات الفعلية للكيانات:

Catalog
CatalogItem
AttributeSchema
AttributeDefinition
Variant
Offer

مثال:

CatalogItem
id = ...
name = "..."
status = "active"
attributes = {...}
ان وجدت كمان 
AttributeSchema
AttributeDefinition
Variant
Offer

هذه البيانات هي Merchant-Owned Truth.

Mujeeb هو مصدرها.

Gemini لا ينشئها ولا يعدلها.

---

5. Catalog AI Projection

الـProjection هو الشكل الذي يستخدمه Mujeeb لإرسال Actual Catalog Data إلى Gemini.

هو ليس Domain Entity جديدًا.

وهو ليس بديلًا عن Catalog Contract.

وهو ليس Search Index.

وظيفته فقط:

PostgreSQL Catalog Data
        ↓
AI Projection
        ↓
Gemini

والـProjection يحتوي على البيانات الفعلية المطلوبة للـAI، مع:

AttributeSchema
AttributeDefinition
CatalogItem
Variant
Offer

بحسب النطاق الذي يتم تقييمه.

---

6. العلاقة بين الطبقات الثلاث

                    Mujeeb 24
                         │
             ┌───────────┴───────────┐
             │                       │
             ▼                       ▼
Catalog Entity Contract       Actual Catalog Data
   "ما هي الكيانات؟"          "ما هي بيانات التاجر؟"
             │                       │
             │                       ▼
             │               Catalog AI Projection
             │                       │
             └───────────┬───────────┘
                         ▼
                      Gemini

Gemini يحتاج الاثنين:

Entity Contract
+
Actual Data

لكن كل واحد له وظيفة مختلفة.

---

7. ما الذي يراه Gemini من البداية؟

عند بدء مسار AI، Gemini يحصل على:

System Rules
+
Business Context
+
Conversation Context
+
Conversation State
+
Catalog Entity Contract
+
Current User Message

ولا يعني هذا أن Actual Catalog Data كلها ترسل في نفس اللحظة.

بيانات الكتالوج الفعلية تدخل حسب مسار التقييم الذي أغلقناه سابقًا.

---

8. لماذا نرسل Catalog Entity Contract من البداية؟

لأن Gemini يجب ألا يضطر إلى تخمين معنى البيانات.

مثال:

{
  "pricing_mode": "starting_from"
}

Gemini يعرف من Entity Contract أن:

starting_from
=
السعر يبدأ من قيمة معينة

وكذلك:

availability_mode
availability_status
price_verification_status
fulfillment_mode

ونفس الشيء بالنسبة إلى:

AttributeSchema
AttributeDefinition
Variant
Offer

---

9. لا نكرر Entity Contract مع كل Batch

Catalog Entity Contract هو تعريف مشترك.

لا نعتبر كل Batch نظام كتالوج جديدًا.

الشكل:

Conversation / AI Runtime
        │
        ├── Catalog Entity Contract
        │
        ├── Batch 1 → Actual Catalog Data
        ├── Batch 2 → Actual Catalog Data
        ├── Batch 3 → Actual Catalog Data
        └── Batch N → Actual Catalog Data

إذا احتاج تنفيذ Gemini إعادة إرسال العقد بسبب طريقة عمل الـAPI أو الـruntime، فهذا قرار نقل/تنفيذ فقط، وليس تغييرًا في الـDomain Contract.

---

10. Actual Catalog Data والـBatches

الـBatches تخص البيانات الفعلية.

وليس Entity Contract.

Catalog Entity Contract
        ↓
ثابت

Actual Catalog Data
        ↓
Token Counting
        ↓
Batching

وبالتالي حجم الكتالوج هو الذي يؤثر على:

tokens
batches
requests

وليس عدد تعريفات الكيانات.

---

11. مسؤولية Gemini

Gemini مسؤول عن:

فهم العميل
فهم Conversation Context
فهم Catalog Entity Contract
فهم Actual Catalog Data
المقارنة
الاستدلال
اختيار المعلومات المهمة
إنتاج Proposal

---

12. مسؤولية Mujeeb

Mujeeb مسؤول عن:

قراءة Catalog الحقيقي
Tenant Isolation
بناء AI Projection
Token-aware Batching
توفير البيانات الفعلية
التحقق من المخرجات
Policy
Authorization
Execution

Mujeeb لا يحتوي على:

semantic product matching
semantic search
product ranking
customer intent interpretation

---

13. Catalog Data Access Boundary

نحتاج Boundary واضح بين Gemini وبيانات الكتالوج الفعلية.

هذا Boundary:

Read Only
Tenant Scoped
Structured
No SQL

وظيفته:

«إعطاء Gemini بيانات كتالوج فعلية مصرح بها، وليس البحث له عن المنتجات.»

---

14. لا نستخدم Query Semantic

لا توجد:

query
search_query
semantic_query
keyword_search
vector_search

ولا توجد:

search_catalog(query)

نهائيًا.

---

15. لا يستطيع Gemini اختيار Business

Gemini لا يرسل:

business_id
tenant_id

Mujeeb يحدد Business من الـauthenticated context.

ثم:

Authenticated Business
        ↓
Tenant Scope
        ↓
Catalog Data

---

16. البيانات الفعلية ليست Raw PostgreSQL Rows

Mujeeb لا يرسل:

database row

كما هي.

بل:

PostgreSQL
   ↓
Catalog AI Projection
   ↓
Gemini

لأن الـProjection هي الطبقة التي تحدد ما يحتاجه الـAI وما لا يحتاجه.

---

17. Catalog Entity Contract لا يحتوي بيانات خاصة بالتاجر

لا نخلط بين:

Catalog Entity Contract

و:

Business Catalog Instance

مثلًا:

Entity Contract:
Offer.amount = NUMERIC(20,4)

بينما:

Actual Data:
amount = 12000
currency = YER

الأول تعريف للنظام.

الثاني بيانات التاجر.

---

18. العلاقة مع AttributeSchema

هناك مستويان أيضًا:

Entity Contract

Gemini يعرف أن:

AttributeSchema

هو تعريف منظم للخصائص.

ويعرف أن:

AttributeDefinition

يمثل خاصية واحدة.

Actual Catalog Data

Gemini قد يحصل على:

AttributeSchema:
Product Attributes
v1

ومعه:

AttributeDefinition:
color
size
material

ومعها:

CatalogItem.attributes

مثل:

{
  "color": "black",
  "size": "XL"
}

وبذلك يستطيع Gemini تفسير البيانات بدل رؤية JSON مجهول المعنى.

---

19. تدفق AI الكامل

Customer
   ↓
Message
   ↓
Mujeeb
   ↓
Context Builder
   │
   ├── System Rules
   ├── Business Context
   ├── Conversation Context
   ├── Conversation State
   └── Catalog Entity Contract
   ↓
Gemini
   ↓
Catalog Evaluation
   ↓
Actual Catalog Data
   ↓
AI Projection
   ↓
Token-aware Batching
   ↓
Gemini
   ↓
Candidate / Evidence
   ↓
Final Gemini Evaluation
   ↓
AI Proposal
   ↓
Mujeeb Validation
   ↓
Policy
   ↓
Authorization
   ↓
Execution / Send / Human

---

20. القرار النهائي

نعتمد نهائيًا:

Catalog Entity Contract
        +
Actual Catalog Data
        +
Catalog AI Projection

مع الفصل الكامل بينها.

Catalog Entity Contract

تعريف الكيانات الستة
والحقول
والأنواع
والعلاقات
والقيود
والـEnums
والمعاني

Actual Catalog Data

بيانات التاجر الحقيقية
من PostgreSQL

Catalog AI Projection

تمثيل Actual Catalog Data
المناسب لـGemini

---

21. ما تم إغلاقه

✅ Gemini يعرف Catalog Entity Contract كاملًا
✅ جميع الكيانات الستة داخلة في العقد
✅ AttributeSchema داخل العقد
✅ AttributeDefinition داخل العقد
✅ Variant داخل العقد
✅ Offer داخل العقد
✅ Actual Catalog Data منفصلة عن Entity Contract
✅ Catalog AI Projection منفصلة عن Domain Model
✅ بيانات التاجر تأتي من Mujeeb
✅ بيانات الكتالوج لا تعني Entity Contract
✅ Entity Contract لا يعتمد على حجم الكتالوج
✅ Batching يطبق على Actual Catalog Data
✅ لا Semantic Search
✅ لا search_catalog(query)
✅ لا Product Matching داخل Mujeeb
✅ لا SQL يصل إلى Gemini
✅ لا business_id يحدده Gemini
✅ Tenant Isolation مسؤولية Mujeeb
✅ Gemini = Reasoning
✅ Mujeeb = Data Boundary + Validation + Authorization + Execution

22. ملاحظة تصحيحية مهمة

==هذه الوثيقة تحل محل القرار السابق الذي جعل Gemini يرى Item IDs / Variant IDs / Offer IDs ثم يطلب التفاصيل باعتبار ذلك المسار الأساسي.==

القرار الصحيح الآن هو:

Gemini يعرف أولًا:
Catalog Entity Contract كامل

ثم يحصل على:
Actual Catalog Data

عبر:
Catalog AI Projection
+
Batching
+
Read Boundary عند الحاجة

⑤ Catalog Entity Contract + Actual Catalog Data Boundary = CLOSED