# PR-020 — Provider/Chatwoot Conversation Reference Binding

**الحالة:** منفذ ومختبر محليًا على مستوى العقد؛ لا يوجد SocialAPI live ولا Chatwoot callback auto-trigger في هذه الدفعة.

## لماذا هذه الدفعة مطلوبة؟

Chatwoot conversation ID وSocialAPI provider conversation ID مراجع خارجية مختلفة. لا يجوز مساواتهما أو إرسال بحث عشوائي إلى SocialAPI. يجب أن يملك Mujeeb مرجع provider حاليًا يحمل الربط الصريح بينهما.

```text
Chatwoot account + inbox + conversation
        ↓ exact lookup داخل Mujeeb
Provider ConversationReference
        ↓ current + active + connection match
SocialAPI provider conversation + account
```

## Schema

أضيفت migration forward-only رقم `000035`:

- `chatwoot_account_id`
- `chatwoot_inbox_id`
- `chatwoot_conversation_id`

إلى `conversation_references`.

وتضيف migration:

1. check constraint يمنع القيم الفارغة عندما تُستخدم أعمدة Chatwoot.
2. unique partial index يمنع وجود provider mapping حالي مكرر لنفس business/account/inbox/conversation.
3. lookup index على business وChatwoot identifiers للمراجع provider الحالية.

لم تُعدّل migrations `000001` إلى `000034`.

## Application contract

أضيف إلى `ConversationReferenceRepository`:

```go
GetCurrentProviderByChatwoot(
    ctx,
    businessID,
    accountID,
    inboxID,
    chatwootConversationID,
)
```

وأضيف:

```go
BindProviderToChatwoot(ctx, ProviderChatwootBindingDraft)
```

الأولى للقراءة الدقيقة من callback، والثانية لربط provider reference موجودة بمعرّفات Chatwoot التي أعادها Workspace adapter. لا تنشئ العملية provider conversation ID من Chatwoot ID.

## Resolver وAutoReply bridge

`ChatwootProviderReferenceResolver` يتحقق من الآتي قبل إرجاع binding:

- business scope مطابق.
- system هو `provider`.
- reference حالي وmapping status هو `active`.
- provider reference وresource ID غير فارغين.
- connection موجود في نفس business.
- channel connection حالته `active`.
- provider/channel/account/connection متطابقة.
- Chatwoot account/inbox/conversation مطابقة للقيم المطلوبة.

`ChatwootAutoReplyBridge` يحول binding صالحًا إلى `AutoReplyCommand`. إذا لم يوجد provider reference أو كان stale أو mismatched، فالنتيجة `Blocked` مع سبب typed ولا يتم إنشاء OutboundMessage أو Outbox.

## السلوك الآمن

عند غياب المرجع:

```text
Chatwoot message محفوظ
        ↓
missing_provider_conversation_reference
        ↓
AutoReply blocked
        ↓
لا SocialAPI lookup عشوائي
لا OutboundMessage
لا Outbox
```

وعند وجود المرجع الحالي الصحيح:

```text
Chatwoot callback data
        ↓
exact Mujeeb lookup
        ↓
provider conversation reference
        ↓
SafeAutoReplyRuntime
        ↓
AIDecision
        ↓
OutboundMessage
        ↓
Outbox
```

## الاختبارات

| الحالة | النتيجة |
|---|---|
| valid provider/Chatwoot binding | PASS — unit |
| missing provider reference | PASS — blocked |
| stale/non-current reference | PASS — rejected |
| provider/connection mismatch | PASS — rejected |
| tenant mismatch | PASS — rejected/no match |
| valid bridge creates AutoReply Outbox | PASS — unit fakes |
| duplicate current mapping | ADDED — PostgreSQL integration test، التشغيل مؤجل لغياب `POSTGRES_TEST_DSN` |
| migration 000035 first run | PASS عبر schema script: applied-35 |
| migration second run | PASS: applied-0 |
| SocialAPI live | NOT RUN |
| Chatwoot live callback triggering AutoReply | NOT WIRED في هذه الدفعة |

## الحدود المقصودة

لا تعيد هذه الدفعة بناء Chatwoot inbound؛ الجزء السابق الذي يجسد callback إلى Customer وConversation وConversationReference وCommunicationMessage وEvent Ledger يبقى كما هو.

ولا توصل `ChatwootWebhookService` مباشرةً بالـbridge في هذه الدفعة؛ لأن ذلك يحتاج تحديد message direction/echo suppression وربط نتيجة materialization بالـprovider reference دون تشغيل الرد على رسائل Mujeeb الخارجة. الـbridge أصبح جاهزًا كعقد application مستقل لاستخدامه بعد اعتماد نقطة orchestration تلك.

لا يُرسل أي token أو secret إلى GitHub، ولم يُنفذ أي provider network call.
