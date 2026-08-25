# تدقيق Communication Runtime — 2026-08-25

## النتيجة المرحلية

المسار المثبت حاليًا ليس SocialAPI → Mujeeb → Chatwoot كاملًا. SocialAPIWebhookService يتحقق من التوقيع، يطبع الحدث إلى `channel.InboundEvent`، يحفظ raw payload، يحل ChannelConnection حسب provider account، ويسجل `inbound_event_ledger`. عند عدم وجود connection يسجل الحدث `unresolved`. لا توجد فيه مكالمة إلى `ChatwootInboundStore` ولا إنشاء Customer/Conversation/ConversationReference/CommunicationMessage.

ChatwootWebhookService هو المسار الذي يملك materialization الحقيقي: بعد HMAC وnormalization ينادي `ChatwootInboundStore.Materialize`. PostgreSQL ChatwootInboundStore ينشئ/يعيد استخدام Customer وConversation وConversationReference وCommunicationMessage ويسجل/يكمل Event Ledger داخل transaction واحدة. بعد ذلك، عند inbound صريح وغير خاص، يصل إلى ChatwootAutoReplyBridge ثم AIDecision/OutboundMessage/Outbox. هذا المسار مثبت بوحدة واختبار PostgreSQL مركب.

OutboxProcessor موجود ويجري claim ثم resolve ثم provider SendMessage خارج transaction ثم completion أو quarantine للـunknown outcome. هذا لا يحل فجوة SocialAPI inbound materialization.

## PASS المثبت

- SocialAPI signature verification وnormalization: unit/contract fake.
- SocialAPI account-reference resolution وEvent Ledger/raw payload recording: application unit fake.
- Chatwoot HMAC/normalization/direction/echo suppression: unit/contract.
- Chatwoot callback إلى Mujeeb materialization: PostgreSQL integration، وcallback Chatwoot محلي حقيقي في التقرير السابق.
- Chatwoot inbound إلى AutoReply إلى Outbox: PostgreSQL integration مركب.
- Outbox persistence/claim/send boundary: PostgreSQL integration وfake provider.

## أول فجوة فعلية

أول PR غير مغلق في هذا المسار هو **SocialAPI Inbound Communication Materialization / Conversation Mapping**. يجب ألا نعيد فتح Chatwoot inbound ولا Outbox ولا E2E. المطلوب هو إضافة boundary/application service وPostgreSQL implementation واختبارات للمسار:

`verified SocialAPI event → normalized event → resolved business/connection → Mujeeb Customer/Conversation/ConversationReference/CommunicationMessage + ledger state`

إن كان Chatwoot mirror outbound مطلوبًا لاحقًا، فهو PR مستقل لأن `CommunicationWorkspace` network calls لا يجوز تنفيذها داخل DB transaction. PR الحالي يجب أن يثبت Mujeeb-owned materialization فقط، ولا يدعي إنشاء Chatwoot message أو SocialAPI live delivery.

## ملاحظة تصميمية

إعادة استخدام `ChatwootInboundStore` لمسار SocialAPI غير صحيح دلاليًا لأنه يحمل Chatwoot account/inbox/contact semantics. يلزم عقد typed مستقل مثل `ProviderInboundStore` أو `SocialAPIInboundStore` يحدد business/connection/provider account/provider conversation/external user/message/content/timestamps/raw payload. لا نستخدم `map[string]any` ولا ننسخ Chatwoot schema.
