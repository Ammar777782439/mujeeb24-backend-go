# PR-009 — قرار CommunicationMessage وMessage Timeline

## حالة القرار

**منفذ ومرفوع إلى private GitHub بعد اجتياز اختبارات PostgreSQL 16 والتحقق النهائي.** هذه الوثيقة تميّز بين ما هو مثبت من الكود والمخطط، وما هو قرار تصميم، وما لم يُنفذ.

## A. Contract evidence

| الادعاء | نوع الدليل | النتيجة المثبتة |
|---|---|---|
| `CommunicationMessageReference` كيان مستقل | عقد Domain communication، القسم 4.4 | ليس `ConversationReference` وليس `OutboundMessage` وليس نسخة Chatwoot |
| وجود use case للقراءة | `internal/application/queries` | `ListConversationMessagesQuery` وhandler alias موجودان |
| وجود Dashboard endpoint | HTTP Dashboard contract وHuma registration | `GET /businesses/{business_id}/conversations/{id}/messages` يمر عبر `ListConversationMessages` |
| وجود runtime mapping | `handlers/query_facades.go` و`MessageView` | HTTP scope → typed query → application service → DTO projection |
| غياب الجدول في baseline | migrations `000001–000027` | لم يكن هناك `communication_messages` أو message timeline مستقل |
| الفصل عن Provider/Chatwoot | Domain contract | السجل الداخلي يحتفظ بالمراجع اللازمة للربط والموثوقية، لا ينسخ نماذج الأنظمة الخارجية |

## B. لماذا هو مطلوب

`ConversationReference` يثبت mapping للمحادثة، و`InboundEventLedger` يثبت قبول الحدث ومعالجته، و`OutboundMessage` يثبت نية الإرسال وdelivery lifecycle. لا يملك أي واحد منها timeline موحدة تُعرض في Dashboard وتربط رسالة inbound أو outbound بالمحادثة وبالمراجع الخارجية. لذلك كانت البنية السابقة ناقصة فعليًا رغم اكتمال جداول foundation.

الهدف من `communication_messages` هو **سجل الرسالة الموحّد للقراءة والربط** داخل Business tenant. لا يحل محل EventStore في inbound dedupe، ولا يحل محل Outbox أو OutboundMessage في ضمان الإرسال، ولا يدخل في Chatwoot UI model.

## C. Final schema وسبب كل عنصر

| العنصر | النوع/القيد | السبب |
|---|---|---|
| `id` | UUID PK، مع `UNIQUE (business_id,id)` | هوية السجل ومنع الاستناد إلى ID خارج tenant وحده |
| `business_id` | UUID NOT NULL، FK إلى `businesses` | الملكية وtenant boundary |
| `conversation_reference_id` | UUID NOT NULL، composite FK `(business_id,id)` | كل رسالة مرتبطة بمرجع محادثة من نفس Business |
| `inbound_event_id` | UUID اختياري، composite FK | ربط اختياري بحدث inbound؛ لا يفرض one-to-one لأن event قد يمثل lifecycle/interaction أكثر تعقيدًا |
| `outbound_message_id` | UUID اختياري، composite FK | ربط اختياري بنية الإرسال؛ لا يدمج lifecycle ولا يفرض one-to-one غير مثبت في العقد |
| `direction` | `inbound | outbound` | اتجاه الرسالة في القناة |
| `origin` | `customer | ai | human | automation | system` | مصدر الرسالة الداخلي |
| `transport` | `provider | chatwoot | mujeeb` | مسار النقل ومنع loop عند mirror |
| `provider_message_id` | اختياري مع non-empty check وunique partial index `(business_id,transport,provider_message_id)` | dedupe/mapping مرجع Provider داخل tenant؛ uniqueness مقيدة بوجود المرجع |
| `chatwoot_message_id` | اختياري مع non-empty check وunique partial index `(business_id,transport,chatwoot_message_id)` | mapping مرجع Chatwoot داخل tenant دون نسخ Chatwoot schema |
| `content_type` | `text | image | video | audio | file | mixed | unknown` | عقد V1 صريح للمحتوى |
| `text_content` | اختياري، إلزامي عندما `content_type=text` | يدعم text-only V1 دون `map[string]any` |
| `content_reference` | NOT NULL، non-empty | مرجع لمحتوى/مرفق خارج السجل أو canonical content reference |
| `occurred_at` | TIMESTAMPTZ NOT NULL | ترتيب الحدث الحقيقي |
| `created_at` | TIMESTAMPTZ NOT NULL | وقت إنشاء سجل Mujeeb وحل التعادل |

الفهارس هي timeline index على `(business_id, conversation_reference_id, occurred_at DESC, created_at DESC, id DESC)`، وفهرسا inbound/outbound links للربط اللاحق. لا يوجد عمود `status` في V1.

### قرار status

لا نكرر OutboundMessage delivery lifecycle في CommunicationMessage. عند القراءة يعيد repository status projection كما يلي:

| حالة السجل | status المعروض | المعنى |
|---|---|---|
| inbound بلا OutboundMessage | `received` | سُجلت رسالة واردة في timeline، ولا تعني أن ردًا أُرسل |
| outbound بلا OutboundMessage | `recorded` | سجل نقل/مرآة محفوظ، ولا تعني نجاح التسليم |
| مرتبط بـ`outbound_message_id` | حالة OutboundMessage الحالية | `pending | sending | accepted | sent | delivered | read | failed | unknown` |

`received` و`recorded` حالتان عرضيتان للـtimeline وليستا حالات OutboundMessage. لم نضف عمود status أو constraint له في migration.

### قرارات القيود المقصودة

لا توجد unique partial constraint على `outbound_message_id` أو `inbound_event_id`؛ عقد Domain يثبت optional links ولا يثبت one-to-one. إضافة one-to-one الآن قد ترفض رسالة mirror أو أكثر من سجل دلالي لنفس الحدث. يمكن إضافة forward migration لاحقًا فقط إذا أثبت EventStore/Application invariant ذلك.

الـforeign keys المركبة ترفض cross-business references. ولأن FK inbound يحتاج unique target مناسبًا، أضافت `000028` أولًا `UNIQUE (business_id,id)` إلى `inbound_event_ledger` ثم أنشأت FK communication message. هذا تصحيح forward-only ولا يعدّل migration قديمة.

## D. Migration number

الرقم الفعلي التالي في المستودع كان `000028`. الملف هو `migrations/000028_communication_messages.up.sql`. لا توجد تعديلات على `000001–000027`، وschema runner يثبت `applied=28` في التشغيل الأول، ثم `applied=0` في التشغيل الثاني، و`schema_migrations=28`.

## E. Application port

`internal/application/ports/message_repository.go` يعرّف record contract typed:

- `CommunicationMessageRecord` للقراءة، مع Business/Conversation/reference IDs، inbound/outbound links، direction/origin/transport، Provider/Chatwoot references، content fields، timestamps، وstatus projection.
- `CommunicationMessageDraft` للإدخال typed عبر `Record`.
- `MessagePage` مع `Items`, `NextCursor`, و`HasMore`.
- `MessageRepository.Record` و`ListByConversation` فقط؛ لا CRUD عام غير مطلوب من use case الحالي.

لا يعبر pgx أو SQL rows إلى Application، ولا يستخدم `map[string]any` بدلًا من record contract.

## F. PostgreSQL repository

`internal/adapters/secondary/persistence/postgres/message_repository.go` ينفذ `MessageRepository` فوق `SQLExecutor`. `Record` transaction-aware؛ إذا كان context يحمل transaction يستخدمها، وإذا لم يحملها يستخدم pool كما يفعل باقي repositories. `ListByConversation` يفحص وجود المحادثة داخل `(business_id,id)` أولًا، ويعيد typed `RepositoryNotFound` للمحادثة المفقودة أو التابعة لـBusiness آخر، ثم ينفذ query tenant-scoped.

القراءة تستخدم keyset cursor opaque مشفرًا بـRaw URL Base64 لمجموعة `(occurred_at, created_at, id)`. الترتيب descending deterministic ولا يستخدم OFFSET. أخطاء malformed cursor تعود `RepositoryInvalid`.

## G. ListConversationMessages mapping

```text
HTTP GET /businesses/{business_id}/conversations/{id}/messages
  → Huma contract + dto.Message
  → handlers.requireScope
  → queries.ListConversationMessagesQuery
  → services.MessageQueryService
  → ports.MessageRepository.ListByConversation
  → postgres.MessageRepository
  → commands.MessageView
  → dto.Message
```

`MessageQueryService` يحول `TextContent` الاختياري إلى text projection، وينقل `Direction`, `Origin`, `Status`, Provider/Chatwoot references، `OccurredAt`, و`CreatedAt`. لا يدعي هذا أن `cmd/api` أصبح موصولًا بقاعدة البيانات؛ bootstrap الحالي ما زال يبني `handlers.Dependencies{}`، ولذلك production dependency injection مؤجل.

## H. الاختبارات المثبتة فعليًا

تم تشغيل التالي بنجاح بعد آخر تعديل:

| الاختبار | النتيجة |
|---|---|
| `GOTOOLCHAIN=local go test ./...` | PASS |
| `GOTOOLCHAIN=local go vet ./...` | PASS |
| `scripts/test-postgres-schema.sh` على PostgreSQL 16 | PASS؛ foundation/full constraints، runner `28 ثم 0` |
| `POSTGRES_TEST_DSN=... go test -tags=integration -count=1 ./internal/adapters/secondary/persistence/postgres` | PASS على PostgreSQL 16 Docker |

Integration يغطي migration، typed insert/read عبر `Record`، conversation scope، cross-tenant typed not-found، missing conversation not-found، ordering، opaque keyset pagination، malformed cursor، tenant-scoped provider references، outbound status projection، text content fixture، وcommit/rollback لسجلات CommunicationMessage داخل TransactionManager، إضافة إلى application service mapping.

## I. الملفات المتغيرة

- `migrations/000028_communication_messages.up.sql`
- `internal/application/ports/message_repository.go`
- `internal/application/services/message_queries.go`
- `internal/application/commands/views.go`
- `internal/adapters/secondary/persistence/postgres/message_repository.go`
- `internal/adapters/secondary/persistence/postgres/business_repository_integration_test.go`
- `internal/adapters/primary/http/dto/types.go`
- `internal/adapters/primary/http/handlers/query_facades.go`
- `contracts/domain_channel_identity_communication_contract_ar.md`
- `scripts/test-postgres-schema.sh`
- `docs/pr009-communication-message-decision-ar.md`
- وثائق PR-009 والحالة العامة ستُحدّث في نفس commit قبل الرفع.

## J. Commit SHA

Commit المنفذ والمرفوع هو `207f1b2379f093fe0ec8b8c8a0847ed634113710` (`feat: add communication message timeline persistence`). تم التحقق من تطابق `HEAD` و`origin/main` على هذا SHA، مع بقاء PR-009 مفتوحًا وظيفيًا لما تبقى من Persistence/Reliability.

## K. ما تبقى من PR-009

PR-009 **ما زال مفتوحًا**. أُنجزت الآن قاعدة PostgreSQL pool/transactions، repository foundation الأساسية، وCommunicationMessage timeline persistence. المتبقي هو إكمال repository surfaces الضرورية الأخرى لـCatalog/Sales/AI/Audit حسب Use Cases، ثم EventStore مع atomic inbound dedupe، ثم Outbox/obligation persistence، ثم bootstrap dependency wiring والاختبارات الخاصة بها. لا يبدأ SocialAPI أو Chatwoot أو AI runtime قبل إكمال Reliability foundation المطلوبة.
