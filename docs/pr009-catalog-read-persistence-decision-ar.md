# PR-009 — Catalog Read Persistence

## الحكم التنفيذي

Commit المرفوع موثق في تقرير الإغلاق وسجل Git؛ لا نكرر hash داخل محتوى commit حتى لا يصبح self-referential بعد amend.

هذه الدفعة تنفذ **Catalog read/write persistence** المطلوبة حاليًا من Application queries والـcommands، عبر الجداول الموجودة `000013–000018` وإضافة resource-version forward migration في `000029`. لم تُعدّل migrations قديمة، ولم يُنشأ جدول generic.

## Contract evidence

| المصدر | ما يثبته |
|---|---|
| `internal/application/queries/queries.go` | read surface الحالي: List/Get Catalog، List/Get CatalogItem، List Offers، List Variants، List/Get AttributeSchema؛ لا يوجد GetOffer أو GetVariant |
| `internal/application/commands/customers_catalog.go` | write commands معرفة typed، لكنها لا تثبت وجود Application implementation أو resource-version storage |
| Domain Catalog contracts | Catalog ملك Business، Item تابع لـCatalog، Variant اختيارية، Offer منفصل، attributes مرتبطة بـschema version، وmoney ليس float64 داخل Domain |
| HTTP Dashboard contract | filters وpagination للـCatalog، و`If-Match` للتعديلات الحساسة |
| migrations `000013–000018` | الجداول والقيود والفهارس الفعلية، بما فيها composite tenant FKs وJSONB object checks |

## Application port

أضيف `internal/application/ports/catalog_repository.go` بواجهة واحدة للقراءة:

| Method | Scope |
|---|---|
| `ListCatalogs` / `GetCatalog` | business-scoped catalogs مع status filter |
| `ListCatalogItems` / `GetCatalogItem` | catalog-scoped items مع search/status |
| `ListOffers` | item-scoped offers مع status |
| `ListVariants` | item-scoped variants مع status |
| `ListAttributeSchemas` / `GetAttributeSchema` | business-scoped schemas، name/version filters، وdefinitions مرتبة |

Records typed. `attributes` تعاد كـraw JSON bytes في port لأن الحقول المرنة معتمدة في schema، ولا تستخدم `map[string]any` كـrecord contract. Offer `amount` يحفظه port كسلسلة عشرية دقيقة، ولا يتحول إلى float داخل Application.

## PostgreSQL repository

`internal/adapters/secondary/persistence/postgres/catalog_repository.go` ينفذ الواجهة فوق `SQLExecutor`. كل query يمر عبر business scope، ويجري فحص وجود parent قبل List Items/Offers/Variants. أي parent غير موجود أو تابع لـBusiness آخر يعيد `RepositoryNotFound` typed ولا يعيد صفحة فارغة قد تخفي خطأ scope.

القوائم تستخدم keyset pagination opaque cursor بترتيب `(updated_at DESC, id DESC)`. AttributeSchema يستخدم `(version DESC, id DESC)`. حدود الصفحة الافتراضية 50 والحد الأقصى 100، وmalformed cursor يعيد `RepositoryInvalid`.

## Application mapping وHTTP

أضيفت services typed منفصلة لكل query حتى تطابق Go handler interface `Handle` دون overload مصطنع. تحوّل records إلى `CatalogView`, `CatalogItemView`, `OfferView`, `VariantView`, و`AttributeSchemaView`. أضيفت الحقول الآمنة الموجودة فعليًا في schema مثل description/timestamps/attributes/definitions/availability.

تم فصل `CatalogListInput` عن `BusinessListInput`، فأصبح filter `status` يصل من HTTP إلى `ListCatalogsQuery` ويظهر في OpenAPI المولد. كما أصبح version filter اختياريًا فعليًا بدل إرسال pointer بقيمة صفر.

HTTP DTO الحالي يمثل Offer amount كـ`*float64`؛ لذلك يبقى exact decimal محفوظًا في persistence/Application record، والتحويل إلى DTO يتم فقط عند boundary القائم. توحيد HTTP money contract إلى decimal-safe representation قرار مستقل ولم يُخفَ داخل PR-009.

## الاختبارات الفعلية

Integration test PostgreSQL 16 يغطي:

- insert/read لـCatalog وAttributeSchema/Definitions وCatalogItem وVariant وOffer؛
- status/search/version filters؛
- deterministic ordering وopaque keyset pagination؛
- raw JSON object preservation؛
- Offer decimal/currency/availability projection؛
- definitions ordering and mapping؛
- cross-tenant typed not-found للـCatalog والـItem والـOffer والـVariant والـSchema؛
- malformed cursor؛
- visibility داخل TransactionManager ثم commit وrollback؛
- Application query mapping.

## Catalog write contract and implementation

تم تنفيذ write path بعد تثبيت القرار، في migration forward-only `000029_catalog_resource_versions.up.sql`. أضيف `resource_version BIGINT NOT NULL DEFAULT 1` إلى `catalogs`, `catalog_items`, `offers`, و`variants` مع check موجب. لم تُعدّل `000001–000028`، ولا تُشتق version من `updated_at`.

يظل `ResourceVersion` في Application/HTTP opaque string، بينما يخزن PostgreSQL عدادًا موجبًا. كل Catalog PATCH ينفذ conditional update على `(business_id, id, resource_version)` ثم يزيد العداد ذريًا. عدم وجود المورد يعيد `not_found`، ووجوده مع version قديمة يعيد `stale_resource`، وكلاهما typed. لا يوجد delete تدميري.

بالنسبة للمال، يحافظ Application command الحالي على `AmountMinor *int64` كما يثبت helper HTTP أنه يقبل رقمًا بدقتين عشريتين ويضربه في 100. repository write يحفظ القيمة في `offers.amount NUMERIC(20,4)` بقسمة minor units على 100، ويعيد decimal text exact في record. هذا يبقي المال integer/decimal-safe داخل Application ولا يجعل `float64` جزءًا من Domain أو persistence contract. تغيير HTTP DTO نفسه إلى decimal string قرار مستقل ولم يُخفَ هنا.

أضيفت command services typed لكل عمليات Catalog الحالية: إنشاء/تحديث Catalog، إنشاء schema version مع definitions، إنشاء/تحديث CatalogItem، إنشاء/تحديث Offer، وإنشاء/تحديث Variant. schema version allocation محمي داخل transaction بقفل transaction-level على business/name، وschema مع definitions تُنشأ كوحدة ذرية. attributes تمر كـraw JSON bytes بعد JSON encoding في Application، ولا يستخدم record contract `map[string]any`.

## ما لم يُنفذ بعد

تم إغلاق Catalog Persistence من ناحية الـread/write surfaces الحالية، لكن **bootstrap dependency wiring** لم يُنفذ بعد؛ `cmd/api` ما زال يحتاج تركيب repository وTransactionManager وtyped command/query services داخل `handlers.Dependencies`. كما أن validation الدلالية الكاملة لـattributes مقابل definitions وprovider price verification تبقى مسؤولية Domain/Application لاحقة، وليست مسؤولية SQL adapter.
