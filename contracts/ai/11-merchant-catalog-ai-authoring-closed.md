11. Merchant Catalog AI Authoring — العقد المعماري الحالي (Proposal V3)

> **العقد الحالي:** هذه الوثيقة محدّثة لمسار `Merchant Catalog AI` مع `Proposal V3` و`Strict Wire`. الاسم العربي للمواصفة وقواعدها من النموذج، والمفاتيح التقنية والتخزين بعد الاعتماد من Go. المرجع التفصيلي [هنا](../../docs/architecture/merchant-catalog-attribute-schema-ai-contract-ar.md).
>
11.1 الغرض

Merchant Catalog AI هو وكيل محادثي مستقل مخصص للتاجر، وهدفه الأساسي مساعدة التاجر على إضافة منتج إلى الكتالوج بكل تبعياته المطلوبة من خلال المحادثة.

الوكيل لا يهدف إلى تعليم التاجر بنية الكتالوج، ولا إلى شرح كيانات النظام، ولا إلى تحويل نفسه إلى واجهة إدارة تقنية.

المهمة الأساسية هي:

Merchant
  ↓
يطلب إضافة منتج
  ↓
Merchant Catalog AI
  ↓
يفهم الطلب
  ↓
يجمع البيانات الناقصة عبر الأسئلة
  ↓
يبني عملية إضافة المنتج كاملة
  ↓
Catalog Operation Proposal
  ↓
Mujeeb Validation
  ↓
Policy
  ↓
Authorization
  ↓
Execution

---

11.2 استقلال Merchant Catalog AI عن Customer Sales AI

يوجد وكيلان مستقلان:

Customer Sales AI
    ↓
Customer AI Runtime
    ↓
Catalog Read Boundary

و:

Merchant Catalog AI
    ↓
Merchant AI Runtime
    ↓
Catalog Authoring Boundary

يشتركان في البنية المشتركة فقط، مثل:

Catalog Contract
Catalog Domain
Application Services
Validation primitives
Authorization primitives
Audit
Observability
Model provider infrastructure

ولا يشتركان في:

System Prompt
Agent Role
Tool Permissions
Conversation Purpose
Proposal Contract
Execution Workflow

---

11.3 المهمة الأساسية للوكيل

المثال الأساسي:

التاجر:
أريد إضافة آيفون 15 الأسود 128GB.

الوكيل لا يشرح للتاجر ماهو CatalogItem أو Variant.

بل يحدد البيانات التي يحتاجها لإنجاز العملية، ثم يسأل:

ما السعر؟
ما العملة؟
هل المنتج متوفر؟
هل توجد نسخ أخرى؟

ويستمر في جمع البيانات حتى تصبح عملية الإضافة قابلة للبناء والتنفيذ وفق العقد والسياسات.

---

11.4 المنتج الكامل وليس العنصر المنفرد فقط

إنشاء المنتج قد يتضمن عدة أجزاء مترابطة حسب ما يقدمه التاجر وحسب قواعد Catalog Domain:

CatalogItem
├── Attributes
├── Variant(s)
└── Offer(s)

وعندما تتطلب العملية ذلك، يمكن أن تتضمن أيضًا معلومات مرتبطة بـ:

AttributeSchema
AttributeDefinition

وفق Catalog Contract والقواعد الموجودة في Mujeeb.

Merchant Catalog AI لا يخترع هذه التبعيات من نفسه.

إذا كانت البيانات المطلوبة غير موجودة، يسأل التاجر عنها أو يتوقف حتى تتوفر.

---

11.5 Create ليس مجرد Item Draft

الهدف ليس إنتاج:

ItemName
ItemType
Attributes

فقط.

بل بناء عملية إنشاء Catalog كاملة تمثل طلب التاجر.

مثال:

Create Product
├── CatalogItem
│   ├── name
│   ├── item_type
│   ├── descriptions
│   ├── pricing_mode
│   ├── availability_mode
│   ├── fulfillment_mode
│   └── attributes
│
├── Variants
│   ├── Black / 128GB
│   └── Blue / 256GB
│
└── Offers
    ├── Offer for Black / 128GB
    └── Offer for Blue / 256GB

لا يشترط أن تحتوي كل عملية على كل هذه الأجزاء؛ وجودها يعتمد على طلب التاجر وقواعد المجال.

---

11.6 الفصل بين الفهم والعملية

Merchant Catalog AI يقوم بمرحلتين منطقيتين:

أولًا: فهم المحادثة

مثل:

إضافة منتج
تعديل منتج
حذف منتج
استكمال بيانات
تأكيد بيانات
تصحيح بيانات

ثانيًا: بناء Operation Proposal

عمليات التعديل الفعلية تكون:

create
update
delete

ولا يتم اعتبار سؤال التاجر أو طلب التوضيح عملية كتابة.

مثال:

ask merchant

ليس Mutation.

بل هو جزء من المحادثة للوصول إلى Proposal صحيح.

---

11.7 النموذج لا ينفذ

النموذج مسؤول عن:

فهم كلام التاجر
استخراج البيانات
اكتشاف البيانات الناقصة
طرح الأسئلة
تنظيم المعلومات
بناء Proposal

النموذج ليس مسؤولًا عن:

Database writes
Authorization
Tenant selection
Policy decision
Final validation
Transaction execution

---

11.8 حدود التنفيذ

المسار الإجباري هو:

Merchant
   ↓
Merchant Catalog AI
   ↓
Catalog Operation Proposal
   ↓
Structural Validation
   ↓
Reference Validation
   ↓
Tenant / Ownership Validation
   ↓
Policy Evaluation
   ↓
Authorization
   ↓
Confirmation where required
   ↓
Catalog Application Service
   ↓
Catalog Domain

النموذج لا يتجاوز أي طبقة من هذه الطبقات.

---

11.9 قراءة بيانات الكتالوج

قبل بناء عملية جديدة قد يحتاج الوكيل إلى معرفة الموجود حاليًا.

لذلك يمكن أن يحصل على بيانات قراءة/اكتشاف محدودة من Mujeeb.

أمثلة:

قراءة الكتالوج
قراءة العناصر
قراءة النسخ
قراءة العروض
قراءة Attribute Schemas
قراءة Attribute Definitions

هذه القدرات:

Read-only
Tenant-scoped
Mujeeb-owned

ولا تتحول إلى محرك بحث دلالي.

لا يوجد:

search_catalog(query)

ولا Semantic Search Engine داخل Merchant Catalog AI.

---

11.10 البيانات التي تصل إلى النموذج

النموذج يحتاج إلى السياق اللازم للعمل، ويشمل على الأقل:

Catalog Entity Contract
Current Catalog Evidence
Business Context
Conversation Context
Conversation State
Current Merchant Message

لكن:

PostgreSQL = Source of Truth
Mujeeb = Owner of Evidence and Scope
النموذج = Reasoning Layer

النموذج لا يختار "business_id" أو نطاق التاجر بنفسه.

---

11.11 المواصفات العربية والقوالب وقواعدها

المصدر المرجعي لتفاصيل التخزين والتوليد والفحص: [عقد AI وقوالب المواصفات العربية](../../docs/architecture/merchant-catalog-attribute-schema-ai-contract-ar.md).

**المفاهيم:**
- `AttributeSchema` قالب قابل لإعادة الاستخدام؛ و`AttributeDefinition` تعريف مستقل بــ`attribute_key` و`label` و`data_type` و`validation_rules`.
- `CatalogItem` يحمل `attribute_schema_id` و`attribute_schema_version`، ويحفظ قيم المواصفات في `attributes` كـJSON Object.
- `Variant` يحتفظ بقيمه في `Variant.attributes`، مستعينًا بقالب العنصر الأب. وقد يحتوي فقط جزءًا من تعريفات القالب.
- `Offer` يملك السعر والتوفر؛ لا نخلطها بالمواصفات.

**المعنى واللغة:** النموذج وحده يستخرج `label` العربي والقيمة ونوعها `data_type` وقواعدها `validation_rules` بناء على طلب التاجر أو أدلة الكتالوج. لا يكتب Go اسمًا عربيًا أو قائمة ألوان من عنده ولا تترجم React أسماء المفاتيح.

**اختيار القالب:** يستعلم النموذج من `merchant_catalog_list_attribute_schemas` عن القوالب الموجودة ويختار ID موثوقًا إن كان مناسبًا. إذا احتاج قالبًا جديدًا يقدم `attribute_schema_name` عربيًا و`attribute_definitions` لكل مواصفة جديدة. المفاتيح `snake_case` في مقترح AI مؤقتة، وGo يولّد `attr_...` للتعريفات الجديدة أو يعيد استخدام المفاتيح المعروفة.

**قواعد الاختيار:** يدعم `validation_rules` مثل `enum` و`min/max` و`regex`، مع ضرورة `enum` لـ`select/multi_select` في مسار AI. `enum` تحدد القيم المسموحة في القالب ولا تنشئ Variants تلقائيًا. القيم المقترحة للـVariant تخضع لـ`ValidatePartialAttributes`. ممنوع اختلاق خيارات أو تعديل قواعد تعريف موجود بصمت.

**دون تنفيذ مسبق:** أي إنشاء/تحديث لتعريفات القالب وروابط المنتج وقيم الخيار لا يحدث أثناء قراءة النموذج أو عند `PROPOSAL_STORED`؛ بل بعد الاعتماد الصريح داخل `ExecuteApproved` مع صلاحيات التاجر والتحقق والمعاملة.

**الحالات التاريخية:** `AttributeSchema` اختياري في الدومين؛ قد توجد منتجات/Variants بمفاتيح حرة مثل `color`. لا يغيّر هذا المسار بياناتها ولا يترجمها تلقائيًا.

---

11.12 Variants

الـVariant جزء من إنشاء المنتج أو تعديله عند الحاجة، ويحمل `id` و`business_id` و`catalog_item_id` و`name` و`attributes` و`status` وفق Catalog Contract.

`Variant.attributes` و`CatalogItem.attributes` كائنات JSON وليست مصفوفات `attribute_key + value`. عند استخدام قالب رسمي تكون مفاتيحهما مُعرّفات `attr_...` المقابلة لتعريفات `attribute_definitions`، والقيم هي القيم التي قدّمها التاجر. التمثيل المؤقت للـAI ليس قاعدة البيانات.

---
 
11.13 Offers

Offer جزء من عملية Authoring وليس عنصرًا اختياريًا في تصميم الوكيل نفسه.

عند الحاجة، يشمل العرض معلومات مثل:

pricing_mode
amount
currency
pricing_unit
price_source
price_verification_status

availability_mode
availability_status
availability_source
availability_checked_at
availability_valid_until
availability_evidence_ref

fulfillment_mode

validity_from
validity_until

status

إذا لم يقدم التاجر معلومات تجارية مطلوبة، لا يخترعها النموذج.

يطلبها من التاجر أو يترك العملية في حالة غير مكتملة وفق قواعد المجال.

---

11.14 Create / Update / Delete

Create

يبني Proposal لإنشاء المنتج بكل أجزائه المطلوبة.

Update

يحدد الكيان المستهدف والتغييرات المطلوبة، ثم يخضع للـ Validation والـ Policy.

Delete

يتطلب تأكيدًا صريحًا من التاجر افتراضيًا قبل التنفيذ.

مثال:

Merchant
  ↓
احذف هذا المنتج
  ↓
AI يفهم الطلب
  ↓
Delete Proposal
  ↓
Mujeeb Validation
  ↓
Confirmation
  ↓
Merchant confirms
  ↓
Authorization
  ↓
Delete

---

11.15 العمليات المركبة

إذا طلب التاجر إنشاء منتج مع عدة Variants وOffers، فلا ينفذ النموذج سلسلة عمليات مستقلة غير منضبطة.

بدلًا من ذلك:

Merchant Request
      ↓
One Authoring Proposal
      ↓
Validation
      ↓
One Application Use Case
      ↓
Domain Transaction where applicable

الهدف هو أن يتولى Mujeeb العملية المركبة كوحدة تطبيقية منضبطة، وليس أن يصبح النموذج هو منسق الكتابات في قاعدة البيانات.

---

11.16 Tool Calling

يمكن لـ Merchant Catalog AI استخدام أدوات قراءة واكتشاف عند الحاجة:

النموذج
   ↓
Read/Discovery Tool
   ↓
Mujeeb
   ↓
Evidence
   ↓
النموذج

لكن النسخة الأولى لا تجعل الكتابة المباشرة أدوات النموذج.

الكتابة تمر عبر:

Proposal
→ Validation
→ Policy
→ Authorization
→ Application Service

---

11.17 Anti-Hallucination Boundary

منع الهلوسة لا يعتمد على:

confidence
temperature
prompt فقط

بل على:

Catalog Contract
+
Actual Evidence
+
Structured Output
+
Deterministic Validation
+
Reference Validation
+
Ownership Validation
+
Policy
+
Authorization

---

11.18 الممنوعات

Merchant Catalog AI لا يقوم بـ:

SQL
Direct DB Write
Direct DB Update
Direct DB Delete
Authorization Decision
Tenant Selection
Inventing Prices
Inventing Availability
Inventing Merchant Data
Semantic Search
search_catalog(query)
Customer Sales Operations
Provider API Execution

---

11.19 النتيجة النهائية

الوكيل هو:

Merchant Catalog AI
=
Conversational Product Authoring Agent

وليس:

Catalog Documentation Bot
Catalog Structure Explainer
Search Engine
Database Agent

ومهمته الأساسية:

"ساعد التاجر على إضافة منتج إلى الكتالوج
بكل بياناته وتبعياته المطلوبة،
واسأل التاجر عن أي شيء ناقص،
ثم سلّم Mujeeb Proposal صالحًا للتحقق والتنفيذ."

---

11.20 حالة العقد الحالي

هذه وثيقة Merchant Catalog AI Authoring المرجعية للميزة على فرع `fix/merchant-catalog-strict-wire-v3` (PR #18). عقد الأعمال `Proposal V3` ونقل Strict Wire يردان في [عقد المقترح](./12-merchant-catalog-ai-proposal-contract-closed.md) وفي [المرجع المعماري للقوالب](../../docs/architecture/merchant-catalog-attribute-schema-ai-contract-ar.md).

الاستكشاف أدوات قراءة فقط؛ المقترح مراجعة فقط؛ تغييرات القالب/العنصر/الخيار/العرض لا تُنفذ إلا بعد موافقة التاجر. يُحافظ على Catalog Domain كما هو. وصف الفرع لا يعني الدمج أو النشر.
