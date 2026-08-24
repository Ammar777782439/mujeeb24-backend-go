# PR-009 — PostgreSQL Adapter وTransactionManager

## نطاق هذه الدفعة

هذه الدفعة تبدأ تنفيذ Persistence الحقيقي دون القفز إلى Repositories أو Event Ledger أو Idempotency/Outbox implementations. الحزمة `internal/adapters/secondary/persistence/postgres` تملك pool lifecycle و`Within` transaction boundary، بينما Application يعتمد على `application/ports.TransactionManager` فقط.

```text
Application
    ↓ application/ports.TransactionManager
PostgreSQL Adapter
    ↓
pgxpool.Pool / pgx transaction
```

لا يستورد Domain أو Application أي نوع من pgx. لا يوجد SQL داخل HTTP handlers، ولا اتصال SocialAPI أو Chatwoot أو AI في هذه المرحلة.

## Pool

`postgres.Open` يتحقق من database URL، يقرأ pgxpool config، يطبق `PoolConfig`، وينفذ `Ping` قبل إعادة adapter جاهز. الإعدادات الافتراضية الحالية هي `MaxConns=10` و`MinConns=1` وconnection lifetime ساعة وidle time ثلاثون دقيقة وhealth check دقيقة وconnect timeout خمس ثوانٍ. يمكن override القيم الموجبة من bootstrap لاحقًا.

`Close` مسؤول عن إغلاق pool، و`Ping` يعيد `ErrPoolClosed` إذا لم يكن adapter موصولًا. لا تحتوي هذه الدفعة على global pool أو singleton مخفي.

## TransactionManager

`Adapter.Within(ctx, fn)` يطبق القواعد التالية:

1. يرفض context الملغى قبل `BEGIN`.
2. يبدأ transaction واحدًا فقط.
3. يحقن transaction في callback context.
4. إذا نجح callback ينفذ `COMMIT`.
5. إذا أعاد callback خطأ ينفذ `ROLLBACK` ويعيد خطأ callback.
6. يرفض nested transactions عبر `ErrNestedTransaction` بدل إنشاء transaction مستقلة مخفية.
7. لا ينفذ network calls أو provider calls؛ callback يملك وحدة العمل Application/Persistence.

Repositories اللاحقة ستستخدم transaction context عبر adapter contract مناسب، ولن تصل إلى pgx من Application مباشرة.

## الاختبارات

تغطي اختبارات الوحدة نجاح commit، callback failure وrollback، nested transaction rejection، cancellation قبل begin، cancellation داخل callback، وnil-pool/invalid URL lifecycle behavior. لا تدعي هذه الاختبارات نجاح اتصال PostgreSQL حقيقي؛ integration test بقاعدة PostgreSQL منفصلة يأتي مع Repository foundation.

## ما لم يُنفذ

لم تُنفذ Repositories أو TransactionManager bootstrap wiring في `cmd/api`، ولم تُنفذ EventStore أو IdempotencyStore أو OutboxStore. لا نضيف Schema أو migrations جديدة في هذه الدفعة، ولا نبدأ SocialAPI أو Chatwoot أو AI.

## معيار الانتقال التالي

بعد مراجعة هذه boundary واختبارها، تكون الدفعة التالية هي Repository foundation فوق نفس TransactionManager، ثم EventStore وinbound dedupe، ثم Outbox؛ مع الحفاظ على قاعدة أن inbound EventStore يملك idempotency الذرية ولا نضيف IdempotencyStore عامًا موازيًا بلا use case.
