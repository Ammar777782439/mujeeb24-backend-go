# PR-021 — Chatwoot Inbound → AutoReply Orchestration

**الحالة:** منفذ ومختبر محليًا. لا يوجد Chatwoot live callback في هذه الدفعة، ولا SocialAPI live.

## العقد المعتمد

توثيق Chatwoot الرسمي يعرّف `message_created` ويستخدم `message_type` للتمييز بين رسائل العميل `incoming` ورسائل الوكيل `outgoing`. كما يوضح أن Webhook payload يحمل raw body موقّعًا بـ`X-Chatwoot-Signature` و`X-Chatwoot-Timestamp`.

المصادر:

- https://www.chatwoot.com/hc/user-guide/articles/1677693021-how-to-use-webhooks
- https://www.chatwoot.com/hc/user-guide/articles/1677839703-how-to-create-an-api-channel-inbox
- https://developers.chatwoot.com/api-reference/messages/create-new-message

## التغيير

أصبح Chatwoot normalizer يقبل `message_type` كنص أو رقم، ويُنتج:

- `DirectionInbound` و`OriginCustomer` للرسالة incoming.
- `DirectionOutbound` و`OriginHuman` للرسالة outgoing من user/agent.
- `DirectionOutbound` و`OriginAutomation` للرسالة outgoing من AgentBot/Captain Assistant.
- `DirectionOutbound` و`OriginSystem` للرسالة private/activity/template.
- اتجاه فارغ للنوع غير المعروف أو الغائب.

لا يعتمد القرار على sender ID فقط.

## Orchestration

```text
Chatwoot signed callback
        ↓
Verify raw-body HMAC
        ↓
Normalize message_type/direction
        ↓
Materialize Mujeeb records atomically
        ↓
if duplicate: stop
if not explicit inbound: stop
if private: stop
        ↓
ChatwootProviderReferenceResolver
        ↓
ChatwootAutoReplyBridge
        ↓
SafeAutoReplyRuntime
        ↓
AIDecision
        ↓
OutboundMessage
        ↓
Outbox
```

الـbridge يستخدم نتيجة materialization، وبالتالي يعمل على Mujeeb conversation ID، ثم يحل provider reference عبر Chatwoot account/inbox/conversation exact lookup. لا يساوي Chatwoot conversation ID بـSocialAPI conversation ID.

## Echo suppression

المسار الآمن للرسالة الخارجة هو:

```text
Mujeeb outbound → Chatwoot
Chatwoot webhook message_type=outgoing
DirectionOutbound
No AutoReply
No second Outbox
```

وإذا كانت الرسالة private أو activity أو template أو بلا `message_type` صريح، فلن تدخل AutoReply. هذا يمنع الحلقة:

```text
AI reply → Chatwoot → webhook → AI reply → ...
```

كما أن materialized duplicate لا يُرسل إلى bridge مرة ثانية.

## Feature flag

أضيف `CHATWOOT_AUTOREPLY_ENABLED` إلى `ProcessConfig`، وقيمته الافتراضية `false`. لا يُحقن bridge في API runtime إلا عندما يكون flag مفعّلًا وChatwoot webhook adapter موجودًا. هذا يمنع الردود التلقائية غير المقصودة أثناء التشغيل العادي.

عند تفعيل flag، يستخدم runtime الحالي `SafeAutoReplyRuntime` الحتمي للاختبار، وليس LLM production.

## الاختبارات

| الحالة | النتيجة |
|---|---|
| normalizer incoming string | PASS |
| normalizer numeric direction mapping | PASS — unit mapping |
| outgoing human | PASS — no AutoReply |
| outgoing AgentBot | PASS — no AutoReply |
| private message | PASS — no AutoReply |
| unknown/missing message type | PASS — unclassified/no AutoReply |
| incoming → materialization → bridge → Outbox | PASS — internal fake store/repositories |
| duplicate callback | PASS — no second AutoReply/Outbox |
| missing provider reference | PASS — blocked/no Outbox |
| tenant/reference mismatch | PASS — rejected |
| Bootstrap flag default disabled | PASS |
| full `go test ./...` | PASS |
| `go vet ./...` | PASS |
| OpenAPI drift | PASS |
| schema validation | PASS: applied-35 ثم applied-0 |
| Chatwoot live callback in this PR | NOT RUN |
| SocialAPI live | NOT RUN |

## الملفات

- `internal/domain/channel/channel.go`
- `internal/adapters/secondary/workspaces/chatwoot/client.go`
- `internal/adapters/secondary/workspaces/chatwoot/webhook_test.go`
- `internal/application/ports/chatwoot_inbound.go`
- `internal/application/services/webhook_ingestion.go`
- `internal/application/services/webhook_ingestion_test.go`
- `internal/adapters/secondary/persistence/postgres/chatwoot_inbound_store.go`
- `internal/platform/config/config.go`
- `internal/bootstrap/integrations.go`
- `internal/bootstrap/api.go`
- `internal/bootstrap/integrations_test.go`

## الحدود

Chatwoot inbound materialization السابق لم يُعد بناؤه؛ تمت إضافة direction metadata وربط orchestration بعد نجاح materialization. لا يوجد إرسال Chatwoot إضافي من هذا المسار، ولا provider network call أثناء الاختبارات. اختبار PostgreSQL المحدد لمسار Chatwoot callback يعتمد على `POSTGRES_TEST_DSN` إن أريد تشغيله منفصلًا؛ أما schema validation فيشغّل migration runner في PostgreSQL مؤقت مع applied-35 ثم applied-0.
