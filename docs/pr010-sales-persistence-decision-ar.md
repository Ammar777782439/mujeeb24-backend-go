# PR-010 — Sales Persistence Decision

## الحكم التنفيذي

تم تنفيذ **Sales Persistence الحالية** فوق PostgreSQL وApplication ports، دون اختزال CommercialTransaction إلى Order ودون استخدام `lead_attributions` كبديل صامت عن Lead aggregate. الدفعة تشمل migration forward-only `000030`، typed repositories، command/query services، واختبارات PostgreSQL 16 حقيقية.

## A. Evidence verified by repository

| Source | Verified fact |
|---|---|
| `internal/application/commands/leads_transactions.go` | Lead commands: Create/Update/Qualify/MarkLost. Transaction commands: Create/Update draft، Confirm/Cancel، Submit/Approve/Reject review. |
| `internal/application/queries/queries.go` | Lead queries: List/Get Lead، List Attributions، List Scores. Transaction queries: List/Get Transaction، Get Review، وcustomer transaction list alias. |
| `internal/application/commands/views.go` | Current views expose Lead and Transaction IDs, business/customer scope, state/status، وResourceVersion. |
| `internal/application/ports/sales_repositories.go` | Typed records/pages/drafts/patches وrepository interfaces للـLead وCommercialTransaction وchild lifecycle records. |
| `internal/adapters/secondary/persistence/postgres/lead_repository.go` | Tenant-scoped Lead list/get/create/update/qualify/lost، attribution history، score history، opaque cursors، typed errors. |
| `internal/adapters/secondary/persistence/postgres/transaction_repository.go` | Universal transaction list/get، draft lines، snapshots، confirm/cancel، review submit/decision، optimistic concurrency، transaction-aware writes. |

## B. Lead contract and reconciliation

`leads` هو aggregate root ويملك customer scope وqualification context وstatus وactor/lost fields وresource version. أضافت `000030` الحقول التي يثبتها Domain contract: source conversation reference، source channel، intent reference، ownership reference، next action، qualification state، qualification reason/evidence، وresource version.

`lead_attributions` بقي append-only attribution history يحتوي source interaction/catalog references، بينما `lead_scores` بقي score history. لا يدّعي repository أنه ينشئ assignment workflow أو score calculation؛ هو يحفظ ويقرأ السجلات التي يطلبها Application.

الـLead status الأصلي في schema (`new`, `interested`, `qualified`, `won`, `lost`) بقي محفوظًا للتوافق، بينما `qualification_state` الجديد يطابق contract (`new`, `qualified`, `working`, `converted`, `lost`, `disqualified`). Qualify يحدّث status/state إلى `qualified`، وMarkLost يحدّثهما إلى `lost`. لا يسمح repository بتحويل Lead مفقود أو won إلى lost.

## C. CommercialTransaction universal contract

CommercialTransaction يقبل الأنواع السبعة فقط: `order`, `booking`, `appointment`, `service_request`, `reservation`, `quote`, و`subscription`. الاختبار الفعلي ينشئ نوعًا من كل نوع، مع بقاء الكيان العام خاليًا من حقول vertical-specific مثل flight/doctor/room.

`order_lines` تُكتب ضمن draft transaction وتحفظ snapshots مستقلة: اسم item، selected attributes، pricing، availability، fulfillment، quantity، unit price، line total، والعملة. SQL يتحقق من CatalogItem وOffer وVariant scope، ولا يعيد تفسير transaction القديمة من Catalog الحالي.

Confirmation وReview child records one-to-one حسب القيود الأصلية. Submit review يرفع `requires_human_review` ويزيد transaction resource version، وapprove/reject يحدّث review وtransaction version داخل نفس transaction. أضيف `decision_reason` في `000030` كي لا تضيع Reason الخاصة بقرار المراجع.

## D. Migration and schema reconciliation

الرقم الفعلي هو **000030**: `migrations/000030_sales_contract_reconciliation.up.sql`. لم تُعدّل migrations `000001–000029`.

تضيف migration resource versions إلى Lead وCommercialTransaction، وتضيف Lead source/qualification fields، Transaction review reason، cancellation reason وhuman-review flag، وorder-line pricing/availability/fulfillment snapshots مع JSON object constraints وفهارس Sales المطلوبة.

## E. Concurrency, state, and money

كل PATCH/transition يتطلب `ExpectedVersion` موجبًا. التحديث الشرطي يطابق business/id/version ثم يزيد العداد ذريًا. المورد غير الموجود يعيد `not_found`، والنسخة القديمة تعيد typed `stale`، والحالة غير المسموحة تعيد typed conflict يتحول Application إلى `invalid_state_transition`.

Transaction totals وline snapshots تستخدم PostgreSQL `NUMERIC(20,4)`. Application يستقبل quantity كـDecimal بعد تحويل HTTP، ويتحقق من أنها rational positive decimal. لا يستخدم repository `float64` للحسابات المالية، ولا يعيد حساب total من floating-point input. Price/availability/fulfillment snapshots تُقرأ من Offer الحالية لحظة إنشاء line ثم تُحفظ تاريخيًا.

## F. Application ports and repository surface

| Aggregate | Read | Write |
|---|---|---|
| Lead | List/Get، List attributions، List score history | Create/Update context، Qualify، MarkLost |
| CommercialTransaction | List/Get، Get review، List customer transactions | Create/Update draft، Confirm، Cancel، Submit/Approve/Reject review، line snapshots |

لا يوجد generic CRUD لكل child table. Assignment/provider attribution وvertical-specific transaction details لا تُخترع داخل repository؛ تحتاج Use Case/Domain contract مستقلًا إذا أُضيفت لاحقًا.

## G. Application mapping

`internal/application/services/sales_queries.go` يطبق services typed لكل Lead وTransaction query ويحوّل raw records إلى views. `internal/application/services/sales_commands.go` يطبق services typed لكل commands الحالية، ويملك validation لـIf-Match وtransaction type وJSON objects وpositive decimal quantities، ثم يمرر writes عبر `TransactionManager`.

HTTP façade mapping السابق يبقى منفصلًا؛ وجود services/repositories لا يعني أن `cmd/api` أصبح موصولًا بها. runtime bootstrap wiring ما زال دفعة مستقلة، ولا ندّعي functional production endpoint من هذا commit.

## H. Tests actually run

| Command | Result |
|---|---|
| `GOTOOLCHAIN=local gofmt` | PASS |
| `GOTOOLCHAIN=local go test ./...` | PASS |
| `GOTOOLCHAIN=local go vet ./...` | PASS |
| `GOTOOLCHAIN=local go test -tags=integration -run '^$' ./internal/adapters/secondary/persistence/postgres` | PASS |
| `go generate ./internal/adapters/primary/http/contract` | PASS |
| `scripts/check-openapi-generated.sh` | PASS |
| `scripts/test-postgres-schema.sh` | PASS على PostgreSQL 16؛ `applied=30` ثم `applied=0` |
| `POSTGRES_TEST_DSN=... GOTOOLCHAIN=local go test -tags=integration -count=1 ./internal/adapters/secondary/persistence/postgres` | PASS على PostgreSQL 16 Docker |
| `git diff --check` | PASS |

Integration test يغطي Lead create/update/qualify/lost وresource versions، tenant isolation وpagination وmalformed cursor وattribution/score mapping؛ ويغطي transaction types السبعة، line snapshots، decimal projection، draft update، stale update، confirmation، review submit/approve/reject، stale cancel، query mapping، commit، وrollback.

## I. Files changed

- `migrations/000030_sales_contract_reconciliation.up.sql`
- `internal/application/ports/sales_repositories.go`
- `internal/application/services/sales_commands.go`
- `internal/application/services/sales_queries.go`
- `internal/adapters/secondary/persistence/postgres/lead_repository.go`
- `internal/adapters/secondary/persistence/postgres/transaction_repository.go`
- `internal/adapters/secondary/persistence/postgres/sales_repository_integration_test.go`
- `scripts/test-postgres-schema.sh`
- `docs/pr010-sales-persistence-decision-ar.md`
- `docs/pr009-repository-surface-matrix-ar.md`
- `docs/pr009-postgres-transaction-boundary-ar.md`
- `docs/project-status-ar.md`
- `docs/implementation-roadmap-ar.md`

## J. Commit SHA

سيُثبت SHA النهائي في رسالة التسليم وسجل Git بعد نجاح آخر validation؛ لا يوضع hash داخل محتوى commit حتى لا يصبح self-referential عند amend.

## K. Remaining work

بعد إغلاق Sales Persistence الحالية، لا يزال PR-009/سلسلة Persistence مفتوحًا في البنود التالية:

| Remaining | Status |
|---|---|
| AI/Audit repositories | التالي بعد Sales |
| EventStore وatomic inbound dedupe | مؤجل بعد AI/Audit |
| Outbox persistence/worker boundary | مؤجل بعد EventStore |
| Bootstrap dependency wiring في `cmd/api` | غير منفذ؛ services/repositories ليست production-wired بعد |
| Domain semantic validation للـattributes والـvertical details | لاحقة ومسؤولة عن invariants المتخصصة |
| Provider Simulator وSocialAPI/Chatwoot | خارج هذه الدفعة وغير منفذ |

**الخطوة التالية المنضبطة هي AI/Audit Persistence**، ثم EventStore مع inbound idempotency الذرية، ثم Outbox. لا نلمس Provider runtime قبل إكمال Reliability foundation.
