# العقد الهندسي الحالي — مساعد كتالوج التاجر وقوالب المواصفات العربية

**المصدر المرجعي الحالي للميزة على الفرع `fix/merchant-catalog-strict-wire-v3` (PR #18).** يصف هذا المستند المسار المنفّذ في كود الفرع، ولا يعني أن هذا الكود نُشر في `main` أو الإنتاج. لا توجد ملاحق سابقة أو مساران متوازيان للنقل. المرجع التجاري الأوسع: [Catalog Contract](../../مخطط%20مهم/Mujeeb%2024%20%E2%80%94%20Catalog%20Contract.md)، و[Merchant Catalog AI Authoring](../../contracts/ai/11-merchant-catalog-ai-authoring-closed.md)، و[Proposal V3](../../contracts/ai/12-merchant-catalog-ai-proposal-contract-closed.md).

## 1. المسؤوليات

| المكوّن | المسؤولية |
| --- | --- |
| التاجر | حقيقة المنتج والسعر والمواصفات، ومراجعة المقترح واعتماده |
| نموذج AI | يفهم طلب التاجر بالعربية، ويستخرج قيمة المواصفة و`label` العربي و`data_type` و`validation_rules` الموثوقة، ويختار قالبًا مناسبًا من نتائج القراءة، أو يؤلف اسم قالب عربي جديد عند الحاجة |
| Go Gateway | أدوات القراءة، Strict Wire، فحص الـJSON والحقول والمراجع، وتحويل النقل إلى Canonical Proposal V3 |
| Go Executor | لا ينشئ أي سجل من مجرد رسالة أو Proposal؛ عند الموافقة يفحص التزامن والصلاحيات، ويطابق تعريفات القالب ويولّد `attr_...` للتعريف الجديد ويحفظ في معاملة |
| PostgreSQL | القوالب والتعريفات وإصداراتها والقيم الحقيقية للمنتج والخيار |
| React | تعرض `label` المحفوظ وتستخدم `data_type` و`validation_rules` لإنشاء محرّر مناسب. **لا تترجم أسماء المفاتيح** |

Go **لا يترجم** `battery` إلى «البطارية»، ولا يؤلف اسم قالب أو خيارات `enum` أو يستنتج نية تجارية بقواعد كلمات ثابتة. والنموذج لا ينشئ `attr_...` ولا يختلق UUID لقالب موجود.

## 2. نموذج التخزين والعلاقة الفعلية

```text
attribute_schemas
  id, business_id, name, version
    └── attribute_definitions
          schema_id, attribute_key, label, data_type, validation_rules, ...

catalog_items
  id, attribute_schema_id, attribute_schema_version, attributes JSONB
    └── variants
          catalog_item_id, attributes JSONB

offers
  price, currency, availability ... (not attributes)
```

- `CatalogItem` يرتبط بقالب وإصدار محددين؛ الرابط **اختياري في Catalog Domain الأساسي**، ولهذا قد توجد منتجات قديمة تحمل مفاتيح حرة.
- `Variant` يستخدم تعريفات قالب المنتج الأب، ولا يحمل `attribute_schema_id` مستقلاً. لا يلزم أن يحتوي كل حقول القالب؛ يستعمل Go `ValidatePartialAttributes` للقيم المقدّمة.
- `attribute_definitions.attribute_key` هو المفتاح التقني؛ `label` اسم العرض العربي؛ `data_type` نوع القيمة؛ `validation_rules` القيود مثل `enum`.
- `CatalogItem.attributes` و`Variant.attributes` **JSON Objects** تحتوي `attribute_key → actual_value`، وليست صفوف تعريفات `{key,label,value}` أو مصفوفات `key/value`.
- السعر والعملة والتوفر من مسؤولية `Offer`، مثل `update.existing_offers` عند تحديث عرض موجود.
- تعريفات القوالب تُدار بإصدارات؛ لا تتبدل مفاتيح تعريف سابق ولا تُعدّل `enum` محفوظة بصمت.

### مثال متوافق مع البيانات المعروضة في pgAdmin

```json
{
  "attribute_definitions": [
    {
      "attribute_key": "attr_4b40819c06f01d6c",
      "label": "سعة التخزين",
      "data_type": "number",
      "validation_rules": {}
    },
    {
      "attribute_key": "attr_b93f45cdba1161f5",
      "label": "اللون",
      "data_type": "multi_select",
      "validation_rules": {"enum": ["أسود", "أزرق", "أبيض", "رمادي"]}
    }
  ],
  "variant_attributes": {
    "attr_4b40819c06f01d6c": 128,
    "attr_b93f45cdba1161f5": ["أزرق"]
  }
}
```

**التعريفات والقيم موضوعة هنا في مثال واحد للشرح فقط**؛ في PostgreSQL التعريفات صفوف في جدولها، وقيم الخيار JSONB في `variants`. React تطابق المفتاح فتظهر «سعة التخزين: 128» و«اللون: أزرق». القالب يحدد الألوان *المسموحة* لكنه لا ينشئ Variants تلقائيًا.

## 3. دورة AI كاملة

1. التاجر يصف المنتج أو الخدمة أو التعديل.
2. النموذج يستخدم أدوات القراءة الفعلية ومنها `merchant_catalog_list_attribute_schemas` عندما يحتاج البحث عن قالب، وأدوات العناصر والخيارات والعروض حسب العملية.
3. إذا كان قالب موجود مناسبًا، يختار النموذج `attribute_schema_id` ظهر في نتائج القراءة الموثوقة ويستخدم مفاتيح تعريفاته الفعلية.
4. إذا لم يوجد قالب مناسب، يولّد النموذج **مؤقتًا** لكل مواصفة `key` دلاليًا بالإنجليزية `snake_case`، و`label` عربيًا، و`data_type`، و`validation_rules` مستندة إلى طلب التاجر/الأدلة، و`attribute_schema_name` عربيًا لإنشاء القالب اللازم.
5. Gateway يفحص المخرجات، ويحوّلها إلى `merchantcatalogai.Proposal` V3. `PROPOSAL_STORED` **للمراجعة، وليس للحفظ التجاري**.
6. عند الموافقة الصريحة فقط، يقوم `ExecuteApproved` بالتأكد من المراجع والصلاحيات ونسخ البيانات، ومطابقة/إنشاء تعريفات وإصدار قالب مناسب، وتحويل مفاتيح المواصفات الجديدة إلى `attr_...`، وربط `attribute_schema_id` والإصدار، وحفظ القيم داخل معاملة.
7. React تعرض التسميات والقواعد المخزنة من تعريفات القالب، دون ترجمة.

**اختلاف الصياغة التجارية لا يجيز لـGo الدمج الدلالي لتسميتين**. إذا وجد تعريف بمعنى أو قواعد مختلفة لا يستبدلها بصمت، ولا يخترع حقائق ناقصة.

## 4. عقد النقل الحالي مقابل عقد الأعمال

- عقد النقل لمساعد التاجر هو `merchantCatalogWireSchema()`، مع أداة `submit_merchant_catalog_proposal` في تكامل DeepSeek. يقيّد create/update/delete/ask_merchant وحقولًا معروفة.
- `{name,value_json}` وسيط لنقل **قيمة حقل واحد** (قد تكون `attributes` JSON Object). و`attribute_definitions[].validation_rules_json` سلسلة JSON لقواعد **تعريف واحد**؛ ليست JSON للمقترح كاملًا.
- `catalogWireDefinitions` يفك القواعد، و`catalogWireParse` يحول الـWire إلى `Proposal V3`، ثم تطبَّق مراحل الفحص والتطبيع وفحص المراجع.
- الشكل النهائي في `Proposal V3`: `attribute_definitions` فيها `key,label,data_type,validation_rules`، ويظل `create.attributes` و`update.changes.attributes` و`variants[].attributes` JSON Objects.
- `create.attribute_schema_id` أو `update.attribute_schema_id` يختار قالبًا **موجودًا**. و`attribute_schema_name` يحدد اسم قالب **جديد** من فهم النموذج.
- مجرد نجاح Strict Schema لا يضمن صحة المعنى أو الادعاءات التجارية. تبقى الحماية النهائية بمراجعة التاجر والتحقق في Go.
- لا تحمل المقترح التجاري كاملًا في حقل نصي مفتوح؛ النقل المعتمد هنا هو هيكل Wire مغلق مع JSON لقيم الحقول والقواعد الفردية فقط.

## 5. قواعد التحقق `validation_rules`

النموذج ينتج القيم المسموحة والقواعد مستندًا إلى التاجر أو الأدلة، وGo **يتحقق منها ولا يخترعها**.

| القاعدة | السلوك |
| --- | --- |
| `enum` | قائمة القيم المسموحة، لا تعني أن المنتج يمتلك كل القيم |
| `select` | يحتاج `enum`، والقيمة المختارة مفردة حسب النوع |
| `multi_select` | يحتاج `enum`، وقيمة المنتج/الخيار قائمة من القيم المسموحة |
| `min` / `max` | حدود رقمية تتحقق منها الخدمة |
| `regex` | صيغة النص عند استخدام هذه القاعدة |
| `currency` / `timezone` | قيود مدعومة في المسار |

مسار AI الجديد يقبل فقط أسماء القواعد التي يستطيع Go التحقق منها؛ يحظر `enum` فارغة أو متكررة أو تحتوي كائنات غير مناسبة، ويحد عدد الاختيارات وحجم JSON. لا يبدّل قواعد تعريف محفوظ عند إعادة استخدام قالب موجود. القيم المقترحة على مستوى Variant تُفحص عبر `ValidatePartialAttributes`. قواعد مجال Catalog الأساسية تظل قابلة للتوسّع، لكن هذا لا يعني قبول قواعد ينتجها AI ولا يفسرها المدقق.

## 6. التوافق مع البيانات الحالية

- المنتجات التي تحتوي مفاتيح حرة مثل `color` و`storage` بلا قالب تُعرض كما هي حتى تُربط بتعريفات موثوقة. **لا يوجد ترحيل تلقائي أو تعريب جماعي**.
- بعض المنتجات قد تحتوي مفاتيح حرة حتى وهي مرتبطة بقالب؛ وجود القالب لا يعني أن جميع مفاتيحها مُدارة.
- لا تُترجم قيم التاجر مثل `5000mAh` و`Samsung` ولا تُغيّر أنواعها؛ تُعرض التسميات العربية للتعريفات فقط.
- تغيير إصدار قالب لا يغيّر روابط كل المنتجات إلى الإصدار الجديد تلقائيًا.
- لا تغييرات في DDL ولا migrations لهذا التكامل.

## 7. ملفات التنفيذ المعتمدة

- `internal/domain/ai/prompts/merchant_catalog_v2.go`: تعليمات النموذج.
- `internal/application/merchantcatalogai/proposal.go`: عقد Proposal V3.
- `internal/application/merchantcatalogai/discovery_attribute_schemas.go`, `discovery_references.go`: قراءة القوالب والتحقق من IDs.
- `internal/adapters/secondary/ai/gateway/merchant_catalog_wire.go`, `merchant_catalog_wire_rules.go`, `merchant_catalog_schema.go`, `native_merchant_catalog_adapter.go`: Strict Wire وتحويل القواعد.
- `internal/application/merchantcatalogai/attribute_definition_validation.go`, `attribute_schema_materialization.go`, `execution.go`: التحقق وإنشاء التعريفات عند اعتماد المقترح.
- `internal/application/catalogvalidation/attributes.go`: التحقق من أنواع القيم وقواعدها.
- `internal/application/ports/catalog_repository.go` و`internal/adapters/secondary/persistence/postgres/catalog_repository.go`: ربط القالب والإصدار وقيم الكتالوج.

## 8. حالات التحقق الوظيفي

- اختيار قالب موجود من مرجع حقيقي وإعادة استخدام مفاتيحه وتسمياته وقواعده.
- إنشاء قالب عربي وتعريفات جديدة بعد الموافقة فقط، مع مفاتيح `attr_...` من Go.
- إنشاء/تعديل قيم Variant بما يوافق القواعد، ورفض اختيار لون خارج `enum`.
- رفض بيانات ناقصة أو `schema_id` مختلق أو تغيير قواعد تعريف موجود بصمت.
- الحفاظ على مفاتيح وإصدارات القوالب الحالية، وذرية التعديلات في PostgreSQL.
- مراجعة عربية واضحة قبل الاعتماد، وفصل عرض/تعديل الأسعار في `Offers`.

**قاعدة ملزمة:** النموذج يفهم ويكتب `label/data_type/validation_rules/value` ويختار قالبًا؛ Go يتحقق ويولّد الهوية التقنية فقط ويحفظ بعد الموافقة؛ PostgreSQL يحفظ التعريفات والقيم بروابطها؛ React تعرض التسميات المخزنة دون ترجمة.
