# PR-009 — Repository Surface Matrix

## الهدف

لا نبني عشرات methods على كل جدول لمجرد أن الجدول موجود. نضيف فقط ما تحتاجه Application Use Cases الحالية، ونثبت كل مجموعة باختبار PostgreSQL حقيقي قبل الانتقال للمجموعة التالية.

## الأولوية الحالية

| المجموعة | Use Cases المرتبطة | الجداول الأساسية | الحالة |
|---|---|---|---|
| Foundation | قراءة tenant/business scope | `businesses` | منفذ |
| Identity/Communication | عرض العملاء والمحادثات، قراءة/إنشاء outbound، وعرض message timeline | `customers`, `conversations`, `conversation_references`, `communication_messages`, `outbound_messages` | Business/Customer/Conversation/Connection/Reference/Outbound وCommunicationMessage foundation منفذة ومثبتة بـPostgreSQL 16 |
| Channels | عرض connection وcapabilities | `channel_connections`, `channel_connection_capabilities` | `ChannelConnection.GetByID` و`ChannelCapability.ListByConnection` منفذان ومثبتان بـPostgreSQL 16؛ لا CRUD كتابة غير مطلوب |
| Catalog | قراءة وكتابة catalogs/items/offers/variants/schemas عبر Use Cases الحالية | `catalogs`, `attribute_schemas`, `attribute_definitions`, `catalog_items`, `offers`, `variants` + `resource_version` في 000029 | **Read/write surface منفذ ومثبت بـPostgreSQL 16**؛ bootstrap wiring وsemantic attribute validation مؤجلان |
| Sales | Leads وtransactions وreviews/order lines | `leads`, `lead_attributions`, `lead_scores`, `commercial_transactions`, `transaction_reviews`, `transaction_confirmations`, `order_lines` | مؤجلة بعد Catalog |
| AI/Audit | قراءة decisions وتسجيل/عرض audit | `ai_decisions`, `audit_events` | مؤجلة بعد Sales |
| Reliability | atomic inbound dedupe وoutbox | `inbound_event_ledger`, `outbox_entries` | Go implementation مؤجلة؛ EventStore يملك inbound idempotency |

## Communication surface المنفذ

تم تنفيذ ports وrepositories للـConversationReference وOutboundMessage فوق SQLExecutor. `CreatePending` يفرض الحقول اللازمة من schema ويترك unique constraint `(provider_ref, connection_id, provider_idempotency_key)` مصدر conflict semantics؛ لا ينفذ network call ولا يرسل Provider.

تمت إضافة `MessageRepository` بسطح محدود ومطلوب فعليًا:

| Method | الغرض | السلوك |
|---|---|---|
| `Record` | إدخال CommunicationMessage typed | transaction-aware، يعيد record projection وstatus projection |
| `ListByConversation` | قراءة Dashboard timeline | tenant-scoped، يتحقق من وجود المحادثة، keyset pagination، cursor opaque |

`CommunicationMessage` ليس بديلًا عن `ConversationReference` أو `OutboundMessage` أو `InboundEventLedger`. لا يوجد CRUD عام، ولا one-to-one unique غير مثبت بين message record وinbound/outbound links.

## Catalog read surface

تم تنفيذ `CatalogRepository` وservices typed لقراءات Catalog الحالية فقط: `List/GetCatalog`, `List/GetCatalogItem`, `ListOffers`, `ListVariants`, و`List/GetAttributeSchema`. القوائم تستخدم tenant-scoped parent checks وopaque keyset cursors؛ `AttributeSchema` يعيد definitions مرتبة. attributes تعاد كـraw JSON bytes داخل Application port ولا تتحول إلى `map[string]any` record.

تم تنفيذ Catalog read/write surface الحالية. `resource_version` أضيفت في 000029 وتُستخدم في conditional updates، وOffer `AmountMinor` يُحوّل إلى `NUMERIC(20,4)` major units داخل repository مع بقاء exact decimal text في records. لا تزال semantic validation للـattributes وbootstrap wiring خارج هذه الدفعة.

## Channel Capabilities surface

`GetConnectionCapabilities` هو use case القراءة الوحيد المطلوب حاليًا. `ChannelCapabilityRepository.ListByConnection` يعيد `Name`, `Enabled`, `CheckedAt`, و`EvidenceSource` بترتيب capability تصاعديًا، ويجعل connection existence والـtenant scope شرطًا سابقًا؛ الاتصال المفقود أو التابع لـBusiness آخر يعيد `RepositoryNotFound`. لا تُنشأ methods لتحديث capability من Dashboard، لأن capabilities ناتجة عن provider health/contract checks وتبقى الكتابة في مسار integration لاحق.

## Status projection

لا يملك `communication_messages` عمود `status` في V1. الرسالة inbound غير المرتبطة بـOutboundMessage تعرض `received`، والرسالة outbound غير المرتبطة تعرض `recorded`، والرسالة المرتبطة بـ`outbound_message_id` تعرض حالة OutboundMessage الحالية: `pending | sending | accepted | sent | delivered | read | failed | unknown`. هذه projection لا تدّعي نجاح التسليم.

## Tenant وnot-found policy

كل method تستقبل `context.Context` وbusiness scope حيث يلزم. `ListByConversation` يتحقق أولًا من `(business_id, conversation_id)`؛ لذلك تعود المحادثة المفقودة أو cross-tenant كـtyped `RepositoryNotFound` ولا تُعاد أي سجلات من tenant آخر. malformed cursor وinvalid limits تعود `RepositoryInvalid`.

## قواعد التنفيذ

يجب أن يستخدم أي query يقرأ سجلًا tenant-scoped شرط `business_id` مع record ID أو cursor المناسب. يجب ألا يعتمد repository على ID وحده عندما يسمح schema بحدود composite. ويجب أن تعمل methods داخل TransactionManager عندما يستدعيها Use Case داخل transaction، دون بدء nested transaction مخفية.

كل خطأ يعبر من adapter إلى Application عبر typed repository error، مع تحويل واضح لـnot-found وconstraint/conflict وinvalid input. لا يُستخدم `map[string]any` كبديل عن record contract؛ يُسمح فقط بالـraw JSON في الحقول المرنة المعتمدة.

## معيار قبول كل مجموعة

لا تنتقل المجموعة إلى التالية قبل نجاح unit tests وintegration test على PostgreSQL حقيقي، مع تطبيق migrations، وقراءة صحيحة، وnot-found، وcross-tenant rejection، وسلوك transaction عند الحاجة. CommunicationMessage وChannel Capabilities وCatalog read/write surface حققت معيارها في PostgreSQL 16. Catalog لا يُعتبر مربوطًا runtime حتى يكتمل bootstrap wiring، لكن repository/application surface الحالي مثبت. CommunicationMessage غطت ordering/pagination وconstraints وApplication mapping، وChannel Capabilities غطت read/order وchecked_at/evidence وtenant not-found وtransaction commit/rollback وApplication mapping. بعد تثبيت surfaces المتبقية في Catalog/Sales/AI/Audit فقط نبدأ EventStore ثم inbound dedupe ثم Outbox. لا يبدأ Provider runtime قبل إكمال Reliability foundation.
