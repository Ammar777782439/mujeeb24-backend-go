# PR-009 — تقرير Catalog Persistence النهائي

> **Truth Mode:** هذا التقرير يصف ما تحقق من الكود والاختبارات فعليًا، ويفصل بوضوح بين repository/application surface المثبت وبين runtime bootstrap أو provider behavior غير المنفذ.

## A. Verified Contract

يثبت `internal/application/queries/queries.go` أن سطح القراءة المطلوب حاليًا هو `List/GetCatalog`، و`List/GetCatalogItem`، و`ListOffers`، و`ListVariants`، و`List/GetAttributeSchema`. وتثبت أوامر `internal/application/commands/customers_catalog.go` سطح الكتابة الحالي: إنشاء/تحديث Catalog، إنشاء schema version، إنشاء/تحديث CatalogItem، إنشاء/تحديث Offer، وإنشاء/تحديث Variant.

جداول migrations `000013–000018` تثبت ملكية Business والعلاقات بين Catalog وItem وSchema وDefinition وOffer وVariant. عقد HTTP يثبت `If-Match` في PATCH، وعقد الأمر الحالي يثبت `AmountMinor *int64`؛ لذلك أضيفت resource versions في migration forward-only مستقلة، وحُفظت الأموال كـminor units في Application ثم كـNUMERIC major units في PostgreSQL.

## B. Why Required

Catalog هو مصدر المعرفة التجارية الذي ستستخدمه Mujeeb 24 لاحقًا في البحث والعروض والـAI والسياسات التجارية. لذلك لا يكفي وجود SQL tables؛ يجب أن يمر الوصول عبر Application typed ports، tenant scope، parent scope، keyset pagination، وoptimistic concurrency.

## C. Final Schema and rationale

تم الحفاظ على الجداول الأصلية، وأضيفت في `000029` أعمدة `resource_version BIGINT NOT NULL DEFAULT 1` إلى `catalogs`, `catalog_items`, `offers`, و`variants`، مع checks تمنع القيم غير الموجبة. العداد يخدم `If-Match` ويزيد atomically في conditional updates.

`attributes` و`validation_rules` تبقى JSONB، لكن Application record لا يستخدم `map[string]any`؛ attributes تعبر كـraw JSON bytes بعد JSON encoding. Offer amount يقرأ ويكتب بدقة عشرية، مع تحويل `AmountMinor / 100` داخل adapter. القوائم تستخدم `(updated_at DESC, id DESC)`، وAttributeSchema version allocation محمي بقفل transaction-level على Business/name.

## D. Migration number

الرقم الفعلي هو **000029**: `migrations/000029_catalog_resource_versions.up.sql`. لم تُعدّل migrations `000001–000028`. تم تحديث schema runner إلى `applied=29` في التشغيل الأول و`applied=0` في التشغيل الثاني، مع `schema_migrations=29`.

## E. Application port

الواجهة هي `internal/application/ports/catalog_repository.go`. وتشمل typed records وpages وdrafts وpatches، وmethods القراءة والكتابة التالية:

| Surface | Methods |
|---|---|
| Catalog | `ListCatalogs`, `GetCatalog`, `CreateCatalog`, `UpdateCatalog` |
| CatalogItem | `ListCatalogItems`, `GetCatalogItem`, `CreateCatalogItem`, `UpdateCatalogItem` |
| Offer | `ListOffers`, `CreateOffer`, `UpdateOffer` |
| Variant | `ListVariants`, `CreateVariant`, `UpdateVariant` |
| AttributeSchema | `ListAttributeSchemas`, `GetAttributeSchema`, `NextAttributeSchemaVersion`, `CreateAttributeSchemaVersion` |

## F. PostgreSQL repository

ينفذ `internal/adapters/secondary/persistence/postgres/catalog_repository.go` الواجهة فوق `SQLExecutor` transaction-aware. القراءة تتحقق من parent scope وتعيد `RepositoryNotFound` في حالة missing/cross-tenant. التحديثات تستخدم expected resource version؛ المورد المفقود يعيد `not_found`، والنسخة القديمة تعيد typed `stale`، وأخطاء uniqueness/constraints تصنف typed conflict/invalid.

لا توجد network أو Provider calls داخل repository أو transaction. إنشاء schema مع definitions يتم داخل transaction واحدة، و`NextAttributeSchemaVersion` يستخدم transaction-level advisory lock لضمان allocation ذري داخل Business/name.

## G. Application mapping

`internal/application/services/catalog_queries.go` يحتوي service type منفصلًا لكل query حتى يطابق Go `Handle` typed interfaces دون overload. `internal/application/services/catalog_commands.go` ينفذ command services typed لكل mutation الحالية، ويحوّل `ResourceVersion` opaque string إلى عداد موجب عند حدود Application، ثم يعيد view/version/status في `MutationResult`.

تم أيضًا فصل `CatalogListInput` عن `BusinessListInput` كي يصل status filter من HTTP إلى `ListCatalogsQuery`، مع بقاء DTO/Huma مصدر OpenAPI. تم اختبار schema version/definitions وOffer decimal projection وCatalog resource version من 1 إلى 2.

## H. Tests actually run

تم تشغيل الأوامر التالية فعليًا ونجحت:

| Command | Result |
|---|---|
| `GOTOOLCHAIN=local gofmt` | PASS |
| `GOTOOLCHAIN=local go test ./...` | PASS |
| `GOTOOLCHAIN=local go vet ./...` | PASS |
| `GOTOOLCHAIN=local go generate ./internal/adapters/primary/http/contract` | PASS |
| `scripts/check-openapi-generated.sh` | PASS |
| `scripts/test-postgres-schema.sh` | PASS على PostgreSQL 16؛ `applied=29` ثم `applied=0` |
| `POSTGRES_TEST_DSN=... GOTOOLCHAIN=local go test -tags=integration -count=1 ./internal/adapters/secondary/persistence/postgres` | PASS على PostgreSQL 16 Docker |
| `git diff --check` | PASS |

Integration test يغطي insert/read، filters، deterministic ordering، opaque keyset pagination، raw JSON semantics، decimal money، definitions، tenant isolation، typed not-found، malformed cursor، create/update، stale version، schema version، transaction commit، وrollback.

## I. Files changed

- `migrations/000029_catalog_resource_versions.up.sql`
- `internal/application/ports/catalog_repository.go`
- `internal/application/services/catalog_queries.go`
- `internal/application/services/catalog_commands.go`
- `internal/adapters/secondary/persistence/postgres/catalog_repository.go`
- `internal/adapters/secondary/persistence/postgres/business_repository.go`
- `internal/adapters/secondary/persistence/postgres/business_repository_integration_test.go`
- `internal/adapters/primary/http/dto/inputs.go`
- `internal/adapters/primary/http/contract/aliases.go`
- `internal/adapters/primary/http/contract/api.go`
- `internal/adapters/primary/http/handlers/query_facades.go`
- `docs/pr009-catalog-read-persistence-decision-ar.md`
- `docs/pr009-postgres-transaction-boundary-ar.md`
- `docs/pr009-repository-surface-matrix-ar.md`
- `docs/project-status-ar.md`
- `docs/implementation-roadmap-ar.md`
- `scripts/test-postgres-schema.sh`
- `go.mod`

## J. Commit SHA

Commit المرفوع إلى private GitHub على `main` موثق في رسالة التسليم وسجل Git بعد آخر amend.

تم التحقق من أن `HEAD` المحلي يساوي `origin/main`، وأن working tree نظيف.

## K. Remaining PR-009 work

Catalog Persistence الحالية — read/write surfaces وApplication command/query services — مثبتة. ما زال PR-009 مفتوحًا بسبب:

| Remaining | Status |
|---|---|
| Sales repositories | التالي بعد Catalog |
| AI/Audit repositories | مؤجل بعد Sales |
| EventStore وatomic inbound dedupe | مؤجل حتى انتهاء repository surfaces |
| Outbox persistence/worker boundary | مؤجل بعد EventStore |
| Bootstrap dependency wiring في `cmd/api` | غير منفذ؛ `handlers.Dependencies{}` ما زال يحتاج تركيبًا فعليًا |
| Semantic validation للـattributes مقابل schema definitions | مسؤولية Domain/Application لاحقة |
| Provider price verification وSocialAPI/Chatwoot runtime | خارج PR-009 وغير منفذ |

لا نبدأ EventStore أو Outbox أو SocialAPI أو Chatwoot أو AI runtime في هذه النقطة. **الخطوة التالية المنضبطة هي Sales Persistence** فوق نفس TransactionManager، مع إبقاء Reliability layer بعد إكمال Sales وAI/Audit حسب Repository Surface Matrix.
