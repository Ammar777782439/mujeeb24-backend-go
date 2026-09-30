# مراجعة المدير — Domain Contract 03: Channel

## الحكم

تحليل الذكاء الاصطناعي **صحيح بنسبة كبيرة ومتماسك مع العقود السابقة**. وهو يغلق أول Domain يتعامل مع العالم الخارجي دون أن يسرّب SocialAPI أو Chatwoot إلى النواة.

أعتمد الاتجاه العام:

```text
Business
  → ChannelConnection
      → InboundChannelEvent
      → OutboundMessage
          → DeliveryState
```

لكن توجد تعديلات يجب تثبيتها قبل كتابة `channel_connection.go` أو أي Go implementation.

## القرارات المعتمدة

| القرار | الحكم |
|---|---|
| `ChannelConnection` بدل FacebookAccount/InstagramAccount/WhatsAppAccount | معتمد |
| Provider مستقل عن Channel | معتمد |
| Connection lifecycle متعدد الحالات | معتمد |
| عدم حذف Connection عند disconnect | معتمد |
| Capabilities runtime لكل Connection | معتمد |
| Secret Reference بدل token في Domain | معتمد |
| Inbound Event ليس Conversation أو Message | معتمد |
| فصل interaction kind عن Conversation Resolution | معتمد |
| Provider IDs مختلفة عن Mujeeb IDs | معتمد |
| Raw Payload Reference بدل raw map داخل Domain | معتمد |
| Outbound Direction/Origin/Transport منفصلة | معتمد |
| `UNKNOWN` عند الغموض الخارجي | معتمد |
| عدم retry أعمى في حالة UNKNOWN | معتمد |
| DeliveryState داخل Outbound بدل كيانات Delivery متداخلة | معتمد |
| Provider-neutral channel errors | معتمد |

## التصحيحات الإلزامية

### 1. Provider ليس Enum مغلقًا داخل Domain

التحليل يقول إننا نحتاج abstraction `SOCIALAPI`، ثم يقول إن اسم المزود التجاري لا ينبغي أن يتسرب إلى Domain enum. القرار الصحيح هو:

```text
ProviderRef
├── key: opaque string
└── version: optional
```

نستخدم قيمة مثل `socialapi` في mapping والتخزين، لكن Domain لا يحتوي قائمة تجارية مغلقة تمنع إضافة Provider جديد. Registry في Application/Adapter يحدد هل يوجد Adapter لهذا provider.

أما `Channel` فيمكن أن يكون Enum محدودًا بالـV1:

```text
facebook | instagram | whatsapp
```

لأنها قنوات Mujeeb المعتمدة حاليًا، وليس كل ما قد يدعمه Provider خارجي.

### 2. نحتاج فصل event_type عن interaction_kind

`InboundChannelEvent` قد يخبرنا عن أحداث مختلفة:

```text
event_type:
  interaction_received
  interaction_updated
  delivery_status_changed
  conversation_updated
  account_status_changed
```

بينما:

```text
interaction_kind:
  dm | comment | story_reply | mention | review | postback | other
```

ولا نضع `conversation_kind` داخل الحدث إلا إذا كان لدى Provider Conversation حقيقية. Comment قد يكون interaction فقط ويحتاج Application Conversation Resolution.

العقد المنقح:

```text
InboundChannelEvent
├── event_id
├── connection_id
├── provider_event_id
├── event_type
├── interaction_kind: optional
├── provider_message_id: optional
├── provider_conversation_id: optional
├── external_user_id: optional
├── content_reference: optional
├── external_created_at: optional
├── received_at
├── raw_payload_reference
└── signature_verified
```

### 3. Idempotency fallback ليس Content Hash عامًا

إذا أعطى Provider `provider_event_id`، فالمفتاح:

```text
provider + provider_connection_id + provider_event_id
```

إذا لم يعطه، لا ننشئ hash من النص كحل عام؛ Adapter يحدد strategy خاصة بنوع الحدث، وقد يضع الحدث `needs_reconciliation` إذا لم توجد هوية موثوقة.

### 4. SecretReference يجب أن يكون opaque

`secret://channel/conn_123` مناسب كمثال، لكن Domain لا يفسر scheme ولا يبني Secret Store. نستخدم Value Object `SecretReference` غير قابل لعرض القيمة، ويحلّه Adapter/Secret Port فقط.

### 5. DeliveryState وليس Delivery Aggregate مستقلًا

المبدأ صحيح:

```text
OutboundMessage
  └── DeliveryState
```

لكن محاولات HTTP والـbackoff و`next_retry_at` تفاصيل تشغيلية. لذلك:

```text
OutboundMessage = intent + normalized status + provider references
DeliveryAttempt = application/infrastructure record
OutboxJob = scheduling record
```

لا نضع retry policy داخل Domain. يمكن أن يحتوي OutboundMessage `attempt_count` و`last_failure_code` كحالة ملخصة، لكن `next_retry_at` يبقى في Outbox/DeliveryAttempt.

### 6. نثبت معنى الحالات

```text
PENDING   = لم تبدأ محاولة الإرسال
SENDING   = توجد محاولة فعالة
ACCEPTED  = Provider قبل الطلب
SENT      = توجد إشارة Provider أن الرسالة أُنشئت/أُرسلت
DELIVERED = توجد delivery confirmation
READ      = توجد read confirmation
FAILED    = فشل معروف ونهائي أو انتهى retry المسموح
UNKNOWN   = نتيجة خارجية غامضة بعد انقطاع/timeout
```

`SENT` و`DELIVERED` لا يتساويان. و`UNKNOWN` لا ينتقل إلى `FAILED` أو `SENT` بالتخمين؛ يمر عبر Reconciliation.

### 7. Capability تصف القدرة ولا تمنح authorization

`SEND_MESSAGES=true` يعني أن الاتصال يعلن القدرة التقنية، لكنه لا يمنح AI أو الموظف صلاحية الإرسال. Authorization يأتي من BusinessPolicy وApplication وConversation state وChannel rules.

Capabilities V1:

```text
receive_messages
send_messages
receive_comments
reply_comments
private_reply
media_inbound
media_outbound
interactive_messages
templates
delivery_status
read_status
```

كل capability لها `enabled` و`checked_at` و`evidence/source`.

## العقد النهائي لـdomain/channel

```text
internal/domain/channel/
├── channel_connection.go
├── channel_capability.go
├── inbound_event.go
├── outbound_message.go
├── delivery.go
└── channel_errors.go
```

### ChannelConnection

```text
id
business_id
provider_ref
channel
provider_account_ref
provider_connection_ref
status
capabilities
secret_reference
last_health_check_at
created_at
updated_at
```

### InboundChannelEvent

```text
event_id
connection_id
provider_event_id
event_type
interaction_kind?
provider_message_id?
provider_conversation_id?
external_user_id?
content_reference?
external_created_at?
received_at
raw_payload_reference
signature_verified
processing_state
```

### OutboundMessage

```text
id
business_id
conversation_reference_id
channel
origin
direction = outbound
transport
content_reference
provider_idempotency_key
status
provider_message_id?
chatwoot_message_id?
failure_code?
attempt_count
created_at
updated_at
```

`next_retry_at` ليس من قلب Outbound intent؛ مكانه Outbox/DeliveryAttempt.

## Domain Errors

```text
unsupported_capability
connection_unavailable
invalid_channel_state
delivery_rejected
delivery_unknown
invalid_external_reference
provider_rate_limited
provider_auth_required
```

هذه Provider-neutral. Adapter يحول أخطاء SocialAPI/Meta/Chatwoot الخاصة إلى هذه الفئات، ويحتفظ بالتفاصيل في Integration Error/Observability خارج Domain.

## علاقة Channel بـIdentity وCommunication

```text
ChannelConnection
  → InboundChannelEvent
      → Identity Resolution
          → ConversationReference
              → Communication/SalesContext
```

Channel يجيب: ماذا وصل أو ماذا نرسل؟  
Identity يجيب: من العميل؟  
Communication يجيب: إلى أي سياق محادثة ينتمي؟

لا يقرر Channel وحده إنشاء Customer أو Conversation أو Lead.

## معايير إغلاق Channel

1. لا Provider DTO داخل Domain.
2. `ProviderRef` قابل للإضافة ولا يقفل Domain على SocialAPI.
3. `Channel` محدود بالقنوات المدعومة فعليًا في V1.
4. `event_type` منفصل عن `interaction_kind`.
5. Provider Event ID هو أساس idempotency متى توفر.
6. `UNKNOWN` حالة حقيقية وتتطلب reconciliation.
7. Delivery attempts والـbackoff خارج Domain core.
8. Connection لا ترسل من حالة disconnected/failed/reconnect_required.
9. Capability لا تعني Authorization.
10. كل outbound يحتفظ بمراجع Mujeeb وProvider وChatwoot منفصلة.

## القرار

**أعتمد تحليل الذكاء الاصطناعي بعد هذه التصحيحات العشرة.** لا نكتب الكود بعد. الخطوة التالية هي مراجعة `domain/identity` و`domain/communication`، خصوصًا قواعد ربط هوية العميل عبر Facebook وInstagram وWhatsApp دون دمج خاطئ.
