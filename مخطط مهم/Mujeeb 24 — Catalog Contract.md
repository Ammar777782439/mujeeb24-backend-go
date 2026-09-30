

1. الفكرة العامة

الـCatalog في Mujeeb 24 هو النظام الذي يمثل ما يقدمه التاجر للبيع أو التقديم.

البنية الأساسية:

Business
   │
   └── Catalog
         │
         └── CatalogItem
               │
               ├── AttributeSchema
               │      └── AttributeDefinition
               │
               ├── Variant
               │
               └── Offer

هناك ستة كيانات رئيسية:

1. Catalog
2. CatalogItem
3. AttributeSchema
4. AttributeDefinition
5. Variant
6. Offer

---

2. Catalog

يمثل كتالوجًا تابعًا لتاجر معين.

يمكن للتاجر أن يمتلك أكثر من Catalog.

الحقول

الحقل| النوع| إلزامي| المعنى
"id"| UUID| نعم| معرف الكتالوج
"business_id"| UUID| نعم| التاجر/Business صاحب الكتالوج
"name"| TEXT| نعم| اسم الكتالوج
"description"| TEXT| لا| وصف الكتالوج
"status"| TEXT| نعم| حالة الكتالوج
"created_at"| TIMESTAMPTZ| نعم| تاريخ الإنشاء
"updated_at"| TIMESTAMPTZ| نعم| تاريخ آخر تحديث

حالات Catalog

draft
active
archived

قواعد Catalog

"name" لا يمكن أن تكون فارغة أو تحتوي فقط على مسافات.

الـCatalog مرتبط مباشرة بـBusiness.

حذف Business ممنوع إذا كان لديه Catalog مرتبط به، لأن العلاقة تستخدم "ON DELETE RESTRICT".

العلاقة

Business
   │
   └── Catalog

---

3. CatalogItem

هذا هو المنتج/الخدمة/العنصر التجاري الأساسي داخل الكتالوج.

CatalogItem هو الكيان الذي يصف الشيء الذي يقدمه التاجر، بينما Offer يصف كيفية تقديمه تجاريًا.

الحقول

الحقل| النوع| إلزامي| المعنى
"id"| UUID| نعم| معرف العنصر
"business_id"| UUID| نعم| التاجر صاحب العنصر
"catalog_id"| UUID| نعم| الكتالوج الذي ينتمي إليه العنصر
"attribute_schema_id"| UUID| لا| الـAttribute Schema المرتبط
"attribute_schema_version"| INTEGER| لا| إصدار الـSchema المستخدم
"item_type"| TEXT| نعم| نوع العنصر
"name"| TEXT| نعم| اسم العنصر
"short_description"| TEXT| لا| وصف مختصر
"long_description"| TEXT| لا| وصف تفصيلي
"status"| TEXT| نعم| حالة العنصر
"pricing_mode"| TEXT| نعم| طريقة التسعير
"availability_mode"| TEXT| نعم| طريقة تحديد التوفر
"fulfillment_mode"| TEXT| نعم| طريقة التنفيذ/التسليم
"requires_confirmation"| BOOLEAN| نعم| هل يحتاج العنصر إلى تأكيد
"attributes"| JSONB Object| نعم| قيم الخصائص الخاصة بالعنصر
"created_at"| TIMESTAMPTZ| نعم| تاريخ الإنشاء
"updated_at"| TIMESTAMPTZ| نعم| تاريخ آخر تحديث

حالات CatalogItem

draft
active
inactive
archived

item_type

"item_type" ليس Enum في الـDDL الحالي.

المطلوب فقط أن يكون:

غير فارغ

أي أن النظام الحالي لا يفرض قائمة ثابتة مثل "product" أو "service".

---

Pricing Mode في CatalogItem

القيم المقبولة:

fixed
starting_from
per_unit
per_person
per_day
quote_required
dynamic

معنى القيم

fixed

سعر ثابت.

starting_from

السعر يبدأ من قيمة معينة.

per_unit

السعر محسوب لكل وحدة.

per_person

السعر لكل شخص.

per_day

السعر لكل يوم.

quote_required

لا يوجد سعر نهائي ثابت داخل العرض؛ يلزم طلب تسعير.

dynamic

السعر ديناميكي.

---

Availability Mode في CatalogItem

القيم:

stock
schedule
supplier_check
always_available
unknown

معنى القيم

stock

التوفر مرتبط بالمخزون.

schedule

التوفر مرتبط بجدول أو مواعيد.

supplier_check

يجب التحقق من المورد.

always_available

العنصر يعتبر متاحًا دائمًا وفق هذا الـMode.

unknown

طريقة تحديد التوفر غير معروفة.

---

Fulfillment Mode في CatalogItem

القيم:

delivery
pickup
digital
appointment
travel
manual

المعاني

delivery

توصيل.

pickup

استلام.

digital

تسليم رقمي.

appointment

موعد.

travel

خدمة/تنفيذ مرتبط بالسفر أو الرحلات.

manual

تنفيذ يدوي.

---

4. CatalogItem — Attributes

الحقل:

attributes

هو JSONB ويجب أن يكون Object.

القيمة الافتراضية:

{}

أي أن العنصر يمكن أن يحتوي خصائص ديناميكية، مثل:

color
size
weight
material
brand
...

لكن قاعدة البيانات لا تسمح بأن تكون "attributes":

array
string
number
boolean

بل يجب أن تكون JSON Object.

---

5. CatalogItem — Attribute Schema

يمكن ربط CatalogItem بـ:

attribute_schema_id
attribute_schema_version

العلاقة اختيارية.

هناك قاعدة مهمة جدًا:

إما أن يكون الاثنان "NULL":

attribute_schema_id = NULL
attribute_schema_version = NULL

أو يجب أن يكون الاثنان موجودين:

attribute_schema_id ≠ NULL
attribute_schema_version ≠ NULL

والـversion يجب أن تكون أكبر من صفر.

هذا يمنع وجود:

Schema ID بدون Version

أو:

Version بدون Schema ID

---

6. AttributeSchema

يمثل تعريفًا منظمًا لمجموعة الخصائص التي يمكن أن يستخدمها CatalogItem.

الحقول

الحقل| النوع| إلزامي| المعنى
"id"| UUID| نعم| معرف الـSchema
"business_id"| UUID| نعم| التاجر صاحب الـSchema
"name"| TEXT| نعم| اسم الـSchema
"version"| INTEGER| نعم| إصدار الـSchema
"created_at"| TIMESTAMPTZ| نعم| تاريخ الإنشاء
"updated_at"| TIMESTAMPTZ| نعم| تاريخ التحديث

مهم

AttributeSchema ليس لديه status في الـDDL الحالي.

حالته لا تُدار بواسطة:

draft
active
archived

لأنه لا يوجد حقل "status" أصلًا.

الموجود هو:

version

والـversion يجب أن تكون أكبر من "0".

---

Versioning

يمكن لنفس Schema أن توجد بإصدارات مختلفة.

مثلاً:

Product Attributes
v1
v2
v3

والـUnique الحقيقي هو:

business_id + name + version

أي أن التاجر لا يستطيع امتلاك نفس اسم الـSchema ونفس الـversion مرتين.

---

7. AttributeDefinition

يمثل تعريف خاصية واحدة داخل AttributeSchema.

مثلاً إذا كان لدينا Schema للملابس:

Color
Size
Material

فكل واحدة منها AttributeDefinition مستقلة.

الحقول

الحقل| النوع| إلزامي| المعنى
"id"| UUID| نعم| معرف تعريف الخاصية
"schema_id"| UUID| نعم| الـSchema الأب
"attribute_key"| TEXT| نعم| المفتاح البرمجي
"label"| TEXT| نعم| الاسم المعروض
"data_type"| TEXT| نعم| نوع القيمة
"is_required"| BOOLEAN| نعم| هل الخاصية مطلوبة
"is_searchable"| BOOLEAN| نعم| هل يمكن استخدامها في البحث
"validation_rules"| JSONB Object| نعم| قواعد التحقق
"display_order"| INTEGER| نعم| ترتيب عرض الخاصية
"created_at"| TIMESTAMPTZ| نعم| تاريخ الإنشاء
"updated_at"| TIMESTAMPTZ| نعم| تاريخ التحديث

---

8. AttributeDefinition — Data Types

القيم المسموحة:

text
number
boolean
date
datetime
select
multi_select
location
money

المعاني

text

نص.

number

رقم.

boolean

قيمة true/false.

date

تاريخ.

datetime

تاريخ ووقت.

select

اختيار قيمة واحدة.

multi_select

اختيار عدة قيم.

location

موقع.

money

قيمة مالية.

---

9. validation_rules

"validation_rules" هي JSON Object.

الحقل لا يستطيع أن يكون:

array
string
number

بل يجب أن يكون Object.

يمكن أن يحتوي قواعد مثل:

enum values
min
max
regex
currency
timezone

هذه ليست قائمة Enum مغلقة داخل قاعدة البيانات؛ هي أنواع من قواعد التحقق التي يمكن تمثيلها داخل JSON.

---

10. AttributeDefinition Constraints

"attribute_key" يجب ألا تكون فارغة.

"label" يجب ألا تكون فارغة.

"display_order" يجب أن يكون:

>= 0

ولا يمكن أن يتكرر نفس:

schema_id + attribute_key

داخل نفس Schema.

---

11. Variant

Variant يمثل نسخة أو تنويعًا من CatalogItem.

مثلاً:

CatalogItem
└── قميص
      ├── أسود / XL
      ├── أسود / L
      └── أبيض / XL

الحقول

الحقل| النوع| إلزامي| المعنى
"id"| UUID| نعم| معرف الـVariant
"business_id"| UUID| نعم| التاجر
"catalog_item_id"| UUID| نعم| العنصر الأب
"name"| TEXT| نعم| اسم الـVariant
"attributes"| JSONB Object| نعم| خصائص الـVariant
"status"| TEXT| نعم| حالة الـVariant
"created_at"| TIMESTAMPTZ| نعم| تاريخ الإنشاء
"updated_at"| TIMESTAMPTZ| نعم| تاريخ التحديث

Variant.status

القيم:

active
inactive
archived

لا توجد حالة:

draft

لـVariant في الـDDL الذي أرسلته.

Variant.attributes

يجب أن تكون JSON Object.

---

12. Offer

Offer هو الجزء التجاري الفعلي الذي يحدد:

كم السعر؟
كيف يتم التسعير؟
هل متوفر؟
كيف يتم التنفيذ؟
متى يكون صالحًا؟
هل السعر متحقق؟

الحقول

الحقل| النوع| إلزامي| المعنى
"id"| UUID| نعم| معرف العرض
"business_id"| UUID| نعم| التاجر
"catalog_item_id"| UUID| نعم| العنصر المرتبط
"variant_id"| UUID| لا| Variant المرتبط، إن وجد
"name"| TEXT| نعم| اسم العرض
"pricing_mode"| TEXT| نعم| طريقة التسعير
"amount"| NUMERIC(20,4)| لا| قيمة السعر
"currency"| CHAR(3)| لا| العملة
"pricing_unit"| TEXT| لا| وحدة التسعير
"price_source"| TEXT| لا| مصدر السعر
"price_verification_status"| TEXT| نعم| حالة التحقق من السعر
"price_checked_at"| TIMESTAMPTZ| لا| وقت آخر تحقق للسعر
"availability_mode"| TEXT| نعم| طريقة تحديد التوفر
"availability_status"| TEXT| نعم| حالة التوفر
"availability_source"| TEXT| لا| مصدر معلومات التوفر
"availability_checked_at"| TIMESTAMPTZ| لا| وقت فحص التوفر
"availability_valid_until"| TIMESTAMPTZ| لا| نهاية صلاحية معلومة التوفر
"availability_evidence_ref"| TEXT| لا| مرجع دليل التوفر
"fulfillment_mode"| TEXT| نعم| طريقة التنفيذ
"validity_from"| TIMESTAMPTZ| لا| بداية صلاحية العرض
"validity_until"| TIMESTAMPTZ| لا| نهاية صلاحية العرض
"status"| TEXT| نعم| حالة العرض
"created_at"| TIMESTAMPTZ| نعم| تاريخ الإنشاء
"updated_at"| TIMESTAMPTZ| نعم| تاريخ التحديث

---

13. Offer — Pricing Mode

نفس القيم المستخدمة في CatalogItem:

fixed
starting_from
per_unit
per_person
per_day
quote_required
dynamic

---

14. Offer — Pricing Rules

fixed

يجب وجود:

amount
currency

starting_from

يجب وجود:

amount
currency

per_unit

يجب وجود:

amount
currency
pricing_unit

per_person

يجب وجود:

amount
currency
pricing_unit

per_day

يجب وجود:

amount
currency
pricing_unit

quote_required

لا يشترط وجود:

amount
currency
pricing_unit

dynamic

يمكن أن يكون:

amount = NULL

وإذا كان "amount" موجودًا، فيجب أن تكون "currency" موجودة.

---

15. Offer — Amount

السعر يستخدم:

NUMERIC(20,4)

ولا يمكن أن يكون أقل من صفر.

أي:

amount >= 0

السعر لا يمكن أن يكون رقمًا سالبًا.

---

16. Offer — Currency

"currency" اختيارية من ناحية الـNULL.

لكن إذا كانت موجودة فيجب أن تكون:

3 أحرف إنجليزية كبيرة

مثل:

YER
USD
SAR

والـDDL يفرض نمط:

^[A-Z]{3}$

---

17. pricing_unit

"pricing_unit" هو نص اختياري.

لا يوجد له Enum في الـDDL الحالي.

وهو يصبح مطلوبًا تحديدًا عندما يكون:

pricing_mode =
per_unit
per_person
per_day

---

18. price_source

مصدر السعر.

هو "TEXT" اختياري.

لا توجد قائمة Enum مفروضة عليه في الـDDL الحالي.

---

19. price_verification_status

حالة التحقق من السعر.

القيم المقبولة:

unverified
verified
stale
rejected

المعنى

unverified

السعر موجود لكن لم يتم التحقق منه.

verified

تم التحقق من السعر.

stale

السعر كان معروفًا لكنه أصبح قديمًا.

rejected

تم رفض السعر أو اعتباره غير صالح.

---

20. availability_mode في Offer

القيم:

stock
schedule
supplier_check
always_available
unknown

وهي نفس طرق تحديد التوفر الموجودة في CatalogItem.

---

21. availability_status في Offer

هذه ليست طريقة تحديد التوفر، بل نتيجة التحقق الحالية.

القيم:

available
unavailable
unknown
requires_check
stale

الفرق

availability_mode

يجيب:

«كيف نعرف التوفر؟»

بينما:

availability_status

يجيب:

«ما هي حالة التوفر الآن؟»

---

22. Availability Evidence

Offer يحتفظ بمعلومات التحقق:

availability_source
availability_checked_at
availability_valid_until
availability_evidence_ref

لذلك لا يعتمد التوفر فقط على كلمة "available".

هناك أيضًا:

من أين أتت المعلومة؟
متى تم فحصها؟
متى تنتهي صلاحيتها؟
ما مرجع الدليل؟

---

23. fulfillment_mode في Offer

القيم:

delivery
pickup
digital
appointment
travel
manual

---

24. Offer Validity

Offer يمكن أن يمتلك فترة صلاحية:

validity_from
validity_until

والقاعدة:

إذا كان كلاهما موجودًا:

validity_until >= validity_from

لا يمكن أن تنتهي صلاحية العرض قبل بدايته.

---

25. Offer Status

القيم:

draft
active
inactive
expired
archived

الفرق عن CatalogItem

CatalogItem:

draft
active
inactive
archived

Offer:

draft
active
inactive
expired
archived

أي أن Offer لديه:

expired

إضافة إلى الحالات الأخرى.

---

26. Availability Validity

معلومة التوفر يمكن أن تكون لها:

availability_checked_at
availability_valid_until

إذا كان الاثنان موجودين، فيجب أن تكون:

availability_valid_until >= availability_checked_at

---

27. العلاقات بين الكيانات

Business → Catalog

Business
   └── Catalog

كل Catalog تابع لـBusiness.

---

Catalog → CatalogItem

Catalog
   └── CatalogItem

كل CatalogItem يجب أن ينتمي إلى Catalog.

والـCatalog نفسه يجب أن يكون تابعًا لنفس Business.

أي أن العلاقة محمية بواسطة:

business_id + catalog_id

---

CatalogItem → AttributeSchema

CatalogItem
   └── AttributeSchema

العلاقة اختيارية.

إذا استخدم CatalogItem Schema، يجب أن يحتوي:

attribute_schema_id
attribute_schema_version

معًا.

---

AttributeSchema → AttributeDefinition

AttributeSchema
   └── AttributeDefinition
       ├── Attribute
       ├── Attribute
       └── Attribute

كل AttributeDefinition يتبع Schema واحدًا.

---

CatalogItem → Variant

CatalogItem
   ├── Variant
   ├── Variant
   └── Variant

Variant تابع مباشرة لـCatalogItem.

---

CatalogItem → Offer

CatalogItem
   ├── Offer
   ├── Offer
   └── Offer

Offer تابع لـCatalogItem.

---

Offer → Variant

Offer يستطيع أن يكون مرتبطًا بـVariant.

CatalogItem
   │
   ├── Variant A
   │
   └── Offer
         └── Variant A

إذا كان "variant_id" موجودًا، فيجب أن يكون الـVariant تابعًا لنفس CatalogItem ونفس Business.

---

28. Tenant Isolation

كل الكيانات الرئيسية تحمل "business_id" حيث يلزم:

Catalog
CatalogItem
Variant
Offer
AttributeSchema

والعلاقات تستخدم Business scope أيضًا في الأجزاء التي تحتاج ذلك.

الهدف أن لا يستطيع عنصر من Business أن يرتبط بعنصر من Business آخر.

---

29. الحذف

العلاقات تستخدم "ON DELETE RESTRICT".

وهذا يعني أن النظام يمنع حذف الكيان الأب إذا كانت هناك كيانات تابعة مرتبطة به.

الأمثلة:

Catalog
   ↓
CatalogItem

لا يمكن حذف Catalog إذا كان هناك CatalogItem مرتبط به.

وكذلك:

CatalogItem
   ↓
Variant
Offer

لا يمكن حذف CatalogItem بطريقة تكسر هذه العلاقات.

---

30. Unique Rules المهمة

Catalog

لكل Business:

business_id + id

Unique.

CatalogItem

لكل Business:

business_id + id

Unique.

AttributeSchema

لكل Business:

business_id + id

Unique.

والـSchema name/version:

business_id + name + version

Unique.

AttributeDefinition

داخل Schema:

schema_id + attribute_key

Unique.

Variant

هناك Unique على:

business_id + id

وUnique على:

business_id + catalog_item_id + id

Offer

هناك Unique على:

business_id + id

وUnique على:

business_id + catalog_item_id + id

---

31. الحالات النهائية للـCatalog

Catalog

draft
active
archived

CatalogItem

draft
active
inactive
archived

Variant

active
inactive
archived

Offer

draft
active
inactive
expired
archived

Price Verification

unverified
verified
stale
rejected

Availability Status

available
unavailable
unknown
requires_check
stale

---

32. القيم النهائية للـCatalog

Pricing Mode

fixed
starting_from
per_unit
per_person
per_day
quote_required
dynamic

Availability Mode

stock
schedule
supplier_check
always_available
unknown

Fulfillment Mode

delivery
pickup
digital
appointment
travel
manual

Attribute Data Type

text
number
boolean
date
datetime
select
multi_select
location
money

---

33. الأشياء التي ليست Enums

هذه الحقول في الـDDL الحالي هي TEXT ولا توجد لها قائمة قيم مغلقة:

item_type
pricing_unit
price_source
availability_source
availability_evidence_ref
attribute_key
label
catalog name
catalog description
item name
item descriptions
offer name
variant name

وبالتالي لا ينبغي إعطاء شخص آخر قائمة قيم مختلقة لها على أنها Enums رسمية.

---

34. الخلاصة المعمارية

Catalog
│
├── CatalogItem
│   ├── basic information
│   ├── item_type
│   ├── status
│   ├── pricing_mode
│   ├── availability_mode
│   ├── fulfillment_mode
│   ├── requires_confirmation
│   ├── attributes
│   └── optional AttributeSchema
│
├── AttributeSchema
│   └── AttributeDefinition
│       ├── attribute_key
│       ├── label
│       ├── data_type
│       ├── required
│       ├── searchable
│       ├── validation_rules
│       └── display_order
│
├── Variant
│   ├── name
│   ├── attributes
│   └── status
│
└── Offer
    ├── pricing
    │   ├── pricing_mode
    │   ├── amount
    │   ├── currency
    │   ├── pricing_unit
    │   ├── price_source
    │   └── price_verification_status
    │
    ├── availability
    │   ├── availability_mode
    │   ├── availability_status
    │   ├── availability_source
    │   ├── availability_checked_at
    │   ├── availability_valid_until
    │   └── availability_evidence_ref
    │
    ├── fulfillment_mode
    ├── validity_from
    ├── validity_until
    └── status

هذه هي بنية Catalog الكاملة وفق الـDDL الذي أعطيتني إياه، بما فيها الحقول، أنواعها، الحقول الاختيارية والإلزامية، العلاقات، الحالات، الـEnums، وقواعد التحقق الأساسية.