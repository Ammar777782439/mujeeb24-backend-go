# PR-012 — EventStore وAtomic Inbound Dedupe

## الحكم التنفيذي

هذه الدفعة تنفذ **Inbound Event Ledger durable persistence** و**atomic dedupe/claim** فقط. لا تنفذ Outbox أو Provider Simulator أو SocialAPI أو Chatwoot أو AI Runtime أو production bootstrap wiring.

> EventStore يجيب عن: **ماذا وصل؟ وهل يحق لعامل واحد أن يعالجه الآن؟**

## A. العقد المثبت

الهوية المنطقية للحدث ليست نص الرسالة ولا hash النص، بل:

```text
provider_ref + provider_connection_ref + provider_event_id
```

وهي unique على PostgreSQL. يحتفظ السجل بمرجع raw payload وpayload hash، لا بالـraw provider payload داخل Domain/Application record.

الـEventStore port في `internal/application/ports/event_store.go` يملك العمليات التالية فقط:

| Operation | Meaning |
|---|---|
| `RecordIfAbsent` | insert ذرّي أو إعادة السجل الموجود عند duplicate |
| `Get` | قراءة بــbusiness scope |
| `List` | قراءة keyset مع filters |
| `Claim` | منح lease ذري لعامل واحد |
| `MarkProcessed` | إنهاء المعالجة بالـowner/token |
| `MarkRetryableFailure` | إعادة الحدث إلى retryable state مع next attempt |
| `MoveToDeadLetter` | إيقاف إعادة المحاولة مع error code |

لا يوجد `IdempotencyStore` عام موازٍ؛ EventStore هو مالك inbound idempotency في V1.

## B. لماذا EventStore مطلوب؟

عند تكرار webhook من Provider يجب ألا تنشأ Message أو Lead أو Transaction أو AI Decision ثانية. `RecordIfAbsent` ينفذ `INSERT ... ON CONFLICT` على unique provider identity، لذلك لا توجد نافذة race من نوع `SELECT ثم INSERT`.

أما claim فليس مجرد `MarkProcessing`. PostgreSQL يمنح lease واحدًا فقط، ويحفظ owner وtoken وexpiry. عامل آخر يرى الحدث الموجود ولا يحصل على `Claimed=true`. العامل القديم لا يستطيع إكمال أو فشل الحدث بعد انتهاء lease أو بعد انتقال الملكية.

## C. Migration/schema rationale

الرقم التالي الفعلي هو **000032**: `migrations/000032_event_store_lease_results.up.sql`. migration forward-only ولا تعدّل `000001–000031`.

أضافت:

| Column/constraint | Reason |
|---|---|
| `processing_lease_token` | fencing token UUID يمنع worker قديمًا من الكتابة |
| `processing_result_code` | نتيجة داخلية آمنة بعد المعالجة، وليست provider payload |
| `processed_at` | وقت اكتمال الخطوات الداخلية المطلوبة |
| signature check | الحدث غير الموثق لا يدخل الحالة الطبيعية؛ يقتصر على unresolved/rejected |
| lease-token check | processing يتطلب token، وباقي الحالات تنظفه |
| processed-at check | processed يتطلب timestamp، وباقي الحالات لا تحمل timestamp قديمًا |
| claimable index | دعم received/retryable_failed readiness |
| owner/lease index | دعم مراقبة leases processing |

`processed` يعني اكتمال المعالجة الداخلية المطلوبة، ولا يعني أن AI أجاب أو أن العميل استلم ردًا.

## D. Lifecycle وlease semantics

الحالات المستخدمة تأتي من schema الحالية:

```text
received → processing → processed
received → rejected
processing → retryable_failed → processing
processing → dead_letter
```

ويسمح `Claim` أيضًا باستعادة حدث processing انتهى lease الخاص به. أما lease الجديد فيجب أن يكون مستقبليًا، وstate mutation لا ينجح إلا بــowner/token وlease غير منتهٍ.

`attempt_count` يزيد عند كل claim، و`MarkRetryableFailure` يتطلب `next_attempt_at`، لذلك لا تعود المحاولة قبل موعدها. `MoveToDeadLetter` يمحو lease ولا يحدد next attempt.

## E. PostgreSQL implementation

التنفيذ موجود في `internal/adapters/secondary/persistence/postgres/event_store.go` ويستخدم `Adapter.Executor(ctx)`، لذلك يعمل داخل transaction context نفسه عند استدعائه من `Adapter.Within`، دون provider أو network calls.

`RecordIfAbsent` يعيد `created=true` للفائز الأول و`created=false` مع السجل canonical للطلبات المكررة. `Claim` ينفذ conditional update ذريًا، وstate mutation methods تتحقق من lease fencing في WHERE clause.

كل read يحمل business scope عند الحاجة، ولا توجد قراءة Dashboard عامة للأحداث أو كشف cross-tenant existence.

## F. اختبارات PostgreSQL 16

الاختبار في `internal/adapters/secondary/persistence/postgres/event_store_integration_test.go` ويستخدم PostgreSQL 16 fresh database.

| Test area | Verified |
|---|---|
| migration runner | تطبيق migrations ثم القراءة من schema الجديدة |
| sequential dedupe | نفس identity ينتج row واحدًا |
| concurrent dedupe | 32 عاملًا؛ created winner واحد فقط |
| concurrent claim | 32 عاملًا؛ lease winner واحد فقط |
| provider/connection scope | نفس provider event ID لا يتصادم عبر connection/tenant مختلف |
| tenant isolation | cross-business Get يرجع typed not-found |
| keyset pagination | ordering وnext cursor |
| malformed cursor | typed invalid |
| first claim | processing + owner/token + attempt increment |
| active duplicate claim | لا يمنح lease ثانيًا |
| processed transition | تنظيف lease وتسجيل result/processed_at |
| retry transition | error code وnext attempt، ثم claim بعد due time |
| dead letter | state نهائي بلا lease |
| expired/stale worker protection | worker القديم لا يملك mutation صالحًا |
| signature validation | unverified normal event مرفوض؛ rejected مسموح |
| transaction rollback | حدث داخل transaction فاشلة لا يبقى بعد rollback |

## G. ما لم يدخل PR-012

لم يتم تنفيذ أو wiring لـ:

```text
Outbox
Provider Simulator
SocialAPI
Chatwoot
AI Runtime
Lead/Transaction processing
HTTP production inbound handler
Bootstrap dependency injection
```

لا يجوز اعتبار وصول event إلى EventStore دليلًا على إنشاء Conversation أو CommunicationMessage أو AI Decision. هذه مراحل لاحقة في orchestration/vertical slice.

## H. Validation executed

| Command | Result |
|---|---|
| `GOTOOLCHAIN=local go test ./...` | PASS |
| `GOTOOLCHAIN=local go vet ./...` | PASS |
| `GOTOOLCHAIN=local go test -tags=integration -count=1 ./internal/adapters/secondary/persistence/postgres` | PASS على PostgreSQL 16 fresh database |
| `bash scripts/test-postgres-schema.sh` | PASS؛ runner `applied=32` ثم `applied=0` |
| `git diff --check` | PASS |

## I. الملفات

| Area | Files |
|---|---|
| Port | `internal/application/ports/event_store.go` |
| Migration | `migrations/000032_event_store_lease_results.up.sql` |
| PostgreSQL adapter | `internal/adapters/secondary/persistence/postgres/event_store.go` |
| Integration tests | `internal/adapters/secondary/persistence/postgres/event_store_integration_test.go` |
| Schema runner | `scripts/test-postgres-schema.sh` |
| Decision record | `docs/pr012-event-store-decision-ar.md` |

## J. الإغلاق

لا يُعلن PR-012 مغلقًا نهائيًا إلا بعد إعادة validation وcommit/push والتحقق من مساواة `HEAD` مع `origin/main` ونظافة working tree. بعد ذلك تكون الدفعة التالية **PR-013 — Outbox persistence/worker boundary**، وليس Providers أو Bootstrap قبل ترتيبها.
