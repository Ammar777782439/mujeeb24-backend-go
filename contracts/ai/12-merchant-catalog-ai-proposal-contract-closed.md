# 12. Merchant Catalog AI — العقد الحالي للمقترح (Canonical Proposal V3)

> العقد الحالي على فرع `fix/merchant-catalog-strict-wire-v3` / PR #18. للتفاصيل المعمارية للمواصفات العربية والقوالب: [المرجع الموحّد](../../docs/architecture/merchant-catalog-attribute-schema-ai-contract-ar.md).

## 12.1 الغرض

هذا العقد يعرّف الـProposal الوسيط الذي ينتجه Merchant Catalog AI قبل دخول عملية التنفيذ.

الـProposal ليس Domain Entity Contract، ولا يمثل صفوف PostgreSQL مباشرة.

المسار:

Merchant → read-only discovery + Model → Strict Wire → catalogWireParse → Canonical Proposal V3 → Normalize/ValidateReferences/Validate → Merchant Review → Approve → ExecuteApproved transaction → Catalog Domain

## 12.1.1 عقد النقل المنفصل عن عقد الأعمال

**عقد Proposal V3 هو العقد التجاري النهائي في Go**، وليس غلاف الاتصال بالمزود ولا صفوف قاعدة البيانات. فرع Merchant Catalog الحالي يستخدم `merchantCatalogWireSchema()` في مرحلة الإخراج المنظم؛ ثم `catalogWireParse` و`parseMerchantCatalogProposal` يحولان الناتج إلى `merchantcatalogai.Proposal` ويجريان التحقق من البنية والمراجع والحقول.

- `submit_merchant_catalog_proposal` أداة الإخراج النهائي في تكامل DeepSeek؛ ليست أداة كتابة للكتالوج.
- `{name,value_json}` تمثيل محصور لنقل **قيمة حقل منفرد**، و`validation_rules_json` يخص قواعد **تعريف واحد**. لا ترسل المقترح كله كسلسلة JSON عامة.
- `operation` هو `create` أو `update` أو `delete` أو `ask_merchant` بحسب العقد. الحقول غير النشطة تكون فارغة؛ لا تُنشئ كيانات لم يطلبها التاجر.
- `PROPOSAL_STORED` يعني أن المقترح جاهز للمراجعة؛ التعديل الفعلي محصور في `ExecuteApproved` بعد اعتماد التاجر وصلاحياته.

## 12.1.2 تعريفات المواصفات والقوالب في Proposal

عندما يحتاج النموذج إضافة مواصفات مُدارة، يحمل المقترح:
- `attribute_definitions[]`: عناصر `{key,label,data_type,validation_rules}`. `key` اسم `snake_case` مؤقت، و`label` اسم عربي يكتبه النموذج، و`validation_rules` مثل `enum` لمجموعات القيم المقبولة.
- `attribute_schema_name`: الاسم العربي الذي يكتبه النموذج عندما يحتاج **قالبًا جديدًا**.
- `create.attribute_schema_id` أو `update.attribute_schema_id`: ID لقالب **موجود** من أدلة `merchant_catalog_list_attribute_schemas`، وليس من اختراع النموذج.
- `create.attributes` و`update.changes.attributes` و`Variant.attributes`: **JSON Objects** تحتوي المفاتيح الدلالية المؤقتة أو مفاتيح التعريفات الموجودة، مع قيم التاجر الفعلية. لا تصبح هذه مصفوفات `key/value` في Domain.

يتحقق Go من الأسماء العربية والأنواع والقواعد، ومن أن `select/multi_select` معهما `enum`، ومن صحة قيم العنصر والـVariant. عند اعتماد التاجر فقط، ينشئ Go `AttributeDefinition` ويولّد `attr_...` ويحفظ `attribute_schema_id` والإصدار والقيم داخل معاملة. **Go لا يترجم ولا يخترع مواصفات أو ألوانًا أو أسماء قوالب.**

القالب يحدد `enum` الممكنة؛ كل Variant يحتفظ بالقيمة المختارة فقط، وبـ`Offer` مستقل للسعر والتوفر. إعادة استخدام قالب موجود لا تبيح تعديل قواعده بصمت. المنتجات الحرة القديمة لا تُعرّب تلقائيًا.


## 12.2 الفصل بين Domain IDs وProposal References

هناك نوعان مختلفان من المراجع:

### Domain ID

معرّف حقيقي صادر من Mujeeb/PostgreSQL. مثال: variant_id. يُستخدم فقط عندما يكون الـVariant موجودًا بالفعل.

### Proposal Reference

معرّف محلي داخل Proposal لربط كيانات لم تُحفظ بعد. مثال: variant_ref.

هذا المرجع:
- لا يُحفظ في PostgreSQL.
- لا يمثل UUID.
- لا يجوز أن يكون ID من اختراع النموذج.
- يجب أن يكون فريدًا داخل Proposal.
- يستخدم لربط Offer بـVariant جديد قبل إنشاء الـVariant فعليًا.

## 12.3 Create Variant

كل VariantCreate داخل create أو update.new_variants يحتوي: ref, name, attributes.

ref مطلوب.

## 12.4 Create Offer

OfferCreate يدعم: variant_ref, variant_id, name, name_source, pricing fields, price_source, availability fields, fulfillment fields, status.

لكن الاستخدام يعتمد على العملية.

### Create

عند إنشاء CatalogItem جديد:
- variant_id ممنوع.
- العرض العام لا يحتوي variant_ref.
- العرض المرتبط بـVariant جديد يستخدم variant_ref.
- variant_ref يجب أن يطابق VariantCreate.ref داخل نفس Proposal.

### Update / New Offer

عند إنشاء Offer جديد أثناء تعديل CatalogItem موجود:
- Offer يستهدف Variant موجودًا → variant_id من Actual Catalog Evidence.
- Offer يستهدف Variant جديدًا في نفس Proposal → variant_ref.
- لا يجوز وجود variant_id وvariant_ref معًا.

## 12.5 Offer Name

Offer.name هو اسم تجاري للعرض، وليس اسم Variant.

إذا لم يذكر التاجر اسمًا تجاريًا منفصلًا للعرض، فإن الـProposal Contract يعرّف default canonical: سعر البيع.

لا يجوز اشتقاق اسم العرض من اسم Variant.

لذلك:

Variant.name = أحمر
Offer.name = سعر البيع
Offer.variant_ref = variant-red

وليس:

Offer.name = عرض اللون الأحمر

## 12.6 إنشاء العلاقة

في Create:

VariantCreate.ref → OfferCreate.variant_ref → Deterministic validation → Create Variant → PostgreSQL returns Variant.id → Resolve variant_ref → Variant.id → Create Offer.variant_id

هذه العملية لا تعتمد على Variant name matching أو fuzzy matching أو semantic matching أو النموذج-generated UUID أو database lookup بعد كل Variant بالاسم.

## 12.7 Offer Provenance

كل OfferCreate يحمل مصدرًا صريحًا للقيم التي لا يجوز للـAI اختراعها:

- name_source = system_default أو merchant_stated.
- price_source = merchant_stated أو not_stated.

### Offer Name

إذا لم يذكر التاجر اسمًا تجاريًا مستقلًا:
- name_source = system_default.
- name = سعر البيع بالضبط.
- لا يجوز إضافة اسم Variant أو اللون أو الخيار إلى الاسم.

إذا ذكر التاجر اسمًا تجاريًا مستقلًا:
- name_source = merchant_stated.
- يحفظ الاسم الذي ذكره التاجر.

### Offer Price

إذا ذكر التاجر سعرًا محددًا:
- price_source = merchant_stated.
- amount مطلوب ويحفظ بنفس القيمة.
- pricing_mode لا يجوز أن يكون quote_required.
- لا يجوز إسقاط السعر أو تحويله إلى null.

إذا لم يذكر التاجر سعرًا:
- price_source = not_stated.
- amount لا يُخترع.
- quote_required يمكن استخدامه عندما يكون السعر غير محدد.

هذه provenance fields جزء من Proposal Contract وليست تعليمات Prompt فقط.

## 12.7.1 قواعد التحقق

Proposal Validator يجب أن يرفض:
1. Variant بدون ref.
2. Variant refs مكررة داخل نفس Proposal.
3. Create Offer يحتوي variant_id.
4. Create Offer يحتوي variant_ref غير موجود.
5. New Offer يحتوي variant_id وvariant_ref معًا.
6. New Offer يستخدم variant_ref غير موجود في new_variants.

Reference Validation يجب أن يتحقق من variant_id ضد Actual Catalog Evidence عندما يكون الـVariant موجودًا أصلًا.

## 12.8 مسؤولية التنفيذ

النموذج لا يحوّل variant_ref إلى UUID.

Application/Execution layer هي التي:
1. تنشئ الـVariants.
2. تحفظ mapping مؤقتًا: variant_ref → variant_id.
3. تنشئ الـOffers بعد حل المراجع.
4. تكتب offers.variant_id بالـDomain ID الحقيقي.

## 12.9 الإصدار

Proposal schema_version = 3.

وأي Proposal بإصدار غير 3 لا يدخل مسار Merchant Catalog AI v2.

## 12.10 قاعدة المصدر

هذا العقد مصدر الحقيقة لـCanonical Proposal V3، أما تفاصيل النقل الحالية فهي في `merchant_catalog_wire.go` والدليل المعماري للربط العربي؛ لا تخلط بينهما.

أما تعريف Domain entities وحقول PostgreSQL والعلاقات الدائمة فيبقى في Catalog Entity Contract.

والـPrompt لا يعيد تعريف هذه القواعد؛ بل يعمل فوق هذا العقد.


## 12.11 Currency Context

العملة الافتراضية للتاجر ليست افتراضًا عالميًا. إذا كان التاجر يستخدم اسمًا عامًا مثل «ريال» دون تحديد الدولة، فإن Merchant Catalog AI يستخدم فقط `business.default_currency` الذي يمرره Mujeeb في runtime context. إذا لم يوجد default_currency رسمي فلا يخترع AI العملة.
