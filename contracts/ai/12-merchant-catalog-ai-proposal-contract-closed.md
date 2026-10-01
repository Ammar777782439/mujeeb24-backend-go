12. Merchant Catalog AI Proposal Contract — CLOSED v2

## 12.1 الغرض

هذا العقد يعرّف الـProposal الوسيط الذي ينتجه Merchant Catalog AI قبل دخول عملية التنفيذ.

الـProposal ليس Domain Entity Contract، ولا يمثل صفوف PostgreSQL مباشرة.

المسار:

Merchant → Gemini → Proposal Contract → Deterministic Validation → Reference Resolution → Policy → Authorization → Application Service → Catalog Domain

## 12.2 الفصل بين Domain IDs وProposal References

هناك نوعان مختلفان من المراجع:

### Domain ID

معرّف حقيقي صادر من Mujeeb/PostgreSQL. مثال: variant_id. يُستخدم فقط عندما يكون الـVariant موجودًا بالفعل.

### Proposal Reference

معرّف محلي داخل Proposal لربط كيانات لم تُحفظ بعد. مثال: variant_ref.

هذا المرجع:
- لا يُحفظ في PostgreSQL.
- لا يمثل UUID.
- لا يجوز أن يكون ID من اختراع Gemini.
- يجب أن يكون فريدًا داخل Proposal.
- يستخدم لربط Offer بـVariant جديد قبل إنشاء الـVariant فعليًا.

## 12.3 Create Variant

كل VariantCreate داخل create أو update.new_variants يحتوي: ref, name, attributes.

ref مطلوب.

## 12.4 Create Offer

OfferCreate يدعم: variant_ref, variant_id, name, pricing fields, availability fields, fulfillment fields, status.

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

هذه العملية لا تعتمد على Variant name matching أو fuzzy matching أو semantic matching أو Gemini-generated UUID أو database lookup بعد كل Variant بالاسم.

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

## 12.7 قواعد التحقق

Proposal Validator يجب أن يرفض:
1. Variant بدون ref.
2. Variant refs مكررة داخل نفس Proposal.
3. Create Offer يحتوي variant_id.
4. Create Offer يحتوي variant_ref غير موجود.
5. New Offer يحتوي variant_id وvariant_ref معًا.
6. New Offer يستخدم variant_ref غير موجود في new_variants.

Reference Validation يجب أن يتحقق من variant_id ضد Actual Catalog Evidence عندما يكون الـVariant موجودًا أصلًا.

## 12.8 مسؤولية التنفيذ

Gemini لا يحوّل variant_ref إلى UUID.

Application/Execution layer هي التي:
1. تنشئ الـVariants.
2. تحفظ mapping مؤقتًا: variant_ref → variant_id.
3. تنشئ الـOffers بعد حل المراجع.
4. تكتب offers.variant_id بالـDomain ID الحقيقي.

## 12.9 الإصدار

Proposal schema_version = 3.

وأي Proposal بإصدار غير 3 لا يدخل مسار Merchant Catalog AI v2.

## 12.10 قاعدة المصدر

هذا العقد هو مصدر الحقيقة لبنية Proposal الوسيط.

أما تعريف Domain entities وحقول PostgreSQL والعلاقات الدائمة فيبقى في Catalog Entity Contract.

والـPrompt لا يعيد تعريف هذه القواعد؛ بل يعمل فوق هذا العقد.
