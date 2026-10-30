# Merchant Catalog AI — تغطية العقود الحالية

**المصدر:** `fix/merchant-catalog-strict-wire-v3` / PR #18. هذا المستند يوجّه المطور إلى عقد الكود الموجود في الفرع، ولا يصف نشرًا في `main`.

مرجع التنفيذ والتخزين: [قوالب المواصفات العربية — Key/Label/Value](../architecture/merchant-catalog-attribute-schema-ai-contract-ar.md)، [Proposal V3](../../contracts/ai/12-merchant-catalog-ai-proposal-contract-closed.md)، [Merchant Authoring](../../contracts/ai/11-merchant-catalog-ai-authoring-closed.md).

## عقد النقل والتغطية

| السيناريو | ما يجب أن يفعله المسار الحالي |
| --- | --- |
| أدوات الاستكشاف | Read-only tools وIDs من أدلة الكتالوج؛ لا كتابة قبل المراجعة |
| الإخراج النهائي | `merchantCatalogWireSchema` ثم `catalogWireParse` و`Proposal V3`، وليس مقترحًا كاملًا داخل String |
| إنشاء عنصر | حقول `ItemCreate` كما يثبتها Catalog Contract؛ إنشاء Variants وOffers حسب طلب التاجر والقواعد |
| تعديل منتج | `update.item_id` الحقيقي، و`update.changes` للحقول المباشرة، وليس سعر Offer |
| إنشاء خيار | `variant_ref` فريد، و`attributes` قيم خاصة بالخيار، ثم حلّ المرجع عند الإنشاء |
| تعديل خيار موجود | `variant_id` موثوق، وقيمة معرّفة ضمن قالب العنصر أو مفتاح حر مسموح حسب السياسة |
| عرض موجود | `update.existing_offers` لتعديل السعر/التوفر، وليس `attributes` |
| القالب الموجود | النموذج يستكشف `merchant_catalog_list_attribute_schemas` ويستخدم ID فعليًا من الأدلة |
| تعريف جديد | النموذج ينتج `key,label,data_type,validation_rules`، واسم قالب عربي جديد عند الحاجة |
| القيمة والاسم العربي | Go يولّد/يعيد استخدام `attr_...` بعد الموافقة؛ React تعرض `definition.label` ولا تترجم |
| `select/multi_select` | النموذج يحدد `validation_rules.enum` من الأدلة، وGo يفحص القواعد والقيم |
| قائمة خيارات محفوظة | لا يجوز تغيير `enum` لتعريف موجود بصمت؛ لا ينشئ القالب Variants تلقائيًا |
| مواصفات العنصر أو الخيار | `CatalogItem.attributes` و`Variant.attributes` يبقيان JSON Objects مفاتيحهما التقنية وقيمهما الحقيقية |
| حذف/أرشفة | مراجعة الـProposal والتحقق من المراجع والسياسات قبل أي تنفيذ |
| نقص بيانات | يسأل التاجر، ولا يصنع أسعارًا أو مواصفات أو قيم enum |
| موافقة التاجر | `proposal_id` يذهب إلى `/businesses/{business_id}/merchant-ai/executions` ثم `ExecuteApproved`، مع الصلاحيات والنسخ والمعاملة |

## الاستنتاجات الملزمة

- `AttributeSchema` اختياري في Catalog Domain العام؛ يوجد بالفعل بيانات تاريخية بمفاتيح حرة مثل `color`. لا يُنفذ المسار الجديد ترجمة أو ترحيلًا جماعيًا لها.
- `AttributeDefinition` يحفظ `attribute_key` و`label` و`data_type` و`validation_rules`، بينما `variants.attributes[attribute_key]` و`catalog_items.attributes[attribute_key]` يحفظان قيم التاجر الحقيقية فقط.
- كل كتابة ناتجة عن AI مشروطة بالموافقة: `PROPOSAL_STORED` ليس حفظًا للعنصر.
- تغطية الاختبارات الآلية لا تعني أن مزودًا خارجيًا اجتاز تجربة حية أو أن واجهة React عالجت المنتجات القديمة.

**هذا هو مسار Merchant الموثق الحالي.** اعتمد العقود المرتبطة أعلاه؛ أي دليل منفصل يصف نقلًا أو قواعد مواصفات مختلفة ليس مصدرًا تنفيذيًا لهذا المسار.
