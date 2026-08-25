# PR-026 — SocialAPI Inbound Communication Runtime

## الحالة

**منفذ ومختبر محليًا وعلى PostgreSQL 16 حقيقية.** هذا PR يغلق الفجوة الأولى التي ظهرت عند تدقيق Communication Runtime: كان SocialAPI webhook يتوقف بعد normalization وEvent Ledger، بينما لم يكن ينشئ سجلات Mujeeb المملوكة للعميل والمحادثة والرسالة.

هذا PR **ليس E2E خارجيًا**، ولا يشغّل SocialAPI live، ولا ينشئ Chatwoot network message، ولا يرسل ردًا إلى Facebook/Instagram/WhatsApp. جميع external provider tests في هذه الدفعة fake/contract، بينما PostgreSQL integration حقيقية ومعزولة.

## A. الحالة الفعلية قبل PR-026

المسار السابق لـSocialAPI كان:

```text
SocialAPI signed webhook
  → VerifyWebhook
  → NormalizeWebhook
  → raw payload reference
  → resolve ChannelConnection by provider account
  → InboundEventStore.RecordIfAbsent
  → inbound_event_ledger
```

ولم يكن بعد ذلك `ProviderInboundStore` موصولًا. لذلك لم يكن SocialAPI inbound ينشئ Customer أو Conversation أو provider ConversationReference أو CommunicationMessage.

في المقابل، كان Chatwoot inbound يملك materializer مستقلًا ومثبتًا على PostgreSQL، ولذلك لم يُعد PR-026 بناء Chatwoot path.

## B. ما نُفذ

أضيف عقد Application typed مستقل:

```text
ProviderInboundStore.Materialize(ProviderInboundDraft)
```

ويملك هذا العقد فقط materialization داخل Mujeeb. لا توجد فيه Chatwoot DTOs ولا SocialAPI raw DTOs ولا network calls.

التدفق الحالي أصبح:

```text
Verified SocialAPI webhook
  → SocialAPI normalization إلى channel.InboundEvent
  → EventStore RecordIfAbsent
  → PostgreSQL RawPayloadStore
  → ChannelConnection tenant/account resolution
  → ProviderInboundStore.Materialize
  → external identity → Customer
  → provider conversation reference → Conversation
  → CommunicationMessage
  → inbound ledger processed
```

وعند إعادة نفس callback، يستخدم المسار `record.ID` الموجود في Event Ledger، ويتحقق من تطابق tenant وconnection وprovider وevent/account/conversation/user/message references قبل إعادة النتيجة duplicate.

## C. normalization contract

صحّح PR-026 نقطة توافق مهمة: أسماء SocialAPI الخارجية مثل `dm.received` لا تُخزن مباشرة في `inbound_event_ledger.event_type` لأن schema الداخلية تقبل enum مختلفًا. أصبح التحويل كالتالي:

| SocialAPI event | Mujeeb event type | النتيجة |
|---|---|---|
| `dm.received`, `dm.referral`, `dm.postback` | `interaction_received` | رسالة واردة؛ direction=inbound وorigin=customer |
| `comment.received`, `mention.received`, `review.received` | `interaction_received` | تفاعل وارد |
| `interaction.updated`, `message.updated` | `interaction_updated` | lifecycle/update event |
| `dm.status.delivered`, `dm.status.sent`, `dm.status.failed` | `delivery_status_changed` | لا تُعامل كرسالة واردة جديدة |
| `conversation.updated` | `conversation_updated` | lifecycle event |
| account/page lifecycle | `account_status_changed` | يُغلق كـignored materialization عند resolution |

هذا يمنع إدخال provider event names غير المقبولة في Mujeeb schema.

## D. ownership والعلاقات

Mujeeb هو مالك Customer وConversation وConversationReference وCommunicationMessage وEvent Ledger. `external_identities` هو سجل الربط provider-side الموجود أصلًا في schema، ويستخدمه PR-026 لربط `(connection_id, external_user_id)` بالـCustomer داخل نفس Business.

يُنشأ provider ConversationReference بالقيم التالية:

```text
system        = provider
provider_ref  = socialapi
resource_type = conversation
resource_id   = SocialAPI provider conversation ID
connection_id = Mujeeb ChannelConnection ID
is_current    = true
mapping_status= active
```

لا يُساوى SocialAPI conversation ID بأي Chatwoot ID، ولا تُستخدم `chatwoot_contact_links` لمسار SocialAPI.

CommunicationMessage هو السجل الموحّد لعرض الرسالة داخل Mujeeb:

```text
direction          = inbound
origin             = customer
transport          = provider
provider_message_id= SocialAPI provider message ID
content_type       = text أو unknown
text_content       = النص إن وجد
content_reference  = raw payload opaque reference
inbound_event_id   = Event Ledger ID
```

لا يدمج PR-026 هذا السجل مع `OutboundMessage` ولا ينشئ Outbox؛ outbound remains a separate intent/delivery path.

## E. raw payload persistence

كان عقد `RawPayloadStore` موجودًا دون PostgreSQL implementation موصول في runtime. أضاف PR-026 `inbound_webhook_payloads` في migration `000038`، ويحفظ:

| الحقل | الغرض |
|---|---|
| `provider_ref` | فصل المصدر الخارجي |
| `delivery_id` | idempotent delivery boundary |
| `payload` | exact verified raw bytes |
| `payload_hash` | سلامة المحتوى |
| `created_at` | retention/operational timestamp |

الـreference المعاد إلى Event Ledger opaque من نوع `db://inbound_webhook_payloads/{id}`. عند إعادة delivery بنفس bytes يعاد نفس reference، وعند إعادة نفس delivery بمحتوى مختلف ينتج conflict.

## F. transaction وfailure behavior

يحصل Event Ledger insertion أولًا عبر EventStore، ثم ينفذ ProviderInboundStore materialization داخل transaction PostgreSQL واحدة. لا توجد network calls داخل هذه transaction.

إذا فشل إنشاء identity أو customer أو conversation أو reference أو message، تُعمل rollback لكل records التي أنشأتها تلك العملية، ويبقى الحدث قابلًا لإعادة المعالجة وفق حالة ledger الموجودة بدل إعلان نجاح كاذب.

الـmaterializer يرفض:

- tenant أو connection مختلفين عن ledger event.
- provider/account/event/conversation/user/message reference غير المطابق للـledger.
- provider conversation المرتبط مسبقًا بCustomer مختلف.
- duplicate current mapping غير المتسق.

أحداث SocialAPI lifecycle التي لا تحمل conversation/message/user مكتملًا لا تنشئ Conversation؛ تُعلّم `processed` بنتيجة `socialapi_ignored_event` بعد resolution.

## G. الاختبارات المنفذة

| الاختبار | نوع الدليل | النتيجة |
|---|---|---|
| SocialAPI HMAC وnormalization إلى `interaction_received` | unit/contract | PASS |
| SocialAPI status event لا يتحول إلى inbound text | unit/contract | PASS |
| SocialAPI service يمرر provider draft إلى materializer | application unit fake | PASS |
| Raw payload first insert/replay | PostgreSQL integration | PASS |
| Raw payload reuse مع bytes مختلفة | PostgreSQL integration | PASS؛ conflict |
| Provider inbound first materialization | PostgreSQL integration | PASS |
| Customer وexternal identity creation | PostgreSQL integration | PASS |
| Conversation وprovider reference creation | PostgreSQL integration | PASS |
| CommunicationMessage provider projection | PostgreSQL integration | PASS |
| Event Ledger processed result | PostgreSQL integration | PASS |
| Duplicate callback لا ينشئ records إضافية | PostgreSQL integration | PASS |
| tenant/connection mismatch قبل duplicate return | PostgreSQL integration | PASS |
| rollback بعد constraint failure | PostgreSQL integration | PASS |
| full PostgreSQL adapter suite | PostgreSQL 16 temporary isolated DB | PASS |
| `go test ./...` | local | PASS |
| `go vet ./...` | local | PASS |
| OpenAPI generation/drift | local | PASS |
| schema runner first/second run | PostgreSQL 16 | PASS؛ applied=38 ثم 0 |
| runtime secret scan | source excluding tests/docs | PASS |
| SocialAPI live | external live | NOT RUN |
| Chatwoot live outbound mirror | external live | NOT RUN |
| Facebook/Instagram/WhatsApp real message | external live | NOT RUN |
| full E2E | external live | NOT RUN |

## H. الملفات المتغيرة

```text
migrations/000038_inbound_webhook_payloads.up.sql
internal/application/ports/provider_inbound.go
internal/adapters/secondary/persistence/postgres/provider_inbound_store.go
internal/adapters/secondary/persistence/postgres/provider_inbound_store_integration_test.go
internal/adapters/secondary/persistence/postgres/raw_payload_store.go
internal/application/services/webhook_ingestion.go
internal/application/services/webhook_ingestion_test.go
internal/adapters/secondary/providers/socialapi/client.go
internal/adapters/secondary/providers/socialapi/client_test.go
internal/bootstrap/api.go
scripts/test-postgres-schema.sh
docs/work-communication-runtime-audit-ar.md
docs/pr026-socialapi-inbound-runtime-ar.md
```

## I. ما لم يُنفذ في PR-026

لم يُنفذ Chatwoot mirror من SocialAPI inbound. السبب architectural مقصود: Chatwoot network API call يجب أن تكون خارج transaction ومعها delivery/idempotency policy مستقلة، ولا يجوز إخفاؤها داخل provider materializer.

لم يُضف PR-026 AutoReply من SocialAPI inbound مباشرة. AutoReply الحالي مثبت في مسار Chatwoot inbound، بينما provider inbound materialization يصل هنا إلى Mujeeb records فقط. ربط SocialAPI inbound بـChatwoot mirror أو AI/Outbox يحتاج PR مستقلًا بعد تحديد workspace send contract ومنع echo/replay.

لم تُجرَ أي مكالمة live إلى SocialAPI أو Chatwoot، ولم تُنشأ حسابات خارجية أو رسائل حقيقية.

## J. قرار الـPR التالي

بعد إغلاق PR-026، أول فجوة المتبقية في Communication Runtime ليست E2E عامة. الفجوة التالية المحددة هي:

```text
SocialAPI inbound Mujeeb materialization
  → Chatwoot mirror creation/update خارج DB transaction
  → durable mirror delivery obligation/idempotency
  → callback/reference reconciliation
```

لكن هذا ليس جزءًا من PR-026، ولا ينبغي فتحه قبل اعتماد عقد Chatwoot mirror network وoutcome-unknown behavior. Outbox provider send الحالي مسار مستقل ومثبت على مستوى claim/resolve/send boundary، ولا يُعاد بناؤه هنا.
