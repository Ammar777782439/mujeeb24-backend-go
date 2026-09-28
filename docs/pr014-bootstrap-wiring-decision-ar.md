# PR-014 — Bootstrap Wiring

## الحكم التنفيذي

تم تنفيذ Composition Root واحد يركب إعدادات العملية، PostgreSQL، repositories، Application services، HTTP router، وWorker lifecycle. هذه الدفعة لا تضيف feature تجارية ولا تنفذ Provider أو AI runtime.

```text
cmd/api
  → ProcessConfig
  → PostgreSQL Adapter
  → repositories + EventStore + Outbox
  → Application services
  → Huma HTTP router
  → graceful shutdown
```

و:

```text
cmd/worker
  → ProcessConfig
  → PostgreSQL Adapter
  → EventStore + Outbox ports
  → explicit Run/Shutdown lifecycle
```

## A. ما كان مكسورًا قبل PR-014

كان `cmd/api/main.go` ينشئ `handlers.Dependencies{}` فارغة، ثم يبني router مباشرة ويستعمل عنوانًا ثابتًا. وكان `cmd/worker/main.go` يطبع رسالة فقط دون فتح قاعدة البيانات أو امتلاك runtime lifecycle.

النتيجة السابقة كانت أن repositories وApplication services موجودة كقطع منفصلة، لكن لا يوجد Composition Root يربطها في process قابل للفحص.

## B. ProcessConfig

أُضيف `internal/platform/config/config.go` ليحمل إعدادات العملية فقط، وليس بيانات Business أو Customer أو secret material.

المتغيرات المدعومة تشمل:

| Variable | Default/Rule |
|---|---|
| `APP_ENV` | `development` |
| `DATABASE_URL` | required |
| `HTTP_ADDR` | `:3001` |
| `SHUTDOWN_TIMEOUT` | `10s` |
| `DB_MAX_CONNS` | `10` |
| `DB_MIN_CONNS` | `1` |
| `DB_MAX_CONN_LIFETIME` | `1h` |
| `DB_MAX_CONN_IDLE_TIME` | `30m` |
| `DB_HEALTH_CHECK_PERIOD` | `1m` |
| `DB_CONNECT_TIMEOUT` | `5s` |

يفشل loader مبكرًا عند غياب `DATABASE_URL` أو وجود duration غير صالح أو pool configuration غير منطقية.

## C. Composition Root وdependency wiring

الملف `internal/bootstrap/dependencies.go` ينشئ adapters/repositories typed ويحقن Application services الموجودة فعليًا في `handlers.Dependencies`.

تم توصيل الأسطح التالية:

| Surface | Wiring |
|---|---|
| Communication messages | `MessageQueryService` + `MessageRepository` |
| Channel capabilities | `ConnectionCapabilitiesQueryService` + repository |
| Catalog | read services + write command services + `CatalogRepository` |
| Sales | lead/transaction query and command services + repositories |
| AI | decision queries + `RequestHumanReviewCommandService` |
| Audit | audit queries + `AuditRepository` |
| Reliability | `EventStore` و`OutboxStore` داخل runtime |
| Operational | liveness/readiness handlers |

لم يتم اختراع implementations للأسطح غير الموجودة. لذلك بقيت مثل Auth storage وScope provider وOutbound send orchestration غير موصولة بدل إخفائها خلف stubs.

## D. API runtime

الملف `internal/bootstrap/api.go` يعرّف `APIRuntime` ويقوم بالتالي:

1. يفتح PostgreSQL باستخدام `ProcessConfig`.
2. ينشئ typed dependencies.
3. يبني Huma API/router من نفس contract الموجود.
4. يضيف root `/health` للتوافق التشغيلي السابق.
5. يركب `/api/v1/health/live` و`/api/v1/health/ready` عبر Application health handlers.
6. يوفر `Serve()` و`Shutdown(ctx)` واضحين.
7. يغلق HTTP server قبل PostgreSQL.

لا يشغل API migrations تلقائيًا؛ `cmd/migrate` يبقى process مستقلًا كما ينص العقد.

## E. Liveness وReadiness

`liveness` يفحص process فقط ولا يعتمد على Provider. `readiness` يفحص PostgreSQL ويعيد external dependency error عند فشلها، وهو ما يترجم إلى HTTP 503 عبر الـHTTP error mapping الحالي.

تعطل Provider مستقبلاً لا يجب أن يقتل Core process تلقائيًا؛ Provider health يظل منفصلًا عن core readiness.

## F. Worker runtime

الملف `internal/bootstrap/worker.go` يملك `WorkerRuntime` typed يحتوي على:

```text
Database
EventStore
Outbox
```

ويملك `Run(ctx)` و`Shutdown()`. في هذه المرحلة `Run` ينتظر cancellation فقط، لأن job handlers والـqueue server لم تُنفذ بعد. هذا مقصود وليس ادعاء أن Outbox أصبح يرسل رسائل.

## G. Entry points

أصبح `cmd/api/main.go` entrypoint نحيفًا:

```text
LoadFromEnv
→ BuildAPI
→ Serve
→ SIGINT/SIGTERM
→ Shutdown with timeout
```

وأصبح `cmd/worker/main.go`:

```text
LoadFromEnv
→ BuildWorker
→ Run until signal
→ Shutdown database
```

## H. اختبارات Bootstrap

الاختبار في `internal/bootstrap/bootstrap_integration_test.go` ويستخدم PostgreSQL 16 حقيقيًا.

| Test | Result |
|---|---|
| API Build يفتح database | PASS |
| implemented dependencies غير nil | PASS |
| EventStore/Outbox داخل API runtime | PASS |
| Scope provider غير مخترع | PASS |
| `/api/v1/health/live` | HTTP 200 PASS |
| `/api/v1/health/ready` مع PostgreSQL | HTTP 200 PASS |
| API shutdown يغلق database | PASS |
| shutdown ثانية idempotent | PASS |
| Worker Build يركب EventStore/Outbox | PASS |
| Worker cancellation lifecycle | PASS |
| config missing DATABASE_URL | PASS |
| config invalid duration/pool | PASS |

## I. Validation

تم تشغيل الآتي على آخر revision قبل commit:

```text
GOTOOLCHAIN=local go test ./...
GOTOOLCHAIN=local go vet ./...
go test -tags=integration -p 1 -count=1 ./internal/adapters/secondary/persistence/postgres ./internal/bootstrap  على PostgreSQL 16 fresh database
bash scripts/test-postgres-schema.sh
go generate ./internal/adapters/primary/http/contract
bash scripts/check-openapi-generated.sh
git diff --check
```

استخدمنا `-p 1` في integration لأن الاختبارات الحالية تشترك في DSN واحد وتطبق migrations داخل كل package؛ تشغيلها بالتوازي على نفس القاعدة يسبب race بين migration runners، وليس فشلًا في Bootstrap.

## J. الملفات

| Area | File |
|---|---|
| Process config | `internal/platform/config/config.go` |
| Config tests | `internal/platform/config/config_test.go` |
| API composition | `internal/bootstrap/api.go` |
| Dependency composition | `internal/bootstrap/dependencies.go` |
| Health handlers | `internal/bootstrap/health.go` |
| Worker composition | `internal/bootstrap/worker.go` |
| API entrypoint | `cmd/api/main.go` |
| Worker entrypoint | `cmd/worker/main.go` |
| Bootstrap integration tests | `internal/bootstrap/bootstrap_integration_test.go` |
| Environment example | `.env.example` |

## K. الحدود المتبقية

لم يدخل PR-014:

```text
Provider Simulator
SocialAPI
Chatwoot
Meta
AI Runtime/LLM
Queue server implementation
Outbox worker job execution
Auth persistence/Scope provider
Webhook production processing
```

بعد إغلاق هذه الدفعة تكون الخطوة التالية حسب السلسلة الحالية **PR-015 — Provider Simulator**، مع الحفاظ على أن أي provider call أو external side effect يأتي بعد worker/adapter contract المناسب، وليس داخل Bootstrap أو Repository.
