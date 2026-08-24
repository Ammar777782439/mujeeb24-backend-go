# PR-009 — Catalog Read Persistence

## الحكم التنفيذي

Commit المرفوع: `84ea2c6fd342762b82d5e5505fa57609e0415bd8`.

هذه الدفعة تنفذ **Catalog read persistence** المطلوبة حاليًا من Application queries، ولا تدّعي إغلاق Catalog Persistence بالكامل. تم تنفيذ القراءة على الجداول الموجودة `000013–000018` دون تعديل migrations قديمة أو إضافة جدول generic.

تظل عمليات الكتابة التالية مؤجلة عمدًا: `Create/UpdateCatalog`، `CreateAttributeSchemaVersion`، `Create/UpdateCatalogItem`، `Create/UpdateOffer`، و`Create/UpdateVariant`. سبب التأجيل موثق وليس نقصًا مخفيًا: عقود الأوامر موجودة، لكن لا توجد Application command services تنفيذية، كما أن schema لا تملك `resource_version` المطلوب لـ`If-Match`، وOffer command يستخدم `AmountMinor` بينما HTTP DTO/schema يستخدمان تمثيلًا عشريًا مختلفًا. لا يجوز بناء write repository يتجاوز هذه التناقضات.

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

## ما لم يُنفذ

لا توجد في هذه الدفعة write repositories ولا Application command services، ولا resource-version migration، ولا validation كاملة لـattributes مقابل schema definitions، ولا domain lifecycle validation لـactive Offer. هذه مسؤوليات Application/Domain، ولا يصح أن ينفذها SQL read adapter بالنيابة عنها.

لذلك لا ننتقل إلى Sales بعد هذا commit على أنه Catalog مكتمل بالكامل. الخطوة الصحيحة التالية داخل Catalog هي قرار مستقل لتصحيح write contract وresource-version/decimal-money boundary ثم تنفيذ command services وtransactional write repositories واختبارها.
