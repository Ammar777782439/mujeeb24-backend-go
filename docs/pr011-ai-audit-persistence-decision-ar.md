# PR-011 — قرار وإغلاق AI/Audit Persistence

## الحكم التنفيذي

هذه الدفعة لا تنفذ LLM ولا Policy Engine ولا Action Executor. هي تثبت **حفظ Structured AIDecision** وقراءة Dashboard لها، وتنفذ انتقال `RequestHumanReview`، وتثبت **AuditEvent append-only** وقراءة Audit ضمن Business scope.

> `LLM ≠ Executor`، و`AIDecision ≠ Authorization`، و`Audit ≠ Event Ledger`.

## A. العقد الذي تم التحقق منه

| Evidence | Fact verified from code and tests |
|---|---|
| Domain AI/Audit contract | AIDecision structured ويحمل intent/entities/evidence/action/policy/lifecycle، وAuditEvent سجل append-only بلا أسرار أو Chain-of-Thought. |
| HTTP contract | AI: List/Get وRequestHumanReview فقط؛ Audit: List/Get فقط؛ لا Dashboard mutation لـAudit. |
| Application commands/queries | `RequestHumanReviewCommand` هو AI write use case الحالي؛ queries تحدد filters الخاصة بـAI/Audit. |
| migrations 000026/000027 | الجداول الأساسية موجودة، وكان ينقص AI resource version وحقول review، وينقص Audit بعض حقول العقد. |
| Migration 000031 | طبقت على PostgreSQL 16 بنجاح، وrunner أعاد `applied=31` ثم `applied=0`. |

## B. السطح النهائي المغلق

| Aggregate | Read | Write |
|---|---|---|
| AIDecision | List/Get مع lifecycle وconversation وrequires_human filters، keyset `(created_at DESC, id DESC)`، وbusiness scope | `RequestHumanReview` بشكل conditional على `resource_version`؛ يبقى lifecycle كما هو ولا يتحول الطلب إلى authorization أو execution |
| AuditEvent | List/Get مع actor/action/resource/from/until filters، keyset `(occurred_at DESC, id DESC)`، وbusiness scope | Append فقط من Application/internal callers؛ لا update/delete methods |

لا ننشئ `DecisionAudit` table ثانية، ولا generic CRUD، ولا Event Ledger methods داخل Audit repository.

## C. Migration/schema rationale

الرقم التالي الفعلي هو **000031**: `migrations/000031_ai_audit_contract_reconciliation.up.sql`، وهي forward-only ولا تعدّل `000001–000030`.

تضيف migration إلى AI `resource_version` و`decided_at` وحقول طلب Human Review، مع positive/pairing checks وفهارس business/filter support. وتضيف إلى Audit `decision_reference` و`result` و`reason_code` و`before_reference` و`after_reference` و`schema_version` و`redaction_version`، مع checks وفهارس action/actor، ثم trigger يمنع UPDATE وDELETE.

لم تُضف lifecycle execution states جديدة إلى AIDecision؛ التنفيذ اللاحق مصدره Action/Outbox/Transaction. ولم تُضف أي أعمدة للأسرار أو prompt أو Chain-of-Thought أو raw provider payload كامل.

## D. Ownership, isolation, and transaction boundary

كل query وappend/update يحمل `business_id` صراحة. cross-tenant resource يعاد كـtyped not-found بدل كشف وجوده. `RequestHumanReview` يحدّث القرار ذريًا مع `resource_version` المتوقع، ثم يضيف `human.handoff_requested` إلى Audit داخل transaction واحدة؛ failure في Audit يؤدي إلى rollback للقرار. الطلب لا يغير lifecycle إلى authorized ولا ينفذ side effect خارجي.

Audit append-only فعليًا على مستوى PostgreSQL؛ integration test أثبت رفض UPDATE وDELETE بالـtrigger. أي تصحيح تاريخي يجب أن يكون Event جديدًا يشير إلى السابق.

## E. JSON and sensitive data

Application records وports typed. الحقول المرنة تعاد كـraw JSON bytes: `entities` object، و`evidence_references` و`missing_information` و`reason_codes` arrays، وAudit `metadata` object. HTTP projection فقط يحولها إلى `map[string]any` أو string slices التي يطلبها DTO. لا يستخدم PostgreSQL/Application record contract `map[string]any` كبديل عن schema typed.

## F. Repository/Application implementation

`internal/application/ports/ai_audit_repositories.go` يعرّف `AIDecisionRepository` و`AuditEventRepository` typed، مع pages/filters/drafts و`context.Context`. نفذت PostgreSQL repositories في `internal/adapters/secondary/persistence/postgres/ai_audit_repositories.go` باستخدام SQL executor transaction-aware، مع typed not-found/invalid/stale/conflict classification.

خدمات Application في `internal/application/services/ai_audit_queries.go` تنفذ List/Get mapping وbusiness scope وRFC3339 range validation. وخدمة `ai_commands.go` تنفذ `RequestHumanReview` مع If-Match parsing وAudit append الذري.

لأن Huma v2.32.0 لا يدعم pointer query parameters، عُرّف `dto.OptionalBool` كـcustom scalar يدعم `TextUnmarshaler` و`SchemaProvider`: يبقى OpenAPI boolean، ويميز omission عن `false`، ثم يحول HTTP façade القيمة إلى `*bool` فقط عند وجود parameter.

## G. HTTP mapping

تم توسيع Application views بالحقول التي يحتاجها DTO فقط، مع إبقاء raw JSON خارج HTTP boundary حتى projection. AIDecision يعرض conversation/intent/entities/evidence/action/requires_human/missing_information/policy_version/lifecycle/created_at، وAudit يعرض actor/resource/metadata/occurred_at وفق DTO الحالي. لم تُكشف Chatwoot أو SocialAPI أو provider secrets أو SQL details.

## H. اختبارات تم تشغيلها فعليًا

| Validation | Result |
|---|---|
| `GOTOOLCHAIN=local go test ./...` | PASS |
| `GOTOOLCHAIN=local go vet ./...` | PASS |
| `go generate ./internal/adapters/primary/http/contract` | PASS؛ generated OpenAPI أعيد توليده من DTO/contract |
| `scripts/check-openapi-generated.sh` | PASS؛ `openapi_drift_check=PASS` |
| `scripts/test-postgres-schema.sh` | PASS؛ constraints، migration `applied=31` ثم `applied=0` |
| AI/Audit integration على PostgreSQL 16 fresh DB | PASS؛ migration، insert/read، filters، keyset، malformed cursor، not-found، tenant isolation، stale، review transition، application mapping، rollback، metadata constraint، append-only trigger |
| كل PostgreSQL integration tests | PASS؛ package `internal/adapters/secondary/persistence/postgres` على PostgreSQL 16 fresh DB |
| `git diff --check` | PASS |

## I. الملفات التي تغيرت في PR-011

| Area | Files |
|---|---|
| Schema | `migrations/000031_ai_audit_contract_reconciliation.up.sql` |
| Ports | `internal/application/ports/ai_audit_repositories.go` |
| PostgreSQL | `internal/adapters/secondary/persistence/postgres/ai_audit_repositories.go` |
| Application queries/commands | `internal/application/services/ai_audit_queries.go`, `internal/application/services/ai_commands.go` |
| Application views | `internal/application/commands/views.go` |
| Dashboard DTO/projection | `internal/adapters/primary/http/dto/inputs.go`, `internal/adapters/primary/http/dto/types.go`, `internal/adapters/primary/http/handlers/query_facades.go` |
| Integration tests | `internal/adapters/secondary/persistence/postgres/ai_audit_repository_integration_test.go` |
| Schema runner | `scripts/test-postgres-schema.sh` |
| Decision record | `docs/pr011-ai-audit-persistence-decision-ar.md` |

## J. ما لم يدخل هذه الدفعة

لم يتم في PR-011 تنفيذ أو wiring لـEventStore أو atomic inbound dedupe أو Outbox أو Bootstrap أو Provider Simulator أو SocialAPI أو Chatwoot أو AI runtime. كما أن `cmd/api/main.go` ما زال لا يحقن production dependencies؛ وجود repository/service/HTTP contract لا يعني أن runtime أصبح موصولًا.

## K. بوابة الانتقال التالية

بعد commit/push والتحقق من clean tree، الدفعة التالية الوحيدة هي **PR-012 — EventStore + atomic inbound dedupe**. لا نفتح PR-013 Outbox ولا Providers ولا Bootstrap قبل إغلاقها وفق الترتيب المعتمد.
