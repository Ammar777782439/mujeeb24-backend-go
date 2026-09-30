# PR-013 — Outbox Persistence / Worker Boundary

## الحكم التنفيذي

تنفذ هذه الدفعة **durable outbound work** و**worker boundary** فقط. لا تنفذ Provider Simulator أو SocialAPI أو Chatwoot أو AI Runtime أو Bootstrap production wiring أو actual message sending.

> `OutboundMessage` هو نية/حالة اتصال تجاري، بينما `OutboxEntry` هو الالتزام التقني بأن تُنفذ محاولة خارجية لاحقًا.

ينشأ السجلان داخل نفس DB transaction عندما يقرر Use Case إرسالًا خارجيًا:

```text
BEGIN
  Create OutboundMessage
  Enqueue OutboxEntry with dedupe key
COMMIT
```

ما بعد commit هو مسؤولية worker لاحقًا، وليس مسؤولية Repository.

## A. العقد المثبت

تم تعريف `OutboxStore` في `internal/application/ports/outbox.go` بالعمليات المتخصصة التالية:

| Operation | Meaning |
|---|---|
| `Enqueue` | حفظ obligation بحالة pending، مع dedupe key |
| `Get` | قراءة entry بــbusiness scope |
| `List` | قراءة keyset مع status filter |
| `Claim` | منح lease ذري لعامل واحد |
| `MarkCompleted` | تسجيل أن worker نفذ محاولته |
| `MarkRetryableFailure` | جدولة إعادة المحاولة |
| `MoveToDeadLetter` | إيقاف retry مع سبب دائم |
| `Requeue` | إعادة dead-letter إلى pending بقرار صريح |

لا يضع الـport تفاصيل Redis أو Asynq أو provider payload داخله.

## B. لماذا Outbox مطلوب؟

بدون Outbox قد تُحفظ حالة العمل التجاري ثم يحدث crash قبل إرسال side effect الخارجي. Outbox يجعل domain change وطلب التنفيذ الخارجي جزءًا من transaction واحدة؛ فإذا حدث rollback يختفي الاثنان، وإذا حدث commit يبقى job durable حتى لو توقف worker.

Outbox لا يعطي exactly-once عبر network. إذا قبل Provider الطلب ثم توقف worker قبل تسجيل النتيجة، قد تصبح النتيجة ambiguous. لذلك ستحتاج المرحلة المستقبلية إلى provider idempotency أو reconciliation، ولا يجوز إعادة إرسال unknown بشكل أعمى.

## C. الفرق عن EventStore

| Direction | Owner | Question |
|---|---|---|
| Inbound | EventStore | ماذا وصل؟ وهل سبق تسجيله؟ |
| Outbound | Outbox | ماذا نريد أن ننفذ بعد commit؟ |

EventStore يملك inbound dedupe. Outbox يملك durable outbound obligation. لا يوجد `IdempotencyStore` عام جديد.

## D. Migration/schema rationale

الرقم التالي الفعلي هو **000033**: `migrations/000033_outbox_lease_results.up.sql`. migration forward-only ولا تعدّل `000001–000032`.

أضافت:

| Column/constraint | Reason |
|---|---|
| `lease_token UUID` | fencing token يمنع worker قديمًا من تعديل job بعد انتقال lease |
| `result_code TEXT` | نتيجة محاولة worker الداخلية بشكل آمن، وليست provider delivery state |
| `completed_at` | وقت اكتمال محاولة worker |
| lease-token check | processing يتطلب token، وباقي الحالات تنظفه |
| completed-at check | completed يتطلب timestamp، وباقي الحالات لا تحمل timestamp قديمًا |
| business availability index | دعم claim/list داخل tenant |
| owner/lease index | دعم مراقبة leases processing |

الـschema الأصلية تحتفظ بـ`outbound_message_id` وunique `(business_id, outbound_message_id)`، إضافة إلى unique `(business_id, command_type, dedupe_key)`. لذلك لا يُسمح بأكثر من Outbox obligation لنفس logical command داخل Business.

## E. PostgreSQL implementation

التنفيذ موجود في `internal/adapters/secondary/persistence/postgres/outbox_store.go` ويستخدم `Adapter.Executor(ctx)`. لذلك إذا استُدعي داخل `Adapter.Within` فإنه يعمل في نفس transaction context مع `OutboundMessageRepository`.

`Enqueue` لا يستدعي Provider ولا يرسل شبكة. `Claim` يستخدم conditional update ويقبل pending/retryable jobs المستحقة، كما يستطيع استعادة processing job انتهى lease الخاص به. كل completion/failure/dead-letter mutation تتحقق من:

```text
status = processing
lease_owner = expected owner
lease_token = expected token
lease_expires_at > now()
```

## F. Lifecycle

```text
pending → processing → completed
pending → processing → retryable_failed → pending/processing
processing → dead_letter
 dead_letter → pending   (Requeue صريح فقط)
```

`completed` يعني أن worker أنهى محاولته وسجل النتيجة، ولا يعني أن Provider سلّم الرسالة. حالة التسليم الخارجية تبقى في `OutboundMessage`/delivery state اللاحقة.

## G. Atomic pair

اختبار التكامل ينشئ `OutboundMessage` و`OutboxEntry` داخل `Adapter.Within` واحدة. نجاح المعاملة يحفظ السجلين، وفشلها يلغي السجلين معًا. هذا هو الحد المطلوب لمنع:

```text
Outbound intent موجود بلا durable job
```

أو:

```text
Outbox job موجود بلا outbound intent
```

## H. اختبارات PostgreSQL 16

الاختبار موجود في `internal/adapters/secondary/persistence/postgres/outbox_store_integration_test.go` ويستخدم fresh PostgreSQL 16 database.

| Test area | Verified |
|---|---|
| committed atomic pair | OutboundMessage وOutboxEntry محفوظان معًا |
| rollback atomic pair | السجلان يختفيان معًا |
| logical dedupe | duplicate `(business, command_type, dedupe_key)` يرجع conflict |
| outbound link uniqueness | لا يتكرر Outbox لنفس outbound message |
| tenant isolation | cross-business Get يرجع typed not-found |
| keyset pagination | ordering وnext cursor |
| malformed cursor | typed invalid |
| first claim | processing + owner/token + attempt increment |
| active duplicate claim | لا يمنح lease ثانيًا |
| concurrent claim | 32 عاملًا؛ فائز واحد فقط |
| completion | تنظيف lease وتسجيل result/completed_at |
| stale worker fencing | worker القديم لا يستطيع completion |
| retry scheduling | next attempt enforced |
| retry due | claim بعد حلول الموعد |
| dead-letter | state نهائي بلا lease |
| explicit requeue | dead-letter يعود pending بقرار صريح |
| expired lease recovery | عامل جديد يستعيد job بعد expiry |

## I. الملفات

| Area | File |
|---|---|
| Application port | `internal/application/ports/outbox.go` |
| Forward migration | `migrations/000033_outbox_lease_results.up.sql` |
| PostgreSQL adapter | `internal/adapters/secondary/persistence/postgres/outbox_store.go` |
| Integration tests | `internal/adapters/secondary/persistence/postgres/outbox_store_integration_test.go` |
| Schema runner | `scripts/test-postgres-schema.sh` |
| Existing fixture compatibility | `tests/integration/foundation_constraints.sql` |
| Decision record | `docs/pr013-outbox-decision-ar.md` |

## J. Validation and commit

سيُعتبر PR-013 مغلقًا بعد تحقق كل البوابات التالية على آخر revision:

```text
GOTOOLCHAIN=local go test ./...
GOTOOLCHAIN=local go vet ./...
go test -tags=integration -count=1 ./internal/adapters/secondary/persistence/postgres
bash scripts/test-postgres-schema.sh
 git diff --check
HEAD == origin/main
working tree clean
```

سيُسجل SHA النهائي هنا بعد commit/push.

## K. ما تبقى

لا يزال خارج هذه الدفعة:

```text
Provider Simulator
SocialAPI adapter
Chatwoot adapter
AI runtime
Bootstrap production dependency injection
Actual outbound network execution
```

بعد إغلاق PR-013، الدفعة التالية المعتمدة هي **PR-014 — Bootstrap wiring** فقط إذا كان ترتيب المشروع الحالي يتطلب تركيب dependencies قبل simulator. لا يجوز اعتبار Outbox نفسه عاملًا يعمل في production قبل wiring والworker runtime اللاحقين.
