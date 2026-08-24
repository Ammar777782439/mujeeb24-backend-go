# PR-009 — PostgreSQL Adapter وTransactionManager

## نطاق هذه الدفعة

هذه الدفعة تثبت Persistence foundation في PostgreSQL وتضيف CommunicationMessage timeline وCatalog read/write persistence فوقها. الحزمة `internal/adapters/secondary/persistence/postgres` تملك pool lifecycle و`Within` transaction boundary وSQLExecutor وrepositories typed، بينما Application يعتمد على `application/ports` فقط.

```text
Application
    ↓ application/ports
PostgreSQL repositories
    ↓ SQLExecutor
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

`SQLExecutor` يختار transaction من context عندما تكون موجودة، وإلا يستخدم pool. لذلك `MessageRepository.Record` يمكنه العمل داخل `Within` في commit أو rollback دون معرفة Application بتفاصيل pgx.

## Repository foundation وCommunicationMessage

الـfoundation المنفذة تشمل Business/Customer/Conversation/ChannelConnection reads، ConversationReference current read، وOutboundMessage `CreatePending/GetByID`. أضيفت كذلك `ChannelCapabilityRepository.ListByConnection` لقراءة capabilities الخاصة بـConnection من نفس Business. وأضيفت `MessageRepository` بسطح محدود:

- `ChannelCapabilityRepository.ListByConnection(ctx, businessID, connectionID)` لقراءة `Name`, `Enabled`, `CheckedAt`, و`EvidenceSource` بترتيب deterministic، مع typed not-found للـconnection المفقود أو cross-tenant.
- `MessageRepository.Record(ctx, CommunicationMessageDraft)` لإدخال typed في `communication_messages` وإعادة record projection.
- `MessageRepository.ListByConversation(ctx, businessID, conversationID, limit, cursor)` لقراءة timeline.

`communication_messages` أُضيفت في migration forward-only `000028`، وCatalog resource versions في `000029`؛ لا تعدّل migrations `000001–000029` بعد ذلك. كل العلاقات الحساسة تستخدم composite tenant FKs. الفهارس تدعم timeline keyset على `(occurred_at DESC, created_at DESC, id DESC)`، وCatalog lists على `(updated_at DESC, id DESC)`، ومراجع Provider/Chatwoot.

`CatalogRepository` ينفذ كذلك create/update للـCatalog وItem وOffer وVariant، وcreate schema version مع definitions. كل writes تمر من Application عبر TransactionManager؛ resource version conditional update يزيد العداد atomically، وOffer `AmountMinor` يتحول إلى `NUMERIC(20,4)` major units بقسمة 100.

`CommunicationMessage` لا يملك status مستقلًا في V1. repository يعرض `received` للـinbound غير المرتبط، `recorded` للـoutbound غير المرتبط، أو OutboundMessage lifecycle عند وجود `outbound_message_id`. هذا لا يخلط message timeline مع outbound delivery truth.

`ListByConversation` يفحص `(business_id, conversation_id)` قبل القراءة. المحادثة غير الموجودة أو التابعة لـBusiness آخر تعود `RepositoryNotFound` typed، ولا تُعاد صفحة فارغة مضللة أو بيانات cross-tenant. malformed cursor وinvalid input يعودان `RepositoryInvalid`.

## الاختبارات

تم تشغيلها فعليًا بعد آخر تعديل في دفعة Channel Capabilities:

| الأمر | النتيجة |
|---|---|
| `GOTOOLCHAIN=local go test ./...` | PASS |
| `GOTOOLCHAIN=local go vet ./...` | PASS |
| `scripts/test-postgres-schema.sh` | PASS على PostgreSQL 16؛ foundation/full constraints وrunner `applied=29` ثم `applied=0` |
| `POSTGRES_TEST_DSN=... GOTOOLCHAIN=local go test -tags=integration -count=1 ./internal/adapters/secondary/persistence/postgres` | PASS على PostgreSQL 16 Docker |

Integration test يطبق migrations، ويثبت CommunicationMessage عبر `Record`/timeline، وChannel Capabilities عبر read/order و`CheckedAt`/`EvidenceSource` وcross-tenant typed not-found وApplication mapping، وCatalog عبر read filters/keyset وJSON/decimal projection وcreate/update وresource version 1→2 وstale rejection وschema definitions وtransaction commit/rollback. ويظل مثبتًا أيضًا malformed cursor وcontent/reference constraints وoutbound status projection لجزء CommunicationMessage.

## ما لم يُنفذ

لم تُنفذ Repositories الخاصة بـSales/AI/Audit، ولا EventStore أو atomic inbound dedupe أو OutboxStore، ولا bootstrap dependency wiring الفعلي في `cmd/api`؛ ما زال `handlers.Dependencies{}` الافتراضي غير موصول بـPostgreSQL runtime. Channel capabilities نفسها لا تملك Dashboard write path؛ تحديثها يبقى ضمن integration/provider health path لاحق. لا يبدأ SocialAPI أو Chatwoot أو AI runtime قبل اكتمال Reliability foundation.

## معيار الانتقال التالي

الخطوة التالية داخل PR-009 هي تنفيذ Sales repositories الضرورية فوق نفس TransactionManager، مع إبقاء CommunicationMessage وCatalog كـpersistence surfaces مثبتة. بعد ذلك يبدأ EventStore الذي يملك inbound idempotency الذرية، ثم Outbox؛ لا نضيف `IdempotencyStore` عامًا موازيًا بلا use case.
